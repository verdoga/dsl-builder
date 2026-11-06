package identity_test

import (
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func TestNodeParseCanonical(t *testing.T) {
	huge := "184467440737095516160000000000000000001"
	long := strings.Repeat("9", 240)
	cases := []struct {
		value, prefix, number, revision string
	}{
		{"efx7-n001", "efx7", "1", ""},
		{"efx7-n009-v01", "efx7", "9", "1"},
		{"efx7-n010-v09", "efx7", "10", "9"},
		{"efx7-n099-v10", "efx7", "99", "10"},
		{"efx7-n100-v99", "efx7", "100", "99"},
		{"efx7-n999-v100", "efx7", "999", "100"},
		{"efx7-n1000", "efx7", "1000", ""},
		{"efx7-n" + huge + "-v" + long, "efx7", huge, long},
		{"x-n" + long + "-v" + huge, "x", long, huge},
		{"a:b-n001", "a:b", "1", ""},
		{"lesson+1-n001-v06", "lesson+1", "1", "6"},
		{"-n-n001", "-n", "1", ""},
		{"-v-n001-v02", "-v", "1", "2"},
		{"a-n001-n002", "a-n001", "2", ""},
		{"a-n001-v02-n003-v04", "a-n001-v02", "3", "4"},
		{"a-v99-nx-vx-n003", "a-v99-nx-vx", "3", ""},
		{"1/урок🙂-n001-v01", "1/урок🙂", "1", "1"},
		{"a\u0085\u200b-n001", "a\u0085\u200b", "1", ""},
		{"e\u0301-n001", "e\u0301", "1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, err := identity.ParseNodeID(tc.value)
			if err != nil {
				t.Fatalf("ParseNodeID(%q): %v", tc.value, err)
			}
			want := identity.NodeIDParts{Prefix: tc.prefix, Number: tc.number, Revision: tc.revision}
			if got != want {
				t.Errorf("parts = %#v, want %#v", got, want)
			}
		})
	}
}

func TestNodeParseRejectsMalformed(t *testing.T) {
	cases := []string{
		"", "lesson", "-n001", "lesson-p001", "lesson-N001", "lesson-n",
		"lesson-n0", "lesson-n00", "lesson-n000", "lesson-n1", "lesson-n01",
		"lesson-n0001", "lesson-n0100", "lesson-n0999", "lesson-n01000",
		"lesson-n+001", "lesson-n-001", "lesson-n００１", "lesson-n٠٠١",
		"lesson-n001x", "lesson-n001.0", "lesson-n001-v", "lesson-n001-V02",
		"lesson-n001-v0", "lesson-n001-v00", "lesson-n001-v1", "lesson-n001-v001",
		"lesson-n001-v010", "lesson-n001-v099", "lesson-n001-v0100",
		"lesson-n001-v+02", "lesson-n001-v-02", "lesson-n001-v０２",
		"lesson-n001-v02x", "lesson-n001-v02-v03", "lesson-n001-v02-n",
		"Lesson-n001", "УРОК-n001-v02", "bad id-n001", " lesson-n001",
		"lesson-n001 ", "lesson-n001\n", "lesson-n001-v02\r\n",
		"bad\tkey-n001", "bad\x00key-n001", "bad\x7fkey-n001",
		"bad\u00a0key-n001", "bad\ufeffkey-n001", "bad\u2028key-n001",
		"bad\xffkey-n001", "lesson-n001-v02\xff", "lesson-n001\x00",
	}
	for _, value := range cases {
		t.Run(value, func(t *testing.T) {
			got, err := identity.ParseNodeID(value)
			if got != (identity.NodeIDParts{}) {
				t.Errorf("error result = %#v, want zero parts", got)
			}
			nodeRequireError(t, err, identity.CodeInvalidNodeID, value)
		})
	}
}

