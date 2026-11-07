package app

import (
	"bytes"
	"crypto/sha256"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/parsed/model"
)

func analyzeLifecycleFiles(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	files := make(map[string][32]byte)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			files[path] = sha256.Sum256(analyzeFixtureBytes(t, path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func analyzeLifecycleUnchanged(t *testing.T, root string, before map[string][32]byte) {
	t.Helper()
	after := analyzeLifecycleFiles(t, root)
	if len(after) != len(before) {
		t.Fatalf("analysis created/removed files: %v vs %v", after, before)
	}
	for path, hash := range before {
		if after[path] != hash {
			t.Fatalf("analysis wrote %s", path)
		}
	}
}

func TestAnalyzeNeverWritesAndRepeatRequiresRetry(t *testing.T) {
	root := t.TempDir()
	source := analyzeFixtureSource(t, root, "source.json", 2, nil)
	before := analyzeLifecycleFiles(t, root)
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	if op.analyzed || op.current != nil {
		t.Fatal("constructor analyzed input")
	}
	analyzeLifecycleUnchanged(t, root, before)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	firstTree, firstUsage, firstTime := op.tree, op.usage, op.at
	firstInput := analyzeFixtureJSON(t, op.inputs[0])
	firstTitle := op.current.Files[0].Lesson().Document.Title
	analyzeLifecycleUnchanged(t, root, before)
	_ = op.Snapshot()
	analyzeLifecycleUnchanged(t, root, before)
	analyzeFixtureSource(t, root, "source.json", 2, func(result *model.Result) {
		title := "Изменено на диске"
		result.Document.Metadata.Title = &title
	})
	changedBytes := analyzeFixtureBytes(t, source)
	later := analyzeFixtureTime().Add(24 * time.Hour)
	analyzeFixtureIssues(t, op.Analyze(later), CodeState)
	if op.tree != firstTree || op.usage != firstUsage || !op.at.Equal(firstTime) || !bytes.Equal(firstInput, analyzeFixtureJSON(t, op.inputs[0])) || op.current.Files[0].Lesson().Document.Title != firstTitle {
		t.Fatal("repeat mixed old plan and new inputs")
	}
	fresh, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, fresh.Analyze(later))
	if fresh.tree == firstTree || fresh.usage == firstUsage || !fresh.at.Equal(later) || fresh.current.Files[0].Lesson().Document.Title != "Изменено на диске" {
		t.Fatal("new operation did not create fresh analysis")
	}
	if !bytes.Equal(changedBytes, analyzeFixtureBytes(t, source)) {
		t.Fatal("Retry rewrote source")
	}
}

func TestAnalyzeFailedSourceRequiresRetry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.json")
	analyzeFixtureWrite(t, path, []byte("{"))
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	if len(op.Analyze(analyzeFixtureTime())) == 0 {
		t.Fatal("invalid JSON accepted")
	}
	tree := op.tree
	analyzeFixtureSource(t, root, "source.json", 2, nil)
	analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), CodeState)
	if op.tree != tree {
		t.Fatal("repeated Analyze silently retried")
	}
	analyzeFixtureOK(t, op.Retry(analyzeFixtureTime()))
	if !op.analyzed || op.tree == tree {
		t.Fatal("Retry did not recover")
	}
}

func TestAnalyzeNilZeroAndCancelledOperations(t *testing.T) {
	var absent *Operation
	analyzeFixtureIssues(t, absent.Analyze(analyzeFixtureTime()), CodeState)
	var zero Operation
	analyzeFixtureIssues(t, zero.Analyze(analyzeFixtureTime()), CodeState)
	root := t.TempDir()
	analyzeFixtureSource(t, root, "source.json", 2, nil)
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	op.Cancel()
	analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), CodeState)
}
