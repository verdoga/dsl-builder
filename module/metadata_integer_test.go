package module

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func uniqueMetaValidInteger(t *testing.T, number json.Number) {
	t.Helper()
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "INTEGER", Title: "Целое", Version: "1", LineCount: number,
		CreatedAt: "2026-11-04T00:00:00Z", Pages: []lesson.Page{},
	}}
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("number %q must already be a valid nonnegative integer: %+v", number, problems)
	}
}

func TestUniqueMetaEqualIntegerExactValues(t *testing.T) {
	cases := []struct {
		name, left, right string
		equal             bool
	}{
		{"same", "1", "1", true},
		{"decimal zeros", "1", "1.000000000000000000000000", true},
		{"exponent zero", "1.0", "1e0", true},
		{"uppercase signed exponent", "1", "1.000E+000", true},
		{"negative exponent integer", "12300e-2", "123", true},
		{"decimal shifted", "12.3000e+2", "1230", true},
		{"fraction shifted to integer", "0.001e3", "1", true},
		{"very small coefficient", "0.00000000000000000000001e23", "1", true},
		{"zero", "0", "0.0", true},
		{"negative zero", "-0", "0", true},
		{"negative decimal zero", "-0.000e-10", "0e+10", true},
		{"huge positive zero exponent", "0e999999999999999999999999", "0", true},
		{"huge negative zero exponent", "0e-999999999999999999999999", "-0.000", true},
		{"float precision neighbor", "9007199254740992", "9007199254740993", false},
		{"uint64 precision neighbor", "18446744073709551616", "18446744073709551617", false},
		{"large exact decimal", "18446744073709551617", "18446744073709551617.000", true},
		{"large shifted coefficient", "18446744073709551617000e-3", "18446744073709551617", true},
		{"long coefficients differ at end", strings.Repeat("9", 200) + "8", strings.Repeat("9", 200) + "9", false},
		{"long coefficient zeros", strings.Repeat("7", 200) + "000e-3", strings.Repeat("7", 200), true},
		{"large exponent finite", "1e1000", "10e999", true},
		{"large exponent distinct", "1e1000", "1e1001", false},
		{"large exponent coefficient distinct", "1e1000", "2e1000", false},
		{"beyond int exponent", "1e9223372036854775809", "10e9223372036854775808", true},
		{"beyond uint exponent", "1e18446744073709551617", "100e18446744073709551615", true},
		{"arbitrary exponent precision", "1e999999999999999999999999", "10e999999999999999999999998", true},
		{"arbitrary exponents differ", "1e999999999999999999999999", "1e999999999999999999999998", false},
		{"long mantissa huge exponent", strings.Repeat("123456789", 40) + "000e999999999999999999999996", strings.Repeat("123456789", 40) + "e999999999999999999999999", true},
		{"long mantissa huge exponent differs", strings.Repeat("123456789", 40) + "1e999999999999999999999999", strings.Repeat("123456789", 40) + "2e999999999999999999999999", false},
		{"zero differs from one", "0e1000", "1", false},
		{"nonzero magnitude differs", "1000", "100", false},
		{"same digit count differs", "12345678901234567890", "12345678901234567891", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left, right := json.Number(tc.left), json.Number(tc.right)
			uniqueMetaValidInteger(t, left)
			uniqueMetaValidInteger(t, right)
			if got := equalInteger(left, right); got != tc.equal {
				t.Errorf("equalInteger(%q, %q) = %v, want %v", left, right, got, tc.equal)
			}
			if got := equalInteger(right, left); got != tc.equal {
				t.Errorf("reverse equalInteger(%q, %q) = %v, want %v", right, left, got, tc.equal)
			}
			if !equalInteger(left, left) || !equalInteger(right, right) {
				t.Error("valid integers must equal themselves")
			}
			if string(left) != tc.left || string(right) != tc.right {
				t.Fatal("comparison changed original number spelling")
			}
		})
	}
}

func TestUniqueMetaNumericalSpellingsPreserved(t *testing.T) {
	cases := []struct{ lineCount, version, entryLines, entryVersion string }{
		{"1", "1", "1.0", "1e0"},
		{"0", "1.00e0", "-0.000E+1000", "1000e-3"},
		{"9007199254740993", "18446744073709551617", "9007199254740993.000", "184467440737095516170e-1"},
		{"1e18446744073709551617", "10e18446744073709551616", "100e18446744073709551615", "1e18446744073709551617"},
	}
	for _, tc := range cases {
		t.Run(tc.entryLines+"/"+tc.entryVersion, func(t *testing.T) {
			value := uniqueMetaLesson(t)
			value.Document.LineCount, value.Document.Version = json.Number(tc.lineCount), json.Number(tc.version)
			file := uniqueMetaFile(t, value)
			item := uniqueMetaItem(t, file)
			item.Entry.LineCount, item.Entry.Version = json.Number(tc.entryLines), json.Number(tc.entryVersion)
			uniqueMetaCheckEntry(t, item.Entry)
			before := string(uniqueMetaJSON(t, item))
			if problems := validateEntryMetadata(file, item); len(problems) != 0 {
				t.Fatalf("equal mathematical metadata rejected: %+v", problems)
			}
			if string(uniqueMetaJSON(t, item)) != before || file.Lesson().Document.LineCount != json.Number(tc.lineCount) || file.Lesson().Document.Version != json.Number(tc.version) {
				t.Fatal("comparison normalized numeric spelling")
			}
		})
	}
}
