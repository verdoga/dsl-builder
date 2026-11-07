package filetree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func resolverExtendedDiagnostic(t *testing.T, path, code string, err *diagnostic.Error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%q: expected %s", path, code)
	}
	if err.Code != code || (err.Context.Path != path && err.Context.Path != filepath.Clean(path)) {
		t.Errorf("diagnostic = %+v; want %s at %q", err, code, path)
	}
	if !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("reason must explain the failure in Russian: %q", err.Reason)
	}
	if err.Cause == nil || errors.Unwrap(err) != err.Cause || !errors.Is(err, err.Cause) {
		t.Fatalf("filesystem Cause is missing from the error chain: %+v", err)
	}
}

func resolverExtendedExisting(t *testing.T, path string) {
	t.Helper()
	want, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved, info, failure := resolveExisting(path)
	if failure != nil {
		t.Fatalf("resolveExisting(%q): %+v", path, failure)
	}
	if !filepath.IsAbs(resolved) || filepath.Clean(resolved) != resolved || info == nil || !os.SameFile(want, info) {
		t.Fatalf("incorrect target path or metadata: %q, %+v", resolved, info)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil || !os.SameFile(want, resolvedInfo) {
		t.Fatalf("resolved path %q does not address the original object: %v", resolved, err)
	}
	var resolver Resolver
	entry, failure := resolver.Inspect(path)
	if failure != nil {
		t.Fatalf("Inspect(%q): %+v", path, failure)
	}
	if entry.Path != filepath.Clean(path) || entry.IdentityPath != resolved || entry.Info == nil || !os.SameFile(want, entry.Info) {
		t.Fatalf("incorrect discovery path, identity or metadata: %+v; resolved %q", entry, resolved)
	}
}

func TestResolverF004ExistingVolumeRootAndFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "Файл с пробелом.json")
	if err := os.WriteFile(path, []byte("not JSON\x00\xff"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path string }{
		{"VolumeRoot", root}, {"Directory", directory}, {"File", path},
	} {
		t.Run(tc.name, func(t *testing.T) { resolverExtendedExisting(t, tc.path) })
	}
}

func TestResolverF004MissingAndLinkComponentKeepCause(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "отсутствующий.json")
	broken := filepath.Join(root, "неразрешимая ссылка")
	if err := os.Symlink("нет каталога", broken); err != nil {
		t.Fatalf("cannot create required broken-link fixture: %v", err)
	}
	for _, tc := range []struct{ name, path, code string }{
		{"Missing", missing, CodeAccess},
		{"MissingThroughLink", filepath.Join(broken, "file.json"), CodeLink},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resolver Resolver
			_, inspectFailure := resolver.Inspect(tc.path)
			resolverExtendedDiagnostic(t, tc.path, tc.code, inspectFailure)
			_, _, resolveFailure := resolveExisting(tc.path)
			resolverExtendedDiagnostic(t, tc.path, tc.code, resolveFailure)
			for _, failure := range []*diagnostic.Error{inspectFailure, resolveFailure} {
				if !errors.Is(failure, fs.ErrNotExist) {
					t.Errorf("original missing-object Cause is unreachable: %+v", failure)
				}
			}
		})
	}
}

func resolverExtendedUNCCause(t *testing.T, err *diagnostic.Error) {
	t.Helper()
	var pathError *fs.PathError
	if !errors.As(err.Cause, &pathError) {
		// Не все технические причины обязаны быть PathError: например,
		// нормализация Windows может вернуть непосредственно syscall.Errno.
		t.Logf("Cause %T does not expose an addressed path; root observation unavailable", err.Cause)
		return
	}
	// Допустимы обе полные формы UNC: реализация может передать stdlib
	// обычную форму, не меняя адрес обнаружения Entry.Path.
	path := strings.ReplaceAll(pathError.Path, "/", `\`)
	path = strings.TrimSuffix(path, `\`)
	if strings.EqualFold(path, `\\?\UNC\localhost`) || strings.EqualFold(path, `\\localhost`) {
		t.Errorf("filesystem operation addressed an incomplete UNC host: %q", pathError.Path)
	}
}

func TestResolverF004ExtendedUNCKeepsCompleteRootOnFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("NOT RUN: extended UNC filesystem namespace requires Windows")
	}
	// Это read-only probe локального namespace, а не требование наличия SMB
	// службы или share. Отсутствие share — ожидаемо допустимая ветвь проверки.
	// Stat задаёт наблюдаемый oracle существования, имя share не считается
	// гарантированно отсутствующим и никакая сеть/служба не настраивается.
	root := `\\?\UNC\localhost\filetree-F004-resolver-regression`
	for _, tc := range []struct{ name, path string }{
		{"ShareRoot", root}, {"ShareRootTrailingSeparator", root + `\`},
		{"File", root + `\file.json`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, referenceError := os.Stat(tc.path)
			if referenceError == nil {
				resolverExtendedExisting(t, tc.path)
				return
			}
			// Прямой Stat может следовать ссылке. При наличии такой ссылки
			// oracle кода определяется её реальными метаданными, не именем.
			code := CodeAccess
			if info, err := os.Lstat(tc.path); err == nil && info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
				code = CodeLink
			}
			var resolver Resolver
			_, inspectFailure := resolver.Inspect(tc.path)
			resolverExtendedDiagnostic(t, tc.path, code, inspectFailure)
			resolverExtendedUNCCause(t, inspectFailure)
			_, _, resolveFailure := resolveExisting(tc.path)
			resolverExtendedDiagnostic(t, tc.path, code, resolveFailure)
			resolverExtendedUNCCause(t, resolveFailure)
		})
	}
}
