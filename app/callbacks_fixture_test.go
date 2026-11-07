package app

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

func callbacksFixtureModule(t *testing.T, loaded bool, title string) *module.Module {
	t.Helper()
	raw, err := os.ReadFile("testdata/callbacks/lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	context := diagnostic.Context{Path: filepath.Join(t.TempDir(), "исходный урок.json"), FileName: "l001.json"}
	if issues := lesson.ValidateJSON(raw, context); len(issues) != 0 {
		t.Fatalf("lesson fixture shape: %v", issues)
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if title != "" {
		value.Document.Title = title
	}
	file, issues := module.NewFile("l001.json", &value, context)
	if len(issues) != 0 {
		t.Fatalf("NewFile: %v", issues)
	}
	document := file.Lesson().Document
	weight := "0010"
	entries := []manifest.Entry{{
		ID: document.ID, FileName: file.FileName(), Order: &weight,
		Title: document.Title, Subtitle: document.Subtitle, LineCount: document.LineCount,
		CreatedAt: document.CreatedAt, Version: document.Version,
	}}
	sections := []manifest.Section{}
	result := &module.Module{Manifest: manifest.Manifest{
		FormatVersion: manifest.FormatVersion, CreatedAt: "2020-01-02T03:04:05Z",
		Course: manifest.Course{Title: "Курс callbacks", Lessons: &entries, Sections: &sections},
	}, Files: []*module.File{file}}
	if issues := module.Validate(result, context); len(issues) != 0 {
		t.Fatalf("module fixture: %v", issues)
	}
	if loaded {
		if title != "" {
			t.Fatal("loaded fixture must keep exact source lesson")
		}
		manifestRaw, err := json.Marshal(result.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		archive := callbacksFixtureZIP(t, manifestRaw, raw)
		path := filepath.Join(t.TempDir(), "загруженный.zip")
		if err := os.WriteFile(path, archive, 0600); err != nil {
			t.Fatal(err)
		}
		result, issues = moduleio.Read(path)
		if len(issues) != 0 {
			t.Fatalf("Read fixture: %v", issues)
		}
		if !bytes.Equal(result.Files[0].OriginalBytes(), raw) {
			t.Fatal("Read fixture lost exact lesson bytes")
		}
	}
	return result
}

func callbacksFixtureZIP(t *testing.T, manifestRaw, lessonRaw []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range []struct {
		name string
		raw  []byte
	}{{module.ManifestFileName, manifestRaw}, {"l001.json", lessonRaw}} {
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
	return buffer.Bytes()
}

func callbacksFixtureJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func callbacksFixtureChildren(t *testing.T, err error) []*diagnostic.Error {
	t.Helper()
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("expected structured errors.Join children, got %T", err)
	}
	children := joined.Unwrap()
	result := make([]*diagnostic.Error, len(children))
	for index, child := range children {
		problem, ok := child.(*diagnostic.Error)
		if !ok || problem == nil {
			t.Fatalf("child %d: expected nonnil diagnostic, got %T", index, child)
		}
		result[index] = problem
	}
	return result
}

func TestCallbacksFixtureOwners(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		value := callbacksFixtureModule(t, loaded, "")
		var output bytes.Buffer
		if issues := moduleio.Write(&output, value, diagnostic.Context{}); len(issues) != 0 {
			t.Fatalf("Write: %v", issues)
		}
		reader, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range reader.File {
			stream, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			_, readErr := io.Copy(io.Discard, stream)
			closeErr := stream.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("entry read/close: %v / %v", readErr, closeErr)
			}
		}
	}
}
