package parsed

import (
	"fmt"
	"slices"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/order"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// validateLine проверяет форму строки Contract, не классифицируя raw как DSL.
// Связь с родителем отдельно проверяет координатор через validateParent.
func validateLine(line model.Line, last bool, context diagnostic.Context) []*diagnostic.Error {
	context.Line, context.Column = 0, 0
	if line.Number > 0 {
		context.Line = line.Number
	}
	var issues []*diagnostic.Error
	switch line.LineType {
	case model.LineTypeTag, model.LineTypeHeading, model.LineTypeContent,
		model.LineTypeBlockStart, model.LineTypeBlockEnd, model.LineTypeSeparator,
		model.LineTypeBlank, model.LineTypeInvalid:
	default:
		issues = append(issues, inputError(CodeContract, "Неизвестный тип строки.", "type", context, nil))
	}
	if line.NestingLevel < 0 {
		issues = append(issues, inputError(CodeContract, "Глубина вложенности не может быть отрицательной.", "nestingLevel", context, nil))
	}
	if strings.ContainsAny(line.Raw, "\r\n") {
		issues = append(issues, inputError(CodeContract, "Исходная строка raw не должна содержать CR или LF.", "raw", context, nil))
	}
	switch line.LineEnding {
	case model.LineEndingLF, model.LineEndingCRLF:
	case model.LineEndingNone:
		if !last {
			issues = append(issues, inputError(CodeContract, "Пустой EOL допустим только у последней физической строки.", "eol", context, nil))
		}
	default:
		issues = append(issues, inputError(CodeContract, "Недопустимое окончание физической строки.", "eol", context, nil))
	}
	for _, element := range line.Elements {
		issues = append(issues, validateElement(element, line, context)...)
	}
	if issue := validateElementOrder(line.Elements, context); issue != nil {
		issues = append(issues, issue)
	}
	if issue := validateBoundaryElements(line, context); issue != nil {
		issues = append(issues, issue)
	}
	return issues
}

// validateElement проверяет тип, диапазон Unicode-кодовых точек и точный raw.
// Правая граница исключающая; пустой диапазон допустим, value не разбирается.
func validateElement(element model.Element, line model.Line, context diagnostic.Context) []*diagnostic.Error {
	context.Line, context.Column = 0, 0
	if line.Number > 0 {
		context.Line = line.Number
		if element.Start > 0 {
			context.Column = element.Start
		}
	}
	var issues []*diagnostic.Error
	switch element.ElementType {
	case model.ElementTypeTag, model.ElementTypeHeadingLevel, model.ElementTypeTitle,
		model.ElementTypeContent, model.ElementTypeIdentifier, model.ElementTypeName,
		model.ElementTypeVersion, model.ElementTypeNumber, model.ElementTypeMediaType,
		model.ElementTypeSource, model.ElementTypeResourcePath, model.ElementTypePlaceholder,
		model.ElementTypeUnparsed, model.ElementTypeBlockOpen, model.ElementTypeBlockClose:
	default:
		issues = append(issues, inputError(CodeContract, "Неизвестный тип элемента.", "elements.type", context, nil))
	}
	runes := []rune(line.Raw)
	if element.Start < 1 || element.End < element.Start || element.End > len(runes)+1 {
		issues = append(issues, inputError(CodeContract,
			"Диапазон элемента должен находиться в raw и задаваться колонками Unicode с 1; конец исключающий.",
			"elements.start/end", context, nil))
	} else if element.Raw != string(runes[element.Start-1:element.End-1]) {
		issues = append(issues, inputError(CodeContract,
			"Фрагмент raw элемента не совпадает с диапазоном исходной строки.", "elements.raw", context, nil))
	}
	if issue := validateElementValue(element, context); issue != nil {
		issues = append(issues, issue)
	}
	return issues
}

// validateElementOrder проверяет исходный порядок start/end, дубли и пересечения.
// Пустой диапазон не пересекает другие диапазоны; массив не сортируется.
func validateElementOrder(elements []model.Element, context diagnostic.Context) *diagnostic.Error {
	lastEnd := 0
	for i, element := range elements {
		elementContext := context
		elementContext.Column = 0
		if elementContext.Line > 0 && element.Start > 0 {
			elementContext.Column = element.Start
		}
		field := fmt.Sprintf("elements[%d]", i)
		if i > 0 {
			previous := elements[i-1]
			if element.Start < previous.Start || element.Start == previous.Start && element.End < previous.End {
				return inputError(CodeContract, "Элементы должны следовать по start, затем по end.", field, elementContext, nil)
			}
		}
		for j := i - 1; j >= 0 && elements[j].Start == element.Start && elements[j].End == element.End; j-- {
			previous := elements[j]
			sameValue := previous.Value == nil && element.Value == nil ||
				previous.Value != nil && element.Value != nil && *previous.Value == *element.Value
			if previous.ElementType == element.ElementType && previous.Raw == element.Raw && sameValue &&
				slices.Equal(previous.ErrorIDs, element.ErrorIDs) {
				return inputError(CodeContract, "Элемент повторяется в массиве.", field, elementContext, nil)
			}
		}
		if element.Start < element.End {
			if element.Start < lastEnd {
				return inputError(CodeContract, "Диапазоны элементов не должны пересекаться.", field, elementContext, nil)
			}
			lastEnd = element.End
		}
	}
	return nil
}

// validateElementValue проверяет явный словарь Contract, сохраняя свободный
// текст, версии и source без повторного разбора DSL и нормализации.
func validateElementValue(element model.Element, context diagnostic.Context) *diagnostic.Error {
	fail := func(reason string) *diagnostic.Error {
		return inputError(CodeContract, reason, "elements.value", context, nil)
	}
	if element.ElementType == model.ElementTypeUnparsed {
		if element.Value != nil || len(element.ErrorIDs) == 0 {
			return fail("Неразобранный элемент должен иметь value=null и ссылку на диагностику.")
		}
		return nil
	}
	if element.ElementType == model.ElementTypePlaceholder {
		if element.Value == nil && len(element.ErrorIDs) != 0 {
			return fail("Placeholder без ответа должен иметь value=null и пустой errorIds.")
		}
		return nil
	}
	if element.Value == nil {
		if element.ElementType == model.ElementTypeMediaType && len(element.ErrorIDs) != 0 {
			return nil
		}
		return fail("Значение корректного элемента должно быть строкой; null требует предусмотренного контрактом случая.")
	}
	value := *element.Value
	switch element.ElementType {
	case model.ElementTypeTag:
		switch value {
		case "dsl-version", "document-id", "section", "order", "resource-dir", "header", "task", "endtask", "step", "speaking",
			"newpage", "editor", "media", "example", "wordlist", "table", "script", "text", "key", "instr", "note", "alt", "hint",
			"fragment", "include", "answer", "question", "multifill", "choice", "multichoice", "matching", "ordering", "variants", "variant":
		default:
			return fail("Значение tag должно быть одним из 34 канонических имён Contract.")
		}
	case model.ElementTypeHeadingLevel:
		if value != "1" && value != "2" && value != "3" {
			return fail("Уровень заголовка должен быть строкой 1, 2 или 3.")
		}
		if element.Raw != strings.Repeat("#", int(value[0]-'0')) {
			return fail("Raw уровня заголовка должен содержать соответствующее число символов #.")
		}
	case model.ElementTypeMediaType:
		if value != "audio" && value != "video" && value != "image" {
			return fail("Тип медиа должен быть audio, video или image; ошибочное значение представляется null с диагностикой.")
		}
	case model.ElementTypeNumber:
		return order.Validate(value, context)
	case model.ElementTypeBlockOpen:
		if element.Raw != "{" || value != "{" {
			return fail("Открывающая граница должна иметь raw и value, равные {.")
		}
	case model.ElementTypeBlockClose:
		if element.Raw != "}" || value != "}" {
			return fail("Закрывающая граница должна иметь raw и value, равные }.")
		}
	}
	return nil
}

// validateBoundaryElements требует отдельные block-open/block-close/unparsed
// у соответствующих типов строк, не проверяя грамматику и вложения DSL.
func validateBoundaryElements(line model.Line, context diagnostic.Context) *diagnostic.Error {
	var required model.ElementType
	switch line.LineType {
	case model.LineTypeBlockStart:
		required = model.ElementTypeBlockOpen
	case model.LineTypeBlockEnd:
		required = model.ElementTypeBlockClose
	case model.LineTypeInvalid:
		required = model.ElementTypeUnparsed
	default:
		return nil
	}
	for _, element := range line.Elements {
		if element.ElementType == required {
			return nil
		}
	}
	context.Line, context.Column = 0, 0
	if line.Number > 0 {
		context.Line = line.Number
	}
	return inputError(CodeContract, fmt.Sprintf("Строка типа %s должна содержать элемент %s.", line.LineType, required),
		"elements", context, nil)
}
