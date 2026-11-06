package lesson

import (
	"encoding/base64"
	"encoding/json"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// validateInteger проверяет корректное JSON-число, математическую
// целочисленность и значение не меньше minimum средствами стандартной
// библиотеки, без float64 и предела int. minimum здесь равен 0 или 1.
// Целочисленные дробная и экспоненциальная записи допускаются по JSON Schema;
// проверка не нормализует исходную запись и не выполняет арифметику ревизий.
func validateInteger(value json.Number, minimum int, pointer string, context diagnostic.Context) *diagnostic.Error {
	raw := string(value)
	valid, _ := regexp.MatchString(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`, raw)
	if !valid {
		return contractError(CodeContract, "Ожидается корректное JSON-число", pointer, context, nil)
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
		if minimum == 1 {
			return contractError(CodeContract, "Ожидается целое число не меньше 1", pointer, context, nil)
		}
		return nil
	}
	if negative {
		return contractError(CodeContract, "Ожидается неотрицательное целое число", pointer, context, nil)
	}

	// Для ненулевого коэффициента достаточно сравнить показатель с числом
	// дробных разрядов за вычетом конечных нулей. Степень десяти и само
	// значение числа не создаются, даже при сколь угодно длинном показателе.
	trailingZeros := len(coefficient) - len(strings.TrimRight(coefficient, "0"))
	threshold := big.NewInt(int64(fractionDigits - trailingZeros))
	if exponent.Cmp(threshold) < 0 {
		return contractError(CodeContract, "Ожидается математически целое число", pointer, context, nil)
	}
	return nil
}

// validateCreatedAt проверяет всю строку по pattern схемы и format date-time:
// четыре цифры года, календарную дату, время и правила RFC 3339, UTC с Z,
// секунды без дробной части. Одного регулярного выражения недостаточно.
// Не проверяет, является ли время текущим, и не назначает новое значение.
func validateCreatedAt(value string, pointer string, context diagnostic.Context) *diagnostic.Error {
	valid, _ := regexp.MatchString(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`, value)
	if !valid {
		return contractError(CodeContract, "Ожидается дата UTC в формате YYYY-MM-DDTHH:MM:SSZ", pointer, context, nil)
	}

	// time.Parse не поддерживает секунду координации 60 из RFC 3339 §5.7.
	// Проверяем календарь на секунде 59, затем фактический момент вставки.
	calendarValue := value
	leapSecond := value[17:19] == "60"
	if leapSecond {
		calendarValue = value[:17] + "59Z"
	}
	parsed, err := time.Parse(time.RFC3339, calendarValue)
	if err != nil {
		return contractError(CodeContract, "Некорректная календарная дата или время RFC 3339", pointer, context, err)
	}
	if leapSecond {
		// Подтверждённые положительные вставки: RFC 3339, приложение D,
		// и IERS-derived eggert/tz/leap-seconds.list, Git blob
		// 0b5b000f8eddb78d779471f38a46b3f1260281c6 (полный список по 2016).
		// Начальная разность TAI-UTC на 1972-01-01 не была вставкой.
		confirmed := false
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
		if !confirmed || parsed.Hour() != 23 || parsed.Minute() != 59 {
			return contractError(CodeContract, "Секунда координации допустима только в подтверждённый момент её вставки в UTC", pointer, context, nil)
		}
	}
	return nil
}

// validateEmbeddedFile проверяет имя без пути через validateEmbeddedName
// и Content через validateBase64; MIMEType остаётся строкой по схеме.
// Не выводит Base64 в диагностику, не проверяет содержимое медиаконтейнера
// и не воспроизводит таблицу расширений и MIME из resourcefind.
func validateEmbeddedFile(value EmbeddedFile, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if problem := validateEmbeddedName(value.FileName, pointer+"/fileName", context); problem != nil {
		problems = append(problems, problem)
	}
	if problem := validateBase64(value.Content, pointer+"/content", context); problem != nil {
		problems = append(problems, problem)
	}
	return problems
}

// validateEmbeddedName проверяет требование имени без пути из ТЗ 9.5:
// запрещает разделители / и обратную косую черту и путь с обозначением тома.
// Файловая система не опрашивается; дополнительные требования к длине,
// белому списку символов и расширениям не вводятся.
func validateEmbeddedName(value string, pointer string, context diagnostic.Context) *diagnostic.Error {
	// Windows трактует начальные два байта с двоеточием как том даже
	// для относительной записи C:name; filepath.VolumeName зависит от ОС.
	if strings.ContainsAny(value, `/\`) || (len(value) >= 2 && value[1] == ':') {
		return contractError(CodeContract, "Имя встроенного файла должно быть без пути и обозначения тома", pointer, context, nil)
	}
	return nil
}

// validateBase64 проверяет стандартный алфавит, padding, отсутствие переносов
// и корректность полного кодирования по ТЗ 6.2. Пустая строка допустима
// как кодирование пустого файла. Исходный файл и точность его копии неизвестны.
func validateBase64(value string, pointer string, context diagnostic.Context) *diagnostic.Error {
	if strings.ContainsAny(value, "\r\n") {
		return contractError(CodeContract, "Base64 встроенного файла не должен содержать переносы строк", pointer, context, nil)
	}
	if _, err := base64.StdEncoding.Strict().DecodeString(value); err != nil {
		return contractError(CodeContract, "Некорректное полное кодирование стандартного Base64 с padding", pointer, context, err)
	}
	return nil
}
