package catalog

import (
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

// Validate проверяет только размещение рабочего снимка: непустой состав,
// названия через manifest, реестр секций, полноту распределения, наличие
// и запись всех весов, конфликты и порядок уроков. Координирует validateSections,
// validateDistribution, validateWeights, Conflicts и validateLessonOrder.
// Обязательный итоговый вес требуется и у единственного урока.
// Не проверяет дерево lesson, ревизии, связи ZIP и уникальность ID/имён.
// Ошибки идут в порядке: названия, секции, распределение, веса, конфликты,
// сортировка; внутри — текущий порядок секций/уроков. Никаких изменений модели.
func Validate(value *State) []*diagnostic.Error {
	if value == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}}
	}
	var problems []*diagnostic.Error
	if problem := manifest.ValidateTitle(value.CourseTitle, "/course/title", diagnostic.Context{}); problem != nil {
		problems = append(problems, problem)
	}
	if value.UnitTitle != nil {
		if problem := manifest.ValidateTitle(*value.UnitTitle, "/course/unit/title", diagnostic.Context{}); problem != nil {
			problems = append(problems, problem)
		}
	}
	for index, item := range value.Items {
		if problem := manifest.ValidateTitle(item.Entry.Title, "/items/"+strconv.Itoa(index)+"/title", item.Context); problem != nil {
			problems = append(problems, problem)
		}
	}
	if len(value.Items) == 0 {
		problems = append(problems, &diagnostic.Error{Code: CodeEmpty, Reason: "Рабочий снимок не содержит уроков."})
	}
	problems = append(problems, validateSections(value)...)
	problems = append(problems, validateDistribution(value)...)
	problems = append(problems, validateWeights(value.Items)...)
	for _, conflict := range Conflicts(value) {
		problems = append(problems, conflictError(conflict))
	}
	return append(problems, validateLessonOrder(value)...)
}

// ValidateManifest проверяет правила catalog у уже проверенного по схеме
// манифеста, не меняя его. Предназначена для module при проверке всего состава.
// Координирует readPlacement и Validate; не вызывает Import, избегая рекурсии.
// Порядок sections из архива является авторитетным: временные веса неизвестны.
func ValidateManifest(value manifest.Manifest, context diagnostic.Context) []*diagnostic.Error {
	return Validate(readPlacement(value, context))
}

// validateSections проверяет имена существующим manifest.ValidateSectionTitle,
// точную уникальность названий реестра, существование каждого Item.Section
// в реестре и валидность ненулевых временных весов через order.Validate.
// Пустые секции рабочего состояния не являются ошибкой; вывод их исключает.
func validateSections(value *State) []*diagnostic.Error {
	if value == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}}
	}
	var problems []*diagnostic.Error
	registered := make(map[string]bool, len(value.Sections))
	for index, section := range value.Sections {
		if problem := manifest.ValidateSectionTitle(section.Title, "/sections/"+strconv.Itoa(index)+"/title", diagnostic.Context{}); problem != nil {
			problem.Details = append(problem.Details, diagnostic.Detail{Label: "Секция", Value: section.Title})
			problems = append(problems, problem)
		}
		if registered[section.Title] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeDuplicateSection, Reason: "Реестр содержит несколько секций с одним точным именем.",
				Details: []diagnostic.Detail{{Label: "Секция", Value: section.Title}},
			})
		}
		registered[section.Title] = true
		if section.Weight != nil {
			if problem := order.Validate(*section.Weight, diagnostic.Context{}); problem != nil {
				problem.Details = append(problem.Details, diagnostic.Detail{Label: "Секция", Value: section.Title})
				problems = append(problems, problem)
			}
		}
	}
	for _, item := range value.Items {
		if item.Section != nil && !registered[*item.Section] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeState, Reason: "Назначенная уроку секция отсутствует в реестре.", Context: item.Context,
				Details: []diagnostic.Detail{{Label: "Секция", Value: *item.Section}},
			})
		}
	}
	return problems
}

