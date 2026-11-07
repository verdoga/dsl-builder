package preview

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestManifestValueDistinguishesAbsentNullAndString(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"absent", nil, "отсутствует"},
		{"null", json.RawMessage(" null \n"), "null"},
		{"empty string", json.RawMessage(`""`), `""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := manifestValue(tc.raw)
			if err != nil || got != tc.want {
				t.Fatalf("manifestValue() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestManifestValuePreservesExactNumberTokens(t *testing.T) {
	for _, token := range []string{
		"9007199254740993", "184467440737095516160000000000000000000001",
		"1e999999999999999999999999", "-2.3000E+1000000000000000000000", "-0", "0.0000100",
	} {
		t.Run(token, func(t *testing.T) {
			raw := json.RawMessage(" \n" + token + "\t ")
			before := bytes.Clone(raw)
			got, err := manifestValue(raw)
			if err != nil || got != token {
				t.Fatalf("manifestValue() = %q, %v; want exact token %q", got, err, token)
			}
			if !bytes.Equal(raw, before) {
				t.Fatal("RawMessage was changed")
			}
		})
	}
}

func TestManifestValueQuotesWholeStringSafely(t *testing.T) {
	for _, value := range []string{
		"00000000000000000000000000000000000000001",
		"  Кириллица 中文 ⛵ \\\"\t\n\r\x00\x1b[2J\u0085  ",
		"null", "",
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		before := bytes.Clone(raw)
		got, err := manifestValue(raw)
		if err != nil {
			t.Fatal(err)
		}
		manifestTestSafe(t, got, false)
		quoted := manifestTestQuotedValues(t, got)
		if len(quoted) != 1 || quoted[0] != value {
			t.Fatalf("quoted value = %#v; want original %q", quoted, value)
		}
		if !bytes.Equal(raw, before) {
			t.Fatal("RawMessage was changed")
		}
	}
}

func TestManifestValuePreservesNestedNumberTokens(t *testing.T) {
	want := []string{"9007199254740993", "1e999999999999999999999999", "-2.3000E+1000000000000000000000"}
	raw := json.RawMessage(` { "numbers": [ ` + strings.Join(want, ", ") + ` ] } `)
	before := bytes.Clone(raw)
	got, err := manifestValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	var object struct {
		Numbers []json.RawMessage `json:"numbers"`
	}
	if err := json.Unmarshal([]byte(got), &object); err != nil || len(object.Numbers) != len(want) {
		t.Fatalf("nested numbers = %q, %v", got, err)
	}
	for i, token := range object.Numbers {
		if string(token) != want[i] {
			t.Errorf("token %d = %s; want %s", i, token, want[i])
		}
	}
	if strings.ContainsAny(got, " \n\r\t") || !bytes.Equal(raw, before) {
		t.Fatalf("JSON is not compact or input changed: %q", got)
	}
}

func TestManifestValueKeepsArraysAndNestedStrings(t *testing.T) {
	want := []string{"Я\n\x1b[31m\u0085", "А\t\r\x00", "中文 ⛵", "Я\n\x1b[31m\u0085"}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(raw)
	got, err := manifestValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	manifestTestSafe(t, got, false)
	var values []string
	if err := json.Unmarshal([]byte(got), &values); err != nil {
		t.Fatalf("array is not compact JSON: %v", err)
	}
	if !slices.Equal(values, want) || !bytes.Equal(raw, before) {
		t.Fatalf("array order, values or input changed: %#v", values)
	}
	for _, raw := range []json.RawMessage{json.RawMessage(` true `), json.RawMessage(` false `), json.RawMessage(` [ ] `)} {
		got, err := manifestValue(raw)
		if err != nil || strings.ContainsAny(got, " \n\r\t") || !json.Valid([]byte(got)) {
			t.Fatalf("compact value = %q, %v", got, err)
		}
	}
}

func TestManifestValueRejectsInvalidJSONWithoutLeakingIt(t *testing.T) {
	for _, raw := range []json.RawMessage{
		{}, []byte(" \n\t"), []byte(`{"CONFIDENTIAL_MANIFEST_PAYLOAD":`),
		[]byte(`"CONFIDENTIAL_MANIFEST_PAYLOAD" null`), []byte(`01`), []byte(`NaN`),
	} {
		before := bytes.Clone(raw)
		got, err := manifestValue(raw)
		manifestTestError(t, got, err)
		if !bytes.Equal(raw, before) {
			t.Fatal("invalid RawMessage was changed")
		}
	}
}

func manifestTestSafe(t *testing.T, text string, allowLF bool) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Fatal("output is not UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) && !(allowLF && r == '\n') {
			t.Fatalf("raw control character U+%04X in %q", r, text)
		}
	}
}

func manifestTestQuotedValues(t *testing.T, text string) []string {
	t.Helper()
	var values []string
	for {
		start := strings.IndexByte(text, '"')
		if start < 0 {
			return values
		}
		text = text[start:]
		token, err := strconv.QuotedPrefix(text)
		if err != nil {
			t.Fatalf("unterminated quoted value in %q: %v", text, err)
		}
		var value string
		if err := json.Unmarshal([]byte(token), &value); err != nil {
			value, err = strconv.Unquote(token)
			if err != nil {
				t.Fatalf("cannot decode quoted value %q: %v", token, err)
			}
		}
		values = append(values, value)
		text = text[len(token):]
	}
}

func manifestTestError(t *testing.T, text string, err error) {
	t.Helper()
	if err == nil || text != "" {
		t.Fatalf("want empty text and error, got %q, %v", text, err)
	}
	if strings.Contains(err.Error(), "CONFIDENTIAL_MANIFEST_PAYLOAD") {
		t.Fatal("error leaked input JSON")
	}
	if !strings.ContainsFunc(err.Error(), func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Fatalf("error is not in Russian: %v", err)
	}
	manifestTestSafe(t, err.Error(), false)
}
