// Package catalog проектирует размещение уроков по ТЗ 1.0, разделы 7–8,
// 11–14, 16 и 19. Входные метаданные передаёт app из проверенного JSON 1.0;
// DSL и пути не используются для угадывания курса, юнита или секции.
// Пакет не читает файлы, не ведёт диалог, не меняет lesson, ID или имена ZIP.
// Уникальность ID/имён и связи с файлами проверяет module; ограничения
// схемы — manifest и lesson. Сравнение весов принадлежит order.
// Все операции возвращают независимые снимки, не меняя переданные указатели
// и срезы. nil State означает, что операция не применена; ненулевой State
// вместе с ошибками означает редактируемый, но запрещённый к записи план.
// app показывает кандидата и получает нужные подтверждения до его принятия;
// отмена сохраняет прежний снимок. Ошибки не означают частичной записи.
package catalog

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

// State — рабочее размещение, допускающее незавершённое распределение.
// Отдельная модель нужна для пустых создаваемых секций, отсутствующих весов,
// контекстов и временных весов секций; эти сведения не входят в JSON.
type State struct {
	// CourseTitle — введённое пользователем название единственного курса.
	CourseTitle string
	// UnitTitle — название юнита; nil означает отсутствие объекта unit.
	UnitTitle *string
	// Sectioned — намерение распределить все уроки по секциям; при true
	// Item.Section == nil означает незавершённое распределение.
	Sectioned bool
	// Items — записи всех уроков в порядке текущего превью; содержимого нод нет.
	Items []Item
	// Sections — уникальные имена секций в текущем порядке отображения.
	// Пустые секции допустимы во время редактирования и не экспортируются.
	Sections []SectionOrder
}

// Item — существующая запись манифеста и её внутреннее размещение.
type Item struct {
	// Entry — единственное представление восьми полей записи; без их копий.
	Entry manifest.Entry
	// Section — точное имя секции либо nil для непосредственного размещения.
	Section *string
	// RelativePath — путь относительно выбранного входного каталога, переданный
	// после нормализации filetree; объясняет порядок синтеза новой сборки.
	// У импортированных записей неизвестный путь остаётся пустым.
	RelativePath string
	// Context — исходный JSON либо архив и запись ZIP, без выдуманных координат.
	Context diagnostic.Context
}

// SectionOrder — имя и временный вес; порядок массива сохраняет ручной выбор.
type SectionOrder struct {
	// Title — имя секции с учётом регистра, без дублирующего объекта manifest.Section.
	Title string
	// Weight — последний заданный временный вес либо nil; в JSON не попадает.
	// После загрузки архива веса неизвестны, но порядок массива известен.
	Weight *string
}

// Clone копирует State, срезы и все строковые указатели в записях и секциях.
// Нужен для независимых кандидатов, отказа от изменения и сравнения снимков.
func Clone(value *State) *State {
	if value == nil {
		return nil
	}
	cloneString := func(value *string) *string {
		if value == nil {
			return nil
		}
		copied := *value
		return &copied
	}
	result := *value
	result.UnitTitle = cloneString(value.UnitTitle)
	if value.Items != nil {
		result.Items = make([]Item, len(value.Items))
		copy(result.Items, value.Items)
		for index := range result.Items {
			result.Items[index].Entry.Order = cloneString(value.Items[index].Entry.Order)
			result.Items[index].Entry.Subtitle = cloneString(value.Items[index].Entry.Subtitle)
			result.Items[index].Section = cloneString(value.Items[index].Section)
		}
	}
	if value.Sections != nil {
		result.Sections = make([]SectionOrder, len(value.Sections))
		copy(result.Sections, value.Sections)
		for index := range result.Sections {
			result.Sections[index].Weight = cloneString(value.Sections[index].Weight)
		}
	}
	return &result
}

// findItem находит ровно одну запись по назначенному техническому имени.
// Не ищет урок по ID: это уже сделал module с правилами identity.
// Отсутствие или неоднозначный селектор дают CodeSelection, без изменения модели.
func findItem(value *State, fileName string) (int, *diagnostic.Error) {
	selected := -1
	var related []diagnostic.Context
	if value != nil && fileName != "" {
		for index, item := range value.Items {
			if item.Entry.FileName != fileName {
				continue
			}
			selected = index
			context := item.Context
			context.FileName = item.Entry.FileName
			if item.Entry.ID != "" {
				context.LessonID = item.Entry.ID
			}
			related = append(related, context)
		}
	}
	if len(related) == 1 {
		return selected, nil
	}
	problem := &diagnostic.Error{
		Code:    CodeSelection,
		Reason:  "Техническое имя не выбирает ни одной записи урока.",
		Context: diagnostic.Context{FileName: fileName},
		Details: []diagnostic.Detail{{Label: "Техническое имя", Value: fileName}},
	}
	if len(related) > 1 {
		problem.Reason = "Техническое имя выбирает несколько записей урока."
		problem.Context = related[0]
		problem.Related = related
	}
	return -1, problem
}
