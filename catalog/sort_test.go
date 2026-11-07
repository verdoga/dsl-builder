package catalog

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

func sortTestString(value string) *string { return &value }

func sortTestItem(number int, section, weight *string) Item {
	name := fmt.Sprintf("l%03d.json", number)
	return Item{Entry: manifest.Entry{ID: fmt.Sprintf("ID%d", number), FileName: name,
		Title: fmt.Sprintf("Урок %d", number), Subtitle: sortTestString("Подзаголовок"), Order: weight,
		LineCount: json.Number("25"), Version: json.Number("3"), CreatedAt: "2026-11-04T09:00:00Z"},
		Section: section, RelativePath: "вход/" + name,
		Context: diagnostic.Context{Path: "источник/" + name, FileName: name, LessonID: fmt.Sprintf("ID%d", number)}}
}

func sortTestNames(items []Item) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item.Entry.FileName
	}
	return result
}

func TestSortSectionWeightsRequireCompleteExactNames(t *testing.T) {
	value := &State{CourseTitle: "Курс", Sectioned: true, Sections: []SectionOrder{{Title: "A"}, {Title: "a"}}}
	value.Items = []Item{sortTestItem(1, sortTestString("A"), sortTestString("1")), sortTestItem(2, sortTestString("a"), sortTestString("1"))}
	for _, tc := range []struct {
		name    string
		weights []SectionOrder
		code    string
	}{
		{"empty", nil, CodeSectionWeights},
		{"missing", []SectionOrder{{Title: "A", Weight: sortTestString("1")}}, CodeSectionWeights},
		{"duplicate", []SectionOrder{{Title: "A", Weight: sortTestString("1")}, {Title: "A", Weight: sortTestString("2")}}, CodeSectionWeights},
		{"unknown", []SectionOrder{{Title: "A", Weight: sortTestString("1")}, {Title: "B", Weight: sortTestString("2")}}, CodeSectionWeights},
		{"extra", []SectionOrder{{Title: "A", Weight: sortTestString("1")}, {Title: "a", Weight: sortTestString("2")}, {Title: "B", Weight: sortTestString("3")}}, CodeSectionWeights},
		{"nil weight", []SectionOrder{{Title: "A"}, {Title: "a", Weight: sortTestString("2")}}, CodeSectionWeights},
		{"equal valid", []SectionOrder{{Title: "a", Weight: sortTestString("001")}, {Title: "A", Weight: sortTestString("1")}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal(value)
			weightsBefore, _ := json.Marshal(tc.weights)
			problems := validateSectionWeights(value, tc.weights)
			got, errors := SetSectionWeights(value, tc.weights)
			if tc.code == "" {
				if len(problems) != 0 || len(errors) != 0 || got == nil {
					t.Fatalf("valid set rejected: %v / %v", problems, errors)
				}
				if got.Sections[0].Title != "A" || got.Sections[1].Title != "a" {
					t.Fatalf("equal weights tie: %v", got.Sections)
				}
			} else {
				if got != nil {
					t.Fatal("invalid input returned candidate")
				}
				for _, errors := range [][]*diagnostic.Error{problems, errors} {
					found := false
					for _, problem := range errors {
						if problem.Code == tc.code {
							found = true
						}
					}
					if !found {
						t.Fatalf("missing %s: %v", tc.code, errors)
					}
				}
			}
			after, _ := json.Marshal(value)
			weightsAfter, _ := json.Marshal(tc.weights)
			if string(before) != string(after) || string(weightsBefore) != string(weightsAfter) {
				t.Fatal("input mutated")
			}
		})
	}
	for _, raw := range []string{"", "0", "000", "+1", "-1", "1.0", "1e2", " 1", "1\n", "１", "١"} {
		t.Run(fmt.Sprintf("invalid %q", raw), func(t *testing.T) {
			weights := []SectionOrder{{Title: "A", Weight: sortTestString(raw)}, {Title: "a", Weight: sortTestString(raw)}}
			got, problems := SetSectionWeights(value, weights)
			if got != nil {
				t.Fatal("invalid weight returned candidate")
			}
			found, invalidCount := false, 0
			for _, problem := range problems {
				if problem.Code == order.CodeInvalidWeight {
					invalidCount++
					for _, detail := range problem.Details {
						if detail.Label == "Исходный вес" && detail.Value == raw {
							found = true
						}
					}
				}
			}
			if !found || invalidCount != 2 {
				t.Fatalf("missing exact invalid raw %q: %v", raw, problems)
			}
		})
	}
}

