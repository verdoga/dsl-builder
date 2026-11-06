package order_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/order"
)

func TestIntegrationWeightsFromExampleJSON(t *testing.T) {
	type entry struct {
		ID       string  `json:"id"`
		FileName string  `json:"fileName"`
		Order    *string `json:"order"`
	}
	type section struct {
		Lessons []entry `json:"lessons"`
	}
	type level struct {
		Lessons  []entry   `json:"lessons"`
		Sections []section `json:"sections"`
	}
	checked := 0
	for _, name := range []string{"Manifest flat example.json", "Manifest unit example.json"} {
		t.Run(name, func(t *testing.T) {
			var fixture struct {
				Course struct {
					level
					Unit *level `json:"unit"`
				} `json:"course"`
			}
			integrationDecodeJSON(t, "../"+name, &fixture)
			groups := [][]entry{fixture.Course.Lessons}
			for _, section := range fixture.Course.Sections {
				groups = append(groups, section.Lessons)
			}
			if fixture.Course.Unit != nil {
				groups = append(groups, fixture.Course.Unit.Lessons)
				for _, section := range fixture.Course.Unit.Sections {
					groups = append(groups, section.Lessons)
				}
			}
			for _, group := range groups {
				var previous *string
				for _, lesson := range group {
					// null остаётся у владельца модели и не превращается в пустой raw.
					if lesson.Order == nil {
						continue
					}
					before := *lesson.Order
					ctx := diagnostic.Context{Path: name, FileName: lesson.FileName, LessonID: lesson.ID}
					if err := order.Validate(*lesson.Order, ctx); err != nil {
						t.Fatalf("Validate(%q): %v", before, err)
					}
					if previous != nil && order.Compare(*previous, *lesson.Order) != -1 {
						t.Errorf("Compare(%q, %q): want -1 for example sequence", *previous, *lesson.Order)
					}
					if *lesson.Order != before {
						t.Errorf("example spelling changed: %q -> %q", before, *lesson.Order)
					}
					previous = lesson.Order
					checked++
				}
			}
		})
	}
	for n := 1; n <= 7; n++ {
		name := fmt.Sprintf("../golden/%d. parsed.json", n)
		t.Run(name, func(t *testing.T) {
			var fixture struct {
				Document struct {
					Metadata struct {
						Order *string `json:"order"`
					} `json:"metadata"`
				} `json:"document"`
			}
			integrationDecodeJSON(t, name, &fixture)
			if raw := fixture.Document.Metadata.Order; raw != nil {
				before := *raw
				if err := order.Validate(*raw, diagnostic.Context{Path: name}); err != nil {
					t.Fatalf("Validate(%q): %v", *raw, err)
				}
				if got := order.Compare(*raw, before); got != 0 || *raw != before {
					t.Errorf("weight %q changed or self-comparison = %d", before, got)
				}
				checked++
			}
		})
	}
	if checked == 0 {
		t.Fatal("example JSON supplied no string weights to check")
	}
}

func TestIntegrationValidatedScenarioWeightsPreserveSpelling(t *testing.T) {
	for _, tc := range []struct {
		name        string
		left, right string
		want        int
	}{
		{"numeric_duplicate_TZ16_5", "10", "010", 0},
		{"synthesized_TZ18_1_first_pair", "1", "2", -1},
		{"synthesized_TZ18_1_next_pair", "2", "3", -1},
		{"entered_section_weights_TZ16_4", "20", "10", 1},
		{"entered_lesson_weight_TZ18_5", "00030", "20", 1},
		{"entered_weight_arbitrary_precision", "000" + strings.Repeat("9", 200), "1" + strings.Repeat("0", 200), -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := json.Marshal([2]string{tc.left, tc.right})
			if err != nil {
				t.Fatal(err)
			}
			originalLeft, originalRight := tc.left, tc.right
			for _, raw := range []string{tc.left, tc.right} {
				if err := order.Validate(raw, diagnostic.Context{}); err != nil {
					t.Fatalf("Validate(%q): %v", raw, err)
				}
			}
			if got := order.Compare(tc.left, tc.right); got != tc.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
			}
			if got := order.Compare(tc.right, tc.left); got != -tc.want {
				t.Errorf("reverse comparison = %d, want %d", got, -tc.want)
			}
			after, err := json.Marshal([2]string{tc.left, tc.right})
			if err != nil {
				t.Fatal(err)
			}
			if tc.left != originalLeft || tc.right != originalRight || string(after) != string(before) {
				t.Fatal("Validate/Compare changed original spelling")
			}
		})
	}
}

func TestIntegrationInvalidWeightDiagnostic(t *testing.T) {
	if order.CodeInvalidWeight != "order.invalid_weight" {
		t.Fatalf("CodeInvalidWeight = %q", order.CodeInvalidWeight)
	}
	contexts := []diagnostic.Context{
		{},
		{Path: `C:\Курс\вход\A.json`, LessonID: "00Ab", Line: 12},
		{Path: `C:\Курс\модуль.zip`, FileName: "l002.json", LessonID: "00Ab", PageID: "00Ab.001", Line: 12, Column: 7},
	}
	for _, raw := range []string{"", "000", " 010\r\n", "+10", "1e2", "1.0", "١٠", "10\x00"} {
		for index, ctx := range contexts {
			t.Run(fmt.Sprintf("%q/context%d", raw, index), func(t *testing.T) {
				originalRaw, originalContext := raw, ctx
				got := order.Validate(raw, ctx)
				if got == nil {
					t.Fatal("invalid weight accepted")
				}
				var err error = got
				var structured *diagnostic.Error
				if !errors.As(err, &structured) || structured != got {
					t.Fatal("diagnostic.Error unavailable through errors.As")
				}
				if got.Code != "order.invalid_weight" {
					t.Errorf("Code = %q", got.Code)
				}
				if !strings.ContainsFunc(got.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
					t.Errorf("Reason must explain failure in Russian: %q", got.Reason)
				}
				if got.Context != originalContext {
					t.Errorf("Context = %#v, want %#v", got.Context, originalContext)
				}
				matches := 0
				for _, detail := range got.Details {
					if detail.Label == "Исходный вес" {
						matches++
						if detail.Value != originalRaw {
							t.Errorf("raw detail = %q, want %q", detail.Value, originalRaw)
						}
					}
				}
				if matches != 1 {
					t.Errorf("got %d original-weight details, want 1", matches)
				}
				if got.Cause != nil || errors.Unwrap(err) != nil {
					t.Errorf("syntax error has technical cause: %v", got.Cause)
				}
				if raw != originalRaw || ctx != originalContext {
					t.Fatal("Validate changed caller input")
				}
			})
		}
	}
}

func integrationDecodeJSON(t *testing.T, path string, destination any) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, destination); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
