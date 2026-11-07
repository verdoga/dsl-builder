package manifest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func scalarContractContext() diagnostic.Context {
	return diagnostic.Context{Path: "Курс/module.zip", FileName: "l001.json", LessonID: "Урок-A", PageID: "page", Line: 7, Column: 3}
}

func scalarContractCheck(t *testing.T, err *diagnostic.Error, valid bool, code, pointer string, context diagnostic.Context) {
	t.Helper()
	if valid {
		if err != nil {
			t.Fatalf("допустимое значение отклонено: %+v", err)
		}
		return
	}
	if err == nil {
		t.Fatal("недопустимое значение принято")
	}
	if err.Code != code || err.Context != context {
		t.Errorf("Code/Context = %q/%+v; ожидаются %q/%+v", err.Code, err.Context, code, context)
	}
	if !utf8.ValidString(err.Reason) || !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("причина должна быть понятным русским текстом UTF-8: %q", err.Reason)
	}
	for _, detail := range err.Details {
		if detail.Value == pointer {
			return
		}
	}
	t.Errorf("Details не содержит точный JSON Pointer %q: %+v", pointer, err.Details)
}

func TestScalarContractTitleECMAScriptWhitespace(t *testing.T) {
	context := scalarContractContext()
	// Точный класс ECMAScript \s: Go unicode.IsSpace включает лишний U+0085
	// и не включает U+FEFF; оба различия существенны для pattern схемы.
	spaces := "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"
	for _, r := range spaces {
		t.Run(fmt.Sprintf("space_U+%04X", r), func(t *testing.T) {
			scalarContractCheck(t, ValidateTitle(string(r), "/course/title", context), false, CodeTitle, "/course/title", context)
			scalarContractCheck(t, ValidateTitle(string(r)+"Я"+string(r), "/course/title", context), true, CodeTitle, "/course/title", context)
		})
	}
	for i, value := range []string{"", spaces, string([]byte{0xff}), "Я\xff", string([]byte{0xc0, 0xaf}), string([]byte{0xed, 0xa0, 0x80}), string([]byte{0xf4, 0x90, 0x80, 0x80}), "\xe2\x82"} {
		t.Run(fmt.Sprintf("invalid_%d", i), func(t *testing.T) {
			scalarContractCheck(t, ValidateTitle(value, "/course/title", context), false, CodeTitle, "/course/title", context)
		})
	}
	for i, value := range []string{"Курс", "  Я  ", "A  B", "\tA\r\n", "\u0085", "\u180e", "\u200b", "\u2060", "\x00", "\x1f", "\x7f", "\ufffd", "🎓", "e\u0301"} {
		t.Run(fmt.Sprintf("valid_%d", i), func(t *testing.T) {
			scalarContractCheck(t, ValidateTitle(value, "/course/title", context), true, CodeTitle, "/course/title", context)
		})
	}
}

func TestScalarContractSectionTitleExactRestrictions(t *testing.T) {
	context := scalarContractContext()
	pointer := "/course/sections/0/title"
	for i, value := range []string{"", " ", "\u00a0\ufeff", " A", "A ", "A  B", "A   B", "\tA", "A\tB", "A\t", "\rA", "A\rB", "A\r", "\nA", "A\nB", "A\n", "A\r\n", "A\xff"} {
		t.Run(fmt.Sprintf("invalid_%d", i), func(t *testing.T) {
			scalarContractCheck(t, ValidateSectionTitle(value, pointer, context), false, CodeTitle, pointer, context)
		})
	}
	for i, value := range []string{"Studentbook", "studentbook", "Раздел 1", "Я", "A B", "\u00a0A\u00a0", "A\u00a0\u00a0B", "\ufeffA\ufeff", "A\vB", "A\fB", "\vA\f", "A\u2028B", "A\u2029B", "\u3000A\u3000", "A \u00a0 B", "\u0085", "\u180e", "\x00", "A\x7fB", "a/b~c", "🎓"} {
		t.Run(fmt.Sprintf("valid_%d", i), func(t *testing.T) {
			scalarContractCheck(t, ValidateSectionTitle(value, pointer, context), true, CodeTitle, pointer, context)
		})
	}
}

