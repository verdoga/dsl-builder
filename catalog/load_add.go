package catalog

import (
	"sort"
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

// Import переносит уже проверенный по форме и схеме манифест в State.
// Сохраняет точное размещение и порядок всех массивов, Order и метаданные;
// не синтезирует веса, не сортирует и не присваивает веса секциям.
// context задаёт архив; контексты записей дополняются FileName и ID.
// Дата манифеста остаётся у вызывающего кода, поскольку не часть размещения.
// Дубли имён секций и смешанный состав не исправляет: возвращает ошибки
// Validate и снимок для анализа; решение о продолжении принимает app/module.
// Допустимый схемой null единственного веса сохраняется и блокирует экспорт
// до явного ввода; старый пример не отменяет ТЗ 8.5.
func Import(value manifest.Manifest, context diagnostic.Context) (*State, []*diagnostic.Error) {
	candidate := readPlacement(value, context)
	return candidate, Validate(candidate)
}

// Add создаёт кандидат с полным набором новых записей, без повторной
// обработки старых уроков. Предусловие: module проверил новые ID и имена
// между собой и относительно архива, назначил новые технические имена.
// Проверяет новые имена секций и заданные веса до добавления всего набора;
// неверный исходник даёт nil State, старый снимок остаётся неизменным.
// Отсутствующие веса не синтезирует: Refresh возвращает их как блокировки.
// Новые section берёт из входов; при первом появлении секций включает
// Sectioned, оставляя старые нераспределённые записи для явного распределения.
// Координирует appendItems, appendSections и Refresh. Смешанный кандидат
// остаётся редактируемым; экспорт запрещён до согласования всех уроков.
func Add(value *State, items []Item) (*State, []*diagnostic.Error) {
	if value == nil {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}}
	}
	var problems []*diagnostic.Error
	for index, item := range items {
		context := item.Context
		if context.FileName == "" {
			context.FileName = item.Entry.FileName
		}
		if context.LessonID == "" {
			context.LessonID = item.Entry.ID
		}
		if item.Section != nil {
			if problem := manifest.ValidateSectionTitle(*item.Section, "/items/"+strconv.Itoa(index)+"/section", context); problem != nil {
				problem.Details = append(problem.Details, diagnostic.Detail{Label: "Секция", Value: *item.Section})
				problems = append(problems, problem)
			}
		}
		if item.Entry.Order != nil {
			if problem := order.Validate(*item.Entry.Order, context); problem != nil {
				problems = append(problems, problem)
			}
		}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	candidate, problem := appendItems(value, items)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate.Sections = appendSections(candidate, items)
	for _, item := range items {
		if item.Section != nil {
			candidate.Sectioned = true
			break
		}
	}
	return Refresh(candidate)
}

// appendItems добавляет независимые копии проверенных записей за один шаг;
// старые Entry не меняет. Пустой новый набор даёт CodeEmpty.
func appendItems(value *State, items []Item) (*State, *diagnostic.Error) {
	if value == nil {
		return nil, &diagnostic.Error{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}
	}
	if len(items) == 0 {
		return nil, &diagnostic.Error{Code: CodeEmpty, Reason: "Набор добавляемых уроков пуст."}
	}
	candidate := Clone(value)
	added := Clone(&State{Items: items}).Items
	candidate.Items = append(candidate.Items, added...)
	return candidate, nil
}

// appendSections объединяет точные имена новых и существующих секций.
// Существующий порядок сохраняется, отсутствующие имена добавляются в конец
// по алфавиту. Их временные веса nil; уроки других секций не перемещаются.
func appendSections(value *State, items []Item) []SectionOrder {
	var sections []SectionOrder
	if value != nil {
		sections = Clone(&State{Sections: value.Sections}).Sections
	}
	seen := make(map[string]bool, len(sections))
	for _, section := range sections {
		seen[section.Title] = true
	}
	var missing []string
	for _, item := range items {
		if item.Section != nil && !seen[*item.Section] {
			missing = append(missing, *item.Section)
			seen[*item.Section] = true
		}
	}
	sort.Slice(missing, func(left, right int) bool {
		return compareSectionTitles(missing[left], missing[right]) < 0
	})
	for _, title := range missing {
		sections = append(sections, SectionOrder{Title: title})
	}
	return sections
}
