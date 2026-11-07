package catalog

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func stateTestString(t *testing.T, value string) *string {
	t.Helper()
	return &value
}

func stateTestSnapshot(t *testing.T, value *State) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func stateTestFixture(t *testing.T) *State {
	t.Helper()
	return &State{
		CourseTitle: "Курс", UnitTitle: stateTestString(t, "Юнит"), Sectioned: true,
		Items: []Item{
			{
				Entry: manifest.Entry{
					ID: "Lesson+А", FileName: "l003.json", Order: stateTestString(t, "000123456789012345678901234567890"),
					Title: "Урок", Subtitle: stateTestString(t, "Подзаголовок"),
					LineCount: json.Number("123456789012345678901234567890"), CreatedAt: "2026-11-04T09:00:00Z", Version: json.Number("100000000000000000000"),
				},
				Section: stateTestString(t, "Studentbook"), RelativePath: "Юнит/A.json",
				Context: diagnostic.Context{Path: "архив.zip", FileName: "l003.json", LessonID: "Lesson+А", PageID: "lesson+а_001", Line: 7, Column: 3},
			},
			{Entry: manifest.Entry{ID: "БезУказателей", FileName: "l001.json", LineCount: "0", Version: "1"}},
			{Entry: manifest.Entry{ID: "ПустыеСтроки", FileName: "l002.json", Order: stateTestString(t, ""), Subtitle: stateTestString(t, "")}, Section: stateTestString(t, "")},
		},
		Sections: []SectionOrder{{Title: "Studentbook", Weight: stateTestString(t, "00020")}, {Title: "Без веса"}, {Title: "Пустой вес", Weight: stateTestString(t, "")}},
	}
}

func TestStateClonePreservesNilAndEmpty(t *testing.T) {
	if got := Clone(nil); got != nil {
		t.Fatalf("Clone(nil) = %#v, want nil", got)
	}
	for _, source := range []*State{{}, {Items: []Item{}, Sections: []SectionOrder{}, UnitTitle: stateTestString(t, "")}, stateTestFixture(t)} {
		before := stateTestSnapshot(t, source)
		got := Clone(source)
		if got == nil || got == source {
			t.Fatalf("Clone must return a distinct nonnil State: %p -> %p", source, got)
		}
		if stateTestSnapshot(t, got) != before || stateTestSnapshot(t, source) != before {
			t.Fatalf("Clone changed values or nil/empty distinctions: want %s, got %s", before, stateTestSnapshot(t, got))
		}
		if source.UnitTitle != nil && got.UnitTitle == source.UnitTitle {
			t.Fatal("Clone retained UnitTitle, including a present empty title")
		}
		for i, item := range source.Items {
			original, copied := item.Entry, got.Items[i].Entry
			original.Order, original.Subtitle = nil, nil
			copied.Order, copied.Subtitle = nil, nil
			if original != copied {
				t.Fatalf("Clone changed entry scalars or raw json.Number at index %d", i)
			}
		}
	}
}

func TestStateCloneOwnsEveryStringPointer(t *testing.T) {
	pointers := []struct {
		name string
		get  func(*State) *string
	}{
		{"unit", func(s *State) *string { return s.UnitTitle }},
		{"order", func(s *State) *string { return s.Items[0].Entry.Order }},
		{"subtitle", func(s *State) *string { return s.Items[0].Entry.Subtitle }},
		{"section", func(s *State) *string { return s.Items[0].Section }},
		{"empty order", func(s *State) *string { return s.Items[2].Entry.Order }},
		{"empty subtitle", func(s *State) *string { return s.Items[2].Entry.Subtitle }},
		{"empty section", func(s *State) *string { return s.Items[2].Section }},
		{"section weight", func(s *State) *string { return s.Sections[0].Weight }},
		{"empty section weight", func(s *State) *string { return s.Sections[2].Weight }},
	}
	for _, pointer := range pointers {
		t.Run(pointer.name, func(t *testing.T) {
			source := stateTestFixture(t)
			before := stateTestSnapshot(t, source)
			candidate := Clone(source)
			if candidate == nil || pointer.get(candidate) == nil {
				t.Fatal("Clone lost a present pointer")
			}
			if pointer.get(candidate) == pointer.get(source) {
				t.Fatal("Clone retained a source pointer")
			}
			*pointer.get(candidate) = "Изменённый кандидат"
			if stateTestSnapshot(t, source) != before {
				t.Fatal("Editing candidate changed source")
			}
			candidateBefore := stateTestSnapshot(t, candidate)
			*pointer.get(source) = "Изменённый источник"
			if stateTestSnapshot(t, candidate) != candidateBefore {
				t.Fatal("Editing source changed candidate")
			}
		})
	}
}

