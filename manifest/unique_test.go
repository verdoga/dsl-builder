package manifest

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func uniqueContractString(value string) *string { return &value }

func uniqueContractEntry() Entry {
	return Entry{ID: "Урок:A", FileName: "l001.json", Order: uniqueContractString("1"),
		Title: "Название", Subtitle: uniqueContractString("Подзаголовок"),
		LineCount: "10", CreatedAt: "2026-11-04T09:00:00Z", Version: "1"}
}

func uniqueContractJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestUniqueContractEqualEntryFields(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Entry)
		equal  bool
	}{
		{"same values separate pointers", func(e *Entry) {
			e.Order = uniqueContractString("1")
			e.Subtitle = uniqueContractString("Подзаголовок")
		}, true},
		{"ID case", func(e *Entry) { e.ID = "урок:A" }, false},
		{"fileName", func(e *Entry) { e.FileName = "l002.json" }, false},
		{"order spelling", func(e *Entry) { e.Order = uniqueContractString("01") }, false},
		{"order null", func(e *Entry) { e.Order = nil }, false},
		{"title case", func(e *Entry) { e.Title = "название" }, false},
		{"title whitespace", func(e *Entry) { e.Title = " Название" }, false},
		{"subtitle case", func(e *Entry) { e.Subtitle = uniqueContractString("подзаголовок") }, false},
		{"subtitle null", func(e *Entry) { e.Subtitle = nil }, false},
		{"lineCount", func(e *Entry) { e.LineCount = "11" }, false},
		{"createdAt", func(e *Entry) { e.CreatedAt = "2026-11-04T09:00:01Z" }, false},
		{"version", func(e *Entry) { e.Version = "2" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left, right := uniqueContractEntry(), uniqueContractEntry()
			tc.change(&right)
			before := uniqueContractJSON(t, []Entry{left, right})
			if equalEntry(left, right) != tc.equal || equalEntry(right, left) != tc.equal {
				t.Fatalf("equality must be symmetric and equal %v", tc.equal)
			}
			if after := uniqueContractJSON(t, []Entry{left, right}); after != before {
				t.Fatal("equality changed its operands")
			}
		})
	}
	left, right := uniqueContractEntry(), uniqueContractEntry()
	left.Order, right.Order, left.Subtitle, right.Subtitle = nil, nil, nil, nil
	if !equalEntry(left, right) {
		t.Fatal("matching nulls must be equal")
	}
	right.Subtitle = uniqueContractString("")
	if equalEntry(left, right) || equalEntry(right, left) {
		t.Fatal("null subtitle differs from empty string")
	}
	left.Title, right.Title = "é", "e\u0301"
	left.Subtitle, right.Subtitle = nil, nil
	if equalEntry(left, right) {
		t.Fatal("strings must not undergo Unicode normalization")
	}
}

func TestUniqueContractEqualEntryNumbers(t *testing.T) {
	cases := []struct {
		left, right string
		equal       bool
	}{
		{"1", "1.0", true}, {"10", "1e1", true}, {"100", "10000e-2", true},
		{"1", "0.01e+2", true}, {"9007199254740992", "9007199254740993", false},
		{"18446744073709551616", "18446744073709551616.000", true},
		{"1e4000", "10e3999", true}, {"1e4000", "2e4000", false},
		{"1e9223372036854775808", "10e9223372036854775807", true},
		{"1e9223372036854775808", "1e9223372036854775809", false},
		{strings.Repeat("9", 600), strings.Repeat("9", 600) + ".0", true},
		{strings.Repeat("9", 600), strings.Repeat("9", 599) + "8", false},
	}
	for i, tc := range cases {
		for _, field := range []string{"lineCount", "version"} {
			t.Run(fmt.Sprintf("%s/%d", field, i), func(t *testing.T) {
				left, right := uniqueContractEntry(), uniqueContractEntry()
				if field == "lineCount" {
					left.LineCount, right.LineCount = json.Number(tc.left), json.Number(tc.right)
				} else {
					left.Version, right.Version = json.Number(tc.left), json.Number(tc.right)
				}
				before := uniqueContractJSON(t, []Entry{left, right})
				if equalEntry(left, right) != tc.equal || equalEntry(right, left) != tc.equal {
					t.Fatalf("%s: %s and %s equality must be %v", field, tc.left, tc.right, tc.equal)
				}
				if uniqueContractJSON(t, []Entry{left, right}) != before {
					t.Fatal("numbers were normalized in place")
				}
			})
		}
	}
	left, right := uniqueContractEntry(), uniqueContractEntry()
	left.LineCount, right.LineCount = "-0", "0.000e-999999999999999999999"
	if !equalEntry(left, right) {
		t.Fatal("all valid representations of zero lineCount are equal")
	}
}

