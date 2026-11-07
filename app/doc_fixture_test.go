package app

import (
	"archive/zip"
	"bytes"
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
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func docFixtureAt() time.Time {
	return time.Date(2026, 11, 4, 12, 30, 0, 0, time.UTC)
}

func docFixtureOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %v", issues)
	}
}

func docFixtureCode(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return
		}
	}
	t.Fatalf("want code %q, got %v", code, issues)
}

func docFixtureText(value string) *string { return &value }

func docFixtureInput(id, title, subtitle, section, order, body string) model.Result {
	result := model.Result{Format: "1.0", Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{}}
	result.Document.DSLVersion = docFixtureText("1.2")
	result.Document.Encoding = docFixtureText("UTF-8")
	result.Document.Metadata = model.DocumentMetadata{
		DocumentID: docFixtureText(id), Title: docFixtureText(title), ResourceDirs: []string{},
	}
	add := func(kind model.LineType, raw string, elements ...model.Element) {
		result.Lines = append(result.Lines, model.Line{
			Number: len(result.Lines) + 1, LineType: kind, Raw: raw, LineEnding: "\n", Elements: elements,
		})
	}
	element := func(kind model.ElementType, raw, value string, start int) model.Element {
		return model.Element{ElementType: kind, Raw: raw, Value: docFixtureText(value),
			Start: start, End: start + len([]rune(raw)), ErrorIDs: []string{}}
	}
	add("tag", "@document-id "+id, element("tag", "@document-id", "document-id", 1), element("identifier", id, id, 14))
	add("heading", "# "+title, element("heading-level", "#", "1", 1), element("title", title, title, 3))
	if subtitle != "" {
		result.Document.Metadata.Subtitle = docFixtureText(subtitle)
		add("heading", "## "+subtitle, element("heading-level", "##", "2", 1), element("title", subtitle, subtitle, 4))
	}
	for _, field := range []struct{ tag, kind, value string }{{"section", "name", section}, {"order", "number", order}} {
		if field.value == "" {
			continue
		}
		add("tag", "@"+field.tag+" "+field.value,
			element("tag", "@"+field.tag, field.tag, 1), element(model.ElementType(field.kind), field.value, field.value, len(field.tag)+3))
	}
	if section != "" {
		result.Document.Metadata.Section = docFixtureText(section)
	}
	if order != "" {
		result.Document.Metadata.Order = docFixtureText(order)
	}
	add("content", body, element("content", body, body, 1))
	count := len(result.Lines)
	result.Document.LineCount = &count
	return result
}

func docFixtureWrite(t *testing.T, path string, result model.Result) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func docFixtureBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func docFixtureAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected filesystem result for %s: %v", path, err)
	}
}

func docFixtureArchive(t *testing.T, base, source, fileName string) (string, *module.Module) {
	t.Helper()
	input, issues := parsed.Load(source)
	docFixtureOK(t, issues)
	value, _, issues := convert.Build(input, &filetree.Resolver{}, docFixtureAt(), diagnostic.Context{FileName: fileName})
	docFixtureOK(t, issues)
	file, issues := module.NewFile(fileName, value, diagnostic.Context{Path: source})
	docFixtureOK(t, issues)
	placement, issues := catalog.Create("Курс", "", []catalog.Item{{
		Entry:   catalog.EntryFromLesson(value.Document, fileName, input.Result.Document.Metadata.Order),
		Section: input.Result.Document.Metadata.Section, RelativePath: filepath.Base(source),
	}})
	docFixtureOK(t, issues)
	manifest, issues := catalog.Export(placement, docFixtureAt().Format(time.RFC3339), diagnostic.Context{})
	docFixtureOK(t, issues)
	whole := &module.Module{Manifest: *manifest, Files: []*module.File{file}}
	docFixtureOK(t, module.Validate(whole, diagnostic.Context{}))
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, entry := range []struct {
		name  string
		value any
	}{{"manifest.json", manifest}, {fileName, value}} {
		stream, err := writer.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.MarshalIndent(entry.value, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Write(append(raw, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "исходный.zip")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, issues := moduleio.Read(path)
	docFixtureOK(t, issues)
	if !strings.Contains(string(loaded.Files[0].OriginalBytes()), "\t") {
		t.Fatal("baseline must preserve distinctive source JSON formatting")
	}
	return path, loaded
}

func docFixtureSave(t *testing.T, op *Operation, overwrite bool) *module.Module {
	t.Helper()
	preview, issues := op.ReviewSave(op.Snapshot().Token)
	docFixtureOK(t, issues)
	if preview == nil {
		t.Fatal("missing save preview")
	}
	docFixtureOK(t, op.ConfirmSave(preview.Token, true, overwrite))
	result, issues := op.Save(docFixtureAt().Add(time.Hour))
	docFixtureOK(t, issues)
	if !result.Installed || result.TemporaryPath != "" || result.OriginalPath != "" {
		t.Fatalf("incomplete real save: %+v", result)
	}
	loaded, issues := moduleio.Read(result.TargetPath)
	docFixtureOK(t, issues)
	return loaded
}
