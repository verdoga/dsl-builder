package moduleio

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func TestJSONWriteLoadedLessonKeepsExactBytes(t *testing.T) {
	raw := JSONWriteFixture(t, "loaded.json")
	want := bytes.Clone(raw)
	value := JSONWriteLesson(t, raw)
	context := diagnostic.Context{Path: "Архив.zip", Line: 17, Column: 3}
	file, problems := module.LoadedFile("l020.json", value, raw, context)
	if len(problems) != 0 {
		t.Fatalf("LoadedFile: %v", problems)
	}
	JSONWriteValidate(t, []*module.File{file})
	snapshot := JSONWriteMarshal(t, file.Lesson())
	source := file.Context()
	// Снимок уже принадлежит File: внешние модели и буферы можно менять.
	raw[0] = '!'
	value.Document.Title = "изменён снаружи"
	copyOfLesson := file.Lesson()
	copyOfLesson.Document.Pages[0].Nodes[0].Data[0].Value = "чужая правка"
	copyOfOriginal := file.OriginalBytes()
	copyOfOriginal[0] = '?'
	for attempt := 0; attempt < 2; attempt++ {
		got, problem := lessonBytes(file)
		if problem != nil || !bytes.Equal(got, want) {
			t.Fatalf("lessonBytes changed loaded JSON: error=%v", problem)
		}
		got[0] = '#'
	}
	if !bytes.Equal(file.OriginalBytes(), want) || !bytes.Equal(JSONWriteMarshal(t, file.Lesson()), snapshot) || file.Context() != source {
		t.Fatal("lessonBytes changed the immutable File")
	}
}

func TestJSONWriteNewAndReplacedLessonsUsePreparedModel(t *testing.T) {
	raw := JSONWriteFixture(t, "loaded.json")
	previousRaw := bytes.ReplaceAll(raw, []byte("  Урок <&>  "), []byte("Старый урок"))
	previous, problems := module.LoadedFile("l020.json", JSONWriteLesson(t, previousRaw), previousRaw, diagnostic.Context{Path: "старый.zip"})
	if len(problems) != 0 {
		t.Fatalf("LoadedFile: %v", problems)
	}
	for _, kind := range []string{"new", "replace"} {
		t.Run(kind, func(t *testing.T) {
			prepared := JSONWriteLesson(t, raw)
			context := diagnostic.Context{Path: "новый.json", Line: 8}
			var file *module.File
			if kind == "new" {
				file, problems = module.NewFile("l020.json", prepared, context)
			} else {
				file, problems = module.ReplaceFile(previous, prepared, context)
			}
			if len(problems) != 0 {
				t.Fatalf("%s: %v", kind, problems)
			}
			if file.OriginalBytes() != nil {
				t.Fatal("prepared File has original bytes")
			}
			JSONWriteValidate(t, []*module.File{file})
			before := JSONWriteMarshal(t, file.Lesson())
			source := file.Context()
			prepared.Document.Pages[0].Nodes[1].Data[0].Value = "правка после снимка"
			got, problem := lessonBytes(file)
			if problem != nil {
				t.Fatalf("lessonBytes: %v", problem)
			}
			JSONWriteExact(t, got, "rich.expected.json")
			roundtrip := JSONWriteLesson(t, got)
			JSONWriteTypedData(t, roundtrip)
			if !bytes.Equal(JSONWriteMarshal(t, file.Lesson()), before) || !bytes.Equal(previous.OriginalBytes(), previousRaw) || file.Context() != source {
				t.Fatal("serialization changed a snapshot")
			}
		})
	}
}

func TestJSONWriteLessonKeepsEmptyPagesAndZeroLineCount(t *testing.T) {
	want := JSONWriteFixture(t, "empty.expected.json")
	file, problems := module.NewFile("l001.json", JSONWriteLesson(t, want), diagnostic.Context{})
	if len(problems) != 0 {
		t.Fatalf("NewFile: %v", problems)
	}
	JSONWriteValidate(t, []*module.File{file})
	got, problem := lessonBytes(file)
	if problem != nil {
		t.Fatalf("lessonBytes: %v", problem)
	}
	JSONWriteExact(t, got, "empty.expected.json")
}

