package app_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

func destinationConsoleRegressionTime() time.Time {
	return time.Date(2026, 11, 7, 15, 0, 0, 0, time.UTC)
}

func destinationConsoleRegressionWrite(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
}

func destinationConsoleRegressionDirectory(t *testing.T, root, directory string) string {
	t.Helper()
	resolved, issue := filetree.Resolve(root, directory)
	if issue != nil {
		t.Fatal(issue)
	}
	if err := os.MkdirAll(resolved, 0700); err != nil {
		t.Fatal(err)
	}
	return resolved
}

func destinationConsoleRegressionInventory(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		observed := fmt.Sprintf("%s mode=%s size=%d mtime=%s", relative, info.Mode(), info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano))
		if !entry.IsDir() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			observed += fmt.Sprintf(" sha256=%x", sha256.Sum256(raw))
		}
		entries = append(entries, observed)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func destinationConsoleRegressionNewCreate(t *testing.T, root, sourceArgument string) *app.Operation {
	t.Helper()
	before := destinationConsoleRegressionInventory(t, root)
	t.Cleanup(func() {
		if after := destinationConsoleRegressionInventory(t, root); !slices.Equal(before, after) {
			t.Errorf("операция до согласия изменила файлы: before=%q after=%q", before, after)
		}
	})
	op, issues := app.NewCreate(root, sourceArgument, "Курс регрессионной проверки", "")
	destinationConsoleRegressionNoIssues(t, issues)
	if op == nil {
		t.Fatal("NewCreate вернул nil")
	}
	t.Cleanup(op.Cancel)
	destinationConsoleRegressionNoIssues(t, op.Analyze(destinationConsoleRegressionTime()))
	snapshot := op.Snapshot()
	if !snapshot.ManifestReady || len(snapshot.Blockers) != 0 || !snapshot.Actions.ChooseDestination || snapshot.Actions.Save || snapshot.WriteResult != nil {
		t.Fatalf("реальный golden2 fixture не готов: %+v", snapshot)
	}
	return op
}

func destinationConsoleRegressionSource(t *testing.T, root, directory string) string {
	t.Helper()
	source := destinationConsoleRegressionDirectory(t, root, directory)
	raw, err := os.ReadFile(filepath.Join("..", "golden", "2. parsed.json"))
	if err != nil {
		t.Fatal(err)
	}
	destinationConsoleRegressionWrite(t, filepath.Join(source, "source.json"), raw)
	return source
}

func destinationConsoleRegressionNoIssues(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("неожиданная диагностика: %v", issues)
	}
}

func destinationConsoleRegressionIssue(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return
		}
	}
	t.Fatalf("нет диагностики %q: %v", code, issues)
}

func destinationConsoleRegressionTarget(t *testing.T, root, directory, name string, exists bool) app.Destination {
	t.Helper()
	// Oracle — настоящий владелец разрешения путей; кавычки не снимаются.
	resolved, issue := filetree.Resolve(root, directory)
	if issue != nil {
		t.Fatal(issue)
	}
	path, issue := filetree.Resolve(resolved, name)
	if issue != nil {
		t.Fatal(issue)
	}
	return app.Destination{Directory: resolved, FileName: name, Path: path, Exists: exists}
}

func destinationConsoleRegressionWantTarget(t *testing.T, snapshot app.Snapshot, want app.Destination) {
	t.Helper()
	if snapshot.Target == nil || *snapshot.Target != want {
		t.Fatalf("цель=%+v, буквальный Resolve требует %+v", snapshot.Target, want)
	}
}

func destinationConsoleRegressionModel(t *testing.T, snapshot app.Snapshot) string {
	t.Helper()
	var result bytes.Buffer
	encoder := json.NewEncoder(&result)
	if snapshot.Module == nil || snapshot.Catalog == nil {
		t.Fatal("нет подготовленной модели")
	}
	for _, err := range []error{encoder.Encode(snapshot.Catalog), encoder.Encode(snapshot.Module.Manifest), encoder.Encode(snapshot.Resources)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range snapshot.Module.Files {
		for _, err := range []error{encoder.Encode(file.FileName()), encoder.Encode(file.Lesson()), encoder.Encode(file.OriginalBytes())} {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return result.String()
}

func destinationConsoleRegressionNoConsent(t *testing.T, op *app.Operation) {
	t.Helper()
	if snapshot := op.Snapshot(); snapshot.Actions.Save || snapshot.WriteResult != nil {
		t.Fatalf("без нового согласия запись доступна: %+v", snapshot)
	}
	result, issues := op.Save(destinationConsoleRegressionTime())
	destinationConsoleRegressionIssue(t, issues, app.CodeUnconfirmed)
	if result.Installed || result.TargetPath != "" || result.OriginalPath != "" || result.TemporaryPath != "" {
		t.Fatalf("отклонённая запись вернула файловый результат: %+v", result)
	}
}