func TestStateCloneOwnsStateAndSliceElements(t *testing.T) {
	mutations := []struct {
		name string
		edit func(*State)
	}{
		{"state", func(s *State) { s.CourseTitle = "Другой курс"; s.Sectioned = false; s.UnitTitle = nil }},
		{"entry", func(s *State) { s.Items[0].Entry = manifest.Entry{ID: "Другой", FileName: "l999.json"} }},
		{"item", func(s *State) {
			s.Items[0].RelativePath = "другой.json"
			s.Items[0].Context = diagnostic.Context{Path: "другой.zip"}
			s.Items[0].Section = nil
		}},
		{"section", func(s *State) { s.Sections[0] = SectionOrder{Title: "Другая"} }},
		{"reorder", func(s *State) {
			s.Items[0], s.Items[1] = s.Items[1], s.Items[0]
			s.Sections[0], s.Sections[1] = s.Sections[1], s.Sections[0]
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			source := stateTestFixture(t)
			candidate := Clone(source)
			before := stateTestSnapshot(t, source)
			mutation.edit(candidate)
			if stateTestSnapshot(t, source) != before {
				t.Fatal("Candidate mutation changed source")
			}
			source = stateTestFixture(t)
			candidate = Clone(source)
			before = stateTestSnapshot(t, candidate)
			mutation.edit(source)
			if stateTestSnapshot(t, candidate) != before {
				t.Fatal("Source mutation changed candidate")
			}
		})
	}
	items := make([]Item, 1)
	sections := make([]SectionOrder, 1)
	source := &State{Items: items[:0], Sections: sections[:0]}
	candidate := Clone(source)
	candidate.Items = append(candidate.Items, Item{RelativePath: "новый.json"})
	candidate.Sections = append(candidate.Sections, SectionOrder{Title: "Новая"})
	if items[0].RelativePath != "" || sections[0].Title != "" {
		t.Fatal("Appending to empty candidate reused source backing arrays")
	}
}

func TestStateFindItemUsesExactUniqueFileName(t *testing.T) {
	for _, test := range []struct {
		name     string
		selector string
		want     int
	}{
		{"first", "l003.json", 0}, {"middle", "l001.json", 1}, {"last", "l002.json", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := stateTestFixture(t)
			// Even an ID equal to another record's file name must not participate.
			source.Items[0].Entry.ID = "l002.json"
			source.Items[1].Entry.ID = source.Items[2].Entry.ID
			before := stateTestSnapshot(t, source)
			index, problem := findItem(source, test.selector)
			if problem != nil || index != test.want {
				t.Fatalf("findItem(%q) = %d, %v; want %d, nil", test.selector, index, problem, test.want)
			}
			if stateTestSnapshot(t, source) != before {
				t.Fatal("Selection changed state")
			}
		})
	}
}

func TestStateFindItemRejectsMissingAndAmbiguousSelectors(t *testing.T) {
	for _, selector := range []string{"", "Lesson+А", "lesson+а", "L003.JSON", "l003", " l003.json", "l003.json ", "l003.json\n", "dir/l003.json", "l999.json"} {
		t.Run(selector, func(t *testing.T) { stateTestSelectionError(t, stateTestFixture(t), selector) })
	}
	t.Run("nil state", func(t *testing.T) { stateTestSelectionError(t, nil, "l001.json") })
	t.Run("nil items", func(t *testing.T) { stateTestSelectionError(t, &State{}, "l001.json") })
	t.Run("empty items", func(t *testing.T) { stateTestSelectionError(t, &State{Items: []Item{}}, "l001.json") })
	t.Run("duplicate across sections", func(t *testing.T) {
		source := stateTestFixture(t)
		source.Items[2].Entry.FileName = source.Items[0].Entry.FileName
		source.Items[2].Section = stateTestString(t, "Другая секция")
		stateTestSelectionError(t, source, "l003.json")
	})
}

func stateTestSelectionError(t *testing.T, source *State, selector string) {
	t.Helper()
	before := stateTestSnapshot(t, source)
	_, problem := findItem(source, selector)
	if problem == nil || problem.Code != CodeSelection {
		t.Fatalf("findItem(%q) error = %#v; want %s", selector, problem, CodeSelection)
	}
	russian := false
	for _, r := range problem.Reason {
		russian = russian || unicode.In(r, unicode.Cyrillic)
	}
	if !utf8.ValidString(problem.Reason) || !russian || strings.TrimSpace(problem.Reason) == "" {
		t.Fatalf("Selection must explain its failure in Russian: %q", problem.Reason)
	}
	if stateTestSnapshot(t, source) != before {
		t.Fatal("Failed selection changed state")
	}
	// Diagnostic storage may be edited by a caller without changing the model.
	problem.Context = diagnostic.Context{Path: "изменённый контекст"}
	for i := range problem.Related {
		problem.Related[i] = diagnostic.Context{FileName: "l999.json"}
	}
	for i := range problem.Details {
		problem.Details[i] = diagnostic.Detail{Value: "изменено"}
	}
	if stateTestSnapshot(t, source) != before {
		t.Fatal("Diagnostic mutation changed state")
	}
}
