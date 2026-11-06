package lesson

import (
	"errors"
	"fmt"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestLessonDiagnosticsStableCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"JSON", CodeJSON, "lesson.json"},
		{"contract", CodeContract, "lesson.contract"},
		{"ID prefix", CodeIDPrefix, "lesson.id_prefix"},
		{"duplicate ID", CodeDuplicateID, "lesson.duplicate_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("код = %q, требуется %q", tc.got, tc.want)
			}
		})
	}
}

func TestLessonDiagnosticsPreservesArguments(t *testing.T) {
	sentinel := errors.New("исходная техническая причина")
	fullContext := diagnostic.Context{
		Path:     `C:\Уроки и материалы\..\Архив.ZIP`,
		FileName: "Урок 001.JSON",
		LessonID: "AbCdEfGh",
		PageID:   "AbCdEfGh-001",
		Line:     27,
		Column:   8,
	}
	for _, tc := range []struct {
		name    string
		code    string
		reason  string
		pointer string
		context diagnostic.Context
		cause   error
	}{
		{"root without context", CodeJSON, "Ожидается объект JSON", "", diagnostic.Context{}, nil},
		{"page with cause", CodeContract, "Недопустимый тип данных", "/document/pages/2/nodes/10/data/0/value", fullContext, sentinel},
		{"escaped pointer", CodeIDPrefix, "ID относится к другому уроку", "/document/поле~1имя~0/0", fullContext, fmt.Errorf("обёртка: %w", sentinel)},
		{"known line only", CodeDuplicateID, "Повтор полного ID", "/document/pages/0/id", diagnostic.Context{Line: 12}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.context
			got := contractError(tc.code, tc.reason, tc.pointer, tc.context, tc.cause)
			if got == nil {
				t.Fatal("contractError вернул nil")
			}
			if got.Code != tc.code || got.Reason != tc.reason {
				t.Errorf("Code/Reason = %q/%q, требуется %q/%q", got.Code, got.Reason, tc.code, tc.reason)
			}
			if got.Context != before {
				t.Errorf("Context = %#v, требуется %#v", got.Context, before)
			}
			if tc.context != before {
				t.Errorf("переданный Context изменился: %#v", tc.context)
			}
			if got.Cause != tc.cause || errors.Unwrap(got) != tc.cause {
				t.Error("исходная Cause не сохранена в стандартной цепочке")
			}
			lessonDiagnosticsPointerDetail(t, got, tc.pointer)
			got.Context.Path = "другой путь"
			if tc.context != before {
				t.Error("изменение Context результата затронуло аргумент")
			}
		})
	}
}

func TestLessonDiagnosticsStandardErrorChain(t *testing.T) {
	inner := &diagnostic.Error{
		Code:    "source.invalid",
		Reason:  "Исходная ошибка",
		Details: []diagnostic.Detail{{Label: "Исходное значение", Value: "АбВ~ /"}},
	}
	cause := fmt.Errorf("техническая обёртка: %w", inner)
	got := contractError(CodeContract, "Нарушение контракта урока", "/document/id", diagnostic.Context{}, cause)
	if got == nil {
		t.Fatal("contractError вернул nil")
	}
	if !errors.Is(got, cause) || !errors.Is(got, inner) {
		t.Error("errors.Is не достигает исходной ошибки через обёртку")
	}
	var outer *diagnostic.Error
	if !errors.As(fmt.Errorf("вызывающий код: %w", got), &outer) || outer != got {
		t.Error("errors.As не находит diagnostic.Error урока")
	}
	var original *diagnostic.Error
	if !errors.As(errors.Unwrap(got), &original) || original != inner {
		t.Fatal("исходная типизированная причина потеряна")
	}
	if original.Code != "source.invalid" || original.Details[0].Value != "АбВ~ /" {
		t.Error("исходные код или точное значение причины изменены")
	}
}

func TestLessonDiagnosticsConstructionIsPassiveAndIndependent(t *testing.T) {
	cause := &lessonDiagnosticsObservedCause{}
	context := diagnostic.Context{Path: "несуществующий/урок.json", LessonID: "ИсходныйID"}
	first := contractError(CodeJSON, "Неверный тип поля", "/document/title", context, cause)
	second := contractError(CodeContract, "Отсутствует массив", "/document/pages", context, nil)
	if first == nil || second == nil {
		t.Fatal("contractError вернул nil")
	}
	if cause.errorCalls != 0 || cause.unwrapCalls != 0 {
		t.Errorf("создание диагностики вызвало Cause: Error=%d, Unwrap=%d", cause.errorCalls, cause.unwrapCalls)
	}
	if first.Cause != cause || second.Cause != nil {
		t.Error("причина изменилась между независимыми вызовами")
	}
	index := lessonDiagnosticsPointerDetail(t, first, "/document/title")
	first.Details[index].Value = "/изменено"
	first.Context.LessonID = "ИзменённыйID"
	first.Reason = "Изменённая причина"
	lessonDiagnosticsPointerDetail(t, second, "/document/pages")
	if second.Context != context || second.Reason != "Отсутствует массив" || second.Code != CodeContract {
		t.Error("изменение первой ошибки затронуло независимую вторую ошибку")
	}
}

func lessonDiagnosticsPointerDetail(t *testing.T, err *diagnostic.Error, pointer string) int {
	t.Helper()
	for i, detail := range err.Details {
		if detail.Label == "Поле JSON" && detail.Value == pointer {
			return i
		}
	}
	t.Fatalf("нет подробности «Поле JSON» с точным указателем %q: %#v", pointer, err.Details)
	return -1
}

type lessonDiagnosticsObservedCause struct {
	errorCalls  int
	unwrapCalls int
}

func (e *lessonDiagnosticsObservedCause) Error() string {
	e.errorCalls++
	return "техническая причина"
}

func (e *lessonDiagnosticsObservedCause) Unwrap() error {
	e.unwrapCalls++
	return nil
}
