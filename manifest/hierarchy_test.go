package manifest_test

import (
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/manifest"
)

func TestHierarchyModelCourseFormsJSON(t *testing.T) {
	lessons := []manifest.Entry{}
	sections := []manifest.Section{}
	for _, tc := range []struct {
		name   string
		course manifest.Course
		want   string
	}{
		{"unit", manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит", Lessons: lessons, Sections: sections}}, `{"title":"Курс","unit":{"title":"Юнит","lessons":[],"sections":[]}}`},
		{"direct", manifest.Course{Title: "Курс", Lessons: &lessons, Sections: &sections}, `{"title":"Курс","lessons":[],"sections":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hierarchyModelJSON(t, tc.course, tc.want)
			var decoded manifest.Course
			if err := json.Unmarshal([]byte(tc.want), &decoded); err != nil {
				t.Fatal(err)
			}
			if tc.course.Unit != nil {
				if decoded.Unit == nil || decoded.Lessons != nil || decoded.Sections != nil {
					t.Fatalf("не сохранена форма юнита: %#v", decoded)
				}
				if decoded.Unit.Lessons == nil || decoded.Unit.Sections == nil {
					t.Fatal("пустые массивы юнита стали nil")
				}
			} else {
				if decoded.Unit != nil || decoded.Lessons == nil || decoded.Sections == nil {
					t.Fatalf("не сохранена прямая форма: %#v", decoded)
				}
				if *decoded.Lessons == nil || *decoded.Sections == nil {
					t.Fatal("пустые массивы курса стали nil")
				}
			}
			hierarchyModelJSON(t, decoded, tc.want)
		})
	}
}

func TestHierarchyModelCoursePointerPresence(t *testing.T) {
	var nilLessons []manifest.Entry
	var nilSections []manifest.Section
	emptyLessons := []manifest.Entry{}
	emptySections := []manifest.Section{}
	// Недопустимые по схеме состояния всё равно должны различаться в модели:
	// отсутствие указателя опускает поле, указатель на nil-срез даёт null.
	for _, tc := range []struct {
		name   string
		course manifest.Course
		want   string
	}{
		{"absent", manifest.Course{Title: "Курс"}, `{"title":"Курс"}`},
		{"nil_lessons", manifest.Course{Title: "Курс", Lessons: &nilLessons}, `{"title":"Курс","lessons":null}`},
		{"nil_sections", manifest.Course{Title: "Курс", Sections: &nilSections}, `{"title":"Курс","sections":null}`},
		{"empty_lessons", manifest.Course{Title: "Курс", Lessons: &emptyLessons}, `{"title":"Курс","lessons":[]}`},
		{"empty_sections", manifest.Course{Title: "Курс", Sections: &emptySections}, `{"title":"Курс","sections":[]}`},
		{"both_nil_slices", manifest.Course{Title: "Курс", Lessons: &nilLessons, Sections: &nilSections}, `{"title":"Курс","lessons":null,"sections":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) { hierarchyModelJSON(t, tc.course, tc.want) })
	}
}

func TestHierarchyModelRequiredArraysAndNoSectionWeight(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"unit_zero", manifest.Unit{}, `{"title":"","lessons":null,"sections":null}`},
		{"unit_empty", manifest.Unit{Title: "Юнит", Lessons: []manifest.Entry{}, Sections: []manifest.Section{}}, `{"title":"Юнит","lessons":[],"sections":[]}`},
		{"section_zero", manifest.Section{}, `{"title":"","lessons":null}`},
		{"section_empty", manifest.Section{Title: "Studentbook", Lessons: []manifest.Entry{}}, `{"title":"Studentbook","lessons":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) { hierarchyModelJSON(t, tc.value, tc.want) })
	}
}

func TestHierarchyModelPreservesNamesAndArrayOrder(t *testing.T) {
	// Порядок намеренно отличается от числового веса, имени файла и алфавита:
	// модель не выполняет работу catalog и не нормализует строки.
	const source = `{"title":"  Курс Ё  ","unit":{"title":" Юнит 1 ","lessons":[{"id":"MiXeD:B","fileName":"l009.json","order":"00020","title":" Второй ","subtitle":null,"lineCount":99,"createdAt":"2026-11-04T09:00:00Z","version":7},{"id":"a+1","fileName":"l001.json","order":"01","title":"Первый","subtitle":"","lineCount":0,"createdAt":"2026-11-03T08:00:00Z","version":1}],"sections":[{"title":"studentbook","lessons":[{"id":"Z","fileName":"l003.json","order":null,"title":"Z","subtitle":null,"lineCount":3,"createdAt":"2026-11-04T09:00:00Z","version":1}]},{"title":"Studentbook","lessons":[{"id":"B","fileName":"l002.json","order":null,"title":"B","subtitle":null,"lineCount":2,"createdAt":"2026-11-04T09:00:00Z","version":1}]},{"title":"Ёж","lessons":[{"id":"C","fileName":"l004.json","order":"0001","title":"C","subtitle":null,"lineCount":4,"createdAt":"2026-11-04T09:00:00Z","version":1}]}]}}`
	var course manifest.Course
	if err := json.Unmarshal([]byte(source), &course); err != nil {
		t.Fatal(err)
	}
	if course.Title != "  Курс Ё  " || course.Unit == nil || course.Unit.Title != " Юнит 1 " {
		t.Fatalf("названия изменены: %#v", course)
	}
	unit := course.Unit
	if len(unit.Lessons) != 2 || unit.Lessons[0].ID != "MiXeD:B" || unit.Lessons[1].ID != "a+1" {
		t.Fatalf("порядок уроков или ID изменён: %#v", unit.Lessons)
	}
	if unit.Lessons[0].Order == nil || *unit.Lessons[0].Order != "00020" {
		t.Fatal("написание веса изменено")
	}
	if len(unit.Sections) != 3 {
		t.Fatalf("число секций: %d", len(unit.Sections))
	}
	for i, title := range []string{"studentbook", "Studentbook", "Ёж"} {
		if unit.Sections[i].Title != title {
			t.Errorf("sections[%d].title = %q, требуется %q", i, unit.Sections[i].Title, title)
		}
	}
	hierarchyModelJSON(t, course, source)
	// Та же последовательность должна представляться непосредственно в course.
	flat := manifest.Course{Title: course.Title, Lessons: &unit.Lessons, Sections: &unit.Sections}
	encoded, err := json.Marshal(flat)
	if err != nil {
		t.Fatal(err)
	}
	var decoded manifest.Course
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Unit != nil || decoded.Lessons == nil || decoded.Sections == nil {
		t.Fatalf("форма курса изменена: %#v", decoded)
	}
	hierarchyModelJSON(t, *decoded.Lessons, hierarchyModelEncode(t, unit.Lessons))
	hierarchyModelJSON(t, *decoded.Sections, hierarchyModelEncode(t, unit.Sections))
}

func hierarchyModelEncode(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func hierarchyModelJSON(t *testing.T, value any, want string) {
	t.Helper()
	// Проверяем точные поля и порядок массивов, но порядок ключей несущественен.
	got := hierarchyModelEncode(t, value)
	var gotTree, wantTree any
	if err := json.Unmarshal([]byte(got), &gotTree); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &wantTree); err != nil {
		t.Fatal(err)
	}
	gotCanonical := hierarchyModelEncode(t, gotTree)
	wantCanonical := hierarchyModelEncode(t, wantTree)
	if gotCanonical != wantCanonical {
		t.Errorf("JSON = %s; требуется %s", got, want)
	}
}
