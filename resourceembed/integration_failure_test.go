package resourceembed_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourceembed"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestTask11RemovedSelectedFileFailsAtomically(t *testing.T) {
	for _, extension := range []string{".mp4", ".txt", ".ssa"} {
		t.Run(extension, func(t *testing.T) {
			dir := t.TempDir()
			content := []byte("Приватные байты ресурса\x00\xff\r\n")
			for _, suffix := range []string{".mp4", ".txt", ".vtt", ".ssa"} {
				task11Write(t, filepath.Join(dir, "лекция"+suffix), content)
			}
			selection, context, inventory := task11Find(t, dir, "video", "лекция.mp4", nil)
			before := task11CopySelection(selection)
			var usage resourceusage.Registry
			usage.Register(inventory)
			usage.Register(selection.AccountedPaths)
			registered := usage.Snapshot()
			if registered.Total != 4 || registered.Used != 0 || registered.Unused != 4 {
				t.Fatalf("выбранные внешние файлы не зарегистрированы: %+v", registered)
			}
			value := task11Lesson([]lesson.Data{})
			lessonBefore, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			failedPath := filepath.Join(dir, "лекция"+extension)
			if err := os.Remove(failedPath); err != nil {
				t.Fatal(err)
			}
			actual, problem := resourceembed.Embed(selection, context)
			if actual != nil || problem == nil {
				t.Fatalf("ошибка должна вернуть nil без частичных данных: данные=%v, ошибка=%v", actual, problem)
			}
			if problem.Code != resourceembed.CodeRead || problem.Reason != "Не удалось прочитать выбранный ресурс" || problem.Context.Path != failedPath || !slices.Contains(problem.Related, context) {
				t.Fatalf("потерян путь ресурса или полный контекст JSON: %+v", problem)
			}
			var cause *os.PathError
			if problem.Cause == nil || !errors.As(problem.Cause, &cause) || cause.Path != failedPath || !errors.Is(problem, fs.ErrNotExist) {
				t.Fatalf("потеряна исходная ошибка чтения: %+v", problem)
			}
			for _, detail := range problem.Details {
				if strings.Contains(detail.Value, string(content)) || strings.Contains(detail.Value, base64.StdEncoding.EncodeToString(content)) {
					t.Fatal("диагностика раскрывает содержимое ресурса")
				}
			}
			task11Unchanged(t, selection, before)
			if after := usage.Snapshot(); after.Used != 0 || !slices.Equal(after.UnusedPaths, registered.UnusedPaths) {
				t.Fatal("неуспешное встраивание изменило учёт")
			}
			lessonAfter, err := json.Marshal(value)
			if err != nil || !bytes.Equal(lessonBefore, lessonAfter) {
				t.Fatal("неуспешное встраивание изменило урок")
			}
			for _, suffix := range []string{".mp4", ".txt", ".vtt", ".ssa"} {
				if suffix == extension {
					continue
				}
				got, err := os.ReadFile(filepath.Join(dir, "лекция"+suffix))
				if err != nil || !bytes.Equal(got, content) {
					t.Fatalf("оставшийся ресурс %s изменён: %v", suffix, err)
				}
			}
			if got, err := os.ReadFile(context.Path); err != nil || string(got) != "{\"source\":\"не изменять\"}\r\n" {
				t.Fatalf("исходный JSON изменён: %v", err)
			}
		})
	}
}

func TestTask11FileLinksKeepNamesAndSeparateRoles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Цель без имени медиа.bin")
	content := []byte{0, 255, 239, 187, 191, '\r', '\n'}
	task11Write(t, target, content)
	bodyPath := filepath.Join(dir, "Обращение 01.MP4")
	for _, path := range []string{bodyPath, filepath.Join(dir, "Обращение 01.txt"), filepath.Join(dir, "Обращение 01.vtt")} {
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("файловые ссылки недоступны в этой среде: %v", err)
		}
	}
	if err := os.Link(target, filepath.Join(dir, "Обращение 01.srt")); err != nil {
		t.Skipf("жёсткие ссылки недоступны в этой среде: %v", err)
	}
	selection, context, _ := task11Find(t, dir, "video", filepath.Base(bodyPath), nil)
	before := task11CopySelection(selection)
	if selection.Body.Entry.Info.Name() == filepath.Base(bodyPath) || selection.Body.Entry.IdentityPath == bodyPath || len(selection.AccountedPaths) != 1 {
		t.Fatal("фикстура не различает адрес обращения, Info.Name и идентичность")
	}
	if selection.Script == nil || len(selection.Subtitles) != 2 {
		t.Fatal("Find не сохранил все роли ссылок")
	}
	files := slices.Clone(selection.Subtitles)
	files = append(files, *selection.Script)
	for _, file := range files {
		if file.Entry.IdentityPath != selection.Body.Entry.IdentityPath {
			t.Fatal("фикстура не создала одинаковую идентичность у разных ролей")
		}
	}
	actual, problem := resourceembed.Embed(selection, context)
	if problem != nil {
		t.Fatal(problem)
	}
	task11Check(t, actual, []task11Expected{
		{"Обращение 01.MP4", "video/mp4", lesson.RoleBody, content},
		{"Обращение 01.txt", "text/plain", lesson.RoleScript, content},
		{"Обращение 01.vtt", "text/vtt", lesson.RoleSubtitles, content},
		{"Обращение 01.srt", "application/x-subrip", lesson.RoleSubtitles, content},
	})
	task11Unchanged(t, selection, before)
	if err := os.Remove(bodyPath); err != nil {
		t.Fatal(err)
	}
	actual, problem = resourceembed.Embed(selection, context)
	if actual != nil || problem == nil || problem.Code != resourceembed.CodeRead || problem.Context.Path != bodyPath || !errors.Is(problem, fs.ErrNotExist) {
		t.Fatalf("после удаления выбранного адреса нельзя читать IdentityPath: данные=%v, ошибка=%+v", actual, problem)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("цель ссылки изменена: %v", err)
	}
	task11Unchanged(t, selection, before)
}
