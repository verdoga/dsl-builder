package app

import (
	"bytes"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/moduleio"
)

func updatePlacementItem(t *testing.T, state *catalog.State, fileName string) catalog.Item {
	t.Helper()
	if state == nil {
		t.Fatal("missing catalog")
	}
	var selected catalog.Item
	count := 0
	for _, item := range state.Items {
		if item.Entry.FileName == fileName {
			selected, count = item, count+1
		}
	}
	if count != 1 {
		t.Fatalf("exact file selector %s matched %d items", fileName, count)
	}
	return selected
}

func updatePlacementIndex(t *testing.T, snapshot Snapshot, field string) int {
	t.Helper()
	selected, count := -1, 0
	for index, proposal := range snapshot.PlacementProposals {
		if proposal.Field == field {
			selected, count = index, count+1
		}
	}
	if count != 1 {
		t.Fatalf("exact placement field %s matched %d proposals", field, count)
	}
	return selected
}

func TestUpdatePlacementRequiresOneExplicitDecisionPerOriginalField(t *testing.T) {
	before := updateFixtureResult("Урок", nil, []*string{nil}, [][]string{{"Текст"}})
	after := updateFixtureResult("Урок", nil, []*string{nil}, [][]string{{"Текст"}})
	after.Document.Metadata.Section = updateFixtureText("Beta")
	after.Document.Metadata.Order = updateFixtureText("030")
	op, loaded, _, archive := updateFlowStart(t, before, after, []*string{updateFixtureText("Alpha"), updateFixtureText("Beta")}, []string{"010", "20"})
	snapshot := op.Snapshot()
	want, issues := catalog.ProposePlacement(snapshot.Catalog, loaded.Files[0].FileName(), after.Document.Metadata.Section, after.Document.Metadata.Order)
	updateFixtureOK(t, issues)
	updateFlowEqual(t, snapshot.PlacementProposals, want)
	if len(want) != 2 || len(snapshot.PlacementAnswered) != 2 || snapshot.PlacementAnswered[0] || snapshot.PlacementAnswered[1] {
		t.Fatal("placement proposals were not separately pending")
	}
	edit, issues := op.ProposeCourseTitle(snapshot.Token, "Новый курс")
	if edit != nil || len(issues) == 0 {
		t.Fatal("unresolved placement allowed manual catalog edit")
	}
	sectionIndex := updatePlacementIndex(t, snapshot, "section")
	orderIndex := updatePlacementIndex(t, snapshot, "order")
	updateFixtureOK(t, op.DecidePlacement(snapshot.Token, sectionIndex, false))
	next := op.Snapshot()
	updateFlowEqual(t, next.Catalog, snapshot.Catalog)
	if next.Token != snapshot.Token {
		t.Fatal("rejection revoked accepted catalog identity")
	}
	if !next.PlacementAnswered[sectionIndex] || next.PlacementAnswered[orderIndex] {
		t.Fatal("section rejection implicitly answered weight")
	}
	updateFlowEqual(t, next.PlacementProposals, want)
	updateFixtureOK(t, op.DecidePlacement(next.Token, orderIndex, true))
	got := op.Snapshot()
	item := updatePlacementItem(t, got.Catalog, loaded.Files[0].FileName())
	if item.Section == nil || *item.Section != "Alpha" || item.Entry.Order == nil || *item.Entry.Order != "030" {
		t.Fatal("acceptance changed more than the selected placement field")
	}
	accepted := got.Catalog
	if issues := op.DecidePlacement(got.Token, sectionIndex, true); len(issues) == 0 {
		t.Fatal("resolved rejected proposal was reapplied")
	}
	if issues := op.DecidePlacement(op.Snapshot().Token, orderIndex, false); len(issues) == 0 {
		t.Fatal("resolved accepted proposal was reapplied")
	}
	updateFlowEqual(t, op.Snapshot().Catalog, accepted)
	edit, issues = op.ProposeCourseTitle(op.Snapshot().Token, "Новый курс")
	updateFixtureOK(t, issues)
	if edit == nil {
		t.Fatal("resolved proposals did not unlock manual edits")
	}
	updateFixtureOK(t, op.RejectEdit(edit.Token))
	updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
	if op.revised != op.oldFile || op.lessonChanged || op.Snapshot().NoChanges {
		t.Fatal("manifest-only update replaced lesson or lost placement change")
	}
	preview, issues := op.ReviewSave(op.Snapshot().Token)
	updateFixtureOK(t, issues)
	updateFixtureOK(t, op.ConfirmSave(preview.Token, true, true))
	result, issues := op.Save(updateFixtureAt())
	updateFixtureOK(t, issues)
	if !result.Installed {
		t.Fatal("accepted manifest change was not installed")
	}
	written, issues := moduleio.Read(archive)
	updateFixtureOK(t, issues)
	for index, file := range written.Files {
		if !bytes.Equal(file.OriginalBytes(), loaded.Files[index].OriginalBytes()) {
			t.Fatal("manifest-only save reserialized a previously loaded lesson")
		}
	}
}

