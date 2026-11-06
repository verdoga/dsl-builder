package diagnostic_test

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestIntegrationPresentationPreservesStructuredData(t *testing.T) {
	largeWeight := strings.Repeat("9", 60)
	tests := []struct {
		name     string
		issue    *diagnostic.Error
		excluded []string
	}{
		{
			name: "all ambiguous resource candidates and exact search details",
			issue: &diagnostic.Error{
				Code:   "resourcefind.ambiguous",
				Reason: "Найдено несколько подходящих файлов ресурса",
				Context: diagnostic.Context{
					Path:     `C:\Учебные материалы\Юнит 1\исходный.json`,
					LessonID: "000EFX7", PageID: "000EFX7.P0001", Line: 35, Column: 7,
				},
				Related: []diagnostic.Context{
					{Path: `C:\Учебные материалы\Audio\Я 001. часть.mp3`},
					{Path: `\\сервер\Общие файлы\А 001. часть.wav`},
					{Path: `C:\Учебные материалы\Audio\Ё 001. часть.MP3`},
				},
				Details: []diagnostic.Detail{
					{Label: "Тип ресурса", Value: "audio"},
					{Label: "Подстрока поиска", Value: " 001. часть "},
					{Label: "Каталог поиска", Value: `C:\Учебные материалы\Audio`},
					{Label: "Каталог поиска", Value: `\\сервер\Общие файлы`},
					{Label: "Исходное сообщение", Value: ""},
					{Label: "Каталог поиска", Value: `C:\Учебные материалы\Audio`},
				},
			},
			excluded: []string{"000EFX7", " 001. часть ", "Каталог поиска", `\\сервер`},
		},
		{
			name: "all conflicting lessons with original arbitrary length weights",
			issue: &diagnostic.Error{
				Code:   "catalog.conflicting_order",
				Reason: "Уроки имеют одинаковый вес внутри секции",
				Context: diagnostic.Context{
					Path: `C:\Модули\Добавление уроков.zip`, FileName: "manifest.json",
				},
				Related: []diagnostic.Context{
					{Path: `C:\Источники\Я.json`, FileName: "l003.json", LessonID: "0003", PageID: "0003.P01", Line: 8},
					{Path: `C:\Источники\А.json`, FileName: "l001.json", LessonID: "0001", Line: 4, Column: 2},
					{Path: `\\сервер\Уроки\Ё.json`, FileName: "l002.json", LessonID: "0002"},
				},
				Details: []diagnostic.Detail{
					{Label: "Секция", Value: "Вводный раздел"},
					{Label: "Исходный вес", Value: "00" + largeWeight},
					{Label: "Исходный вес", Value: largeWeight},
					{Label: "Исходный вес", Value: "0" + largeWeight},
					{Label: "Название урока", Value: ""},
				},
			},
			excluded: []string{"manifest.json", "l003.json", "Исходный вес", largeWeight},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := tt.issue
			code, reason, context := issue.Code, issue.Reason, issue.Context
			related, details := slices.Clone(issue.Related), slices.Clone(issue.Details)
			cause := errors.New("исходная техническая причина, не краткое описание")
			issue.Cause = cause
			wrapped := fmt.Errorf("Подготовка представления: %w", issue)

			var got *diagnostic.Error
			if !errors.As(wrapped, &got) || got != issue {
				t.Fatal("errors.As не сохранил исходную структурированную диагностику")
			}
			if !errors.Is(wrapped, cause) || got.Unwrap() != cause {
				t.Fatal("исходная техническая причина недоступна через цепочку error")
			}
			short := got.Error()
			if !strings.Contains(short, code) || !strings.Contains(short, reason) {
				t.Errorf("краткое описание %q не содержит Code и Reason", short)
			}
			for _, fragment := range append(tt.excluded, cause.Error(), context.Path) {
				if strings.Contains(short, fragment) {
					t.Errorf("краткое описание содержит контекст, подробность или Cause: %q", fragment)
				}
			}
			if got.Error() != short {
				t.Error("повторный Error изменил краткое описание")
			}
			if got.Code != code || got.Reason != reason || got.Context != context || got.Cause != cause {
				t.Errorf("поля диагностики изменились после представления: %#v", got)
			}
			if !slices.Equal(got.Related, related) {
				t.Errorf("участники или их исходный порядок изменились: %#v", got.Related)
			}
			if !slices.Equal(got.Details, details) {
				t.Errorf("точные значения, пустые значения или порядок подробностей изменились: %#v", got.Details)
			}
		})
	}
}

