package lesson

import "github.com/verdoga/dsl-builder/diagnostic"

const (
	// CodeJSON — синтаксис JSON, отсутствие обязательного поля или неверный JSON-тип.
	CodeJSON = "lesson.json"
	// CodeContract — нарушение ограничений модели и фиксированной схемы.
	CodeContract = "lesson.contract"
	// CodeIDPrefix — ID страницы или ноды принадлежит другому уроку.
	CodeIDPrefix = "lesson.id_prefix"
	// CodeDuplicateID — повтор полного ID страницы или ноды в пределах урока.
	CodeDuplicateID = "lesson.duplicate_id"
)

// contractError создаёт diagnostic.Error с русской причиной, переданным
// контекстом и подробностью «Поле JSON» со значением pointer по RFC 6901.
// Для повторов ID вызывающий код добавляет оба указателя и полный ID;
// при известной странице Context.PageID заполняется до вызова.
// Ошибки identity сохраняют свой код и исходное значение в Details.
// Содержимое Base64 и текстовых блоков в диагностике целиком не повторяется.
func contractError(code string, reason string, pointer string, context diagnostic.Context, cause error) *diagnostic.Error {
	return &diagnostic.Error{
		Code:    code,
		Reason:  reason,
		Context: context,
		Details: []diagnostic.Detail{{Label: "Поле JSON", Value: pointer}},
		Cause:   cause,
	}
}
