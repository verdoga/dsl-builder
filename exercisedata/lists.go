package exercisedata

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// splitLists делит содержимое matching/ordering только по прямому separator,
// определённому isSeparator. Возвращает индекс либо -1. В matching требуется
// ровно один разделитель, в ordering — ноль или один; повтор — CodeCollection.
// Блоки answer/hint проходят как целые конструкции; их текст не исследуется
// на разделители. Наличие пустой нижней части не равнозначно отсутствию.
func splitLists(kind lesson.NodeType, lines []model.Line, context diagnostic.Context) (int, *diagnostic.Error) {
	if len(lines) == 0 {
		return -1, dataError(CodeSource, "Не задано объявление списка", context, nil, nil)
	}
	end, err := blockEnd(lines, 0, context)
	if err != nil {
		return -1, err
	}
	separator := -1
	var sources []diagnostic.Context
	for i := 1; i < end; i++ {
		line := lines[i]
		if line.ParentLine == nil || *line.ParentLine != lines[0].Number {
			continue
		}
		if isSeparator(line) {
			if separator == -1 {
				separator = i
			}
			source := sourceContext(context, line, nil)
			if len(line.Elements) != 0 {
				source = sourceContext(context, line, &line.Elements[0])
			}
			sources = append(sources, source)
			continue
		}
		if line.LineType != model.LineTypeBlockStart {
			continue
		}
		tag, err := tagValue(line, context)
		if err != nil {
			return -1, err
		}
		if tag == "answer" || tag == "hint" {
			close, err := blockEnd(lines[:end], i, context)
			if err != nil {
				return -1, err
			}
			i = close
		}
	}
	if len(sources) > 1 {
		return -1, dataError(CodeCollection, "В упражнении допускается не более одного разделителя списков", sources[1], sources, nil)
	}
	if kind == lesson.NodeMatching && separator == -1 {
		return -1, dataError(CodeCollection, "В сопоставлении требуется разделитель списков", sourceContext(context, lines[0], nil), nil, nil)
	}
	return separator, nil
}

// listItems читает область до разделителя или весь сокращённый ordering.
// Каждая непустая прямая content-строка создаёт item. Следующий значимый
// answer того же родителя связывается только с этим item через attachAnswer;
// readAnswer включает его связанный hint. Повторный/осиротевший answer,
// несвязанный hint и иная значимая конструкция — CodeBinding.
// start включён, end исключён; индексы относятся к переданному lines.
func listItems(lines []model.Line, start int, end int, context diagnostic.Context) ([]entry, *diagnostic.Error) {
	if len(lines) == 0 || start < 1 || start > end || end > len(lines) {
		return nil, dataError(CodeSource, "Неверные границы списка элементов", context, nil, nil)
	}
	items := []entry{}
	var bindingSources []diagnostic.Context
	for i := start; i < end; {
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
			return nil, dataError(CodeBinding, "Элемент списка или ответ имеет неверного родителя", source, nil, nil)
		}
		tag, err := tagValue(line, context)
		if err != nil {
			return nil, err
		}
		if line.LineType == model.LineTypeContent && tag == "" && !isSeparator(line) {
			value, found, source, err := contentValue(line, context)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, dataError(CodeSource, "Не задан текст элемента списка", source, nil, nil)
			}
			if value != "" {
				items = append(items, entry{data: textData(lesson.RoleItem, value, nil, ""), source: source})
				bindingSources = []diagnostic.Context{source}
			}
			i++
			continue
		}
		if tag != "answer" || len(items) == 0 {
			return nil, dataError(CodeBinding, "Ожидался элемент списка или связанный с ним answer", source, append(bindingSources, source), nil)
		}
		var bindingErr *diagnostic.Error
		item := items[len(items)-1]
		if item.answerSource != nil || len(item.data.Answers) != 0 {
			bindingErr = dataError(CodeBinding, "Для элемента повторно задан answer", source, append(bindingSources, source), nil)
		}
		group, next, err := readAnswer(lines[:end], i, false, context)
		if err != nil {
			if bindingErr != nil {
				return nil, bindingErr
			}
			return nil, err
		}
		// Сохраняем места объявлений, включая поглощённый readAnswer hint.
		for j := i; j < next; j++ {
			consumed := lines[j]
			if consumed.ParentLine == nil || *consumed.ParentLine != lines[0].Number {
				continue
			}
			for k := range consumed.Elements {
				element := &consumed.Elements[k]
				if element.ElementType == model.ElementTypeTag && element.Value != nil && (*element.Value == "answer" || *element.Value == "hint") {
					bindingSources = append(bindingSources, sourceContext(context, consumed, element))
					break
				}
			}
		}
		if bindingErr != nil {
			bindingErr.Related = append([]diagnostic.Context(nil), bindingSources...)
			return nil, bindingErr
		}
		attached, err := attachAnswer(items[len(items)-1], group)
		if err != nil {
			err.Related = append([]diagnostic.Context(nil), bindingSources...)
			return nil, err
		}
		items[len(items)-1] = attached
		i = next
	}
	return items, nil
}

