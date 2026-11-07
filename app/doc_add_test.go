package app

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/module"
)

func TestDocAddLifecycleRepairsWorkingPlacementAndPreservesExistingFiles(t *testing.T) {
	base := t.TempDir()
	oldSource := filepath.Join(base, "old.json")
	docFixtureWrite(t, oldSource, docFixtureInput("DA01", "Existing", "", "", "10", "old body"))
	archive, original := docFixtureArchive(t, base, oldSource, "l003.json")
	archiveBytes := docFixtureBytes(t, archive)
	directory := filepath.Join(base, "new")
	docFixtureWrite(t, filepath.Join(directory, "b.json"), docFixtureInput("DA03", "Third", "", "", "", "third body"))
	docFixtureWrite(t, filepath.Join(directory, "a.json"), docFixtureInput("DA02", "Second", "", "", "", "second body"))
	op, issues := NewAdd(base, archive, directory)
	docFixtureOK(t, issues)
	docFixtureCode(t, op.Analyze(docFixtureAt()), catalog.CodeMissingWeight)
	snapshot := op.Snapshot()
	if snapshot.Mode != Add || snapshot.Catalog == nil || len(snapshot.Module.Files) != 3 || snapshot.ManifestReady || snapshot.Actions.ReviewSave || !snapshot.Actions.EditCatalog {
		t.Fatal("add did not retain complete editable working catalog")
	}
	if len(snapshot.NewFileNames) != 2 || snapshot.NewFileNames[0] != "l004.json" || snapshot.NewFileNames[1] != "l005.json" {
		t.Fatalf("add reused an old numbering gap: %v", snapshot.NewFileNames)
	}
	snapshot.NewFileNames[0] = "forged.json"
	if op.Snapshot().NewFileNames[0] != "l004.json" {
		t.Fatal("new names leaked mutable internal slice")
	}
	if preview, issues := op.ReviewSave(snapshot.Token); preview != nil || len(issues) == 0 {
		t.Fatal("missing weights allowed final review")
	}
	weights := map[string]string{"l003.json": "10", "l004.json": "20", "l005.json": "30"}
	proposal, issues := op.ProposeLessonWeights(snapshot.Token, nil, weights)
	docFixtureOK(t, issues)
	if proposal == nil || len(proposal.Blockers) != 0 || op.Snapshot().ManifestReady {
		t.Fatal("staged complete weights were accepted before a decision")
	}
	docFixtureOK(t, op.RejectEdit(proposal.Token))
	docFixtureCode(t, op.Snapshot().Blockers, catalog.CodeMissingWeight)
	proposal, issues = op.ProposeLessonWeights(op.Snapshot().Token, nil, weights)
	docFixtureOK(t, issues)
	docFixtureOK(t, op.AcceptEdit(proposal.Token))
	if !op.Snapshot().ManifestReady || !op.Snapshot().Actions.ReviewSave || len(op.Snapshot().Blockers) != 0 {
		t.Fatal("accepted complete placement did not unblock review")
	}
	if !bytes.Equal(archiveBytes, docFixtureBytes(t, archive)) {
		t.Fatal("add preparation changed archive")
	}
	loaded := docFixtureSave(t, op, true)
	if len(loaded.Files) != 3 {
		t.Fatalf("saved partial addition: %d files", len(loaded.Files))
	}
	wantIDs := map[string]string{"l003.json": "DA01", "l004.json": "DA02", "l005.json": "DA03"}
	for _, file := range loaded.Files {
		document := file.Lesson().Document
		if wantIDs[file.FileName()] != document.ID || document.Version.String() != "1" {
			t.Fatalf("wrong persisted identity/version: %s %+v", file.FileName(), document)
		}
		if file.FileName() == "l003.json" && !bytes.Equal(file.OriginalBytes(), original.Files[0].OriginalBytes()) {
			t.Fatal("add reserialized existing lesson")
		}
	}
}

func TestDocAddExistingIDDoesNotBecomeAnUpdate(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "old.json")
	docFixtureWrite(t, source, docFixtureInput("DA04", "Existing", "", "", "10", "old body"))
	archive, _ := docFixtureArchive(t, base, source, "l002.json")
	before := docFixtureBytes(t, archive)
	incoming := filepath.Join(base, "duplicate.json")
	docFixtureWrite(t, incoming, docFixtureInput("da04", "Replacement", "", "", "20", "different body"))
	op, issues := NewAdd(base, archive, incoming)
	docFixtureOK(t, issues)
	docFixtureCode(t, op.Analyze(docFixtureAt()), module.CodeExistingID)
	if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
		t.Fatal("rejected addition wrote a partial or updated module")
	}
	if !bytes.Equal(before, docFixtureBytes(t, archive)) || op.Snapshot().Actions.ReviewSave {
		t.Fatal("existing-ID failure changed original or authorized review")
	}
}
