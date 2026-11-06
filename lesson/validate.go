package lesson

import (
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// Validate координирует проверку модели и ValidateIDs, не изменяя value.
// nil даёт ошибку контракта; успех — пустой срез ошибок. Сначала возвращаются
// ошибки модели, затем ID; внутри этапа — в порядке полей схемы и массивов,
// без неопределённого обхода map. Непроверенный контейнер не порождает каскад ошибок.
// Context сохраняет известный путь или техническое имя, дополняется ID;
// точное место поля указывается в Details как JSON Pointer.
// Отсутствующие обязательные скалярные поля внешнего JSON проверяет ValidateJSON:
// в корректно созданной Go-модели они всегда сериализуются без omitempty.
// Не проверяет историю ревизий, источники данных, DSL и вычисление checkable.
func Validate(value *Lesson, context diagnostic.Context) []*diagnostic.Error {
	if value == nil {
		return []*diagnostic.Error{contractError(CodeContract, "Урок не задан", "", context, nil)}
	}
	if value.Document.ID != "" {
		context.LessonID = value.Document.ID
	}
	problems := []*diagnostic.Error{}
	if value.FormatVersion != FormatVersion {
		problems = append(problems, contractError(CodeContract, "Поддерживается только версия контракта урока 1.0", "/formatVersion", context, nil))
	}
	problems = append(problems, validateDocument(value.Document, context)...)
	problems = append(problems, ValidateIDs(value, context)...)
	return problems
}

// validateDocument проверяет Version через validateInteger с минимумом 1,
// LineCount с минимумом 0, дату и ненулевой срез Pages; ID проверяет ValidateIDs.
// Title и Subtitle ограничены типами схемы: нет требования непустоты,
// обрезания пробелов или повторной проверки исходных заголовков DSL.
// formatVersion проверяет Validate; страницы делегируются validatePage.
func validateDocument(value Document, context diagnostic.Context) []*diagnostic.Error {
	if value.ID != "" {
		context.LessonID = value.ID
	}
	var problems []*diagnostic.Error
	if problem := validateInteger(value.Version, 1, "/document/version", context); problem != nil {
		problems = append(problems, problem)
	}
	if problem := validateInteger(value.LineCount, 0, "/document/lineCount", context); problem != nil {
		problems = append(problems, problem)
	}
	if problem := validateCreatedAt(value.CreatedAt, "/document/createdAt", context); problem != nil {
		problems = append(problems, problem)
	}
	if value.Pages == nil {
		problems = append(problems, contractError(CodeContract, "Массив страниц не должен быть null", "/document/pages", context, nil))
		return problems
	}
	for index, page := range value.Pages {
		problems = append(problems, validatePage(page, "/document/pages/"+strconv.Itoa(index), context)...)
	}
	return problems
}

// validatePage проверяет ненулевой срез Nodes, дополняет
// Context.PageID и передаёт каждую ноду validateNode. Пустой массив допустим.
// ID проверяет ValidateIDs; названия страниц не сравниваются с документом.
func validatePage(value Page, pointer string, context diagnostic.Context) []*diagnostic.Error {
	if value.ID != "" {
		context.PageID = value.ID
	}
	if value.Nodes == nil {
		return []*diagnostic.Error{contractError(CodeContract, "Массив нод страницы не должен быть null", pointer+"/nodes", context, nil)}
	}
	var problems []*diagnostic.Error
	for index, node := range value.Nodes {
		problems = append(problems, validateNode(node, pointer+"/nodes/"+strconv.Itoa(index), context)...)
	}
	return problems
}