func TestIntegrationJoinedDiagnosticsPreserveBothBranches(t *testing.T) {
	readCause := &fs.PathError{
		Op: "read", Path: `C:\Исходники\Технический JSON.json`, Err: io.ErrUnexpectedEOF,
	}
	writeCause := &os.LinkError{
		Op: "rename", Old: `C:\Модули\Временный.zip`, New: `C:\Модули\Итоговый.zip`, Err: fs.ErrPermission,
	}
	readContext := diagnostic.Context{Path: readCause.Path, LessonID: "000EFX7", Line: 35}
	writeContext := diagnostic.Context{Path: writeCause.New, FileName: "manifest.json"}
	readRelated := []diagnostic.Context{{Path: `\\сервер\Справка\Ё.json`, LessonID: "000EFX8"}}
	writeRelated := []diagnostic.Context{{Path: writeCause.Old}, {Path: writeCause.New}}
	readDetails := []diagnostic.Detail{
		{Label: "Сообщение", Value: "неполный JSON"}, {Label: "Сообщение", Value: ""},
	}
	writeDetails := []diagnostic.Detail{
		{Label: "Архив", Value: writeCause.Old}, {Label: "Архив", Value: writeCause.New},
	}
	readIssue := &diagnostic.Error{
		Code: "parsed.read", Reason: "Не удалось прочитать технический JSON", Context: readContext,
		Related: slices.Clone(readRelated), Details: slices.Clone(readDetails), Cause: readCause,
	}
	writeIssue := &diagnostic.Error{
		Code: "safewrite.replace", Reason: "Не удалось заменить архив", Context: writeContext,
		Related: slices.Clone(writeRelated), Details: slices.Clone(writeDetails), Cause: writeCause,
	}
	joined := errors.Join(fmt.Errorf("Чтение: %w", readIssue), fmt.Errorf("Запись: %w", writeIssue))
	root := &diagnostic.Error{
		Code: "app.operation", Reason: "Операция не завершена",
		Context: diagnostic.Context{Path: `C:\Модули\Операция.zip`},
		Details: []diagnostic.Detail{{Label: "Операция", Value: "Добавление уроков"}}, Cause: joined,
	}
	wrapped := fmt.Errorf("Представление результата: %w", root)
	var got *diagnostic.Error
	if !errors.As(wrapped, &got) || got != root || got.Unwrap() != joined {
		t.Fatal("внешняя диагностика или исходное объединение причин потеряны")
	}
	for _, target := range []error{readIssue, writeIssue, readCause, writeCause, io.ErrUnexpectedEOF, fs.ErrPermission} {
		if !errors.Is(wrapped, target) {
			t.Errorf("ветвь или её техническая причина недоступна через errors.Is: %T", target)
		}
	}
	var gotRead *fs.PathError
	if !errors.As(wrapped, &gotRead) || gotRead != readCause {
		t.Error("errors.As потерял типизированную причину первой ветви")
	}
	var gotWrite *os.LinkError
	if !errors.As(wrapped, &gotWrite) || gotWrite != writeCause {
		t.Error("errors.As потерял типизированную причину второй ветви")
	}
	tree, ok := got.Unwrap().(interface{ Unwrap() []error })
	if !ok {
		t.Fatal("Cause больше не содержит стандартное дерево errors.Join")
	}
	branches := tree.Unwrap()
	if len(branches) != 2 {
		t.Fatalf("число сохранённых ветвей = %d, ожидается 2", len(branches))
	}
	for i, want := range []struct {
		issue   *diagnostic.Error
		context diagnostic.Context
		related []diagnostic.Context
		details []diagnostic.Detail
	}{
		{readIssue, readContext, readRelated, readDetails},
		{writeIssue, writeContext, writeRelated, writeDetails},
	} {
		var branch *diagnostic.Error
		if !errors.As(branches[i], &branch) || branch != want.issue {
			t.Fatalf("ветвь %d не сохранила свою диагностику", i)
		}
		_ = branch.Error()
		if branch.Context != want.context || !slices.Equal(branch.Related, want.related) || !slices.Equal(branch.Details, want.details) {
			t.Errorf("ветвь %d потеряла собственный контекст или подробности: %#v", i, branch)
		}
	}
}
