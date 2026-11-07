package parsed

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

const (
	// CodeRead — невозможно полностью прочитать или закрыть технический JSON.
	CodeRead = "parsed.read"

	// CodeUTF8 — входные байты не являются корректным UTF-8.
	CodeUTF8 = "parsed.utf8"

	// CodeJSON — нарушен синтаксис единственного JSON-документа.
	CodeJSON = "parsed.json"

	// CodeVersion — formatVersion не равен поддерживаемому model.FormatVersion1.
	CodeVersion = "parsed.version"

	// CodeContract — отсутствует поле, неверен тип/null или инвариант Contract.
	CodeContract = "parsed.contract"

	// CodeMissingID — обязательный ID урока отсутствует либо равен null.
	CodeMissingID = "parsed.missing_id"

	// CodeParser — обнаружен хотя бы один блокирующий признак ошибки парсера.
	CodeParser = "parsed.parser"

	// CodeParent — нарушена целостность или глубина связи parentLine.
	CodeParent = "parsed.parent"

	// CodeParentCycle — обнаружен цикл parentLine.
	CodeParentCycle = "parsed.parent_cycle"
)

// inputError создаёт существующий diagnostic.Error с русской Reason,
// известным Context и исходной Cause. Field в Details обозначает JSON-поле;
// координаты синтаксиса JSON указываются отдельно в Details и не выдаются
// за физические строки DSL. Недоступные координаты не выдумываются.
// Ошибки identity и order сохраняют свой Code и дополняются контекстом.
func inputError(code string, reason string, field string, context diagnostic.Context, cause error) *diagnostic.Error {
	result := &diagnostic.Error{Code: code, Reason: reason, Context: context, Cause: cause}
	var dependency *diagnostic.Error
	if errors.As(cause, &dependency) && dependency != nil &&
		(strings.HasPrefix(dependency.Code, "identity.") || strings.HasPrefix(dependency.Code, "order.")) {
		result.Code = dependency.Code
		result.Reason = dependency.Reason
		result.Related = append([]diagnostic.Context(nil), dependency.Related...)
		result.Details = append([]diagnostic.Detail(nil), dependency.Details...)
		// Известные сведения зависимости сохраняются, отсутствующие дополняются.
		result.Context = dependency.Context
		if result.Context.Path == "" {
			result.Context.Path = context.Path
		}
		if result.Context.FileName == "" {
			result.Context.FileName = context.FileName
		}
		if result.Context.LessonID == "" {
			result.Context.LessonID = context.LessonID
		}
		if result.Context.PageID == "" {
			result.Context.PageID = context.PageID
		}
		if result.Context.Line == 0 {
			result.Context.Line = context.Line
			if result.Context.Column == 0 {
				result.Context.Column = context.Column
			}
		} else if result.Context.Line == context.Line && result.Context.Column == 0 {
			result.Context.Column = context.Column
		}
	}
	if field != "" {
		result.Details = append(result.Details, diagnostic.Detail{Label: "Поле JSON", Value: field})
	}
	var syntax *json.SyntaxError
	var mismatch *json.UnmarshalTypeError
	var offset int64
	switch {
	case errors.As(cause, &syntax) && syntax != nil:
		offset = syntax.Offset
	case errors.As(cause, &mismatch) && mismatch != nil:
		offset = mismatch.Offset
	}
	if offset > 0 {
		result.Details = append(result.Details, diagnostic.Detail{
			Label: "Смещение в JSON (байты)", Value: strconv.FormatInt(offset, 10),
		})
	}
	return result
}
