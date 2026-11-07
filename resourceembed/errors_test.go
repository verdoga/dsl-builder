package resourceembed

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/resourcefind"
)

func TestTask3ReadErrorPreservesCauseAndSourceContext(t *testing.T) {
	file, _ := task3SelectedFile(t, false)
	if err := os.Remove(file.Entry.Path); err != nil {
		t.Fatal(err)
	}
	_, original := os.ReadFile(file.Entry.Path)
	if original == nil {
		t.Fatal("удалённый выбранный файл неожиданно прочитан")
	}
	cause := fmt.Errorf("ошибка полного чтения: %w", original)
	contexts := []struct {
		name  string
		value diagnostic.Context
	}{
		{"все сведения", diagnostic.Context{
			Path: `C:\Исходники уроков\Урок Ё.json`, FileName: "l042.json",
			LessonID: "Ab-42", PageID: "ab-42-p003", Line: 37, Column: 14,
		}},
		{"строка без колонки", diagnostic.Context{
			Path: `C:\Исходники уроков\Урок Ё.json`, Line: 7,
		}},
		{"только техническое имя", diagnostic.Context{FileName: "l042.json"}},
		{"неизвестные сведения", diagnostic.Context{}},
	}
	for _, tc := range contexts {
		t.Run(tc.name, func(t *testing.T) {
			got := readError(file, tc.value, cause)
			task3CheckDiagnostic(t, got, file.Entry.Path, tc.value)
			if got.Cause != cause {
				t.Errorf("Cause = %v; нужна исходная ошибка %v", got.Cause, cause)
			}
			if !errors.Is(got, cause) || !errors.Is(got, os.ErrNotExist) {
				t.Error("errors.Is не сохраняет исходную цепочку ошибки чтения")
			}
			var originalPathError, actualPathError *os.PathError
			if !errors.As(original, &originalPathError) ||
				!errors.As(got, &actualPathError) || actualPathError != originalPathError {
				t.Errorf("errors.As потерял исходный *os.PathError: %v", actualPathError)
			}
			var actualDiagnostic *diagnostic.Error
			if !errors.As(fmt.Errorf("операция: %w", got), &actualDiagnostic) || actualDiagnostic != got {
				t.Error("диагностика недоступна вызывающему коду через errors.As")
			}
		})
	}
}

func TestTask3ReadErrorUsesSelectedAliasPath(t *testing.T) {
	file, _ := task3SelectedFile(t, true)
	if file.Entry.Path == file.Entry.IdentityPath || filepath.Base(file.Entry.Path) == file.Entry.Info.Name() {
		t.Fatal("фикстура не различает адрес обращения, идентичность и имя цели")
	}
	context := diagnostic.Context{Path: `C:\JSON\Исток.json`, LessonID: "Ё42", Line: 12}
	got := readError(file, context, os.ErrPermission)
	task3CheckDiagnostic(t, got, file.Entry.Path, context)
}

func TestTask3ReadErrorDoesNotPrintOrExposeResourceContent(t *testing.T) {
	file, bytes := task3SelectedFile(t, false)
	context := diagnostic.Context{Path: `C:\JSON\Исток.json`, PageID: "ab42-p001"}
	var got *diagnostic.Error
	output := task3CaptureOutput(t, func() {
		got = readError(file, context, os.ErrPermission)
	})
	if output != "" {
		t.Errorf("readError вывел сообщение или меню: %q", output)
	}
	task3CheckDiagnostic(t, got, file.Entry.Path, context)
	for _, detail := range got.Details {
		for _, text := range []string{detail.Label, detail.Value} {
			if strings.Contains(text, string(bytes)) || strings.Contains(text, base64.StdEncoding.EncodeToString(bytes)) {
				t.Error("Details содержит исходные байты или Base64 ресурса")
			}
		}
	}
}

func task3CheckDiagnostic(t *testing.T, got *diagnostic.Error, resourcePath string, context diagnostic.Context) {
	t.Helper()
	if got == nil {
		t.Fatal("readError вернул nil")
	}
	if CodeRead != "resourceembed.read" || got.Code != CodeRead {
		t.Errorf("CodeRead = %q, диагностика.Code = %q", CodeRead, got.Code)
	}
	if got.Reason != "Не удалось прочитать выбранный ресурс" {
		t.Errorf("Reason = %q", got.Reason)
	}
	if got.Context.Path != resourcePath {
		t.Errorf("Context.Path = %q; нужен точный Entry.Path %q", got.Context.Path, resourcePath)
	}
	for _, related := range got.Related {
		if related == context {
			return
		}
	}
	t.Errorf("Related не содержит исходный JSON-контекст без изменений: %+v; получено %+v", context, got.Related)
}

func task3SelectedFile(t *testing.T, alias bool) (resourcefind.File, []byte) {
	t.Helper()
	dir := t.TempDir()
	bytes := []byte("СЕКРЕТНЫЕ БАЙТЫ РЕСУРСА task3\x00\xff\r\n")
	target := filepath.Join(dir, "Цель другие имя.PNG")
	if err := os.WriteFile(target, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	path := target
	if alias {
		path = filepath.Join(dir, "Обращение Ё с пробелом.png")
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("ОС не разрешила создание реальной файловой ссылки: %v", err)
		}
	}
	jsonPath := filepath.Join(dir, "Технический JSON.json")
	context := diagnostic.Context{Path: jsonPath}
	finder, _, err := resourcefind.Prepare(&filetree.Resolver{}, jsonPath, nil, context)
	if err != nil {
		t.Fatalf("resourcefind.Prepare: %v", err)
	}
	selected, err := finder.Find("image", `"`+path+`"`, path, context)
	if err != nil {
		t.Fatalf("resourcefind.Find: %v", err)
	}
	return selected.Body, bytes
}

func task3CaptureOutput(t *testing.T, call func()) string {
	t.Helper()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout-")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.CreateTemp(t.TempDir(), "stderr-")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	func() {
		previousOut, previousErr := os.Stdout, os.Stderr
		defer func() { os.Stdout, os.Stderr = previousOut, previousErr }()
		os.Stdout, os.Stderr = stdout, stderr
		call()
	}()
	out, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	errout, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(out) + string(errout)
}
