package moduleio_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func IntegrationRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/integration/loaded.lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func IntegrationLesson(t *testing.T, raw []byte) *lesson.Lesson {
	t.Helper()
	if problems := lesson.ValidateJSON(raw, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("fixture schema: %v", problems)
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return &value
}

func IntegrationNew(t *testing.T, name, id string) *module.File {
	t.Helper()
	value := &lesson.Lesson{FormatVersion: "1.0", Document: lesson.Document{ID: id, Version: "1", Title: "Урок " + id, LineCount: "0", CreatedAt: "2026-11-01T01:02:03Z", Pages: []lesson.Page{}}}
	file, problems := module.NewFile(name, value, diagnostic.Context{Path: "private-source.json", Line: 17, Column: 4})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	return file
}

func IntegrationModel(t *testing.T, files ...*module.File) *module.Module {
	t.Helper()
	entries := make([]manifest.Entry, len(files))
	for i, file := range files {
		d := file.Lesson().Document
		order := string(rune('1' + i))
		entries[i] = manifest.Entry{ID: d.ID, FileName: file.FileName(), Order: &order, Title: d.Title, Subtitle: d.Subtitle, LineCount: d.LineCount, CreatedAt: d.CreatedAt, Version: d.Version}
	}
	sections := []manifest.Section{}
	value := &module.Module{Files: files, Manifest: manifest.Manifest{FormatVersion: "1.0", CreatedAt: "2026-11-02T03:04:05Z", Course: manifest.Course{Title: "Курс 日本語", Lessons: &entries, Sections: &sections}}}
	if problems := module.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("fixture module: %v", problems)
	}
	return value
}

func IntegrationJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type IntegrationEntry struct {
	Name   string
	Raw    []byte
	Method uint16
	Mode   os.FileMode
}

func IntegrationZIP(t *testing.T, entries ...IntegrationEntry) []byte {
	t.Helper()
	var dst bytes.Buffer
	writer := zip.NewWriter(&dst)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: entry.Method}
		if entry.Mode != 0 {
			header.SetMode(entry.Mode)
		}
		stream, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Write(entry.Raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return dst.Bytes()
}

func IntegrationPath(t *testing.T, raw []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Модуль 日本語.zip")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func IntegrationContents(t *testing.T, raw []byte) ([]string, map[string][]byte) {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	contents := map[string][]byte{}
	for _, entry := range reader.File {
		if entry.Method != zip.Deflate || !entry.Mode().IsRegular() {
			t.Fatalf("entry header: %s %v %d", entry.Name, entry.Mode(), entry.Method)
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("entry read: %v / %v", readErr, closeErr)
		}
		names = append(names, entry.Name)
		contents[entry.Name] = content
	}
	return names, contents
}

func TestIntegrationFixtureContract(t *testing.T) {
	raw := IntegrationRaw(t)
	value := IntegrationLesson(t, raw)
	loaded, problems := module.LoadedFile("l009.json", value, raw, diagnostic.Context{})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	IntegrationModel(t, loaded, IntegrationNew(t, "l002.json", "AB88"))
}
