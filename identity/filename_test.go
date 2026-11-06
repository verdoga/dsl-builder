package identity_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func TestFileNameParseCanonical(t *testing.T) {
	huge := "1" + strings.Repeat("234567890", 40)
	cases := []struct {
		name   string
		input  string
		number string
	}{
		{"one", "l001.json", "1"},
		{"nine", "l009.json", "9"},
		{"ten", "l010.json", "10"},
		{"ninety-nine", "l099.json", "99"},
		{"hundred", "l100.json", "100"},
		{"999", "l999.json", "999"},
		{"1000", "l1000.json", "1000"},
		{"1001", "l1001.json", "1001"},
		{"beyond-uint64", "l18446744073709551616.json", "18446744073709551616"},
		{"hundreds-of-digits", "l" + huge + ".json", huge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identity.ParseFileName(tc.input)
			if err != nil {
				t.Fatalf("ParseFileName(%q): %v", tc.input, err)
			}
			if got != tc.number {
				t.Errorf("ParseFileName(%q) = %q, want %q", tc.input, got, tc.number)
			}
		})
	}
}

func TestFileNameFormatCanonical(t *testing.T) {
	huge := "9" + strings.Repeat("876543210", 40)
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"one", "1", "l001.json"},
		{"ten", "10", "l010.json"},
		{"hundred", "100", "l100.json"},
		{"999", "999", "l999.json"},
		{"1000", "1000", "l1000.json"},
		{"short-padding", "01", "l001.json"},
		{"already-padded", "001", "l001.json"},
		{"excess-padding", "0000000001", "l001.json"},
		{"padded-999", "000999", "l999.json"},
		{"padded-1000", "0001000", "l1000.json"},
		{"beyond-uint64", "18446744073709551616", "l18446744073709551616.json"},
		{"hundreds-of-digits", huge, "l" + huge + ".json"},
		{"huge-padding", strings.Repeat("0", 400) + huge, "l" + huge + ".json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identity.FormatFileName(tc.input)
			if err != nil {
				t.Fatalf("FormatFileName(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("FormatFileName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFileNameParseRejectInvalid(t *testing.T) {
	cases := []struct{ name, input string }{
		{"empty", ""}, {"manifest", "manifest.json"},
		{"zero", "l000.json"}, {"short-zero", "l0.json"},
		{"long-zero", "l0000.json"}, {"huge-zero", "l" + strings.Repeat("0", 400) + ".json"},
		{"one-digit", "l1.json"}, {"two-digits", "l01.json"},
		{"extra-padding", "l0001.json"}, {"padded-999", "l0999.json"},
		{"padded-1000", "l01000.json"},
		{"uppercase-prefix", "L001.json"}, {"uppercase-extension", "l001.JSON"},
		{"mixed-extension", "l001.Json"}, {"missing-prefix", "001.json"},
		{"missing-number", "l.json"}, {"missing-extension", "l001"},
		{"extra-prefix", "xl001.json"}, {"extra-suffix", "l001.json.bak"},
		{"version-suffix", "l001-v01.json"}, {"wildcard-dot", "l001Xjson"},
		{"unix-path", "dir/l001.json"}, {"relative-path", "./l001.json"},
		{"windows-path", `dir\l001.json`}, {"trailing-path", "l001.json/"},
		{"leading-space", " l001.json"}, {"trailing-space", "l001.json "},
		{"internal-space", "l0 01.json"}, {"leading-tab", "\tl001.json"},
		{"trailing-newline", "l001.json\n"}, {"trailing-crlf", "l001.json\r\n"},
		{"trailing-line-separator", "l001.json\u2028"},
		{"leading-bom", "\ufeffl001.json"}, {"trailing-nbsp", "l001.json\u00a0"},
		{"nul", "l001.json\x00"}, {"invalid-utf8", "l001.json\xff"},
		{"arabic-digits", "l٠٠١.json"}, {"fullwidth-digits", "l００１.json"},
		{"mixed-digits", "l00١.json"}, {"lookalike-prefix", "ℓ001.json"},
		{"plus", "l+01.json"}, {"minus", "l-01.json"},
		{"decimal", "l1.0.json"}, {"exponent", "l1e3.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identity.ParseFileName(tc.input)
			if got != "" {
				t.Errorf("ParseFileName(%q) = %q on invalid input, want empty", tc.input, got)
			}
			fileNameAssertDiagnostic(t, err, identity.CodeInvalidFileName, tc.input)
		})
	}
}

func TestFileNameFormatRejectInvalidNumber(t *testing.T) {
	cases := []struct{ name, input string }{
		{"empty", ""}, {"zero", "0"}, {"padded-zero", "000"},
		{"huge-zero", strings.Repeat("0", 400)},
		{"plus", "+1"}, {"minus", "-1"}, {"negative-zero", "-0"},
		{"leading-space", " 1"}, {"trailing-space", "1 "},
		{"internal-space", "1 0"}, {"tab", "\t1"},
		{"newline", "1\n"}, {"nbsp", "\u00a01"},
		{"bom", "\ufeff1"}, {"nul", "1\x00"}, {"invalid-utf8", "1\xff"},
		{"arabic-digits", "١"}, {"fullwidth-digits", "１"},
		{"mixed-digits", "00١"}, {"decimal", "1.0"},
		{"exponent", "1e3"}, {"hex", "0x10"}, {"underscore", "1_000"},
		{"filename", "l001.json"}, {"path", "1/2"}, {"letter", "12a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := identity.FormatFileName(tc.input)
			if got != "" {
				t.Errorf("FormatFileName(%q) = %q on invalid input, want empty", tc.input, got)
			}
			fileNameAssertDiagnostic(t, err, identity.CodeInvalidNumber, tc.input)
		})
	}
}

func fileNameAssertDiagnostic(t *testing.T, err *diagnostic.Error, code, input string) {
	t.Helper()
	if err == nil {
		t.Fatalf("input %q: expected diagnostic %q", input, code)
	}
	if err.Code != code {
		t.Errorf("input %q: Code = %q, want %q", input, err.Code, code)
	}
	if err.Context != (diagnostic.Context{}) {
		t.Errorf("input %q: Context = %+v, want empty", input, err.Context)
	}
	if !utf8.ValidString(err.Reason) || !strings.ContainsAny(err.Reason, "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯабвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
		t.Errorf("input %q: Reason must contain Russian text in valid UTF-8: %q", input, err.Reason)
	}
	for _, detail := range err.Details {
		if detail.Value == input {
			return
		}
	}
	t.Errorf("Details omit exact erroneous input %q: %+v", input, err.Details)
}
