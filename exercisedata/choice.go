package exercisedata

import (
	"sort"
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// choiceData координирует localInstruction, choiceOptions, requireCount
// с минимумом 2, checkChoiceAnswers и collectionData. Возвращает инструкцию
// при наличии, затем ровно одну options. Порядок исходных вариантов сохранён.
// singleChoice отличается ограничением одной группы и одного правильного
// варианта; multipleChoice сохраняет все группы и экземпляры без объединения.
func choiceData(kind lesson.NodeType, lines []model.Line, context diagnostic.Context) ([]lesson.Data, bool, *diagnostic.Error) {
	if len(lines) == 0 {
		return nil, false, dataError(CodeSource, "Не задано объявление упражнения выбора", context, nil, nil)
	}
	instruction, err := localInstruction(lines[0], context)
	if err != nil {
		return nil, false, err
	}
	options, groups, err := choiceOptions(lines, context)
	if err != nil {
		return nil, false, err
	}
	context = sourceContext(context, lines[0], nil)
	if err := requireCount(lesson.RoleOptions, options, 2, context); err != nil {
		return nil, false, err
	}
	checkable, err := checkChoiceAnswers(kind, options, groups, context)
	if err != nil {
		return nil, false, err
	}
	data := make([]lesson.Data, 0, 2)
	if instruction != nil {
		data = append(data, *instruction)
	}
	data = append(data, collectionData(lesson.RoleOptions, options))
	return data, checkable, nil
}

// choiceOptions читает прямые строки содержимого через contentValue,
// создавая обычные option с пустыми Answers/Hint; answer передаёт readAnswer
// и correctOptions. Возвращает также места всех исходных групп answer.
// Blank и пустые content-строки пропускаются, непустые значения сохраняются.
// Вложенные строки answer/hint не становятся дополнительными вариантами.
// Неизвестный тег, несвязанный hint, separator или неверный родитель —
// CodeBinding/CodeSource; границы полного блока определены checkSource.
func choiceOptions(lines []model.Line, context diagnostic.Context) ([]entry, []diagnostic.Context, *diagnostic.Error) {
	if len(lines) == 0 {
		return nil, nil, dataError(CodeSource, "Не задано объявление упражнения выбора", context, nil, nil)
	}
	end, err := blockEnd(lines, 0, context)
	if err != nil {
		return nil, nil, err
	}
	options := []entry{}
	groups := []diagnostic.Context{}
	answerAt := -1
	for i := 1; i < end; {
		line := lines[i]
		if line.LineType == model.LineTypeBlank {
			i++
			continue
		}
		source := sourceContext(context, line, nil)
		for j := range line.Elements {
			if line.Elements[j].ElementType == model.ElementTypeTag {
				source = sourceContext(context, line, &line.Elements[j])
				break
			}
		}
		if line.ParentLine == nil || *line.ParentLine != lines[0].Number {
			return nil, nil, dataError(CodeBinding, "Вариант выбора или answer имеет неверного родителя", source, nil, nil)
		}
		tag, err := tagValue(line, context)
		if err != nil {
			return nil, nil, err
		}
		if line.LineType == model.LineTypeContent && tag == "" {
			value, found, source, err := contentValue(line, context)
			if err != nil {
				return nil, nil, err
			}
			if !found {
				return nil, nil, dataError(CodeSource, "Не задан текст варианта выбора", source, nil, nil)
			}
			if value != "" {
				options = append(options, entry{data: textData(lesson.RoleOption, value, nil, ""), source: source})
				answerAt = -1
			}
			i++
			continue
		}
		if tag != "answer" {
			var related []diagnostic.Context
			if tag == "hint" && answerAt >= 0 {
				// readAnswer уже поглотил связанную hint; сохраняем места объявлений всей связи.
				for _, participant := range lines[answerAt : i+1] {
					if participant.ParentLine == nil || *participant.ParentLine != lines[0].Number {
						continue
					}
					for j := range participant.Elements {
						element := &participant.Elements[j]
						if element.ElementType == model.ElementTypeTag && element.Value != nil && (*element.Value == "answer" || *element.Value == "hint") {
							related = append(related, sourceContext(context, participant, element))
						}
					}
				}
			}
			return nil, nil, dataError(CodeBinding, "В упражнении выбора ожидался текст варианта или answer", source, related, nil)
		}
		group, next, err := readAnswer(lines[:end], i, false, context)
		if err != nil {
			return nil, nil, err
		}
		options = append(options, correctOptions(group)...)
		groups = append(groups, group.source)
		answerAt = i
		i = next
	}
	return options, groups, nil
}

// correctOptions превращает каждое значение одной группы в отдельный option:
// Value — собственный текст, Answers содержит только этот же текст,
// Hint — общая подсказка группы, скопированная каждому варианту.
// Места группы и отдельных значений сохраняются; экземпляры не объединяются.
func correctOptions(group answerGroup) []entry {
	options := make([]entry, len(group.values))
	for i, value := range group.values {
		source := group.source
		if i < len(group.sources) {
			source = group.sources[i]
		}
		answerSource := group.source
		options[i] = entry{
			data:          textData(lesson.RoleOption, value, []string{value}, group.hint),
			source:        source,
			answerSource:  &answerSource,
			answerSources: []diagnostic.Context{source},
		}
	}
	return options
}

// checkChoiceAnswers при отсутствии групп возвращает false. Для singleChoice
// требует ровно одну группу с одним правильным вариантом; для multipleChoice
// разрешает любое положительное количество групп и правильных экземпляров.
// Затем вызывает checkChoiceLabels; только успех даёт true.
func checkChoiceAnswers(kind lesson.NodeType, options []entry, groups []diagnostic.Context, context diagnostic.Context) (bool, *diagnostic.Error) {
	if len(groups) == 0 {
		return false, nil
	}
	correct := 0
	for _, option := range options {
		if len(option.data.Answers) != 0 {
			correct++
		}
	}
	if correct == 0 || (kind == lesson.NodeSingleChoice && (len(groups) != 1 || correct != 1)) {
		related := append([]diagnostic.Context(nil), groups...)
		for _, option := range options {
			if len(option.data.Answers) != 0 {
				related = append(related, option.source)
			}
		}
		sort.SliceStable(related, func(i, j int) bool {
			if related[i].Line != related[j].Line {
				return related[i].Line < related[j].Line
			}
			return related[i].Column < related[j].Column
		})
		return false, dataError(CodeAnswer, "Недопустимое количество групп answer или правильных вариантов выбора", context, related, []diagnostic.Detail{
			{Label: "Количество групп answer", Value: strconv.Itoa(len(groups))},
			{Label: "Количество правильных вариантов", Value: strconv.Itoa(correct)},
		})
	}
	if err := checkChoiceLabels(options, context); err != nil {
		return false, err
	}
	return true, nil
}

// checkChoiceLabels при включённой проверке запрещает точно одинаковые
// подписи вариантов с разными признаками правильности. Регистр, пробелы
// и пунктуация значимы; одинаковые подписи с одинаковой правильностью
// сохраняются отдельными экземплярами. Ошибка указывает оба места.
func checkChoiceLabels(options []entry, context diagnostic.Context) *diagnostic.Error {
	correctness := make(map[string]uint8)
	for _, option := range options {
		value, ok := option.data.Value.(string)
		if !ok {
			return dataError(CodeSource, "Подпись варианта выбора должна быть текстом", option.source, nil, nil)
		}
		if len(option.data.Answers) == 0 {
			correctness[value] |= 1
		} else {
			correctness[value] |= 2
		}
	}
	var related []diagnostic.Context
	var details []diagnostic.Detail
	for _, option := range options {
		value := option.data.Value.(string)
		if correctness[value] == 3 {
			related = append(related, option.source)
			details = append(details, diagnostic.Detail{Label: "Противоречивая подпись", Value: value})
		}
	}
	if len(related) != 0 {
		return dataError(CodeContradiction, "Одинаковые подписи вариантов имеют разные признаки правильности", context, related, details)
	}
	return nil
}
