package catalog

import (
	"sort"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/order"
)

// Conflict — все уроки одной группы с одним числовым весом для превью.
type Conflict struct {
	// Section — точное имя группы либо nil для уроков без секций.
	Section *string
	// Weight — каноническая строка общего числового веса без ведущих нулей.
	// Не заменяет исходные строки Entry.Order участников.
	Weight string
	// Items — все участники, включая первый, с именами, названиями,
	// исходными строками весов и контекстами; не только конфликтующая пара.
	Items []Item
}

// Conflicts собирает все классы численно равных корректных весов внутри
// одной секции либо общей группы, используя order.Compare. Разные секции
// не конфликтуют. Неверные/отсутствующие веса пропускает: их сообщает Validate.
// Порядок конфликтов — порядок групп, затем числовой вес; участников — Items.
// Исходные записи не меняет; каноническое отображение убирает только нули.
func Conflicts(value *State) []Conflict {
	if value == nil {
		return nil
	}
	snapshot := Clone(value)
	// Непосредственная группа предшествует реестру секций. Неизвестные
	// реестру назначения тоже проверяются, в порядке первого появления.
	groups := []Conflict{{}}
	sectionIndices := make(map[string]int, len(snapshot.Sections))
	for _, section := range snapshot.Sections {
		if _, exists := sectionIndices[section.Title]; exists {
			continue
		}
		title := section.Title
		sectionIndices[title] = len(groups)
		groups = append(groups, Conflict{Section: &title})
	}
	for _, item := range snapshot.Items {
		index := 0
		if item.Section != nil {
			var exists bool
			index, exists = sectionIndices[*item.Section]
			if !exists {
				title := *item.Section
				index = len(groups)
				sectionIndices[title] = index
				groups = append(groups, Conflict{Section: &title})
			}
		}
		if item.Entry.Order == nil || order.Validate(*item.Entry.Order, item.Context) != nil {
			continue
		}
		groups[index].Items = append(groups[index].Items, item)
	}
	var conflicts []Conflict
	for _, group := range groups {
		sort.SliceStable(group.Items, func(i, j int) bool {
			return order.Compare(*group.Items[i].Entry.Order, *group.Items[j].Entry.Order) < 0
		})
		for start := 0; start < len(group.Items); {
			end := start + 1
			for end < len(group.Items) && order.Compare(*group.Items[start].Entry.Order, *group.Items[end].Entry.Order) == 0 {
				end++
			}
			if end-start > 1 {
				conflict := Conflict{
					Weight: strings.TrimLeft(*group.Items[start].Entry.Order, "0"),
					Items:  append([]Item(nil), group.Items[start:end]...),
				}
				if group.Section != nil {
					title := *group.Section
					conflict.Section = &title
				}
				conflicts = append(conflicts, conflict)
			}
			start = end
		}
	}
	return conflicts
}

// conflictError превращает один Conflict в блокирующий CodeWeightConflict.
// Related содержит контексты всех участников, Details — секцию, общий вес,
// технические имена, названия и исходный вес каждого в том же порядке.
// Доступный исходный путь не заменяется document.filePath или догадкой.
func conflictError(value Conflict) *diagnostic.Error {
	section := "Без секции"
	if value.Section != nil {
		section = *value.Section
	}
	problem := &diagnostic.Error{
		Code:   CodeWeightConflict,
		Reason: "Уроки одной группы имеют одинаковый числовой вес.",
		Details: []diagnostic.Detail{
			{Label: "Секция", Value: section},
			{Label: "Общий числовой вес", Value: value.Weight},
		},
	}
	for _, item := range value.Items {
		problem.Related = append(problem.Related, item.Context)
		raw := ""
		if item.Entry.Order != nil {
			raw = *item.Entry.Order
		}
		problem.Details = append(problem.Details,
			diagnostic.Detail{Label: "Техническое имя", Value: item.Entry.FileName},
			diagnostic.Detail{Label: "Название", Value: item.Entry.Title},
			diagnostic.Detail{Label: "Исходный вес", Value: raw},
		)
	}
	if len(problem.Related) > 0 {
		problem.Context = problem.Related[0]
	}
	return problem
}
