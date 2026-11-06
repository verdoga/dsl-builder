package identity

import (
	"strings"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// ValidateLessonID проверяет непустой ID в корректном UTF-8 по Manifest.id:
// ^[^\s\u0000-\u001F\u007F]+(?![\s\S]) — pattern после декодирования JSON.
// Во всей строке запрещены только символы класса \s в семантике ECMAScript
// этой схемы, U+0000–U+001F и U+007F; набор не расширяется до всех
// Unicode-управляющих символов и не подменяется классами Go regexp.
// Остальные символы допустимы, включая двоеточие и плюс: A:B и lesson+1.
// Белый список DSL, ограничения длины и первого символа не применяются.
// Строка не обрезается, не исправляется и сохраняет исходный регистр.
// Отсутствие поля и null проверяет parsed; ID здесь не генерируется.
// Пустота, неверный UTF-8 или нарушение pattern дают CodeInvalidLessonID.
func ValidateLessonID(value string) *diagnostic.Error {
	reason := ""
	switch {
	case value == "":
		reason = "ID урока не должен быть пустым."
	case !utf8.ValidString(value):
		reason = "ID урока должен быть корректной строкой UTF-8."
	default:
		for _, r := range value {
			switch {
			case r <= '\u001f', r == '\u007f',
				r == ' ', r == '\u00a0', r == '\u1680',
				r >= '\u2000' && r <= '\u200a',
				r == '\u2028', r == '\u2029', r == '\u202f',
				r == '\u205f', r == '\u3000', r == '\ufeff':
				reason = "ID урока содержит запрещённый пробельный или управляющий символ."
			}
			if reason != "" {
				break
			}
		}
	}
	if reason != "" {
		return &diagnostic.Error{
			Code:   CodeInvalidLessonID,
			Reason: reason,
			Details: []diagnostic.Detail{
				{Label: "ID урока", Value: value},
			},
		}
	}
	return nil
}

// LessonKey возвращает ключ идентичности проверенного ID урока:
// strings.ToLower(value), без Unicode-нормализации и системной локали.
// Равенство ключей задаёт сравнение без учёта регистра по DSL 1.10.
// Это также префикс генерируемых ID страниц и нод по ТЗ 7.2.
// Ключ не заменяет исходное написание document.id или manifest.id.
// Проверку value предварительно выполняет ValidateLessonID.
func LessonKey(value string) string {
	return strings.ToLower(value)
}
