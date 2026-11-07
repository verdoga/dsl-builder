package filetree

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestFiletreeStableErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"path", CodePath, "filetree.path"},
		{"access", CodeAccess, "filetree.access"},
		{"link", CodeLink, "filetree.link"},
		{"kind", CodeKind, "filetree.kind"},
		{"extension", CodeExtension, "filetree.extension"},
		{"empty_json", CodeEmptyJSON, "filetree.empty_json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("code = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestPathErrorPreservesDiagnosticFields(t *testing.T) {
	tests := []struct {
		code   string
		reason string
	}{
		{CodePath, "Путь не разрешается относительно базы"},
		{CodeAccess, "Не удалось получить доступ к файлу"},
		{CodeLink, "Не удалось разрешить файловую ссылку"},
		{CodeKind, "Требуется обычный файл"},
		{CodeExtension, "Требуется расширение .json"},
		{CodeEmptyJSON, "В каталоге не найдены JSON-файлы"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			const path = `C:\Курс с пробелами\Юнит\A.JSON`
			cause := &fs.PathError{Op: "stat", Path: path, Err: fs.ErrPermission}
			got := pathError(tt.code, tt.reason, path, cause)
			if got == nil {
				t.Fatal("pathError returned nil")
			}
			if got.Code != tt.code || got.Reason != tt.reason {
				t.Errorf("code/reason = %q/%q, want %q/%q", got.Code, got.Reason, tt.code, tt.reason)
			}
			if want := (diagnostic.Context{Path: path}); got.Context != want {
				t.Errorf("Context = %#v, want %#v", got.Context, want)
			}
			if got.Cause != cause {
				t.Errorf("Cause = %v, want original error %v", got.Cause, cause)
			}
			if len(got.Related) != 0 || len(got.Details) != 0 {
				t.Errorf("unknown context was invented: Related=%#v Details=%#v", got.Related, got.Details)
			}
		})
	}
}

func TestPathErrorPreservesPathBeforeAndAfterResolution(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"drive_path", `D:\Каталог\Тема 01\Файл.json`},
		{"unc_path", `\\сервер\Ресурсы\Видео 01.mp4`},
		{"original_relative_reference", `..\Исходники\A.json`},
		{"drive_relative_reference", `C:Урок.json`},
		{"original_spelling", `  "..\Папка\..\Урок.JSON"  `},
		{"empty_reference", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathError(CodePath, "Не удалось разрешить путь", tt.path, nil)
			if got == nil {
				t.Fatal("pathError returned nil")
			}
			if want := (diagnostic.Context{Path: tt.path}); got.Context != want {
				t.Errorf("Context = %#v, want original reference %#v", got.Context, want)
			}
			if got.Cause != nil || errors.Unwrap(got) != nil {
				t.Errorf("nil cause changed: Cause=%v Unwrap=%v", got.Cause, errors.Unwrap(got))
			}
		})
	}
}

func TestPathErrorPreservesEmptyValues(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		reason string
		path   string
	}{
		{name: "all_empty"},
		{name: "empty_code", reason: "Исходная причина", path: `C:\Урок.json`},
		{name: "empty_reason", code: CodePath, path: `C:\Урок.json`},
		{name: "empty_path", code: CodePath, reason: "Исходная причина"},
		{name: "verbatim_text", code: "owner.code", reason: "  Исходная причина\n", path: "  Урок.json  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathError(tt.code, tt.reason, tt.path, nil)
			if got == nil {
				t.Fatal("pathError returned nil")
			}
			if got.Code != tt.code || got.Reason != tt.reason || got.Context != (diagnostic.Context{Path: tt.path}) {
				t.Errorf("fields changed: got %#v; want code=%q reason=%q path=%q", got, tt.code, tt.reason, tt.path)
			}
			if got.Cause != nil {
				t.Errorf("Cause = %v, want nil", got.Cause)
			}
		})
	}
}

func TestPathErrorSupportsStandardErrorChain(t *testing.T) {
	const path = `C:\Курс\Нет доступа.json`
	original := &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
	cause := fmt.Errorf("техническая причина: %w", original)
	got := pathError(CodeAccess, "Не удалось открыть файл", path, cause)
	if got == nil {
		t.Fatal("pathError returned nil")
	}
	if got.Cause != cause || errors.Unwrap(got) != cause {
		t.Error("the immediate cause was replaced")
	}
	for _, target := range []error{got, cause, original, fs.ErrPermission} {
		if !errors.Is(got, target) {
			t.Errorf("errors.Is did not reach %T: %v", target, target)
		}
	}
	if errors.Is(got, fs.ErrNotExist) {
		t.Error("errors.Is matched an unrelated cause")
	}
	wrapped := fmt.Errorf("обработка входа: %w", got)
	var detail *diagnostic.Error
	if !errors.As(wrapped, &detail) || detail != got {
		t.Errorf("errors.As diagnostic = %v, want original diagnostic", detail)
	}
	var pathCause *fs.PathError
	if !errors.As(wrapped, &pathCause) || pathCause != original {
		t.Errorf("errors.As PathError = %v, want original technical error", pathCause)
	}
}
