package revision

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func TestMetadataNextRevisionExactIncrement(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{"initial", "1", "2"},
		{"decimal", "5.000", "6"},
		{"exponent", "5e0", "6"},
		{"fraction and exponent", "0.005E+3", "6"},
		{"negative exponent", "120000e-4", "13"},
		{"above float precision", "9007199254740993", "9007199254740994"},
		{"above uint64", "18446744073709551615", "18446744073709551616"},
		{"long carry", strings.Repeat("9", 256), "1" + strings.Repeat("0", 256)},
		{"positive exponent", "12345678901234567890123456789e7", "123456789012345678901234567890000001"},
		{"cancelled exponent", "7" + strings.Repeat("0", 4096) + "e-4096", "8"},
		{"long exponent spelling", "6e+" + strings.Repeat("0", 128) + "2", "601"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := json.Number(tt.input)
			metadataTestValidateNumber(t, input, true)
			if got := nextRevision(input); got != json.Number(tt.want) {
				t.Fatalf("nextRevision = %q; want %q", got, tt.want)
			}
			if string(input) != tt.input {
				t.Fatal("input number changed")
			}
		})
	}
}

func TestMetadataIntegerValueExactForms(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{"plain", "9223372036854775808", "9223372036854775808"},
		{"fraction", "12345678901234567890123456789.000", "12345678901234567890123456789"},
		{"fraction scaled", "0.000500E+6", "500"},
		{"coefficient scaled down", "1234500e-2", "12345"},
		{"positive scale", "1.25e3", "1250"},
		{"negative zero", "-0.00", "0"},
		{"zero huge positive exponent", "0e184467440737095516160000", "0"},
		{"zero huge negative exponent", "0.000e-184467440737095516160000", "0"},
		{"negative zero huge exponent", "-0e+" + strings.Repeat("9", 128), "0"},
		{"cancelled exponent", "42" + strings.Repeat("0", 4096) + "e-4096", "42"},
		{"fraction cancelled exponent", "42." + strings.Repeat("0", 4096) + "e0", "42"},
		{"long exponent spelling", "5e-" + strings.Repeat("0", 128) + "0", "5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := json.Number(tt.input)
			// Ноль допустим в LineCount: это тоже предусловие integerValue.
			metadataTestValidateNumber(t, input, false)
			got := integerValue(input)
			if got == nil || got.String() != tt.want {
				t.Fatalf("integerValue = %v; want %s", got, tt.want)
			}
			if string(input) != tt.input {
				t.Fatal("input number changed")
			}
		})
	}
}

func TestMetadataVersionTimeUTCSecondsAndYears(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"UTC truncation", time.Date(2024, 2, 29, 23, 59, 59, 999999999, time.UTC), "2024-02-29T23:59:59Z"},
		{"previous UTC year", time.Date(2025, 1, 1, 0, 15, 1, 123, time.FixedZone("east", 5*3600+1800)), "2024-12-31T18:45:01Z"},
		{"next UTC year", time.Date(2024, 12, 31, 23, 45, 2, 999999999, time.FixedZone("west", -3*3600-1800)), "2025-01-01T03:15:02Z"},
		{"zero time", time.Time{}, "0001-01-01T00:00:00Z"},
		{"year zero", time.Date(0, 1, 2, 3, 4, 5, 0, time.UTC), "0000-01-02T03:04:05Z"},
		{"last four digit year", time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), "9999-12-31T23:59:59Z"},
		{"five digit year left for validation", time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), "10000-01-01T00:00:00Z"},
		{"negative year left for validation", time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC), "-0001-01-01T00:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.at
			if got := versionTime(tt.at); got != tt.want {
				t.Fatalf("versionTime = %q; want %q", got, tt.want)
			}
			if tt.at != before {
				t.Fatal("supplied time changed")
			}
		})
	}
}

