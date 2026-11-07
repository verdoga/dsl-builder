package module

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
)

func uniqueMetaLesson(t *testing.T) *lesson.Lesson {
	t.Helper()
	body := lesson.RoleBody
	subtitle := " Подзаголовок\nс точными пробелами "
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "MeTa+Я", Title: " Название Ё ", Subtitle: &subtitle,
		LineCount: "9007199254740993", Version: "18446744073709551617",
		CreatedAt: "2026-11-04T12:34:56Z",
		Pages: []lesson.Page{{ID: "meta+я-p001", Nodes: []lesson.Node{
			{ID: "meta+я-n001", Type: lesson.NodeText, Nodes: []lesson.Node{}, Data: []lesson.Data{
				{Type: lesson.DataText, Role: &body, Value: "PRIVATE_NODE_BODY_МЕТАДАННЫЕ", Answers: []string{}},
			}},
			{ID: "meta+я-n002", Type: lesson.NodeMedia, Nodes: []lesson.Node{}, Data: []lesson.Data{
				{Type: lesson.DataMedia, Role: &body, Value: lesson.EmbeddedFile{
					FileName: "аудио.bin", MIMEType: "application/octet-stream", Content: "cHJpdmF0ZS1tZWRpYS1ieXRlcw==",
				}, Answers: []string{}},
			}},
		}}},
	}}
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("invalid lesson fixture: %+v", problems)
	}
	return value
}

func uniqueMetaFile(t *testing.T, value *lesson.Lesson) *File {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	context := diagnostic.Context{Path: `C:\Курс с пробелом\исходный.zip`, FileName: "l007.json", LessonID: value.Document.ID}
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		t.Fatalf("invalid raw fixture: %+v", problems)
	}
	var decoded lesson.Lesson
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	file, problems := LoadedFile("l007.json", &decoded, raw, context)
	if len(problems) != 0 || file == nil {
		t.Fatalf("LoadedFile = %v, %+v", file, problems)
	}
	return file
}

func uniqueMetaItem(t *testing.T, file *File) catalog.Item {
	t.Helper()
	weight := "00018446744073709551617"
	entry := catalog.EntryFromLesson(file.Lesson().Document, file.FileName(), &weight)
	return catalog.Item{Entry: entry, RelativePath: "вход/урок.json", Context: diagnostic.Context{
		Path: `C:\Курс с пробелом\кандидат.zip`, FileName: entry.FileName, LessonID: entry.ID,
	}}
}

func uniqueMetaCheckEntry(t *testing.T, entry manifest.Entry) {
	t.Helper()
	entries, sections := []manifest.Entry{entry}, []manifest.Section{}
	value := manifest.Manifest{FormatVersion: manifest.FormatVersion, CreatedAt: "2026-11-07T00:00:00Z",
		Course: manifest.Course{Title: "Курс", Lessons: &entries, Sections: &sections}}
	if problems := manifest.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("invalid entry fixture: %+v", problems)
	}
}

func uniqueMetaJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestUniqueMetaEachFieldMismatch(t *testing.T) {
	cases := []struct {
		field string
		edit  func(*manifest.Entry)
	}{
		{"id", func(e *manifest.Entry) { e.ID = "meta+я" }},
		{"title", func(e *manifest.Entry) { e.Title = "Название Ё" }},
		{"subtitle", func(e *manifest.Entry) { e.Subtitle = nil }},
		{"lineCount", func(e *manifest.Entry) { e.LineCount = "9007199254740992" }},
		{"createdAt", func(e *manifest.Entry) { e.CreatedAt = "2026-11-04T12:34:57Z" }},
		{"version", func(e *manifest.Entry) { e.Version = "18446744073709551616" }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			file := uniqueMetaFile(t, uniqueMetaLesson(t))
			item := uniqueMetaItem(t, file)
			tc.edit(&item.Entry)
			item.Context.LessonID = item.Entry.ID
			uniqueMetaCheckEntry(t, item.Entry)
			beforeItem, beforeLesson, beforeRaw := uniqueMetaJSON(t, item), uniqueMetaJSON(t, file.Lesson()), file.OriginalBytes()
			problems := validateEntryMetadata(file, item)
			if len(problems) != 1 {
				t.Fatalf("metadata errors = %+v, want exactly one for %s", problems, tc.field)
			}
			uniqueMetaDiagnostic(t, problems[0], file, item, tc.field)
			if !bytes.Equal(beforeItem, uniqueMetaJSON(t, item)) || !bytes.Equal(beforeLesson, uniqueMetaJSON(t, file.Lesson())) || !bytes.Equal(beforeRaw, file.OriginalBytes()) {
				t.Fatal("metadata comparison changed the entry, lesson or original bytes")
			}
		})
	}
}

func TestUniqueMetaAllMismatchesInEntryOrder(t *testing.T) {
	file := uniqueMetaFile(t, uniqueMetaLesson(t))
	item := uniqueMetaItem(t, file)
	item.Entry.ID, item.Entry.Title, item.Entry.Subtitle = "meta+я", "Другое название", nil
	item.Entry.LineCount, item.Entry.CreatedAt, item.Entry.Version = "9007199254740992", "2026-11-05T12:34:56Z", "18446744073709551616"
	item.Context.LessonID = item.Entry.ID
	uniqueMetaCheckEntry(t, item.Entry)
	before := uniqueMetaJSON(t, item)
	problems := validateEntryMetadata(file, item)
	fields := []string{"id", "title", "subtitle", "lineCount", "createdAt", "version"}
	if len(problems) != len(fields) {
		t.Fatalf("metadata errors = %+v, want six independent errors", problems)
	}
	for index, field := range fields {
		uniqueMetaDiagnostic(t, problems[index], file, item, field)
	}
	if !bytes.Equal(before, uniqueMetaJSON(t, item)) {
		t.Fatal("mismatched entry was repaired")
	}
}

func TestUniqueMetaSubtitlePresenceAndValue(t *testing.T) {
	empty, text, sameText := "", "Подзаголовок", "Подзаголовок"
	cases := []struct {
		name             string
		actual, expected *string
		mismatch         bool
	}{
		{"both null", nil, nil, false},
		{"null versus empty", nil, &empty, true},
		{"empty versus null", &empty, nil, true},
		{"both empty different pointers", &empty, new(string), false},
		{"equal different pointers", &text, &sameText, false},
		{"different values", &text, &empty, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := uniqueMetaLesson(t)
			value.Document.Subtitle = tc.expected
			file := uniqueMetaFile(t, value)
			item := uniqueMetaItem(t, file)
			item.Entry.Subtitle = tc.actual
			uniqueMetaCheckEntry(t, item.Entry)
			problems := validateEntryMetadata(file, item)
			if !tc.mismatch {
				if len(problems) != 0 {
					t.Fatalf("equal subtitles rejected: %+v", problems)
				}
				return
			}
			if len(problems) != 1 {
				t.Fatalf("subtitle mismatch errors = %+v", problems)
			}
			uniqueMetaDiagnostic(t, problems[0], file, item, "subtitle")
		})
	}
}
