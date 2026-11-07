package catalog

import (
	"encoding/json"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

// EntryFromLesson копирует ID, Title, Subtitle, LineCount, CreatedAt и Version
// из окончательного Document и добавляет назначенные fileName и weight.
// Не вычисляет ревизию, не меняет дату, регистр ID или само значение weight.
// nil Subtitle остаётся null; числовые json.Number копируются без float64.
// Предусловие: урок проверен lesson, имя выдано module, веса проверяет catalog.
// Полное соответствие схемы записи проверяется manifest при экспорте.
func EntryFromLesson(value lesson.Document, fileName string, weight *string) manifest.Entry {
	entry := manifest.Entry{
		ID:        value.ID,
		FileName:  fileName,
		Title:     value.Title,
		LineCount: value.LineCount,
		CreatedAt: value.CreatedAt,
		Version:   value.Version,
	}
	if value.Subtitle != nil {
		subtitle := *value.Subtitle
		entry.Subtitle = &subtitle
	}
	if weight != nil {
		raw := *weight
		entry.Order = &raw
	}
	return entry
}

// SyncLesson синхронизирует одну запись с окончательным уроком после revision
// и решений по названиям, включая отказ пользователя. Сохраняет Section,
// Order и FileName; ID обязан точно совпадать с уже сохранённым написанием.
// Не подставляет метаданные неподтверждённого кандидата; при отсутствии
// содержательных изменений app передаёт прежний Document, включая LineCount.
// Координирует findItem, EntryFromLesson и Refresh на независимой копии.
// Несовпадение ID даёт CodeIdentity; урок и другие записи не изменяются.
func SyncLesson(value *State, fileName string, document lesson.Document) (*State, []*diagnostic.Error) {
	if value == nil {
		return Refresh(nil)
	}
	index, problem := findItem(value, fileName)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	item := value.Items[index]
	if item.Entry.ID != document.ID {
		context := item.Context
		context.FileName = item.Entry.FileName
		if item.Entry.ID != "" {
			context.LessonID = item.Entry.ID
		}
		return nil, []*diagnostic.Error{{
			Code:    CodeIdentity,
			Reason:  "ID окончательного урока должен точно совпадать с сохранённым ID записи.",
			Context: context,
			Details: []diagnostic.Detail{
				{Label: "Сохранённый ID", Value: item.Entry.ID},
				{Label: "ID окончательного урока", Value: document.ID},
			},
		}}
	}
	candidate := Clone(value)
	candidate.Items[index].Entry = EntryFromLesson(document, item.Entry.FileName, item.Entry.Order)
	return Refresh(candidate)
}

// ProposePlacement описывает различия section/order нового технического JSON
// с текущей записью, не применяя их. Входной section == nil предлагает
// непосредственное размещение, но не выполняет его; nil weight не предлагает
// удаление старого веса. Строковые веса сравниваются точно: 01 → 1 меняет JSON.
// Проверяет ненулевые значения через manifest.ValidateSectionTitle и
// order.Validate. Возвращает отдельное Change для каждого различия, чтобы
// app мог независимо показать и подтвердить каждое «было → станет».
// Принятые изменения применяются MoveLesson и SetLessonWeight;
// при отказе текущий снимок и размещение остаются прежними.
func ProposePlacement(value *State, fileName string, section *string, weight *string) ([]Change, []*diagnostic.Error) {
	if value == nil {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует рабочий снимок размещения."}}
	}
	index, problem := findItem(value, fileName)
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	item := value.Items[index]
	context := item.Context
	context.FileName = item.Entry.FileName
	if item.Entry.ID != "" {
		context.LessonID = item.Entry.ID
	}
	var problems []*diagnostic.Error
	if section != nil {
		if problem := manifest.ValidateSectionTitle(*section, "/document/metadata/section", context); problem != nil {
			problems = append(problems, problem)
		}
	}
	if weight != nil {
		if problem := order.Validate(*weight, context); problem != nil {
			problems = append(problems, problem)
		}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	var changes []Change
	if (item.Section == nil) != (section == nil) ||
		item.Section != nil && section != nil && *item.Section != *section {
		before, _ := json.Marshal(item.Section)
		after, _ := json.Marshal(section)
		changes = append(changes, Change{
			FileName: item.Entry.FileName, Field: "section", Before: before, After: after,
		})
	}
	if weight != nil && (item.Entry.Order == nil || *item.Entry.Order != *weight) {
		before, _ := json.Marshal(item.Entry.Order)
		after, _ := json.Marshal(weight)
		changes = append(changes, Change{
			FileName: item.Entry.FileName, Field: "order", Before: before, After: after,
		})
	}
	return changes, nil
}
