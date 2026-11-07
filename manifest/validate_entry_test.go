package manifest

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/order"
)

func entryValidationContractString(value string) *string { return &value }

func entryValidationContractValid() Entry {
	return Entry{ID: "AbC:lesson+1", FileName: "l001.json", Order: entryValidationContractString("001"),
		Title: " Название\tурока ", Subtitle: nil, LineCount: json.Number("0"),
		CreatedAt: "2024-02-29T12:34:56Z", Version: json.Number("1")}
}

func entryValidationContractRun(t *testing.T, value Entry, allowNull bool, pointer string, context diagnostic.Context) []*diagnostic.Error {
	t.Helper()
	before := value
	var weight, subtitle string
	if value.Order != nil {
		weight = *value.Order
	}
	if value.Subtitle != nil {
		subtitle = *value.Subtitle
	}
	errors := validateEntry(value, allowNull, pointer, context)
	if value != before || value.Order != nil && *value.Order != weight || value.Subtitle != nil && *value.Subtitle != subtitle {
		t.Fatal("validateEntry изменил запись или строку по указателю")
	}
	return errors
}

func entryValidationContractDiagnostic(t *testing.T, got *diagnostic.Error, pointer string, context diagnostic.Context) {
	t.Helper()
	if got == nil {
		t.Fatal("получена nil вместо ошибки")
	}
	if got.Code == "" || got.Reason == "" || !utf8.ValidString(got.Reason) || !strings.ContainsAny(got.Reason, "абвгдежзийклмнопрстуфхцчшщыэюяАБВГДЕЖЗИЙКЛМНОПРСТУФХЦЧШЩЫЭЮЯ") {
		t.Errorf("нужны код и русская причина: %#v", got)
	}
	if got.Context != context {
		t.Errorf("Context = %#v, требуется %#v", got.Context, context)
	}
	for _, detail := range got.Details {
		if detail.Value == pointer {
			return
		}
	}
	t.Errorf("нет JSON Pointer %q в Details: %#v", pointer, got.Details)
}

func TestEntryValidationContractAcceptedScalars(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Entry)
	}{
		{"baseline", func(*Entry) {}},
		{"null-order", func(e *Entry) { e.Order = nil }},
		{"empty-subtitle", func(e *Entry) { e.Subtitle = entryValidationContractString("") }},
		{"arbitrary-subtitle", func(e *Entry) { e.Subtitle = entryValidationContractString("\x00\t\n \ufeffРусский 😀") }},
		{"ecmascript-not-whitespace", func(e *Entry) { e.ID = "A\u0085\u200b"; e.Title = "\u0085" }},
		{"large-name", func(e *Entry) { e.FileName = "l" + strings.Repeat("9", 300) + ".json" }},
		{"huge-order", func(e *Entry) { e.Order = entryValidationContractString("000" + strings.Repeat("9", 1000)) }},
		{"large-integers", func(e *Entry) { e.LineCount = json.Number(strings.Repeat("9", 300)); e.Version = e.LineCount }},
		{"integer-spellings", func(e *Entry) { e.LineCount = "-0.000"; e.Version = "100e-2" }},
		{"fraction-zero", func(e *Entry) { e.LineCount = "25.000"; e.Version = "1.0" }},
		{"leap-second", func(e *Entry) { e.CreatedAt = "2016-12-31T23:59:60Z" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := entryValidationContractValid()
			tc.edit(&value)
			if got := entryValidationContractRun(t, value, true, "/course/lessons/0", diagnostic.Context{}); len(got) != 0 {
				t.Fatalf("допустимая запись отклонена: %v", got)
			}
		})
	}
}