func TestSortSectionTitlesUnicodeAndOriginalCaseTie(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		want        int
	}{
		{"Яблоко", "ёж", -1}, {"ЁЖ", "ёж", -1}, {"A", "a", -1}, {"alpha", "Бета", -1},
		{"İ", "i", 1}, {"é", "e\u0301", 1}, {"одинаково", "одинаково", 0},
	} {
		if got := compareSectionTitles(tc.left, tc.right); got != tc.want {
			t.Errorf("compare(%q,%q)=%d, want %d", tc.left, tc.right, got, tc.want)
		}
		if got := compareSectionTitles(tc.right, tc.left); got != -tc.want {
			t.Errorf("reverse compare=%d, want %d", got, -tc.want)
		}
	}
}

func TestSortSectionWeightsArbitraryPrecisionAndCopy(t *testing.T) {
	huge := strings.Repeat("9", 200)
	values := []SectionOrder{{Title: "huge", Weight: sortTestString(huge)}, {Title: "ёж", Weight: sortTestString("0002")},
		{Title: "ЁЖ", Weight: sortTestString("2")}, {Title: "ten", Weight: sortTestString("10")}, {Title: "one", Weight: sortTestString("0001")}}
	before, _ := json.Marshal(values)
	got := sortSectionsByWeight(values)
	var titles []string
	for _, section := range got {
		titles = append(titles, section.Title)
	}
	if !reflect.DeepEqual(titles, []string{"one", "ЁЖ", "ёж", "ten", "huge"}) {
		t.Fatalf("section order: %v", titles)
	}
	after, _ := json.Marshal(values)
	if string(before) != string(after) {
		t.Fatal("input reordered")
	}
	if *got[0].Weight != "0001" || *got[4].Weight != huge {
		t.Fatal("raw weights changed")
	}
	got[0].Title = "изменено"
	if values[0].Title != "huge" {
		t.Fatal("result aliases input slice")
	}
}

func TestSortItemsStableNumericPreviewAndInvalidTail(t *testing.T) {
	value := &State{CourseTitle: "Курс"}
	for i, raw := range []string{"0", "+1", "", "１", "-1", "1.0", "1e2", "1\n", " 1", "000"} {
		value.Items = append(value.Items, sortTestItem(i+1, nil, sortTestString(raw)))
	}
	value.Items = append(value.Items, sortTestItem(11, nil, nil), sortTestItem(12, nil, sortTestString(strings.Repeat("9", 200))),
		sortTestItem(13, nil, sortTestString("10")), sortTestItem(14, nil, sortTestString("002")),
		sortTestItem(15, nil, sortTestString("2")), sortTestItem(16, nil, sortTestString("0001")))
	before, _ := json.Marshal(value)
	got := sortItems(value)
	want := []string{"l016.json", "l014.json", "l015.json", "l013.json", "l012.json"}
	want = append(want, sortTestNames(value.Items[:11])...)
	if !reflect.DeepEqual(sortTestNames(got), want) {
		t.Fatalf("preview: %v, want %v", sortTestNames(got), want)
	}
	after, _ := json.Marshal(value)
	if string(before) != string(after) {
		t.Fatal("sortItems changed source")
	}
	for _, item := range got {
		if !reflect.DeepEqual(item, value.Items[sortTestIndex(value.Items, item.Entry.FileName)]) {
			t.Fatalf("item changed: %+v", item)
		}
	}
}

func sortTestIndex(items []Item, name string) int {
	for i := range items {
		if items[i].Entry.FileName == name {
			return i
		}
	}
	return -1
}

func TestSortItemsGroupsFollowRegistryAndUnassignedComeFirst(t *testing.T) {
	value := &State{CourseTitle: "Курс", Sectioned: true, Sections: []SectionOrder{{Title: "Z"}, {Title: "a"}, {Title: "A"}}}
	value.Items = []Item{sortTestItem(1, sortTestString("A"), sortTestString("1")), sortTestItem(2, sortTestString("a"), sortTestString("10")),
		sortTestItem(3, sortTestString("Z"), sortTestString("2")), sortTestItem(4, nil, sortTestString("3")),
		sortTestItem(5, sortTestString("a"), sortTestString("1")), sortTestItem(6, nil, nil), sortTestItem(7, sortTestString("Z"), sortTestString("1"))}
	got := sortItems(value)
	want := []string{"l004.json", "l006.json", "l007.json", "l003.json", "l005.json", "l002.json", "l001.json"}
	if !reflect.DeepEqual(sortTestNames(got), want) {
		t.Fatalf("grouped: %v", sortTestNames(got))
	}
}

