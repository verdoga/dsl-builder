package parsed

import (
	"fmt"
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// parserErrors независимо собирает четыре блокирующих признака ТЗ 5.3:
// document.hasErrors, severity=error, line.type=invalid, element.type=unparsed.
// Несогласованность признаков не отменяет ни одного срабатывания.
// Порядок: признак документа, error-диагностики в исходном порядке,
// затем invalid/unparsed по строкам и элементам. Warning/recommendation
// не включаются в пользовательские сообщения или Details.
// При true без доступной error-диагностики всё равно возвращается ошибка.
func parserErrors(result *model.Result, context diagnostic.Context) []*diagnostic.Error {
	var errors []*diagnostic.Error
	var diagnostics []model.Diagnostic
	for _, item := range result.Diagnostics {
		if item.SeverityLevel == model.SeverityError {
			diagnostics = append(diagnostics, item)
		}
	}
	if result.Document.HasErrors {
		errors = append(errors, parserMarker("document.hasErrors=true", context, diagnostics))
	}
	for _, item := range diagnostics {
		errors = append(errors, parserDiagnostic(item, context))
	}
	for _, line := range result.Lines {
		lineContext := context
		lineContext.Line, lineContext.Column = line.Number, 0
		if line.LineType == model.LineTypeInvalid {
			ids := make(map[string]bool)
			for _, element := range line.Elements {
				for _, id := range element.ErrorIDs {
					ids[id] = true
				}
			}
			var related []model.Diagnostic
			for _, item := range diagnostics {
				if ids[item.ID] || item.Location != nil && item.Location.Start.Line == line.Number {
					related = append(related, item)
				}
			}
			errors = append(errors, parserMarker("lines[].type=invalid", lineContext, related))
		}
		for _, element := range line.Elements {
			if element.ElementType != model.ElementTypeUnparsed {
				continue
			}
			ids := make(map[string]bool)
			for _, id := range element.ErrorIDs {
				ids[id] = true
			}
			var related []model.Diagnostic
			for _, item := range diagnostics {
				if ids[item.ID] {
					related = append(related, item)
				}
			}
			elementContext := lineContext
			elementContext.Column = element.Start
			errors = append(errors, parserMarker("lines[].elements[].type=unparsed", elementContext, related))
		}
	}
	return errors
}

// parserDiagnostic преобразует только severity=error в diagnostic.Error.
// Сохраняет code/message/source/id/fatal в Details, начало location
// в Context, дополнительные начала диапазонов в Related; точные концы
// диапазонов при наличии — в Details. Путь всегда от технического JSON.
// Русская Reason объясняет блокировку, исходное сообщение не переписывается.
func parserDiagnostic(item model.Diagnostic, context diagnostic.Context) *diagnostic.Error {
	if item.SeverityLevel != model.SeverityError {
		return nil
	}
	err := &diagnostic.Error{
		Code:    CodeParser,
		Reason:  "Диагностика парсера уровня error блокирует обработку документа.",
		Context: context,
		Details: []diagnostic.Detail{
			{Label: "Признак ошибки парсера", Value: "diagnostics[].severity=error"},
			{Label: "Код диагностики парсера", Value: string(item.DiagnosticCode)},
			{Label: "Сообщение парсера", Value: item.Message},
			{Label: "Источник диагностики парсера", Value: item.Source},
			{Label: "ID диагностики парсера", Value: item.ID},
			{Label: "Признак fatal", Value: strconv.FormatBool(item.Fatal)},
		},
	}
	if item.Location != nil {
		err.Context.Line = item.Location.Start.Line
		err.Context.Column = item.Location.Start.Column
		err.Details = append(err.Details, diagnostic.Detail{
			Label: "Основной диапазон парсера (конец исключающий)",
			Value: fmt.Sprintf("%d:%d–%d:%d", item.Location.Start.Line, item.Location.Start.Column,
				item.Location.End.Line, item.Location.End.Column),
		})
	}
	for _, location := range item.RelatedLocations {
		related := context
		related.Line, related.Column = location.Start.Line, location.Start.Column
		err.Related = append(err.Related, related)
		err.Details = append(err.Details, diagnostic.Detail{
			Label: "Связанный диапазон парсера (конец исключающий)",
			Value: fmt.Sprintf("%d:%d–%d:%d", location.Start.Line, location.Start.Column,
				location.End.Line, location.End.Column),
		})
	}
	return err
}

// parserMarker оформляет один признак с известными строкой/колонкой.
// related содержит только доступные error-диагностики, связанные с признаком;
// их код и сообщение сохраняются, отсутствующие сведения не выдумываются.
// Для document.hasErrors доступны все error, для invalid — начинающиеся
// на строке либо связанные через её элементы, для unparsed — его errorIds.
func parserMarker(marker string, context diagnostic.Context, related []model.Diagnostic) *diagnostic.Error {
	err := &diagnostic.Error{
		Code:    CodeParser,
		Reason:  "Признак ошибки парсера блокирует обработку документа.",
		Context: context,
		Details: []diagnostic.Detail{{Label: "Признак ошибки парсера", Value: marker}},
	}
	for _, item := range related {
		// Даже прямой вызов не должен включать предупреждения и рекомендации.
		converted := parserDiagnostic(item, context)
		if converted == nil {
			continue
		}
		if item.Location != nil {
			err.Related = append(err.Related, converted.Context)
		}
		err.Related = append(err.Related, converted.Related...)
		err.Details = append(err.Details, converted.Details...)
	}
	return err
}
