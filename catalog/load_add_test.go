package catalog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

func TestLoadAddInvalidNewSetIsAtomic(t *testing.T) {
	old := &State{CourseTitle: "Курс", Items: []Item{loadAddTestItem("Old", "1", "")}}
	for _, tc := range []struct {
		kind, code string
		invalid    []string
	}{
		{"section", manifest.CodeTitle, []string{"", " ", " A", "A ", "A  B", "A\tB", "A\rB", "A\nB", "\xff"}},
		{"weight", order.CodeInvalidWeight, []string{"", "0", "000", "-1", "+1", "1.0", "1e2", " 1", "1 ", "1\n", "١", "１", "\xff"}},
	} {
		for _, raw := range tc.invalid {
			for _, index := range []int{0, 2} {
				items := []Item{loadAddTestItem("A", "2", ""), loadAddTestItem("B", "3", ""), loadAddTestItem("C", "4", "")}
				if tc.kind == "section" {
					items[index].Section = loadAddTestString(raw)
				} else {
					items[index].Entry.Order = loadAddTestString(raw)
				}
				before, inputs := loadAddTestJSON(t, old), loadAddTestJSON(t, items)
				got, errs := Add(old, items)
				loadAddTestFailIf(t, got != nil, "принят неверный %s %q в позиции %d", tc.kind, raw, index)
				problem := loadAddTestCode(t, errs, tc.code)
				loadAddTestFailIf(t, problem.Context != items[index].Context, "потерян контекст: %+v", problem)
				loadAddTestFailIf(t, tc.kind == "weight" && !strings.Contains(loadAddTestJSON(t, problem.Details), loadAddTestJSON(t, raw)), "потерян raw %q", raw)
				loadAddTestFailIf(t, loadAddTestJSON(t, old) != before || loadAddTestJSON(t, items) != inputs, "ошибка изменила вход")
			}
		}
	}
	for _, items := range [][]Item{nil, {}} {
		got, errs := Add(old, items)
		loadAddTestFailIf(t, got != nil, "пустой Add принят")
		loadAddTestCode(t, errs, CodeEmpty)
		got, err := appendItems(old, items)
		loadAddTestFailIf(t, got != nil || err == nil || err.Code != CodeEmpty, "appendItems empty: %v %v", got, err)
	}
	got, errs := Add(nil, []Item{loadAddTestItem("A", "1", "")})
	loadAddTestFailIf(t, got != nil, "nil State принят")
	loadAddTestCode(t, errs, CodeState)
}

func TestLoadAddPreservesOldMetadataAndAppendsExactSections(t *testing.T) {
	old := &State{CourseTitle: " Курс ", UnitTitle: loadAddTestString(" Юнит "), Sectioned: true, Items: []Item{loadAddTestItem("Old", "0002", "Z"), loadAddTestItem("Other", "1", "Old")}, Sections: []SectionOrder{{Title: "Z", Weight: loadAddTestString("009")}, {Title: "Old", Weight: loadAddTestString("01")}}}
	items := []Item{loadAddTestItem("A", "1", "beta"), loadAddTestItem("B", "1", "alpha"), loadAddTestItem("C", "1", "Alpha"), loadAddTestItem("D", "3", "Z"), loadAddTestItem("E", "2", "beta")}
	before, input := loadAddTestJSON(t, old), loadAddTestJSON(t, items)
	got, errs := Add(old, items)
	loadAddTestFailIf(t, got == nil || len(errs) != 0, "Add: %v", errs)
	wantSections := []SectionOrder{{Title: "Z", Weight: loadAddTestString("009")}, {Title: "Old", Weight: loadAddTestString("01")}, {Title: "Alpha"}, {Title: "alpha"}, {Title: "beta"}}
	loadAddTestFailIf(t, loadAddTestJSON(t, got.Sections) != loadAddTestJSON(t, wantSections), "порядок/веса секций: %+v", got.Sections)
	wantItems := []Item{old.Items[0], items[3], old.Items[1], items[2], items[1], items[0], items[4]}
	loadAddTestFailIf(t, loadAddTestJSON(t, got.Items) != loadAddTestJSON(t, wantItems) || got.CourseTitle != old.CourseTitle || *got.UnitTitle != *old.UnitTitle, "изменены метаданные или порядок")
	loadAddTestFailIf(t, loadAddTestJSON(t, old) != before || loadAddTestJSON(t, items) != input, "Add изменил вход")
	loadAddTestDetached(t, got, old, &State{Items: items})
}

