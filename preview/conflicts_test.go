package preview

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func TestOrderConflictsPreservesEveryGroupAndParticipant(t *testing.T) {
	section := "Студенческая книга Ω"
	huge := "98765432109876543210987654321098765432109876543210987654321"
	values := []catalog.Conflict{
		{Section: &section, Weight: huge, Items: []catalog.Item{
			conflictsTestItem("l903.json", "Первый переданный урок", "00"+huge, `C:\Материалы\Я.json`, ""),
			conflictsTestItem("l002.json", "Второй переданный урок", huge, `C:\Архивы\Юнит 1.zip`, "l707.json"),
			conflictsTestItem("l101.json", "Третий переданный урок", "0"+huge, `C:\Материалы\А.json`, ""),
		}},
		{Weight: "4", Items: []catalog.Item{
			conflictsTestItem("l505.json", "Общая группа первый", "004", `D:\Вход\Б.json`, ""),
			conflictsTestItem("l004.json", "Общая группа второй", "4", `D:\Вход\А.json`, ""),
		}},
	}
	before := conflictsTestSnapshot(t, values)
	got := OrderConflicts(values)
	conflictsTestSafe(t, got)
	conflictsTestQuoted(t, got, section)
	conflictsTestBlocking(t, got)
	previous := -1
	for _, conflict := range values {
		if !strings.Contains(got, conflict.Weight) {
			t.Errorf("не показан общий числовой вес %q", conflict.Weight)
		}
		for _, item := range conflict.Items {
			position := strings.Index(got, item.Entry.Title)
			if position <= previous {
				t.Errorf("порядок групп/участников нарушен у %q", item.Entry.Title)
			}
			previous = position
			for _, value := range []string{item.Entry.FileName, item.Entry.Title, *item.Entry.Order, item.Context.Path} {
				conflictsTestQuoted(t, got, value)
			}
			if item.Context.FileName != "" {
				conflictsTestQuoted(t, got, item.Context.FileName)
			}
		}
	}
	general := strings.ToLower(got[strings.Index(got, "Третий переданный урок")+len("Третий переданный урок"):])
	if !strings.Contains(general, "общ") && !strings.Contains(general, "без секци") {
		t.Error("не обозначена общая группа уроков без секций")
	}
	if after := conflictsTestSnapshot(t, values); after != before {
		t.Error("входные группы, участники или строковые указатели изменены")
	}
	if again := OrderConflicts(values); again != got {
		t.Error("повторное представление того же снимка отличается")
	}
}

func TestOrderConflictsUsesSuppliedWeightWithoutRecomputation(t *testing.T) {
	// Различные маркеры проверяют границу представления: Weight уже передан
	// владельцем результата, preview не выводит его заново из Entry.Order.
	weight := "271828182845904523536028747135266249775724709369995"
	values := []catalog.Conflict{{Weight: weight, Items: []catalog.Item{
		conflictsTestItem("l001.json", "Один", "7", "", ""),
		conflictsTestItem("l002.json", "Два", "007", "", ""),
		conflictsTestItem("l003.json", "Три", "07", "", ""),
	}}}
	got := OrderConflicts(values)
	if !strings.Contains(got, weight) {
		t.Errorf("переданный общий вес отсутствует: %q", got)
	}
	for _, raw := range []string{"7", "007", "07"} {
		conflictsTestQuoted(t, got, raw)
	}
	conflictsTestBlocking(t, got)
}

