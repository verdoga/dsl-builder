package parsed

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/order"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// validateContract координирует проверки Contract и возвращает только полный
// индекс. Независимые ошибки следуют порядку полей и исходных массивов.
func validateContract(result *model.Result, context diagnostic.Context) (LineIndex, []*diagnostic.Error) {
	if result == nil {
		return LineIndex{}, []*diagnostic.Error{inputError(CodeContract, "Отсутствует результат обработки документа.", "", context, nil)}
	}
	var issues []*diagnostic.Error
	fields := make(map[*diagnostic.Error]string)
	collect := func(prefix string, found []*diagnostic.Error) {
		for _, issue := range found {
			field := ""
			for _, detail := range issue.Details {
				if detail.Label == "Поле JSON" {
					field = detail.Value
				}
			}
			if strings.Contains(prefix, "].elements[") {
				field = strings.TrimPrefix(field, "elements.")
				if field == "" {
					field = "value"
				}
			}
			if strings.HasPrefix(field, "diagnostics[].") {
				field = prefix + strings.TrimPrefix(field, "diagnostics[]")
			} else if field != "formatVersion" && !strings.HasPrefix(field, "document.") &&
				!strings.HasPrefix(field, "processing[") && !strings.HasPrefix(field, "lines[") &&
				!strings.HasPrefix(field, "diagnostics[") {
				field = prefix + "." + field
			}
			fields[issue] = field
			issues = append(issues, issue)
		}
	}
	if result.Format != model.FormatVersion1 {
		collect("", []*diagnostic.Error{inputError(CodeVersion, "Неподдерживаемая версия входного контракта.", "formatVersion", context, nil)})
	}
	collect("document", validateDocument(result.Document, context))
	collect("document.metadata", validateMetadata(result.Document.Metadata, context))
	processing, processingIssues := validateProcessing(result.Processing, context)
	collect("processing", processingIssues)
	byNumber, numberIssues := indexNumbers(result.Lines, result.Document.LineCount, context)
	collect("lines", numberIssues)
	resolvable := true
	counts := make(map[int]int, len(result.Lines))
	for _, line := range result.Lines {
		counts[line.Number]++
	}
	for i, line := range result.Lines {
		prefix := fmt.Sprintf("lines[%d]", i)
		// Строковые поля проверяем отдельно, чтобы адрес каждого элемента
		// задавался индексом массива, даже при одинаковых или неверных колонках.
		lineOnly := line
		lineOnly.Elements = nil
		for _, issue := range validateLine(lineOnly, i == len(result.Lines)-1, context) {
			// Проверка границы без элементов не относится к исходной строке;
			// ниже она вызывается с полным массивом.
			if !slices.Contains(issue.Details, diagnostic.Detail{Label: "Поле JSON", Value: "elements"}) {
				collect(prefix, []*diagnostic.Error{issue})
			}
		}
		for j, element := range line.Elements {
			collect(fmt.Sprintf("%s.elements[%d]", prefix, j), validateElement(element, line, context))
		}
		lineContext := context
		lineContext.Line, lineContext.Column = 0, 0
		if line.Number > 0 {
			lineContext.Line = line.Number
		}
		if issue := validateElementOrder(line.Elements, lineContext); issue != nil {
			collect(prefix, []*diagnostic.Error{issue})
		}
		if issue := validateBoundaryElements(line, context); issue != nil {
			collect(prefix, []*diagnostic.Error{issue})
		}
		if issue := validateParent(line, result.Lines, byNumber, context); issue != nil {
			collect(prefix, []*diagnostic.Error{issue})
		}
		if line.Number <= 0 || counts[line.Number] != 1 ||
			line.ParentLine != nil && (*line.ParentLine <= 0 || counts[*line.ParentLine] != 1) {
			resolvable = false
		}
	}
	if resolvable {
		for _, issue := range validateParentCycles(result.Lines, byNumber, context) {
			collect(fmt.Sprintf("lines[%d]", byNumber[issue.Context.Line]), []*diagnostic.Error{issue})
		}
	}
	diagnostics, diagnosticIssues := indexDiagnostics(result.Diagnostics, context)
	collect("diagnostics", diagnosticIssues)
	for i, item := range result.Diagnostics {
		collect(fmt.Sprintf("diagnostics[%d]", i), validateDiagnostic(item, processing, context))
	}
	if issue := validateDiagnosticOrder(result.Diagnostics, processing, context); issue != nil {
		collect("diagnostics", []*diagnostic.Error{issue})
	}
	collect("", validateErrorReferences(result, diagnostics, context))
	collect("", validateErrorFlags(result, context))
	// Поля коротких ошибок соседних проверок дополняются только для сортировки.
	// Исходные Context и Details не изменяются.
	orders := map[string][]string{
		"":            {"formatVersion", "document", "processing", "lines", "diagnostics"},
		"document":    {"dslVersion", "fileName", "filePath", "encoding", "hasBom", "lineCount", "byteLength", "sha256", "metadata", "hasErrors"},
		"metadata":    {"documentId", "title", "subtitle", "section", "order", "resourceDirs"},
		"processing":  {"id", "tool", "version", "startedAt", "durationMs"},
		"lines":       {"line", "type", "nestingLevel", "parentLine", "raw", "eol", "hasErrors", "elements"},
		"elements":    {"type", "raw", "value", "start", "end", "errorIds"},
		"diagnostics": {"id", "source", "code", "severity", "message", "scope", "fatal", "location", "relatedLocations"},
		"location":    {"start", "end"}, "relatedLocations": {"start", "end"},
		"start": {"line", "column"}, "end": {"line", "column"},
	}
	key := func(field string) []int {
		parts := strings.FieldsFunc(field, func(r rune) bool { return r == '.' || r == '[' || r == ']' || r == '/' })
		var result []int
		scope := ""
		for _, part := range parts {
			if n, err := strconv.Atoi(part); err == nil {
				result = append(result, n)
				continue
			}
			position := slices.Index(orders[scope], part)
			if position < 0 {
				position = len(orders[scope])
			}
			result = append(result, position)
			scope = part
		}
		return result
	}
	slices.SortStableFunc(issues, func(a, b *diagnostic.Error) int { return slices.Compare(key(fields[a]), key(fields[b])) })
	if len(issues) != 0 {
		return LineIndex{}, issues
	}
	return LineIndex{ByNumber: byNumber, Children: indexChildren(result.Lines)}, nil
}

