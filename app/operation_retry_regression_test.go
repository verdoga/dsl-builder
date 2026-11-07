package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOperationRetryHealthyPlanRefusalPreservesConsentAndOwners(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "lesson.json")
	operationWriteInput(t, source, "HEALTHY1", "Принятый урок", nil, nil)
	op, issues := NewCreate(base, base, "Курс", "")
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	operationNoIssues(t, op.Analyze(at))
	operationNoIssues(t, op.SetDestination(op.Snapshot().Token, base, "healthy"))
	save, issues := op.ReviewSave(op.Snapshot().Token)
	operationNoIssues(t, issues)
	operationNoIssues(t, op.ConfirmSave(save.Token, true, false))
	oldTree, oldUsage, oldKey := op.tree, op.usage, op.key
	oldCurrent, oldTarget, oldSaving := op.current, op.target, op.saving
	raw := operationWriteInput(t, source, "HEALTHY1", "Внешняя правка", nil, nil)
	if len(op.Snapshot().Blockers) != 0 || op.Snapshot().Actions.Retry || !op.Snapshot().Actions.Save {
		t.Fatal("fixture must remain a healthy confirmed plan after an external source edit")
	}
	if issues := op.Retry(at.Add(time.Hour)); len(issues) == 0 {
		t.Fatal("Retry accepted a healthy plan without an error prerequisite")
	}
	if op.tree != oldTree || op.usage != oldUsage || op.key != oldKey || op.current != oldCurrent || op.target != oldTarget || op.saving != oldSaving || !op.at.Equal(at) {
		t.Fatal("refused healthy Retry changed accepted state or analysis owners")
	}
	snapshot := op.Snapshot()
	if !snapshot.Actions.Save || snapshot.Module.Files[0].Lesson().Document.Title != "Принятый урок" {
		t.Fatal("refused healthy Retry changed the prepared lesson or consent")
	}
	operationNoIssues(t, op.ConfirmSave(save.Token, true, false))
	if !bytes.Equal(operationRead(t, source), raw) {
		t.Fatal("refused healthy Retry modified the externally edited source")
	}
	if _, err := os.Stat(filepath.Join(base, "healthy.zip")); !os.IsNotExist(err) {
		t.Fatalf("refused healthy Retry wrote the destination: %v", err)
	}
}

func TestOperationRetryCopiedFailedOperationCannotResetOwnerState(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "lesson.json")
	if err := os.WriteFile(source, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	op, issues := NewCreate(base, base, "Курс", "")
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if issues := op.Analyze(at); len(issues) == 0 || !op.Snapshot().Actions.Retry {
		t.Fatal("fixture must be a failed analysis eligible for owner Retry")
	}
	copied := *op
	oldTree, oldUsage, oldKey := op.tree, op.usage, op.key
	raw := operationWriteInput(t, source, "COPYRETRY1", "Исправлен", nil, nil)
	if issues := copied.Retry(at.Add(time.Hour)); len(issues) == 0 {
		t.Fatal("copied Operation restarted the original owner's failed analysis")
	}
	if copied.tree != oldTree || copied.usage != oldUsage || copied.key != oldKey || !copied.at.Equal(at) || op.tree != oldTree || op.usage != oldUsage || op.key != oldKey || op.analyzed {
		t.Fatal("refused copied Retry changed shared owner state")
	}
	operationNoIssues(t, op.Retry(at.Add(2*time.Hour)))
	if !op.analyzed || op.tree == oldTree || op.usage == oldUsage || op.key == oldKey || op.key.owner != op {
		t.Fatal("original owner could not retry with fresh dependencies and key")
	}
	if !bytes.Equal(operationRead(t, source), raw) {
		t.Fatal("Retry changed the repaired source")
	}
}