func TestUniqueContractEqualSectionSequence(t *testing.T) {
	a, b := uniqueContractEntry(), uniqueContractEntry()
	b.ID, b.FileName, b.Order = "Другой", "l002.json", uniqueContractString("2")
	left := Section{Title: "Studentbook", Lessons: []Entry{a, b}}
	cases := []struct {
		name  string
		right Section
		equal bool
	}{
		{"equal", Section{Title: "Studentbook", Lessons: []Entry{a, b}}, true},
		{"title case", Section{Title: "studentbook", Lessons: []Entry{a, b}}, false},
		{"same title different entries", Section{Title: "Studentbook", Lessons: []Entry{b}}, false},
		{"reversed lessons", Section{Title: "Studentbook", Lessons: []Entry{b, a}}, false},
		{"prefix", Section{Title: "Studentbook", Lessons: []Entry{a}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := uniqueContractJSON(t, []Section{left, tc.right})
			if equalSection(left, tc.right) != tc.equal || equalSection(tc.right, left) != tc.equal {
				t.Fatalf("section equality must be %v", tc.equal)
			}
			if uniqueContractJSON(t, []Section{left, tc.right}) != before {
				t.Fatal("sections were reordered or changed")
			}
		})
	}
	right := Section{Title: left.Title, Lessons: []Entry{a, b}}
	right.Lessons[0].LineCount, right.Lessons[1].Version = "1e1", "1.0"
	if !equalSection(left, right) {
		t.Fatal("section equality must use numeric entry equality")
	}
}

func uniqueContractCheckDuplicates(t *testing.T, call func() []*diagnostic.Error, pointer string, indices []int, context diagnostic.Context) {
	t.Helper()
	errs := call()
	if len(indices) == 0 {
		if len(errs) != 0 {
			t.Fatalf("distinct objects reported as duplicates: %#v", errs)
		}
		return
	}
	if len(errs) == 0 {
		t.Fatal("duplicates were accepted")
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(pointer) + `/[0-9]+`)
	found := map[string]bool{}
	for _, err := range errs {
		if err == nil || err.Code != CodeUniqueItems {
			t.Fatalf("expected nonnil uniqueItems diagnostic: %#v", err)
		}
		if !strings.ContainsFunc(err.Reason, func(r rune) bool { return r >= 'А' && r <= 'я' || r == 'ё' || r == 'Ё' }) {
			t.Fatalf("expected Russian reason: %q", err.Reason)
		}
		if err.Context.Path != context.Path || err.Context.PageID != "" || err.Context.Line != 0 || err.Context.Column != 0 {
			t.Fatalf("lost archive or invented coordinates: %#v", err.Context)
		}
		for _, detail := range err.Details {
			for _, match := range pattern.FindAllString(detail.Value, -1) {
				found[match] = true
			}
		}
	}
	want, got := []string{}, []string{}
	for _, index := range indices {
		want = append(want, fmt.Sprintf("%s/%d", pointer, index))
	}
	for match := range found {
		got = append(got, match)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("duplicate pointers = %v, want all and only %v", got, want)
	}
	for range 5 {
		if !reflect.DeepEqual(errs, call()) {
			t.Fatal("diagnostics are not deterministic")
		}
	}
}

