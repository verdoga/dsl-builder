package catalog

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strings"
)

// Change — одно предметное изменение манифеста для независимого показа
// «было → станет»; не команда, не подтверждение и не JSON Patch.
type Change struct {
	// FileName — неизменный селектор записи; пусто для курса/юнита/всех секций.
	FileName string
	// Section — имя секции для изменения порядка её уроков; nil для общей группы.
	Section *string
	// Field — course.title, course.unit, sections, section, lessons либо одно
	// из полей Entry: id, fileName, order, title, subtitle, lineCount, createdAt,
	// version; entry обозначает добавленную/отсутствующую целую запись.
	Field string
	// Before — JSON-значение до правки; nil означает отсутствие объекта,
	// литерал null отличается от отсутствия, пустая строка остаётся строкой.
	Before json.RawMessage
	// After — JSON-значение после правки с теми же правилами отсутствия/null.
	After json.RawMessage
}

// Changes координирует changesTitles, changesSections и changesEntries.
// Сравнивает логический экспорт двух согласованных снимков без времени
// корневого manifest, внутренних контекстов, путей и временных весов секций.
// Пустые рабочие секции и один лишь Sectioned не создают изменение JSON.
// Не является единственной проверкой готовности: перед записью нужен Export.
// Строки сравнивает точно, числа Entry — математически без потери точности;
// порядок массивов значим. Пустой результат означает отсутствие изменения
// манифеста, но app отдельно учитывает изменение содержимого урока.
// Порядок отчёта: названия, секции, записи по их порядку после изменения,
// затем исчезнувшие; для полей записи — порядок manifest.Entry.
// Удаление записи здесь только описывается; API удаления уроков пакет не даёт.
func Changes(before *State, after *State) []Change {
	ordered := func(value *State) *State {
		if value == nil {
			return &State{}
		}
		result := *value
		result.Items = make([]Item, 0, len(value.Items))
		for _, item := range value.Items {
			if item.Section == nil {
				result.Items = append(result.Items, item)
			}
		}
		for _, section := range value.Sections {
			for _, item := range value.Items {
				if item.Section != nil && *item.Section == section.Title {
					result.Items = append(result.Items, item)
				}
			}
		}
		return &result
	}
	before, after = ordered(before), ordered(after)
	result := changesTitles(before, after)
	result = append(result, changesSections(before, after)...)
	return append(result, changesEntries(before.Items, after.Items)...)
}

// changesTitles описывает название курса и необязательный юнит:
// course.unit содержит имя либо null, поэтому создание/удаление отличается
// от переименования. Перестройка вложенности не дублируется по каждому полю.
func changesTitles(before *State, after *State) []Change {
	var result []Change
	if before.CourseTitle != after.CourseTitle {
		oldTitle, _ := json.Marshal(before.CourseTitle)
		newTitle, _ := json.Marshal(after.CourseTitle)
		result = append(result, Change{Field: "course.title", Before: oldTitle, After: newTitle})
	}
	oldUnit, _ := json.Marshal(before.UnitTitle)
	newUnit, _ := json.Marshal(after.UnitTitle)
	if !bytes.Equal(oldUnit, newUnit) {
		result = append(result, Change{Field: "course.unit", Before: oldUnit, After: newUnit})
	}
	return result
}

