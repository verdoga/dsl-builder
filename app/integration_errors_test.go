package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/moduleio"
)

func TestIntegrationInitialPlacementFailsBeforeMissingMedia(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	first := integrationMediaInput("MEDIA1")
	first.Document.Metadata.Order = integrationText("10")
	integrationWriteInput(t, filepath.Join(source, "a.json"), first)
	integrationWriteInput(t, filepath.Join(source, "b.json"), integrationMediaInput("MEDIA2"))
	op, issues := app.NewCreate(base, source, "Course", "")
	integrationOK(t, issues)
	issues = op.Analyze(integrationAt())
	integrationCode(t, issues, catalog.CodePartialWeight)
	for _, issue := range issues {
		if issue.Code != catalog.CodePartialWeight {
			t.Fatalf("later conversion ran after invalid initial placement: %+v", issue)
		}
	}
	if op.Snapshot().Module != nil && len(op.Snapshot().Module.Files) > 0 {
		t.Fatal("published partial conversion")
	}
}

func TestIntegrationUnreadableSourceDiagnosticsAreIndependentAndRetryable(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	integrationWriteInput(t, filepath.Join(source, "one.json"), integrationInput("ONE1", []string{"body"}))
	op, issues := app.NewCreate(base, source, "Course", "")
	integrationOK(t, issues)
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	issues = op.Analyze(integrationAt())
	if len(issues) == 0 {
		t.Fatal("missing source accepted")
	}
	snapshot := op.Snapshot()
	if len(snapshot.Blockers) == 0 || !snapshot.Actions.Retry || snapshot.Actions.ReviewSave {
		t.Fatal("source error state unavailable")
	}
	var sourcePathError *os.PathError
	found := -1
	for index, issue := range snapshot.Blockers {
		if errors.As(issue, &sourcePathError) {
			found = index
			break
		}
	}
	if found < 0 {
		t.Fatal("structured OS error cause discarded")
	}
	oldReason := snapshot.Blockers[found].Reason
	sourcePathError.Path = "forged-path"
	snapshot.Blockers[found].Reason = "forged-reason"
	snapshot.Blockers[found].Related = append(snapshot.Blockers[found].Related, diagnostic.Context{Path: "forged-related"})
	fresh := op.Snapshot()
	var freshPathError *os.PathError
	if !errors.As(fresh.Blockers[found], &freshPathError) || freshPathError.Path == "forged-path" || fresh.Blockers[found].Reason != oldReason {
		t.Fatal("mutable diagnostic/OS cause escaped")
	}
	integrationWriteInput(t, filepath.Join(source, "one.json"), integrationInput("ONE1", []string{"fixed"}))
	integrationOK(t, op.Retry(integrationAt()))
	if len(op.Snapshot().Blockers) != 0 || !op.Snapshot().ManifestReady {
		t.Fatal("Retry kept old source diagnostics")
	}
}

func TestIntegrationInvalidExistingZIPCannotBeEditedOrOverwritten(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "bad.zip")
	raw := []byte("not a ZIP")
	integrationWrite(t, archive, raw)
	path := filepath.Join(base, "new.json")
	integrationWriteInput(t, path, integrationInput("NEW1", []string{"body"}))
	for _, mode := range []string{"add", "update"} {
		t.Run(mode, func(t *testing.T) {
			op, issues := app.NewAdd(base, archive, path)
			if mode == "update" {
				op, issues = app.NewUpdate(base, archive, path)
			}
			integrationOK(t, issues)
			if issues = op.Analyze(integrationAt()); len(issues) == 0 {
				t.Fatal("bad old ZIP accepted")
			}
			if op.Snapshot().Actions.EditCatalog || op.Snapshot().Actions.ReviewSave {
				t.Fatal("bad archive became editable")
			}
			_, issues = op.Save(integrationAt())
			if len(issues) == 0 {
				t.Fatal("bad archive overwritten")
			}
			integrationSameBytes(t, archive, raw)
		})
	}
	_, issues := moduleio.Read(archive)
	if len(issues) == 0 {
		t.Fatal("fixture is unexpectedly valid")
	}
}
