package catalog_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

func TestValidateNilEmptyAndSingleMissingWeight(t *testing.T) {
	validateTestCodes(t, catalog.Validate(nil), catalog.CodeState)
	validateTestCodes(t, catalog.Validate(&catalog.State{CourseTitle: "Курс"}), catalog.CodeEmpty)
	for _, sectioned := range []bool{false, true} {
		s := &catalog.State{CourseTitle: "Курс", Sectioned: sectioned, Items: []catalog.Item{validateTestItem("1", nil, "1")}}
		s.Items[0].Entry.Order = nil
		if sectioned {
			s.Sections = []catalog.SectionOrder{{Title: "Секция"}}
			s.Items[0].Section = validateTestString("Секция")
		}
		problems := catalog.Validate(s)
		validateTestCodes(t, problems, catalog.CodeMissingWeight)
		if problems[0].Context != s.Items[0].Context {
			t.Errorf("потерян контекст единственного урока: %+v", problems[0])
		}
	}
}

func TestValidateNamesUseManifestRulesAndKeepItemOrder(t *testing.T) {
	s := &catalog.State{CourseTitle: "\ufeff", UnitTitle: validateTestString("\t"), Items: []catalog.Item{validateTestItem("1", nil, "1"), validateTestItem("2", nil, "2")}}
	s.Items[0].Entry.Title, s.Items[1].Entry.Title = "", "\u00a0"
	problems := catalog.Validate(s)
	validateTestCodes(t, problems, manifest.CodeTitle, manifest.CodeTitle, manifest.CodeTitle, manifest.CodeTitle)
	for i := range s.Items {
		if problems[i+2].Context != s.Items[i].Context {
			t.Errorf("ошибка названия %d потеряла контекст или порядок", i)
		}
	}
	for _, title := range []string{" Курс ", "\u0085", "Курс\nдва"} {
		s.CourseTitle, s.UnitTitle = title, nil
		for i := range s.Items {
			s.Items[i].Entry.Title = title
		}
		validateTestCodes(t, catalog.Validate(s))
	}
}

func TestValidateExactSectionRegistry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sections []catalog.SectionOrder
		assigned string
		want     []string
	}{
		{"exact_case_distinct", []catalog.SectionOrder{{Title: "Book"}, {Title: "book"}, {Title: "Пустая"}}, "book", nil},
		{"duplicate", []catalog.SectionOrder{{Title: "Book"}, {Title: "Book"}}, "Book", []string{catalog.CodeDuplicateSection}},
		{"missing_exact_case", []catalog.SectionOrder{{Title: "Book"}}, "book", []string{catalog.CodeState}},
		{"empty_title", []catalog.SectionOrder{{Title: ""}}, "", []string{manifest.CodeTitle}},
		{"no_normalization", []catalog.SectionOrder{{Title: " Book "}}, " Book ", []string{manifest.CodeTitle}},
		{"double_space", []catalog.SectionOrder{{Title: "A  B"}}, "A  B", []string{manifest.CodeTitle}},
		{"tab", []catalog.SectionOrder{{Title: "A\tB"}}, "A\tB", []string{manifest.CodeTitle}},
		{"allowed_nbsp", []catalog.SectionOrder{{Title: "\u00a0Book\u00a0"}}, "\u00a0Book\u00a0", nil},
		{"invalid_temporary_weight", []catalog.SectionOrder{{Title: "Book", Weight: validateTestString(" 01")}}, "Book", []string{order.CodeInvalidWeight}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &catalog.State{CourseTitle: "Курс", Sectioned: true, Sections: tc.sections, Items: []catalog.Item{validateTestItem("1", validateTestString(tc.assigned), "1")}}
			before := validateTestSnapshot(t, s)
			validateTestCodes(t, catalog.Validate(s), tc.want...)
			if validateTestSnapshot(t, s) != before {
				t.Fatal("Validate изменил реестр или назначение")
			}
		})
	}
}

func TestValidateDistributionListsEveryUnassignedItem(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		s := &catalog.State{CourseTitle: "Курс", Sectioned: true, Sections: []catalog.SectionOrder{{Title: "S"}}, Items: []catalog.Item{validateTestItem("1", nil, "1"), validateTestItem("2", nil, "2")}}
		want := []string{catalog.CodeUnassigned}
		if mixed {
			s.Items = append(s.Items, validateTestItem("3", validateTestString("S"), "1"))
			want = []string{catalog.CodeMixedSections, catalog.CodeUnassigned}
		}
		problems := catalog.Validate(s)
		validateTestCodes(t, problems, want...)
		if !slices.Equal(problems[len(problems)-1].Related, []diagnostic.Context{s.Items[0].Context, s.Items[1].Context}) {
			t.Errorf("Related не содержит всех нераспределённых в исходном порядке: %+v", problems)
		}
	}
	s := &catalog.State{CourseTitle: "Курс", Sections: []catalog.SectionOrder{{Title: "S"}}, Items: []catalog.Item{validateTestItem("1", validateTestString("S"), "1")}}
	validateTestCodes(t, catalog.Validate(s), catalog.CodeMixedSections)
}

