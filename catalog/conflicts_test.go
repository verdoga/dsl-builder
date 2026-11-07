package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func conflictsTestString(t *testing.T, value string) *string {
	t.Helper()
	return &value
}

func conflictsTestItem(t *testing.T, number int, section *string, weight *string) Item {
	t.Helper()
	name := fmt.Sprintf("l%03d.json", number)
	return Item{
		Entry: manifest.Entry{ID: fmt.Sprintf("ID-%d", number), FileName: name, Order: weight,
			Title: fmt.Sprintf("Урок %d", number), Subtitle: conflictsTestString(t, "Подзаголовок"),
			LineCount: "15", Version: "1", CreatedAt: "2026-11-04T09:00:00Z"},
		Section: section, RelativePath: "Вход/" + name,
		Context: diagnostic.Context{Path: `C:\Исходники\` + name, FileName: name,
			LessonID: fmt.Sprintf("ID-%d", number), PageID: "page", Line: number, Column: 2},
	}
}

func conflictsTestJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestConflictsEmptyAndAllDuplicateParticipants(t *testing.T) {
	for _, state := range []*State{nil, {}, {Items: []Item{}}} {
		if got := Conflicts(state); len(got) != 0 {
			t.Fatalf("пустой состав: получены конфликты %v", got)
		}
	}
	for count := 1; count <= 3; count++ {
		t.Run(fmt.Sprintf("participants_%d", count), func(t *testing.T) {
			state := &State{CourseTitle: "Курс"}
			for i, weight := range []string{"1", "01", "001"}[:count] {
				state.Items = append(state.Items, conflictsTestItem(t, i+1, nil, conflictsTestString(t, weight)))
			}
			before := conflictsTestJSON(t, state)
			got := Conflicts(state)
			if count == 1 {
				if len(got) != 0 {
					t.Fatalf("единственный участник: %v", got)
				}
			} else {
				want := []Conflict{{Weight: "1", Items: state.Items}}
				if conflictsTestJSON(t, got) != conflictsTestJSON(t, want) {
					t.Fatalf("не все участники или потеря исходных данных: %s", conflictsTestJSON(t, got))
				}
			}
			if conflictsTestJSON(t, state) != before {
				t.Fatal("Conflicts изменил входной снимок")
			}
		})
	}
}

func TestConflictsExactGroupsNumericOrderAndStableParticipants(t *testing.T) {
	upper, lower, workbook := conflictsTestString(t, "Studentbook"), conflictsTestString(t, "studentbook"), conflictsTestString(t, "Workbook")
	huge := "1" + strings.Repeat("0", 250)
	state := &State{Sectioned: true, Sections: []SectionOrder{{Title: *workbook}, {Title: *upper}, {Title: *lower}}}
	for i, spec := range []struct {
		section *string
		weight  string
	}{
		{upper, "10"}, {lower, "1"}, {workbook, huge}, {nil, "1"}, {workbook, "10"},
		{upper, "010"}, {workbook, "2"}, {lower, "01"}, {nil, "001"}, {workbook, "000" + huge},
		{workbook, "02"}, {workbook, "010"}, {upper, "0010"},
	} {
		state.Items = append(state.Items, conflictsTestItem(t, i+1, spec.section, conflictsTestString(t, spec.weight)))
	}
	items := state.Items
	want := []Conflict{
		{Weight: "1", Items: []Item{items[3], items[8]}},
		{Section: workbook, Weight: "2", Items: []Item{items[6], items[10]}},
		{Section: workbook, Weight: "10", Items: []Item{items[4], items[11]}},
		{Section: workbook, Weight: huge, Items: []Item{items[2], items[9]}},
		{Section: upper, Weight: "10", Items: []Item{items[0], items[5], items[12]}},
		{Section: lower, Weight: "1", Items: []Item{items[1], items[7]}},
	}
	before := conflictsTestJSON(t, state)
	for repeat := 0; repeat < 10; repeat++ {
		if got := conflictsTestJSON(t, Conflicts(state)); got != conflictsTestJSON(t, want) {
			t.Fatalf("порядок групп/чисел/участников или точность: %s", got)
		}
	}
	if conflictsTestJSON(t, state) != before {
		t.Fatal("изменён исходный состав")
	}
	// Одинаковые веса одиночных уроков разных exact-case секций допустимы.
	state = &State{Sectioned: true, Sections: []SectionOrder{{Title: *upper}, {Title: *lower}}, Items: []Item{
		conflictsTestItem(t, 1, upper, conflictsTestString(t, "01")),
		conflictsTestItem(t, 2, lower, conflictsTestString(t, "1")),
	}}
	if got := Conflicts(state); len(got) != 0 {
		t.Fatalf("разные секции объединены: %v", got)
	}
}

func TestConflictsSkipInvalidAndMissingWeights(t *testing.T) {
	state := &State{Items: []Item{conflictsTestItem(t, 1, nil, nil), conflictsTestItem(t, 2, nil, nil)}}
	for _, raw := range []string{"", "0", "000", "-1", "+1", "1.0", "1e0", " 1", "1 ", "1\n", "١", "１", "1\x00", "x"} {
		for repeat := 0; repeat < 2; repeat++ {
			state.Items = append(state.Items, conflictsTestItem(t, len(state.Items)+1, nil, conflictsTestString(t, raw)))
		}
	}
	// Эти соседние большие значения различны, хотя float64 их не различает.
	for _, raw := range []string{"9007199254740992", "9007199254740993"} {
		state.Items = append(state.Items, conflictsTestItem(t, len(state.Items)+1, nil, conflictsTestString(t, raw)))
	}
	before := conflictsTestJSON(t, state)
	if got := Conflicts(state); len(got) != 0 {
		t.Fatalf("неверные/отсутствующие либо различные точные веса конфликтуют: %v", got)
	}
	state.Items = append(state.Items, conflictsTestItem(t, 101, nil, conflictsTestString(t, "1")), conflictsTestItem(t, 102, nil, conflictsTestString(t, "001")))
	want := []Conflict{{Weight: "1", Items: state.Items[len(state.Items)-2:]}}
	if got := conflictsTestJSON(t, Conflicts(state)); got != conflictsTestJSON(t, want) {
		t.Fatalf("валидный конфликт среди неверных весов потерян: %s", got)
	}
	state.Items = state.Items[:len(state.Items)-2]
	if conflictsTestJSON(t, state) != before {
		t.Fatal("пропуск неверных весов изменил вход")
	}
}

func TestConflictsOwnReturnedSlicesAndStringPointers(t *testing.T) {
	section := conflictsTestString(t, "Секция")
	state := &State{Sectioned: true, Sections: []SectionOrder{{Title: *section}}, Items: []Item{
		conflictsTestItem(t, 1, section, conflictsTestString(t, "01")), conflictsTestItem(t, 2, section, conflictsTestString(t, "1")),
	}}
	before := conflictsTestJSON(t, state)
	got, independent := Conflicts(state), Conflicts(state)
	if len(got) != 1 || len(got[0].Items) != 2 {
		t.Fatalf("нет ожидаемого конфликта: %v", got)
	}
	independentBefore := conflictsTestJSON(t, independent)
	*got[0].Section = "Изменённая группа"
	*got[0].Items[0].Section = "Изменённая секция"
	*got[0].Items[0].Entry.Order = "99"
	*got[0].Items[0].Entry.Subtitle = "Изменённый подзаголовок"
	got[0].Items[0].Entry.Title = "Изменённый заголовок"
	got[0].Items[0].Context.Path = "изменённый путь"
	got[0].Items[1] = Item{}
	if conflictsTestJSON(t, state) != before || conflictsTestJSON(t, independent) != independentBefore {
		t.Fatal("результат разделяет изменяемые данные с входом или другим вызовом")
	}
	*state.Items[0].Entry.Order = "7"
	*state.Items[0].Entry.Subtitle = "Другой подзаголовок"
	*state.Items[0].Section = "Другая секция"
	state.Items[1] = Item{}
	if conflictsTestJSON(t, independent) != independentBefore {
		t.Fatal("изменение входа затронуло ранее возвращённый конфликт")
	}
}

func TestConflictErrorKeepsAllContextsRawDetailsAndOwnership(t *testing.T) {
	for _, section := range []*string{nil, conflictsTestString(t, "Studentbook")} {
		value := Conflict{Section: section, Weight: "10"}
		for i, weight := range []string{"10", "010", "0010"} {
			value.Items = append(value.Items, conflictsTestItem(t, i+1, section, conflictsTestString(t, weight)))
		}
		value.Items[1].Context = diagnostic.Context{Path: `C:\Модули\архив.zip`, FileName: "l002.json", LessonID: "ID-2"}
		value.Items[2].Context = diagnostic.Context{}
		before := conflictsTestJSON(t, value)
		got := conflictError(value)
		if got == nil || got.Code != CodeWeightConflict || got.Cause != nil || !strings.ContainsFunc(got.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
			t.Fatalf("неверная блокирующая диагностика: %+v", got)
		}
		if got.Context != value.Items[0].Context || len(got.Related) != len(value.Items) {
			t.Fatalf("основной контекст/число связанных мест: %+v", got)
		}
		for i, item := range value.Items {
			if got.Related[i] != item.Context {
				t.Fatalf("Related[%d] потерял точный контекст: %+v", i, got.Related[i])
			}
		}
		var want []string
		if section != nil {
			want = append(want, *section)
		}
		want = append(want, value.Weight)
		for _, item := range value.Items {
			want = append(want, item.Entry.FileName, item.Entry.Title, *item.Entry.Order)
		}
		index := 0
		for _, detail := range got.Details {
			if !strings.ContainsFunc(detail.Label, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
				t.Fatalf("нет русского названия подробности: %+v", detail)
			}
			if index < len(want) && detail.Value == want[index] {
				index++
			}
		}
		if index != len(want) {
			t.Fatalf("Details потерял точные значения или порядок: %+v; требуется %q", got.Details, want)
		}
		if conflictsTestJSON(t, value) != before {
			t.Fatal("conflictError изменил вход")
		}
		other := conflictError(value)
		otherBefore := conflictsTestJSON(t, other)
		got.Related[0].Path = "другой путь"
		got.Details[0].Value = "другая подробность"
		if conflictsTestJSON(t, value) != before || conflictsTestJSON(t, other) != otherBefore {
			t.Fatal("диагностика разделяет изменяемые данные с другим результатом или входом")
		}
	}
}
