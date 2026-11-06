package textdata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

const (
	// CodeUnsupportedType — запрошены собственные данные чужого типа ноды.
	CodeUnsupportedType = "textdata.unsupported_type"
	// CodeInput — локальные исходные строки не соответствуют назначению входа.
	CodeInput = "textdata.input"
	// CodeMissingValue — требуемый семантический текст элемента неизвестен.
	CodeMissingValue = "textdata.missing_value"
	// CodeExampleSeparator — неоднозначное или пустое разделение примера.
	CodeExampleSeparator = "textdata.example_separator"
	// CodeInstructionHint — невозможно однозначно выделить подсказку инструкции.
	CodeInstructionHint = "textdata.instruction_hint"
)

// dataError создаёт существующий diagnostic.Error с русской причиной,
// переданным кодом и контекстом; полный текст блока в диагностику не копирует.
// Context.Path — путь технического JSON от вызывающего кода, не DSL FilePath.
// При наличии line указывает исходный Number; при наличии element указывает
// его Start в Unicode-колонках. nil сохраняет известный внешний контекст.
// Для неоднозначности вызывающий код добавляет все связанные места в Related.
// Источник одного включения не смешивается с другим по совпавшим номерам строк.
func dataError(code string, reason string, line *model.Line, element *model.Element, context diagnostic.Context) *diagnostic.Error {
	if line != nil {
		context.Line = line.Number
	}
	if element != nil {
		context.Column = element.Start
	}
	return &diagnostic.Error{
		Code:    code,
		Reason:  reason,
		Context: context,
	}
}
