package manifest

import (
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/order"
)

// validateEntry координирует проверки восьми полей одной записи.
// ID проверяет существующая identity.ValidateLessonID, имя — identity.ParseFileName;
// узкие проверки DSL, назначение номеров и сравнение ID не выполняются.
// Ненулевой Order проверяет существующая order.Validate без изменения строки;
// allowNullOrder определяется длиной содержащего массива, а не всего модуля.
// Title проверяет ValidateTitle, даты и целые — функции validate_values.go;
// Subtitle допускает null и любую корректную UTF-8 строку без minLength.
// Ошибки зависимостей сохраняют Code/Reason/Details и получают контекст
// записи и JSON Pointer; собственный парсер ID или арифметика веса не нужны.
func validateEntry(value Entry, allowNullOrder bool, pointer string, context diagnostic.Context) []*diagnostic.Error {
	if value.FileName != "" {
		context.FileName = value.FileName
	}
	if value.ID != "" {
		context.LessonID = value.ID
	}
	var problems []*diagnostic.Error
	if problem := identity.ValidateLessonID(value.ID); problem != nil {
		problem.Context = context
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "JSON Pointer", Value: pointer + "/id"})
		problems = append(problems, problem)
	}
	if _, problem := identity.ParseFileName(value.FileName); problem != nil {
		problem.Context = context
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "JSON Pointer", Value: pointer + "/fileName"})
		problems = append(problems, problem)
	}
	if value.Order == nil {
		if !allowNullOrder {
			problems = append(problems, &diagnostic.Error{
				Code:    CodeNullOrder,
				Reason:  "Вес null допустим только для единственного урока в массиве lessons.",
				Context: context,
				Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer + "/order"}},
			})
		}
	} else if problem := order.Validate(*value.Order, context); problem != nil {
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "JSON Pointer", Value: pointer + "/order"})
		problems = append(problems, problem)
	}
	if problem := ValidateTitle(value.Title, pointer+"/title", context); problem != nil {
		problems = append(problems, problem)
	}
	if value.Subtitle != nil && !utf8.ValidString(*value.Subtitle) {
		problems = append(problems, &diagnostic.Error{
			Code:    CodeShape,
			Reason:  "Подзаголовок должен быть корректной строкой UTF-8.",
			Context: context,
			Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer + "/subtitle"}},
		})
	}
	if problem := validateInteger(value.LineCount, 0, pointer+"/lineCount", context); problem != nil {
		problems = append(problems, problem)
	}
	if problem := validateCreatedAt(value.CreatedAt, pointer+"/createdAt", context); problem != nil {
		problems = append(problems, problem)
	}
	if problem := validateInteger(value.Version, 1, pointer+"/version", context); problem != nil {
		problems = append(problems, problem)
	}
	return problems
}
