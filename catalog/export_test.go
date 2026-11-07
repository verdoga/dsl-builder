package catalog

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/manifest"
)

func exportTestPtr(t *testing.T, raw string) *string { t.Helper(); return &raw }
func exportTestJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
func exportTestEntry(t *testing.T, id, file, weight string) manifest.Entry {
	t.Helper()
	return manifest.Entry{ID: id, FileName: file, Order: exportTestPtr(t, weight), Title: "Урок " + id,
		LineCount: "900719925474099312345678901234567890", Version: "123456789012345678901234567890",
		CreatedAt: "2026-01-02T03:04:05Z"}
}
func exportTestState(t *testing.T, sectioned, unit bool) *State {
	t.Helper()
	s := &State{CourseTitle: "Курс", Sectioned: sectioned, Items: []Item{
		{Entry: exportTestEntry(t, "First:A", "l009.json", "0001"), RelativePath: "secret/a", Context: diagnostic.Context{Path: "input.json"}},
		{Entry: exportTestEntry(t, "Second+Б", "l002.json", "999999999999999999999999999999999999999")},
		{Entry: exportTestEntry(t, "Third", "l1000.json", "1000000000000000000000000000000000000000")},
	}}
	s.Items[1].Entry.Subtitle = exportTestPtr(t, "")
	s.Items[2].Entry.Subtitle = exportTestPtr(t, "Подзаголовок")
	if unit {
		s.UnitTitle = exportTestPtr(t, "Юнит")
	}
	if sectioned {
		s.Sections = []SectionOrder{{Title: "z", Weight: exportTestPtr(t, "9")}, {Title: "Empty"}, {Title: "A", Weight: exportTestPtr(t, "1")}}
		s.Items[0].Section = exportTestPtr(t, "A")
		s.Items[1].Section = exportTestPtr(t, "z")
		s.Items[2].Section = exportTestPtr(t, "z")
	}
	return s
}
func exportTestContents(t *testing.T, m *manifest.Manifest) ([]manifest.Entry, []manifest.Section) {
	t.Helper()
	if m.Course.Unit != nil {
		return m.Course.Unit.Lessons, m.Course.Unit.Sections
	}
	if m.Course.Lessons == nil || m.Course.Sections == nil {
		t.Fatal("нет обязательных массивов course")
	}
	return *m.Course.Lessons, *m.Course.Sections
}
func TestExportFourFormsPreserveExactDataAndOwnership(t *testing.T) {
	for _, unit := range []bool{false, true} {
		for _, sectioned := range []bool{false, true} {
			name := map[bool]string{false: "direct", true: "unit"}[unit] + map[bool]string{false: "/flat", true: "/sectioned"}[sectioned]
			t.Run(name, func(t *testing.T) {
				s := exportTestState(t, sectioned, unit)
				before := exportTestJSON(t, s)
				lessons, sections := []manifest.Entry{}, []manifest.Section{}
				if sectioned {
					sections = append(sections, manifest.Section{Title: "z", Lessons: []manifest.Entry{s.Items[1].Entry, s.Items[2].Entry}}, manifest.Section{Title: "A", Lessons: []manifest.Entry{s.Items[0].Entry}})
				} else {
					lessons = append(lessons, s.Items[0].Entry, s.Items[1].Entry, s.Items[2].Entry)
				}
				course := manifest.Course{Title: "Курс", Lessons: &lessons, Sections: &sections}
				if unit {
					course = manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит", Lessons: lessons, Sections: sections}}
				}
				want := manifest.Manifest{FormatVersion: manifest.FormatVersion, CreatedAt: "2031-06-07T08:09:10Z", Course: course}
				wantJSON := exportTestJSON(t, want)
				got, problems := Export(s, want.CreatedAt, diagnostic.Context{Path: "result.zip", FileName: "manifest.json"})
				if len(problems) != 0 || got == nil {
					t.Fatalf("Export: %v, %v", got, problems)
				}
				if raw := exportTestJSON(t, got); raw != wantJSON {
					t.Fatalf("манифест:\n%s\nожидалось:\n%s", raw, wantJSON)
				}
				var tree any
				decoder := json.NewDecoder(bytes.NewBufferString(exportTestJSON(t, got)))
				decoder.UseNumber()
				if err := decoder.Decode(&tree); err != nil {
					t.Fatal(err)
				}
				if errs := manifest.ValidateShape(tree, diagnostic.Context{}); len(errs) != 0 {
					t.Fatalf("форма: %v", errs)
				}
				if errs := manifest.Validate(*got, diagnostic.Context{}); len(errs) != 0 {
					t.Fatalf("схема: %v", errs)
				}
				if exportTestJSON(t, s) != before {
					t.Fatal("Export изменил State")
				}
				later, errs := Export(s, "2040-02-03T04:05:06Z", diagnostic.Context{})
				if later == nil || len(errs) != 0 {
					t.Fatalf("другая внешняя дата: %v", errs)
				}
				want.CreatedAt = later.CreatedAt
				if later.CreatedAt != "2040-02-03T04:05:06Z" || exportTestJSON(t, later) != exportTestJSON(t, want) {
					t.Fatal("внешняя дата не перенесена точно либо изменила уроки")
				}
				outLessons, outSections := exportTestContents(t, got)
				if sectioned {
					outLessons = outSections[0].Lessons
					outSections[0].Title = "changed"
				}
				outLessons[0].Title = "changed"
				*outLessons[0].Order = "77"
				if sectioned {
					*outLessons[0].Subtitle = "changed"
				} else {
					*outLessons[1].Subtitle = "changed"
				}
				if unit {
					got.Course.Unit.Title = "changed"
				}
				if exportTestJSON(t, s) != before {
					t.Fatal("изменение экспорта затронуло State")
				}
				frozen := exportTestJSON(t, later)
				s.Items[0].Entry.Title = "input changed"
				*s.Items[1].Entry.Order = "88"
				*s.Items[2].Entry.Subtitle = "input changed"
				if unit {
					*s.UnitTitle = "input changed"
				}
				if sectioned {
					s.Sections[0].Title = "input changed"
					*s.Items[1].Section = "input changed"
				}
				if exportTestJSON(t, later) != frozen {
					t.Fatal("изменение State затронуло экспорт")
				}
			})
		}
	}
}
func TestExportRejectsCatalogAndManifestErrors(t *testing.T) {
	cases := []struct {
		name, code string
		change     func(*State)
		date       string
		schema     bool
	}{
		{"empty", CodeEmpty, func(s *State) { s.Items = nil }, "", false},
		{"missing", CodeMissingWeight, func(s *State) { s.Items[0].Entry.Order = nil }, "", false},
		{"conflict", CodeWeightConflict, func(s *State) { s.Items[1].Entry.Order = exportTestPtr(t, "01") }, "", false},
		{"unsorted", CodeLessonOrder, func(s *State) { s.Items[0], s.Items[1] = s.Items[1], s.Items[0] }, "", false},
		{"unassigned", CodeUnassigned, func(s *State) { s.Sectioned = true }, "", false},
		{"mixed", CodeMixedSections, func(s *State) { s.Items[0].Section = exportTestPtr(t, "A"); s.Sections = []SectionOrder{{Title: "A"}} }, "", false},
		{"root-date", manifest.CodeCreatedAt, func(*State) {}, "bad", true},
		{"id", identity.CodeInvalidLessonID, func(s *State) { s.Items[0].Entry.ID = "bad id" }, "", true},
		{"file", identity.CodeInvalidFileName, func(s *State) { s.Items[0].Entry.FileName = "wrong.json" }, "", true},
		{"title", manifest.CodeTitle, func(s *State) { s.Items[0].Entry.Title = " " }, "", false},
		{"subtitle", manifest.CodeShape, func(s *State) { s.Items[0].Entry.Subtitle = exportTestPtr(t, "\xff") }, "", true},
		{"lines", manifest.CodeInteger, func(s *State) { s.Items[0].Entry.LineCount = "-1" }, "", true},
		{"revision", manifest.CodeInteger, func(s *State) { s.Items[0].Entry.Version = "0" }, "", true},
		{"lesson-date", manifest.CodeCreatedAt, func(s *State) { s.Items[0].Entry.CreatedAt = "bad" }, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := exportTestState(t, false, false)
			tc.change(s)
			before := exportTestJSON(t, s)
			if tc.schema && len(Validate(s)) != 0 {
				t.Fatal("fixture должен пройти catalog.Validate")
			}
			date := tc.date
			if date == "" {
				date = "2030-01-02T03:04:05Z"
			}
			ctx := diagnostic.Context{Path: "архив.zip", FileName: "manifest.json"}
			got, errs := Export(s, date, ctx)
			if got != nil {
				t.Fatal("ошибочный экспорт вернул Manifest")
			}
			found := false
			for _, err := range errs {
				if err.Code == tc.code {
					found = true
					if tc.schema && err.Context.Path != ctx.Path {
						t.Errorf("потерян архив: %+v", err.Context)
					}
				}
			}
			if !found {
				t.Fatalf("нет кода %s: %v", tc.code, errs)
			}
			if exportTestJSON(t, s) != before {
				t.Fatal("неуспешный экспорт изменил State")
			}
		})
	}
	if got, errs := Export(nil, "2030-01-02T03:04:05Z", diagnostic.Context{}); got != nil || len(errs) == 0 {
		t.Fatal("nil State принят")
	}
}
func TestReadPlacementPreservesBranchesOrderDuplicatesAndOwnership(t *testing.T) {
	for _, unit := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "unit"}[unit], func(t *testing.T) {
			first := exportTestEntry(t, "A:B", "l003.json", "0009")
			first.Order = nil
			second := exportTestEntry(t, "Б+2", "l001.json", "20")
			second.Subtitle = exportTestPtr(t, "")
			third := exportTestEntry(t, "Last", "l010.json", "0001")
			third.Subtitle = exportTestPtr(t, "text")
			lessons := []manifest.Entry{first}
			sections := []manifest.Section{{Title: "z", Lessons: []manifest.Entry{second, third}}, {Title: "z", Lessons: []manifest.Entry{first}}, {Title: "Z", Lessons: []manifest.Entry{third}}}
			m := manifest.Manifest{FormatVersion: manifest.FormatVersion, CreatedAt: "2030-01-02T03:04:05Z", Course: manifest.Course{Title: "Курс", Lessons: &lessons, Sections: &sections}}
			if unit {
				m.Course = manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит", Lessons: lessons, Sections: sections}}
			}
			if errs := manifest.Validate(m, diagnostic.Context{}); len(errs) != 0 {
				t.Fatalf("fixture: %v", errs)
			}
			before := exportTestJSON(t, m)
			ctx := diagnostic.Context{Path: "архив.zip", FileName: "manifest.json"}
			s := readPlacement(m, ctx)
			if s == nil {
				t.Fatal("nil State")
			}
			want := &State{CourseTitle: "Курс", Sectioned: true, Sections: []SectionOrder{{Title: "z"}, {Title: "z"}, {Title: "Z"}}}
			if unit {
				want.UnitTitle = exportTestPtr(t, "Юнит")
			}
			for i, entry := range []manifest.Entry{first, second, third, first, third} {
				item := Item{Entry: entry, Context: diagnostic.Context{Path: ctx.Path, FileName: entry.FileName, LessonID: entry.ID}}
				if i > 0 {
					title := "z"
					if i == 4 {
						title = "Z"
					}
					item.Section = exportTestPtr(t, title)
				}
				want.Items = append(want.Items, item)
			}
			if exportTestJSON(t, s) != exportTestJSON(t, want) {
				t.Fatalf("размещение:\n%s\nожидалось:\n%s", exportTestJSON(t, s), exportTestJSON(t, want))
			}
			if exportTestJSON(t, m) != before {
				t.Fatal("readPlacement изменил Manifest")
			}
			s.Items[1].Entry.Title = "changed"
			*s.Items[1].Entry.Order = "77"
			*s.Items[1].Entry.Subtitle = "changed"
			*s.Items[1].Section = "changed"
			s.Sections[0].Title = "changed"
			if unit {
				*s.UnitTitle = "changed"
			}
			if exportTestJSON(t, m) != before {
				t.Fatal("State разделяет данные с Manifest")
			}
			if *s.Items[2].Section != "z" || *s.Items[3].Section != "z" {
				t.Fatal("назначения секции разделяют указатель")
			}
			frozen := exportTestJSON(t, s)
			lessons[0].Title = "input changed"
			*sections[0].Lessons[1].Order = "88"
			*sections[0].Lessons[1].Subtitle = "input changed"
			sections[1].Title = "input changed"
			if unit {
				m.Course.Unit.Title = "input changed"
			}
			if exportTestJSON(t, s) != frozen {
				t.Fatal("Manifest разделяет данные со State")
			}
		})
	}
}
func TestReadPlacementFlatHasNoSections(t *testing.T) {
	for _, unit := range []bool{false, true} {
		lessons := []manifest.Entry{exportTestEntry(t, "Second", "l002.json", "20"), exportTestEntry(t, "First", "l001.json", "0001")}
		sections := []manifest.Section{}
		m := manifest.Manifest{FormatVersion: manifest.FormatVersion, CreatedAt: "2030-01-02T03:04:05Z", Course: manifest.Course{Title: "Курс", Lessons: &lessons, Sections: &sections}}
		if unit {
			m.Course = manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит", Lessons: lessons, Sections: sections}}
		}
		if errs := manifest.Validate(m, diagnostic.Context{}); len(errs) != 0 {
			t.Fatalf("fixture: %v", errs)
		}
		s := readPlacement(m, diagnostic.Context{Path: "flat.zip"})
		if s == nil || s.Sectioned || len(s.Sections) != 0 || len(s.Items) != 2 {
			t.Fatalf("плоская ветвь: %+v", s)
		}
		for i := range lessons {
			if s.Items[i].Section != nil || s.Items[i].RelativePath != "" || exportTestJSON(t, s.Items[i].Entry) != exportTestJSON(t, lessons[i]) {
				t.Fatalf("урок %d изменён", i)
			}
		}
	}
}
