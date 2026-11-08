package exercisedata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// matchingData координирует splitLists, listItems,
// listLabels, requireCount, answerCoverage и checkLabelReferences.
// Минимум: один item и два option. Результат: ровно одна items,
// затем ровно одна options; их листья строго текстовые.
// Ответы/подсказки принадлежат конкретным item, нижние options ответов не имеют.
// Полное отсутствие ответов даёт checkable=false; полная корректная привязка
// даёт true; частичная, отсутствующая или неоднозначная ссылка — ошибка.
func matchingData(lines []model.Line, context diagnostic.Context) ([]lesson.Data, bool, *diagnostic.Error) {
	if len(lines) == 0 {
		return nil, false, dataError(CodeSource, "Не задано объявление сопоставления", context, nil, nil)
	}
	separator, err := splitLists(lesson.NodeMatching, lines, context)
	if err != nil {
		return nil, false, err
	}
	end, err := blockEnd(lines, 0, context)
	if err != nil {
		return nil, false, err
	}
	context = sourceContext(context, lines[0], nil)
	items, err := listItems(lines, 1, separator, context)
	if err != nil {
		return nil, false, err
	}
	if err := requireCount(lesson.RoleItems, items, 1, context); err != nil {
		return nil, false, err
	}
	options, err := listLabels(lines, separator+1, end, lesson.RoleOption, context)
	if err != nil {
		return nil, false, err
	}
	if err := requireCount(lesson.RoleOptions, options, 2, context); err != nil {
		return nil, false, err
	}
	checkable, err := answerCoverage(items, context)
	if err != nil {
		return nil, false, err
	}
	if checkable {
		if err := checkLabelReferences(items, options, context); err != nil {
			return nil, false, err
		}
	}
	data := []lesson.Data{collectionData(lesson.RoleItems, items), collectionData(lesson.RoleOptions, options)}
	return data, checkable, nil
}
