package resourcefind

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

func TestScopeResolveDirectoriesExactOrder(t *testing.T) {
	base := filepath.Join(t.TempDir(), "JSON адрес", "Урок")
	absolute := filepath.Join(t.TempDir(), "Внешние ресурсы")
	values := []string{"../Audio", "  пробелы  ", `"quoted"`, `буквально\n`, absolute, "../Audio", ".", "missing"}
	want := []string{filepath.Join(filepath.Dir(base), "Audio"), filepath.Join(base, "  пробелы  "), filepath.Join(base, `"quoted"`), filepath.Join(base, `буквально\n`), absolute, filepath.Join(filepath.Dir(base), "Audio"), base, filepath.Join(base, "missing")}
	before := slices.Clone(values)
	got, err := resolveDirectories(base, values, scopeContext(base))
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("resolveDirectories = %q, %v; want %q", got, err, want)
	}
	if !slices.Equal(values, before) {
		t.Fatal("изменён входной resourceDirs")
	}
	got[0] = "changed"
	if !slices.Equal(values, before) {
		t.Fatal("результат разделяет входной срез")
	}
	for _, empty := range [][]string{nil, {}} {
		got, err := resolveDirectories("not absolute", empty, diagnostic.Context{})
		if err != nil || len(got) != 0 {
			t.Fatalf("empty = %q, %v", got, err)
		}
	}
	if runtime.GOOS == "windows" {
		got, err := resolveDirectories(`C:\JSON\Урок`, []string{`D:\Медиа\..\Видео`, `\\server\share\Audio`, `C:local`, `\root`}, diagnostic.Context{})
		want := []string{`D:\Видео`, `\\server\share\Audio`, `C:\JSON\Урок\local`, `C:\root`}
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("Windows = %q, %v; want %q", got, err, want)
		}
	}
}

func TestScopeResolveDirectoriesEveryErrorIsAtomic(t *testing.T) {
	base := t.TempDir()
	tests := []struct{ name, base, value string }{
		{"empty", base, ""}, {"nul", base, "bad\x00path"},
		{"relative base", "relative", "Audio"}, {"nul base", base + "\x00", "Audio"},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, struct{ name, base, value string }{"other drive relative", `C:\JSON`, `D:Audio`})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, expected := filetree.Resolve(tt.base, tt.value)
			if expected == nil {
				t.Fatal("fixture не вызывает ошибку filetree.Resolve")
			}
			context := scopeContext(base)
			for _, values := range [][]string{{tt.value}, {base, tt.value, base}} {
				got, issue := resolveDirectories(tt.base, values, context)
				if got != nil {
					t.Fatalf("частичный результат: %q", got)
				}
				scopeError(t, issue, expected, context)
			}
		})
	}
}

func scopeContext(base string) diagnostic.Context {
	return diagnostic.Context{Path: filepath.Join(base, "источник.json"), FileName: "l007.json", LessonID: "007ЁЖ", PageID: "007ёж-p002", Line: 35, Column: 9}
}

func scopeWrite(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("resource\x00\xff"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func scopeError(t *testing.T, got, want *diagnostic.Error, context diagnostic.Context) {
	t.Helper()
	if got == nil {
		t.Fatal("ожидалась блокирующая ошибка")
	}
	if got.Code != want.Code || got.Reason != want.Reason || got.Context != want.Context {
		t.Fatalf("ошибка = %#v; исходная filetree = %#v", got, want)
	}
	if !slices.Contains(got.Related, context) {
		t.Fatalf("нет точного JSON context в Related: %#v", got.Related)
	}
	if want.Cause == nil {
		if got.Cause != nil {
			t.Fatalf("неожиданная Cause: %v", got.Cause)
		}
	} else {
		var original, actual *os.PathError
		if errors.As(want.Cause, &original) {
			if !errors.As(got.Cause, &actual) || actual.Op != original.Op || actual.Path != original.Path || !errors.Is(actual, original.Err) {
				t.Fatalf("Cause = %#v; want %#v", got.Cause, original)
			}
		} else if got.Cause == nil {
			t.Fatal("потеряна Cause")
		}
	}
}

func scopePaths(entries []filetree.Entry) []string {
	paths := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = entry.Path
	}
	return paths
}
