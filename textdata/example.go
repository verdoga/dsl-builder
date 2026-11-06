package textdata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// buildExample координирует необязательный title, splitExample и joinBody.
// Однострочный пример всегда содержит body из inlineText; дефисы и стрелки
// в нём не разделяют данные. Блок без разделителя даёт body; блок с одним
// содержательным разделителем даёт before, затем after. Название, если
// присутствует, стоит первым. У всех данных Answers пуст и Hint пуст.
func buildExample(declaration model.Line, lines []model.Line, context diagnostic.Context) ([]lesson.Data, *diagnostic.Error) {
	result := make([]lesson.Data, 0, 3)
	title, err := declarationTitle(lesson.NodeExample, declaration, context)
	if err != nil {
		return nil, err
	}
	if title != nil {
		result = append(result, textDatum(lesson.RoleTitle, *title, ""))
	}
	if declaration.LineType != model.LineTypeBlockStart {
		body, _, err := inlineText(declaration, context)
		if err != nil {
			return nil, err
		}
		return append(result, textDatum(lesson.RoleBody, body, "")), nil
	}
	before, after, separated, err := splitExample(lines, context)
	if err != nil {
		return nil, err
	}
	body, present, err := joinBody(before, false, context)
	if err != nil {
		return nil, err
	}
	if !separated {
		if present {
			result = append(result, textDatum(lesson.RoleBody, body, ""))
		}
		return result, nil
	}
	following, _, err := joinBody(after, false, context)
	if err != nil {
		return nil, err
	}
	result = append(result, textDatum(lesson.RoleBefore, body, ""), textDatum(lesson.RoleAfter, following, ""))
	return result, nil
}

// splitExample разделяет только собственные строки блочного example по
// isExampleSeparator, исключая сам разделитель. При отсутствии разделителя
// before содержит всё тело, after пуст, separated=false; вызывающий код
// использует роль body. Повторный разделитель либо отсутствие непустого
// содержимого с одной из сторон дают CodeExampleSeparator с координатами.
// Внутристрочные дефисы, стрелки и экранированный литерал не разделяются.
func splitExample(lines []model.Line, context diagnostic.Context) (before []model.Line, after []model.Line, separated bool, err *diagnostic.Error) {
	separators := make([]int, 0, 1)
	for index, line := range lines {
		if isExampleSeparator(line) {
			separators = append(separators, index)
		}
	}
	if len(separators) == 0 {
		return lines, nil, false, nil
	}
	separator := &lines[separators[0]]
	if len(separators) > 1 {
		separator = &lines[separators[1]]
	}
	var element *model.Element
	if len(separator.Elements) != 0 {
		element = &separator.Elements[0]
	}
	if len(separators) > 1 {
		err = dataError(CodeExampleSeparator, "В примере указан повторный разделитель", separator, element, context)
		for _, index := range separators {
			line := &lines[index]
			var marker *model.Element
			if len(line.Elements) != 0 {
				marker = &line.Elements[0]
			}
			place := dataError(CodeExampleSeparator, "", line, marker, context).Context
			err.Related = append(err.Related, place)
		}
		return nil, nil, false, err
	}
	before, after = lines[:separators[0]], lines[separators[0]+1:]
	_, beforePresent, err := joinBody(before, false, context)
	if err != nil {
		return nil, nil, false, err
	}
	_, afterPresent, err := joinBody(after, false, context)
	if err != nil {
		return nil, nil, false, err
	}
	if !beforePresent || !afterPresent {
		return nil, nil, false, dataError(CodeExampleSeparator, "Разделитель примера требует непустого содержимого с обеих сторон", separator, element, context)
	}
	return before, after, true, nil
}

// isExampleSeparator признаёт нормативный LineTypeSeparator.
// Совместимость со старым golden 3, строка 15: допускается LineTypeContent
// с единственным content, у которого Raw и ненулевое Value равны ---,
// а вне его диапазона в Line.Raw находятся только U+0020/TAB.
// Это узкое чтение сохранённой структурной записи по ТЗ 10.2; Value=---
// само по себе недостаточно. Экранированный Raw не является разделителем.
// Вызывается исключительно для строк тела блочного example.
func isExampleSeparator(line model.Line) bool {
	if line.LineType == model.LineTypeSeparator {
		return true
	}
	if line.LineType != model.LineTypeContent || len(line.Elements) != 1 {
		return false
	}
	element := line.Elements[0]
	if element.ElementType != model.ElementTypeContent || element.Raw != "---" || element.Value == nil || *element.Value != "---" {
		return false
	}
	raw := []rune(line.Raw)
	if element.Start < 1 || element.End < element.Start || element.End-element.Start != 3 || element.End > len(raw)+1 {
		return false
	}
	if string(raw[element.Start-1:element.End-1]) != "---" {
		return false
	}
	for index, character := range raw {
		if index >= element.Start-1 && index < element.End-1 {
			continue
		}
		if character != ' ' && character != '\t' {
			return false
		}
	}
	return true
}