func TestValidateAllPhasesAreReportedInOrderWithoutMutation(t *testing.T) {
	s := &catalog.State{CourseTitle: "", Sectioned: true, Sections: []catalog.SectionOrder{{Title: "S"}, {Title: "S"}}, Items: []catalog.Item{
		validateTestItem("1", nil, "1"), validateTestItem("2", validateTestString("S"), "3"),
		validateTestItem("3", validateTestString("S"), "003"), validateTestItem("6", validateTestString("S"), "0003"),
		validateTestItem("4", validateTestString("S"), "2"), validateTestItem("5", validateTestString("S"), "5"),
	}}
	s.Items[0].Entry.Order = nil
	before := validateTestSnapshot(t, s)
	problems := catalog.Validate(s)
	validateTestCodes(t, problems, manifest.CodeTitle, catalog.CodeDuplicateSection, catalog.CodeMixedSections, catalog.CodeUnassigned, catalog.CodeMissingWeight, catalog.CodeWeightConflict, catalog.CodeLessonOrder)
	if !slices.Equal(problems[5].Related, []diagnostic.Context{s.Items[1].Context, s.Items[2].Context, s.Items[3].Context}) {
		t.Error("конфликт не содержит всех участников")
	}
	for _, raw := range []string{"3", "003", "0003", "l002.json", "l003.json", "l006.json", "Урок 2", "Урок 3", "Урок 6", "S"} {
		if !validateTestDetail(problems[5], raw) {
			t.Errorf("в конфликте отсутствует точная подробность %q: %+v", raw, problems[5].Details)
		}
	}
	if validateTestSnapshot(t, s) != before {
		t.Fatal("Validate исправил исходный снимок")
	}
	problems[5].Related[0].Path = "испорчено"
	problems[5].Details[0].Value = "испорчено"
	again := catalog.Validate(s)
	if again[5].Related[0] != s.Items[1].Context || validateTestSnapshot(t, s) != before {
		t.Error("диагностика разделяет изменяемые данные со снимком")
	}
}

func TestValidateInvalidWeightsNeverCauseCascade(t *testing.T) {
	for _, raw := range []string{"", "0", "000", "-1", "+1", "1.0", "1e2", " 1", "1\n", "١"} {
		t.Run(raw, func(t *testing.T) {
			s := &catalog.State{CourseTitle: "Курс", Items: []catalog.Item{validateTestItem("1", nil, "9"), validateTestItem("2", nil, raw), validateTestItem("3", nil, "1")}}
			problems := catalog.Validate(s)
			validateTestCodes(t, problems, order.CodeInvalidWeight)
			if problems[0].Context != s.Items[1].Context || !validateTestDetail(problems[0], raw) {
				t.Error("потеряны исходный вес или контекст")
			}
		})
	}
	s := &catalog.State{CourseTitle: "Курс", Items: []catalog.Item{validateTestItem("1", nil, "9"), validateTestItem("2", nil, "2"), validateTestItem("3", nil, "1")}}
	s.Items[1].Entry.Order = nil
	validateTestCodes(t, catalog.Validate(s), catalog.CodeMissingWeight)
	s.Items[2].Entry.Order = nil
	problems := catalog.Validate(s)
	validateTestCodes(t, problems, catalog.CodeMissingWeight, catalog.CodeMissingWeight)
	if problems[0].Context != s.Items[1].Context || problems[1].Context != s.Items[2].Context {
		t.Error("не все отсутствующие веса сообщены в порядке уроков")
	}
}

func TestValidateNumericOrderAndIndependentGroups(t *testing.T) {
	large := "1" + strings.Repeat("0", 500)
	s := &catalog.State{CourseTitle: "Курс", Sectioned: true, Sections: []catalog.SectionOrder{{Title: "Z"}, {Title: "A"}}, Items: []catalog.Item{
		validateTestItem("1", validateTestString("Z"), "0002"), validateTestItem("2", validateTestString("Z"), large),
		validateTestItem("3", validateTestString("A"), "2"),
	}}
	validateTestCodes(t, catalog.Validate(s))
	s.Items[0], s.Items[1] = s.Items[1], s.Items[0]
	before := validateTestSnapshot(t, s)
	validateTestCodes(t, catalog.Validate(s), catalog.CodeLessonOrder)
	if validateTestSnapshot(t, s) != before {
		t.Error("проверка молча пересортировала уроки")
	}
}