func TestUpdatePlacementNonNilConflictRemainsEditableThroughRevisionSync(t *testing.T) {
	before := updateFixtureResult("До", nil, []*string{nil}, [][]string{{"Текст"}})
	after := updateFixtureResult("После", nil, []*string{nil}, [][]string{{"Новый текст"}})
	after.Document.Metadata.Section = updateFixtureText("Beta")
	after.Document.Metadata.Order = updateFixtureText("020")
	op, loaded, _, _ := updateFlowStart(t, before, after, []*string{updateFixtureText("Alpha"), updateFixtureText("Beta")}, []string{"010", "20"})
	section := updatePlacementIndex(t, op.Snapshot(), "section")
	weight := updatePlacementIndex(t, op.Snapshot(), "order")
	updateFixtureOK(t, op.DecidePlacement(op.Snapshot().Token, section, true))
	item := updatePlacementItem(t, op.Snapshot().Catalog, loaded.Files[0].FileName())
	if item.Section == nil || *item.Section != "Beta" || *item.Entry.Order != "010" || op.Snapshot().PlacementAnswered[weight] {
		t.Fatal("section acceptance applied weight without its decision")
	}
	issues := op.DecidePlacement(op.Snapshot().Token, weight, true)
	// Ошибки готовности могут возвращаться непосредственно или через Snapshot.
	if len(issues) != 0 {
		updateFlowCode(t, issues, catalog.CodeWeightConflict)
	}
	snapshot := op.Snapshot()
	if !snapshot.PlacementAnswered[weight] || len(snapshot.Conflicts) != 1 || snapshot.Catalog == nil {
		t.Fatal("non-nil conflicting owner candidate was discarded")
	}
	updateFlowDecide(t, op, []bool{false})
	issues = op.PrepareRevision(op.Snapshot().Token)
	if len(issues) != 0 {
		updateFlowCode(t, issues, catalog.CodeWeightConflict)
	}
	snapshot = op.Snapshot()
	if !snapshot.RevisionReady || snapshot.After.Lesson().Document.Version != "6" || snapshot.Actions.ReviewSave || !snapshot.Actions.EditCatalog {
		t.Fatal("Sync non-nil blockers were treated as nil failure or made saveable")
	}
	updateFlowCode(t, snapshot.Blockers, catalog.CodeWeightConflict)
	edit, issues := op.ProposeLessonWeight(snapshot.Token, loaded.Files[0].FileName(), "30")
	updateFixtureOK(t, issues)
	if edit == nil {
		t.Fatal("published conflict could not be edited")
	}
	updateFixtureOK(t, op.AcceptEdit(edit.Token))
	if len(op.Snapshot().Blockers) != 0 || !op.Snapshot().Actions.ReviewSave {
		t.Fatal("explicit weight repair did not restore save review")
	}
}

func TestUpdateSyncPreservesAcceptedManualSectionOrder(t *testing.T) {
	before := updateFixtureResult("До", nil, []*string{nil}, [][]string{{"Текст"}})
	after := updateFixtureResult("До", nil, []*string{nil}, [][]string{{"Новый текст"}})
	op, _, _, _ := updateFlowStart(t, before, after, []*string{updateFixtureText("Alpha"), updateFixtureText("Beta")}, []string{"010", "20"})
	for index := range op.Snapshot().PlacementProposals {
		updateFixtureOK(t, op.DecidePlacement(op.Snapshot().Token, index, false))
	}
	edit, issues := op.ProposeSectionWeights(op.Snapshot().Token, []catalog.SectionOrder{
		{Title: "Alpha", Weight: updateFixtureText("20")}, {Title: "Beta", Weight: updateFixtureText("10")},
	})
	updateFixtureOK(t, issues)
	updateFixtureOK(t, op.AcceptEdit(edit.Token))
	accepted := op.Snapshot().Catalog.Sections
	updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
	updateFlowEqual(t, op.Snapshot().Catalog.Sections, accepted)
	if accepted[0].Title != "Beta" || op.Snapshot().Module.Manifest.Course.Unit.Sections[0].Title != "Beta" {
		t.Fatal("revision sync restored alphabetical/imported section order")
	}
}
