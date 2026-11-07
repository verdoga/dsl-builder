package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func weightsTestString(value string) *string { return &value }

func weightsTestTime() time.Time {
	return time.Date(2026, 11, 7, 12, 13, 14, 0, time.UTC)
}

func weightsTestOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("owner diagnostics: %v", issues)
	}
}

func weightsTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func weightsTestRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func weightsTestSource(t *testing.T, directory, name, id string, section, order *string) string {
	t.Helper()
	count, text, title := 1, "Текст", "Урок "+id
	value := model.Result{
		Format: model.FormatVersion1,
		Document: model.Document{LineCount: &count, Metadata: model.DocumentMetadata{
			DocumentID: &id, Title: &title, Section: section, Order: order, ResourceDirs: []string{},
		}},
		Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Lines: []model.Line{{Number: 1, LineType: model.LineTypeContent, Raw: text,
			Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: text,
				Value: &text, Start: 1, End: 6, ErrorIDs: []string{}}}}},
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, weightsTestJSON(t, value), 0600); err != nil {
		t.Fatal(err)
	}
	_, issues := parsed.Load(path)
	weightsTestOK(t, issues)
	return path
}

func weightsTestCreate(t *testing.T, sectioned bool) (*app.Operation, []string) {
	t.Helper()
	directory := t.TempDir()
	var paths []string
	for index, raw := range []string{"20", "10", "40", "30"} {
		var section *string
		if sectioned {
			section = weightsTestString([]string{"Бета", "Альфа", "Бета", "Альфа"}[index])
		}
		paths = append(paths, weightsTestSource(t, directory, fmt.Sprintf("%d.json", index),
			fmt.Sprintf("WT%02d", index), section, weightsTestString(raw)))
	}
	op, issues := app.NewCreate(directory, directory, "Курс весов", "Юнит")
	weightsTestOK(t, issues)
	weightsTestOK(t, op.Analyze(weightsTestTime()))
	t.Cleanup(op.Cancel)
	return op, paths
}

func weightsTestAdd(t *testing.T, sectioned bool) (*app.Operation, []string) {
	t.Helper()
	return weightsTestAddWithWeight(t, sectioned, weightsTestString("50"))
}

func weightsTestAddWithWeight(t *testing.T, sectioned bool, order *string) (*app.Operation, []string) {
	t.Helper()
	initial, paths := weightsTestCreate(t, sectioned)
	var archive bytes.Buffer
	weightsTestOK(t, moduleio.Write(&archive, initial.Snapshot().Module, diagnostic.Context{}))
	directory := t.TempDir()
	path := filepath.Join(directory, "модуль.zip")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	_, issues := moduleio.Read(path)
	weightsTestOK(t, issues)
	var section *string
	if sectioned {
		section = weightsTestString("Бета")
	}
	source := weightsTestSource(t, directory, "новый.json", "WT99", section, order)
	op, issues := app.NewAdd(directory, path, source)
	weightsTestOK(t, issues)
	issues = op.Analyze(weightsTestTime())
	if order != nil {
		weightsTestOK(t, issues)
	} else if op.Snapshot().Catalog == nil || !op.Snapshot().Actions.EditCatalog {
		t.Fatalf("missing-weight fixture is not editable: %v", issues)
	}
	t.Cleanup(op.Cancel)
	return op, append(paths, path, source)
}

func weightsTestDialog(op *app.Operation, input io.Reader, output io.Writer) *dialog {
	return &dialog{input: bufio.NewReader(input), output: output, operation: op}
}

func weightsTestCapture(t *testing.T, op *app.Operation, paths []string) func() {
	t.Helper()
	before := op.Snapshot()
	catalogJSON := weightsTestJSON(t, before.Catalog)
	moduleJSON := weightsTestJSON(t, before.Module.Manifest)
	files := make(map[string][]byte)
	for _, path := range paths {
		files[path] = weightsTestRead(t, path)
	}
	return func() {
		t.Helper()
		after := op.Snapshot()
		if !bytes.Equal(catalogJSON, weightsTestJSON(t, after.Catalog)) ||
			!bytes.Equal(moduleJSON, weightsTestJSON(t, after.Module.Manifest)) {
			t.Fatal("local input changed accepted catalog or manifest")
		}
		for index, file := range before.Module.Files {
			current := after.Module.Files[index]
			if file.FileName() != current.FileName() ||
				!bytes.Equal(file.OriginalBytes(), current.OriginalBytes()) ||
				!bytes.Equal(weightsTestJSON(t, file.Lesson()), weightsTestJSON(t, current.Lesson())) {
				t.Fatal("local input changed accepted lesson or original bytes")
			}
		}
		for path, raw := range files {
			if !bytes.Equal(raw, weightsTestRead(t, path)) {
				t.Fatalf("source changed: %s", path)
			}
		}
		if after.Token != before.Token || after.Actions != before.Actions {
			t.Fatal("local cancellation changed token, consent or available actions")
		}
	}
}

func weightsTestOrder(t *testing.T, snapshot app.Snapshot, name string) *string {
	t.Helper()
	for _, item := range snapshot.Catalog.Items {
		if item.Entry.FileName == name {
			return item.Entry.Order
		}
	}
	t.Fatalf("missing file %s", name)
	return nil
}

func weightsTestConsent(t *testing.T, op *app.Operation) {
	t.Helper()
	snapshot := op.Snapshot()
	if snapshot.Mode == app.Create {
		weightsTestOK(t, op.SetDestination(snapshot.Token, t.TempDir(), "цель"))
		snapshot = op.Snapshot()
	}
	shown, issues := op.ReviewSave(snapshot.Token)
	weightsTestOK(t, issues)
	if shown == nil {
		t.Fatal("owner did not prepare save preview")
	}
	weightsTestOK(t, op.ConfirmSave(shown.Token, true, shown.Destination.Exists))
	if !op.Snapshot().Actions.Save {
		t.Fatal("fixture did not establish consent")
	}
}

func weightsTestDTO() app.Snapshot {
	return app.Snapshot{Catalog: &catalog.State{Sectioned: true,
		Sections: []catalog.SectionOrder{{Title: "Бета"}, {Title: "Альфа"}},
		Items: []catalog.Item{
			{Entry: manifest.Entry{FileName: "l004.json"}, Section: weightsTestString("Бета")},
			{Entry: manifest.Entry{FileName: "l009.json"}},
			{Entry: manifest.Entry{FileName: "l002.json"}, Section: weightsTestString("Альфа")},
			{Entry: manifest.Entry{FileName: "l007.json"}, Section: weightsTestString("Бета")},
			{Entry: manifest.Entry{FileName: "l001.json"}},
		},
	}}
}