func TestEntryValidationContractRejectsEachField(t *testing.T) {
	cases := []struct {
		field, code string
		edit        func(*Entry)
	}{
		{"id", identity.CodeInvalidLessonID, func(e *Entry) { e.ID = "" }},
		{"id", identity.CodeInvalidLessonID, func(e *Entry) { e.ID = " A" }},
		{"id", identity.CodeInvalidLessonID, func(e *Entry) { e.ID = "A\ufeff" }},
		{"id", identity.CodeInvalidLessonID, func(e *Entry) { e.ID = "A\x7f" }},
		{"id", identity.CodeInvalidLessonID, func(e *Entry) { e.ID = string([]byte{0xff}) }},
		{"fileName", identity.CodeInvalidFileName, func(e *Entry) { e.FileName = "l000.json" }},
		{"fileName", identity.CodeInvalidFileName, func(e *Entry) { e.FileName = "l0001.json" }},
		{"fileName", identity.CodeInvalidFileName, func(e *Entry) { e.FileName = "l01.json" }},
		{"fileName", identity.CodeInvalidFileName, func(e *Entry) { e.FileName = "L001.json" }},
		{"fileName", identity.CodeInvalidFileName, func(e *Entry) { e.FileName = "dir/l001.json" }},
		{"fileName", identity.CodeInvalidFileName, func(e *Entry) { e.FileName = "l001.json\n" }},
		{"order", order.CodeInvalidWeight, func(e *Entry) { e.Order = entryValidationContractString("") }},
		{"order", order.CodeInvalidWeight, func(e *Entry) { e.Order = entryValidationContractString("000") }},
		{"order", order.CodeInvalidWeight, func(e *Entry) { e.Order = entryValidationContractString("+1") }},
		{"order", order.CodeInvalidWeight, func(e *Entry) { e.Order = entryValidationContractString("1.0") }},
		{"order", order.CodeInvalidWeight, func(e *Entry) { e.Order = entryValidationContractString("1e2") }},
		{"order", order.CodeInvalidWeight, func(e *Entry) { e.Order = entryValidationContractString("１") }},
		{"order", order.CodeInvalidWeight, func(e *Entry) { e.Order = entryValidationContractString("1\n") }},
		{"title", CodeTitle, func(e *Entry) { e.Title = "" }},
		{"title", CodeTitle, func(e *Entry) { e.Title = " \t\r\n\u00a0\ufeff" }},
		{"title", CodeTitle, func(e *Entry) { e.Title = string([]byte{0xff}) }},
		{"subtitle", "", func(e *Entry) { e.Subtitle = entryValidationContractString(string([]byte{0xff})) }},
		{"lineCount", CodeInteger, func(e *Entry) { e.LineCount = "" }},
		{"lineCount", CodeInteger, func(e *Entry) { e.LineCount = "-1" }},
		{"lineCount", CodeInteger, func(e *Entry) { e.LineCount = "0.1" }},
		{"lineCount", CodeInteger, func(e *Entry) { e.LineCount = "01" }},
		{"createdAt", CodeCreatedAt, func(e *Entry) { e.CreatedAt = "2023-02-29T12:34:56Z" }},
		{"createdAt", CodeCreatedAt, func(e *Entry) { e.CreatedAt = "2024-02-29T12:34:56+00:00" }},
		{"createdAt", CodeCreatedAt, func(e *Entry) { e.CreatedAt = "2024-02-29T12:34:56.0Z" }},
		{"version", CodeInteger, func(e *Entry) { e.Version = "0" }},
		{"version", CodeInteger, func(e *Entry) { e.Version = "-1" }},
		{"version", CodeInteger, func(e *Entry) { e.Version = "1.1" }},
		{"version", CodeInteger, func(e *Entry) { e.Version = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			value := entryValidationContractValid()
			tc.edit(&value)
			context := diagnostic.Context{Path: `C:\Архивы\курс.zip`, FileName: "manifest.json", LessonID: "outer"}
			got := entryValidationContractRun(t, value, true, "/course/unit/sections/2/lessons/3", context)
			if len(got) != 1 {
				t.Fatalf("нужна одна ошибка %s, получено %v", tc.field, got)
			}
			context.FileName, context.LessonID = value.FileName, value.ID
			if value.ID == "" {
				context.LessonID = "outer"
			}
			entryValidationContractDiagnostic(t, got[0], "/course/unit/sections/2/lessons/3/"+tc.field, context)
			if tc.code != "" && got[0].Code != tc.code {
				t.Errorf("Code = %q, требуется %q", got[0].Code, tc.code)
			}
		})
	}
}

