package textdata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// buildInstruction координирует inlineText либо splitInstructionHint,
// joinBody и instructionHint. Создаёт один body с объединённым текстом
// инструкции; вложенная подсказка записывается в Hint этого body целиком.
// Подсказка не включается в Value и не порождает отдельные данные или ноду.
// Порядок обычных строк до и после hint сохраняется. Answers пуст.
// При декларации multifill или ordering читает только её inlineText:
// роль body принадлежит отдельной ноде instruction, а не упражнению.
// Отдельный instr и объявленная инструкция обрабатываются независимыми
// вызовами; совпадение текста не вызывает объединения или удаления.
// Пустая выделенная инструкция не создаёт body; решение не создавать
// ноду без исходной инструкции принимает structure до вызова Build.
// Подсказка без содержимого самой инструкции даёт CodeInstructionHint,
// чтобы ошибочный локальный вход не приводил к незаметной утрате подсказки.
func buildInstruction(declaration model.Line, lines []model.Line, context diagnostic.Context) ([]lesson.Data, *diagnostic.Error) {
	result := make([]lesson.Data, 0, 1)
	block := declaration.LineType == model.LineTypeBlockStart
	standalone := true
	for _, element := range declaration.Elements {
		if element.ElementType == model.ElementTypeTag && element.Value != nil && *element.Value != "instr" {
			block = false
			standalone = false
		}
	}
	if !block {
		value, present, err := inlineText(declaration, context)
		if err != nil {
			return nil, err
		}
		if value != "" || standalone && present {
			result = append(result, textDatum(lesson.RoleBody, value, ""))
		}
		return result, nil
	}

	body, hintDeclaration, hintLines, err := splitInstructionHint(declaration, lines, context)
	if err != nil {
		return nil, err
	}
	value, _, err := joinBody(body, false, context)
	if err != nil {
		return nil, err
	}
	if value == "" {
		if hintDeclaration != nil {
			return nil, dataError(CodeInstructionHint, "Подсказка не имеет содержимого инструкции", hintDeclaration, nil, context)
		}
		return result, nil
	}
	hint, err := instructionHint(hintDeclaration, hintLines, context)
	if err != nil {
		return nil, err
	}
	return append(result, textDatum(lesson.RoleBody, value, hint)), nil
}

// splitInstructionHint отделяет собственные текстовые строки инструкции
// от единственного непосредственно вложенного hint по tag.Value и ParentLine.
// Вход ограничен одним instr одного экземпляра; глобальный индекс не строится.
// Возвращает обычные строки, исходную декларацию hint или nil и только его
// строки содержимого; границы hint не попадают в оба текстовых результата.
// Вторая подсказка, чужие вложенные теги или неоднозначная локальная связь
// дают CodeInstructionHint. Связывать соседний самостоятельный hint нельзя.
// Привязка подсказок к question и другим упражнениям остаётся exercisedata.
func splitInstructionHint(declaration model.Line, lines []model.Line, context diagnostic.Context) (body []model.Line, hintDeclaration *model.Line, hintLines []model.Line, err *diagnostic.Error) {
	body = make([]model.Line, 0, len(lines))
	hintLines = make([]model.Line, 0)
	var related []diagnostic.Context
	for i := range lines {
		for j := range lines[i].Elements {
			element := &lines[i].Elements[j]
			if element.ElementType == model.ElementTypeTag {
				place := context
				place.Line, place.Column = lines[i].Number, element.Start
				related = append(related, place)
			}
		}
	}

	insideHint := false
	for i := range lines {
		line := &lines[i]
		var tag *model.Element
		tagCount := 0
		for j := range line.Elements {
			if line.Elements[j].ElementType == model.ElementTypeTag {
				tagCount++
				if tag == nil {
					tag = &line.Elements[j]
				}
			}
		}
		switch line.LineType {
		case model.LineTypeTag, model.LineTypeBlockStart:
			if tagCount != 1 || tag.Value == nil || *tag.Value != "hint" {
				err = dataError(CodeInstructionHint, "В инструкции допустима только однозначная вложенная подсказка", line, tag, context)
				break
			}
			if hintDeclaration != nil {
				err = dataError(CodeInstructionHint, "Инструкция содержит несколько подсказок", line, tag, context)
				break
			}
			if line.ParentLine == nil || *line.ParentLine != declaration.Number {
				err = dataError(CodeInstructionHint, "Подсказка не вложена непосредственно в эту инструкцию", line, tag, context)
				break
			}
			hintDeclaration = line
			insideHint = line.LineType == model.LineTypeBlockStart
		case model.LineTypeBlockEnd:
			var closeElement *model.Element
			for j := range line.Elements {
				if line.Elements[j].ElementType == model.ElementTypeBlockClose {
					closeElement = &line.Elements[j]
					break
				}
			}
			if tagCount != 0 || !insideHint || line.ParentLine == nil || *line.ParentLine != hintDeclaration.Number {
				err = dataError(CodeInstructionHint, "Граница не соответствует вложенной подсказке инструкции", line, closeElement, context)
				break
			}
			insideHint = false
		case model.LineTypeContent, model.LineTypeBlank:
			if tagCount != 0 {
				err = dataError(CodeInstructionHint, "Текст инструкции или подсказки содержит чужой вложенный тег", line, tag, context)
				break
			}
			parent := declaration.Number
			if insideHint {
				parent = hintDeclaration.Number
			}
			if line.ParentLine == nil || *line.ParentLine != parent {
				err = dataError(CodeInstructionHint, "Неоднозначна принадлежность строки инструкции или подсказки", line, nil, context)
				break
			}
			if insideHint {
				hintLines = append(hintLines, *line)
			} else {
				body = append(body, *line)
			}
		default:
			err = dataError(CodeInstructionHint, "Недопустимая строка внутри инструкции или подсказки", line, tag, context)
		}
		if err != nil {
			break
		}
	}
	if err == nil && insideHint {
		err = dataError(CodeInstructionHint, "У вложенной подсказки отсутствует закрывающая граница", hintDeclaration, nil, context)
	}
	if err != nil {
		err.Related = related
		found := false
		for _, place := range related {
			if place.Line == err.Context.Line && place.Column == err.Context.Column {
				found = true
			}
		}
		if !found {
			err.Related = append(err.Related, err.Context)
		}
		return nil, nil, nil, err
	}
	return body, hintDeclaration, hintLines, nil
}

// instructionHint возвращает пустую строку при hintDeclaration=nil,
// inlineText для однострочной подсказки либо joinBody с keepEmpty=false
// для блочной. Сохраняет порядок, разметку и переводы \n; ничего не связывает.
func instructionHint(hintDeclaration *model.Line, hintLines []model.Line, context diagnostic.Context) (string, *diagnostic.Error) {
	if hintDeclaration == nil {
		return "", nil
	}
	if hintDeclaration.LineType == model.LineTypeBlockStart {
		value, _, err := joinBody(hintLines, false, context)
		return value, err
	}
	value, _, err := inlineText(*hintDeclaration, context)
	return value, err
}