// validateDistribution запрещает одновременное наличие назначенных и
// неназначенных уроков, а также любое ненулевое назначение при Sectioned=false.
// При Sectioned=true каждый неназначенный урок блокирует запись, даже если
// пока не распределён ни один; Related перечисляет все незавершённые записи.
func validateDistribution(value *State) []*diagnostic.Error {
	if value == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}}
	}
	var assigned, unassigned []diagnostic.Context
	for _, item := range value.Items {
		if item.Section == nil {
			unassigned = append(unassigned, item.Context)
		} else {
			assigned = append(assigned, item.Context)
		}
	}
	var problems []*diagnostic.Error
	if len(assigned) > 0 && (len(unassigned) > 0 || !value.Sectioned) {
		problem := &diagnostic.Error{Code: CodeMixedSections, Reason: "Размещение уроков не соответствует единой форме: с секциями либо без секций."}
		for _, item := range value.Items {
			problem.Related = append(problem.Related, item.Context)
		}
		problem.Context = assigned[0]
		problems = append(problems, problem)
	}
	if value.Sectioned && len(unassigned) > 0 {
		problems = append(problems, &diagnostic.Error{
			Code: CodeUnassigned, Reason: "Распределение всех уроков по секциям не завершено.",
			Context: unassigned[0], Related: unassigned,
		})
	}
	return problems
}

// validateWeights сообщает каждое nil Entry.Order как CodeMissingWeight;
// ненулевые значения проверяет только через order.Validate с контекстом Item.
// Не применяет правило частичного наличия новой сборки к обновлению/добавлению.
func validateWeights(items []Item) []*diagnostic.Error {
	var problems []*diagnostic.Error
	for _, item := range items {
		if item.Entry.Order == nil {
			problems = append(problems, &diagnostic.Error{
				Code: CodeMissingWeight, Reason: "Не задан обязательный итоговый вес урока.", Context: item.Context,
				Details: []diagnostic.Detail{{Label: "Техническое имя", Value: item.Entry.FileName}},
			})
		} else if problem := order.Validate(*item.Entry.Order, item.Context); problem != nil {
			problems = append(problems, problem)
		}
	}
	return problems
}

// validateLessonOrder проверяет возрастание корректных числовых весов каждой
// группы без скрытой пересортировки. Равенство отражает Conflicts;
// сравнения через отсутствующий или неверный вес пропускаются без каскада.
func validateLessonOrder(value *State) []*diagnostic.Error {
	if value == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}}
	}
	groups := []*string{nil}
	registered := make(map[string]bool, len(value.Sections))
	for _, section := range value.Sections {
		if !registered[section.Title] {
			title := section.Title
			groups = append(groups, &title)
			registered[title] = true
		}
	}
	for _, item := range value.Items {
		if item.Section != nil && !registered[*item.Section] {
			title := *item.Section
			groups = append(groups, &title)
			registered[title] = true
		}
	}
	var problems []*diagnostic.Error
	for _, group := range groups {
		previous := -1
		for index, item := range value.Items {
			if (item.Section == nil) != (group == nil) || (group != nil && *item.Section != *group) {
				continue
			}
			if item.Entry.Order == nil || order.Validate(*item.Entry.Order, item.Context) != nil {
				previous = -1
				continue
			}
			if previous >= 0 && order.Compare(*value.Items[previous].Entry.Order, *item.Entry.Order) > 0 {
				before := value.Items[previous]
				problem := &diagnostic.Error{
					Code: CodeLessonOrder, Reason: "Порядок уроков группы не соответствует возрастанию числовых весов.",
					Context: item.Context, Related: []diagnostic.Context{before.Context, item.Context},
					Details: []diagnostic.Detail{
						{Label: "Техническое имя предыдущего урока", Value: before.Entry.FileName},
						{Label: "Исходный вес предыдущего урока", Value: *before.Entry.Order},
						{Label: "Техническое имя", Value: item.Entry.FileName},
						{Label: "Исходный вес", Value: *item.Entry.Order},
					},
				}
				if group != nil {
					problem.Details = append(problem.Details, diagnostic.Detail{Label: "Секция", Value: *group})
				}
				problems = append(problems, problem)
			}
			previous = index
		}
	}
	return problems
}