// validateDocument проверяет известные размеры, UTF-8 и SHA-256, сохраняя null.
// Исходный DSL не открывается, его версия и строки не пересчитываются.
func validateDocument(document model.Document, context diagnostic.Context) []*diagnostic.Error {
	var issues []*diagnostic.Error
	if issue := validateSourceName(document, context); issue != nil {
		issues = append(issues, issue)
	}
	if document.Encoding != nil && *document.Encoding != "UTF-8" {
		issues = append(issues, inputError(CodeContract, "Известная кодировка должна быть UTF-8.", "document.encoding", context, nil))
	}
	if document.LineCount != nil && *document.LineCount < 0 {
		issues = append(issues, inputError(CodeContract, "Число физических строк не может быть отрицательным.", "document.lineCount", context, nil))
	}
	if document.ByteLength != nil && *document.ByteLength < 0 {
		issues = append(issues, inputError(CodeContract, "Размер исходника в байтах не может быть отрицательным.", "document.byteLength", context, nil))
	}
	if document.SHA256 != nil {
		valid := len(*document.SHA256) == 64
		for _, r := range *document.SHA256 {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
				valid = false
			}
		}
		if !valid {
			issues = append(issues, inputError(CodeContract, "SHA-256 должен содержать 64 строчных шестнадцатеричных символа.", "document.sha256", context, nil))
		}
	}
	return issues
}

