package catalog_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

func createTestPtr(t *testing.T, s string) *string {
	t.Helper()
	return &s
}

func createTestItem(t *testing.T, n int, section, weight *string) catalog.Item {
	t.Helper()
	name := fmt.Sprintf("l%03d.json", n)
	id := fmt.Sprintf("Lesson+%d", n)
	return catalog.Item{
		Entry: manifest.Entry{ID: id, FileName: name, Order: weight, Title: "Урок " + id,
			Subtitle: createTestPtr(t, "Подзаголовок"), LineCount: "9007199254740993", CreatedAt: "2026-11-04T09:00:00Z", Version: "2"},
		Section: section, RelativePath: fmt.Sprintf("%03d/Work.json", n),
		Context: diagnostic.Context{Path: "input/" + name, FileName: name, LessonID: id, Line: 7, Column: 2},
	}
}

func createTestCode(t *testing.T, errors []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, err := range errors {
		if err != nil && err.Code == code {
			return err
		}
	}
	t.Fatalf("нет кода %q: %v", code, errors)
	return nil
}

func TestCreateRejectsInvalidInitialPlacement(t *testing.T) {
	one := createTestPtr(t, "1")
	section := createTestPtr(t, "Section")
	cases := []struct {
		name  string
		items []catalog.Item
		code  string
	}{
		{"empty", nil, catalog.CodeEmpty},
		{"mixed", []catalog.Item{createTestItem(t, 1, nil, one), createTestItem(t, 2, section, one)}, catalog.CodeMixedSections},
		{"partial flat", []catalog.Item{createTestItem(t, 1, nil, nil), createTestItem(t, 2, nil, one)}, catalog.CodePartialWeight},
		{"partial section", []catalog.Item{createTestItem(t, 1, section, nil), createTestItem(t, 2, section, one)}, catalog.CodePartialWeight},
	}
	for _, raw := range []string{"", " Section", "Section ", "Two  words", "a\tb", "a\nb", "a\rb", "\ufeff", string([]byte{255})} {
		cases = append(cases, struct {
			name  string
			items []catalog.Item
			code  string
		}{
			"section " + fmt.Sprintf("%q", raw), []catalog.Item{createTestItem(t, 1, createTestPtr(t, raw), one)}, manifest.CodeTitle})
	}
	for _, raw := range []string{"", "0", "000", "-1", "+1", "1.0", "1e2", " 1", "1\n", "١"} {
		cases = append(cases, struct {
			name  string
			items []catalog.Item
			code  string
		}{
			"weight " + fmt.Sprintf("%q", raw), []catalog.Item{createTestItem(t, 1, nil, createTestPtr(t, raw))}, order.CodeInvalidWeight})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal(tc.items)
			state, errors := catalog.Create("Курс", "", tc.items)
			if state != nil {
				t.Fatalf("неверный вход дал State: %+v", state)
			}
			problem := createTestCode(t, errors, tc.code)
			if tc.code == order.CodeInvalidWeight {
				if problem.Context != tc.items[0].Context {
					t.Errorf("контекст потерян: %+v", problem.Context)
				}
				found := false
				for _, detail := range problem.Details {
					if detail.Label == "Исходный вес" && detail.Value == *tc.items[0].Entry.Order {
						found = true
					}
				}
				if !found {
					t.Errorf("нет точного исходного веса: %+v", problem.Details)
				}
			}
			after, _ := json.Marshal(tc.items)
			if string(before) != string(after) {
				t.Error("Create изменил вход")
			}
		})
	}
}

func TestCreatePlacementPrevalidationIgnoresUnconvertedEntryFields(t *testing.T) {
	items := []catalog.Item{{Entry: manifest.Entry{ID: "A", FileName: "l001.json"}, RelativePath: "a.json"}}
	if errors := catalog.ValidateNewPlacement(items); len(errors) != 0 {
		t.Fatalf("поля до конвертации проверены: %v", errors)
	}
	items[0].Entry.Title = "\ufeff"
	items[0].Entry.CreatedAt, items[0].Entry.Version, items[0].Entry.LineCount = "bad", "-1", "-1"
	if errors := catalog.ValidateNewPlacement(items); len(errors) != 0 {
		t.Fatalf("не-placement поля проверены: %v", errors)
	}
	items = []catalog.Item{
		createTestItem(t, 1, createTestPtr(t, "A"), nil), createTestItem(t, 2, createTestPtr(t, "A"), createTestPtr(t, "1")),
		createTestItem(t, 3, createTestPtr(t, "B"), createTestPtr(t, "2")), createTestItem(t, 4, createTestPtr(t, "B"), nil),
		createTestItem(t, 5, createTestPtr(t, " Bad"), createTestPtr(t, "0")),
	}
	errors := catalog.ValidateNewPlacement(items)
	partial := 0
	for _, err := range errors {
		if err.Code != catalog.CodePartialWeight {
			continue
		}
		partial++
		if len(err.Related) != 2 {
			t.Errorf("partial Related=%+v", err.Related)
			continue
		}
		start := 0
		if err.Related[0] == items[2].Context {
			start = 2
		}
		if err.Related[0] != items[start].Context || err.Related[1] != items[start+1].Context {
			t.Errorf("не все участники группы: %+v", err.Related)
		}
	}
	if partial != 2 {
		t.Errorf("partial групп=%d, требуется 2", partial)
	}
	createTestCode(t, errors, manifest.CodeTitle)
	createTestCode(t, errors, order.CodeInvalidWeight)
	if state, errors := catalog.Create("Курс", "", items); state != nil || len(errors) == 0 {
		t.Fatalf("ожидался nil+errors, получено %+v, %v", state, errors)
	}
}

