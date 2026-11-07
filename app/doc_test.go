package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/moduleio"
)

func TestDocCreateLifecycleKeepsProposalsSeparateUntilAccepted(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "вход", "урок.json")
	raw := docFixtureWrite(t, source, docFixtureInput("DC01", "Урок", "", "", "", "Текст"))
	op, issues := NewCreate(base, filepath.Dir(source), "Курс", "")
	docFixtureOK(t, issues)
	docFixtureOK(t, op.Analyze(docFixtureAt()))
	before := op.Snapshot()
	if before.Mode != Create || !before.ManifestReady || before.Catalog.CourseTitle != "Курс" || len(before.Module.Files) != 1 {
		t.Fatalf("incomplete analyzed snapshot: %+v", before)
	}
	if !before.Actions.EditCatalog || !before.Actions.ChooseDestination || before.Actions.Save {
		t.Fatalf("unexpected lifecycle actions: %+v", before.Actions)
	}
	proposal, issues := op.ProposeCourseTitle(before.Token, "Отклонённый курс")
	docFixtureOK(t, issues)
	if proposal == nil || proposal.Catalog.CourseTitle != "Отклонённый курс" || op.Snapshot().Catalog.CourseTitle != "Курс" {
		t.Fatal("proposal changed the accepted catalog")
	}
	if _, issues := op.ReviewSave(before.Token); len(issues) == 0 {
		t.Fatal("save review accepted while edit is pending")
	}
	docFixtureOK(t, op.RejectEdit(proposal.Token))
	if after := op.Snapshot(); after.Token != before.Token || after.Catalog.CourseTitle != "Курс" {
		t.Fatal("reject changed accepted snapshot")
	}
	proposal, issues = op.ProposeCourseTitle(before.Token, "Принятый курс")
	docFixtureOK(t, issues)
	proposal.Catalog.CourseTitle = "Внешняя подмена"
	docFixtureOK(t, op.AcceptEdit(proposal.Token))
	accepted := op.Snapshot()
	if accepted.Catalog.CourseTitle != "Принятый курс" || accepted.Token == before.Token {
		t.Fatal("accept did not publish its closed candidate")
	}
	docFixtureCode(t, op.SetDestination(before.Token, base, "результат"), CodeStaleReview)
	target := filepath.Join(base, "результат.zip")
	docFixtureOK(t, op.SetDestination(accepted.Token, base, "результат"))
	docFixtureAbsent(t, target)
	preview, issues := op.ReviewSave(op.Snapshot().Token)
	docFixtureOK(t, issues)
	if preview == nil || preview.Destination.Path != target || preview.Destination.Exists {
		t.Fatalf("wrong destination preview: %+v", preview)
	}
	docFixtureOK(t, op.ConfirmSave(preview.Token, false, false))
	if op.Snapshot().Actions.Save {
		t.Fatal("declining write left save authorized")
	}
	if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
		t.Fatal("declined write was attempted")
	}
	docFixtureAbsent(t, target)
	loaded := docFixtureSave(t, op, false)
	if loaded.Manifest.Course.Title != "Принятый курс" || loaded.Files[0].FileName() != "l001.json" || loaded.Files[0].Lesson().Document.Title != "Урок" {
		t.Fatal("saved archive differs from accepted preview")
	}
	if !bytes.Equal(raw, docFixtureBytes(t, source)) {
		t.Fatal("create changed input JSON")
	}
	installed := docFixtureBytes(t, target)
	if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
		t.Fatal("finished operation wrote twice")
	}
	if !bytes.Equal(installed, docFixtureBytes(t, target)) || op.Snapshot().WriteResult == nil || !op.Snapshot().WriteResult.Installed {
		t.Fatal("second save changed archive or lost actual result")
	}
	docFixtureOK(t, moduleio.Verify(target))
}

func TestDocSnapshotMutationCannotAuthorizeOrPatchCreate(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "one.json")
	docFixtureWrite(t, source, docFixtureInput("DC02", "Original", "Sub", "", "10", "body"))
	op, issues := NewCreate(base, filepath.Dir(source), "Course", "Unit")
	docFixtureOK(t, issues)
	docFixtureOK(t, op.Analyze(docFixtureAt()))
	snapshot := op.Snapshot()
	snapshot.Catalog.CourseTitle = "forged"
	*snapshot.Catalog.UnitTitle = "forged"
	*snapshot.Catalog.Items[0].Entry.Order = "999"
	snapshot.Module.Manifest.Course.Title = "forged"
	snapshot.Module.Manifest.Course.Unit.Lessons[0].Title = "forged"
	snapshot.Module.Files[0].Lesson().Document.Title = "forged"
	snapshot.Module.Files[0] = nil
	snapshot.Actions.Save = true
	snapshot.ManifestReady = true
	snapshot.Blockers = nil
	current := op.Snapshot()
	if current.Catalog.CourseTitle != "Course" || *current.Catalog.UnitTitle != "Unit" || *current.Catalog.Items[0].Entry.Order != "10" || current.Module.Files[0].Lesson().Document.Title != "Original" {
		t.Fatal("snapshot leaked mutable catalog/module state")
	}
	if current.Module.Manifest.Course.Unit.Lessons[0].Title != "Original" || current.Actions.Save {
		t.Fatal("external preview modified internal manifest/authorization")
	}
	target := filepath.Join(base, "closed.zip")
	docFixtureOK(t, op.SetDestination(current.Token, base, "closed"))
	if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
		t.Fatal("snapshot mutation authorized save")
	}
	docFixtureAbsent(t, target)
	loaded := docFixtureSave(t, op, false)
	if loaded.Manifest.Course.Title != "Course" || loaded.Manifest.Course.Unit.Lessons[0].Title != "Original" {
		t.Fatal("untrusted snapshot values reached real ZIP")
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected transaction leftovers: %v", entries)
	}
}
