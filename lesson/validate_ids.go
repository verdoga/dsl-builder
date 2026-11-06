package lesson

import (
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

// ValidateIDs проверяет ID урока через identity.ValidateLessonID v2, затем
// синтаксис ID страниц и нод, принадлежность
// страниц и нод identity.LessonKey(Document.ID) и уникальность полных ID
// внутри одного урока. Проверяет страницы и все уровни дерева нод.
// nil ошибочен; успешный результат — пустой срез. Исходный урок не изменяет.
// Непоследовательные номера допустимы; номера не преобразуются в int.
// Не сравнивает базовые номера с игнорированием суффикса и не проверяет
// ревизию суффикса относительно Document.Version — это зона revision.
func ValidateIDs(value *Lesson, context diagnostic.Context) []*diagnostic.Error {
	if value == nil {
		return []*diagnostic.Error{contractError(CodeContract, "Урок не задан", "", context, nil)}
	}
	if value.Document.ID != "" {
		context.LessonID = value.Document.ID
	}
	problems := []*diagnostic.Error{}
	lessonKey := ""
	if problem := identity.ValidateLessonID(value.Document.ID); problem != nil {
		problem.Context = context
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: "/document/id"})
		problems = append(problems, problem)
	} else {
		lessonKey = identity.LessonKey(value.Document.ID)
	}
	index := &idIndex{pages: make(map[string]string), nodes: make(map[string]string)}
	for i, page := range value.Document.Pages {
		pageContext := context
		if page.ID != "" {
			pageContext.PageID = page.ID
		}
		pointer := "/document/pages/" + strconv.Itoa(i)
		problems = append(problems, validatePageID(page.ID, lessonKey, index, pointer+"/id", pageContext)...)
		for j, node := range page.Nodes {
			problems = append(problems, validateNodeIDs(node, lessonKey, index, pointer+"/nodes/"+strconv.Itoa(j), pageContext)...)
		}
	}
	return problems
}

// idIndex хранит первые места полных ID для диагностики повторов одного урока.
// Состояние существует только во время ValidateIDs и не сериализуется.
type idIndex struct {
	// pages — полный ID страницы и JSON Pointer первого вхождения.
	pages map[string]string

	// nodes — полный ID ноды и JSON Pointer первого вхождения во всех страницах.
	nodes map[string]string
}

// validatePageID делегирует разбор identity.ParsePageID, проверяет префикс
// через validatePagePrefix и регистрирует полный ID через checkUniqueID.
// Ошибку identity дополняет Context и указателем поля, сохраняя исходный Code.
func validatePageID(value string, lessonKey string, index *idIndex, pointer string, context diagnostic.Context) []*diagnostic.Error {
	parts, problem := identity.ParsePageID(value)
	if problem != nil {
		problem.Context = context
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: pointer})
		return []*diagnostic.Error{problem}
	}
	var problems []*diagnostic.Error
	if problem := validatePagePrefix(parts, lessonKey, pointer, context); problem != nil {
		problems = append(problems, problem)
	}
	if problem := checkUniqueID(value, index.pages, pointer, context); problem != nil {
		problems = append(problems, problem)
	}
	return problems
}

// validatePagePrefix сравнивает разобранный Prefix с ключом содержащего урока.
func validatePagePrefix(parts identity.PageIDParts, lessonKey string, pointer string, context diagnostic.Context) *diagnostic.Error {
	if lessonKey == "" || parts.Prefix == lessonKey {
		return nil
	}
	problem := contractError(CodeIDPrefix, "ID страницы принадлежит другому уроку", pointer, context, nil)
	problem.Details = append(problem.Details,
		diagnostic.Detail{Label: "Префикс ID", Value: parts.Prefix},
		diagnostic.Detail{Label: "Ключ урока", Value: lessonKey})
	return problem
}

// validateNodeID делегирует разбор identity.ParseNodeID, проверяет префикс
// через validateNodePrefix и регистрирует полный ID через checkUniqueID.
// Форматирование и выдача новых ID не выполняются.
func validateNodeID(value string, lessonKey string, index *idIndex, pointer string, context diagnostic.Context) []*diagnostic.Error {
	parts, problem := identity.ParseNodeID(value)
	if problem != nil {
		problem.Context = context
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: pointer})
		return []*diagnostic.Error{problem}
	}
	var problems []*diagnostic.Error
	if problem := validateNodePrefix(parts, lessonKey, pointer, context); problem != nil {
		problems = append(problems, problem)
	}
	if problem := checkUniqueID(value, index.nodes, pointer, context); problem != nil {
		problems = append(problems, problem)
	}
	return problems
}

// validateNodePrefix сравнивает разобранный Prefix с ключом содержащего урока.
func validateNodePrefix(parts identity.NodeIDParts, lessonKey string, pointer string, context diagnostic.Context) *diagnostic.Error {
	if lessonKey == "" || parts.Prefix == lessonKey {
		return nil
	}
	problem := contractError(CodeIDPrefix, "ID ноды принадлежит другому уроку", pointer, context, nil)
	problem.Details = append(problem.Details,
		diagnostic.Detail{Label: "Префикс ID", Value: parts.Prefix},
		diagnostic.Detail{Label: "Ключ урока", Value: lessonKey})
	return problem
}

// checkUniqueID сравнивает полные проверенные строки точным равенством;
// при повторе возвращает ошибку с указателями первого и текущего вхождений.
// При первом вхождении запоминает pointer в переданном локальном индексе.
func checkUniqueID(value string, seen map[string]string, pointer string, context diagnostic.Context) *diagnostic.Error {
	if first, exists := seen[value]; exists {
		problem := contractError(CodeDuplicateID, "Полный ID повторяется в уроке", pointer, context, nil)
		problem.Details = append(problem.Details,
			diagnostic.Detail{Label: "Первое поле JSON", Value: first},
			diagnostic.Detail{Label: "ID", Value: value})
		return problem
	}
	seen[value] = pointer
	return nil
}

// validateNodeIDs проверяет ID одной ноды и её потомков в порядке дерева,
// используя один общий индекс для всех страниц урока.
func validateNodeIDs(value Node, lessonKey string, index *idIndex, pointer string, context diagnostic.Context) []*diagnostic.Error {
	problems := validateNodeID(value.ID, lessonKey, index, pointer+"/id", context)
	for i, node := range value.Nodes {
		problems = append(problems, validateNodeIDs(node, lessonKey, index, pointer+"/nodes/"+strconv.Itoa(i), context)...)
	}
	return problems
}
