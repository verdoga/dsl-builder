package app

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func updateFixtureText(value string) *string { return &value }

func updateFixtureAt() time.Time {
	return time.Date(2026, 11, 7, 12, 30, 0, 0, time.FixedZone("fixture", 3*60*60))
}

func updateFixtureOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", issues)
	}
}

func updateFixtureResult(title string, subtitle *string, titles []*string, bodies [][]string) model.Result {
	result := model.Result{
		Format: model.FormatVersion1,
		Document: model.Document{DSLVersion: updateFixtureText("1.2"), Encoding: updateFixtureText("UTF-8"),
			Metadata: model.DocumentMetadata{DocumentID: updateFixtureText("Efx7"), Title: &title,
				Subtitle: subtitle, ResourceDirs: []string{}}},
		Processing: []model.Processing{}, Lines: []model.Line{}, Diagnostics: []model.Diagnostic{},
	}
	add := func(kind model.LineType, raw string, elements []model.Element) {
		result.Lines = append(result.Lines, model.Line{Number: len(result.Lines) + 1,
			LineType: kind, Raw: raw, LineEnding: model.LineEndingLF, Elements: elements})
	}
	element := func(kind model.ElementType, raw, value string, start int) model.Element {
		return model.Element{ElementType: kind, Raw: raw, Value: &value,
			Start: start, End: start + len([]rune(raw)), ErrorIDs: []string{}}
	}
	heading := func(level int, title string) {
		prefix := strings.Repeat("#", level)
		add(model.LineTypeHeading, prefix+" "+title, []model.Element{
			element(model.ElementTypeHeadingLevel, prefix, string(rune('0'+level)), 1),
			element(model.ElementTypeTitle, title, title, level+2),
		})
	}
	heading(1, title)
	if subtitle != nil {
		heading(2, *subtitle)
	}
	for page, pageTitle := range titles {
		if pageTitle != nil {
			heading(3, *pageTitle)
		} else if page > 0 {
			add(model.LineTypeTag, "@newpage", []model.Element{element(model.ElementTypeTag, "@newpage", "newpage", 1)})
		}
		for _, body := range bodies[page] {
			add(model.LineTypeTag, "@instr "+body, []model.Element{
				element(model.ElementTypeTag, "@instr", "instr", 1),
				element(model.ElementTypeContent, body, body, 8),
			})
		}
	}
	count := len(result.Lines)
	result.Document.LineCount = &count
	return result
}

func updateFixtureSource(t *testing.T, path string, result model.Result) *lesson.Lesson {
	t.Helper()
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	input, issues := parsed.Load(path)
	updateFixtureOK(t, issues)
	value, _, issues := convert.Build(input, &filetree.Resolver{}, updateFixtureAt(), diagnostic.Context{Path: path})
	updateFixtureOK(t, issues)
	return value
}

func updateFixtureArchive(t *testing.T, path string, values []*lesson.Lesson, sections []*string, orders []string) *module.Module {
	t.Helper()
	names, issues := module.AssignFileNames(nil, len(values), diagnostic.Context{Path: path})
	updateFixtureOK(t, issues)
	files := make([]*module.File, len(values))
	items := make([]catalog.Item, len(values))
	for index, value := range values {
		files[index], issues = module.NewFile(names[index], value, diagnostic.Context{Path: path})
		updateFixtureOK(t, issues)
		items[index] = catalog.Item{Entry: catalog.EntryFromLesson(value.Document, names[index], &orders[index]),
			Section: sections[index], RelativePath: names[index], Context: files[index].Context()}
	}
	state, issues := catalog.Create("Курс", "Юнит", items)
	updateFixtureOK(t, issues)
	manifest, issues := catalog.Export(state, "2026-11-01T08:00:00Z", diagnostic.Context{Path: path})
	updateFixtureOK(t, issues)
	updateFixtureOK(t, module.Validate(&module.Module{Manifest: *manifest, Files: files}, diagnostic.Context{Path: path}))
	dst, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(dst)
	writeJSON := func(name string, value any) {
		raw, err := json.MarshalIndent(value, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(append(raw, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(module.ManifestFileName, manifest)
	for _, file := range files {
		writeJSON(file.FileName(), file.Lesson())
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, issues := moduleio.Read(path)
	updateFixtureOK(t, issues)
	return loaded
}

func updateFixtureScenario(t *testing.T, before, after model.Result, sections []*string, orders []string) (string, string, string, *module.Module, *lesson.Lesson) {
	t.Helper()
	directory := t.TempDir()
	old := updateFixtureSource(t, filepath.Join(directory, "old.json"), before)
	old.Document.Version = "5"
	old.Document.CreatedAt = "2026-11-01T08:00:00Z"
	other := updateFixtureResult("Нетронутый", nil, []*string{nil}, [][]string{{"Другой материал"}})
	other.Document.Metadata.DocumentID = updateFixtureText("Other8")
	untouched := updateFixtureSource(t, filepath.Join(directory, "other.json"), other)
	archive := filepath.Join(directory, "Модуль.zip")
	loaded := updateFixtureArchive(t, archive, []*lesson.Lesson{old, untouched}, sections, orders)
	source := filepath.Join(directory, "new.json")
	candidate := updateFixtureSource(t, source, after)
	return directory, archive, source, loaded, candidate
}

func TestUpdateFixtureOwnersRoundTrip(t *testing.T) {
	before := updateFixtureResult("Старое", nil, []*string{updateFixtureText("Страница")}, [][]string{{"Текст"}})
	after := updateFixtureResult("Новое", updateFixtureText("Подзаголовок"), []*string{nil, nil}, [][]string{{"Текст"}, {"Новая страница"}})
	_, _, _, loaded, candidate := updateFixtureScenario(t, before, after, []*string{nil, nil}, []string{"010", "20"})
	if len(loaded.Files) != 2 || len(loaded.Files[0].OriginalBytes()) == 0 || len(candidate.Document.Pages) != 2 {
		t.Fatal("owner fixture did not preserve bytes or build requested pages")
	}
}