func TestValidateManifestPreservesAuthoritativePlacement(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip", FileName: "manifest.json", Line: 7, Column: 3}
	for _, unit := range []bool{false, true} {
		for _, scenario := range []string{"authoritative_sections", "duplicate_sections", "mixed", "single_null", "unsorted", "conflict"} {
			t.Run(scenario+map[bool]string{false: "/course", true: "/unit"}[unit], func(t *testing.T) {
				a, b := validateTestItem("1", nil, "1").Entry, validateTestItem("2", nil, "2").Entry
				lessons, sections := []manifest.Entry{}, []manifest.Section{{Title: "Z", Lessons: []manifest.Entry{a}}, {Title: "A", Lessons: []manifest.Entry{b}}}
				var want []string
				switch scenario {
				case "duplicate_sections":
					sections[1].Title = "Z"
					want = []string{catalog.CodeDuplicateSection}
				case "mixed":
					lessons = []manifest.Entry{a}
					sections = sections[1:]
					want = []string{catalog.CodeMixedSections, catalog.CodeUnassigned}
				case "single_null":
					a.Order = nil
					lessons, sections = []manifest.Entry{a}, []manifest.Section{}
					want = []string{catalog.CodeMissingWeight}
				case "unsorted":
					lessons, sections = []manifest.Entry{b, a}, []manifest.Section{}
					want = []string{catalog.CodeLessonOrder}
				case "conflict":
					b.Order = validateTestString("01")
					lessons, sections = []manifest.Entry{a, b}, []manifest.Section{}
					want = []string{catalog.CodeWeightConflict}
				}
				m := manifest.Manifest{FormatVersion: manifest.FormatVersion, CreatedAt: "2026-11-04T09:00:00Z", Course: manifest.Course{Title: "Курс", Lessons: &lessons, Sections: &sections}}
				if unit {
					m.Course = manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит", Lessons: lessons, Sections: sections}}
				}
				validateTestCodes(t, manifest.Validate(m, context))
				before := validateTestSnapshot(t, m)
				problems := catalog.ValidateManifest(m, context)
				validateTestCodes(t, problems, want...)
				if scenario == "single_null" && problems[0].Context != (diagnostic.Context{Path: context.Path, FileName: a.FileName, LessonID: a.ID, Line: 7, Column: 3}) {
					t.Errorf("контекст записи потерян: %+v", problems[0])
				}
				if scenario == "conflict" {
					for i, c := range problems[0].Related {
						if c.Path != context.Path || c.FileName != lessons[i].FileName || c.LessonID != lessons[i].ID {
							t.Errorf("неверный Related: %+v", c)
						}
					}
					if len(problems[0].Related) != 2 {
						t.Error("потерян участник конфликта")
					}
				}
				if validateTestSnapshot(t, m) != before {
					t.Error("ValidateManifest изменил входной манифест")
				}
			})
		}
	}
}

func validateTestString(value string) *string { return &value }

func validateTestItem(number string, section *string, weight string) catalog.Item {
	file := "l" + strings.Repeat("0", 3-len(number)) + number + ".json"
	return catalog.Item{Entry: manifest.Entry{ID: "ID" + number, FileName: file, Title: "Урок " + number, Order: validateTestString(weight), Subtitle: validateTestString(""), LineCount: "12", Version: "1", CreatedAt: "2026-11-04T09:00:00Z"}, Section: section, RelativePath: "вход/" + number + ".json", Context: diagnostic.Context{Path: "источник/" + number + ".json", FileName: file, LessonID: "ID" + number, PageID: "страница", Line: 12, Column: 4}}
}

func validateTestCodes(t *testing.T, problems []*diagnostic.Error, want ...string) {
	t.Helper()
	var got []string
	for _, p := range problems {
		if p == nil {
			t.Fatal("nil вместо ошибки")
		}
		if p.Reason == "" {
			t.Errorf("ошибка %s без причины", p.Code)
		}
		got = append(got, p.Code)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("коды = %v, нужны %v; ошибки: %+v", got, want, problems)
	}
}

func validateTestSnapshot(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func validateTestDetail(problem *diagnostic.Error, raw string) bool {
	for _, detail := range problem.Details {
		if detail.Value == raw {
			return true
		}
	}
	return false
}
