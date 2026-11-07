package exercisedata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

const (
	// CodeKind — неподдерживаемый тип упражнения, включая multifill.
	CodeKind = "exercisedata.kind"
	// CodeSource — непригодная локальная форма входа или отсутствующее Value.
	CodeSource = "exercisedata.source"
	// CodeCollection — неверный разделитель или недостаточное число элементов.
	CodeCollection = "exercisedata.collection"
	// CodeBinding — осиротевший, повторный или неверно расположенный answer/hint.
	CodeBinding = "exercisedata.binding"
	// CodeAnswer — пустые значения либо недопустимое число правильных вариантов.
	CodeAnswer = "exercisedata.answer"
	// CodeIncompleteAnswers — ответы заданы только у части items.
	CodeIncompleteAnswers = "exercisedata.incomplete_answers"
	// CodeReference — отсутствующая, неоднозначная или недопустимая ссылка ответа.
	CodeReference = "exercisedata.reference"
	// CodeContradiction — противоречие признаков выбора или невозможный порядок.
	CodeContradiction = "exercisedata.contradiction"
)

// sourceContext дополняет базовый контекст исходными Number и Element.Start;
// nil element оставляет Column=0. Не подставляет document.filePath вместо Path
// и не заменяет физическую строку позицией в проекции или номером ноды.
func sourceContext(base diagnostic.Context, line model.Line, element *model.Element) diagnostic.Context {
	base.Line = line.Number
	base.Column = 0
	if element != nil {
		base.Column = element.Start
	}
	return base
}

// dataError создаёт существующий diagnostic.Error с русской причиной,
// точным местом, связанными местами и подробностями. Не выводит сообщения
// и не повторяет целиком упражнение. Конфликт сохраняет всех известных
// участников; порядок Related/Details детерминирован порядком источника.
func dataError(code string, reason string, context diagnostic.Context, related []diagnostic.Context, details []diagnostic.Detail) *diagnostic.Error {
	var relatedCopy []diagnostic.Context
	if related != nil {
		relatedCopy = make([]diagnostic.Context, len(related))
		copy(relatedCopy, related)
	}
	var detailsCopy []diagnostic.Detail
	if details != nil {
		detailsCopy = make([]diagnostic.Detail, len(details))
		copy(detailsCopy, details)
	}
	return &diagnostic.Error{
		Code:    code,
		Reason:  reason,
		Context: context,
		Related: relatedCopy,
		Details: detailsCopy,
	}
}