func TestMetadataStampInitialPreservesCandidateFields(t *testing.T) {
	for _, count := range []json.Number{"0", "123456789012345678901234567890", "1200.00e-2", "0e-184467440737095516160000"} {
		t.Run(string(count), func(t *testing.T) {
			metadataTestValidateNumber(t, count, false)
			result := metadataTestDocument(t)
			result.LineCount = count
			want := metadataTestJSON(t, result)
			subtitle, page := result.Subtitle, &result.Pages[0]
			at := time.Date(2025, 1, 1, 1, 2, 3, 999999999, time.FixedZone("east", 3600))
			stampInitial(&result, at)
			if result.Version != "1" || result.CreatedAt != "2025-01-01T00:02:03Z" || result.LineCount != count {
				t.Fatalf("incorrect initial metadata: version=%q date=%q count=%q", result.Version, result.CreatedAt, result.LineCount)
			}
			if result.Subtitle != subtitle || &result.Pages[0] != page {
				t.Fatal("unrelated pointer or pages backing array replaced")
			}
			result.Version, result.CreatedAt = "99.0", "2001-02-03T04:05:06Z"
			if !bytes.Equal(metadataTestJSON(t, result), want) {
				t.Fatal("stampInitial changed fields other than Version and CreatedAt")
			}
		})
	}
}

func TestMetadataStampUpdatePreservesContentAndExactCount(t *testing.T) {
	for _, count := range []json.Number{"0", "90071992547409931234567890", "1.200e3", "0e+184467440737095516160000"} {
		t.Run(string(count), func(t *testing.T) {
			metadataTestValidateNumber(t, count, false)
			result := metadataTestDocument(t)
			want := metadataTestJSON(t, result)
			subtitle, page := result.Subtitle, &result.Pages[0]
			revision := json.Number("100000000000000000000000000000001")
			at := time.Date(2024, 2, 29, 22, 3, 4, 987654321, time.FixedZone("west", -2*3600))
			stampUpdate(&result, revision, count, at)
			if result.Version != revision || result.CreatedAt != "2024-03-01T00:03:04Z" || result.LineCount != count {
				t.Fatalf("incorrect update metadata: version=%q date=%q count=%q", result.Version, result.CreatedAt, result.LineCount)
			}
			if result.Subtitle != subtitle || &result.Pages[0] != page {
				t.Fatal("unrelated pointer or pages backing array replaced")
			}
			result.Version, result.CreatedAt, result.LineCount = "99.0", "2001-02-03T04:05:06Z", "37"
			if !bytes.Equal(metadataTestJSON(t, result), want) {
				t.Fatal("stampUpdate changed fields other than Version, CreatedAt and LineCount")
			}
		})
	}
}

func metadataTestDocument(t *testing.T) lesson.Document {
	t.Helper()
	subtitle, pageTitle, speaking, role := " Подзаголовок ", "Страница", false, lesson.RoleBody
	return lesson.Document{
		ID: "EFX7", Version: "99.0", Title: "Название\nурока", Subtitle: &subtitle,
		LineCount: "37", CreatedAt: "2001-02-03T04:05:06Z",
		Pages: []lesson.Page{{ID: "efx7-p1000", Title: &pageTitle, Nodes: []lesson.Node{{
			ID: "efx7-n1001", Type: lesson.NodeTask, Flags: lesson.NodeFlags{Speaking: &speaking},
			Data:  []lesson.Data{{Type: lesson.DataText, Role: &role, Value: "одна\nдве\nтри", Answers: []string{"ответ"}, Hint: "подсказка"}},
			Nodes: []lesson.Node{{ID: "efx7-n1002", Type: lesson.NodeText, Data: []lesson.Data{}, Nodes: []lesson.Node{}}},
		}}}},
	}
}

func metadataTestValidateNumber(t *testing.T, number json.Number, version bool) {
	t.Helper()
	document := metadataTestDocument(t)
	if version {
		document.Version = number
	} else {
		document.LineCount = number
	}
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: document}
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("fixture violates lesson.Validate precondition: %v", problems)
	}
}

func metadataTestJSON(t *testing.T, document lesson.Document) []byte {
	t.Helper()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
