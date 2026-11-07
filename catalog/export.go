package catalog

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

// Export формирует готовый Manifest без изменения State. Координирует
// Validate, exportContents, exportCourse и manifest.Validate. При любой ошибке
// возвращает nil Manifest. Не сортирует молча и не пишет файлы.
// formatVersion берёт из manifest.FormatVersion; createdAt передаёт app:
// старую дату для сопоставления, новое UTC-время при фактической новой записи.
// Само изменение этой даты не повод записывать архив; решение принимает app.
// Экспорт не меняет даты/версии записей уроков, имена, ID и ведущие нули весов.
func Export(value *State, createdAt string, context diagnostic.Context) (*manifest.Manifest, []*diagnostic.Error) {
	if problems := Validate(value); len(problems) != 0 {
		return nil, problems
	}
	lessons, sections := exportContents(value)
	result := &manifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		CreatedAt:     createdAt,
		Course:        exportCourse(value, lessons, sections),
	}
	if problems := manifest.Validate(*result, context); len(problems) != 0 {
		return nil, problems
	}
	return result, nil
}

// exportContents создаёт ненулевые массивы: либо lessons без sections,
// либо sections с пустым lessons. Пустые рабочие секции пропускает.
// Сохраняет текущий порядок, все восемь полей Entry и явный null Subtitle;
// внутренние пути, веса секций и признаки рабочего состояния не переносятся.
func exportContents(value *State) ([]manifest.Entry, []manifest.Section) {
	lessons := make([]manifest.Entry, 0)
	sections := make([]manifest.Section, 0)
	value = Clone(value)
	if value == nil {
		return lessons, sections
	}
	if !value.Sectioned {
		for _, item := range value.Items {
			lessons = append(lessons, item.Entry)
		}
		return lessons, sections
	}
	for _, section := range value.Sections {
		entries := make([]manifest.Entry, 0)
		for _, item := range value.Items {
			if item.Section != nil && *item.Section == section.Title {
				entries = append(entries, item.Entry)
			}
		}
		if len(entries) != 0 {
			sections = append(sections, manifest.Section{Title: section.Title, Lessons: entries})
		}
	}
	return lessons, sections
}

// exportCourse выбирает ровно одну форму схемы: Course.Unit при UnitTitle
// либо указатели Course.Lessons/Course.Sections при отсутствии юнита.
// Указатели никогда не указывают на nil-срез; Unit имеет оба ненулевых массива.
func exportCourse(value *State, lessons []manifest.Entry, sections []manifest.Section) manifest.Course {
	if lessons == nil {
		lessons = make([]manifest.Entry, 0)
	}
	if sections == nil {
		sections = make([]manifest.Section, 0)
	}
	course := manifest.Course{Title: value.CourseTitle}
	if value.UnitTitle != nil {
		course.Unit = &manifest.Unit{
			Title:    *value.UnitTitle,
			Lessons:  lessons,
			Sections: sections,
		}
	} else {
		course.Lessons = &lessons
		course.Sections = &sections
	}
	return course
}

// readPlacement переносит выбранную ветвь course/unit в рабочий снимок
// без исправления значений, объединения повторов и изменения порядка.
// Предусловие: форма и схема уже проверены manifest; вызывают Import
// и ValidateManifest. Каждый объект sections сохраняется для обнаружения дублей.
func readPlacement(value manifest.Manifest, context diagnostic.Context) *State {
	result := &State{
		CourseTitle: value.Course.Title,
		Items:       make([]Item, 0),
		Sections:    make([]SectionOrder, 0),
	}
	var lessons []manifest.Entry
	var sections []manifest.Section
	if value.Course.Unit != nil {
		result.UnitTitle = &value.Course.Unit.Title
		lessons = value.Course.Unit.Lessons
		sections = value.Course.Unit.Sections
	} else {
		if value.Course.Lessons != nil {
			lessons = *value.Course.Lessons
		}
		if value.Course.Sections != nil {
			sections = *value.Course.Sections
		}
	}
	result.Sectioned = len(sections) != 0
	appendEntries := func(entries []manifest.Entry, section *string) {
		for _, entry := range entries {
			entryContext := context
			entryContext.FileName = entry.FileName
			entryContext.LessonID = entry.ID
			result.Items = append(result.Items, Item{
				Entry:   entry,
				Section: section,
				Context: entryContext,
			})
		}
	}
	appendEntries(lessons, nil)
	for _, section := range sections {
		result.Sections = append(result.Sections, SectionOrder{Title: section.Title})
		appendEntries(section.Lessons, &section.Title)
	}
	return Clone(result)
}
