package moduleio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

type ReadZIPEntry struct {
	name   string
	raw    []byte
	method uint16
	mode   os.FileMode
}

func ReadFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/read/lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	if problems := lesson.ValidateJSON(raw, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("fixture: %v", problems)
	}
	return raw
}

func ReadManifest(t *testing.T, raw []byte, names ...string) []byte {
	t.Helper()
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	entries := make([]manifest.Entry, len(names))
	for i, name := range names {
		order := string(rune('1' + i))
		d := value.Document
		entries[i] = manifest.Entry{ID: d.ID, FileName: name, Order: &order, Title: d.Title, Subtitle: d.Subtitle, LineCount: d.LineCount, CreatedAt: d.CreatedAt, Version: d.Version}
	}
	sections := []manifest.Section{}
	valueManifest := manifest.Manifest{FormatVersion: "1.0", CreatedAt: "2026-11-04T09:00:00Z", Course: manifest.Course{Title: "Курс", Lessons: &entries, Sections: &sections}}
	result, err := json.Marshal(valueManifest)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func ReadZIP(t *testing.T, entries ...ReadZIPEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: entry.method}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		stream, err := writer.CreateHeader(header)
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
	return buf.Bytes()
}

func ReadReader(t *testing.T, raw []byte) *zip.Reader {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func ReadPath(t *testing.T, raw []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Модуль Ω.zip")
	if err := os.WriteFile(path, raw, 0444); err != nil {
		t.Fatal(err)
	}
	return path
}

func ReadRequireCode(t *testing.T, problems []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, problem := range problems {
		if problem == nil {
			t.Fatal("nil diagnostic")
		}
		if problem.Code == code {
			return problem
		}
	}
	t.Fatalf("missing %s: %v", code, problems)
	return nil
}

func TestReadPreservesExactBytesModelsAndZIPOrder(t *testing.T) {
	raw := ReadFixture(t)
	second := bytes.ReplaceAll(raw, []byte("MiXeD+1"), []byte("Second"))
	second = bytes.ReplaceAll(second, []byte("mixed+1"), []byte("second"))
	manifestRaw := ReadManifest(t, raw, "l002.json")
	var m manifest.Manifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil {
		t.Fatal(err)
	}
	var other manifest.Manifest
	if err := json.Unmarshal(ReadManifest(t, second, "l1000.json"), &other); err != nil {
		t.Fatal(err)
	}
	added := (*other.Course.Lessons)[0]
	order := "00000000000000000000000000000000000000002"
	added.Order = &order
	*m.Course.Lessons = append(*m.Course.Lessons, added)
	manifestRaw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	// Equal integer metadata need not have identical number lexemes.
	manifestRaw = bytes.ReplaceAll(manifestRaw, []byte("9007199254740993.0"), []byte("9007199254740993"))
	archive := ReadZIP(t, ReadZIPEntry{name: "l1000.json", raw: second, method: zip.Deflate}, ReadZIPEntry{name: module.ManifestFileName, raw: manifestRaw}, ReadZIPEntry{name: "l002.json", raw: raw})
	path := ReadPath(t, archive)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	value, problems := Read(path)
	if len(problems) != 0 || value == nil {
		t.Fatalf("Read: %v", problems)
	}
	if len(value.Files) != 2 || value.Files[0].FileName() != "l1000.json" || value.Files[1].FileName() != "l002.json" {
		t.Fatalf("ZIP order: %+v", value.Files)
	}
	for i := range *m.Course.Lessons {
		(*m.Course.Lessons)[i].Version = "9007199254740993"
	}
	if !reflect.DeepEqual(value.Manifest, m) {
		t.Fatal("manifest changed")
	}
	for i, wantRaw := range [][]byte{second, raw} {
		file := value.Files[i]
		if !bytes.Equal(file.OriginalBytes(), wantRaw) {
			t.Fatal("raw was reserialized")
		}
		var want lesson.Lesson
		if err := json.Unmarshal(wantRaw, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(file.Lesson(), &want) {
			t.Fatal("model differs from same raw")
		}
		if file.Context() != (diagnostic.Context{Path: path, FileName: file.FileName(), LessonID: want.Document.ID}) {
			t.Fatalf("source: %+v", file.Context())
		}
		mutated := file.Lesson()
		mutated.Document.Pages[0].Nodes[2].Data[0].Value.([]lesson.Data)[0].Value = "changed"
		original := file.OriginalBytes()
		original[0] ^= 1
		if !reflect.DeepEqual(file.Lesson(), &want) || !bytes.Equal(file.OriginalBytes(), wantRaw) {
			t.Fatal("snapshot aliases returned state")
		}
	}
	disk, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(disk, archive) {
		t.Fatal("source archive changed", err)
	}
	after, err := os.Stat(path)
	if err != nil || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		t.Fatal("source metadata changed", err)
	}
}

func TestReadOpenAndArchiveErrorsHaveNoModel(t *testing.T) {
	for _, test := range []struct {
		name, path, code string
		cause            error
	}{
		{name: "missing", path: filepath.Join(t.TempDir(), "missing.zip"), code: CodeOpen, cause: os.ErrNotExist},
		{name: "structure", path: ReadPath(t, []byte("not a zip")), code: CodeArchive, cause: zip.ErrFormat},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, problems := Read(test.path)
			if value != nil {
				t.Fatal("partial model")
			}
			problem := ReadRequireCode(t, problems, test.code)
			if problem.Context.Path != test.path || !errors.Is(problem, test.cause) {
				t.Fatalf("lost source/cause: %+v", problem)
			}
		})
	}
}
