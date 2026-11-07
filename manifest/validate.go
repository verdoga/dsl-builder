package manifest

import (
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// Validate проверяет готовую модель по фиксированной схеме: версию, даты,
// скаляры, форму course, непустое содержимое уровня, массивы и uniqueItems.
// Это небольшой координатор validateCourse и проверок корневых значений.
// Не проверяет связи ZIP, уникальность отдельных ID/имён/весов/названий,
// сортировку, смешанное размещение и обязательность итогового веса по ТЗ.
// Для прочитанного JSON предварительно требуется успешный ValidateShape.
// Возвращает все независимые ошибки в порядке полей модели и индексов массивов;
// после ошибки формы не обращается к отсутствующим ветвям. Успех — пустой срез.
// context задаёт известный архив и manifest.json; внутри записи сохраняется
// Path, а FileName и LessonID дополняются её значениями, если они известны.
// Details всегда содержит JSON Pointer поля; координаты DSL не выдумываются.
func Validate(value Manifest, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if value.FormatVersion != FormatVersion {
		problems = append(problems, &diagnostic.Error{
			Code: CodeVersion, Reason: "Поддерживается только версия формата манифеста 1.0.", Context: context,
			Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: "/formatVersion"}},
		})
	}
	if problem := validateCreatedAt(value.CreatedAt, "/createdAt", context); problem != nil {
		problems = append(problems, problem)
	}
	return append(problems, validateCourse(value.Course, "/course", context)...)
}

// validateCourse проверяет название и ровно одну форму: unit без lessons/sections
// либо обе непосредственные коллекции без unit; делегирует проверку содержимого.
func validateCourse(value Course, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if problem := ValidateTitle(value.Title, pointer+"/title", context); problem != nil {
		problems = append(problems, problem)
	}
	if value.Unit != nil {
		if value.Lessons != nil || value.Sections != nil {
			problems = append(problems, &diagnostic.Error{
				Code: CodeContents, Reason: "Курс с unit не должен содержать непосредственные lessons или sections.", Context: context,
				Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
			})
		}
		return append(problems, validateUnit(*value.Unit, pointer+"/unit", context)...)
	}
	if value.Lessons != nil && value.Sections != nil {
		return append(problems, validateContents(*value.Lessons, *value.Sections, pointer, context)...)
	}
	problems = append(problems, &diagnostic.Error{
		Code: CodeContents, Reason: "Курс без unit должен содержать оба массива lessons и sections.", Context: context,
		Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
	})
	if value.Lessons != nil {
		problems = append(problems, validateEntries(*value.Lessons, 0, pointer+"/lessons", context)...)
	}
	if value.Sections != nil {
		problems = append(problems, validateSections(*value.Sections, pointer+"/sections", context)...)
	}
	return problems
}

// validateUnit проверяет название и делегирует validateContents обязательные массивы.
func validateUnit(value Unit, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if problem := ValidateTitle(value.Title, pointer+"/title", context); problem != nil {
		problems = append(problems, problem)
	}
	return append(problems, validateContents(value.Lessons, value.Sections, pointer, context)...)
}

// validateContents проверяет ненулевые массивы и хотя бы один элемент суммарно,
// затем передаёт lessons и sections профильным проверкам. Одновременная
// непустота массивов допускается схемой; запрет смешения принадлежит catalog.
func validateContents(lessons []Entry, sections []Section, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if lessons != nil && sections != nil && len(lessons) == 0 && len(sections) == 0 {
		problems = append(problems, &diagnostic.Error{
			Code: CodeContents, Reason: "Курс или юнит должен содержать хотя бы один урок или секцию.", Context: context,
			Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
		})
	}
	problems = append(problems, validateEntries(lessons, 0, pointer+"/lessons", context)...)
	return append(problems, validateSections(sections, pointer+"/sections", context)...)
}

// validateSections проверяет ненулевой массив, каждую секцию и uniqueItems;
// пустой массив разрешён. Одинаковый title сам по себе здесь не ошибка.
func validateSections(values []Section, pointer string, context diagnostic.Context) []*diagnostic.Error {
	if values == nil {
		return []*diagnostic.Error{{
			Code: CodeShape, Reason: "Массив sections не должен быть null.", Context: context,
			Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
		}}
	}
	var problems []*diagnostic.Error
	var checked []Section
	var indices []int
	for index, section := range values {
		sectionProblems := validateSection(section, pointer+"/"+strconv.Itoa(index), context)
		problems = append(problems, sectionProblems...)
		valid := true
		for _, problem := range sectionProblems {
			if problem.Code != CodeUniqueItems {
				valid = false
			}
		}
		if valid {
			checked = append(checked, section)
			indices = append(indices, index)
		}
	}
	duplicates := validateUniqueSections(checked, pointer, context)
	for _, problem := range duplicates {
		for index := range problem.Details {
			filtered, _ := strconv.Atoi(problem.Details[index].Value[len(pointer)+1:])
			problem.Details[index].Value = pointer + "/" + strconv.Itoa(indices[filtered])
		}
	}
	return append(problems, duplicates...)
}

// validateSection проверяет имя и непустое lessons через профильные функции.
func validateSection(value Section, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if problem := ValidateSectionTitle(value.Title, pointer+"/title", context); problem != nil {
		problems = append(problems, problem)
	}
	return append(problems, validateEntries(value.Lessons, 1, pointer+"/lessons", context)...)
}

// validateEntries проверяет ненулевой массив, минимум minimumItems (0 либо 1),
// записи и uniqueItems. При длине от двух null order запрещён у каждой записи;
// при длине один null допустим. Отдельные одинаковые числовые веса не ищет.
func validateEntries(values []Entry, minimumItems int, pointer string, context diagnostic.Context) []*diagnostic.Error {
	if values == nil {
		return []*diagnostic.Error{{
			Code: CodeShape, Reason: "Массив lessons не должен быть null.", Context: context,
			Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
		}}
	}
	var problems []*diagnostic.Error
	if len(values) < minimumItems {
		problems = append(problems, &diagnostic.Error{
			Code: CodeContents, Reason: "Секция должна содержать хотя бы один урок.", Context: context,
			Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
		})
	}
	var checked []Entry
	var indices []int
	for index, entry := range values {
		entryProblems := validateEntry(entry, len(values) == 1, pointer+"/"+strconv.Itoa(index), context)
		problems = append(problems, entryProblems...)
		if len(entryProblems) == 0 {
			checked = append(checked, entry)
			indices = append(indices, index)
		}
	}
	duplicates := validateUniqueEntries(checked, pointer, context)
	for _, problem := range duplicates {
		for index := range problem.Details {
			filtered, _ := strconv.Atoi(problem.Details[index].Value[len(pointer)+1:])
			problem.Details[index].Value = pointer + "/" + strconv.Itoa(indices[filtered])
		}
	}
	return append(problems, duplicates...)
}
