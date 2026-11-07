package catalog

import (
	"math/big"
	"sort"
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

// ValidateNewPlacement проверяет исходные Section и Entry.Order по группам:
// смешение секций и их отсутствия, имена через manifest.ValidateSectionTitle,
// ненулевые веса через order.Validate, частичное наличие через checkPresence.
// Остальные поля Entry не рассматривает: app может вызвать до преобразования,
// заполнив только ID, FileName, Order, Section, RelativePath и Context.
// Ничего не синтезирует и не исправляет. Возвращает все независимые ошибки.
func ValidateNewPlacement(items []Item) []*diagnostic.Error {
	var problems []*diagnostic.Error
	groups := [][]Item{nil}
	sections := make(map[string]int)
	assigned, unassigned := false, false
	for index, item := range items {
		context := item.Context
		if context.FileName == "" {
			context.FileName = item.Entry.FileName
		}
		if context.LessonID == "" {
			context.LessonID = item.Entry.ID
		}
		group := 0
		if item.Section == nil {
			unassigned = true
		} else {
			assigned = true
			if problem := manifest.ValidateSectionTitle(*item.Section, "/items/"+strconv.Itoa(index)+"/section", context); problem != nil {
				problem.Details = append(problem.Details, diagnostic.Detail{Label: "Секция", Value: *item.Section})
				problems = append(problems, problem)
			}
			var exists bool
			group, exists = sections[*item.Section]
			if !exists {
				group = len(groups)
				sections[*item.Section] = group
				groups = append(groups, nil)
			}
		}
		groups[group] = append(groups[group], item)
		if item.Entry.Order != nil {
			if problem := order.Validate(*item.Entry.Order, context); problem != nil {
				problems = append(problems, problem)
			}
		}
	}
	if assigned && unassigned {
		problem := &diagnostic.Error{Code: CodeMixedSections, Reason: "Исходный состав смешивает уроки с секциями и без секций."}
		for _, item := range items {
			context := item.Context
			if context.FileName == "" {
				context.FileName = item.Entry.FileName
			}
			if context.LessonID == "" {
				context.LessonID = item.Entry.ID
			}
			problem.Related = append(problem.Related, context)
		}
		problem.Context = problem.Related[0]
		problems = append(problems, problem)
	}
	for _, group := range groups {
		if problem := checkPresence(group); problem != nil {
			problems = append(problems, problem)
		}
	}
	return problems
}

// Create создаёт новую рабочую модель после преобразования всех уроков.
// Получает готовые записи через EntryFromLesson и метаданные section/order
// через Item; технические имена уже назначены module по входным путям.
// Координирует проверку названий, ValidateNewPlacement, initializeSections,
// initializeWeights и Refresh. Пустой набор, неверные исходные значения,
// смешанный состав и частичное наличие весов дают nil State.
// Числовые совпадения дают ненулевой State с ошибками для исправления в превью.
// Пустой unitTitle означает nil; непустой проверяется manifest.ValidateTitle.
// Предусловие: app передаёт items в порядке filetree.SortRelative —
// по относительному пути без учёта регистра, затем исходному написанию.
// Сортировка путей повторно не реализуется: filetree не прямая зависимость.
func Create(courseTitle string, unitTitle string, items []Item) (*State, []*diagnostic.Error) {
	var problems []*diagnostic.Error
	if problem := manifest.ValidateTitle(courseTitle, "/course/title", diagnostic.Context{}); problem != nil {
		problems = append(problems, problem)
	}
	if unitTitle != "" {
		if problem := manifest.ValidateTitle(unitTitle, "/course/unit/title", diagnostic.Context{}); problem != nil {
			problems = append(problems, problem)
		}
	}
	if len(items) == 0 {
		problems = append(problems, &diagnostic.Error{Code: CodeEmpty, Reason: "Новая сборка не содержит уроков."})
	}
	problems = append(problems, ValidateNewPlacement(items)...)
	if len(problems) != 0 {
		return nil, problems
	}
	weighted, problems := initializeWeights(items)
	if len(problems) != 0 {
		return nil, problems
	}
	sections := initializeSections(weighted)
	candidate := &State{CourseTitle: courseTitle, Items: weighted, Sections: sections, Sectioned: len(sections) != 0}
	if unitTitle != "" {
		candidate.UnitTitle = &unitTitle
	}
	return Refresh(candidate)
}

// initializeSections объединяет точные одинаковые имена и первоначально
// сортирует уникальные секции через compareSectionTitles; регистр сохраняет.
// Предусловие: ValidateNewPlacement успешна; исходные имена не нормализует.
func initializeSections(items []Item) []SectionOrder {
	var sections []SectionOrder
	seen := make(map[string]bool)
	for _, item := range items {
		if item.Section != nil && !seen[*item.Section] {
			sections = append(sections, SectionOrder{Title: *item.Section})
			seen[*item.Section] = true
		}
	}
	sort.Slice(sections, func(left, right int) bool {
		return compareSectionTitles(sections[left].Title, sections[right].Title) < 0
	})
	return sections
}

// initializeWeights координирует checkPresence и synthesizeGroup отдельно
// для каждой секции либо единственной группы без секций. Заданные строки
// остаются точными копиями, включая ведущие нули. Синтез применяется только здесь.
func initializeWeights(items []Item) ([]Item, []*diagnostic.Error) {
	copied := Clone(&State{Items: items}).Items
	groups := [][]int{nil}
	sections := make(map[string]int)
	for index, item := range copied {
		group := 0
		if item.Section != nil {
			var exists bool
			group, exists = sections[*item.Section]
			if !exists {
				group = len(groups)
				sections[*item.Section] = group
				groups = append(groups, nil)
			}
		}
		groups[group] = append(groups[group], index)
	}
	var problems []*diagnostic.Error
	for _, indices := range groups {
		group := make([]Item, len(indices))
		for index, position := range indices {
			group[index] = copied[position]
		}
		if problem := checkPresence(group); problem != nil {
			problems = append(problems, problem)
			continue
		}
		if len(group) == 0 || group[0].Entry.Order != nil {
			continue
		}
		synthesized := synthesizeGroup(group)
		for index, position := range indices {
			copied[position] = synthesized[index]
		}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return copied, nil
}

// checkPresence проверяет только правило «все заданы либо все отсутствуют»
// одной группы новой сборки. При частичном наличии сообщает всех участников;
// пустая строка не считается отсутствием. Проверку записи делает order.Validate.
func checkPresence(items []Item) *diagnostic.Error {
	present := 0
	for _, item := range items {
		if item.Entry.Order != nil {
			present++
		}
	}
	if present == 0 || present == len(items) {
		return nil
	}
	problem := &diagnostic.Error{Code: CodePartialWeight, Reason: "Исходный вес должен быть задан у всех уроков группы либо отсутствовать у всех."}
	if items[0].Section != nil {
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "Секция", Value: *items[0].Section})
	}
	for _, item := range items {
		context := item.Context
		if context.FileName == "" {
			context.FileName = item.Entry.FileName
		}
		if context.LessonID == "" {
			context.LessonID = item.Entry.ID
		}
		problem.Related = append(problem.Related, context)
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "Техническое имя", Value: item.Entry.FileName})
		if item.Entry.Order == nil {
			problem.Details = append(problem.Details, diagnostic.Detail{Label: "Наличие исходного веса", Value: "Отсутствует"})
		} else {
			problem.Details = append(problem.Details, diagnostic.Detail{Label: "Исходный вес", Value: *item.Entry.Order})
		}
	}
	problem.Context = problem.Related[0]
	return problem
}

// synthesizeGroup назначает 1, 2, … одной группе, где все Order равны nil.
// initializeWeights сохраняет в группе относительный порядок общего входного
// среза, уже отсортированного app через filetree.SortRelative. Поэтому синтез
// соответствует относительным путям без дублирования функции ComparePaths.
// Последовательность десятичная, без искусственной верхней границы;
// изменяются только Entry.Order копий, исходные JSON и уроки не затрагиваются.
// Предусловия о путях и порядке обеспечиваются перед вызовом Create.
func synthesizeGroup(items []Item) []Item {
	copied := Clone(&State{Items: items}).Items
	var weight big.Int
	one := big.NewInt(1)
	for index := range copied {
		weight.Add(&weight, one)
		raw := weight.String()
		copied[index].Entry.Order = &raw
	}
	return copied
}
