package identity

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// ParseFileName проверяет техническое имя урока по Manifest.schema:
// строчная ASCII l, положительный номер минимальной ширины 3 и .json.
// Допустимы l001.json, l999.json, l1000.json; запрещены l000.json,
// l01.json, l0001.json, иной регистр, разделители путей и лишние символы.
// Возвращает номер ASCII-цифрами без ведущих нулей, любой длины.
// manifest.json не является техническим именем урока.
// Ошибка имеет CodeInvalidFileName, числовой результат тогда пуст.
func ParseFileName(value string) (string, *diagnostic.Error) {
	if len(value) >= 9 && value[0] == 'l' && strings.HasSuffix(value, ".json") {
		digits := value[1 : len(value)-5]
		valid := len(digits) == 3 || digits[0] != '0'
		for i := 0; i < len(digits); i++ {
			if digits[i] < '0' || digits[i] > '9' {
				valid = false
				break
			}
		}
		number := strings.TrimLeft(digits, "0")
		if valid && number != "" {
			return number, nil
		}
	}
	return "", &diagnostic.Error{
		Code:   CodeInvalidFileName,
		Reason: "Техническое имя урока должно содержать строчную l, положительный номер минимальной ширины 3 и расширение .json.",
		Details: []diagnostic.Detail{
			{Label: "Техническое имя", Value: value},
		},
	}
}

// FormatFileName проверяет положительный number и выдаёт техническое имя
// со строчной l, номером минимальной ширины 3 и расширением .json.
// Ведущие нули входного числа заменяются каноническим padding;
// номер не выбирается и не увеличивается. Ошибка имеет CodeInvalidNumber.
func FormatFileName(number string) (string, *diagnostic.Error) {
	valid := true
	for i := 0; i < len(number); i++ {
		if number[i] < '0' || number[i] > '9' {
			valid = false
			break
		}
	}
	digits := strings.TrimLeft(number, "0")
	if !valid || digits == "" {
		return "", &diagnostic.Error{
			Code:   CodeInvalidNumber,
			Reason: "Номер технического имени должен быть положительным целым числом из ASCII-цифр.",
			Details: []diagnostic.Detail{
				{Label: "Номер технического имени", Value: number},
			},
		}
	}
	if len(digits) < 3 {
		digits = strings.Repeat("0", 3-len(digits)) + digits
	}
	return "l" + digits + ".json", nil
}