func TestJSONWriteManifestPreservesPreparedValuesAndForms(t *testing.T) {
	var files []*module.File
	for _, item := range []struct{ id, name string }{{"7", "l020.json"}, {"8", "l003.json"}, {"9", "l1000.json"}} {
		raw := JSONWriteFixture(t, "rich.expected.json")
		raw = bytes.ReplaceAll(raw, []byte("JW7"), []byte("JW"+item.id))
		raw = bytes.ReplaceAll(raw, []byte("jw7-"), []byte("jw"+item.id+"-"))
		file, problems := module.NewFile(item.name, JSONWriteLesson(t, raw), diagnostic.Context{Path: "источник" + item.id})
		if len(problems) != 0 {
			t.Fatalf("NewFile: %v", problems)
		}
		files = append(files, file)
	}
	for _, form := range []string{"flat", "unit", "sections", "unit_sections"} {
		t.Run(form, func(t *testing.T) {
			fixture := "manifest_" + form + ".expected.json"
			raw := JSONWriteFixture(t, fixture)
			context := diagnostic.Context{Path: "кандидат.zip", FileName: "чужое имя", Line: 12, Column: 2}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			var tree any
			if err := decoder.Decode(&tree); err != nil {
				t.Fatal(err)
			}
			if problems := manifest.ValidateShape(tree, context); len(problems) != 0 {
				t.Fatalf("ValidateShape: %v", problems)
			}
			var prepared manifest.Manifest
			if err := json.Unmarshal(raw, &prepared); err != nil {
				t.Fatal(err)
			}
			if problems := module.Validate(&module.Module{Manifest: prepared, Files: files}, context); len(problems) != 0 {
				t.Fatalf("module.Validate: %v", problems)
			}
			before := JSONWriteMarshal(t, prepared)
			for attempt := 0; attempt < 2; attempt++ {
				got, problem := manifestBytes(prepared, context)
				if problem != nil {
					t.Fatalf("manifestBytes: %v", problem)
				}
				JSONWriteExact(t, got, fixture)
				got[0] = '!'
			}
			if !bytes.Equal(JSONWriteMarshal(t, prepared), before) {
				t.Fatal("manifestBytes changed prepared dates, numbers, names or arrays")
			}
		})
	}
}

func JSONWriteFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/json_write/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func JSONWriteLesson(t *testing.T, raw []byte) *lesson.Lesson {
	t.Helper()
	if !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("fixture must be UTF-8 without BOM")
	}
	if problems := lesson.ValidateJSON(raw, diagnostic.Context{Path: "нормативная фикстура"}); len(problems) != 0 {
		t.Fatalf("lesson.ValidateJSON: %v", problems)
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return &value
}

func JSONWriteValidate(t *testing.T, files []*module.File) {
	t.Helper()
	entries := make([]manifest.Entry, len(files))
	for i, file := range files {
		doc := file.Lesson().Document
		order := "000184467440737095516161234567890"
		entries[i] = manifest.Entry{ID: doc.ID, FileName: file.FileName(), Order: &order, Title: doc.Title,
			Subtitle: doc.Subtitle, LineCount: doc.LineCount, CreatedAt: doc.CreatedAt, Version: doc.Version}
	}
	sections := []manifest.Section{}
	prepared := manifest.Manifest{FormatVersion: "1.0", CreatedAt: "1999-12-31T23:59:59Z",
		Course: manifest.Course{Title: "Курс", Lessons: &entries, Sections: &sections}}
	if problems := module.Validate(&module.Module{Manifest: prepared, Files: files}, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("module.Validate precondition: %v", problems)
	}
}

func JSONWriteMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func JSONWriteExact(t *testing.T, got []byte, fixture string) {
	t.Helper()
	if !bytes.Equal(got, JSONWriteFixture(t, fixture)) {
		t.Fatalf("JSON differs from normative two-space snapshot %s", fixture)
	}
	if !utf8.Valid(got) || bytes.HasPrefix(got, []byte{0xef, 0xbb, 0xbf}) || bytes.Contains(got, []byte("\r")) {
		t.Fatal("serialized JSON must be UTF-8 without BOM and use standard indentation")
	}
}

func JSONWriteTypedData(t *testing.T, value *lesson.Lesson) {
	t.Helper()
	doc := value.Document
	if doc.Version.String() != "900719925474099312345678901234567890.0" || doc.LineCount.String() != "7e1" {
		t.Fatal("number precision or lexemes changed")
	}
	nodes := doc.Pages[0].Nodes
	for i, want := range [][]byte{{0, 255, 1, 128, 10, 13}, []byte("\ufeffТекст\r\n\n"), []byte("WEBVTT\r\n\r\n00:00.000 --> 00:01.000\r\nHi\r\n")} {
		embedded, ok := nodes[0].Data[i].Value.(lesson.EmbeddedFile)
		if !ok {
			t.Fatal("media Value lost its owner type")
		}
		decoded, err := base64.StdEncoding.DecodeString(embedded.Content)
		if err != nil || !bytes.Equal(decoded, want) {
			t.Fatal("embedded resource bytes changed")
		}
	}
	collection, ok := nodes[2].Data[0].Value.([]lesson.Data)
	if !ok || len(collection) != 2 || collection[0].Value != "второй" {
		t.Fatal("collection type or order changed")
	}
	if text, ok := nodes[1].Data[0].Value.(string); !ok || !strings.Contains(text, "\n\n<b>Я</b>\\t") {
		t.Fatal("multifill text changed")
	}
}