func TestOrderConflictsEscapesCompleteExternalStrings(t *testing.T) {
	value := " начало Кириллица Ω 😀 \"\\\n\r\t\x00\x07\b\x1b[2J\x7f\u0085 " + strings.Repeat("длинное значение ", 80) + "КОНЕЦ "
	for _, field := range []string{"section", "weight", "fileName", "title", "order", "path", "archiveEntry"} {
		t.Run(field, func(t *testing.T) {
			conflict := catalog.Conflict{Weight: "8", Items: []catalog.Item{
				conflictsTestItem("l008.json", "Восьмой", "008", `C:\Исходники\Урок.json`, "l008.json"),
				conflictsTestItem("l009.json", "Девятый", "8", "", ""),
			}}
			switch field {
			case "section":
				conflict.Section = &value
			case "weight":
				conflict.Weight = value
			case "fileName":
				conflict.Items[0].Entry.FileName = value
			case "title":
				conflict.Items[0].Entry.Title = value
			case "order":
				conflict.Items[0].Entry.Order = &value
			case "path":
				conflict.Items[0].Context.Path = value
			case "archiveEntry":
				conflict.Items[0].Context.FileName = value
			}
			values := []catalog.Conflict{conflict}
			before := conflictsTestSnapshot(t, values)
			got := OrderConflicts(values)
			conflictsTestSafe(t, got)
			conflictsTestQuoted(t, got, value)
			if conflictsTestSnapshot(t, values) != before {
				t.Error("экранирование изменило исходную модель")
			}
		})
	}
}

func TestOrderConflictsEmptyMeansOnlyNoWeightConflicts(t *testing.T) {
	for _, values := range [][]catalog.Conflict{nil, {}} {
		got := OrderConflicts(values)
		conflictsTestSafe(t, got)
		lower := strings.ToLower(got)
		if !strings.Contains(lower, "конфликт") || (!strings.Contains(lower, "нет") && !strings.Contains(lower, "отсутств") && !strings.Contains(lower, "не обнаруж")) {
			t.Errorf("нет явного сообщения об отсутствии конфликтов весов: %q", got)
		}
		for _, forbidden := range []string{"ошибка сборки", "запрещ", "готов", "успеш", "запись разреш", "можно запис", "изменений нет", "операция заверш"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("пустой список не определяет состояние всей операции: %q", got)
			}
		}
	}
}

func conflictsTestItem(name, title, raw, path, archiveEntry string) catalog.Item {
	return catalog.Item{
		Entry: manifest.Entry{
			ID: "LESSON-" + name, FileName: name, Title: title, Order: &raw,
			LineCount: json.Number("12"), Version: json.Number("1"), CreatedAt: "2026-11-04T09:00:00Z",
		},
		RelativePath: "НЕ ИСТОЧНИК/" + name,
		Context:      diagnostic.Context{Path: path, FileName: archiveEntry},
	}
}

func conflictsTestSnapshot(t *testing.T, values []catalog.Conflict) string {
	t.Helper()
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func conflictsTestBlocking(t *testing.T, got string) {
	t.Helper()
	if !strings.Contains(got, "ОШИБКА СБОРКИ") {
		t.Error("непустой список не обозначен как ОШИБКА СБОРКИ")
	}
	lower := strings.ToLower(got)
	if !strings.Contains(lower, "запис") || (!strings.Contains(lower, "запрещ") && !strings.Contains(lower, "нельзя")) || !strings.Contains(lower, "исправ") {
		t.Errorf("нет запрета записи до исправления весов: %q", got)
	}
}

func conflictsTestSafe(t *testing.T, got string) {
	t.Helper()
	if !utf8.ValidString(got) {
		t.Error("текст не является UTF-8")
	}
	for _, r := range got {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("неэкранированный управляющий символ U+%04X", r)
			break
		}
	}
}

func conflictsTestQuoted(t *testing.T, got, want string) {
	t.Helper()
	// Декодируем кавычки, не закрепляя конкретный вид допустимых escape-кодов.
	for start := 0; start < len(got); start++ {
		if got[start] != '"' {
			continue
		}
		for end := start + 1; end < len(got); end++ {
			if got[end] == '\\' {
				end++
				continue
			}
			if got[end] == '"' {
				decoded, err := strconv.Unquote(got[start : end+1])
				if err == nil && decoded == want {
					return
				}
				start = end
				break
			}
		}
	}
	t.Errorf("нет полного значения в безопасных кавычках: %q", want)
}
