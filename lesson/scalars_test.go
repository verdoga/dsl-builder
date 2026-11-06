package lesson

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func lessonScalarsContext() diagnostic.Context {
	return diagnostic.Context{Path: "архив.zip", FileName: "урок.json", LessonID: "AbC", PageID: "AbC-p001", Line: 17, Column: 4}
}

func lessonScalarsCheck(t *testing.T, got *diagnostic.Error, valid bool, pointer string) {
	t.Helper()
	if valid {
		if got != nil {
			t.Fatalf("valid input: %v", got)
		}
		return
	}
	if got == nil {
		t.Fatal("invalid input accepted")
	}
	if got.Code != CodeContract || got.Context != lessonScalarsContext() {
		t.Errorf("diagnostic code/context = %q, %+v", got.Code, got.Context)
	}
	if !strings.ContainsFunc(got.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("reason must explain the problem in Russian: %q", got.Reason)
	}
	found := false
	for _, detail := range got.Details {
		if detail.Label == "Поле JSON" && detail.Value == pointer {
			found = true
		}
	}
	if !found {
		t.Errorf("missing JSON pointer %q in %+v", pointer, got.Details)
	}
	if got.Cause != nil && !errors.Is(got, got.Cause) {
		t.Error("technical cause is not in the error chain")
	}
}

func TestLessonScalarsInteger(t *testing.T) {
	cases := []struct {
		value     string
		zero, one bool
	}{
		{"0", true, false}, {"-0", true, false}, {"-0.000e+100", true, false},
		{"0e-999999999999999999999999999999", true, false},
		{"1", true, true}, {"1.0", true, true}, {"1e0", true, true}, {"1E+000", true, true},
		{"10e-1", true, true}, {"100.00e-2", true, true}, {"0.001e3", true, true},
		{"9007199254740993", true, true}, {"18446744073709551616", true, true},
		{strings.Repeat("9", 4096), true, true},
		{"1e999999999999999999999999999999", true, true},
		{"-1", false, false}, {"-1.0", false, false}, {"-0.1e1", false, false},
		{"0.1", false, false}, {"1e-1", false, false}, {"100e-3", false, false},
		{"9007199254740992.5", false, false}, {"1.0000000000000000000000001", false, false},
		{"1e-999999999999999999999999999999", false, false},
		{"", false, false}, {"+1", false, false}, {"01", false, false}, {"-01", false, false},
		{".1", false, false}, {"1.", false, false}, {"1e", false, false}, {"1e+", false, false},
		{"NaN", false, false}, {"Inf", false, false}, {"0x10", false, false},
		{" 1", false, false}, {"1\n", false, false}, {"1 2", false, false},
		{"null", false, false}, {"true", false, false}, {`"1"`, false, false}, {"١", false, false},
	}
	for i, tc := range cases {
		for minimum, valid := range []bool{tc.zero, tc.one} {
			t.Run(strconv.Itoa(i)+"/minimum"+strconv.Itoa(minimum), func(t *testing.T) {
				number := json.Number(tc.value)
				lessonScalarsCheck(t, validateInteger(number, minimum, "/document/version", lessonScalarsContext()), valid, "/document/version")
				if number.String() != tc.value {
					t.Error("number spelling changed")
				}
			})
		}
	}
}

func TestLessonScalarsCreatedAt(t *testing.T) {
	valid := []string{
		"0000-02-29T00:00:00Z", "0001-01-01T00:00:00Z", "1900-02-28T23:59:59Z",
		"2000-02-29T12:34:56Z", "2024-02-29T23:59:59Z", "9999-12-31T23:59:59Z",
		"2015-06-30T23:59:60Z", "2016-12-31T23:59:60Z",
	}
	invalid := []string{
		"", "2024-02-30T00:00:00Z", "1900-02-29T00:00:00Z", "2100-02-29T00:00:00Z",
		"2023-02-29T00:00:00Z", "2024-04-31T00:00:00Z", "2024-00-01T00:00:00Z",
		"2024-13-01T00:00:00Z", "2024-01-00T00:00:00Z", "2024-01-32T00:00:00Z",
		"2024-01-01T24:00:00Z", "2024-01-01T23:60:00Z", "2016-12-31T23:59:61Z",
		"2016-12-31T23:58:60Z", "2016-12-30T23:59:60Z", "2016-05-31T23:59:60Z",
		"2024-01-01T00:00:00.0Z", "2024-01-01T00:00:00,1Z", "2024-01-01T00:00:00+00:00",
		"2024-01-01T00:00:00-00:00", "2024-01-01t00:00:00z", "2024-01-01 00:00:00Z",
		"2024-1-01T00:00:00Z", "2024-01-01T0:00:00Z", "024-01-01T00:00:00Z",
		"10000-01-01T00:00:00Z", "2024-01-01T00:00:00Z\n", " 2024-01-01T00:00:00Z",
	}
	for _, group := range []struct {
		values []string
		valid  bool
	}{{valid, true}, {invalid, false}} {
		for _, value := range group.values {
			t.Run(value, func(t *testing.T) {
				lessonScalarsCheck(t, validateCreatedAt(value, "/document/createdAt", lessonScalarsContext()), group.valid, "/document/createdAt")
			})
		}
	}
}

