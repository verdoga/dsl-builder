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
	"github.com/verdoga/dsl-builder/moduleio"
)

type GoldenEntry struct {
	name string
	raw  []byte
}

func GoldenPrepared(t *testing.T, raw []byte) *module.Module {
	t.Helper()
	value := GoldenLesson(t, raw)
	file, problems := module.NewFile("l001.json", value, diagnostic.Context{Path: "prepared.json"})
	GoldenNoProblems(t, problems)
	order := "000900719925474099312345678901234567890"
	entries := []manifest.Entry{GoldenEntryFor(t, file.FileName(), value.Document, order)}
	sections := []manifest.Section{}
	candidate := &module.Module{
		Manifest: manifest.Manifest{FormatVersion: "1.0", CreatedAt: "2026-11-04T09:00:00Z",
			Course: manifest.Course{Title: "Курс <&>", Lessons: &entries, Sections: &sections}},
		Files: []*module.File{file},
	}
	GoldenNoProblems(t, module.Validate(candidate, diagnostic.Context{}))
	return candidate
}

func GoldenEntryFor(t *testing.T, name string, document lesson.Document, order string) manifest.Entry {
	t.Helper()
	return manifest.Entry{
		ID: document.ID, FileName: name, Order: &order, Title: document.Title,
		Subtitle: document.Subtitle, LineCount: document.LineCount,
		CreatedAt: document.CreatedAt, Version: document.Version,
	}
}

func GoldenJSON(t *testing.T, value manifest.Manifest) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func GoldenZIP(t *testing.T, entries []GoldenEntry) string {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		stream, err := writer.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Write(entry.raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "исходный модуль.zip")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func GoldenWritten(t *testing.T, value *module.Module) string {
	t.Helper()
	var buffer bytes.Buffer
	GoldenNoProblems(t, moduleio.Write(&buffer, value, diagnostic.Context{Path: "кандидат.zip"}))
	path := filepath.Join(t.TempDir(), "записанный модуль.zip")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	GoldenNoProblems(t, moduleio.Verify(path))
	return path
}

func GoldenLoaded(t *testing.T, path string) *module.Module {
	t.Helper()
	value, problems := moduleio.Read(path)
	GoldenNoProblems(t, problems)
	if value == nil {
		t.Fatal("Read returned no module without diagnostics")
	}
	GoldenNoProblems(t, module.Validate(value, diagnostic.Context{}))
	return value
}

func GoldenOutputEntries(t *testing.T, path string) []GoldenEntry {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	entries := make([]GoldenEntry, 0, len(reader.File))
	for _, entry := range reader.File {
		if entry.Method != zip.Deflate || !entry.Mode().IsRegular() {
			t.Fatalf("output entry %q is not a regular Deflate entry", entry.Name)
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read output %q: read=%v close=%v", entry.Name, readErr, closeErr)
		}
		entries = append(entries, GoldenEntry{entry.Name, raw})
	}
	return entries
}

func GoldenAssertOutput(t *testing.T, path string, value *module.Module, lessonRaw [][]byte) {
	t.Helper()
	entries := GoldenOutputEntries(t, path)
	if len(entries) != len(value.Files)+1 || entries[0].name != module.ManifestFileName {
		t.Fatal("output must contain exactly manifest then Files")
	}
	actual := GoldenManifest(t, entries[0].raw)
	if !bytes.Equal(GoldenJSON(t, actual), GoldenJSON(t, value.Manifest)) {
		t.Fatal("manifest hierarchy, metadata, lexemes or array order changed")
	}
	if !bytes.Equal(entries[0].raw, GoldenJSON(t, value.Manifest)) {
		t.Fatal("manifest must use standard MarshalIndent with two spaces")
	}
	for i, file := range value.Files {
		if entries[i+1].name != file.FileName() || !bytes.Equal(entries[i+1].raw, lessonRaw[i]) {
			t.Fatalf("uncompressed source bytes/order changed for %q", file.FileName())
		}
	}
}
