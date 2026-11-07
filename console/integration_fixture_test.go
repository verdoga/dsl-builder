package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func integrationTestPointer[T any](v T) *T { return &v }
func integrationTestIssues(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("real owner issues: %+v", issues)
	}
}

// JSON is built before Load, with no DSL parsing or loaded-model mutation.
func integrationTestSource(t *testing.T, directory, id, title, body string, section, weight *string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if weight == nil {
		weight = integrationTestPointer("1")
	}
	value := model.Result{
		Format: model.FormatVersion1,
		Document: model.Document{DSLVersion: integrationTestPointer("1.2"), Encoding: integrationTestPointer("UTF-8"), HasBOM: integrationTestPointer(false), LineCount: integrationTestPointer(1),
			Metadata: model.DocumentMetadata{DocumentID: &id, Title: &title, Section: section, Order: weight, ResourceDirs: []string{}}},
		Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Lines: []model.Line{{Number: 1, LineType: model.LineTypeContent, Raw: body, LineEnding: model.LineEndingNone, Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: body, Value: &body, Start: 1, End: len([]rune(body)) + 1, ErrorIDs: []string{}}}}},
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, id+".json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, issues := parsed.Load(path)
	integrationTestIssues(t, issues)
	return path
}

func integrationTestCreateOperation(t *testing.T, base, source, course, unit string) *app.Operation {
	t.Helper()
	op, issues := app.NewCreate(base, source, course, unit)
	integrationTestIssues(t, issues)
	integrationTestIssues(t, op.Analyze(time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)))
	return op
}
func integrationTestSeed(t *testing.T, base, source, name string) string {
	t.Helper()
	op := integrationTestCreateOperation(t, base, source, "Старый курс", "")
	integrationTestIssues(t, op.SetDestination(op.Snapshot().Token, base, name))
	p, issues := op.ReviewSave(op.Snapshot().Token)
	integrationTestIssues(t, issues)
	if p == nil {
		t.Fatal("real seed SavePreview absent")
	}
	integrationTestIssues(t, op.ConfirmSave(p.Token, true, false))
	result, issues := op.Save(time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC))
	integrationTestIssues(t, issues)
	if !result.Installed {
		t.Fatal("real seed not installed")
	}
	return filepath.Join(base, name)
}
func integrationTestRead(t *testing.T, path string) *module.Module {
	t.Helper()
	value, issues := moduleio.Read(path)
	integrationTestIssues(t, issues)
	if value == nil {
		t.Fatal("moduleio.Read returned nil")
	}
	return value
}
func integrationTestBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func integrationTestNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected target or stat error: %s: %v", path, err)
	}
}