func TestLoadAddEditableMissingMixedAndConflictingCandidates(t *testing.T) {
	for _, tc := range []struct {
		name, oldSection, newSection, oldWeight, newWeight string
		codes                                              []string
	}{
		{"missing", "", "", "", "", []string{CodeMissingWeight}},
		{"first sections", "", "New", "1", "2", []string{CodeMixedSections, CodeUnassigned}},
		{"mixed existing", "S", "", "1", "2", []string{CodeMixedSections, CodeUnassigned}},
		{"conflict", "S", "S", "1", "001", []string{CodeWeightConflict}},
		{"new group conflict", "Old", "S", "1", "001", []string{CodeWeightConflict}},
		{"identity outside catalog", "", "", "1", "2", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := &State{CourseTitle: "Курс", Items: []Item{loadAddTestItem("Old", tc.oldWeight, tc.oldSection)}}
			if tc.oldSection != "" {
				old.Sectioned = true
				old.Sections = []SectionOrder{{Title: tc.oldSection}}
			}
			items := []Item{loadAddTestItem("New", tc.newWeight, tc.newSection)}
			participants := []diagnostic.Context{old.Items[0].Context, items[0].Context}
			if tc.name == "new group conflict" {
				items = append(items, loadAddTestItem("New2", "1", "S"))
				participants = []diagnostic.Context{items[0].Context, items[1].Context}
			}
			if tc.name == "identity outside catalog" {
				items[0].Entry.ID, items[0].Entry.FileName = old.Items[0].Entry.ID, old.Items[0].Entry.FileName
			}
			before, input := loadAddTestJSON(t, old), loadAddTestJSON(t, items)
			got, errs := Add(old, items)
			loadAddTestFailIf(t, got == nil || len(got.Items) != 1+len(items), "нет полного кандидата: %v", errs)
			for _, code := range tc.codes {
				loadAddTestCode(t, errs, code)
			}
			loadAddTestFailIf(t, len(tc.codes) == 0 && len(errs) != 0, "%v", errs)
			for _, item := range got.Items {
				loadAddTestFailIf(t, tc.name == "missing" && item.Entry.Order != nil, "синтезирован вес")
			}
			loadAddTestFailIf(t, tc.name == "first sections" && (!got.Sectioned || got.Items[0].Section != nil), "старый урок распределён скрыто")
			if strings.Contains(tc.name, "conflict") {
				problem := loadAddTestCode(t, errs, CodeWeightConflict)
				loadAddTestFailIf(t, loadAddTestJSON(t, problem.Related) != loadAddTestJSON(t, participants), "потеряны участники: %+v", problem)
			}
			if len(tc.codes) != 0 {
				loadAddTestBlocked(t, got)
			}
			loadAddTestFailIf(t, loadAddTestJSON(t, old) != before || loadAddTestJSON(t, items) != input, "изменён вход")
			loadAddTestDetached(t, got, old, &State{Items: items})
		})
	}
}

