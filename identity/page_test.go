package identity_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func TestPageParseCanonical(t *testing.T) {
	huge := "18446744073709551616" + strings.Repeat("9876543210", 30)
	cases := []struct {
		name, input, prefix, number string
	}{
		{"minimum", "efx7-p001", "efx7", "1"},
		{"two_digits", "efx7-p010", "efx7", "10"},
		{"hundred", "efx7-p100", "efx7", "100"},
		{"999", "efx7-p999", "efx7", "999"},
		{"1000", "efx7-p1000", "efx7", "1000"},
		{"beyond_uint64", "a-p18446744073709551616", "a", "18446744073709551616"},
		{"huge", "a-p" + huge, "a", huge},
		{"colon", "a:b-p001", "a:b", "1"},
		{"plus", "lesson+1-p001", "lesson+1", "1"},
		{"rightmost", "a-p001-p002", "a-p001", "2"},
		{"similar_fragments", "a-p-n001-v02-p003", "a-p-n001-v02", "3"},
		{"prefix_ends_p", "a-p-p004", "a-p", "4"},
		{"prefix_ends_dash", "a--p001", "a-", "1"},
		{"punctuation", ":+/\\?-p001", ":+/\\?", "1"},
		{"cyrillic", "урок-ё-п-p001", "урок-ё-п", "1"},
		{"combining", "e\u0301-p001", "e\u0301", "1"},
		{"non_whitespace_controls", "a\u0085\u200b\u180e-p001", "a\u0085\u200b\u180e", "1"},
		{"long_prefix", strings.Repeat("a", 1024) + "-p001", strings.Repeat("a", 1024), "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identity.ParsePageID(tc.input)
			if err != nil {
				t.Fatalf("ParsePageID(%q): %v", tc.input, err)
			}
			want := identity.PageIDParts{Prefix: tc.prefix, Number: tc.number}
			if got != want {
				t.Errorf("parts = %#v, want %#v", got, want)
			}
		})
	}
}

func TestPageParseRejectsNoncanonical(t *testing.T) {
	cases := []struct{ name, input string }{
		{"empty", ""}, {"no_separator", "a001"}, {"no_number", "a-p"},
		{"no_prefix", "-p001"}, {"uppercase_prefix", "A-p001"},
		{"unicode_uppercase", "Урок-p001"}, {"uppercase_marker", "a-P001"},
		{"one_digit", "a-p1"}, {"two_digits", "a-p01"},
		{"zero", "a-p000"}, {"long_zero", "a-p0000"},
		{"excess_padding", "a-p0001"}, {"padding_1000", "a-p01000"},
		{"padding_huge", "a-p018446744073709551616"},
		{"signed", "a-p+001"}, {"negative", "a-p-001"},
		{"arabic_digits", "a-p٠٠١"}, {"fullwidth_digits", "a-p００１"},
		{"mixed_digits", "a-p00١"}, {"decimal", "a-p1.0"},
		{"revision_suffix", "a-p001-v02"}, {"extra_marker", "a-p001-p"},
		{"trailing_letter", "a-p001x"}, {"trailing_space", "a-p001 "},
		{"trailing_newline", "a-p001\n"}, {"trailing_nul", "a-p001\x00"},
		{"leading_space", " a-p001"}, {"embedded_space", "a b-p001"},
		{"invalid_utf8_prefix", "a\xff-p001"}, {"invalid_utf8_number", "a-p00\xff"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identity.ParsePageID(tc.input)
			if got != (identity.PageIDParts{}) {
				t.Errorf("failure parts = %#v, want zero", got)
			}
			pageCheckDiagnostic(t, err, identity.CodeInvalidPageID, tc.input)
		})
	}
}

