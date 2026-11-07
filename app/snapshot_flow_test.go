package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func snapshotFlowSource(t *testing.T, dir, id, title string, weight *string) string {
	t.Helper()
	zero := 0
	result := model.Result{Format: model.FormatVersion1,
		Document: model.Document{LineCount: &zero, Metadata: model.DocumentMetadata{
			DocumentID: snapshotFixtureText(id), Title: snapshotFixtureText(title),
			Order: weight, ResourceDirs: []string{}}},
		Processing: []model.Processing{}, Lines: []model.Line{}, Diagnostics: []model.Diagnostic{}}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "source.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	input, issues := parsed.Load(path)
	snapshotFixtureOK(t, issues)
	if input == nil {
		t.Fatal("fixture must be a real validated input")
	}
	return path
}

func TestSnapshotPublicCreateUpdateAndAddShowAcceptedStates(t *testing.T) {
	base, sources := t.TempDir(), t.TempDir()
	source := snapshotFlowSource(t, sources, "flow-a", "Начальное название", snapshotFixtureText("1"))
	op, issues := NewCreate(base, sources, "Курс", "")
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, op.Analyze(snapshotFixtureTime()))
	s := op.Snapshot()
	if s.Mode != Create || s.NoChanges || !s.ManifestReady || s.Module == nil || len(s.Module.Files) != 1 ||
		len(s.NewFileNames) != 0 || !s.Actions.UnusedResources || s.Resources.Total != 0 {
		t.Fatalf("created composition snapshot: %+v", s)
	}
	snapshotFixtureOK(t, op.SetDestination(s.Token, base, "модуль"))
	s = op.Snapshot()
	if s.Target == nil || s.Target.Path != filepath.Join(base, "модуль.zip") || s.Target.Exists {
		t.Fatalf("destination not shown before consent: %+v", s.Target)
	}
	preview, issues := op.ReviewSave(s.Token)
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, op.ConfirmSave(preview.Token, true, false))
	result, issues := op.Save(snapshotFixtureTime().Add(time.Hour))
	snapshotFixtureOK(t, issues)
	if !result.Installed || op.Snapshot().WriteResult == nil || !op.Snapshot().WriteResult.Installed ||
		op.Snapshot().Actions.Save || op.Snapshot().Actions.Retry {
		t.Fatal("installed result not reflected in snapshot")
	}
	loaded, issues := moduleio.Read(result.TargetPath)
	snapshotFixtureOK(t, issues)
	before := loaded.Files[0].OriginalBytes()

	update, issues := NewUpdate(base, result.TargetPath, source)
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, update.Analyze(snapshotFixtureTime().Add(2*time.Hour)))
	u := update.Snapshot()
	if !u.Actions.PrepareRevision || u.NoChanges {
		t.Fatal("update without fields still requires prepared revision before no-change result")
	}
	snapshotFixtureOK(t, update.PrepareRevision(u.Token))
	u = update.Snapshot()
	if !u.NoChanges || !u.ManifestReady || u.Actions.ReviewSave || u.Actions.Save || u.Before == nil || u.After == nil {
		t.Fatalf("same source update snapshot: %+v", u)
	}
	if string(u.After.OriginalBytes()) != string(before) || u.After.Lesson().Document.Version != "1" {
		t.Fatal("unchanged update lost old bytes/version")
	}
	changed := snapshotFlowSource(t, sources, "flow-a", "Новое название", snapshotFixtureText("1"))
	update, issues = NewUpdate(base, result.TargetPath, changed)
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, update.Analyze(snapshotFixtureTime()))
	u = update.Snapshot()
	if u.Plan == nil || len(u.Plan.Fields) != 1 || !u.Actions.DecideFields || u.FieldAnswered[0] {
		t.Fatal("title proposal did not remain an unanswered separate decision")
	}
	snapshotFixtureOK(t, update.DecideField(u.Token, 0, false))
	snapshotFixtureOK(t, update.PrepareRevision(update.Snapshot().Token))
	u = update.Snapshot()
	if !u.NoChanges || len(u.Plan.Fields) != 1 || !u.FieldAnswered[0] || u.FieldAccepted[0] || len(u.AcceptedFields) != 0 {
		t.Fatal("rejected title changed accepted model or removed original plan")
	}
	if u.After.Lesson().Document.Title != "Начальное название" || string(u.After.OriginalBytes()) != string(before) {
		t.Fatal("rejected title replaced original file")
	}

	newSource := snapshotFlowSource(t, t.TempDir(), "flow-b", "Новый урок", nil)
	add, issues := NewAdd(base, result.TargetPath, newSource)
	snapshotFixtureOK(t, issues)
	if issues := add.Analyze(snapshotFixtureTime()); len(issues) == 0 {
		t.Fatal("add without weight must expose a blocking diagnostic")
	}
	a := add.Snapshot()
	if a.Module == nil || len(a.Module.Files) != 2 || a.Catalog == nil || a.ManifestReady || a.NoChanges ||
		len(a.NewFileNames) != 1 || a.NewFileNames[0] != "l002.json" || !a.Actions.EditCatalog || !a.Actions.Retry {
		t.Fatalf("editable add snapshot incomplete: %+v", a)
	}
	edit, issues := add.ProposeLessonWeight(a.Token, "l002.json", "2")
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, add.AcceptEdit(edit.Token))
	a = add.Snapshot()
	if !a.ManifestReady || a.NoChanges || !a.Actions.ReviewSave || len(a.Blockers) != 0 || len(a.ManifestChanges) == 0 {
		t.Fatal("accepted missing-weight repair did not refresh add snapshot")
	}
}

func TestSnapshotPublicSourceFailureOffersRetryWithoutPartialComposition(t *testing.T) {
	base, sources := t.TempDir(), t.TempDir()
	op, issues := NewCreate(base, sources, "Курс", "")
	snapshotFixtureOK(t, issues)
	cold := op.Snapshot()
	if cold.Resources.Total != 0 || cold.Actions.UnusedResources || cold.Actions.Save || cold.Actions.ReviewSave {
		t.Fatalf("resources/write available before analysis: %+v", cold)
	}
	if err := os.WriteFile(filepath.Join(sources, "invalid.json"), []byte(`{"formatVersion":`), 0600); err != nil {
		t.Fatal(err)
	}
	if len(op.Analyze(snapshotFixtureTime())) == 0 {
		t.Fatal("invalid source did not block analysis")
	}
	s := op.Snapshot()
	if s.Module != nil || s.Catalog != nil || len(s.Blockers) == 0 || s.ManifestReady ||
		!s.Actions.Retry || !s.Actions.MainMenu || !s.Actions.Exit || s.Actions.Save || s.Actions.ReviewSave {
		t.Fatalf("failed analysis published partial result or hid continuation: %+v", s)
	}
}
