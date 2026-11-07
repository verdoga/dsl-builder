package catalog

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func TestLoadAddImportExamplesAndEveryForm(t *testing.T) {
	for _, name := range []string{"Manifest flat example.json", "Manifest unit example.json"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../" + name)
			loadAddTestFailIf(t, err != nil, "%v", err)
			var m manifest.Manifest
			loadAddTestFailIf(t, json.Unmarshal(raw, &m) != nil, "не декодирован пример")
			ctx := diagnostic.Context{Path: "Курс/archive.zip", FileName: "manifest.json"}
			before := loadAddTestJSON(t, m)
			got, errs := Import(m, ctx)
			loadAddTestFailIf(t, loadAddTestJSON(t, m) != before, "Import изменил пример")
			want := loadAddTestImported(t, m, ctx, got)
			loadAddTestFailIf(t, m.Course.Unit == nil && (len(errs) != 1 || errs[0] == nil || errs[0].Code != CodeMissingWeight) || m.Course.Unit != nil && len(errs) != 0, "неверные блокировки примера: %v", errs)
			loadAddTestDetached(t, got, want)
		})
	}
	for _, form := range [][2]bool{{false, false}, {false, true}, {true, false}, {true, true}} {
		unit, sectioned := form[0], form[1]
		a, b := loadAddTestItem("A", "00020", ""), loadAddTestItem("B", "01", "")
		lessons, sections := []manifest.Entry{a.Entry, b.Entry}, []manifest.Section{}
		if sectioned {
			lessons = []manifest.Entry{}
			sections = []manifest.Section{{Title: "Z", Lessons: []manifest.Entry{a.Entry, b.Entry}}, {Title: "a", Lessons: []manifest.Entry{loadAddTestItem("C", "1", "").Entry}}}
		}
		m := loadAddTestManifest(unit, lessons, sections)
		ctx := diagnostic.Context{Path: "archive.zip", FileName: "manifest.json", Line: 9, Column: 2}
		before := loadAddTestJSON(t, m)
		got, errs := Import(m, ctx)
		want := loadAddTestImported(t, m, ctx, got)
		loadAddTestCode(t, errs, CodeLessonOrder)
		loadAddTestFailIf(t, loadAddTestJSON(t, m) != before, "Import изменил manifest")
		loadAddTestDetached(t, got, want)
	}
}

func TestLoadAddImportKeepsMixedDuplicateSectionsAndNull(t *testing.T) {
	a, b, c := loadAddTestItem("A", "2", ""), loadAddTestItem("B", "1", ""), loadAddTestItem("C", "3", "")
	m := loadAddTestManifest(false, []manifest.Entry{c.Entry}, []manifest.Section{{Title: "Z", Lessons: []manifest.Entry{a.Entry}}, {Title: "Z", Lessons: []manifest.Entry{b.Entry}}})
	got, errs := Import(m, diagnostic.Context{Path: "archive.zip"})
	loadAddTestImported(t, m, diagnostic.Context{Path: "archive.zip"}, got)
	for _, code := range []string{CodeDuplicateSection, CodeMixedSections, CodeUnassigned, CodeLessonOrder} {
		loadAddTestCode(t, errs, code)
	}
	a.Entry.Order = nil
	for _, unit := range []bool{false, true} {
		m = loadAddTestManifest(unit, []manifest.Entry{a.Entry}, []manifest.Section{})
		got, errs = Import(m, diagnostic.Context{})
		loadAddTestImported(t, m, diagnostic.Context{}, got)
		loadAddTestCode(t, errs, CodeMissingWeight)
		loadAddTestBlocked(t, got)
		got.Items[0].Entry.Order = loadAddTestString("0001")
		fixed, problems := Refresh(got)
		loadAddTestFailIf(t, len(problems) != 0, "%v", problems)
		exported, problems := Export(fixed, m.CreatedAt, diagnostic.Context{})
		loadAddTestFailIf(t, exported == nil || len(problems) != 0, "явный вес не исправил импорт: %v", problems)
	}
}

func loadAddTestImported(t *testing.T, m manifest.Manifest, ctx diagnostic.Context, got *State) *State {
	t.Helper()
	loadAddTestFailIf(t, len(manifest.Validate(m, ctx)) != 0, "неверная fixture")
	want := &State{CourseTitle: m.Course.Title}
	lessons, sections := []manifest.Entry{}, []manifest.Section{}
	if m.Course.Unit != nil {
		lessons, sections, want.UnitTitle = m.Course.Unit.Lessons, m.Course.Unit.Sections, &m.Course.Unit.Title
	} else {
		lessons, sections = *m.Course.Lessons, *m.Course.Sections
	}
	want.Sectioned = len(sections) != 0
	groups := append([]manifest.Section{{Lessons: lessons}}, sections...)
	for i := range groups {
		var section *string
		if i != 0 {
			section = &sections[i-1].Title
			want.Sections = append(want.Sections, SectionOrder{Title: *section})
		}
		for _, entry := range groups[i].Lessons {
			c := ctx
			c.FileName, c.LessonID = entry.FileName, entry.ID
			want.Items = append(want.Items, Item{Entry: entry, Section: section, Context: c})
		}
	}
	loadAddTestFailIf(t, got == nil || got.CourseTitle != want.CourseTitle || loadAddTestJSON(t, got.UnitTitle) != loadAddTestJSON(t, want.UnitTitle) || got.Sectioned != want.Sectioned || loadAddTestJSON(t, got.Items) != loadAddTestJSON(t, want.Items) || len(got.Sections) != len(want.Sections), "Import изменил размещение/метаданные: %s; want %s", loadAddTestJSON(t, got), loadAddTestJSON(t, want))
	for i := range want.Sections {
		loadAddTestFailIf(t, got.Sections[i].Title != want.Sections[i].Title || got.Sections[i].Weight != nil, "Import изменил секцию/назначил временный вес")
	}
	return want
}

func loadAddTestManifest(unit bool, lessons []manifest.Entry, sections []manifest.Section) manifest.Manifest {
	m := manifest.Manifest{FormatVersion: "1.0", CreatedAt: "2026-11-04T09:00:00Z", Course: manifest.Course{Title: " Курс Ё "}}
	if unit {
		m.Course.Unit = &manifest.Unit{Title: " Юнит ", Lessons: lessons, Sections: sections}
	} else {
		m.Course.Lessons, m.Course.Sections = &lessons, &sections
	}
	return m
}
