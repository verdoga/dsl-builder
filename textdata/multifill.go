package textdata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// buildMultifill вызывает joinBody с keepEmpty=true и создаёт ровно один
// body, включая Value="" для пустого или однострочного multifill.
// В lines входят только предзаполненные строки; инструкция из объявления
// передаётся отдельному вызову Build для NodeInstruction внешним конвейером.
// Поддерживает content.value="" и LineTypeBlank без потери пустых строк.
// Полный Raw каждого placeholder сохраняется composeElements, включая
// специальную запись с ответом. Content.Value сохраняет уже обработанный
// парсером текст; его не разбирают повторно, в том числе похожие на теги строки.
// Не извлекает, не нормализует и не оценивает ответы, не связывает подсказки
// с пропусками. Answers всегда пуст, Hint пуст; флаги не возвращаются.
// Вызывающий код оставляет Checkable=nil и пустой объект Flags по ТЗ 10.7.
func buildMultifill(lines []model.Line, context diagnostic.Context) ([]lesson.Data, *diagnostic.Error) {
	body, _, err := joinBody(lines, true, context)
	if err != nil {
		return nil, err
	}
	return []lesson.Data{textDatum(lesson.RoleBody, body, "")}, nil
}
