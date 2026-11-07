package manifest

import (
	"encoding/json"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// ValidateTitle проверяет корректный UTF-8, minLength 1 и наличие символа
// вне класса пробелов ECMAScript, соответствующего pattern \S схемы.
// Пробелы не обрезаются; ограничения имени секции сюда не добавляются.
// pointer — JSON Pointer проверяемого поля; пригодно для названий курса,
// юнита и урока до включения в полную модель. Успех — nil.
func ValidateTitle(value string, pointer string, context diagnostic.Context) *diagnostic.Error {
	reason := ""
	switch {
	case !utf8.ValidString(value):
		reason = "Название должно быть корректной строкой UTF-8."
	case value == "":
		reason = "Название не должно быть пустым."
	default:
		for _, r := range value {
			// ECMAScript WhiteSpace и LineTerminator: unicode.IsSpace
			// имеет другой набор, в частности для U+0085 и U+FEFF.
			switch {
			case r == '\t', r == '\v', r == '\f', r == '\n', r == '\r',
				r == ' ', r == '\u00a0', r == '\u1680',
				r >= '\u2000' && r <= '\u200a',
				r == '\u2028', r == '\u2029', r == '\u202f',
				r == '\u205f', r == '\u3000', r == '\ufeff':
			default:
				return nil
			}
		}
		reason = "Название должно содержать хотя бы один непробельный символ."
	}
	return &diagnostic.Error{
		Code: CodeTitle, Reason: reason, Context: context,
		Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
	}
}

// ValidateSectionTitle применяет ValidateTitle и точное отрицание pattern схемы:
// запрещены крайние U+0020, два соседних U+0020, TAB, CR и LF в любом месте.
// Другие пробелы не запрещаются сверх ограничений схемы; имя не исправляется.
func ValidateSectionTitle(value string, pointer string, context diagnostic.Context) *diagnostic.Error {
	if problem := ValidateTitle(value, pointer, context); problem != nil {
		return problem
	}
	if strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") ||
		strings.Contains(value, "  ") || strings.ContainsAny(value, "\t\r\n") {
		return &diagnostic.Error{
			Code:    CodeTitle,
			Reason:  "Имя секции не должно содержать крайние или повторные обычные пробелы, табуляцию и переводы строк.",
			Context: context,
			Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
		}
	}
	return nil
}

// validateCreatedAt проверяет полную запись YYYY-MM-DDTHH:MM:SSZ:
// четыре цифры года, верхний регистр T/Z, без долей, смещения и лишних знаков.
// Помимо pattern обязательна календарная проверка format date-time по RFC 3339,
// включая допустимость високосной секунды 60; один time.Parse не заменяет её.
// Время не назначается, не округляется и не сверяется с текущими часами.
func validateCreatedAt(value string, pointer string, context diagnostic.Context) *diagnostic.Error {
	problem := &diagnostic.Error{
		Code:    CodeCreatedAt,
		Reason:  "Ожидается календарно допустимая дата UTC в формате YYYY-MM-DDTHH:MM:SSZ по RFC 3339.",
		Context: context,
		Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
	}
	if len(value) != 20 || value[4] != '-' || value[7] != '-' ||
		value[10] != 'T' || value[13] != ':' || value[16] != ':' || value[19] != 'Z' {
		return problem
	}
	for i := range len(value) {
		if i == 4 || i == 7 || i == 10 || i == 13 || i == 16 || i == 19 {
			continue
		}
		if value[i] < '0' || value[i] > '9' {
			return problem
		}
	}

	calendarValue := value
	leapSecond := value[17:19] == "60"
	if leapSecond {
		// time.Parse не принимает 60: календарь проверяется на секунде 59,
		// а допустимость вставки — отдельно по RFC 3339 §5.7 и приложению D.
		calendarValue = value[:17] + "59Z"
	}
	if _, err := time.Parse(time.RFC3339, calendarValue); err != nil {
		problem.Cause = err
		return problem
	}
	if leapSecond {
		confirmed := false
		// Подтверждённые положительные вставки UTC, полный список по 2016:
		// RFC 3339, приложение D, и IERS-derived tz/leap-seconds.list,
		// Git blob 0b5b000f8eddb78d779471f38a46b3f1260281c6.
		// Начальное смещение 1972-01-01 не является вставкой секунды.
		switch value[:10] {
		case "1972-06-30", "1972-12-31", "1973-12-31", "1974-12-31",
			"1975-12-31", "1976-12-31", "1977-12-31", "1978-12-31",
			"1979-12-31", "1981-06-30", "1982-06-30", "1983-06-30",
			"1985-06-30", "1987-12-31", "1989-12-31", "1990-12-31",
			"1992-06-30", "1993-06-30", "1994-06-30", "1995-12-31",
			"1997-06-30", "1998-12-31", "2005-12-31", "2008-12-31",
			"2012-06-30", "2015-06-30", "2016-12-31":
			confirmed = true
		}
		if !confirmed || value[11:16] != "23:59" {
			problem.Reason = "Секунда 60 допустима только в подтверждённый момент вставки секунды координации UTC."
			return problem
		}
	}
	return nil
}

// validateInteger проверяет непустой корректный JSON number, математическую
// целочисленность и нижнюю границу minimum (0 либо 1) без float64 и int64.
// Целые записи с дробной частью из нулей или экспонентой допустимы по схеме;
// исходная запись не нормализуется. Ограничения длины/верхней границы нет.
func validateInteger(value json.Number, minimum int, pointer string, context diagnostic.Context) *diagnostic.Error {
	raw := string(value)
	problem := &diagnostic.Error{
		Code:    CodeInteger,
		Reason:  "Ожидается корректное JSON-число с целым значением не меньше " + strconv.Itoa(minimum) + ".",
		Context: context,
		Details: []diagnostic.Detail{{Label: "JSON Pointer", Value: pointer}},
	}
	valid, _ := regexp.MatchString(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`, raw)
	if !valid {
		return problem
	}
	negative := raw[0] == '-'
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
	if strings.Trim(coefficient, "0") == "" {
		if minimum == 0 {
			return nil
		}
		return problem
	}
	if negative {
		return problem
	}

	// Ненулевое положительное значение целое тогда и только тогда, когда
	// показатель покрывает дробные разряды за вычетом конечных нулей.
	// Само число и степень десяти не разворачиваются, даже при огромном
	// показателе. Порог зависит только от длины имеющейся строки.
	trailingZeros := len(coefficient) - len(strings.TrimRight(coefficient, "0"))
	var threshold big.Int
	threshold.SetString(strconv.Itoa(fractionDigits-trailingZeros), 10)
	if exponent.Cmp(&threshold) < 0 {
		return problem
	}
	return nil
}
