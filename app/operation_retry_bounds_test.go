package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestOperationRetryRejectsNilZeroAndCancelledOperations(t *testing.T) {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var absent *Operation
	operationHasCode(t, absent.Retry(at), CodeState)
	var zero Operation
	operationHasCode(t, zero.Retry(at), CodeState)
	if zero.key != nil || zero.tree != nil || zero.usage != nil || zero.analyzed {
		t.Fatal("Retry initialized a zero Operation")
	}
	base := t.TempDir()
	op, issues := NewCreate(base, "sources", "Курс", "")
	operationNoIssues(t, issues)
	op.Cancel()
	operationHasCode(t, op.Retry(at), CodeState)
	if !op.finished || op.tree != nil || op.usage != nil {
		t.Fatal("Retry restarted a cancelled operation")
	}
}

func TestOperationRetryRejectsInstalledOrRecoveryWithoutDiscardingResult(t *testing.T) {
	for _, scenario := range []string{"installed", "temporary", "backup", "installed-and-recovery"} {
		t.Run(scenario, func(t *testing.T) {
			base := t.TempDir()
			operationWriteInput(t, filepath.Join(base, "source.json"), "BOUNDS1", "Урок", nil, nil)
			op, issues := NewCreate(base, base, "Курс", "")
			operationNoIssues(t, issues)
			at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			operationNoIssues(t, op.Analyze(at))
			result := safewrite.Result{TargetPath: filepath.Join(base, "target.zip")}
			switch scenario {
			case "installed":
				result.Installed = true
			case "temporary":
				result.TemporaryPath = filepath.Join(base, "candidate.tmp")
			case "backup":
				result.OriginalPath = filepath.Join(base, "original.backup")
			case "installed-and-recovery":
				result.Installed = true
				result.OriginalPath = filepath.Join(base, "original.backup")
				result.TemporaryPath = filepath.Join(base, "candidate.tmp")
			}
			// Явное предусловие фактического результата; owner-алгоритмы не подменены.
			op.result = &result
			op.finished = result.Installed
			oldTree, oldUsage, oldKey, oldCurrent := op.tree, op.usage, op.key, op.current
			for _, path := range []string{result.TargetPath, result.OriginalPath, result.TemporaryPath} {
				if path != "" {
					if err := os.WriteFile(path, []byte(path), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if issues := op.Retry(at.Add(time.Hour)); len(issues) == 0 {
				t.Fatal("Retry accepted installed or preserved recovery state")
			}
			if op.result == nil || *op.result != result || op.tree != oldTree || op.usage != oldUsage || op.key != oldKey || op.current != oldCurrent || !op.at.Equal(at) {
				t.Fatal("rejected Retry discarded or changed the previous result/state")
			}
			for _, path := range []string{result.TargetPath, result.OriginalPath, result.TemporaryPath} {
				if path != "" && !bytes.Equal(operationRead(t, path), []byte(path)) {
					t.Fatalf("rejected Retry altered preserved file %q", path)
				}
			}
		})
	}
}

func TestOperationRetryAllowsFullyRolledBackTarget(t *testing.T) {
	base := t.TempDir()
	operationWriteInput(t, filepath.Join(base, "source.json"), "ROLLBACK1", "Урок", nil, nil)
	op, issues := NewCreate(base, base, "Курс", "")
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	operationNoIssues(t, op.Analyze(at))
	target := filepath.Join(base, "rolled-back.zip")
	if err := os.WriteFile(target, []byte("original preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	operationNoIssues(t, op.SetDestination(op.Snapshot().Token, base, "rolled-back.zip"))
	save, issues := op.ReviewSave(op.Snapshot().Token)
	operationNoIssues(t, issues)
	operationNoIssues(t, op.ConfirmSave(save.Token, true, true))
	// Явный результат неуспешной установки после полного отката и очистки.
	// Проверяем контракт состояния, без имитации сбоя ОС или safewrite.
	op.recordWriteResult(safewrite.Result{TargetPath: target, OriginalPath: target}, []*diagnostic.Error{{
		Code: "safewrite.installCandidate", Reason: "Не удалось установить проверенный результат",
		Context: diagnostic.Context{Path: target},
	}})
	if !op.Snapshot().Actions.Retry || op.result == nil || op.result.Installed || len(op.issues) == 0 {
		t.Fatal("fixture did not retain a failed, fully rolled-back write")
	}
	operationNoIssues(t, op.Retry(at.Add(time.Hour)))
	if op.result != nil || !op.analyzed {
		t.Fatal("complete rollback was treated as residual recovery")
	}
	if !bytes.Equal(operationRead(t, target), []byte("original preserved")) {
		t.Fatal("Retry changed rolled-back target")
	}
}

func TestOperationRetryAddRereadsSourceAndOriginalArchive(t *testing.T) {
	base := t.TempDir()
	archive := operationWriteArchive(t, base, "EXISTING1")
	archiveBytes := operationRead(t, archive)
	source := filepath.Join(base, "add.json")
	if err := os.WriteFile(source, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	op, issues := NewAdd(base, `"baseline.zip"`, `"add.json"`)
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if issues := op.Analyze(at); len(issues) == 0 {
		t.Fatal("corrupt source was accepted")
	}
	oldTree, oldUsage := op.tree, op.usage
	weight := "2"
	operationWriteInput(t, source, "ADDED1", "Добавленный урок", &weight, nil)
	operationNoIssues(t, op.Retry(at.Add(time.Hour)))
	snapshot := op.Snapshot()
	if !op.analyzed || snapshot.Module == nil || len(snapshot.Module.Files) != 2 || op.tree == oldTree || op.usage == oldUsage {
		t.Fatal("Retry Add did not reload the complete composition")
	}
	if snapshot.ArchivePath != archive || snapshot.Target == nil || snapshot.Target.Path != archive || !snapshot.Target.Exists {
		t.Fatal("Retry Add lost the original archive destination")
	}
	if len(snapshot.Module.Files[0].OriginalBytes()) == 0 || !bytes.Equal(operationRead(t, archive), archiveBytes) {
		t.Fatal("Retry Add lost old lesson bytes or changed source archive")
	}
}
