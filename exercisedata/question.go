package exercisedata

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// questionData координирует contentValue, questionAnswer и textData.
// Создаёт единственный DataText/RoleBody с формулировкой вопроса.
// Отсутствующая, nil или пустая обязательная формулировка — CodeSource.
// Нет связанного answer — Answers пуст, Hint пуст, checkable=false.
// Корректный answer — все его альтернативы и связанный hint у body,
// checkable=true. Отдельные ноды answer и hint не создаются.
func questionData(lines []model.Line, context diagnostic.Context) ([]lesson.Data, bool, *diagnostic.Error) {
	if err := checkSource(lesson.NodeQuestion, lines, context); err != nil {
		return nil, false, err
	}
	value, found, source, err := contentValue(lines[0], context)
	if err != nil {
		return nil, false, err
	}
	if !found || strings.TrimSpace(value) == "" {
		return nil, false, dataError(CodeSource, "Не задана обязательная формулировка вопроса", source, nil, nil)
	}
	group, err := questionAnswer(lines, context)
	if err != nil {
		return nil, false, err
	}
	if group == nil {
		return []lesson.Data{textData(lesson.RoleBody, value, nil, "")}, false, nil
	}
	return []lesson.Data{textData(lesson.RoleBody, value, group.values, group.hint)}, true, nil
}

// questionAnswer читает необязательный единственный answer после question
// через nextSignificant/readAnswer. Он должен иметь тот же ParentLine;
// семантическую принадлежность внешнему контейнеру гарантирует проекция.
// После связанного hint допустим только игнорируемый хвост проекции.
// Второй answer, лишний hint или постороннее содержимое — CodeBinding,
// а не альтернативный способ связывания или молчаливое отбрасывание.
func questionAnswer(lines []model.Line, context diagnostic.Context) (*answerGroup, *diagnostic.Error) {
	if err := checkSource(lesson.NodeQuestion, lines, context); err != nil {
		return nil, err
	}
	questionSource := sourceContext(context, lines[0], nil)
	for i := range lines[0].Elements {
		if lines[0].Elements[i].ElementType == model.ElementTypeTag {
			questionSource = sourceContext(context, lines[0], &lines[0].Elements[i])
			break
		}
	}
	at, err := nextSignificant(lines, 1, true, context)
	if err != nil {
		return nil, err
	}
	if at == len(lines) {
		return nil, nil
	}
	line := lines[at]
	tag, err := tagValue(line, context)
	if err != nil {
		return nil, err
	}
	source := sourceContext(context, line, nil)
	for i := range line.Elements {
		if line.Elements[i].ElementType == model.ElementTypeTag {
			source = sourceContext(context, line, &line.Elements[i])
			break
		}
	}
	parent := lines[0].ParentLine
	if tag != "answer" || (parent == nil) != (line.ParentLine == nil) ||
		(parent != nil && line.ParentLine != nil && *parent != *line.ParentLine) {
		return nil, dataError(CodeBinding, "После вопроса допустим только связанный answer того же родителя", source, []diagnostic.Context{questionSource, source}, nil)
	}
	group, next, err := readAnswer(lines, at, true, context)
	if err != nil {
		return nil, err
	}
	tail, err := nextSignificant(lines, next, true, context)
	if err != nil {
		return nil, err
	}
	if tail == len(lines) {
		return &group, nil
	}
	related := []diagnostic.Context{questionSource, group.source}
	for i := at + 1; i < next; i++ {
		for j := range lines[i].Elements {
			element := &lines[i].Elements[j]
			if element.ElementType == model.ElementTypeTag && element.Value != nil && *element.Value == "hint" {
				related = append(related, sourceContext(context, lines[i], element))
			}
		}
	}
	source = sourceContext(context, lines[tail], nil)
	for i := range lines[tail].Elements {
		if lines[tail].Elements[i].ElementType == model.ElementTypeTag {
			source = sourceContext(context, lines[tail], &lines[tail].Elements[i])
			break
		}
	}
	related = append(related, source)
	return nil, dataError(CodeBinding, "После ответа и связанной подсказки передано постороннее содержимое", source, related, nil)
}