func TestCreateSynthesizesFlatWeightsInSuppliedOrder(t *testing.T) {
	items := make([]catalog.Item, 12)
	for i := range items {
		items[i] = createTestItem(t, i+1, nil, nil)
	}
	state, errors := catalog.Create(" Курс ", "", items)
	if state == nil || len(errors) != 0 {
		t.Fatalf("Create=%+v, %v", state, errors)
	}
	if state.UnitTitle != nil || state.Sectioned || len(state.Sections) != 0 || state.CourseTitle != " Курс " {
		t.Fatalf("неверная flat форма: %+v", state)
	}
	if len(state.Items) != len(items) {
		t.Fatalf("Items=%d", len(state.Items))
	}
	for i, item := range state.Items {
		if item.Entry.Order == nil || *item.Entry.Order != fmt.Sprint(i+1) {
			t.Errorf("вес %d=%v", i, item.Entry.Order)
		}
		want := items[i]
		want.Entry.Order = item.Entry.Order
		gotJSON, _ := json.Marshal(item)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("изменены метаданные %d: %s", i, gotJSON)
		}
		if items[i].Entry.Order != nil {
			t.Errorf("входной Order %d изменён", i)
		}
	}
}

func TestCreateGroupsExactSectionsAndSynthesizesIndependently(t *testing.T) {
	items := []catalog.Item{
		createTestItem(t, 1, createTestPtr(t, "beta"), nil), createTestItem(t, 2, createTestPtr(t, "alpha"), nil),
		createTestItem(t, 3, createTestPtr(t, "Alpha"), createTestPtr(t, "001")), createTestItem(t, 4, createTestPtr(t, "alpha"), nil),
		createTestItem(t, 5, createTestPtr(t, "beta"), nil),
	}
	state, errors := catalog.Create("Курс", "Юнит", items)
	if state == nil || len(errors) != 0 {
		t.Fatalf("Create=%+v, %v", state, errors)
	}
	if !state.Sectioned || state.UnitTitle == nil || *state.UnitTitle != "Юнит" || len(state.Sections) != 3 {
		t.Fatalf("неверная форма: %+v", state)
	}
	for i, title := range []string{"Alpha", "alpha", "beta"} {
		if state.Sections[i].Title != title {
			t.Errorf("секция %d=%q", i, state.Sections[i].Title)
		}
	}
	for i, want := range []struct{ file, weight string }{{"l003.json", "001"}, {"l002.json", "1"}, {"l004.json", "2"}, {"l001.json", "1"}, {"l005.json", "2"}} {
		if state.Items[i].Entry.FileName != want.file || state.Items[i].Entry.Order == nil || *state.Items[i].Entry.Order != want.weight {
			t.Errorf("item %d=%+v", i, state.Items[i])
		}
	}
	for i := range items {
		if i != 2 && items[i].Entry.Order != nil {
			t.Errorf("синтез записан во вход %d", i)
		}
	}
}

func TestCreatePreservesRawWeightsAndSortsArbitraryPrecision(t *testing.T) {
	huge := "1" + strings.Repeat("0", 200)
	items := []catalog.Item{createTestItem(t, 1, nil, createTestPtr(t, huge)), createTestItem(t, 2, nil, createTestPtr(t, "00010")), createTestItem(t, 3, nil, createTestPtr(t, "0002"))}
	state, errors := catalog.Create("Курс", "", items)
	if state == nil || len(errors) != 0 {
		t.Fatalf("Create=%+v, %v", state, errors)
	}
	for i, want := range []int{2, 1, 0} {
		if state.Items[i].Entry.FileName != items[want].Entry.FileName || *state.Items[i].Entry.Order != *items[want].Entry.Order {
			t.Errorf("неверная сортировка/raw %d: %+v", i, state.Items[i])
		}
		if state.Items[i].Entry.Order == items[want].Entry.Order {
			t.Errorf("общий Order %d", i)
		}
	}
}

