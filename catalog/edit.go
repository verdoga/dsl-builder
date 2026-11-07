package catalog

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

// SetCourseTitle строит кандидат с одним изменённым названием курса,
// проверенным manifest.ValidateTitle, и вызывает Refresh; путь не используется.
func SetCourseTitle(value *State, title string) (*State, []*diagnostic.Error) {
	if value == nil {
		return Refresh(nil)
	}
	if problem := manifest.ValidateTitle(title, "/course/title", diagnostic.Context{}); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate := Clone(value)
	candidate.CourseTitle = title
	return Refresh(candidate)
}

// SetUnitTitle строит кандидат с новым названием либо без юнита при title == "".
// Непустое название проверяет manifest.ValidateTitle. Переносит вложенность
// только при экспорте, сохраняя уроки и секции; затем вызывает Refresh.
func SetUnitTitle(value *State, title string) (*State, []*diagnostic.Error) {
	if value == nil {
		return Refresh(nil)
	}
	if title != "" {
		if problem := manifest.ValidateTitle(title, "/course/unit/title", diagnostic.Context{}); problem != nil {
			return nil, []*diagnostic.Error{problem}
		}
	}
	candidate := Clone(value)
	candidate.UnitTitle = nil
	if title != "" {
		candidate.UnitTitle = &title
	}
	return Refresh(candidate)
}

// NormalizeSectionTitle приводит новый пользовательский ввод к схеме:
// заменяет TAB, CR, LF обычным пробелом, убирает крайние U+0020 и повторные
// U+0020. Не меняет регистр, прочие Unicode-пробелы и Unicode-нормализацию.
// Результат проверяет manifest.ValidateSectionTitle; пустое имя ошибочно.
// Исходные метаданные JSON и прочитанные имена этим способом не исправляются.
func NormalizeSectionTitle(raw string, context diagnostic.Context) (string, *diagnostic.Error) {
	var title strings.Builder
	space := true
	for index := range len(raw) {
		character := raw[index]
		if character == '\t' || character == '\r' || character == '\n' {
			character = ' '
		}
		if character == ' ' && space {
			continue
		}
		title.WriteByte(character)
		space = character == ' '
	}
	normalized := strings.TrimSuffix(title.String(), " ")
	if problem := manifest.ValidateSectionTitle(normalized, "/sections/title", context); problem != nil {
		return "", problem
	}
	return normalized, nil
}

// CreateSection принимает уже нормализованное и подтверждённое имя,
// проверяет manifest.ValidateSectionTitle, добавляет секцию в конец либо
// использует уже существующую с точно тем же именем. Включает Sectioned;
// неперенесённые уроки остаются видимыми и блокируют экспорт через Refresh.
// Пустая секция сохраняется в рабочем снимке до распределения.
func CreateSection(value *State, title string) (*State, []*diagnostic.Error) {
	if value == nil {
		return Refresh(nil)
	}
	if problem := manifest.ValidateSectionTitle(title, "/sections/title", diagnostic.Context{}); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate := Clone(value)
	candidate.Sectioned = true
	for _, section := range candidate.Sections {
		if section.Title == title {
			return Refresh(candidate)
		}
	}
	candidate.Sections = append(candidate.Sections, SectionOrder{Title: title})
	return Refresh(candidate)
}

// MoveLesson строит кандидат с одной новой Section и вызывает Refresh.
// Ненулевое имя уже проверено/нормализовано в зависимости от источника;
// отсутствующая секция добавляется через CreateSection. nil оставляет урок
// без секции. При первом ненулевом назначении включает Sectioned; не распределяет
// остальные уроки автоматически. Вес, метаданные и имя файла сохраняет;
// совпадения в новой группе видны в возвращённых ошибках всего состава.
func MoveLesson(value *State, fileName string, section *string) (*State, []*diagnostic.Error) {
	if value == nil {
		return Refresh(nil)
	}
	index, problem := findItem(value, fileName)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate := Clone(value)
	candidate.Items[index].Section = nil
	if section != nil {
		title := *section
		candidate.Items[index].Section = &title
		candidate.Sectioned = true
		registered := false
		for _, existing := range candidate.Sections {
			if existing.Title == title {
				registered = true
				break
			}
		}
		if !registered {
			return CreateSection(candidate, title)
		}
	}
	return Refresh(candidate)
}

// UseFlatPlacement завершает явно выбранное размещение без секций, только
// если Section уже nil у всех уроков. Убирает пустые рабочие секции и сбрасывает
// Sectioned; не снимает назначения автоматически. Проверяет новую общую
// группу через Refresh, где одинаковые прежние веса могут стать конфликтами.
func UseFlatPlacement(value *State) (*State, []*diagnostic.Error) {
	if value == nil {
		return Refresh(nil)
	}
	var assigned []diagnostic.Context
	for _, item := range value.Items {
		if item.Section != nil {
			context := item.Context
			context.FileName = item.Entry.FileName
			if item.Entry.ID != "" {
				context.LessonID = item.Entry.ID
			}
			assigned = append(assigned, context)
		}
	}
	if len(assigned) != 0 {
		return nil, []*diagnostic.Error{{
			Code: CodeMixedSections, Reason: "Перед выбором размещения без секций необходимо явно снять назначения всех уроков.",
			Context: assigned[0], Related: assigned,
		}}
	}
	candidate := Clone(value)
	candidate.Sections = nil
	candidate.Sectioned = false
	return Refresh(candidate)
}

// SetLessonWeight проверяет raw через order.Validate, строит кандидат
// с точной строкой веса одной записи и вызывает Refresh. Применим для
// последовательного ввода весов выбранной группы, в том числе после импорта.
// Временные совпадения допустимы в рабочем кандидате, но запрещают экспорт.
// Отсутствие, пустая строка и ноль не подставляются вместо нового веса.
func SetLessonWeight(value *State, fileName string, raw string) (*State, []*diagnostic.Error) {
	if value == nil {
		return Refresh(nil)
	}
	index, problem := findItem(value, fileName)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	context := value.Items[index].Context
	context.FileName = value.Items[index].Entry.FileName
	if value.Items[index].Entry.ID != "" {
		context.LessonID = value.Items[index].Entry.ID
	}
	if problem := order.Validate(raw, context); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate := Clone(value)
	candidate.Items[index].Entry.Order = &raw
	return Refresh(candidate)
}