func TestLoadAddAppendHelpersDoNotMergeOrShare(t *testing.T) {
	shared := loadAddTestString("S")
	a := loadAddTestItem("Old", "1", "S")
	a.Section, a.Entry.Subtitle = shared, shared
	old := &State{CourseTitle: "Курс", UnitTitle: shared, Items: []Item{a}, Sections: []SectionOrder{{Title: "Z", Weight: shared}, {Title: "S"}, {Title: "S"}}}
	b := loadAddTestItem("New", "2", "S")
	b.Section, b.Entry.Subtitle = shared, shared
	items := []Item{b, loadAddTestItem("C", "3", "a"), loadAddTestItem("D", "4", "A"), loadAddTestItem("E", "5", "a")}
	before, input := loadAddTestJSON(t, old), loadAddTestJSON(t, items)
	sections := appendSections(old, items)
	want := []SectionOrder{{Title: "Z", Weight: shared}, {Title: "S"}, {Title: "S"}, {Title: "A"}, {Title: "a"}}
	loadAddTestFailIf(t, loadAddTestJSON(t, sections) != loadAddTestJSON(t, want), "старые дубли слиты или неверный exact-case алфавит")
	*sections[0].Weight = "changed"
	sections[1].Title = "changed"
	loadAddTestFailIf(t, loadAddTestJSON(t, old) != before, "appendSections вернул общие данные")
	got, err := appendItems(old, items)
	loadAddTestFailIf(t, got == nil || err != nil || len(got.Items) != 5, "appendItems: %v", err)
	loadAddTestFailIf(t, loadAddTestJSON(t, got.Items[:1]) != loadAddTestJSON(t, old.Items) || loadAddTestJSON(t, got.Items[1:]) != input, "appendItems переписал записи")
	loadAddTestDetached(t, got, old, &State{Items: items})
}

func loadAddTestItem(id, weight, section string) Item {
	i := Item{Entry: manifest.Entry{ID: id, FileName: "l001.json", Order: loadAddTestString(weight), Title: " Урок " + id, Subtitle: loadAddTestString(""), LineCount: "9007199254740993", CreatedAt: "2000-02-29T01:02:03Z", Version: "18446744073709551617"}, RelativePath: "Каталог/" + id + ".json", Context: diagnostic.Context{Path: id + ".json", FileName: "l001.json", LessonID: id, Line: 7, Column: 2}}
	if weight == "" {
		i.Entry.Order = nil
	}
	if section != "" {
		i.Section = loadAddTestString(section)
	}
	return i
}

func loadAddTestString(s string) *string { return &s }

func loadAddTestJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	loadAddTestFailIf(t, err != nil, "%v", err)
	return string(b)
}

func loadAddTestCode(t *testing.T, errs []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, err := range errs {
		if err != nil && err.Code == code {
			return err
		}
	}
	t.Fatalf("нет %s: %v", code, errs)
	return nil
}

func loadAddTestBlocked(t *testing.T, s *State) {
	t.Helper()
	m, errs := Export(s, "2026-11-04T09:00:00Z", diagnostic.Context{})
	loadAddTestFailIf(t, m != nil || len(errs) == 0, "ошибочный кандидат экспортирован: %v", errs)
}

func loadAddTestDetached(t *testing.T, got *State, sources ...*State) {
	t.Helper()
	pointers := func(s *State) []*string {
		p := []*string{s.UnitTitle}
		for i := range s.Items {
			p = append(p, s.Items[i].Entry.Order, s.Items[i].Entry.Subtitle, s.Items[i].Section)
		}
		for i := range s.Sections {
			p = append(p, s.Sections[i].Weight)
		}
		return p
	}
	seen := make(map[*string]bool)
	for _, p := range pointers(got) {
		loadAddTestFailIf(t, p != nil && seen[p], "результат разделяет строковые указатели")
		seen[p] = true
	}
	for _, source := range sources {
		for _, p := range pointers(source) {
			loadAddTestFailIf(t, p != nil && seen[p], "результат разделяет указатель с входом")
		}
		for i := range got.Items {
			for j := range source.Items {
				loadAddTestFailIf(t, &got.Items[i] == &source.Items[j], "общий срез Items")
			}
		}
		for i := range got.Sections {
			for j := range source.Sections {
				loadAddTestFailIf(t, &got.Sections[i] == &source.Sections[j], "общий срез Sections")
			}
		}
	}
}

func loadAddTestFailIf(t *testing.T, fail bool, format string, args ...any) {
	t.Helper()
	if fail {
		t.Fatalf(format, args...)
	}
}
