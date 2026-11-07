package app

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func docGuardsCreate(t *testing.T) (*Operation, string) {
	t.Helper()
	base := t.TempDir()
	source := filepath.Join(base, "in", "one.json")
	docFixtureWrite(t, source, docFixtureInput("DG01", "Title", "", "", "10", "body"))
	op, issues := NewCreate(base, filepath.Dir(source), "Course", "")
	docFixtureOK(t, issues)
	docFixtureOK(t, op.Analyze(docFixtureAt()))
	docFixtureOK(t, op.SetDestination(op.Snapshot().Token, base, "guarded"))
	return op, filepath.Join(base, "guarded.zip")
}

func TestDocTokensBelongToOneOperationAndOneAcceptedPlan(t *testing.T) {
	op, target := docGuardsCreate(t)
	foreign, _ := docGuardsCreate(t)
	for name, token := range map[string]ReviewToken{"zero": {}, "foreign": foreign.Snapshot().Token} {
		t.Run(name, func(t *testing.T) {
			if preview, issues := op.ReviewSave(token); preview != nil {
				t.Fatal("foreign/zero token produced save preview")
			} else {
				docFixtureCode(t, issues, CodeStaleReview)
			}
		})
	}
	accepted := op.Snapshot()
	copied := *op
	if preview, issues := copied.ReviewSave(accepted.Token); preview != nil {
		t.Fatal("copied operation took ownership of original token")
	} else if len(issues) == 0 {
		t.Fatal("copied operation token was not rejected")
	}
	preview, issues := op.ReviewSave(accepted.Token)
	docFixtureOK(t, issues)
	if issues := copied.ConfirmSave(preview.Token, true, false); len(issues) == 0 {
		t.Fatal("copied operation accepted original save token")
	}
	if _, issues := copied.Save(docFixtureAt()); len(issues) == 0 {
		t.Fatal("copied operation saved with original authorization")
	}
	docFixtureOK(t, op.ConfirmSave(preview.Token, true, false))
	confirmedCopy := *op
	if _, issues := confirmedCopy.Save(docFixtureAt()); len(issues) == 0 {
		t.Fatal("copy of a confirmed operation wrote its owner's archive")
	}
	docFixtureAbsent(t, target)
	if !op.Snapshot().Actions.Save {
		t.Fatal("rejected copy consumed the original operation's consent")
	}
	edit, issues := op.ProposeCourseTitle(accepted.Token, "Changed")
	docFixtureOK(t, issues)
	docFixtureOK(t, op.AcceptEdit(edit.Token))
	docFixtureCode(t, op.ConfirmSave(preview.Token, true, false), CodeStaleReview)
	if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
		t.Fatal("stale consent wrote changed plan")
	}
	docFixtureAbsent(t, target)
	loaded := docFixtureSave(t, op, false)
	if loaded.Manifest.Course.Title != "Changed" {
		t.Fatal("fresh consent did not write current accepted plan")
	}
}

func TestDocCancelCoversEOFMenuAndExitWithoutConsole(t *testing.T) {
	for _, event := range []string{"EOF", "main-menu", "exit"} {
		t.Run(event, func(t *testing.T) {
			op, target := docGuardsCreate(t)
			preview, issues := op.ReviewSave(op.Snapshot().Token)
			docFixtureOK(t, issues)
			docFixtureOK(t, op.ConfirmSave(preview.Token, true, false))
			op.Cancel()
			op.Cancel()
			if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
				t.Fatal("cancelled operation wrote archive")
			}
			if issues := op.Retry(docFixtureAt()); len(issues) == 0 {
				t.Fatal("cancelled operation restarted")
			}
			if snapshot := op.Snapshot(); snapshot.Actions.Save || snapshot.Actions.Retry || !snapshot.Actions.MainMenu || !snapshot.Actions.Exit {
				t.Fatalf("cancelled lifecycle actions: %+v", snapshot.Actions)
			}
			docFixtureAbsent(t, target)
		})
	}
}

func TestDocRecordedInstalledAndRecoveryOutcomesNeverWriteAgain(t *testing.T) {
	for _, outcome := range []struct {
		name      string
		installed bool
		temporary bool
		backup    bool
	}{{"installed-cleanup-error", true, false, true}, {"remaining-candidate", false, true, false}, {"remaining-backup", false, false, true}} {
		t.Run(outcome.name, func(t *testing.T) {
			op, target := docGuardsCreate(t)
			preview, issues := op.ReviewSave(op.Snapshot().Token)
			docFixtureOK(t, issues)
			docFixtureOK(t, op.ConfirmSave(preview.Token, true, false))
			result := safewrite.Result{TargetPath: target, Installed: outcome.installed}
			if outcome.temporary {
				result.TemporaryPath = target + ".remaining"
			}
			if outcome.backup {
				result.OriginalPath = target + ".backup"
			}
			problem := &diagnostic.Error{Code: "safewrite.cleanup", Reason: "Не удалось завершить очистку."}
			// Редкое состояние ОС передаётся штатной границе учёта результата;
			// файловая транзакция и callbacks не подменяются.
			op.recordWriteResult(result, []*diagnostic.Error{problem})
			snapshot := op.Snapshot()
			if snapshot.WriteResult == nil || *snapshot.WriteResult != result || snapshot.Actions.Save || snapshot.Actions.Retry {
				t.Fatal("actual installed/recovery outcome was hidden or made retryable")
			}
			snapshot.WriteResult.Installed = false
			snapshot.WriteResult.OriginalPath = "forged"
			if *op.Snapshot().WriteResult != result {
				t.Fatal("result snapshot mutated internal outcome")
			}
			if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
				t.Fatal("operation retried recorded write outcome")
			}
			if issues := op.Retry(docFixtureAt()); len(issues) == 0 {
				t.Fatal("analysis discarded installed/recovery result")
			}
			op.Cancel()
			if *op.Snapshot().WriteResult != result {
				t.Fatal("cancel erased recorded recovery information")
			}
			docFixtureAbsent(t, target)
		})
	}
}

func TestDocCancelAfterRealInstallationKeepsArchive(t *testing.T) {
	op, target := docGuardsCreate(t)
	docFixtureSave(t, op, false)
	before := docFixtureBytes(t, target)
	op.Cancel()
	if !bytes.Equal(before, docFixtureBytes(t, target)) || op.Snapshot().WriteResult == nil || !op.Snapshot().WriteResult.Installed {
		t.Fatal("cancel rolled back an installed result")
	}
}
