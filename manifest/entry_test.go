package manifest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/manifest"
)

func TestEntryModelMarshalIncludesExactlyEightRequiredFields(t *testing.T) {
	entry := manifest.Entry{LineCount: json.Number("0"), Version: json.Number("1")}
	fields := entryModelFields(t, entry)
	want := map[string]string{
		"id": `""`, "fileName": `""`, "order": "null", "title": `""`,
		"subtitle": "null", "lineCount": "0", "createdAt": `""`, "version": "1",
	}
	if len(fields) != len(want) {
		t.Fatalf("JSON содержит %d полей, требуется ровно восемь: %v", len(fields), fields)
	}
	for name, raw := range want {
		got, exists := fields[name]
		if !exists {
			t.Errorf("отсутствует обязательное поле %q", name)
			continue
		}
		if string(got) != raw {
			t.Errorf("%s = %s, требуется %s", name, got, raw)
		}
	}
}

func TestEntryModelNullableStringsDistinguishNullAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name     string
		order    *string
		subtitle *string
		want     [2]string
	}{
		{"both_null", nil, nil, [2]string{"null", "null"}},
		{"both_empty", entryModelString(""), entryModelString(""), [2]string{`""`, `""`}},
		{"null_order_empty_subtitle", nil, entryModelString(""), [2]string{"null", `""`}},
		{"empty_order_null_subtitle", entryModelString(""), nil, [2]string{`""`, "null"}},
		{"leading_zeros_and_whitespace", entryModelString("000010"), entryModelString(" \t\r\n"), [2]string{`"000010"`, `" \t\r\n"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := entryModelValidEntry()
			entry.Order, entry.Subtitle = tc.order, tc.subtitle
			fields := entryModelFields(t, entry)
			for index, name := range []string{"order", "subtitle"} {
				if string(fields[name]) != tc.want[index] {
					t.Errorf("%s = %s, требуется %s", name, fields[name], tc.want[index])
				}
			}
			entryModelAssertEqual(t, entryModelRoundTrip(t, entry), entry)
		})
	}
}

func TestEntryModelNumbersPreserveExactJSONSpelling(t *testing.T) {
	for _, tc := range []struct {
		name, lineCount, version string
	}{
		{"minimum", "0", "1"},
		{"negative_zero", "-0", "1"},
		{"beyond_float64", "9007199254740993", "9007199254740995"},
		{"beyond_uint64", "18446744073709551616", "18446744073709551617"},
		{"decimal_integer", "123456789012345678901234567890.000", "7.0"},
		{"exponent_case", "9e+30", "1E+31"},
		{"negative_exponent_integer", "100e-2", "1000E-3"},
		{"arbitrary_precision", strings.Repeat("9", 4096), "1" + strings.Repeat("0", 4096)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := entryModelValidEntry()
			entry.LineCount, entry.Version = json.Number(tc.lineCount), json.Number(tc.version)
			fields := entryModelFields(t, entry)
			for _, number := range []struct{ field, want string }{
				{"lineCount", tc.lineCount}, {"version", tc.version},
			} {
				if string(fields[number.field]) != number.want {
					t.Errorf("%s потеряло исходную числовую запись: получено %s, требуется %s", number.field, fields[number.field], number.want)
				}
			}
			entryModelAssertEqual(t, entryModelRoundTrip(t, entry), entry)
		})
	}
}

func TestEntryModelRoundTripPreservesAllLessonMetadata(t *testing.T) {
	entry := manifest.Entry{
		ID:        "AbЁж:lesson+1",
		FileName:  "l1000.json",
		Order:     entryModelString("000" + strings.Repeat("9", 4096)),
		Title:     "  Заголовок Ё и е\u0308 🦔\nВторая строка  ",
		Subtitle:  entryModelString("\tПодзаголовок с \"кавычками\"\r\n"),
		LineCount: json.Number("9007199254740993"),
		CreatedAt: "2000-02-29T01:02:03Z",
		Version:   json.Number("18446744073709551617"),
	}
	before := entry
	got := entryModelRoundTrip(t, entry)
	entryModelAssertEqual(t, got, before)
	entryModelAssertEqual(t, entry, before)
	// Эти поля независимы: ID не выводится из технического имени,
	// а lineCount и ревизия не вычисляются при кодировании модели.
	fields := entryModelFields(t, got)
	for _, tc := range []struct{ name, want string }{
		{"id", entry.ID}, {"fileName", entry.FileName}, {"order", *entry.Order},
		{"title", entry.Title}, {"subtitle", *entry.Subtitle}, {"createdAt", entry.CreatedAt},
	} {
		var decoded string
		if err := json.Unmarshal(fields[tc.name], &decoded); err != nil {
			t.Fatalf("%s не является строкой JSON: %v", tc.name, err)
		}
		if decoded != tc.want {
			t.Errorf("изменено поле %s: %q вместо %q", tc.name, decoded, tc.want)
		}
	}
}

func entryModelValidEntry() manifest.Entry {
	return manifest.Entry{
		ID: "MiXeD-Ёж", FileName: "l001.json", Order: entryModelString("001"),
		Title: "Урок", LineCount: json.Number("42"),
		CreatedAt: "2026-11-04T09:00:00Z", Version: json.Number("1"),
	}
}

func entryModelString(value string) *string {
	return &value
}

func entryModelFields(t *testing.T, entry manifest.Entry) map[string]json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("не удалось сериализовать Entry: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("Entry не сериализован объектом JSON: %v", err)
	}
	return fields
}

func entryModelRoundTrip(t *testing.T, entry manifest.Entry) manifest.Entry {
	t.Helper()
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var decoded manifest.Entry
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("не удалось декодировать Entry: %v", err)
	}
	return decoded
}

func entryModelAssertEqual(t *testing.T, got, want manifest.Entry) {
	t.Helper()
	if got.ID != want.ID || got.FileName != want.FileName || got.Title != want.Title ||
		got.LineCount != want.LineCount || got.CreatedAt != want.CreatedAt || got.Version != want.Version {
		t.Error("строки или точная запись чисел Entry изменены после сериализации")
	}
	for _, nullable := range []struct {
		name      string
		got, want *string
	}{
		{"order", got.Order, want.Order}, {"subtitle", got.Subtitle, want.Subtitle},
	} {
		if nullable.got == nil || nullable.want == nil {
			if (nullable.got == nil) != (nullable.want == nil) {
				t.Errorf("%s: потеряно различие null и строки", nullable.name)
			}
			continue
		}
		if *nullable.got != *nullable.want {
			t.Errorf("%s: изменено исходное строковое значение", nullable.name)
		}
	}
}
