package diagnostic_test

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestErrorSummaryUsesCodeAndReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   string
		reason string
	}{
		{"resource", "resourcefind.ambiguous", "Найдено несколько подходящих ресурсов"},
		{"parser", "parsed.parser", "Парсер сообщил об ошибке: строка «ёж 🦔»"},
		{"caller supplied", "проверка.свой_код", "  Причина с сохранёнными пробелами  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var err error = &diagnostic.Error{Code: tc.code, Reason: tc.reason}
			got := err.Error()
			if !strings.Contains(got, tc.code) || !strings.Contains(got, tc.reason) {
				t.Errorf("Error() = %q; должны присутствовать Code %q и Reason %q", got, tc.code, tc.reason)
			}
		})
	}
}

func TestErrorSummaryExcludesStructuredDataAndCause(t *testing.T) {
	cause := &errorTestObservedCause{}
	issue := errorTestStructuredIssue(cause)
	want := (&diagnostic.Error{Code: issue.Code, Reason: issue.Reason}).Error()
	got := issue.Error()
	if got != want {
		t.Errorf("контекст, подробности или Cause изменили краткий текст: %q; без них %q", got, want)
	}
	for _, excluded := range []string{
		issue.Context.Path, issue.Context.FileName, issue.Context.LessonID, issue.Context.PageID,
		issue.Related[0].Path, issue.Related[1].Path,
		issue.Details[0].Label, issue.Details[0].Value,
		issue.Details[1].Label, issue.Details[1].Value,
		"техническая причина, не пользовательское описание",
	} {
		if strings.Contains(got, excluded) {
			t.Errorf("краткий текст %q содержит структурированное или техническое значение %q", got, excluded)
		}
	}
	if cause.calls != 0 {
		t.Errorf("Error() обратился к Cause.Error() %d раз", cause.calls)
	}
}

func TestErrorMethodsPreserveStructuredFields(t *testing.T) {
	cause := &errorTestObservedCause{}
	issue := errorTestStructuredIssue(cause)
	before := *issue
	related := slices.Clone(issue.Related)
	details := slices.Clone(issue.Details)
	for range 2 {
		_ = issue.Error()
		if got := issue.Unwrap(); got != cause {
			t.Fatal("Unwrap() не вернул исходную Cause")
		}
	}
	var found *diagnostic.Error
	wrapped := fmt.Errorf("внешняя операция: %w", issue)
	if !errors.As(wrapped, &found) || found != issue {
		t.Fatal("errors.As не вернул исходный *diagnostic.Error")
	}
	if found.Code != before.Code || found.Reason != before.Reason || found.Context != before.Context || found.Cause != before.Cause {
		t.Error("изменены Code, Reason, Context или Cause")
	}
	if !slices.Equal(found.Related, related) || !slices.Equal(found.Details, details) {
		t.Error("изменены значения или порядок Related/Details")
	}
	if !slices.Equal(before.Related, related) || !slices.Equal(before.Details, details) {
		t.Error("изменены исходные backing arrays Related/Details")
	}
	if cause.calls != 0 {
		t.Errorf("методы диагностики вызвали Cause.Error() %d раз", cause.calls)
	}
}

func TestErrorUnwrapPreservesStandardErrorChains(t *testing.T) {
	sentinel := errors.New("исходная ошибка чтения")
	other := errors.New("независимая ошибка закрытия")
	pathCause := &os.PathError{Op: "open", Path: `C:\Уроки\ёж 🦔.json`, Err: sentinel}
	for _, tc := range []struct {
		name  string
		cause error
		want  []error
		path  *os.PathError
	}{
		{name: "nil"},
		{name: "ordinary", cause: sentinel, want: []error{sentinel}},
		{name: "wrapped", cause: fmt.Errorf("чтение: %w", sentinel), want: []error{sentinel}},
		{name: "typed", cause: pathCause, want: []error{sentinel}, path: pathCause},
		{name: "joined", cause: errors.Join(pathCause, other), want: []error{sentinel, other}, path: pathCause},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue := &diagnostic.Error{Code: "parsed.read", Reason: "Не удалось прочитать исходный JSON", Cause: tc.cause}
			var err error = issue
			if got := issue.Unwrap(); got != tc.cause {
				t.Fatal("Unwrap() должен вернуть саму Cause без замены или распаковки")
			}
			if got := errors.Unwrap(err); got != tc.cause {
				t.Fatal("errors.Unwrap() должен вернуть саму Cause")
			}
			wrapped := fmt.Errorf("операция: %w", err)
			for _, target := range tc.want {
				if !errors.Is(wrapped, target) {
					t.Error("errors.Is не нашёл исходную причину")
				}
			}
			if errors.Is(wrapped, errors.New("посторонняя ошибка")) {
				t.Error("errors.Is сопоставил постороннюю ошибку")
			}
			var found *diagnostic.Error
			if !errors.As(wrapped, &found) || found != issue {
				t.Error("структурированная диагностика недоступна через errors.As")
			}
			var foundPath *os.PathError
			if ok := errors.As(wrapped, &foundPath); ok != (tc.path != nil) || foundPath != tc.path {
				t.Error("errors.As не сохранил доступ к исходному типу технической причины")
			}
		})
	}
}

func errorTestStructuredIssue(cause error) *diagnostic.Error {
	return &diagnostic.Error{
		Code:   "catalog.conflict",
		Reason: "Совпали веса уроков",
		Context: diagnostic.Context{
			Path: `C:\Исходники\..\Уроки с пробелами\ёж 🦔.json`, FileName: "l0007.json",
			LessonID: "0007ЁЖ", PageID: "0007ЁЖ.p0002", Line: 35, Column: 8,
		},
		Related: []diagnostic.Context{
			{Path: `D:\Уроки\Б.json`, LessonID: "0009Б", Line: 9},
			{Path: `D:\Уроки\А.json`, LessonID: "0008А", Line: 4, Column: 3},
			{Path: `D:\Уроки\Б.json`, LessonID: "0009Б", Line: 9},
		},
		Details: []diagnostic.Detail{
			{Label: "Исходный вес", Value: "0001234567890123456789012345678901234567890"},
			{Label: "Подстрока поиска", Value: " 00.12 ёж 🦔 "},
			{Label: "Каталог поиска", Value: `C:\Медиа\Б`},
			{Label: "Каталог поиска", Value: ""},
		},
		Cause: cause,
	}
}

type errorTestObservedCause struct {
	calls int
}

func (e *errorTestObservedCause) Error() string {
	e.calls++
	return "техническая причина, не пользовательское описание"
}
