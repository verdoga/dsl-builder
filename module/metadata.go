package module

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
)

// validateEntryMetadata проверяет шесть метаданных записи по итоговому уроку:
// ID, Title, Subtitle, LineCount, CreatedAt, Version. Эталон формирует
// catalog.EntryFromLesson с прежними FileName и Order, не дублируя их источники.
// ID, Title, CreatedAt сравниваются точно, включая регистр; Subtitle —
// с различием nil и пустой строки. Совпадение ключей ID не разрешает отличное
// написание манифеста. FileName уже сопоставлен, Order есть только в манифесте.
// Не сверяет корневой Manifest.CreatedAt с датами отдельных уроков.
// Возвращает отдельный CodeMetadata на поле в порядке manifest.Entry,
// с точными значениями записи и урока в Details; не исправляет рассогласование.
func validateEntryMetadata(file *File, item catalog.Item) []*diagnostic.Error {
	entry := item.Entry
	expected := catalog.EntryFromLesson(file.value.Document, entry.FileName, entry.Order)
	context := item.Context
	context.FileName = entry.FileName
	context.LessonID = entry.ID
	var problems []*diagnostic.Error
	appendMismatch := func(field, recorded, actual string) {
		problems = append(problems, &diagnostic.Error{
			Code:    CodeMetadata,
			Reason:  "Поле записи манифеста не совпадает с метаданными урока.",
			Context: context,
			Related: []diagnostic.Context{context, file.Context()},
			Details: []diagnostic.Detail{
				{Label: "Поле", Value: field},
				{Label: "Значение записи манифеста", Value: recorded},
				{Label: "Значение урока", Value: actual},
			},
		})
	}
	if entry.ID != expected.ID {
		appendMismatch("id", entry.ID, expected.ID)
	}
	if entry.Title != expected.Title {
		appendMismatch("title", entry.Title, expected.Title)
	}
	if (entry.Subtitle == nil) != (expected.Subtitle == nil) ||
		(entry.Subtitle != nil && expected.Subtitle != nil && *entry.Subtitle != *expected.Subtitle) {
		// JSON-представление различает null, пустую строку и строку "null".
		recorded, _ := json.Marshal(entry.Subtitle)
		actual, _ := json.Marshal(expected.Subtitle)
		appendMismatch("subtitle", string(recorded), string(actual))
	}
	if !equalInteger(entry.LineCount, expected.LineCount) {
		appendMismatch("lineCount", string(entry.LineCount), string(expected.LineCount))
	}
	if entry.CreatedAt != expected.CreatedAt {
		appendMismatch("createdAt", entry.CreatedAt, expected.CreatedAt)
	}
	if !equalInteger(entry.Version, expected.Version) {
		appendMismatch("version", string(entry.Version), string(expected.Version))
	}
	return problems
}

// equalInteger сравнивает математические значения уже проверенных целых
// json.Number без float64 и машинного предела. Различия 1, 1.0, 1e0 сами
// по себе не дают рассогласования. Это равенство метаданных, не арифметика
// ревизий и не проверка схемы; существующие проверки остаются у владельцев.
func equalInteger(left json.Number, right json.Number) bool {
	// Коэффициент без крайних нулей и показатель задают число без выделения
	// памяти под его полную десятичную запись даже при огромной экспоненте.
	normalize := func(value json.Number) (string, big.Int) {
		raw := string(value)
		negative := strings.HasPrefix(raw, "-")
		coefficient := strings.TrimPrefix(raw, "-")
		var exponent big.Int
		if index := strings.IndexAny(coefficient, "eE"); index >= 0 {
			exponent.SetString(coefficient[index+1:], 10)
			coefficient = coefficient[:index]
		}
		fractionDigits := 0
		if index := strings.IndexByte(coefficient, '.'); index >= 0 {
			fractionDigits = len(coefficient) - index - 1
			coefficient = coefficient[:index] + coefficient[index+1:]
		}
		coefficient = strings.TrimLeft(coefficient, "0")
		if coefficient == "" {
			return "0", big.Int{}
		}
		trimmed := strings.TrimRight(coefficient, "0")
		var adjustment big.Int
		adjustment.SetString(strconv.Itoa(len(coefficient)-len(trimmed)-fractionDigits), 10)
		exponent.Add(&exponent, &adjustment)
		if negative {
			trimmed = "-" + trimmed
		}
		return trimmed, exponent
	}
	leftCoefficient, leftExponent := normalize(left)
	rightCoefficient, rightExponent := normalize(right)
	return leftCoefficient == rightCoefficient && leftExponent.Cmp(&rightExponent) == 0
}
