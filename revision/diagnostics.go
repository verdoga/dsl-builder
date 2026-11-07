package revision

import "github.com/verdoga/dsl-builder/diagnostic"

const (
	// CodeInput — отсутствует обязательная модель первичного урока.
	CodeInput = "revision.input"
	// CodeLessonMismatch — старый урок и кандидат имеют разные ключи ID.
	CodeLessonMismatch = "revision.lesson_mismatch"
	// CodePlan — план отсутствует либо содержит неисполнимые адреса или форму.
	CodePlan = "revision.plan"
	// CodeDecision — набор отдельных решений неполон или содержит неизвестное.
	CodeDecision = "revision.decision"
	// CodeHistory — базовые номера или суффиксы противоречат текущей ревизии.
	CodeHistory = "revision.history"
)

// revisionError создаёт diagnostic.Error с русской причиной, известным
// контекстом и JSON Pointer либо адресом поля плана в Details.
// При конфликте вызывающий шаг дополняет Related и Details обоими местами.
// Ошибки lesson и identity передаются с исходным Code, дополненным
// контекстом снимка; Base64 и весь JSON ноды в сообщение не включаются.
func revisionError(code string, reason string, pointer string, context diagnostic.Context) *diagnostic.Error {
	return &diagnostic.Error{
		Code:    code,
		Reason:  reason,
		Context: context,
		Details: []diagnostic.Detail{{Label: "Поле JSON", Value: pointer}},
	}
}
