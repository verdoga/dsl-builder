package parsed

import (
	"fmt"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// indexDiagnostics проверяет непустые уникальные id и возвращает индекс
// diagnostic.id -> позиция в исходном массиве, не меняя его порядок.
func indexDiagnostics(diagnostics []model.Diagnostic, context diagnostic.Context) (map[string]int, []*diagnostic.Error) {
	index := make(map[string]int, len(diagnostics))
	var issues []*diagnostic.Error
	for i, item := range diagnostics {
		field := fmt.Sprintf("diagnostics[%d].id", i)
		if item.ID == "" {
			issues = append(issues, inputError(CodeContract, "ID диагностики не должен быть пустым.", field, context, nil))
		} else if _, exists := index[item.ID]; exists {
			issues = append(issues, inputError(CodeContract, "ID диагностики повторяется.", field, context, nil))
		} else {
			index[item.ID] = i
		}
	}
	return index, issues
}

// validateDiagnostic проверяет severity/scope по model, fatal только у error,
// существование source в processing. Code и tool не закрываются словарём.
// Location для element обязателен, для line при наличии лежит в одной строке;
// block допускает несколько строк. RelatedLocations не повторяют основную
// location и друг друга. Диапазоны делегируются validateLocation.
func validateDiagnostic(item model.Diagnostic, processing map[string]int, context diagnostic.Context) []*diagnostic.Error {
	context.Line, context.Column = 0, 0
	if item.Location != nil && item.Location.Start.Line > 0 {
		context.Line = item.Location.Start.Line
		if item.Location.Start.Column > 0 {
			context.Column = item.Location.Start.Column
		}
	}
	var issues []*diagnostic.Error
	add := func(reason, field string) {
		issues = append(issues, inputError(CodeContract, reason, "diagnostics[]."+field, context, nil))
	}
	if _, exists := processing[item.Source]; !exists {
		add("Источник диагностики отсутствует в processing.", "source")
	}
	switch item.SeverityLevel {
	case model.SeverityError, model.SeverityWarning, model.SeverityRecommendation:
	default:
		add("Уровень серьёзности диагностики не предусмотрен контрактом.", "severity")
	}
	switch item.DiagnosticScope {
	case model.ScopeElement, model.ScopeLine, model.ScopeBlock, model.ScopeDocument:
	default:
		add("Область диагностики не предусмотрена контрактом.", "scope")
	}
	if item.Fatal && item.SeverityLevel != model.SeverityError {
		add("Признак fatal допустим только для диагностики уровня error.", "fatal")
	}
	if item.Location == nil {
		if item.DiagnosticScope == model.ScopeElement {
			add("Диагностика элемента должна иметь основной диапазон.", "location")
		}
	} else {
		if issue := validateLocation(*item.Location, context); issue != nil {
			issues = append(issues, issue)
		}
		if item.DiagnosticScope == model.ScopeLine && item.Location.Start.Line != item.Location.End.Line {
			add("Основной диапазон диагностики строки должен находиться в одной строке.", "location")
		}
	}
	seen := make(map[model.Location]bool, len(item.RelatedLocations)+1)
	if item.Location != nil {
		seen[*item.Location] = true
	}
	for i, location := range item.RelatedLocations {
		field := fmt.Sprintf("relatedLocations[%d]", i)
		if issue := validateLocation(location, context); issue != nil {
			issue.Details = append(issue.Details, diagnostic.Detail{Label: "Поле JSON", Value: "diagnostics[]." + field})
			issues = append(issues, issue)
		}
		if seen[location] {
			add("Связанный диапазон повторяет основной или другой связанный диапазон.", field)
		}
		seen[location] = true
	}
	return issues
}

// validateLocation проверяет положительные строки/колонки и неубывание
// концов диапазона по строке, затем колонке. Конец исключающий, пустой
// диапазон допустим. Нового типа позиции или координат не вводится.
func validateLocation(location model.Location, context diagnostic.Context) *diagnostic.Error {
	if location.Start.Line <= 0 || location.Start.Column <= 0 || location.End.Line <= 0 || location.End.Column <= 0 {
		return inputError(CodeContract, "Строки и колонки диапазона должны быть положительными.", "location", context, nil)
	}
	if location.End.Line < location.Start.Line || location.End.Line == location.Start.Line && location.End.Column < location.Start.Column {
		return inputError(CodeContract, "Исключающий конец диапазона не должен предшествовать его началу.", "location", context, nil)
	}
	return nil
}

// validateDiagnosticOrder проверяет порядок известных source в processing,
// затем доступных начальных строк и колонок; равные координаты сохраняют
// входной порядок обнаружения. Для location=null новый порядок не навязывается.
func validateDiagnosticOrder(items []model.Diagnostic, processing map[string]int, context diagnostic.Context) *diagnostic.Error {
	previousSource := -1
	var previousStart model.Position
	hasStart := false
	for i, item := range items {
		source, known := processing[item.Source]
		if !known {
			continue
		}
		itemContext := context
		itemContext.Line, itemContext.Column = 0, 0
		if item.Location != nil && item.Location.Start.Line > 0 {
			itemContext.Line = item.Location.Start.Line
			if item.Location.Start.Column > 0 {
				itemContext.Column = item.Location.Start.Column
			}
		}
		if source < previousSource {
			return inputError(CodeContract, "Диагностики должны следовать порядку запусков в processing.", fmt.Sprintf("diagnostics[%d].source", i), itemContext, nil)
		}
		if source != previousSource {
			hasStart = false
		}
		previousSource = source
		if item.Location == nil || item.Location.Start.Line <= 0 || item.Location.Start.Column <= 0 {
			continue
		}
		start := item.Location.Start
		if hasStart && (start.Line < previousStart.Line || start.Line == previousStart.Line && start.Column < previousStart.Column) {
			return inputError(CodeContract, "Диагностики одного запуска должны следовать порядку начальных строк и колонок.", fmt.Sprintf("diagnostics[%d].location.start", i), itemContext, nil)
		}
		previousStart, hasStart = start, true
	}
	return nil
}

// validateErrorReferences проверяет errorIds без повторов, существование
// диагностик scope=element и порядок ссылок по массиву diagnostics.
// Для каждой element-диагностики требует хотя бы одну обратную ссылку
// из элемента с точно совпадающим основным диапазоном.
// Для других scope ссылка из errorIds запрещена; предупреждения не выводятся.
func validateErrorReferences(result *model.Result, diagnostics map[string]int, context diagnostic.Context) []*diagnostic.Error {
	var issues []*diagnostic.Error
	matched := make([]bool, len(result.Diagnostics))
	for i, line := range result.Lines {
		for j, element := range line.Elements {
			elementContext := context
			elementContext.Line, elementContext.Column = 0, 0
			if line.Number > 0 {
				elementContext.Line = line.Number
				if element.Start > 0 {
					elementContext.Column = element.Start
				}
			}
			seen := make(map[string]bool, len(element.ErrorIDs))
			previous := -1
			for k, id := range element.ErrorIDs {
				field := fmt.Sprintf("lines[%d].elements[%d].errorIds[%d]", i, j, k)
				if seen[id] {
					issues = append(issues, inputError(CodeContract, "Ссылка на диагностику повторяется в errorIds.", field, elementContext, nil))
					continue
				}
				seen[id] = true
				position, exists := diagnostics[id]
				if !exists || position < 0 || position >= len(result.Diagnostics) || result.Diagnostics[position].ID != id {
					issues = append(issues, inputError(CodeContract, "Ссылка errorIds указывает на отсутствующую диагностику.", field, elementContext, nil))
					continue
				}
				if position < previous {
					issues = append(issues, inputError(CodeContract, "Ссылки errorIds должны следовать порядку массива diagnostics.", field, elementContext, nil))
				}
				previous = position
				item := result.Diagnostics[position]
				if item.DiagnosticScope != model.ScopeElement {
					issues = append(issues, inputError(CodeContract, "Ссылка errorIds допустима только на диагностику области element.", field, elementContext, nil))
					continue
				}
				if item.Location != nil && item.Location.Start.Line == line.Number && item.Location.End.Line == line.Number &&
					item.Location.Start.Column == element.Start && item.Location.End.Column == element.End {
					matched[position] = true
				}
			}
		}
	}
	for i, item := range result.Diagnostics {
		if item.DiagnosticScope != model.ScopeElement || matched[i] {
			continue
		}
		itemContext := context
		itemContext.Line, itemContext.Column = 0, 0
		if item.Location != nil && item.Location.Start.Line > 0 {
			itemContext.Line = item.Location.Start.Line
			if item.Location.Start.Column > 0 {
				itemContext.Column = item.Location.Start.Column
			}
		}
		issues = append(issues, inputError(CodeContract, "Диагностика элемента должна иметь обратную ссылку из элемента с тем же основным диапазоном.", fmt.Sprintf("diagnostics[%d].location", i), itemContext, nil))
	}
	return issues
}

// validateErrorFlags проверяет document.hasErrors как наличие severity=error
// и line.hasErrors как error с основной location на этой строке либо ссылку
// элемента на error; дочерние ошибки не наследуются. Это проверка Contract,
// не замена независимой предварительной блокировки в parserErrors.
func validateErrorFlags(result *model.Result, context diagnostic.Context) []*diagnostic.Error {
	hasErrors := false
	errorIDs := make(map[string]bool)
	errorLines := make(map[int]bool)
	for _, item := range result.Diagnostics {
		if item.SeverityLevel == model.SeverityError {
			hasErrors = true
			errorIDs[item.ID] = true
			if item.Location != nil {
				errorLines[item.Location.Start.Line] = true
			}
		}
	}
	var issues []*diagnostic.Error
	if result.Document.HasErrors != hasErrors {
		issues = append(issues, inputError(CodeContract, "Признак document.hasErrors должен совпадать с наличием диагностики уровня error.", "document.hasErrors", context, nil))
	}
	for i, line := range result.Lines {
		expected := errorLines[line.Number]
		for _, element := range line.Elements {
			for _, id := range element.ErrorIDs {
				expected = expected || errorIDs[id]
			}
		}
		if line.HasErrors != expected {
			lineContext := context
			lineContext.Line, lineContext.Column = 0, 0
			if line.Number > 0 {
				lineContext.Line = line.Number
			}
			issues = append(issues, inputError(CodeContract, "Признак line.hasErrors должен совпадать с наличием собственной ошибки строки или ссылки элемента на error.", fmt.Sprintf("lines[%d].hasErrors", i), lineContext, nil))
		}
	}
	return issues
}