func TestUniqueContractValidateUniqueEntries(t *testing.T) {
	a, b, c := uniqueContractEntry(), uniqueContractEntry(), uniqueContractEntry()
	b.Title, c.Title = "Второй", "Одиночный"
	aNumeric := uniqueContractEntry()
	aNumeric.LineCount, aNumeric.Version = "1e1", "1.0"
	values := []Entry{a, b, aNumeric, c, b, a}
	context := diagnostic.Context{Path: `C:\Курс\модуль.zip`, FileName: "manifest.json"}
	pointer := "/course/unit/lessons"
	before := uniqueContractJSON(t, values)
	uniqueContractCheckDuplicates(t, func() []*diagnostic.Error { return validateUniqueEntries(values, pointer, context) }, pointer, []int{0, 1, 2, 4, 5}, context)
	if uniqueContractJSON(t, values) != before {
		t.Fatal("uniqueItems changed entries")
	}
	for _, distinct := range [][]Entry{nil, {}, {a}, {a, b, c}} {
		uniqueContractCheckDuplicates(t, func() []*diagnostic.Error { return validateUniqueEntries(distinct, pointer, context) }, pointer, nil, context)
	}
	for _, field := range []string{"id", "fileName", "order", "title"} {
		t.Run("only repeated "+field, func(t *testing.T) {
			other := Entry{ID: "Другой", FileName: "l002.json", Order: uniqueContractString("2"), Title: "Другой", Subtitle: nil, LineCount: "20", CreatedAt: "2026-11-05T10:00:00Z", Version: "2"}
			switch field {
			case "id":
				other.ID = a.ID
			case "fileName":
				other.FileName = a.FileName
			case "order":
				other.Order = uniqueContractString(*a.Order)
			case "title":
				other.Title = a.Title
			}
			if errs := validateUniqueEntries([]Entry{a, other}, pointer, context); len(errs) != 0 {
				t.Fatalf("single field repetition is not uniqueItems: %#v", errs)
			}
		})
	}
	b = uniqueContractEntry()
	b.Order = uniqueContractString("01")
	if errs := validateUniqueEntries([]Entry{a, b}, pointer, context); len(errs) != 0 {
		t.Fatal("numeric order conflict belongs to catalog")
	}
}

func TestUniqueContractValidateUniqueSections(t *testing.T) {
	a, b := uniqueContractEntry(), uniqueContractEntry()
	b.ID, b.Order = "Другой", uniqueContractString("2")
	first := Section{Title: "Studentbook", Lessons: []Entry{a, b}}
	numeric := Section{Title: "Studentbook", Lessons: []Entry{a, b}}
	numeric.Lessons[0].Version = "1.0"
	second := Section{Title: "Workbook", Lessons: []Entry{a}}
	values := []Section{first, second, {Title: first.Title, Lessons: []Entry{b, a}}, numeric, second, first}
	context := diagnostic.Context{Path: "архив.zip", FileName: "manifest.json"}
	pointer := "/course/sections"
	before := uniqueContractJSON(t, values)
	uniqueContractCheckDuplicates(t, func() []*diagnostic.Error { return validateUniqueSections(values, pointer, context) }, pointer, []int{0, 1, 3, 4, 5}, context)
	if uniqueContractJSON(t, values) != before {
		t.Fatal("uniqueItems changed sections")
	}
	for _, distinct := range [][]Section{nil, {}, {first}, {first, values[2]}, {first, {Title: first.Title, Lessons: []Entry{a}}}, {first, {Title: "studentbook", Lessons: first.Lessons}}} {
		uniqueContractCheckDuplicates(t, func() []*diagnostic.Error { return validateUniqueSections(distinct, pointer, context) }, pointer, nil, context)
	}
}
