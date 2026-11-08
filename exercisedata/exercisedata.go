// Package exercisedata формирует только собственные данные пяти упражнений.
// Проект v1: ТЗ 10.2, 10.8, 10.9 и 19; входной контракт 1.0.
// Использует существующие parsed/model, lesson и diagnostic без их изменения.
// Не строит дерево, не присваивает ID, не читает файлы, не разбирает DSL,
// не вычисляет ответы пользователя и не обрабатывает multifill.
package exercisedata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// Build координирует checkSource и один из questionData, choiceData,
// matchingData, orderingData. Алгоритмы извлечения делегированы малым шагам.
// kind допускает только NodeQuestion, NodeSingleChoice, NodeMultipleChoice,
// NodeMatching и NodeOrdering; иной тип даёт CodeKind.
//
// lines — проекция одного упражнения из проверенного входа после развёртывания
// фрагментов; первая строка — его объявление. Для блочных упражнений передаются
// все строки до соответствующего block-end включительно. Для question — его
// строка и только относящиеся к нему answer и hint с их полными блоками.
// Промежуточные blank и разрешённые editor можно сохранить. Внешние границы
// task/step/variant/страницы/include и несвязанные hint в проекцию не входят.
// Вызывающий слой выделяет проекцию по семантическому контейнеру, не только
// parentLine, и не склеивает её через include или другую значимую конструкцию.
// Исходные Number, ParentLine, Elements и координаты не перенумеровываются;
// строки и все их вложенные значения доступны только для чтения.
//
// Значения берутся из Element.Value без повторного снятия экранирования,
// изменения регистра, пробелов, пунктуации, разметки или порядка экземпляров.
// Raw используется только для совместимости разделителей старых golden.
// Каждый выходной Data имеет обязательные Answers (не nil), Hint и Role;
// Value содержит string либо ненулевой []lesson.Data. Ресурсных данных нет.
//
// Объявленная инструкция входит первой в data только для choice/multichoice.
// Текст объявления matching и ordering в data не попадает: structure размещает
// отдельную instruction перед упражнением, textdata формирует её данные
// из объявления.
// Самостоятельные instr/hint и hint внутри instr обрабатывает textdata;
// пакет связывает только hint, следующий за answer этого упражнения.
//
// При успехе data независимы от входа и других вызовов; checkable имеет
// явное значение true/false. Вызывающий код записывает его в Checkable
// существующего lesson.NodeFlags, не устанавливая Speaking или Stepped.
// При ошибке data=nil, checkable=false и err!=nil: частичный результат
// неприменим. Полную проверку итогового урока выполняет lesson.Validate.
// Контекст содержит путь технического JSON, а не document.filePath;
// пакет дополняет его известными исходными строкой и колонкой.
func Build(kind lesson.NodeType, lines []model.Line, context diagnostic.Context) (data []lesson.Data, checkable bool, err *diagnostic.Error) {
	if err = checkSource(kind, lines, context); err != nil {
		return nil, false, err
	}
	switch kind {
	case lesson.NodeQuestion:
		data, checkable, err = questionData(lines, context)
	case lesson.NodeSingleChoice, lesson.NodeMultipleChoice:
		data, checkable, err = choiceData(kind, lines, context)
	case lesson.NodeMatching:
		data, checkable, err = matchingData(lines, context)
	case lesson.NodeOrdering:
		data, checkable, err = orderingData(lines, context)
	default:
		return nil, false, dataError(CodeKind, "Неподдерживаемый тип упражнения", context, nil, nil)
	}
	if err != nil {
		return nil, false, err
	}
	return data, checkable, nil
}