func TestLessonScalarsCreatedAtLeapSeconds(t *testing.T) {
	// RFC 3339 §§5.7–5.8 и Appendix D: :60 требует фактической вставки.
	// IERS-derived tz/leap-seconds.list подтверждает вставки 2015/2016:
	// DTAI меняется 1 Jul 2015 и 1 Jan 2017, но не 1 Jul 2016.
	// Таблица — отдельные исторические факты, а не алгоритм календаря.
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"1990-12-31T23:59:60Z", true},
		{"2015-06-30T23:59:60Z", true},
		{"2016-12-31T23:59:60Z", true},
		{"1990-06-30T23:59:60Z", false},
		{"2016-06-30T23:59:60Z", false},
		{"1990-06-30T23:59:59Z", true},
		{"2016-06-30T23:59:59Z", true},
		{"2016-12-31T23:59:59Z", true},
		{"2015-06-30T22:59:60Z", false},
		{"2015-06-30T23:58:60Z", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			lessonScalarsCheck(t, validateCreatedAt(tc.value, "/document/createdAt", lessonScalarsContext()), tc.valid, "/document/createdAt")
		})
	}
}

func TestLessonScalarsBase64(t *testing.T) {
	valid := []string{"", "AA==", "AAA=", "AAAA", "+/8=", "Zg==", "Zm8=", "Zm9v"}
	bytes := make([]byte, 256)
	for i := range bytes {
		bytes[i] = byte(i)
	}
	valid = append(valid, base64.StdEncoding.EncodeToString(bytes))
	invalid := []string{
		"A", "AA", "AAA", "A===", "AA=", "AA===", "AAAA=", "====", "=AAA", "A=AA",
		"AA==AAAA", "AA==AA==", "AB==", "AAB=", "//==", "///=", "-_8=", "éAAA",
		" AA==", "AA== ", "AA\t==", "AA\n==", "AA\r==", "AA==\r\n", "AA==\x00",
	}
	for _, group := range []struct {
		values []string
		valid  bool
	}{{valid, true}, {invalid, false}} {
		for _, value := range group.values {
			t.Run(value, func(t *testing.T) {
				got := validateBase64(value, "/content", lessonScalarsContext())
				lessonScalarsCheck(t, got, group.valid, "/content")
			})
		}
	}
}

func TestLessonScalarsEmbeddedName(t *testing.T) {
	valid := []string{"", "clip.mp4", "Без расширения", ".hidden", "a..b", "CON", "name:part", "a\x00b", strings.Repeat("я", 4096)}
	invalid := []string{"/clip.mp4", "dir/clip.mp4", "../clip.mp4", `dir\clip.mp4`, `\clip.mp4`, `..\clip.mp4`, `C:clip.mp4`, "C:", `C:\clip.mp4`, `\\server\share\clip.mp4`, `\\?\C:\clip.mp4`}
	for _, group := range []struct {
		values []string
		valid  bool
	}{{valid, true}, {invalid, false}} {
		for _, value := range group.values {
			t.Run(value, func(t *testing.T) {
				lessonScalarsCheck(t, validateEmbeddedName(value, "/fileName", lessonScalarsContext()), group.valid, "/fileName")
			})
		}
	}
}

func TestLessonScalarsEmbeddedFile(t *testing.T) {
	const pointer = "/document/pages/2/nodes/0/data/1/value"
	const secret = "DO_NOT_PRINT_THIS_BASE64_CONTENT=="
	for _, tc := range []struct {
		name, content string
		fields        []string
	}{
		{"clip.unknown", "AA==", nil}, {"", "", nil},
		{"dir/clip", "AA==", []string{"fileName"}},
		{"clip", secret, []string{"content"}},
		{`C:clip`, secret, []string{"fileName", "content"}},
	} {
		t.Run(tc.name+"/"+tc.content, func(t *testing.T) {
			file := EmbeddedFile{FileName: tc.name, MIMEType: "unregistered MIME\nvalue", Content: tc.content}
			if tc.name == "" {
				file.MIMEType = ""
			}
			before := file
			got := validateEmbeddedFile(file, pointer, lessonScalarsContext())
			if len(got) != len(tc.fields) {
				t.Fatalf("got %d errors, want %d", len(got), len(tc.fields))
			}
			for i, field := range tc.fields {
				lessonScalarsCheck(t, got[i], false, pointer+"/"+field)
				texts := []string{got[i].Reason, got[i].Error()}
				if got[i].Cause != nil {
					texts = append(texts, got[i].Cause.Error())
				}
				for _, detail := range got[i].Details {
					texts = append(texts, detail.Label, detail.Value)
				}
				for _, text := range texts {
					if strings.Contains(text, secret) {
						t.Error("embedded content disclosed in diagnostic")
					}
				}
			}
			if file != before {
				t.Error("embedded file changed")
			}
		})
	}
}
