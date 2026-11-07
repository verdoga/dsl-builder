package app

import (
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
)

func catalogEditCreate(t *testing.T, sections []string) *Operation {
	t.Helper()
	dir := catalogEditSources(t, sections)
	op, issues := NewCreate(dir, dir, "Курс", "Юнит")
	catalogEditOK(t, issues)
	catalogEditOK(t, op.Analyze(catalogEditTime()))
	return op
}

func catalogEditJSON(t *testing.T, value *catalog.State) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func catalogEditItem(t *testing.T, value *catalog.State, name string) catalog.Item {
	t.Helper()
	for _, item := range value.Items {
		if item.Entry.FileName == name {
			return item
		}
	}
	t.Fatalf("missing %s", name)
	return catalog.Item{}
}

func catalogEditComposition(t *testing.T, snapshot Snapshot) string {
	t.Helper()
	raw, err := json.Marshal(snapshot.Module.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	result := string(raw)
	for _, file := range snapshot.Module.Files {
		raw, err = json.Marshal(file.Lesson())
		if err != nil {
			t.Fatal(err)
		}
		result += file.FileName() + string(raw) + string(file.OriginalBytes())
	}
	return result
}

func TestCatalogEditStagesIndependentCandidateAndRejectPreservesToken(t *testing.T) {
	op := catalogEditCreate(t, []string{"", ""})
	before := op.Snapshot()
	original := catalogEditJSON(t, before.Catalog)
	composition := catalogEditComposition(t, before)
	preview, issues := op.ProposeUnitTitle(before.Token, "Новый юнит")
	catalogEditOK(t, issues)
	if preview == nil || preview.Catalog.UnitTitle == nil || *preview.Catalog.UnitTitle != "Новый юнит" {
		t.Fatal("unit proposal missing")
	}
	*preview.Catalog.UnitTitle = "внешняя правка"
	*preview.Catalog.Items[0].Entry.Order = "999"
	if len(preview.Changes) == 0 {
		t.Fatal("proposal does not describe changes")
	}
	preview.Changes[0].After[0] = '!'
	if catalogEditJSON(t, op.Snapshot().Catalog) != original || catalogEditComposition(t, op.Snapshot()) != composition {
		t.Fatal("staged or external edits mutated accepted catalog")
	}
	catalogEditOK(t, op.RejectEdit(preview.Token))
	after := op.Snapshot()
	if after.Token != before.Token || catalogEditJSON(t, after.Catalog) != original || catalogEditComposition(t, after) != composition {
		t.Fatal("reject invalidated accepted snapshot")
	}
	preview, issues = op.ProposeCourseTitle(before.Token, "Следующий курс")
	catalogEditOK(t, issues)
	if preview == nil {
		t.Fatal("accepted snapshot token became unusable after reject")
	}
}

func TestCatalogEditAcceptUsesClosedCandidateAndRevokesOldToken(t *testing.T) {
	op := catalogEditCreate(t, []string{"", ""})
	before := op.Snapshot()
	preview, issues := op.ProposeLessonWeight(before.Token, "l001.json", "0009")
	catalogEditOK(t, issues)
	*catalogEditItem(t, preview.Catalog, "l001.json").Entry.Order = "888"
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	after := op.Snapshot()
	if got := *catalogEditItem(t, after.Catalog, "l001.json").Entry.Order; got != "0009" {
		t.Fatalf("accept used exposed candidate: %q", got)
	}
	if after.Token == before.Token || after.Actions.Save {
		t.Fatal("accepted edit retained old authorization")
	}
	_, issues = op.ProposeCourseTitle(before.Token, "stale")
	catalogEditCode(t, issues, CodeStaleReview)
	catalogEditCode(t, op.AcceptEdit(preview.Token), CodeStaleReview)
}

func TestCatalogEditInvalidInputLeavesAcceptedSnapshotUsable(t *testing.T) {
	op := catalogEditCreate(t, []string{"", ""})
	before := op.Snapshot()
	for _, run := range []struct {
		name    string
		propose func() (*EditPreview, bool)
	}{
		{"course", func() (*EditPreview, bool) { p, e := op.ProposeCourseTitle(before.Token, " \t"); return p, len(e) != 0 }},
		{"unit", func() (*EditPreview, bool) { p, e := op.ProposeUnitTitle(before.Token, " \t"); return p, len(e) != 0 }},
		{"weight", func() (*EditPreview, bool) {
			p, e := op.ProposeLessonWeight(before.Token, "l001.json", "0")
			return p, len(e) != 0
		}},
		{"unknown lesson", func() (*EditPreview, bool) {
			p, e := op.ProposeLessonWeight(before.Token, "l999.json", "5")
			return p, len(e) != 0
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			preview, failed := run.propose()
			if preview != nil || !failed {
				t.Fatal("invalid input produced a candidate")
			}
			after := op.Snapshot()
			if after.Token != before.Token || catalogEditJSON(t, after.Catalog) != catalogEditJSON(t, before.Catalog) {
				t.Fatal("invalid input changed accepted state")
			}
		})
	}
	preview, issues := op.ProposeUnitTitle(before.Token, "")
	catalogEditOK(t, issues)
	if preview == nil || preview.Catalog.UnitTitle != nil {
		t.Fatal("empty unit title must propose removal")
	}
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	if op.Snapshot().Catalog.UnitTitle != nil {
		t.Fatal("unit removal not accepted")
	}
}