func TestScalarContractCreatedAtCalendarAndExactUTC(t *testing.T) {
	context := scalarContractContext()
	pointer := "/createdAt"
	for _, value := range []string{"2026-11-04T09:00:00Z", "0000-02-29T00:00:00Z", "0001-01-01T00:00:00Z", "9999-12-31T23:59:59Z", "2000-02-29T12:34:56Z", "2024-02-29T00:00:00Z", "1900-02-28T23:59:59Z", "2015-06-30T23:59:60Z", "2016-12-31T23:59:60Z"} {
		t.Run("valid_"+value, func(t *testing.T) {
			scalarContractCheck(t, validateCreatedAt(value, pointer, context), true, CodeCreatedAt, pointer, context)
		})
	}
	for i, value := range []string{
		"", "2023-02-29T00:00:00Z", "1900-02-29T00:00:00Z", "2100-02-29T00:00:00Z", "2024-02-30T00:00:00Z",
		"2026-04-31T00:00:00Z", "2026-06-31T00:00:00Z", "2026-09-31T00:00:00Z", "2026-11-31T00:00:00Z",
		"2026-00-01T00:00:00Z", "2026-13-01T00:00:00Z", "2026-01-00T00:00:00Z", "2026-01-32T00:00:00Z",
		"2026-01-01T24:00:00Z", "2026-01-01T23:60:00Z", "2026-01-01T23:59:61Z",
		"2016-12-31T12:00:60Z", "2016-12-31T23:58:60Z", "2016-12-30T23:59:60Z",
		"2026-11-04t09:00:00Z", "2026-11-04T09:00:00z", "2026-11-04 09:00:00Z", "2026-11-04T09:00:00+00:00",
		"2026-11-04T09:00:00-00:00", "2026-11-04T09:00:00.0Z", "2026-11-04T09:00:00,0Z",
		"2026-1-04T09:00:00Z", "2026-11-4T09:00:00Z", "2026-11-04T9:00:00Z", "2026-11-04T09:0:00Z", "2026-11-04T09:00:0Z",
		"026-11-04T09:00:00Z", "12026-11-04T09:00:00Z", "+2026-11-04T09:00:00Z", "２０２６-11-04T09:00:00Z",
		" 2026-11-04T09:00:00Z", "2026-11-04T09:00:00Z ", "2026-11-04T09:00:00Z\n", "2026-11-04T09:00:00Z\x00", "2026-11-04T09:00:00Z\xff",
	} {
		t.Run(fmt.Sprintf("invalid_%d", i), func(t *testing.T) {
			scalarContractCheck(t, validateCreatedAt(value, pointer, context), false, CodeCreatedAt, pointer, context)
		})
	}
}

func TestScalarContractIntegerJSONGrammarAndMathematicalValue(t *testing.T) {
	context := scalarContractContext()
	for _, minimum := range []int{0, 1} {
		t.Run(fmt.Sprintf("minimum_%d", minimum), func(t *testing.T) {
			pointer := "/course/lessons/0/lineCount"
			for i, value := range []string{"1", "2", "9223372036854775808", "18446744073709551616", "9007199254740993", "1.0", "1.0000000000000000000000000", "1e0", "1E+0000", "1e-0", "1e2", "12.34e2", "123400e-2", "0.1e1", "0.00100e+3", "1000e-3", "100.00E-2", "1e2147483648", "1e9223372036854775808", "1e+" + strings.Repeat("9", 256), "1e-" + strings.Repeat("0", 256), strings.Repeat("9", 8192), "1." + strings.Repeat("0", 8192)} {
				t.Run(fmt.Sprintf("positive_%d", i), func(t *testing.T) {
					scalarContractCheck(t, validateInteger(json.Number(value), minimum, pointer, context), true, CodeInteger, pointer, context)
				})
			}
			for i, value := range []string{"0", "-0", "0.0", "-0.000", "0e0", "-0e-1", "0e+" + strings.Repeat("9", 256), "-0.000e-" + strings.Repeat("9", 256)} {
				t.Run(fmt.Sprintf("zero_%d", i), func(t *testing.T) {
					scalarContractCheck(t, validateInteger(json.Number(value), minimum, pointer, context), minimum == 0, CodeInteger, pointer, context)
				})
			}
			for i, value := range []string{
				"", " ", "+1", "01", "00", "-01", "00.0", "01e2", "-", "--1", "1.", ".1", "1e", "1e+", "1e-", "1e+-2", "1e1.0",
				"NaN", "Infinity", "-Infinity", "0x10", "1_000", "1/1", "true", "null", "\"1\"", "１", "١", "1 2", " 1", "1 ", "1\n", "\t1", "1\x00", "1\xff",
				"-1", "-1.0", "-1e3", "-0.1e1", "-" + strings.Repeat("9", 8192), "0.1", "1.5", "9007199254740992.1", "0.99999999999999999999999999", "1e-1", "1.01e1", "1001e-3",
				"1e-2147483648", "1e-9223372036854775808", "1e-" + strings.Repeat("9", 256),
			} {
				t.Run(fmt.Sprintf("invalid_%d", i), func(t *testing.T) {
					scalarContractCheck(t, validateInteger(json.Number(value), minimum, pointer, context), false, CodeInteger, pointer, context)
				})
			}
		})
	}
}

func TestScalarContractUnknownContextAndRootPointer(t *testing.T) {
	context := diagnostic.Context{}
	for _, test := range []struct {
		code     string
		validate func(string, string, diagnostic.Context) *diagnostic.Error
	}{{CodeTitle, ValidateTitle}, {CodeTitle, ValidateSectionTitle}, {CodeCreatedAt, validateCreatedAt}} {
		scalarContractCheck(t, test.validate("", "", context), false, test.code, "", context)
	}
	scalarContractCheck(t, validateInteger("", 0, "", context), false, CodeInteger, "", context)
}
