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

func TestOperationCopyCannotUseOriginalReviewToken(t *testing.T) {
	base := t.TempDir()
	operationWriteInput(t, filepath.Join(base, "lesson.json"), "COPY1", "Урок", nil, nil)
	op, issues := NewCreate(base, base, "Курс", "")
	operationNoIssues(t, issues)
	operationNoIssues(t, op.Analyze(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))
	token := op.Snapshot().Token
	copied := *op
	preview, issues := copied.ProposeCourseTitle(token, "Чужой курс")
	if preview != nil {
		t.Fatal("copied Operation accepted the owner's token")
	}
	operationHasCode(t, issues, CodeStaleReview)
	if op.Snapshot().Catalog.CourseTitle != "Курс" || copied.edit != nil {
		t.Fatal("rejected copy action changed state")
	}
	preview, issues = op.ProposeCourseTitle(token, "Новый курс")
	operationNoIssues(t, issues)
	if preview == nil {
		t.Fatal("original owner lost its valid token")
	}
	operationNoIssues(t, op.RejectEdit(preview.Token))
	other, issues := NewCreate(base, base, "Другой курс", "")
	operationNoIssues(t, issues)
	operationNoIssues(t, other.Analyze(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))
	preview, issues = other.ProposeCourseTitle(token, "Подмена")
	if preview != nil {
		t.Fatal("different operation accepted a foreign token")
	}
	operationHasCode(t, issues, CodeStaleReview)
}

func TestOperationCancelIsNilSafeAndInvalidatesPendingTokens(t *testing.T) {
	var absent *Operation
	absent.Cancel()
	absent.Cancel()
	for _, pending := range []string{"edit", "confirmed-save"} {
		t.Run(pending, func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "lesson.json")
			raw := operationWriteInput(t, source, "CANCEL1", "Урок", nil, nil)
			op, issues := NewCreate(base, base, "Курс", "")
			operationNoIssues(t, issues)
			at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
			operationNoIssues(t, op.Analyze(at))
			accepted := op.Snapshot().Token
			var pendingToken ReviewToken
			if pending == "edit" {
				edit, issues := op.ProposeCourseTitle(accepted, "Непринятый курс")
				operationNoIssues(t, issues)
				pendingToken = edit.Token
			} else {
				operationNoIssues(t, op.SetDestination(accepted, base, "cancelled"))
				accepted = op.Snapshot().Token
				save, issues := op.ReviewSave(accepted)
				operationNoIssues(t, issues)
				pendingToken = save.Token
				operationNoIssues(t, op.ConfirmSave(pendingToken, true, false))
			}
			op.Cancel()
			op.Cancel()
			if !op.finished {
				t.Fatal("cancelled operation remains active")
			}
			snapshot := op.Snapshot()
			if !snapshot.Actions.MainMenu || !snapshot.Actions.Exit || snapshot.Actions.Save || snapshot.Actions.Retry || snapshot.Actions.EditCatalog {
				t.Fatalf("actions after cancel: %+v", snapshot.Actions)
			}
			if preview, issues := op.ProposeCourseTitle(accepted, "После отмены"); preview != nil || len(issues) == 0 {
				t.Fatal("accepted-state token survived cancellation")
			}
			if pending == "edit" {
				if issues := op.AcceptEdit(pendingToken); len(issues) == 0 {
					t.Fatal("edit token survived cancellation")
				}
			} else if issues := op.ConfirmSave(pendingToken, true, false); len(issues) == 0 {
				t.Fatal("save token survived cancellation")
			}
			operationHasCode(t, op.Retry(at.Add(time.Hour)), CodeState)
			if result, issues := op.Save(at); result.Installed || len(issues) == 0 {
				t.Fatal("cancelled operation wrote a result")
			}
			if _, err := os.Stat(filepath.Join(base, "cancelled.zip")); !os.IsNotExist(err) {
				t.Fatalf("cancel created destination: %v", err)
			}
			if !bytes.Equal(operationRead(t, source), raw) {
				t.Fatal("cancel changed source bytes")
			}
		})
	}
}

func TestOperationCancelPreservesInstalledAndRecoveryFiles(t *testing.T) {
	for _, installed := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovery", true: "installed-with-cleanup-error"}[installed], func(t *testing.T) {
			base := t.TempDir()
			op, issues := NewCreate(base, "sources", "Курс", "")
			operationNoIssues(t, issues)
			result := safewrite.Result{
				TargetPath: filepath.Join(base, "target.zip"), Installed: installed,
				OriginalPath: filepath.Join(base, "original.backup"), TemporaryPath: filepath.Join(base, "candidate.tmp"),
			}
			for _, path := range []string{result.TargetPath, result.OriginalPath, result.TemporaryPath} {
				if err := os.WriteFile(path, []byte(path), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// Предусловие результата транзакции, без имитации safewrite и сбоев ОС.
			op.result = &result
			op.issues = []*diagnostic.Error{{Code: "safewrite.cleanup", Reason: "Не завершена очистка"}}
			op.finished = installed
			op.Cancel()
			op.Cancel()
			snapshot := op.Snapshot()
			if snapshot.WriteResult == nil || *snapshot.WriteResult != result {
				t.Fatalf("cancel lost actual write result: %+v", snapshot.WriteResult)
			}
			for _, path := range []string{result.TargetPath, result.OriginalPath, result.TemporaryPath} {
				if !bytes.Equal(operationRead(t, path), []byte(path)) {
					t.Fatalf("cancel altered preserved file %q", path)
				}
			}
		})
	}
}
