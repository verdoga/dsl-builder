package revision

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestDiagnosticsCodes(t *testing.T) {
	for _, tc := range []struct{ name, got, want string }{
		{"input", CodeInput, "revision.input"},
		{"lesson mismatch", CodeLessonMismatch, "revision.lesson_mismatch"},
		{"plan", CodePlan, "revision.plan"},
		{"decision", CodeDecision, "revision.decision"},
		{"history", CodeHistory, "revision.history"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("код = %q, нужен %q", tc.got, tc.want)
			}
		})
	}
}

func TestDiagnosticsPreserveReasonContextAndAddress(t *testing.T) {
	context := diagnostic.Context{
		Path: `C:\Уроки с пробелами\..\Ёж.json`, FileName: "l0007.json",
		LessonID: "УРОК+7", PageID: "урок+7-p1000", Line: 35, Column: 8,
	}
	for _, tc := range []struct{ name, code, pointer string }{
		{"model field", CodeInput, "/document/id"},
		{"lesson identity", CodeLessonMismatch, "/document/id"},
		{"plan address", CodePlan, "plan.Pages[2].Nodes[1].Path"},
		{"decision address", CodeDecision, "decisions[3]"},
		{"nested node", CodeHistory, "/document/pages/0/nodes/1/nodes/2/id"},
		{"escaped pointer", CodePlan, "/поле~1с~0символами/0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const reason = "  Невозможно применить изменение: проверьте адрес «ёж» 🦔  "
			before := context
			issue := revisionError(tc.code, reason, tc.pointer, context)
			if issue == nil {
				t.Fatal("revisionError вернул nil")
			}
			if issue.Code != tc.code || issue.Reason != reason {
				t.Errorf("код или причина изменены: %q, %q", issue.Code, issue.Reason)
			}
			if issue.Context != before || context != before {
				t.Errorf("изменён известный контекст: %#v", issue.Context)
			}
			diagnosticsTestRequireAddress(t, issue, tc.pointer)
			if len(issue.Related) != 0 || issue.Cause != nil {
				t.Error("конструктор выдумал конфликтующее место или техническую причину")
			}
			var found *diagnostic.Error
			if !errors.As(issue, &found) || found != issue {
				t.Error("результат недоступен как исходный *diagnostic.Error")
			}
		})
	}
}

func TestDiagnosticsUnknownContextAndRootAddress(t *testing.T) {
	issue := revisionError(CodeInput, "Урок не задан", "", diagnostic.Context{})
	if issue == nil {
		t.Fatal("revisionError вернул nil")
	}
	if issue.Context != (diagnostic.Context{}) {
		t.Errorf("для неизвестного места выдуман контекст: %#v", issue.Context)
	}
	for _, detail := range issue.Details {
		if detail.Value != "" {
			t.Errorf("для неизвестного адреса выдумана подробность: %#v", detail)
		}
	}
}

func TestDiagnosticsCallsOwnTheirResults(t *testing.T) {
	context := diagnostic.Context{Path: `D:\Архивы\Ё.zip`, FileName: "l007.json"}
	const reason = "Нарушена история ревизии"
	const pointer = "/document/pages/0/nodes/0/id"
	first := revisionError(CodeHistory, reason, pointer, context)
	second := revisionError(CodeHistory, reason, pointer, context)
	if first == nil || second == nil {
		t.Fatal("revisionError вернул nil")
	}
	if first == second {
		t.Fatal("независимые вызовы возвращают один указатель")
	}
	diagnosticsTestRequireAddress(t, first, pointer)
	diagnosticsTestRequireAddress(t, second, pointer)
	secondDetails := slices.Clone(second.Details)
	first.Code, first.Reason = "другой.код", "Другая причина"
	first.Context.Path = "изменённый путь"
	for i := range first.Details {
		first.Details[i] = diagnostic.Detail{Label: "Изменено", Value: "другой адрес"}
	}
	first.Related = append(first.Related, diagnostic.Context{Path: "место конфликта"})
	third := revisionError(CodeHistory, reason, pointer, context)
	for _, issue := range []*diagnostic.Error{second, third} {
		if issue == nil {
			t.Fatal("последующий вызов вернул nil")
		}
		if issue.Code != CodeHistory || issue.Reason != reason || issue.Context != context {
			t.Error("изменение первого результата затронуло независимый вызов")
		}
		if !slices.Equal(issue.Details, secondDetails) || len(issue.Related) != 0 {
			t.Error("подробности или связанные места разделяются между вызовами")
		}
	}
}

func TestDiagnosticsSummaryDoesNotExposeStructuredData(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Секретный путь\урок.json`, LessonID: "особый-ID"}
	const pointer = "/document/pages/9/nodes/8/id"
	issue := revisionError(CodeHistory, "Суффикс превышает ревизию урока", pointer, context)
	if issue == nil {
		t.Fatal("revisionError вернул nil")
	}
	code, reason, beforeContext := issue.Code, issue.Reason, issue.Context
	details := slices.Clone(issue.Details)
	for range 2 {
		text := issue.Error()
		if !strings.Contains(text, code) || !strings.Contains(text, reason) {
			t.Errorf("краткое сообщение не содержит код и причину: %q", text)
		}
		for _, excluded := range []string{context.Path, context.LessonID, pointer, `"data":`, `"nodes":`, `"content":`} {
			if strings.Contains(text, excluded) {
				t.Errorf("краткое сообщение раскрывает структурированные данные: %q", excluded)
			}
		}
		if issue.Code != code || issue.Reason != reason || issue.Context != beforeContext || !slices.Equal(issue.Details, details) {
			t.Error("чтение сообщения изменило диагностику")
		}
	}
}

func diagnosticsTestRequireAddress(t *testing.T, issue *diagnostic.Error, pointer string) {
	t.Helper()
	for _, detail := range issue.Details {
		if detail.Value == pointer {
			if strings.TrimSpace(detail.Label) == "" {
				t.Error("подробность с адресом не имеет понятного названия")
			}
			return
		}
	}
	t.Errorf("Details не содержат точный адрес %q: %#v", pointer, issue.Details)
}
