package exercisedata

import (
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// orderingData координирует splitLists, listItems, listLabels, requireCount,
// answerCoverage и проверку выбранного формата. Минимум два item; при явном
// разделителе — минимум две position. Возвращает items и только при явных
// позициях positions; синтетических positions для сокращённой формы нет.
// Ответы и hint относятся к item; порядок элементов и подписей сохранён,
// перестановку для показа выполняет плеер. Инструкцию обработает вызывающий слой.
// Без ответов checkable=false; частичная настройка ошибочна. При полной
// конфигурации расширенной формы вызывает checkLabelReferences, сокращённой —
// checkNumberedPositions. True выдаётся только после соответствующей проверки.
func orderingData(lines []model.Line, context diagnostic.Context) ([]lesson.Data, bool, *diagnostic.Error) {
	separator, err := splitLists(lesson.NodeOrdering, lines, context)
	if err != nil {
		return nil, false, err
	}
	end, err := blockEnd(lines, 0, context)
	if err != nil {
		return nil, false, err
	}
	context = sourceContext(context, lines[0], nil)
	itemEnd := end
	if separator >= 0 {
		itemEnd = separator
	}
	items, err := listItems(lines, 1, itemEnd, context)
	if err != nil {
		return nil, false, err
	}
	if err := requireCount(lesson.RoleItems, items, 2, context); err != nil {
		return nil, false, err
	}
	var positions []entry
	if separator >= 0 {
		positions, err = listLabels(lines, separator+1, end, lesson.RolePosition, context)
		if err != nil {
			return nil, false, err
		}
		if err := requireCount(lesson.RolePositions, positions, 2, context); err != nil {
			return nil, false, err
		}
	}
	checkable, err := answerCoverage(items, context)
	if err != nil {
		return nil, false, err
	}
	if checkable {
		if separator >= 0 {
			err = checkLabelReferences(items, positions, context)
		} else {
			err = checkNumberedPositions(items, context)
		}
		if err != nil {
			return nil, false, err
		}
	}
	result := []lesson.Data{collectionData(lesson.RoleItems, items)}
	if separator >= 0 {
		result = append(result, collectionData(lesson.RolePositions, positions))
	}
	return result, checkable, nil
}

// checkNumberedPositions координирует numberedChoices и requirePermutation.
// Не извлекает числа из подписей item и не выводит ответы из исходного порядка.
// Повторяющиеся тексты item сохраняет разными экземплярами с собственными
// наборами допустимых позиций; взаимозаменяемость при оценке — задача плеера.
func checkNumberedPositions(items []entry, context diagnostic.Context) *diagnostic.Error {
	choices, err := numberedChoices(items, context)
	if err != nil {
		return err
	}
	return requirePermutation(choices, items, context)
}

// numberedChoices проверяет каждый ответ через positionIndex и возвращает
// допустимые индексы позиций для каждого исходного экземпляра item.
// Повтор одной допустимой позиции внутри ответа не меняет множество
// допустимости; исходный массив Answers остаётся без удаления повторов.
func numberedChoices(items []entry, context diagnostic.Context) ([][]int, *diagnostic.Error) {
	choices := make([][]int, len(items))
	for i, item := range items {
		seen := make(map[int]bool)
		for j, value := range item.data.Answers {
			source := item.source
			if item.answerSource != nil {
				source = *item.answerSource
			}
			if j < len(item.answerSources) {
				source = item.answerSources[j]
			}
			position, err := positionIndex(value, len(items), source)
			if err != nil {
				return nil, err
			}
			if !seen[position] {
				choices[i] = append(choices[i], position)
				seen[position] = true
			}
		}
	}
	return choices, nil
}

// positionIndex требует ASCII-запись положительного целого без знака,
// ведущих нулей и дробной части, в диапазоне 1..count; возвращает индекс с 0.
// Произвольно длинное значение сначала сравнивается с count как десятичная
// строка, без переполнения int/float64. В int переводится лишь заведомо
// допустимый индекс реально существующего среза. Answers не переписывается.
func positionIndex(value string, count int, source diagnostic.Context) (int, *diagnostic.Error) {
	valid := count > 0 && len(value) > 0 && value[0] != '0'
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			valid = false
			break
		}
	}
	maximum := strconv.Itoa(count)
	if len(value) > len(maximum) || (len(value) == len(maximum) && value > maximum) {
		valid = false
	}
	if !valid {
		return -1, dataError(CodeReference, "Ответ должен задавать позицию от 1 до числа элементов без знака и ведущих нулей", source, nil, []diagnostic.Detail{
			{Label: "Значение ответа", Value: value},
			{Label: "Число элементов", Value: maximum},
		})
	}
	number := 0
	for i := 0; i < len(value); i++ {
		number = number*10 + int(value[i]-'0')
	}
	return number - 1, nil
}

// requirePermutation проверяет существование хотя бы одной полной перестановки
// исходных экземпляров по их допустимым позициям: одна позиция на каждый item
// и один item на позицию. Делегирует шаг назначения assignPosition; жадный
// выбор первого ответа недостаточен. Невозможность — CodeContradiction.
// Существующую перестановку не записывает в данные и не меняет их порядок.
func requirePermutation(choices [][]int, items []entry, context diagnostic.Context) *diagnostic.Error {
	owner := make([]int, len(items))
	for i := range owner {
		owner[i] = -1
	}
	for i := range items {
		if assignPosition(i, choices, owner, make([]bool, len(owner))) {
			continue
		}
		var related []diagnostic.Context
		for _, item := range items {
			related = append(related, item.source)
			if item.answerSource != nil {
				related = append(related, *item.answerSource)
			}
			for _, source := range item.answerSources {
				duplicate := false
				for _, known := range related {
					if known == source {
						duplicate = true
						break
					}
				}
				if !duplicate {
					related = append(related, source)
				}
			}
		}
		return dataError(CodeContradiction, "Допустимые позиции элементов не образуют полной перестановки", context, related, nil)
	}
	return nil
}

// assignPosition решает только шаг поиска увеличивающего пути для одного item
// в двудольном соответствии экземпляров и позиций; owner хранит текущего
// владельца позиции либо -1, visited — позиции текущего поиска. Обновляет
// только временное соответствие. Нет искусственного предела числа элементов.
func assignPosition(item int, choices [][]int, owner []int, visited []bool) bool {
	for _, position := range choices[item] {
		if visited[position] {
			continue
		}
		visited[position] = true
		if owner[position] == -1 || assignPosition(owner[position], choices, owner, visited) {
			owner[position] = item
			return true
		}
	}
	return false
}
