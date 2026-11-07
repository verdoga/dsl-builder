package moduleio_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

type DocStream struct {
	bytes.Buffer
	writes, closes, syncs int
	failure               error
}

func (s *DocStream) Write(raw []byte) (int, error) {
	s.writes++
	if s.failure != nil {
		return 0, s.failure
	}
	return s.Buffer.Write(raw)
}

func (s *DocStream) Close() error { s.closes++; return nil }
func (s *DocStream) Sync() error  { s.syncs++; return nil }

func DocModel(t *testing.T) (*module.Module, []byte) {
	t.Helper()
	raw, err := os.ReadFile("testdata/doc/lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	if problems := lesson.ValidateJSON(raw, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("fixture schema: %v", problems)
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	file, problems := module.LoadedFile("l001.json", &value, raw, diagnostic.Context{
		Path: "источник.json", Line: 23, Column: 7,
	})
	if len(problems) != 0 {
		t.Fatalf("fixture file: %v", problems)
	}
	order := "01"
	doc := value.Document
	entries := []manifest.Entry{{ID: doc.ID, FileName: file.FileName(), Order: &order,
		Title: doc.Title, Subtitle: doc.Subtitle, LineCount: doc.LineCount,
		CreatedAt: doc.CreatedAt, Version: doc.Version}}
	sections := []manifest.Section{}
	result := &module.Module{Files: []*module.File{file}, Manifest: manifest.Manifest{
		FormatVersion: manifest.FormatVersion, CreatedAt: "2026-11-04T09:00:00Z",
		Course: manifest.Course{Title: "Курс", Lessons: &entries, Sections: &sections},
	}}
	if problems := module.Validate(result, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("fixture module: %v", problems)
	}
	return result, raw
}

func DocArchive(t *testing.T, path string, invalid bool) []byte {
	t.Helper()
	model, raw := DocModel(t)
	manifestRaw, err := json.Marshal(model.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	names := []string{module.ManifestFileName, "l001.json"}
	contents := [][]byte{manifestRaw, raw}
	if invalid {
		names = append(names, "l001.json", "L001.json", "escape/side.json")
		contents = append(contents, raw, raw, []byte("не извлекать"))
	}
	for i, name := range names {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(contents[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, result.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func DocCorruptArchive(t *testing.T, path string) []byte {
	t.Helper()
	raw := DocArchive(t, path, false)
	_, lessonRaw := DocModel(t)
	position := bytes.Index(raw, lessonRaw)
	if position < 0 || lessonRaw[len(lessonRaw)-1] != '\n' {
		t.Fatal("stored fixture location missing")
	}
	// Меняем допустимый JSON whitespace, оставляя старый CRC в ZIP.
	raw[position+len(lessonRaw)-1] = '\t'
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func DocJoin(problems []*diagnostic.Error) error {
	var values []error
	for _, problem := range problems {
		if problem != nil {
			values = append(values, problem)
		}
	}
	return errors.Join(values...)
}

func DocJoined(t *testing.T, joined error, problems []*diagnostic.Error) {
	t.Helper()
	if len(problems) == 0 || joined == nil {
		t.Fatal("expected structured errors")
	}
	var first *diagnostic.Error
	if !errors.As(joined, &first) || first != problems[0] {
		t.Fatal("errors.As lost the first diagnostic")
	}
	for _, problem := range problems {
		if problem == nil || !errors.Is(joined, problem) {
			t.Fatal("errors.Join lost a diagnostic")
		}
		if problem.Cause != nil && !errors.Is(joined, problem.Cause) {
			t.Fatal("errors.Join lost a technical cause")
		}
		if problem.Code == "" || problem.Reason == "" {
			t.Fatalf("incomplete diagnostic: %#v", problem)
		}
	}
}

func DocClosed(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "linux" {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			target, _ := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
			if target == path {
				t.Fatalf("archive descriptor still open: %s", entry.Name())
			}
		}
	}
	// На Windows переименование и удаление также требуют закрытого reader.
	moved := path + ".moved"
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(moved); err != nil {
		t.Fatal(err)
	}
}