// validateSourceName проверяет абсолютность Windows-пути и точное совпадение
// последнего компонента с fileName без проверки существования исходника.
func validateSourceName(document model.Document, context diagnostic.Context) *diagnostic.Error {
	if document.FilePath == nil {
		return nil
	}
	path := strings.ReplaceAll(*document.FilePath, "/", `\`)
	drive := len(path) >= 3 && (path[0] >= 'A' && path[0] <= 'Z' || path[0] >= 'a' && path[0] <= 'z') && path[1:3] == `:\`
	unc := false
	if strings.HasPrefix(path, `\\`) {
		parts := strings.Split(path[2:], `\`)
		unc = len(parts) >= 3 && parts[0] != "" && parts[1] != ""
	}
	if !drive && !unc {
		return inputError(CodeContract, "Путь исходного DSL должен быть абсолютным путём Windows.", "document.filePath", context, nil)
	}
	name := path[strings.LastIndex(path, `\`)+1:]
	if name == "" || document.FileName == nil || name != *document.FileName {
		return inputError(CodeContract, "Имя исходного DSL должно совпадать с последним компонентом filePath.", "document.fileName", context, nil)
	}
	return nil
}

// validateMetadata проверяет обязательный ID и заданный вес через владельцев
// правил. Свободные строки, ресурсные пути, регистр и null сохраняются.
func validateMetadata(metadata model.DocumentMetadata, context diagnostic.Context) []*diagnostic.Error {
	var issues []*diagnostic.Error
	if metadata.DocumentID == nil {
		issues = append(issues, inputError(CodeMissingID, "Обязательный ID урока отсутствует или равен null.", "document.metadata.documentId", context, nil))
	} else if issue := identity.ValidateLessonID(*metadata.DocumentID); issue != nil {
		issues = append(issues, inputError(issue.Code, issue.Reason, "document.metadata.documentId", context, issue))
	}
	if metadata.Order != nil {
		if issue := order.Validate(*metadata.Order, context); issue != nil {
			issues = append(issues, inputError(issue.Code, issue.Reason, "document.metadata.order", context, issue))
		}
	}
	return issues
}

// validateProcessing проверяет ID, время RFC 3339 с зоной и известную
// длительность. Открытые tool/version и исходный порядок не изменяются.
func validateProcessing(processing []model.Processing, context diagnostic.Context) (map[string]int, []*diagnostic.Error) {
	index := make(map[string]int, len(processing))
	var issues []*diagnostic.Error
	stampForm := regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])[Tt]([01][0-9]|2[0-3]):[0-5][0-9]:([0-5][0-9]|60)(\.[0-9]+)?([Zz]|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)
	for i, run := range processing {
		field := fmt.Sprintf("processing[%d].", i)
		if run.ID == "" {
			issues = append(issues, inputError(CodeContract, "ID запуска инструмента не должен быть пустым.", field+"id", context, nil))
		} else if _, exists := index[run.ID]; exists {
			issues = append(issues, inputError(CodeContract, "ID запуска инструмента повторяется.", field+"id", context, nil))
		} else {
			index[run.ID] = i
		}
		if run.StartedAt != nil {
			stamp := *run.StartedAt
			valid := stampForm.MatchString(stamp)
			var err error
			if valid {
				// Parse не поддерживает lowercase t/z и секунду вставки.
				// Меняется только локальная копия; модель хранит исходную строку.
				candidate := strings.ToUpper(stamp)
				leap := candidate[17:19] == "60"
				if leap {
					candidate = candidate[:17] + "59" + candidate[19:]
				}
				var instant time.Time
				instant, err = time.Parse(time.RFC3339, candidate)
				if err == nil && leap {
					utc := instant.UTC()
					// Реальные вставки происходили после 23:59:59 UTC;
					// смещение может переносить локальную дату на следующий день.
					valid = utc.Hour() == 23 && utc.Minute() == 59 && slices.Contains([]string{
						"1972-06-30", "1972-12-31", "1973-12-31", "1974-12-31", "1975-12-31", "1976-12-31",
						"1977-12-31", "1978-12-31", "1979-12-31", "1981-06-30", "1982-06-30", "1983-06-30",
						"1985-06-30", "1987-12-31", "1989-12-31", "1990-12-31", "1992-06-30", "1993-06-30",
						"1994-06-30", "1995-12-31", "1997-06-30", "1998-12-31", "2005-12-31", "2008-12-31",
						"2012-06-30", "2015-06-30", "2016-12-31",
					}, utc.Format("2006-01-02"))
				}
			}
			if err != nil || !valid {
				issues = append(issues, inputError(CodeContract, "Время начала должно соответствовать RFC 3339 с часовым поясом.", field+"startedAt", context, err))
			}
		}
		if run.DurationMs != nil && *run.DurationMs < 0 {
			issues = append(issues, inputError(CodeContract, "Продолжительность запуска не может быть отрицательной.", field+"durationMs", context, nil))
		}
	}
	return index, issues
}
