package module

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func uniqueMetaDiagnostic(t *testing.T, problem *diagnostic.Error, file *File, item catalog.Item, field string) {
	t.Helper()
	if problem == nil {
		t.Fatal("nil metadata diagnostic")
	}
	if problem.Code != CodeMetadata {
		t.Errorf("code = %q, want %q", problem.Code, CodeMetadata)
	}
	if strings.TrimSpace(problem.Reason) == "" || !strings.ContainsFunc(problem.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("reason must explain mismatch in Russian: %q", problem.Reason)
	}
	entryContext, fileContext := item.Context, file.Context()
	if problem.Context != entryContext && problem.Context != fileContext {
		t.Errorf("context = %+v; want one exact known participant: %+v or %+v", problem.Context, entryContext, fileContext)
	}
	hasEntry, hasFile := problem.Context == entryContext, problem.Context == fileContext
	for _, related := range problem.Related {
		if related != entryContext && related != fileContext {
			t.Errorf("invented or incomplete related context: %+v", related)
		}
		hasEntry = hasEntry || related == entryContext
		hasFile = hasFile || related == fileContext
	}
	if !hasEntry || !hasFile {
		t.Errorf("both entry and lesson contexts must be available: context=%+v related=%+v", problem.Context, problem.Related)
	}
	fieldIndex := -1
	for index, detail := range problem.Details {
		if detail.Label == "" {
			t.Error("detail has no descriptive label")
		}
		if detail.Value == field || detail.Value == "/"+field || detail.Value == "/document/"+field {
			fieldIndex = index
		}
	}
	if fieldIndex < 0 {
		t.Errorf("details omit field %q: %+v", field, problem.Details)
	}
	expected := catalog.EntryFromLesson(file.Lesson().Document, item.Entry.FileName, item.Entry.Order)
	actualValues := uniqueMetaFieldValues(t, item.Entry, field)
	expectedValues := uniqueMetaFieldValues(t, expected, field)
	actualIndex := uniqueMetaFindDetail(t, problem.Details, actualValues, fieldIndex, -1)
	expectedIndex := uniqueMetaFindDetail(t, problem.Details, expectedValues, fieldIndex, actualIndex)
	if actualIndex < 0 || expectedIndex < 0 {
		t.Errorf("details must preserve distinct entry and lesson values for %s: got %+v, entry %q, lesson %q", field, problem.Details, actualValues, expectedValues)
	}
	output := string(uniqueMetaJSON(t, problem))
	for _, secret := range []string{"PRIVATE_NODE_BODY_МЕТАДАННЫЕ", "cHJpdmF0ZS1tZWRpYS1ieXRlcw==", "аудио.bin", "0x"} {
		if strings.Contains(output, secret) {
			t.Errorf("metadata diagnostic leaks node data or pointer: %q", secret)
		}
	}
}

func uniqueMetaFieldValues(t *testing.T, entry manifest.Entry, field string) []string {
	t.Helper()
	var text string
	switch field {
	case "id":
		text = entry.ID
	case "title":
		text = entry.Title
	case "subtitle":
		if entry.Subtitle == nil {
			return []string{"null"}
		}
		text = *entry.Subtitle
	case "lineCount":
		return []string{string(entry.LineCount)}
	case "createdAt":
		text = entry.CreatedAt
	case "version":
		return []string{string(entry.Version)}
	default:
		t.Fatalf("unknown test field %q", field)
	}
	// Контракт требует точное значение; строка может быть показана буквально
	// либо как JSON-строка, сохраняющая пустоту, пробелы и переносы.
	return []string{text, string(uniqueMetaJSON(t, text))}
}

func uniqueMetaFindDetail(t *testing.T, details []diagnostic.Detail, values []string, exclude, other int) int {
	t.Helper()
	for index, detail := range details {
		if index == exclude || index == other {
			continue
		}
		for _, value := range values {
			if detail.Value == value {
				return index
			}
		}
	}
	return -1
}

func TestUniqueMetaSuccessfulComparisonPreservesEverything(t *testing.T) {
	for _, weight := range []*string{nil, new(string)} {
		if weight != nil {
			*weight = "0001844674407370955161700000000000000000000000001"
		}
		t.Run(string(uniqueMetaJSON(t, weight)), func(t *testing.T) {
			file := uniqueMetaFile(t, uniqueMetaLesson(t))
			item := uniqueMetaItem(t, file)
			item.Entry = catalog.EntryFromLesson(file.Lesson().Document, file.FileName(), weight)
			section := "Секция Ё"
			item.Section = &section
			uniqueMetaCheckEntry(t, item.Entry)
			beforeItem, beforeLesson, beforeRaw := uniqueMetaJSON(t, item), uniqueMetaJSON(t, file.Lesson()), file.OriginalBytes()
			beforeContext := file.Context()
			if problems := validateEntryMetadata(file, item); len(problems) != 0 {
				t.Fatalf("catalog.EntryFromLesson metadata rejected: %+v", problems)
			}
			if !bytes.Equal(beforeItem, uniqueMetaJSON(t, item)) || !bytes.Equal(beforeLesson, uniqueMetaJSON(t, file.Lesson())) || !bytes.Equal(beforeRaw, file.OriginalBytes()) || beforeContext != file.Context() {
				t.Fatal("metadata validation changed an input or snapshot")
			}
		})
	}
}

func TestUniqueMetaLiteralTitleAndStableDiagnostics(t *testing.T) {
	for _, title := range []string{" Название ё ", "Название Ё", " Название Е\u0308 "} {
		t.Run(title, func(t *testing.T) {
			file := uniqueMetaFile(t, uniqueMetaLesson(t))
			item := uniqueMetaItem(t, file)
			item.Entry.Title = title
			uniqueMetaCheckEntry(t, item.Entry)
			first := validateEntryMetadata(file, item)
			if len(first) != 1 {
				t.Fatalf("literal title mismatch errors = %+v", first)
			}
			uniqueMetaDiagnostic(t, first[0], file, item, "title")
			second := validateEntryMetadata(file, item)
			if !bytes.Equal(uniqueMetaJSON(t, first), uniqueMetaJSON(t, second)) {
				t.Fatal("repeated comparison changed diagnostic order or values")
			}
		})
	}
}

func TestUniqueMetaMismatchKeepsNumberSpellingInDetails(t *testing.T) {
	file := uniqueMetaFile(t, uniqueMetaLesson(t))
	item := uniqueMetaItem(t, file)
	item.Entry.LineCount = json.Number("9007199254740992.0000")
	item.Entry.Version = json.Number("184467440737095516160e-1")
	uniqueMetaCheckEntry(t, item.Entry)
	problems := validateEntryMetadata(file, item)
	if len(problems) != 2 {
		t.Fatalf("numeric mismatch errors = %+v, want two", problems)
	}
	uniqueMetaDiagnostic(t, problems[0], file, item, "lineCount")
	uniqueMetaDiagnostic(t, problems[1], file, item, "version")
}
