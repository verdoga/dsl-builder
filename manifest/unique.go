package manifest

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// validateUniqueEntries проверяет только uniqueItems одного массива lessons.
// Равны целые объекты по equalEntry, не отдельные ID, fileName или веса.
// Детали ошибки содержат JSON Pointer всех повторяющихся элементов.
func validateUniqueEntries(values []Entry, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	grouped := make([]bool, len(values))
	for i := range values {
		if grouped[i] {
			continue
		}
		indices := []int{i}
		for j := i + 1; j < len(values); j++ {
			if !grouped[j] && equalEntry(values[i], values[j]) {
				indices = append(indices, j)
				grouped[j] = true
			}
		}
		if len(indices) == 1 {
			continue
		}
		problem := &diagnostic.Error{
			Code:    CodeUniqueItems,
			Reason:  "Массив lessons содержит полностью одинаковые записи урока.",
			Context: context,
		}
		for _, index := range indices {
			entryContext := context
			if values[index].FileName != "" {
				entryContext.FileName = values[index].FileName
			}
			if values[index].ID != "" {
				entryContext.LessonID = values[index].ID
			}
			problem.Related = append(problem.Related, entryContext)
			problem.Details = append(problem.Details, diagnostic.Detail{
				Label: "JSON Pointer", Value: pointer + "/" + strconv.Itoa(index),
			})
		}
		problem.Context = problem.Related[0]
		problems = append(problems, problem)
	}
	return problems
}

// validateUniqueSections проверяет uniqueItems одного массива sections.
// Равны полное название и содержимое lessons с учётом порядка записей.
// Секции с одинаковым названием и разными lessons не равны по этой проверке.
func validateUniqueSections(values []Section, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	grouped := make([]bool, len(values))
	for i := range values {
		if grouped[i] {
			continue
		}
		indices := []int{i}
		for j := i + 1; j < len(values); j++ {
			if !grouped[j] && equalSection(values[i], values[j]) {
				indices = append(indices, j)
				grouped[j] = true
			}
		}
		if len(indices) == 1 {
			continue
		}
		problem := &diagnostic.Error{
			Code:    CodeUniqueItems,
			Reason:  "Массив sections содержит полностью одинаковые секции.",
			Context: context,
		}
		for _, index := range indices {
			problem.Details = append(problem.Details, diagnostic.Detail{
				Label: "JSON Pointer", Value: pointer + "/" + strconv.Itoa(index),
			})
			for _, entry := range values[index].Lessons {
				entryContext := context
				if entry.FileName != "" {
					entryContext.FileName = entry.FileName
				}
				if entry.ID != "" {
					entryContext.LessonID = entry.ID
				}
				problem.Related = append(problem.Related, entryContext)
			}
		}
		problems = append(problems, problem)
	}
	return problems
}

// equalEntry сравнивает все восемь полей по равенству JSON-значений:
// строки побуквенно и с регистром, null отдельно от строки, целые численно
// без потери точности. Строки order 1 и 01 различны; числа 1 и 1.0 равны.
// Предусловие: обе записи прошли проверки скаляров; сравнение не меняет их.
func equalEntry(left Entry, right Entry) bool {
	if left.ID != right.ID || left.FileName != right.FileName ||
		left.Title != right.Title || left.CreatedAt != right.CreatedAt {
		return false
	}
	if (left.Order == nil) != (right.Order == nil) ||
		(left.Subtitle == nil) != (right.Subtitle == nil) {
		return false
	}
	if left.Order != nil && *left.Order != *right.Order {
		return false
	}
	if left.Subtitle != nil && *left.Subtitle != *right.Subtitle {
		return false
	}

	// Коэффициент без крайних нулей и показатель задают число без
	// разворачивания степени десяти даже при показателе любой длины.
	normalize := func(raw string) (string, big.Int) {
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
	for _, pair := range [][2]string{
		{string(left.LineCount), string(right.LineCount)},
		{string(left.Version), string(right.Version)},
	} {
		leftCoefficient, leftExponent := normalize(pair[0])
		rightCoefficient, rightExponent := normalize(pair[1])
		if leftCoefficient != rightCoefficient || leftExponent.Cmp(&rightExponent) != 0 {
			return false
		}
	}
	return true
}

// equalSection сравнивает title и последовательность lessons через equalEntry.
// Предусловие: поля обеих секций прошли проверки; массивы не сортируются.
func equalSection(left Section, right Section) bool {
	if left.Title != right.Title || len(left.Lessons) != len(right.Lessons) {
		return false
	}
	for i := range left.Lessons {
		if !equalEntry(left.Lessons[i], right.Lessons[i]) {
			return false
		}
	}
	return true
}
