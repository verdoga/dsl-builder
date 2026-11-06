package identity

import (
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestLessonValidateValidIDs(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"ascii", "EFX7"},
		{"colon", "A:B"},
		{"plus", "lesson+1"},
		{"single_digit", "0"},
		{"single_punctuation", "+"},
		{"leading_combining_mark", "\u0301A"},
		{"punctuation_and_paths", "./A\\B:@#[](){}!?=,;\"'<>|%&*~`"},
		{"embedded_suffixes", "A-p001-n002-v03"},
		{"cyrillic", "УРОК-Ёж"},
		{"non_latin", "漢字😀"},
		{"replacement_character", "A\ufffdB"},
		{"maximum_scalar", "A\U0010ffffB"},
		{"long_id", strings.Repeat("Я+", 10000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := tc.value
			if err := ValidateLessonID(tc.value); err != nil {
				t.Fatalf("ValidateLessonID(%q): %v", tc.value, err)
			}
			if tc.value != original {
				t.Fatalf("исходный ID изменён: %q -> %q", original, tc.value)
			}
		})
	}
}

func TestLessonValidateInvalidUTF8AndEmpty(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"lone_continuation", "\x80"},
		{"invalid_start", "\xff"},
		{"truncated_two_bytes", "A\xc2"},
		{"truncated_three_bytes", "\xe2\x82"},
		{"truncated_four_bytes", "\xf0\x9f\x98"},
		{"overlong_nul", "\xc0\x80"},
		{"overlong_three_bytes", "\xe0\x80\xaf"},
		{"surrogate", "\xed\xa0\x80"},
		{"above_unicode_maximum", "\xf4\x90\x80\x80"},
		{"invalid_after_valid_unicode", "Урок\xfe"},
		{"invalid_between_valid_bytes", "A\xc2Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lessonTestRequireInvalid(t, tc.value)
		})
	}
}

func TestLessonValidateForbiddenCharacters(t *testing.T) {
	// Полный ECMAScript \s: WhiteSpace и LineTerminator. ASCII controls
	// ниже проверяются отдельно, включая все пересечения с этим списком.
	forbidden := []rune{
		'\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680',
		'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005',
		'\u2006', '\u2007', '\u2008', '\u2009', '\u200a',
		'\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff',
	}
	for r := rune(0); r <= 0x1f; r++ {
		forbidden = append(forbidden, r)
	}
	forbidden = append(forbidden, '\u007f')
	positions := []struct {
		name   string
		before string
		after  string
	}{
		{"alone", "", ""},
		{"leading", "", "Valid"},
		{"middle", "Val", "id"},
		{"trailing", "Valid", ""},
	}
	for _, r := range forbidden {
		for _, position := range positions {
			t.Run(fmt.Sprintf("U+%04X/%s", r, position.name), func(t *testing.T) {
				lessonTestRequireInvalid(t, position.before+string(r)+position.after)
			})
		}
	}
}

func TestLessonValidateAllowedUnicodeControlsAndNonspaces(t *testing.T) {
	// U+0085 отличает ECMAScript \s от unicode.IsSpace; U+180E, U+200B
	// и другие Cf нельзя запрещать как общий класс управляющих символов.
	allowed := []rune{
		'\u00ad', '\u034f', '\u061c', '\u1681', '\u180e',
		'\u200b', '\u200c', '\u200d', '\u200e', '\u200f',
		'\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
		'\u2060', '\u2061', '\u2062', '\u2063', '\u2064',
		'\u2066', '\u2067', '\u2068', '\u2069', '\u206a', '\u206f',
		'\ufff9', '\ufffa', '\ufffb', '\U000e0001', '\U000e007f',
	}
	for r := rune(0x80); r <= 0x9f; r++ {
		allowed = append(allowed, r)
	}
	for _, r := range allowed {
		t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
			for _, value := range []string{string(r), string(r) + "ID", "A" + string(r) + "B", "ID" + string(r)} {
				if err := ValidateLessonID(value); err != nil {
					t.Errorf("допустимый Unicode ID %q отклонён: %v", value, err)
				}
			}
		})
	}
}

func TestLessonKeyLowercaseWithoutChangingOriginal(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"ascii", "EFX7", "efx7"},
		{"colon", "A:B", "a:b"},
		{"plus", "Lesson+1", "lesson+1"},
		{"cyrillic", "УРОК-ЁЖ", "урок-ёж"},
		{"turkish_simple_case", "Iİıi", "iiıi"},
		{"greek_no_contextual_case", "ΟΣΣςσ", "οσσςσ"},
		{"sharp_s_no_expansion", "ẞß", "ßß"},
		{"long_s_no_casefold", "Sſ", "sſ"},
		{"kelvin", "KK", "kk"},
		{"composed", "É", "é"},
		{"decomposed", "E\u0301", "e\u0301"},
		{"compatibility_not_normalized", "ﬀⅠＡ", "ﬀⅰａ"},
		{"supplementary_case", "\U00010400", "\U00010428"},
		{"nonspace_control_retained", "A\u0085\u200bB", "a\u0085\u200bb"},
		{"uncased", "漢字😀", "漢字😀"},
		{"long", strings.Repeat("AЯ", 10000), strings.Repeat("aя", 10000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateLessonID(tc.value); err != nil {
				t.Fatalf("ошибка предусловия валидного ID: %v", err)
			}
			original := tc.value
			if got := LessonKey(tc.value); got != tc.want {
				t.Errorf("LessonKey(%q) = %q, want %q", tc.value, got, tc.want)
			}
			if tc.value != original {
				t.Errorf("исходный ID изменён: %q -> %q", original, tc.value)
			}
		})
	}
}

func TestLessonKeyIdentityRelations(t *testing.T) {
	cases := []struct {
		name  string
		left  string
		right string
		equal bool
	}{
		{"ascii_case", "EFX7", "efx7", true},
		{"cyrillic_case", "ЁЖ", "ёж", true},
		{"canonical_forms_stay_distinct", "É", "E\u0301", false},
		{"compatibility_forms_stay_distinct", "ﬀ", "ff", false},
		{"casefold_is_not_lowercase", "ſ", "S", false},
		{"greek_final_sigma", "Σ", "ς", false},
		{"dotless_i", "I", "ı", false},
		{"distinct_ids", "A:B", "A+B", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LessonKey(tc.left) == LessonKey(tc.right); got != tc.equal {
				t.Errorf("равенство ключей %q и %q = %t, want %t", tc.left, tc.right, got, tc.equal)
			}
		})
	}
}

func lessonTestRequireInvalid(t *testing.T, value string) {
	t.Helper()
	err := ValidateLessonID(value)
	if err == nil {
		t.Fatalf("ValidateLessonID(%q): ожидалась ошибка", value)
	}
	if err.Code != CodeInvalidLessonID {
		t.Errorf("Code = %q, want %q", err.Code, CodeInvalidLessonID)
	}
	if !utf8.ValidString(err.Reason) || strings.TrimSpace(err.Reason) == "" ||
		!strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("Reason должна быть непустым русским текстом UTF-8: %q", err.Reason)
	}
	if err.Context != (diagnostic.Context{}) {
		t.Errorf("Context должен оставаться пустым: %+v", err.Context)
	}
	for _, detail := range err.Details {
		if detail.Value == value {
			return
		}
	}
	t.Errorf("Details не содержит точное ошибочное значение %q: %#v", value, err.Details)
}