func TestCreateKeepsNumericConflictsEditable(t *testing.T) {
	items := []catalog.Item{createTestItem(t, 1, nil, createTestPtr(t, "001")), createTestItem(t, 2, nil, createTestPtr(t, "1")), createTestItem(t, 3, nil, createTestPtr(t, "01"))}
	if errors := catalog.ValidateNewPlacement(items); len(errors) != 0 {
		t.Fatalf("числовой конфликт не является ошибкой исходного placement: %v", errors)
	}
	state, errors := catalog.Create("Курс", "", items)
	if state == nil {
		t.Fatalf("конфликт должен дать редактируемый State: %v", errors)
	}
	problem := createTestCode(t, errors, catalog.CodeWeightConflict)
	if len(problem.Related) != 3 || len(state.Items) != 3 {
		t.Fatalf("потеря участников: %+v, %+v", problem, state)
	}
	for i := range items {
		if problem.Related[i] != items[i].Context || state.Items[i].Entry.FileName != items[i].Entry.FileName || *state.Items[i].Entry.Order != *items[i].Entry.Order {
			t.Errorf("конфликт изменил/потерял участника %d", i)
		}
	}
	for _, item := range items {
		for _, raw := range []string{item.Entry.FileName, item.Entry.Title, *item.Entry.Order} {
			found := false
			for _, detail := range problem.Details {
				if detail.Value == raw {
					found = true
				}
			}
			if !found {
				t.Errorf("нет детали %q: %+v", raw, problem.Details)
			}
		}
	}
}

func TestCreateValidatesCourseAndOptionalUnitThroughManifest(t *testing.T) {
	for _, tc := range []struct{ course, unit string }{{"", ""}, {"\ufeff", ""}, {"Курс", " "}, {"Курс", string([]byte{255})}} {
		state, errors := catalog.Create(tc.course, tc.unit, []catalog.Item{createTestItem(t, 1, nil, nil)})
		if state != nil {
			t.Errorf("неверное название дало State: %+v", state)
		}
		createTestCode(t, errors, manifest.CodeTitle)
	}
	state, errors := catalog.Create("Курс\tназвание", " Юнит ", []catalog.Item{createTestItem(t, 1, nil, nil)})
	if state == nil || len(errors) != 0 || state.UnitTitle == nil || *state.UnitTitle != " Юнит " {
		t.Fatalf("названия нельзя нормализовать как секции: %+v, %v", state, errors)
	}
}

func TestCreateAndCloneOwnEveryMutableField(t *testing.T) {
	input := []catalog.Item{createTestItem(t, 1, createTestPtr(t, "Секция"), createTestPtr(t, "001"))}
	state, errors := catalog.Create("Курс", "Юнит", input)
	if state == nil || len(errors) != 0 {
		t.Fatalf("Create=%+v, %v", state, errors)
	}
	if state.Items[0].Section == input[0].Section || state.Items[0].Entry.Order == input[0].Entry.Order || state.Items[0].Entry.Subtitle == input[0].Entry.Subtitle {
		t.Fatal("Create разделяет указатели с входом")
	}
	*input[0].Section, *input[0].Entry.Order, *input[0].Entry.Subtitle = "Другой", "999", "Изменён"
	input[0].Entry.Title = "Другой урок"
	if *state.Items[0].Section != "Секция" || *state.Items[0].Entry.Order != "001" || *state.Items[0].Entry.Subtitle != "Подзаголовок" || state.Items[0].Entry.Title == input[0].Entry.Title {
		t.Fatal("изменение входа затронуло State")
	}
	state.Sections[0].Weight = createTestPtr(t, "007")
	clone := catalog.Clone(state)
	if clone == nil || clone == state || clone.UnitTitle == state.UnitTitle || clone.Items[0].Section == state.Items[0].Section || clone.Items[0].Entry.Order == state.Items[0].Entry.Order || clone.Items[0].Entry.Subtitle == state.Items[0].Entry.Subtitle || clone.Sections[0].Weight == state.Sections[0].Weight {
		t.Fatal("Clone разделяет mutable поля")
	}
	*clone.UnitTitle, *clone.Items[0].Section, *clone.Items[0].Entry.Order, *clone.Items[0].Entry.Subtitle, *clone.Sections[0].Weight = "X", "X", "8", "X", "8"
	clone.Items[0].Entry.Title, clone.Sections[0].Title = "X", "X"
	if *state.UnitTitle != "Юнит" || *state.Items[0].Section != "Секция" || *state.Items[0].Entry.Order != "001" || *state.Items[0].Entry.Subtitle != "Подзаголовок" || *state.Sections[0].Weight != "007" || state.Sections[0].Title != "Секция" || state.Items[0].Entry.Title == "X" {
		t.Fatal("изменение Clone затронуло State")
	}
}
