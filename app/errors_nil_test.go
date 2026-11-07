package app

import (
	"testing"
	"time"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestErrorsNilDecisionMethods(t *testing.T) {
	var op *Operation
	var token ReviewToken
	tests := []struct {
		name string
		call func() []*diagnostic.Error
	}{
		{"DecideField", func() []*diagnostic.Error { return op.DecideField(token, 0, true) }},
		{"DecidePlacement", func() []*diagnostic.Error { return op.DecidePlacement(token, 0, false) }},
		{"PrepareRevision", func() []*diagnostic.Error { return op.PrepareRevision(token) }},
		{"SetDestination", func() []*diagnostic.Error { return op.SetDestination(token, "каталог", "модуль") }},
		{"ConfirmSave", func() []*diagnostic.Error { return op.ConfirmSave(token, true, true) }},
		{"AcceptEdit", func() []*diagnostic.Error { return op.AcceptEdit(token) }},
		{"RejectEdit", func() []*diagnostic.Error { return op.RejectEdit(token) }},
		{"ProposeCourseTitle", func() []*diagnostic.Error {
			_, issues := op.ProposeCourseTitle(token, "Курс")
			return issues
		}},
		{"ProposeUnitTitle", func() []*diagnostic.Error {
			_, issues := op.ProposeUnitTitle(token, "Юнит")
			return issues
		}},
		{"ProposeSection", func() []*diagnostic.Error {
			_, issues := op.ProposeSection(token, "Раздел")
			return issues
		}},
		{"ProposeMove", func() []*diagnostic.Error {
			_, issues := op.ProposeMove(token, "l001.json", nil)
			return issues
		}},
		{"ProposeFlatPlacement", func() []*diagnostic.Error {
			_, issues := op.ProposeFlatPlacement(token)
			return issues
		}},
		{"ProposeLessonWeight", func() []*diagnostic.Error {
			_, issues := op.ProposeLessonWeight(token, "l001.json", "001")
			return issues
		}},
		{"ProposeLessonWeights", func() []*diagnostic.Error {
			_, issues := op.ProposeLessonWeights(token, nil, map[string]string{"l001.json": "1"})
			return issues
		}},
		{"ProposeSectionWeights", func() []*diagnostic.Error {
			_, issues := op.ProposeSectionWeights(token, nil)
			return issues
		}},
		{"ReviewSave", func() []*diagnostic.Error {
			_, issues := op.ReviewSave(token)
			return issues
		}},
		{"Save", func() []*diagnostic.Error {
			result, issues := op.Save(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			if result.Installed {
				t.Error("nil операция не может установить архив")
			}
			return issues
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorsAssertIssue(t, tt.call(), CodeState)
		})
	}
}

func TestErrorsNilSnapshotAndCancel(t *testing.T) {
	var op *Operation
	for i := 0; i < 2; i++ {
		op.Cancel()
		snapshot := op.Snapshot()
		issue := errorsAssertIssue(t, snapshot.Blockers, CodeState)
		if issue.Context != (diagnostic.Context{}) {
			t.Errorf("nil операция получила выдуманное место ошибки: %+v", issue.Context)
		}
		if !snapshot.Actions.MainMenu || !snapshot.Actions.Exit {
			t.Errorf("возврат и выход должны быть доступны: %+v", snapshot.Actions)
		}
		if snapshot.Actions.Save {
			t.Error("nil снимок разрешает запись")
		}
	}
}

func errorsAssertIssue(t *testing.T, issues []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, issue := range issues {
		if issue == nil {
			t.Fatal("срез диагностики содержит nil")
		}
		if issue.Code != code {
			continue
		}
		for _, r := range issue.Reason {
			if unicode.In(r, unicode.Cyrillic) {
				return issue
			}
		}
		t.Fatalf("код %q не имеет понятной русской причины: %q", code, issue.Reason)
	}
	t.Fatalf("нет ожидаемого кода %q: %v", code, issues)
	return nil
}
