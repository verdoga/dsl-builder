package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveMissingDestinationDirectoryKeepsTechnicalCause(t *testing.T) {
	op, _, originalTarget := savePrefixCreate(t)
	directory := filepath.Join(filepath.Dir(originalTarget), "назначение")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	savePrefixOK(t, op.SetDestination(op.Snapshot().Token, directory, "result"))
	preview := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	result, issues := op.Save(savePrefixTime())
	if result.Installed || len(issues) == 0 {
		t.Fatal("Save accepted removed destination directory")
	}
	foundCause := false
	for _, issue := range issues {
		var cause *os.PathError
		if errors.As(issue, &cause) && errors.Is(cause, os.ErrNotExist) {
			foundCause = true
		}
	}
	if !foundCause {
		t.Fatalf("filesystem error cause lost: %v", issues)
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	_, issues = op.Save(savePrefixTime())
	if len(issues) == 0 {
		t.Fatal("filesystem failure retained final consent")
	}
	savePrefixAbsent(t, filepath.Join(directory, "result.zip"))
	savePrefixOK(t, op.SetDestination(op.Snapshot().Token, directory, "result"))
	fresh := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(fresh.Token, true, false))
	result, issues = op.Save(savePrefixTime())
	savePrefixOK(t, issues)
	if !result.Installed {
		t.Fatal("fresh consent failed after directory repair")
	}
}