func TestNodeFormatCanonical(t *testing.T) {
	huge := "184467440737095516160000000000000000001"
	long := strings.Repeat("7", 240)
	cases := []struct {
		lesson, number, revision, want string
	}{
		{"EFX7", "1", "", "efx7-n001"},
		{"EFX7", "0001", "0001", "efx7-n001-v01"},
		{"EFX7", "9", "9", "efx7-n009-v09"},
		{"EFX7", "10", "10", "efx7-n010-v10"},
		{"EFX7", "99", "99", "efx7-n099-v99"},
		{"EFX7", "100", "100", "efx7-n100-v100"},
		{"EFX7", "0999", "0099", "efx7-n999-v99"},
		{"EFX7", "01000", "00100", "efx7-n1000-v100"},
		{"A:B", "1", "", "a:b-n001"},
		{"LESSON+1", "1", "6", "lesson+1-n001-v06"},
		{"A-n001-v02", "3", "4", "a-n001-v02-n003-v04"},
		{"УРОК/🙂", "1", "", "урок/🙂-n001"},
		{"E\u0301", "1", "", "e\u0301-n001"},
		{"A\u0085\u200b", "1", "", "a\u0085\u200b-n001"},
		{"X", "000" + huge, "000" + long, "x-n" + huge + "-v" + long},
		{"X", long, huge, "x-n" + long + "-v" + huge},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			got, err := identity.FormatNodeID(tc.lesson, tc.number, tc.revision)
			if err != nil || got != tc.want {
				t.Fatalf("FormatNodeID(%q, %q, %q) = %q, %v; want %q, nil", tc.lesson, tc.number, tc.revision, got, err, tc.want)
			}
		})
	}
}

func TestNodeFormatRejectsInvalidLessonFirst(t *testing.T) {
	for _, lesson := range []string{"", "bad id", "bad\x00id", "bad\xffid", "bad\ufeffid"} {
		t.Run(lesson, func(t *testing.T) {
			got, err := identity.FormatNodeID(lesson, "", "0")
			if got != "" {
				t.Errorf("error result = %q, want empty", got)
			}
			nodeRequireError(t, err, identity.CodeInvalidLessonID, lesson)
		})
	}
}

func TestNodeFormatRejectsInvalidNumbers(t *testing.T) {
	invalid := []string{"", "0", "000", "+1", "-1", " 1", "1 ", "1\n", "1.0", "1e2", "１", "١", "1\x00", "1\xff"}
	for _, value := range invalid {
		t.Run("number/"+value, func(t *testing.T) {
			got, err := identity.FormatNodeID("Lesson", value, "2")
			if got != "" {
				t.Errorf("error result = %q, want empty", got)
			}
			nodeRequireError(t, err, identity.CodeInvalidNumber, value)
		})
		if value == "" {
			continue // Пустая ревизия допустима и проверена в успешных случаях.
		}
		t.Run("revision/"+value, func(t *testing.T) {
			got, err := identity.FormatNodeID("Lesson", "1", value)
			if got != "" {
				t.Errorf("error result = %q, want empty", got)
			}
			nodeRequireError(t, err, identity.CodeInvalidNumber, value)
		})
	}
}

func TestNodeFormatDistinguishesNumberAndRevisionErrors(t *testing.T) {
	_, numberErr := identity.FormatNodeID("Lesson", "0", "2")
	_, revisionErr := identity.FormatNodeID("Lesson", "1", "0")
	nodeRequireError(t, numberErr, identity.CodeInvalidNumber, "0")
	nodeRequireError(t, revisionErr, identity.CodeInvalidNumber, "0")
	sameDetails := len(numberErr.Details) == len(revisionErr.Details)
	if sameDetails {
		for i, detail := range numberErr.Details {
			if detail != revisionErr.Details[i] {
				sameDetails = false
			}
		}
	}
	if sameDetails {
		t.Error("Details do not distinguish an invalid base number from an invalid revision")
	}
}

func TestNodeReformatSelectedRevision(t *testing.T) {
	for _, original := range []string{"a:b-n035", "a:b-n035-v03", "a-n001-v02-n035-v01"} {
		t.Run(original, func(t *testing.T) {
			parts, err := identity.ParseNodeID(original)
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct{ revision, suffix string }{{"6", "-v06"}, {"100", "-v100"}, {"", ""}} {
				got, formatErr := identity.FormatNodeID(parts.Prefix, parts.Number, tc.revision)
				want := parts.Prefix + "-n035" + tc.suffix
				if formatErr != nil || got != want {
					t.Errorf("selected revision %q = %q, %v; want %q, nil", tc.revision, got, formatErr, want)
				}
			}
		})
	}
}

func nodeRequireError(t *testing.T, err *diagnostic.Error, code, value string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected diagnostic error")
	}
	if err.Code != code {
		t.Errorf("Code = %q, want %q", err.Code, code)
	}
	if strings.TrimSpace(err.Reason) == "" || !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("Reason must contain a Russian explanation: %q", err.Reason)
	}
	if err.Context != (diagnostic.Context{}) {
		t.Errorf("Context = %#v, want empty", err.Context)
	}
	for _, detail := range err.Details {
		if detail.Value == value {
			return
		}
	}
	t.Errorf("Details = %#v, must retain exact erroneous value %q", err.Details, value)
}