func TestSortRefreshNilAndEmpty(t *testing.T) {
	got, problems := Refresh(nil)
	if got != nil || len(problems) != 1 || problems[0].Code != CodeState {
		t.Fatalf("Refresh(nil): %v, %v", got, problems)
	}
	got, problems = Refresh(&State{CourseTitle: "Курс"})
	if got == nil || len(problems) != 1 || problems[0].Code != CodeEmpty {
		t.Fatalf("Refresh(empty): %v, %v", got, problems)
	}
}

func TestSortRefreshReportsWholeCandidateAndOwnsPointers(t *testing.T) {
	value := &State{CourseTitle: "Курс", UnitTitle: sortTestString("Юнит"), Sectioned: true,
		Sections: []SectionOrder{{Title: "Z", Weight: sortTestString("20")}, {Title: "A", Weight: sortTestString("10")}}}
	value.Items = []Item{sortTestItem(1, sortTestString("Z"), sortTestString("010")), sortTestItem(2, sortTestString("A"), nil),
		sortTestItem(3, sortTestString("Z"), sortTestString("10")), sortTestItem(4, sortTestString("A"), sortTestString("x")), sortTestItem(5, nil, nil)}
	before, _ := json.Marshal(value)
	got, problems := Refresh(value)
	if got == nil {
		t.Fatalf("no editable candidate: %v", problems)
	}
	if !reflect.DeepEqual(got.Sections, value.Sections) {
		t.Fatal("Refresh changed chosen section order")
	}
	if !reflect.DeepEqual(problems, Validate(got)) {
		t.Fatalf("Refresh hid or changed whole-state errors: %v", problems)
	}
	counts := map[string]int{}
	for _, problem := range problems {
		counts[problem.Code]++
	}
	if counts[CodeMissingWeight] != 2 || counts[order.CodeInvalidWeight] != 1 || counts[CodeWeightConflict] != 1 || counts[CodeUnassigned] == 0 {
		t.Fatalf("missing independent block: %v", counts)
	}
	after, _ := json.Marshal(value)
	if string(before) != string(after) {
		t.Fatal("Refresh mutated source")
	}
	*got.UnitTitle = "другой"
	*got.Sections[0].Weight = "999"
	for i := range got.Items {
		if got.Items[i].Section != nil {
			*got.Items[i].Section = "новая"
		}
		if got.Items[i].Entry.Order != nil {
			*got.Items[i].Entry.Order = "888"
		}
		*got.Items[i].Entry.Subtitle = "новый"
		got.Items[i].Entry.Title = "новое"
		got.Items[i].Context.Path = "другой путь"
	}
	after, _ = json.Marshal(value)
	if string(before) != string(after) {
		t.Fatal("candidate aliases original slices or pointers")
	}
}

func TestSortSetSectionWeightsRefreshesAllItemsAndOwnsInput(t *testing.T) {
	value := &State{CourseTitle: "Курс", Sectioned: true, Sections: []SectionOrder{{Title: "A"}, {Title: "Z"}}}
	value.Items = []Item{sortTestItem(1, sortTestString("A"), sortTestString("2")), sortTestItem(2, sortTestString("Z"), nil),
		sortTestItem(3, sortTestString("A"), sortTestString("1"))}
	weights := []SectionOrder{{Title: "A", Weight: sortTestString("0002")}, {Title: "Z", Weight: sortTestString("1")}}
	before, _ := json.Marshal(value)
	got, problems := SetSectionWeights(value, weights)
	if got == nil {
		t.Fatalf("valid section weights lost candidate: %v", problems)
	}
	if len(problems) != 1 || problems[0].Code != CodeMissingWeight || problems[0].Context != value.Items[1].Context {
		t.Fatalf("unrelated item's missing weight hidden: %v", problems)
	}
	if !reflect.DeepEqual(sortTestNames(got.Items), []string{"l002.json", "l003.json", "l001.json"}) {
		t.Fatalf("stale preview: %v", sortTestNames(got.Items))
	}
	if got.Sections[0].Title != "Z" || got.Sections[1].Title != "A" || *got.Sections[1].Weight != "0002" {
		t.Fatal("wrong candidate section weights")
	}
	*weights[0].Weight = "300"
	weights[1].Title = "другое"
	if *got.Sections[1].Weight != "0002" || got.Sections[0].Title != "Z" {
		t.Fatal("candidate aliases supplied weights")
	}
	*got.Sections[0].Weight = "400"
	*got.Items[1].Entry.Order = "500"
	after, _ := json.Marshal(value)
	if string(before) != string(after) {
		t.Fatal("candidate aliases old state")
	}
}
