package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed/model"
)

type consoleSaveTestFixture struct {
	op             *app.Operation
	source, target string
	old            *module.File
}

func consoleSaveTestIssues(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("owner issues: %v", issues)
	}
}

func consoleSaveTestCreate(t *testing.T, existing bool) consoleSaveTestFixture {
	t.Helper()
	root := t.TempDir()
	sources := filepath.Join(root, "Исходники с пробелами")
	if err := os.Mkdir(sources, 0755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/save/source.json")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sources, "lesson.json")
	if err := os.WriteFile(source, raw, 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !bytes.Equal(consoleSaveTestBytes(t, source), raw) {
			t.Error("source JSON changed")
		}
	})
	op, issues := app.NewCreate(root, sources, "Курс сохранения", "Юнит")
	consoleSaveTestIssues(t, issues)
	consoleSaveTestIssues(t, op.Analyze(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
	target := filepath.Join(root, "Результат модуля.zip")
	if existing {
		if err := os.WriteFile(target, []byte("previous target bytes"), 0640); err != nil {
			t.Fatal(err)
		}
	}
	return consoleSaveTestFixture{op: op, source: source, target: target}
}

func consoleSaveTestTarget(t *testing.T, f consoleSaveTestFixture) app.Snapshot {
	t.Helper()
	consoleSaveTestIssues(t, f.op.SetDestination(f.op.Snapshot().Token, filepath.Dir(f.target), filepath.Base(f.target)))
	s := f.op.Snapshot()
	if s.Target == nil || s.Target.Path != f.target || !s.Actions.ReviewSave {
		t.Fatalf("fixture not save-ready: %+v", s.Actions)
	}
	return s
}

func consoleSaveTestUpdate(t *testing.T, title, order string) consoleSaveTestFixture {
	t.Helper()
	f := consoleSaveTestCreate(t, false)
	var archive bytes.Buffer
	consoleSaveTestIssues(t, moduleio.Write(&archive, f.op.Snapshot().Module, diagnostic.Context{Path: f.target}))
	if err := os.WriteFile(f.target, archive.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	loaded, issues := moduleio.Read(f.target)
	consoleSaveTestIssues(t, issues)
	f.old = loaded.Files[0]
	raw := consoleSaveTestBytes(t, f.source)
	var result model.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if title != "" {
		result.Document.Metadata.Title = &title
	}
	if order != "" {
		result.Document.Metadata.Order = &order
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// Новый JSON подготовлен до Analyze/parsed.Load; загруженный Input не правится.
	source := filepath.Join(filepath.Dir(f.source), "updated.json")
	if err := os.WriteFile(source, raw, 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !bytes.Equal(consoleSaveTestBytes(t, source), raw) {
			t.Error("update source changed")
		}
	})
	op, issues := app.NewUpdate(filepath.Dir(f.target), f.target, source)
	consoleSaveTestIssues(t, issues)
	consoleSaveTestIssues(t, op.Analyze(time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)))
	f.op = op
	return f
}

func consoleSaveTestPreview(t *testing.T, f consoleSaveTestFixture) *app.SavePreview {
	t.Helper()
	if f.op.Snapshot().Mode == app.Create {
		consoleSaveTestTarget(t, f)
	}
	value, issues := f.op.ReviewSave(f.op.Snapshot().Token)
	consoleSaveTestIssues(t, issues)
	if value == nil {
		t.Fatal("nil owner SavePreview")
	}
	return value
}

func consoleSaveTestDialog(op *app.Operation, input io.Reader, output io.Writer) *dialog {
	return &dialog{operation: op, input: bufio.NewReader(input), output: output}
}

func consoleSaveTestBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func consoleSaveTestUnchanged(t *testing.T, path string) func() {
	t.Helper()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return func() {
			t.Helper()
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("unexpected target: %v", err)
			}
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	raw := consoleSaveTestBytes(t, path)
	return func() {
		t.Helper()
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, consoleSaveTestBytes(t, path)) || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) || info.Mode() != after.Mode() {
			t.Error("target bytes/identity/metadata changed")
		}
	}
}

type consoleSaveTestReader struct {
	lines  []string
	before func(int)
	index  int
	err    error
	reads  int
}

func (r *consoleSaveTestReader) Read(p []byte) (int, error) {
	r.reads++
	if r.index >= len(r.lines) {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}
	if r.before != nil {
		r.before(r.index)
	}
	n := copy(p, r.lines[r.index])
	r.lines[r.index] = r.lines[r.index][n:]
	if r.lines[r.index] == "" {
		r.index++
	}
	return n, nil
}

type consoleSaveTestWriter struct {
	bytes.Buffer
	calls        int
	failAt       int
	failed       bool
	afterFailure int
	cause        error
}

func (w *consoleSaveTestWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.failed {
		w.afterFailure++
		return 0, w.cause
	}
	if w.failAt > 0 && w.calls == w.failAt {
		w.failed = true
		if w.cause != nil {
			return 0, w.cause
		}
		return len(p) - 1, nil
	}
	return w.Buffer.Write(p)
}

func (w *consoleSaveTestWriter) WriteString(text string) (int, error) {
	return w.Write([]byte(text))
}

func consoleSaveTestHasCode(err error, code string) bool {
	if err == nil {
		return false
	}
	if issue, ok := err.(*diagnostic.Error); ok && issue != nil && issue.Code == code {
		return true
	}
	if tree, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range tree.Unwrap() {
			if consoleSaveTestHasCode(child, code) {
				return true
			}
		}
		return false
	}
	return consoleSaveTestHasCode(errors.Unwrap(err), code)
}
