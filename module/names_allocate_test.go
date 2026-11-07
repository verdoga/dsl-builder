package module

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestNamesMaximumUsesNumericValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []*File
		want  string
	}{
		{"nil", nil, "0"},
		{"empty", []*File{}, "0"},
		{"padding", []*File{{name: "l009.json"}, {name: "l010.json"}, {name: "l002.json"}}, "10"},
		{"width", []*File{{name: "l999.json"}, {name: "l1000.json"}, {name: "l099.json"}}, "1000"},
		{"uint64", []*File{{name: "l18446744073709551615.json"}, {name: "l18446744073709551616.json"}}, "18446744073709551616"},
		{"long", []*File{{name: "l" + strings.Repeat("9", 179) + ".json"}, {name: "l1" + strings.Repeat("0", 179) + ".json"}}, "1" + strings.Repeat("0", 179)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if problems := validateFileNames(tc.files, diagnostic.Context{}); len(problems) != 0 {
				t.Fatalf("предусловие maxFileNumber нарушено: %v", problems)
			}
			before := slices.Clone(tc.files)
			if got := maxFileNumber(tc.files); got != tc.want {
				t.Fatalf("maxFileNumber=%q, want %q", got, tc.want)
			}
			if !slices.Equal(tc.files, before) {
				t.Fatal("maxFileNumber изменил порядок файлов")
			}
		})
	}
}

func TestNamesAllocateConsecutiveNumbers(t *testing.T) {
	for _, tc := range []struct {
		maximum string
		want    []string
	}{
		{"0", []string{"l001.json", "l002.json", "l003.json"}},
		{"8", []string{"l009.json", "l010.json", "l011.json"}},
		{"998", []string{"l999.json", "l1000.json", "l1001.json"}},
		{"9998", []string{"l9999.json", "l10000.json", "l10001.json"}},
		{"18446744073709551615", []string{"l18446744073709551616.json", "l18446744073709551617.json"}},
		{strings.Repeat("9", 180), []string{"l1" + strings.Repeat("0", 180) + ".json", "l1" + strings.Repeat("0", 179) + "1.json"}},
	} {
		t.Run(tc.maximum, func(t *testing.T) {
			got, problem := allocateNames(tc.maximum, len(tc.want), diagnostic.Context{Path: "назначение"})
			if problem != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("allocateNames=%v, problem=%v, want %v", got, problem, tc.want)
			}
		})
	}
}

func TestNamesAllocationCountIsListLength(t *testing.T) {
	const count = 1001
	for _, assign := range []bool{false, true} {
		var names []string
		if assign {
			var problems []*diagnostic.Error
			names, problems = AssignFileNames(nil, count, diagnostic.Context{})
			if len(problems) != 0 {
				t.Fatal(problems)
			}
		} else {
			var problem *diagnostic.Error
			names, problem = allocateNames("0", count, diagnostic.Context{})
			if problem != nil {
				t.Fatal(problem)
			}
		}
		if len(names) != count {
			t.Fatalf("len(names)=%d, want %d", len(names), count)
		}
		for index, got := range names {
			if want := fmt.Sprintf("l%03d.json", index+1); got != want {
				t.Fatalf("index %d: got %q, want %q", index, got, want)
			}
		}
	}
}
