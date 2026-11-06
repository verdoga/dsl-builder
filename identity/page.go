package identity

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// PageIDParts — разобранные части ID страницы для проверки префикса
// и работы вызывающего пакета с числовым значением номера.
type PageIDParts struct {
	// Prefix — проверенный ID урока в нижнем регистре.
	Prefix string

	// Number — положительный номер ASCII-цифрами без ведущих нулей.
	Number string
}

// ParsePageID проверяет полную каноническую форму prefix-pNNN.
// Prefix соответствует ValidateLessonID и равен своему LessonKey.
// Ограничений DSL для Prefix нет; a:b-p001 и lesson+1-p001 допустимы.
// Последняя часть -p с последующими ASCII-цифрами отделяет номер;
// похожие части внутри Prefix допустимы. Суффикса ревизии у страницы нет.
// Номер положителен, дополнен нулями ровно до минимальной ширины 3:
// p001, p999, p1000 допустимы; p000, p1, p0001 запрещены.
// Ошибка имеет CodeInvalidPageID, результат при успехе не содержит padding.
func ParsePageID(value string) (PageIDParts, *diagnostic.Error) {
	separator := strings.LastIndex(value, "-p")
	reason := ""
	if separator < 0 {
		reason = "ID страницы должен содержать префикс и номер после -p."
	} else {
		prefix := value[:separator]
		number := value[separator+2:]
		switch {
		case ValidateLessonID(prefix) != nil:
			reason = "Префикс ID страницы должен быть допустимым ID урока."
		case prefix != LessonKey(prefix):
			reason = "Префикс ID страницы должен быть в нижнем регистре."
		case len(number) < 3:
			reason = "Номер страницы должен содержать не менее трёх ASCII-цифр."
		default:
			for i := 0; i < len(number); i++ {
				if number[i] < '0' || number[i] > '9' {
					reason = "Номер страницы должен содержать только ASCII-цифры."
					break
				}
			}
			if reason == "" {
				digits := strings.TrimLeft(number, "0")
				if digits == "" || len(number) > 3 && number[0] == '0' {
					reason = "Номер страницы должен быть положительным и иметь каноническое дополнение нулями."
				} else {
					return PageIDParts{Prefix: prefix, Number: digits}, nil
				}
			}
		}
	}
	return PageIDParts{}, &diagnostic.Error{
		Code:   CodeInvalidPageID,
		Reason: reason,
		Details: []diagnostic.Detail{
			{Label: "ID страницы", Value: value},
		},
	}
}

// FormatPageID проверяет lessonID через ValidateLessonID, положительный
// number и возвращает
// LessonKey(lessonID) с -p и номером минимальной ширины 3 по ТЗ 7.2.
// Например, A:B и 1 дают a:b-p001; 1000 не обрезается.
// Ошибки: CodeInvalidLessonID либо CodeInvalidNumber.
func FormatPageID(lessonID string, number string) (string, *diagnostic.Error) {
	if err := ValidateLessonID(lessonID); err != nil {
		return "", err
	}
	reason := ""
	for i := 0; i < len(number); i++ {
		if number[i] < '0' || number[i] > '9' {
			reason = "Номер страницы должен содержать только ASCII-цифры."
			break
		}
	}
	digits := strings.TrimLeft(number, "0")
	if reason == "" && digits == "" {
		reason = "Номер страницы должен быть непустым положительным числом."
	}
	if reason != "" {
		return "", &diagnostic.Error{
			Code:   CodeInvalidNumber,
			Reason: reason,
			Details: []diagnostic.Detail{
				{Label: "Номер страницы", Value: number},
			},
		}
	}
	if len(digits) < 3 {
		digits = strings.Repeat("0", 3-len(digits)) + digits
	}
	return LessonKey(lessonID) + "-p" + digits, nil
}