// listLabels читает область после разделителя: по одной непустой content-строке
// на option либо position, с пустыми Answers и Hint. Теги answer/hint здесь
// запрещены; вложенные блоки не разворачиваются в подписи. Исходные обозначения
// не превращаются в индексы и не удаляются из отображаемого текста.
func listLabels(lines []model.Line, start int, end int, role lesson.DataRole, context diagnostic.Context) ([]entry, *diagnostic.Error) {
	if len(lines) == 0 || start < 1 || start > end || end > len(lines) {
		return nil, dataError(CodeSource, "Неверные границы списка подписей", context, nil, nil)
	}
	labels := []entry{}
	for i := start; i < end; i++ {
		line := lines[i]
		if line.LineType == model.LineTypeBlank {
			continue
		}
		source := sourceContext(context, line, nil)
		for j := range line.Elements {
			if line.Elements[j].ElementType == model.ElementTypeTag {
				source = sourceContext(context, line, &line.Elements[j])
				break
			}
		}
		tag, err := tagValue(line, context)
		if err != nil {
			return nil, err
		}
		if line.LineType != model.LineTypeContent || tag != "" || isSeparator(line) || line.ParentLine == nil || *line.ParentLine != lines[0].Number {
			return nil, dataError(CodeBinding, "В нижнем списке допустимы только прямые текстовые подписи без answer и hint", source, nil, nil)
		}
		value, found, source, err := contentValue(line, context)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, dataError(CodeSource, "Не задан текст подписи списка", source, nil, nil)
		}
		if value != "" {
			labels = append(labels, entry{data: textData(role, value, nil, ""), source: source})
		}
	}
	return labels, nil
}

// checkLabelReferences проверяет каждый ответ item по подписям нижнего списка:
// ровно одно совпадение без учёта регистра посредством strings.EqualFold,
// с учётом всех остальных символов. Вызывается только при полной конфигурации.
// Перед ссылками запрещает совпадающие без учёта регистра подписи нижнего
// списка, даже если на них нет ссылки. Отсутствие/неоднозначность — CodeReference.
// Исходные строки Answers сохраняются, не заменяются индексами или подписями.
// Не устанавливает взаимно однозначного соответствия между разными items:
// несколько элементов вправе ссылаться на один option/явную position.
func checkLabelReferences(items []entry, labels []entry, context diagnostic.Context) *diagnostic.Error {
	if err := checkDistinctLabels(labels, context); err != nil {
		return err
	}
	for _, item := range items {
		for i, value := range item.data.Answers {
			source := item.source
			if item.answerSource != nil {
				source = *item.answerSource
			}
			if i < len(item.answerSources) {
				source = item.answerSources[i]
			}
			if _, err := resolveLabel(value, labels, source); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkDistinctLabels обнаруживает все места совпадающей без учёта регистра
// подписи при включённой проверке; непроверяемые списки не ограничивает.
func checkDistinctLabels(labels []entry, context diagnostic.Context) *diagnostic.Error {
	var related []diagnostic.Context
	var details []diagnostic.Detail
	for i, label := range labels {
		value, ok := label.data.Value.(string)
		if !ok {
			return dataError(CodeSource, "Подпись списка должна быть текстом", label.source, nil, nil)
		}
		duplicate := false
		for j, other := range labels {
			if i == j {
				continue
			}
			otherValue, ok := other.data.Value.(string)
			if !ok {
				return dataError(CodeSource, "Подпись списка должна быть текстом", other.source, nil, nil)
			}
			if strings.EqualFold(value, otherValue) {
				duplicate = true
				break
			}
		}
		if duplicate {
			related = append(related, label.source)
			details = append(details, diagnostic.Detail{Label: "Повторяющаяся подпись", Value: value})
		}
	}
	if len(related) != 0 {
		return dataError(CodeReference, "Подписи нижнего списка совпадают без учёта регистра", context, related, details)
	}
	return nil
}

// resolveLabel проверяет одну ссылку, возвращая индекс единственной подписи.
// source — исходное место значения answer; места кандидатов попадают в Related.
func resolveLabel(value string, labels []entry, source diagnostic.Context) (int, *diagnostic.Error) {
	index := -1
	var matches []diagnostic.Context
	for i, label := range labels {
		text, ok := label.data.Value.(string)
		if !ok {
			return -1, dataError(CodeSource, "Подпись списка должна быть текстом", label.source, nil, nil)
		}
		if strings.EqualFold(value, text) {
			index = i
			matches = append(matches, label.source)
		}
	}
	details := []diagnostic.Detail{{Label: "Значение ответа", Value: value}}
	if len(matches) == 0 {
		candidates := make([]diagnostic.Context, len(labels))
		for i, label := range labels {
			candidates[i] = label.source
		}
		return -1, dataError(CodeReference, "Ответ не соответствует ни одной подписи нижнего списка", source, candidates, details)
	}
	if len(matches) > 1 {
		return -1, dataError(CodeReference, "Ответ соответствует нескольким подписям нижнего списка", source, matches, details)
	}
	return index, nil
}
