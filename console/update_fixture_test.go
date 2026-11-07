package console

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func consoleUpdateTestText(value string) *string { return &value }

func consoleUpdateTestAt() time.Time { return time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC) }

func consoleUpdateTestOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected owner diagnostics: %+v", issues)
	}
}

// consoleUpdateTestResult задаёт только входные строки fixture, не разбирает DSL.
func consoleUpdateTestResult(title string, subtitle *string, titles []*string, bodies [][]string) model.Result {
	result := model.Result{Format: model.FormatVersion1,
		Document: model.Document{DSLVersion: consoleUpdateTestText("1.2"), Encoding: consoleUpdateTestText("UTF-8"),
			Metadata: model.DocumentMetadata{DocumentID: consoleUpdateTestText("Efx7"), Title: &title,
				Subtitle: subtitle, ResourceDirs: []string{}}},
		Processing: []model.Processing{}, Lines: []model.Line{}, Diagnostics: []model.Diagnostic{}}
	element := func(kind model.ElementType, raw, value string, column int) model.Element {
		return model.Element{ElementType: kind, Raw: raw, Value: &value, Start: column,
			End: column + len([]rune(raw)), ErrorIDs: []string{}}
	}
	add := func(kind model.LineType, raw string, elements ...model.Element) {
		result.Lines = append(result.Lines, model.Line{Number: len(result.Lines) + 1,
			LineType: kind, Raw: raw, LineEnding: model.LineEndingLF, Elements: elements})
	}
	heading := func(level int, value string) {
		prefix := strings.Repeat("#", level)
		add(model.LineTypeHeading, prefix+" "+value,
			element(model.ElementTypeHeadingLevel, prefix, string(rune('0'+level)), 1),
			element(model.ElementTypeTitle, value, value, level+2))
	}
	heading(1, title)
	if subtitle != nil {
		heading(2, *subtitle)
	}
	for index, pageTitle := range titles {
		if pageTitle != nil {
			heading(3, *pageTitle)
		} else if index != 0 {
			add(model.LineTypeTag, "@newpage", element(model.ElementTypeTag, "@newpage", "newpage", 1))
		}
		for _, body := range bodies[index] {
			add(model.LineTypeTag, "@instr "+body, element(model.ElementTypeTag, "@instr", "instr", 1),
				element(model.ElementTypeContent, body, body, 8))
		}
	}
	count := len(result.Lines)
	result.Document.LineCount = &count
	return result
}

func consoleUpdateTestSource(t *testing.T, path string, result model.Result) *lesson.Lesson {
	t.Helper()
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	input, issues := parsed.Load(path)
	consoleUpdateTestOK(t, issues)
	value, _, issues := convert.Build(input, &filetree.Resolver{}, consoleUpdateTestAt(), diagnostic.Context{Path: path})
	consoleUpdateTestOK(t, issues)
	return value
}

func consoleUpdateTestOperation(t *testing.T, before, after model.Result, sectioned bool, historyFailure ...bool) *app.Operation {
	t.Helper()
	dir := t.TempDir()
	old := consoleUpdateTestSource(t, filepath.Join(dir, "old.json"), before)
	old.Document.Version, old.Document.CreatedAt = "5", "2026-11-01T08:00:00Z"
	if len(historyFailure) != 0 && historyFailure[0] {
		old.Document.Pages[0].Nodes[0].ID += "-v06"
	}
	other := consoleUpdateTestResult("Нетронутый", nil, []*string{nil}, [][]string{{"Другой материал"}})
	other.Document.Metadata.DocumentID = consoleUpdateTestText("Other8")
	untouched := consoleUpdateTestSource(t, filepath.Join(dir, "other.json"), other)
	archive := filepath.Join(dir, "Модуль.zip")
	files := make([]*module.File, 2)
	items := make([]catalog.Item, 2)
	for index, value := range []*lesson.Lesson{old, untouched} {
		name := []string{"l001.json", "l002.json"}[index]
		file, issues := module.NewFile(name, value, diagnostic.Context{Path: archive})
		consoleUpdateTestOK(t, issues)
		files[index] = file
		items[index] = catalog.Item{Entry: catalog.EntryFromLesson(value.Document, name,
			consoleUpdateTestText([]string{"010", "20"}[index])), Context: file.Context()}
		if sectioned {
			items[index].Section = consoleUpdateTestText([]string{"Alpha", "Beta"}[index])
		}
	}
	state, issues := catalog.Create("Курс", "Юнит", items)
	consoleUpdateTestOK(t, issues)
	manifest, issues := catalog.Export(state, "2026-11-01T08:00:00Z", diagnostic.Context{Path: archive})
	consoleUpdateTestOK(t, issues)
	var raw bytes.Buffer
	consoleUpdateTestOK(t, moduleio.Write(&raw, &module.Module{Manifest: *manifest, Files: files}, diagnostic.Context{Path: archive}))
	if err := os.WriteFile(archive, raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, issues := moduleio.Read(archive)
	consoleUpdateTestOK(t, issues)
	if len(loaded.Files) != 2 || len(loaded.Files[0].OriginalBytes()) == 0 {
		t.Fatal("fixture archive did not preserve real loaded bytes")
	}
	source := filepath.Join(dir, "new.json")
	consoleUpdateTestSource(t, source, after)
	op, issues := app.NewUpdate(dir, archive, source)
	consoleUpdateTestOK(t, issues)
	consoleUpdateTestOK(t, op.Analyze(consoleUpdateTestAt()))
	for _, path := range []string{archive, source, filepath.Join(dir, "old.json"), filepath.Join(dir, "other.json")} {
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("read-only input changed: %s: %v", path, err)
			}
		})
	}
	return op
}

func consoleUpdateTestPlain() model.Result {
	return consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Материал", "Второй"}})
}

func consoleUpdateTestFieldCount(t *testing.T, snapshot app.Snapshot, want int) {
	t.Helper()
	if snapshot.Plan == nil || len(snapshot.Plan.Fields) != want || len(snapshot.FieldAnswered) != want {
		t.Fatalf("fixture needs %d separate fields: %+v", want, snapshot.Plan)
	}
}