// changesSections описывает порядок непустых секций как массив имён,
// переносы уроков как section и порядок уроков каждой группы как lessons
// с массивами технических имён. Имена сравнивает с учётом регистра.
func changesSections(before *State, after *State) []Change {
	// Общая группа присутствует всегда; отсутствующая секция отличается от [].
	groups := func(value *State) ([]string, map[string][]string) {
		titles := make([]string, 0)
		files := map[string][]string{"": {}}
		for _, item := range value.Items {
			title := ""
			if item.Section != nil {
				title = *item.Section
			}
			files[title] = append(files[title], item.Entry.FileName)
		}
		for _, section := range value.Sections {
			if len(files[section.Title]) != 0 {
				titles = append(titles, section.Title)
			}
		}
		return titles, files
	}
	oldTitles, oldGroups := groups(before)
	newTitles, newGroups := groups(after)
	var result []Change
	oldJSON, _ := json.Marshal(oldTitles)
	newJSON, _ := json.Marshal(newTitles)
	if !bytes.Equal(oldJSON, newJSON) {
		result = append(result, Change{Field: "sections", Before: oldJSON, After: newJSON})
	}
	oldItems := make(map[string]Item, len(before.Items))
	for _, item := range before.Items {
		oldItems[item.Entry.FileName] = item
	}
	for _, item := range after.Items {
		previous, exists := oldItems[item.Entry.FileName]
		if !exists {
			continue
		}
		oldJSON, _ := json.Marshal(previous.Section)
		newJSON, _ := json.Marshal(item.Section)
		if !bytes.Equal(oldJSON, newJSON) {
			result = append(result, Change{FileName: item.Entry.FileName, Field: "section", Before: oldJSON, After: newJSON})
		}
	}
	titles := append([]string{""}, newTitles...)
	for _, title := range oldTitles {
		if _, exists := newGroups[title]; !exists {
			titles = append(titles, title)
		}
	}
	for _, title := range titles {
		var oldJSON, newJSON json.RawMessage
		if files, exists := oldGroups[title]; exists {
			oldJSON, _ = json.Marshal(files)
		}
		if files, exists := newGroups[title]; exists {
			newJSON, _ = json.Marshal(files)
		}
		if bytes.Equal(oldJSON, newJSON) {
			continue
		}
		change := Change{Field: "lessons", Before: oldJSON, After: newJSON}
		if title != "" {
			section := title
			change.Section = &section
		}
		result = append(result, change)
	}
	return result
}

// changesEntries сопоставляет записи по FileName, уже стабилизированному module,
// и делегирует changesEntry. Новая/исчезнувшая запись описывается одним entry
// со всеми полями, без ошибочного попарного сопоставления разных уроков.
func changesEntries(before []Item, after []Item) []Change {
	oldItems := make(map[string]Item, len(before))
	newItems := make(map[string]bool, len(after))
	for _, item := range before {
		oldItems[item.Entry.FileName] = item
	}
	var result []Change
	for _, item := range after {
		name := item.Entry.FileName
		newItems[name] = true
		if previous, exists := oldItems[name]; exists {
			result = append(result, changesEntry(previous, item)...)
		} else {
			entry, _ := json.Marshal(item.Entry)
			result = append(result, Change{FileName: name, Field: "entry", After: entry})
		}
	}
	for _, item := range before {
		if !newItems[item.Entry.FileName] {
			entry, _ := json.Marshal(item.Entry)
			result = append(result, Change{FileName: item.Entry.FileName, Field: "entry", Before: entry})
		}
	}
	return result
}

// changesEntry сравнивает только восемь полей manifest.Entry двух Item.
// json.Number сравнивает точно средствами math/big, без ограничения диапазона;
// Order остаётся строкой: 01 и 1 равны для конфликтов, но различны в JSON.
func changesEntry(before Item, after Item) []Change {
	// Канонизируем коэффициент и показатель, не разворачивая степень десяти.
	// Даже показатель длиннее машинного числа остаётся точным и компактным.
	numberKey := func(raw string) string {
		negative := strings.HasPrefix(raw, "-")
		raw = strings.TrimPrefix(raw, "-")
		var power big.Int
		if index := strings.IndexAny(raw, "eE"); index >= 0 {
			power.SetString(raw[index+1:], 10)
			raw = raw[:index]
		}
		if index := strings.IndexByte(raw, '.'); index >= 0 {
			power.Sub(&power, big.NewInt(int64(len(raw)-index-1)))
			raw = raw[:index] + raw[index+1:]
		}
		raw = strings.TrimLeft(raw, "0")
		if raw == "" {
			return "0"
		}
		digits := strings.TrimRight(raw, "0")
		power.Add(&power, big.NewInt(int64(len(raw)-len(digits))))
		if negative {
			digits = "-" + digits
		}
		return digits + "e" + power.String()
	}
	oldJSON, _ := json.Marshal(before.Entry)
	newJSON, _ := json.Marshal(after.Entry)
	var oldFields, newFields map[string]json.RawMessage
	_ = json.Unmarshal(oldJSON, &oldFields)
	_ = json.Unmarshal(newJSON, &newFields)
	var result []Change
	for _, field := range []string{"id", "fileName", "order", "title", "subtitle", "lineCount", "createdAt", "version"} {
		oldValue, newValue := oldFields[field], newFields[field]
		equal := bytes.Equal(oldValue, newValue)
		if field == "lineCount" || field == "version" {
			equal = numberKey(string(oldValue)) == numberKey(string(newValue))
		}
		if !equal {
			result = append(result, Change{FileName: after.Entry.FileName, Field: field, Before: oldValue, After: newValue})
		}
	}
	return result
}
