package catalog

import (
	"sort"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/order"
)

// SetSectionWeights принимает полный набор из одного SectionOrder для
// каждой текущей секции. Проверяет точные имена, отсутствие пропусков/повторов
// и все ненулевые Weight через order.Validate. Ошибка ввода даёт nil State.
// Координирует validateSectionWeights, sortSectionsByWeight и Refresh.
// Равные числовые веса секций допустимы; меняется только порядок sections,
// сами временные числа не являются изменением экспортируемого манифеста.
func SetSectionWeights(value *State, weights []SectionOrder) (*State, []*diagnostic.Error) {
	if problems := validateSectionWeights(value, weights); len(problems) != 0 {
		return nil, problems
	}
	candidate := Clone(value)
	candidate.Sections = sortSectionsByWeight(weights)
	return Refresh(candidate)
}

// validateSectionWeights проверяет полноту и однократность точных имён,
// наличие каждого веса и его запись через order.Validate; ничего не сортирует.
func validateSectionWeights(value *State, weights []SectionOrder) []*diagnostic.Error {
	if value == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}}
	}
	var problems []*diagnostic.Error
	registered := make(map[string]bool, len(value.Sections))
	for _, section := range value.Sections {
		if registered[section.Title] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeState, Reason: "Реестр содержит повторное имя секции.",
				Details: []diagnostic.Detail{{Label: "Секция", Value: section.Title}},
			})
		}
		registered[section.Title] = true
	}
	seen := make(map[string]bool, len(weights))
	for _, section := range weights {
		if !registered[section.Title] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeSectionWeights, Reason: "В наборе весов указана неизвестная секция.",
				Details: []diagnostic.Detail{{Label: "Секция", Value: section.Title}},
			})
		}
		if seen[section.Title] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeSectionWeights, Reason: "Вес одной секции указан несколько раз.",
				Details: []diagnostic.Detail{{Label: "Секция", Value: section.Title}},
			})
		}
		seen[section.Title] = true
		if section.Weight == nil {
			problems = append(problems, &diagnostic.Error{
				Code: CodeSectionWeights, Reason: "Не указан вес секции.",
				Details: []diagnostic.Detail{{Label: "Секция", Value: section.Title}},
			})
		} else if problem := order.Validate(*section.Weight, diagnostic.Context{}); problem != nil {
			problem.Details = append(problem.Details, diagnostic.Detail{Label: "Секция", Value: section.Title})
			problems = append(problems, problem)
		}
	}
	for _, section := range value.Sections {
		if !seen[section.Title] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeSectionWeights, Reason: "В наборе отсутствует вес текущей секции.",
				Details: []diagnostic.Detail{{Label: "Секция", Value: section.Title}},
			})
		}
	}
	return problems
}

// compareSectionTitles задаёт детерминированный алфавит: strings.ToLower,
// затем исходное написание. Локаль ОС не используется. Результат −1, 0, 1.
func compareSectionTitles(left string, right string) int {
	if comparison := strings.Compare(strings.ToLower(left), strings.ToLower(right)); comparison != 0 {
		return comparison
	}
	return strings.Compare(left, right)
}

// sortSectionsByWeight сортирует копию полного проверенного набора сначала
// через order.Compare, затем compareSectionTitles; не ограничивает числа int.
func sortSectionsByWeight(values []SectionOrder) []SectionOrder {
	sections := Clone(&State{Sections: values}).Sections
	valid := make(map[string]bool, len(sections))
	for _, section := range sections {
		if section.Weight != nil {
			valid[*section.Weight] = order.Validate(*section.Weight, diagnostic.Context{}) == nil
		}
	}
	sort.SliceStable(sections, func(i, j int) bool {
		left, right := sections[i], sections[j]
		leftValid := left.Weight != nil && valid[*left.Weight]
		rightValid := right.Weight != nil && valid[*right.Weight]
		if leftValid != rightValid {
			return leftValid
		}
		if !leftValid {
			return false
		}
		if comparison := order.Compare(*left.Weight, *right.Weight); comparison != 0 {
			return comparison < 0
		}
		return compareSectionTitles(left.Title, right.Title) < 0
	})
	return sections
}

// sortItems группирует копию по текущему массиву Sections и сортирует уроки
// внутри каждой группы через order.Compare после успешной order.Validate.
// Нераспределённые записи идут отдельно перед секциями. Для равных весов
// сохраняется прежний порядок; это лишь стабильное превью, конфликт остаётся.
// Отсутствующие и неверные веса идут после корректных в прежнем порядке,
// не передаются order.Compare и не подменяются синтезированными значениями.
func sortItems(value *State) []Item {
	if value == nil {
		return nil
	}
	items := Clone(value).Items
	groups := make(map[string]int, len(value.Sections))
	nextGroup := 1
	for _, section := range value.Sections {
		if _, exists := groups[section.Title]; !exists {
			groups[section.Title] = nextGroup
			nextGroup++
		}
	}
	valid := make(map[string]bool, len(items))
	for _, item := range items {
		// Неизвестные реестру назначения сохраняются после известных групп:
		// Validate должен увидеть их и заблокировать кандидат.
		if item.Section != nil {
			if _, exists := groups[*item.Section]; !exists {
				groups[*item.Section] = nextGroup
				nextGroup++
			}
		}
		if item.Entry.Order != nil {
			valid[*item.Entry.Order] = order.Validate(*item.Entry.Order, item.Context) == nil
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		leftGroup, rightGroup := 0, 0
		if left.Section != nil {
			leftGroup = groups[*left.Section]
		}
		if right.Section != nil {
			rightGroup = groups[*right.Section]
		}
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		leftValid := left.Entry.Order != nil && valid[*left.Entry.Order]
		rightValid := right.Entry.Order != nil && valid[*right.Entry.Order]
		if leftValid != rightValid {
			return leftValid
		}
		return leftValid && order.Compare(*left.Entry.Order, *right.Entry.Order) < 0
	})
	return items
}

// Refresh — небольшой координатор Clone, sortItems и Validate всего состава.
// Секции самовольно не пересортировывает: сохраняет импортированный либо
// выбранный порядок. После каждой предметной правки возвращает актуальное
// превью и все блокировки; ошибки других записей не скрываются.
// nil исходного State даёт CodeState и nil результат.
func Refresh(value *State) (*State, []*diagnostic.Error) {
	candidate := Clone(value)
	if candidate == nil {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}}
	}
	candidate.Items = sortItems(candidate)
	return candidate, Validate(candidate)
}
