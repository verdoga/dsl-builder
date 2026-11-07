package module

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

// CheckNewIDs проверяет ID входов до преобразования: LessonID и Path берутся
// app из проверенных parsed-данных и реального пути технического JSON.
// Использует готовый diagnostic.Context вместо второй модели входа.
// Пустой incoming даёт CodeEmpty; existing пуст при создании либо содержит
// проверенный архив, что является предусловием проверки новых ID.
// Координирует validateIDs и checkExistingIDs; каждый повтор среди входов —
// ошибка, включая различие регистра. Совпадение с архивом даёт CodeExistingID
// с указанием, что требуется обновление. Набор целиком отклоняется при ошибке.
func CheckNewIDs(incoming []diagnostic.Context, existing []*File) []*diagnostic.Error {
	if len(incoming) == 0 {
		return []*diagnostic.Error{{Code: CodeEmpty, Reason: "Отсутствуют входные уроки для проверки ID."}}
	}
	problems := validateIDs(incoming)
	return append(problems, checkExistingIDs(incoming, existing)...)
}

// validateIDs проверяет каждый LessonID через identity.ValidateLessonID,
// группирует корректные значения по identity.LessonKey и сообщает один
// CodeDuplicateID на каждый повтор со всеми участниками, включая первого.
// Ничего не генерирует и не меняет исходное написание. Порядок — порядок входа;
// неверные значения не объединяются в искусственную группу пустого ключа.
func validateIDs(values []diagnostic.Context) []*diagnostic.Error {
	groups := make(map[string][]diagnostic.Context, len(values))
	invalid := make([]*diagnostic.Error, len(values))
	for index, source := range values {
		if problem := identity.ValidateLessonID(source.LessonID); problem != nil {
			problem.Context = source
			invalid[index] = problem
			continue
		}
		key := identity.LessonKey(source.LessonID)
		groups[key] = append(groups[key], source)
	}
	var problems []*diagnostic.Error
	seen := make(map[string]bool, len(groups))
	for index, source := range values {
		if invalid[index] != nil {
			problems = append(problems, invalid[index])
			continue
		}
		key := identity.LessonKey(source.LessonID)
		if seen[key] {
			problems = append(problems, &diagnostic.Error{
				Code: CodeDuplicateID, Reason: "ID урока повторяется в проверяемом наборе без учёта регистра.",
				Context: source, Related: append([]diagnostic.Context(nil), groups[key]...),
				Details: []diagnostic.Detail{{Label: "ID урока", Value: source.LessonID}},
			})
		}
		seen[key] = true
	}
	return problems
}

// checkExistingIDs находит все совпадения корректных входных ID с существующими
// через identity.LessonKey. Related содержит входы и прежние файлы каждого
// конфликтующего ID. Не вызывает ReplaceFile и не выполняет скрытое обновление.
func checkExistingIDs(incoming []diagnostic.Context, existing []*File) []*diagnostic.Error {
	groups := make(map[string][]diagnostic.Context, len(incoming))
	valid := make([]bool, len(incoming))
	for index, source := range incoming {
		if identity.ValidateLessonID(source.LessonID) != nil {
			continue
		}
		valid[index] = true
		key := identity.LessonKey(source.LessonID)
		groups[key] = append(groups[key], source)
	}
	archived := make(map[string][]diagnostic.Context, len(existing))
	for _, file := range existing {
		// existing — файлы уже проверенного архива.
		if file == nil || file.value == nil {
			continue
		}
		key := identity.LessonKey(file.value.Document.ID)
		archived[key] = append(archived[key], file.Context())
	}
	var problems []*diagnostic.Error
	for index, source := range incoming {
		if !valid[index] {
			continue
		}
		key := identity.LessonKey(source.LessonID)
		if len(archived[key]) == 0 {
			continue
		}
		related := append([]diagnostic.Context(nil), groups[key]...)
		related = append(related, archived[key]...)
		problems = append(problems, &diagnostic.Error{
			Code: CodeExistingID, Reason: "ID добавляемого урока уже существует в архиве; требуется режим обновления.",
			Context: source, Related: related,
			Details: []diagnostic.Detail{{Label: "ID урока", Value: source.LessonID}},
		})
	}
	return problems
}

// FindByID проверяет lessonID через identity.ValidateLessonID и находит ровно
// один File по identity.LessonKey. Ноль совпадений — CodeMissingLesson,
// несколько — CodeAmbiguousLesson со всеми кандидатами; первый не выбирается.
// Возвращает неизменяемый файл с прежним написанием ID и техническим именем.
// nil/нулевые файлы дают CodeFile; ни один такой файл молча не пропускается.
// При любой ошибке результат File равен nil, включая ошибку другого кандидата.
func FindByID(files []*File, lessonID string, context diagnostic.Context) (*File, []*diagnostic.Error) {
	context.LessonID = lessonID
	var problems []*diagnostic.Error
	queryProblem := identity.ValidateLessonID(lessonID)
	if queryProblem != nil {
		queryProblem.Context = context
		problems = append(problems, queryProblem)
	}
	var key string
	if queryProblem == nil {
		key = identity.LessonKey(lessonID)
	}
	var matches []*File
	var related []diagnostic.Context
	for _, file := range files {
		if file == nil || file.value == nil {
			source := context
			if file != nil {
				source = file.Context()
				if source.Path == "" {
					source.Path = context.Path
				}
			}
			problems = append(problems, &diagnostic.Error{
				Code: CodeFile, Reason: "Отсутствует файл или снимок урока для поиска по ID.", Context: source,
			})
			continue
		}
		if queryProblem == nil && identity.LessonKey(file.value.Document.ID) == key {
			matches = append(matches, file)
			source := file.Context()
			if source.Path == "" {
				source.Path = context.Path
			}
			related = append(related, source)
		}
	}
	if queryProblem == nil {
		switch len(matches) {
		case 0:
			problems = append(problems, &diagnostic.Error{
				Code: CodeMissingLesson, Reason: "В модуле не найден урок с указанным ID.", Context: context,
				Details: []diagnostic.Detail{{Label: "ID урока", Value: lessonID}},
			})
		case 1:
		default:
			problems = append(problems, &diagnostic.Error{
				Code: CodeAmbiguousLesson, Reason: "Указанному ID соответствуют несколько файлов уроков.",
				Context: context, Related: related,
				Details: []diagnostic.Detail{{Label: "ID урока", Value: lessonID}},
			})
		}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return matches[0], nil
}