func TestEntryValidationContractNullOrderFlag(t *testing.T) {
	for _, allow := range []bool{true, false} {
		value := entryValidationContractValid()
		value.Order = nil
		got := entryValidationContractRun(t, value, allow, "", diagnostic.Context{})
		if allow {
			if len(got) != 0 {
				t.Fatalf("null разрешён: %v", got)
			}
			continue
		}
		if len(got) != 1 || got[0] == nil || got[0].Code != CodeNullOrder {
			t.Fatalf("нужна CodeNullOrder: %v", got)
		}
		entryValidationContractDiagnostic(t, got[0], "/order", diagnostic.Context{FileName: value.FileName, LessonID: value.ID})
	}
	value := entryValidationContractValid()
	if got := entryValidationContractRun(t, value, false, "", diagnostic.Context{}); len(got) != 0 {
		t.Fatalf("запрет null не запрещает строку веса: %v", got)
	}
}

func TestEntryValidationContractAllErrorsInFieldOrder(t *testing.T) {
	value := Entry{ID: "bad id", FileName: "bad.json", Order: entryValidationContractString("00"),
		Title: "", Subtitle: entryValidationContractString(string([]byte{0xff})),
		LineCount: "-1", CreatedAt: "bad date", Version: "0"}
	context := diagnostic.Context{Path: "module.zip", FileName: "manifest.json", PageID: "known-page", Line: 17, Column: 0}
	got := entryValidationContractRun(t, value, false, "/course/lessons/4", context)
	fields := []string{"id", "fileName", "order", "title", "subtitle", "lineCount", "createdAt", "version"}
	if len(got) != len(fields) {
		t.Fatalf("нужны все восемь ошибок, получено %v", got)
	}
	context.FileName, context.LessonID = value.FileName, value.ID
	for i, field := range fields {
		entryValidationContractDiagnostic(t, got[i], "/course/lessons/4/"+field, context)
	}
	value.Order = nil
	got = entryValidationContractRun(t, value, false, "/course/lessons/4", context)
	if len(got) != 8 || got[2] == nil || got[2].Code != CodeNullOrder {
		t.Fatalf("ошибка null должна занимать место order: %v", got)
	}
}

func TestEntryValidationContractDependencyDiagnosticsPreserved(t *testing.T) {
	value := entryValidationContractValid()
	value.ID, value.FileName, value.Order = " bad id", "../l001.json", entryValidationContractString(" 00 ")
	_, nameError := identity.ParseFileName(value.FileName)
	expected := []*diagnostic.Error{identity.ValidateLessonID(value.ID), nameError, order.Validate(*value.Order, diagnostic.Context{})}
	got := entryValidationContractRun(t, value, true, "/lessons/9", diagnostic.Context{Path: "original.zip"})
	if len(got) != len(expected) {
		t.Fatalf("нужны три ошибки: %v", got)
	}
	for i, source := range expected {
		if source == nil || got[i] == nil {
			t.Fatal("ошибка зависимости отсутствует")
		}
		if got[i].Code != source.Code || got[i].Reason != source.Reason || got[i].Cause != source.Cause {
			t.Errorf("диагностика зависимости %d подменена: %#v", i, got[i])
		}
		next := 0
		for _, detail := range got[i].Details {
			if next < len(source.Details) && detail == source.Details[next] {
				next++
			}
		}
		if next != len(source.Details) {
			t.Errorf("Details зависимости потеряны: %#v", got[i].Details)
		}
		field := []string{"id", "fileName", "order"}[i]
		entryValidationContractDiagnostic(t, got[i], "/lessons/9/"+field, diagnostic.Context{Path: "original.zip", FileName: value.FileName, LessonID: value.ID})
	}
}

func TestEntryValidationContractUnknownCoordinatesRemainUnknown(t *testing.T) {
	got := entryValidationContractRun(t, Entry{}, false, "/lessons/0", diagnostic.Context{})
	if len(got) == 0 {
		t.Fatal("нулевая запись должна быть ошибочной")
	}
	for _, err := range got {
		if err == nil {
			t.Fatal("nil ошибка")
		}
		if err.Context != (diagnostic.Context{}) {
			t.Errorf("выдуман контекст: %#v", err.Context)
		}
	}
}
