package order_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/order"
)

func TestValidateAcceptsPositiveASCIIWeights(t *testing.T) {
	weights := []string{
		"1", "9", "10", "100", "1234567890", "01", "001", "0009000",
		"9007199254740993", "9223372036854775808", "18446744073709551616",
		strings.Repeat("9", 262144),
		strings.Repeat("0", 262143) + "1",
		"1" + strings.Repeat("0", 262143),
	}
	for i, raw := range weights {
		t.Run(fmt.Sprintf("weight_%d_length_%d", i, len(raw)), func(t *testing.T) {
			context := diagnostic.Context{Path: `C:\Вход\урок.json`, Line: 17}
			if err := order.Validate(raw, context); err != nil {
				t.Fatalf("Validate rejected positive ASCII weight of length %d: %v", len(raw), err)
			}
		})
	}
	for digit := '1'; digit <= '9'; digit++ {
		t.Run(fmt.Sprintf("nonzero_digit_%c", digit), func(t *testing.T) {
			for _, raw := range []string{string(digit), "000" + string(digit) + "000"} {
				if err := order.Validate(raw, diagnostic.Context{}); err != nil {
					t.Fatalf("Validate(%q) = %v, want nil", raw, err)
				}
			}
		})
	}
}

func TestValidateRejectsInvalidWeightForms(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""}, {"zero", "0"}, {"leading_zeros_only", "00000"},
		{"long_zeros_only", strings.Repeat("0", 262144)},
		{"plus", "+1"}, {"minus", "-1"}, {"negative_zero", "-0"},
		{"decimal", "1.0"}, {"comma_decimal", "1,0"}, {"fraction", "1/2"},
		{"lower_exponent", "1e3"}, {"upper_exponent", "1E+3"},
		{"hexadecimal", "0x10"}, {"binary", "0b10"}, {"octal_prefix", "0o10"},
		{"separator", "1_000"}, {"apostrophe", "1'000"},
		{"NaN", "NaN"}, {"infinity", "Inf"}, {"null_text", "null"},
		{"quoted", `"1"`}, {"ordinary_spaces", " 1 "},
		{"internal_space", "1 0"}, {"final_LF", "1\n"},
		{"final_CRLF", "1\r\n"}, {"extra_line", "1\n2"},
		{"arabic_indic", "١"}, {"extended_arabic_indic", "۱"},
		{"devanagari", "१"}, {"fullwidth", "１"},
		{"mixed_digits", "1٢"}, {"superscript", "¹"}, {"roman", "Ⅰ"},
		{"NBSP", "1\u00a0"}, {"thin_space", "1\u2009"},
		{"zero_width_space", "1\u200b"}, {"BOM", "\ufeff1"},
		{"unicode_line_separator", "1\u2028"},
		{"replacement_rune", "1\ufffd"}, {"unicode_minus", "−1"},
		{"truncated_UTF8", "1\xe2\x82"}, {"overlong_UTF8", "1\xc0\xb1"},
		{"surrogate_UTF8", "1\xed\xa0\x80"}, {"out_of_range_UTF8", "1\xf4\x90\x80\x80"},
		{"long_valid_prefix_then_LF", strings.Repeat("9", 262144) + "\n"},
		{"long_zeros_then_letter", strings.Repeat("0", 262144) + "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertOrderValidateDiagnostic(t, tc.raw, diagnostic.Context{})
		})
	}
}

func TestValidateRejectsEveryNonDigitByteEverywhere(t *testing.T) {
	for value := 0; value <= 255; value++ {
		if value >= '0' && value <= '9' {
			continue
		}
		invalid := string([]byte{byte(value)})
		cases := []struct {
			name string
			raw  string
		}{
			{"leading", invalid + "001"},
			{"middle", "01" + invalid + "0"},
			{"trailing", "001" + invalid},
		}
		for _, tc := range cases {
			t.Run(fmt.Sprintf("byte_%02x_%s", value, tc.name), func(t *testing.T) {
				assertOrderValidateDiagnostic(t, tc.raw, diagnostic.Context{})
			})
		}
	}
}

func TestValidatePreservesAllContextFields(t *testing.T) {
	contexts := []diagnostic.Context{
		{},
		{Path: `C:\Учебные файлы\..\исходник.json`},
		{FileName: "l0001.json"},
		{LessonID: "0007J"},
		{PageID: "0007J.001"},
		{Line: 17},
		{Line: 17, Column: 0},
		{Column: 12},
		{
			Path: `C:\Модуль\..\результат.zip`, FileName: "l001.json",
			LessonID: "0007J", PageID: "0007J.001", Line: 17, Column: 12,
		},
	}
	for i, context := range contexts {
		t.Run(fmt.Sprintf("context_%d", i), func(t *testing.T) {
			for _, raw := range []string{"", "000", " \t01\r\n", "1\xff"} {
				assertOrderValidateDiagnostic(t, raw, context)
			}
		})
	}
}

func assertOrderValidateDiagnostic(t *testing.T, raw string, context diagnostic.Context) {
	t.Helper()
	err := order.Validate(raw, context)
	if err == nil {
		t.Fatalf("Validate accepted invalid raw of length %d", len(raw))
	}
	if order.CodeInvalidWeight != "order.invalid_weight" {
		t.Errorf("CodeInvalidWeight = %q, want order.invalid_weight", order.CodeInvalidWeight)
	}
	if err.Code != "order.invalid_weight" {
		t.Errorf("Code = %q, want order.invalid_weight", err.Code)
	}
	if err.Context != context {
		t.Errorf("Context = %#v, want %#v", err.Context, context)
	}
	if strings.TrimSpace(err.Reason) == "" || !utf8.ValidString(err.Reason) {
		t.Errorf("Reason must be nonempty valid UTF-8: %q", err.Reason)
	}
	if !strings.ContainsAny(err.Reason, "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯабвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
		t.Errorf("Reason must explain the error in Russian: %q", err.Reason)
	}
	if len(err.Details) != 1 {
		t.Fatalf("Details length = %d, want one original-weight detail", len(err.Details))
	}
	if err.Details[0].Label != "Исходный вес" {
		t.Errorf("Details[0].Label = %q, want Исходный вес", err.Details[0].Label)
	}
	if err.Details[0].Value != raw {
		t.Errorf("Details[0].Value differs from exact raw (want length %d, got %d)", len(raw), len(err.Details[0].Value))
	}
	if err.Cause != nil {
		t.Errorf("Cause = %v, want nil", err.Cause)
	}
	if cause := errors.Unwrap(err); cause != nil {
		t.Errorf("Unwrap = %v, want nil", cause)
	}
}
