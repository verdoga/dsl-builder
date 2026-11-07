package parsed

import (
	"fmt"
	"sort"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// LineIndex хранит только рабочие адреса строк; сами model.Line не копирует.
// Индексы среза начинаются с 0, физические номера — с 1.
// Карты и срезы читаются без изменения вместе с Input.Result.
type LineIndex struct {
	// ByNumber связывает номер физической строки с индексом в Result.Lines.
	ByNumber map[int]int

	// Children связывает номер родителя с номерами непосредственных потомков
	// в исходном порядке. Ключ 0 обозначает строки с parentLine=null.
	// Отсутствующий ключ означает отсутствие детей; фиктивной строки 0 нет.
	Children map[int][]int
}

// indexNumbers проверяет положительность, уникальность и исходный порядок
// физических номеров, строит ByNumber без перезаписи дублей.
// При известном lineCount дополнительно требует номера 1..lineCount
// и равенство длине lines; при null не выдумывает полный размер исходника.
func indexNumbers(lines []model.Line, lineCount *int, context diagnostic.Context) (map[int]int, []*diagnostic.Error) {
	byNumber := make(map[int]int, len(lines))
	var issues []*diagnostic.Error
	if lineCount != nil && *lineCount != len(lines) {
		issues = append(issues, inputError(CodeContract,
			"Число физических строк не совпадает с длиной массива lines.",
			"document.lineCount", context, nil))
	}
	for i, line := range lines {
		lineContext := context
		lineContext.Line, lineContext.Column = 0, 0
		if line.Number > 0 {
			lineContext.Line = line.Number
		}
		field := fmt.Sprintf("lines[%d].line", i)
		if line.Number <= 0 {
			issues = append(issues, inputError(CodeContract,
				"Номер физической строки должен быть положительным.", field, lineContext, nil))
		} else if first, exists := byNumber[line.Number]; exists {
			issue := inputError(CodeContract,
				"Номер физической строки повторяется.", field, lineContext, nil)
			firstContext := lineContext
			firstContext.Line = lines[first].Number
			issue.Related = []diagnostic.Context{firstContext}
			issues = append(issues, issue)
		} else {
			byNumber[line.Number] = i
		}
		if i > 0 && line.Number <= lines[i-1].Number {
			issues = append(issues, inputError(CodeContract,
				"Номера физических строк должны строго возрастать в исходном порядке.", field, lineContext, nil))
		}
		if lineCount != nil && (line.Number != i+1 || line.Number > *lineCount) {
			issues = append(issues, inputError(CodeContract,
				"При известном lineCount строки должны иметь последовательные номера от 1 до lineCount.",
				field, lineContext, nil))
		}
	}
	return byNumber, issues
}

// validateParent проверяет одну ссылку: родитель существует и предшествует
// строке, его nestingLevel на 1 меньше. Корень имеет nestingLevel=0.
// heading имеет parentLine=null и глубину 0; родитель block-end — block-start.
// Не трактует nestingLevel как единственный источник семантики task/step.
func validateParent(line model.Line, lines []model.Line, byNumber map[int]int, context diagnostic.Context) *diagnostic.Error {
	context.Line, context.Column = 0, 0
	if line.Number > 0 {
		context.Line = line.Number
	}
	if line.LineType == model.LineTypeHeading && (line.ParentLine != nil || line.NestingLevel != 0) {
		return inputError(CodeParent, "Заголовок должен быть корневой строкой с глубиной 0.", "parentLine", context, nil)
	}
	if line.ParentLine == nil {
		if line.LineType == model.LineTypeBlockEnd {
			return inputError(CodeParent, "Закрывающая граница блока должна ссылаться на открывающую строку.", "parentLine", context, nil)
		}
		if line.NestingLevel != 0 {
			return inputError(CodeParent, "Корневая строка должна иметь глубину 0.", "nestingLevel", context, nil)
		}
		return nil
	}
	parentIndex, exists := byNumber[*line.ParentLine]
	if !exists || parentIndex < 0 || parentIndex >= len(lines) || *line.ParentLine <= 0 {
		return inputError(CodeParent, "Родительская строка не существует.", "parentLine", context, nil)
	}
	parent := lines[parentIndex]
	if parent.Number >= line.Number {
		return inputError(CodeParent, "Родительская строка должна предшествовать дочерней.", "parentLine", context, nil)
	}
	// Вычитание после проверки положительности исключает переполнение int.
	if line.NestingLevel <= 0 || parent.NestingLevel < 0 || line.NestingLevel-1 != parent.NestingLevel {
		return inputError(CodeParent, "Глубина дочерней строки должна быть на 1 больше глубины родителя.", "nestingLevel", context, nil)
	}
	if line.LineType == model.LineTypeBlockEnd && parent.LineType != model.LineTypeBlockStart {
		return inputError(CodeParent, "Родителем закрывающей границы должна быть открывающая строка блока.", "parentLine", context, nil)
	}
	return nil
}

// validateParentCycles проверяет конечность всех цепочек parentLine,
// включая компоненты без корня. Вызывается после проверки существования
// ссылок; возвращает участников цикла в Related без лимита глубины.
// Ошибки порядка родителя не исключают диагностику обнаруженного цикла.
func validateParentCycles(lines []model.Line, byNumber map[int]int, context diagnostic.Context) []*diagnostic.Error {
	state := make([]uint8, len(lines))
	position := make([]int, len(lines))
	var issues []*diagnostic.Error
	for start := range lines {
		if state[start] != 0 {
			continue
		}
		var path []int
		current := start
		for current >= 0 && state[current] == 0 {
			state[current] = 1
			position[current] = len(path)
			path = append(path, current)
			parent := lines[current].ParentLine
			current = -1
			if parent != nil {
				if next, exists := byNumber[*parent]; exists && next >= 0 && next < len(lines) {
					current = next
				}
			}
		}
		if current >= 0 && state[current] == 1 {
			members := append([]int(nil), path[position[current]:]...)
			// Участники перечисляются в порядке исходного массива, а не map.
			sort.Ints(members)
			cycleContext := context
			cycleContext.Line, cycleContext.Column = lines[members[0]].Number, 0
			issue := inputError(CodeParentCycle, "Обнаружен цикл ссылок parentLine.", "parentLine", cycleContext, nil)
			for _, member := range members {
				related := context
				related.Line, related.Column = lines[member].Number, 0
				issue.Related = append(issue.Related, related)
			}
			issues = append(issues, issue)
		}
		for _, visited := range path {
			state[visited] = 2
		}
	}
	return issues
}

// indexChildren строит только непосредственные связи после успешной проверки
// родителей и циклов; сохраняет порядок исходного массива, включая blank
// и границы блоков. Семантические контейнеры и развёртывание фрагментов не строит.
func indexChildren(lines []model.Line) map[int][]int {
	children := make(map[int][]int)
	for _, line := range lines {
		parent := 0
		if line.ParentLine != nil {
			parent = *line.ParentLine
		}
		children[parent] = append(children[parent], line.Number)
	}
	return children
}
