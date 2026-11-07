package app

import (
	"errors"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestSaveRecordResultUsesStructuralRecoveryState(t *testing.T) {
	for _, test := range []struct {
		name      string
		installed bool
		original  string
		temporary string
		failed    bool
		retry     bool
	}{
		{"success", true, "", "", false, false},
		{"installed cleanup error", true, "", "", true, false},
		{"installed with backup", true, "backup", "", true, false},
		{"temporary remains", false, "", "temporary", true, false},
		{"backup remains", false, "backup", "", true, false},
		{"both remain", false, "backup", "temporary", true, false},
		{"original never moved", false, "target", "", true, true},
		{"complete rollback", false, "target", "", true, true},
		{"clean preinstall failure", false, "", "", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			op, _, target := savePrefixCreate(t)
			preview := savePrefixReview(t, op)
			savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
			result := safewrite.Result{TargetPath: target, Installed: test.installed}
			if test.original == "target" {
				result.OriginalPath = target
			} else if test.original != "" {
				result.OriginalPath = target + ".backup"
			}
			if test.temporary != "" {
				result.TemporaryPath = target + ".temporary"
			}
			var issues []*diagnostic.Error
			if test.failed {
				issues = []*diagnostic.Error{{Code: "safewrite.cleanup", Reason: "Ошибка очистки", Context: diagnostic.Context{Path: result.OriginalPath}}}
			}
			// Контракт разрешает проверять фактические Result-состояния напрямую,
			// без подмены safewrite или провоцирования недетерминированных OS-сбоев.
			op.recordWriteResult(result, issues)
			snapshot := op.Snapshot()
			if snapshot.WriteResult == nil || *snapshot.WriteResult != result || snapshot.Actions.Save {
				t.Fatalf("result/consent lost: %+v", snapshot.WriteResult)
			}
			if snapshot.Actions.Retry != test.retry {
				t.Fatalf("Retry=%v, want %v", snapshot.Actions.Retry, test.retry)
			}
			if test.failed {
				savePrefixCode(t, snapshot.Blockers, "safewrite.cleanup")
				issues[0].Reason = "caller mutation"
				issues[0] = nil
				for _, issue := range op.Snapshot().Blockers {
					if issue != nil && issue.Code == "safewrite.cleanup" && issue.Reason != "Ошибка очистки" {
						t.Fatal("recorded errors share caller memory")
					}
				}
			}
			snapshot.WriteResult.TemporaryPath = "snapshot mutation"
			if *op.Snapshot().WriteResult != result {
				t.Fatal("Snapshot WriteResult aliases internal result")
			}
			_, nextIssues := op.Save(savePrefixTime())
			if len(nextIssues) == 0 {
				t.Fatal("recording result retained consent")
			}
			nextIssues = op.Retry(savePrefixTime())
			if test.retry {
				savePrefixOK(t, nextIssues)
			} else if len(nextIssues) == 0 {
				t.Fatal("Retry ignored installed/recovery state")
			}
			savePrefixAbsent(t, target)
		})
	}
}

func TestSaveRecordCopiesJoinedErrorsWithoutFlatteningCauses(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	firstCause := errors.New("first cause")
	secondCause := errors.New("second cause")
	child := &diagnostic.Error{
		Code: "moduleio.write", Reason: "Ошибка записи урока",
		Context: diagnostic.Context{Path: target, FileName: "l001.json", LessonID: "BA82"},
		Related: []diagnostic.Context{{Path: "source.json"}},
		Details: []diagnostic.Detail{{Label: "Имя", Value: "l001.json"}},
		Cause:   &os.PathError{Op: "write", Path: target, Err: firstCause},
	}
	outer := &diagnostic.Error{Code: "safewrite.write", Reason: "Ошибка кандидата", Cause: errors.Join(child, secondCause)}
	op.recordWriteResult(safewrite.Result{TargetPath: target}, []*diagnostic.Error{outer})
	child.Reason = "mutated"
	child.Related[0].Path = "mutated"
	child.Details[0].Value = "mutated"
	outer.Code = "mutated"
	snapshot := op.Snapshot()
	var stored *diagnostic.Error
	for _, issue := range snapshot.Blockers {
		if issue != nil && issue.Code == "safewrite.write" {
			stored = issue
		}
	}
	if stored == nil || !errors.Is(stored, firstCause) || !errors.Is(stored, secondCause) {
		t.Fatal("record flattened joined causes")
	}
	var pathError *os.PathError
	if !errors.As(stored, &pathError) || pathError.Path != target {
		t.Fatal("record lost typed file cause")
	}
	joined, ok := stored.Cause.(interface{ Unwrap() []error })
	if !ok || len(joined.Unwrap()) != 2 {
		t.Fatal("record lost join structure")
	}
	copied, ok := joined.Unwrap()[0].(*diagnostic.Error)
	if !ok || copied.Code != "moduleio.write" || copied.Reason != "Ошибка записи урока" || copied.Related[0].Path != "source.json" || copied.Details[0].Value != "l001.json" {
		t.Fatal("record leaked mutable owner errors")
	}
	copied.Code = "snapshot mutation"
	pathError.Path = "snapshot mutation"
	for _, issue := range op.Snapshot().Blockers {
		if issue != nil && issue.Code == "safewrite.write" {
			var actual *os.PathError
			if !errors.As(issue, &actual) || actual.Path != target {
				t.Fatal("snapshot error mutated operation")
			}
		}
	}
}
