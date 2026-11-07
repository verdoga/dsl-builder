package app_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func integrationAt() time.Time {
	return time.Date(2026, 11, 4, 12, 30, 0, 0, time.FixedZone("fixture", 10800))
}
func integrationText(value string) *string { return &value }

func integrationOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", issues)
	}
}

func integrationCode(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return
		}
	}
	t.Fatalf("missing diagnostic %s: %+v", code, issues)
}

func integrationInput(id string, pages ...[]string) model.Result {
	result := model.Result{Format: model.FormatVersion1, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{}, Lines: []model.Line{}}
	result.Document.DSLVersion = integrationText("1.2")
	result.Document.Encoding = integrationText("UTF-8")
	noBOM := false
	result.Document.HasBOM = &noBOM
	result.Document.Metadata = model.DocumentMetadata{DocumentID: integrationText(id), Title: integrationText("Lesson " + id), ResourceDirs: []string{}}
	add := func(kind model.LineType, raw string, parent *int, elements ...model.Element) {
		line := model.Line{Number: len(result.Lines) + 1, LineType: kind, Raw: raw, LineEnding: model.LineEndingLF, ParentLine: parent, Elements: elements}
		if parent != nil {
			line.NestingLevel = 1
		}
		result.Lines = append(result.Lines, line)
	}
	element := func(kind model.ElementType, raw, value string, start int) model.Element {
		return model.Element{ElementType: kind, Raw: raw, Value: integrationText(value), Start: start, End: start + len([]rune(raw)), ErrorIDs: []string{}}
	}
	for index, texts := range pages {
		title := fmt.Sprintf("Page %d", index+1)
		add(model.LineTypeHeading, "### "+title, nil, element(model.ElementTypeHeadingLevel, "###", "3", 1), element(model.ElementTypeTitle, title, title, 5))
		add(model.LineTypeTag, "@task a", nil, element(model.ElementTypeTag, "@task", "task", 1), element(model.ElementTypeIdentifier, "a", "a", 7))
		parent := len(result.Lines)
		for _, text := range texts {
			add(model.LineTypeTag, "@instr "+text, &parent, element(model.ElementTypeTag, "@instr", "instr", 1), element(model.ElementTypeContent, text, text, 8))
		}
	}
	count := len(result.Lines)
	result.Document.LineCount = &count
	return result
}

func integrationWriteInput(t *testing.T, path string, result model.Result) {
	t.Helper()
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	integrationWrite(t, path, raw)
	input, issues := parsed.Load(path)
	integrationOK(t, issues)
	if input == nil {
		t.Fatal("parsed.Load returned nil input")
	}
}

func integrationWrite(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
}

func integrationRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationArchive(t *testing.T, base string) (string, map[string][]byte) {
	t.Helper()
	var tree filetree.Resolver
	var files []*module.File
	var items []catalog.Item
	for index, id := range []string{"OLD1", "KEEP2"} {
		path := filepath.Join(base, "seed", id+".json")
		integrationWriteInput(t, path, integrationInput(id, []string{"old body", "unchanged"}))
		input, issues := parsed.Load(path)
		integrationOK(t, issues)
		value, _, issues := convert.Build(input, &tree, integrationAt(), diagnostic.Context{Path: path})
		integrationOK(t, issues)
		name := []string{"l001.json", "l003.json"}[index]
		file, issues := module.NewFile(name, value, diagnostic.Context{Path: path})
		integrationOK(t, issues)
		files = append(files, file)
		weight := []string{"10", "20"}[index]
		items = append(items, catalog.Item{Entry: catalog.EntryFromLesson(value.Document, name, &weight), RelativePath: fmt.Sprintf("%d.json", index)})
	}
	state, issues := catalog.Create("Original course", "", items)
	integrationOK(t, issues)
	manifest, issues := catalog.Export(state, integrationAt().UTC().Format("2006-01-02T15:04:05Z"), diagnostic.Context{})
	integrationOK(t, issues)
	var buffer bytes.Buffer
	integrationOK(t, moduleio.Write(&buffer, &module.Module{Manifest: *manifest, Files: files}, diagnostic.Context{}))
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "original.zip")
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(output)
	rawFiles := map[string][]byte{}
	for _, entry := range reader.File {
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(stream)
		if err != nil {
			t.Fatal(err)
		}
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
		if entry.Name != module.ManifestFileName {
			raw = append(append([]byte(" \n\t"), raw...), []byte("\n\t ")...)
			rawFiles[entry.Name] = raw
		}
		dst, err := writer.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dst.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	_, issues = moduleio.Read(path)
	integrationOK(t, issues)
	return path, rawFiles
}

func integrationFile(t *testing.T, value *module.Module, name string) *module.File {
	t.Helper()
	for _, file := range value.Files {
		if file.FileName() == name {
			return file
		}
	}
	t.Fatalf("file %s missing", name)
	return nil
}

func integrationSameBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	if !bytes.Equal(integrationRead(t, path), want) {
		t.Fatalf("file changed: %s", path)
	}
}

func integrationAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected destination %s: %v", path, err)
	}
}

func integrationMediaInput(id string) model.Result {
	result := integrationInput(id, []string{"instruction"})
	result.Document.Metadata.ResourceDirs = []string{"../media", "../media/nested"}
	parent := 2
	result.Lines = append(result.Lines, model.Line{
		Number: 4, LineType: model.LineTypeTag, NestingLevel: 1, ParentLine: &parent,
		Raw: "@media audio clip", LineEnding: model.LineEndingLF,
		Elements: []model.Element{
			{ElementType: model.ElementTypeTag, Raw: "@media", Value: integrationText("media"), Start: 1, End: 7, ErrorIDs: []string{}},
			{ElementType: model.ElementTypeMediaType, Raw: "audio", Value: integrationText("audio"), Start: 8, End: 13, ErrorIDs: []string{}},
			{ElementType: model.ElementTypeSource, Raw: "clip", Value: integrationText("clip"), Start: 14, End: 18, ErrorIDs: []string{}},
		},
	})
	count := len(result.Lines)
	result.Document.LineCount = &count
	return result
}
