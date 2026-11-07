package console_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/preview"
)

func docTestSource(t *testing.T, base, name string, replacements [][2]string, configure func(*model.Result)) string {
	t.Helper()
	raw := docTestRead(t, "testdata/doc/source.json")
	for _, pair := range replacements {
		raw = bytes.ReplaceAll(raw, []byte(pair[0]), []byte(pair[1]))
	}
	var value model.Result
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if configure != nil {
		configure(&value)
	}
	// Поправки JSON делаются до parsed.Load; диапазон raw остаётся точным.
	for i := range value.Lines {
		for j := range value.Lines[i].Elements {
			element := &value.Lines[i].Elements[j]
			element.End = element.Start + utf8.RuneCountInString(element.Raw)
		}
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !bytes.Equal(raw, docTestRead(t, path)) {
			t.Errorf("source changed: %s", path)
		}
	})
	return path
}

func docTestRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func docTestIssues(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("real owner fixture: %v", issues)
	}
}

func docTestCreate(t *testing.T, base, directory, course string) *app.Operation {
	t.Helper()
	op, issues := app.NewCreate(base, directory, course, "")
	docTestIssues(t, issues)
	if op == nil {
		t.Fatal("nil fixture operation")
	}
	t.Cleanup(op.Cancel)
	docTestIssues(t, op.Analyze(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)))
	return op
}

func docTestBaseline(t *testing.T, base string) (string, *module.Module) {
	t.Helper()
	docTestSource(t, base, "baseline/a.json", nil, nil)
	docTestSource(t, base, "baseline/b.json", [][2]string{{"DOC39", "KEEP39"}, {"Урок исходный", "Сохраняемый урок"}}, nil)
	op := docTestCreate(t, base, filepath.Join(base, "baseline"), "Курс исходный")
	docTestIssues(t, op.SetDestination(op.Snapshot().Token, base, "исходный модуль"))
	review, issues := op.ReviewSave(op.Snapshot().Token)
	docTestIssues(t, issues)
	if review == nil {
		t.Fatal("nil baseline SavePreview")
	}
	docTestIssues(t, op.ConfirmSave(review.Token, true, false))
	result, issues := op.Save(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	docTestIssues(t, issues)
	if !result.Installed {
		t.Fatal("baseline not installed")
	}
	return result.TargetPath, docTestModule(t, result.TargetPath)
}

func docTestModule(t *testing.T, path string) *module.Module {
	t.Helper()
	value, issues := moduleio.Read(path)
	docTestIssues(t, issues)
	if value == nil {
		t.Fatal("moduleio.Read returned nil")
	}
	return value
}

func docTestFile(t *testing.T, value *module.Module, name string) *module.File {
	t.Helper()
	for _, file := range value.Files {
		if file.FileName() == name {
			return file
		}
	}
	t.Fatalf("missing %s", name)
	return nil
}

func docTestSameOldFiles(t *testing.T, before, after *module.Module, names ...string) {
	t.Helper()
	for _, name := range names {
		raw := docTestFile(t, before, name).OriginalBytes()
		if len(raw) == 0 || !bytes.Equal(raw, docTestFile(t, after, name).OriginalBytes()) {
			t.Fatalf("old bytes changed: %s", name)
		}
	}
}

func docTestUpdate(t *testing.T, base, archive, source string, answers []bool) *app.Operation {
	t.Helper()
	op, issues := app.NewUpdate(base, archive, source)
	docTestIssues(t, issues)
	if op == nil {
		t.Fatal("nil update fixture")
	}
	t.Cleanup(op.Cancel)
	docTestIssues(t, op.Analyze(time.Date(2026, 11, 4, 12, 1, 0, 0, time.UTC)))
	snapshot := op.Snapshot()
	if snapshot.Plan == nil || len(snapshot.Plan.Fields) != len(answers) {
		t.Fatalf("fixture fields: %+v; answers=%v", snapshot.Plan, answers)
	}
	for i, accept := range answers {
		docTestIssues(t, op.DecideField(op.Snapshot().Token, i, accept))
	}
	docTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
	if !op.Snapshot().RevisionReady {
		t.Fatal("fixture revision not ready")
	}
	return op
}

// docTestFullReview получает ожидаемый текст только от настоящих app/preview.
// Различаться могут часы двух последовательных анализов; иных сокращений нет.
func docTestFullReview(t *testing.T, op *app.Operation) func(string) {
	t.Helper()
	review, issues := op.ReviewSave(op.Snapshot().Token)
	docTestIssues(t, issues)
	if review == nil {
		t.Fatal("nil expected SavePreview")
	}
	s := review.Snapshot
	composition, err := preview.Composition(s.Catalog, s.Module, s.NewFileNames)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := preview.ManifestChanges(s.ManifestChanges)
	if err != nil {
		t.Fatal(err)
	}
	blocks := []string{composition, preview.OrderConflicts(s.Conflicts), manifest, preview.Resources(s.Resources)}
	if s.Mode == app.Update {
		if !s.RevisionReady {
			t.Fatal("expected update revision not ready")
		}
		heading, err := preview.UpdateHeading(s.ArchivePath, s.Before, s.After)
		if err != nil {
			t.Fatal(err)
		}
		fields, err := preview.AcceptedFields(s.Before, s.After, s.AcceptedFields)
		if err != nil {
			t.Fatal(err)
		}
		replacements, err := preview.Replacements(s.Before, s.After, s.Plan)
		if err != nil {
			t.Fatal(err)
		}
		blocks = append(blocks, heading, fields, replacements)
	}
	normalize := func(text string) string {
		return regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`).ReplaceAllString(text, "<UTC>")
	}
	return func(screen string) {
		for _, block := range blocks {
			if block != "" && !strings.Contains(normalize(screen), normalize(block)) {
				t.Fatalf("full SavePreview block missing before consent:\n%s\nSCREEN:\n%s", block, screen)
			}
		}
		docTestRequire(t, screen, review.Destination.Path)
		if review.Destination.Exists && !regexp.MustCompile(`(?i)(существ|перезапис)`).MatchString(screen) {
			t.Fatal("existing destination not identified before consent")
		}
		if review.ManifestTimeWillChange && (!strings.Contains(strings.ToLower(screen), "дат") || !strings.Contains(strings.ToLower(screen), "манифест")) {
			t.Fatal("future manifest date not explained")
		}
	}
}

func docTestAssertUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	if !bytes.Equal(before, docTestRead(t, path)) {
		t.Fatalf("changed without consent: %s", path)
	}
}

func docTestAssertAbsent(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected absent %s; got %v", path, err)
	}
}