func TestPagePrefixSchemaWhitespaceAndControls(t *testing.T) {
	// Набор ECMAScript \s, U+0000–U+001F и U+007F не равен unicode.IsSpace.
	for _, forbidden := range "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff" + "\x00\x01\x1f\x7f" {
		t.Run(string(forbidden), func(t *testing.T) {
			lessonID := "a" + string(forbidden) + "b"
			input := lessonID + "-p001"
			parts, parseErr := identity.ParsePageID(input)
			if parts != (identity.PageIDParts{}) {
				t.Errorf("failure parts = %#v, want zero", parts)
			}
			pageCheckDiagnostic(t, parseErr, identity.CodeInvalidPageID, input)
			formatted, formatErr := identity.FormatPageID(lessonID, "1")
			if formatted != "" {
				t.Errorf("failure result = %q, want empty", formatted)
			}
			pageCheckDiagnostic(t, formatErr, identity.CodeInvalidLessonID, lessonID)
		})
	}
}

func TestPageFormatCanonical(t *testing.T) {
	huge := "18446744073709551616" + strings.Repeat("1234567890", 30)
	cases := []struct{ name, lessonID, number, want string }{
		{"minimum", "EFX7", "1", "efx7-p001"},
		{"one_leading_zero", "EFX7", "01", "efx7-p001"},
		{"excess_input_padding", "EFX7", "00000001", "efx7-p001"},
		{"two_digits", "EFX7", "00010", "efx7-p010"},
		{"hundred", "EFX7", "00100", "efx7-p100"},
		{"999", "EFX7", "00999", "efx7-p999"},
		{"1000", "EFX7", "0001000", "efx7-p1000"},
		{"beyond_uint64", "A", "18446744073709551616", "a-p18446744073709551616"},
		{"huge", "A", "000" + huge, "a-p" + huge},
		{"colon", "A:B", "1", "a:b-p001"},
		{"plus", "Lesson+1", "1", "lesson+1-p001"},
		{"rightmost", "A-p001-N002-v03", "2", "a-p001-n002-v03-p002"},
		{"unicode_lowercase", "УРОК-Ё", "3", "урок-ё-p003"},
		{"no_normalization", "E\u0301", "4", "e\u0301-p004"},
		{"punctuation", ":+/\\?", "1", ":+/\\?-p001"},
		{"non_whitespace_controls", "A\u0085\u200b\u180e", "1", "a\u0085\u200b\u180e-p001"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identity.FormatPageID(tc.lessonID, tc.number)
			if err != nil {
				t.Fatalf("FormatPageID(%q, %q): %v", tc.lessonID, tc.number, err)
			}
			if got != tc.want {
				t.Errorf("result = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPageFormatRejectsInvalidLessonID(t *testing.T) {
	for _, lessonID := range []string{"", " A", "A ", "a\nb", "a\x00b", "a\x7fb", "a\xffb"} {
		t.Run(lessonID, func(t *testing.T) {
			got, err := identity.FormatPageID(lessonID, "1")
			if got != "" {
				t.Errorf("failure result = %q, want empty", got)
			}
			pageCheckDiagnostic(t, err, identity.CodeInvalidLessonID, lessonID)
		})
	}
}

func TestPageFormatRejectsInvalidNumber(t *testing.T) {
	for _, number := range []string{"", "0", "00", "0000", "+1", "-1", "1.0", "1e3", " 1", "1 ", "1\n", "1\x00", "١", "１", "00١", "1_000", "1-v02", "1\xff"} {
		t.Run(number, func(t *testing.T) {
			got, err := identity.FormatPageID("A:B", number)
			if got != "" {
				t.Errorf("failure result = %q, want empty", got)
			}
			pageCheckDiagnostic(t, err, identity.CodeInvalidNumber, number)
		})
	}
}

func pageCheckDiagnostic(t *testing.T, err *diagnostic.Error, code, input string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s for %q, got nil", code, input)
	}
	if err.Code != code {
		t.Errorf("Code = %q, want %q", err.Code, code)
	}
	if err.Context != (diagnostic.Context{}) {
		t.Errorf("Context = %#v, want empty", err.Context)
	}
	if !utf8.ValidString(err.Reason) || strings.TrimSpace(err.Reason) == "" || !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("Reason must be nonempty Russian UTF-8: %q", err.Reason)
	}
	for _, detail := range err.Details {
		if detail.Value == input {
			return
		}
	}
	t.Errorf("Details = %#v, must contain exact input %q", err.Details, input)
}
