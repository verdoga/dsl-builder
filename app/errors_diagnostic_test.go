package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/parsed"
)

func TestErrorsConstructorsPreservePathOwnerCode(t *testing.T) {
	base := t.TempDir()
	badPath := "неверный\x00путь"
	tests := []struct {
		name string
		call func() (*Operation, []*diagnostic.Error)
	}{
		{"Create", func() (*Operation, []*diagnostic.Error) { return NewCreate(base, badPath, "Курс", "") }},
		{"Update", func() (*Operation, []*diagnostic.Error) { return NewUpdate(base, badPath, "lesson.json") }},
		{"Add", func() (*Operation, []*diagnostic.Error) { return NewAdd(base, badPath, "lesson.json") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, issues := tt.call()
			issue := errorsAssertIssue(t, issues, filetree.CodePath)
			if issue.Context.Path != badPath {
				t.Errorf("исходный неверный путь потерян: %q", issue.Context.Path)
			}
		})
	}
}

func TestErrorsMissingSourcePreservesFilesystemCause(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "Нет каталога")
	op, issues := NewCreate(base, missing, "Курс", "")
	if len(issues) != 0 || op == nil {
		t.Fatalf("конструктор: %v", issues)
	}
	issues = op.Analyze(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
	issue := errorsAssertIssue(t, issues, filetree.CodeAccess)
	if issue.Context.Path != missing {
		t.Errorf("путь = %q, нужен %q", issue.Context.Path, missing)
	}
	if !errors.Is(issue, fs.ErrNotExist) {
		t.Errorf("исходная ошибка отсутствия файла потеряна: %v", issue.Cause)
	}
	var pathError *os.PathError
	if !errors.As(issue, &pathError) {
		t.Errorf("структурированная ошибка ОС потеряна: %T", issue.Cause)
	}
	snapshot := op.Snapshot()
	errorsAssertIssue(t, snapshot.Blockers, filetree.CodeAccess)
	if snapshot.Actions.Save || snapshot.Actions.ReviewSave {
		t.Error("ошибка обязательного источника разрешает запись")
	}
	if !snapshot.Actions.Retry || !snapshot.Actions.MainMenu || !snapshot.Actions.Exit {
		t.Errorf("нет продолжения после исправления источника: %+v", snapshot.Actions)
	}
}

func TestErrorsAnalyzePreservesParsedDiagnostics(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "Ошибочный урок.json")
	content := []byte("{\"formatVersion\":]\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	_, ownerIssues := parsed.Load(path)
	owner := errorsAssertIssue(t, ownerIssues, parsed.CodeJSON)
	op, issues := NewCreate(base, base, "Курс", "")
	if len(issues) != 0 || op == nil {
		t.Fatalf("конструктор: %v", issues)
	}
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	issue := errorsAssertIssue(t, op.Analyze(at), parsed.CodeJSON)
	if issue.Context != owner.Context || issue.Reason != owner.Reason {
		t.Errorf("диагностика владельца изменена: %+v; владелец: %+v", issue, owner)
	}
	if issue.Context.Path != path {
		t.Errorf("потерян конкретный JSON-путь: %q", issue.Context.Path)
	}
	if len(issue.Details) != len(owner.Details) || len(issue.Related) != len(owner.Related) {
		t.Fatalf("потеряны подробности владельца: %+v", issue)
	}
	for i := range owner.Details {
		if issue.Details[i] != owner.Details[i] {
			t.Errorf("подробность %d изменена: %+v", i, issue.Details[i])
		}
	}
	for i := range owner.Related {
		if issue.Related[i] != owner.Related[i] {
			t.Errorf("связанное место %d изменено: %+v", i, issue.Related[i])
		}
	}
	var cause, ownerCause *json.SyntaxError
	if !errors.As(owner, &ownerCause) || !errors.As(issue, &cause) {
		t.Fatal("диагностика должна сохранять тип json.SyntaxError в Cause")
	}
	if cause.Offset != ownerCause.Offset {
		t.Errorf("смещение Cause = %d, нужно %d", cause.Offset, ownerCause.Offset)
	}
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("анализ изменил исходник: %q; %v", stored, err)
	}
	if err := os.WriteFile(path, []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	errorsAssertIssue(t, op.Retry(at), parsed.CodeUTF8)
	snapshot := op.Snapshot()
	errorsAssertIssue(t, snapshot.Blockers, parsed.CodeUTF8)
	for _, blocker := range snapshot.Blockers {
		if blocker.Code == parsed.CodeJSON {
			t.Error("Retry оставил прежнюю ошибку вместо повторного чтения")
		}
	}
	if snapshot.Actions.Save || snapshot.Actions.ReviewSave {
		t.Error("ошибка UTF-8 разрешает запись")
	}
}
