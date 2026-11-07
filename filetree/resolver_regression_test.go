package filetree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func resolverRegressionLoopCause(t *testing.T, path string, err *diagnostic.Error) {
	t.Helper()
	if err == nil {
		t.Fatalf("unresolvable loop %q must return an error", path)
	}
	if err.Code != CodeLink || err.Context.Path != path {
		t.Errorf("loop diagnostic = %+v; want CodeLink and path %q", err, path)
	}
	if !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("loop reason must explain the failure in Russian: %q", err.Reason)
	}
	if err.Cause == nil {
		t.Fatal("unresolvable filesystem loop lost its original technical Cause")
	}
	if errors.Unwrap(err) != err.Cause || !errors.Is(err, err.Cause) {
		t.Errorf("original Cause is unavailable through the standard error chain: %+v", err)
	}
	var diagnosticError *diagnostic.Error
	if !errors.As(err, &diagnosticError) || diagnosticError != err {
		t.Error("standard errors.As cannot recover the diagnostic")
	}
	// Контракт не фиксирует syscall, текст или тип технической ошибки.
	// Если исходная причина содержит PathError, цепочка сохраняет и его.
	var originalPathError *fs.PathError
	if errors.As(err.Cause, &originalPathError) {
		var chainedPathError *fs.PathError
		if !errors.As(err, &chainedPathError) || chainedPathError != originalPathError {
			t.Error("standard errors.As cannot recover the original filesystem PathError")
		}
		if originalPathError.Err != nil && !errors.Is(err, originalPathError.Err) {
			t.Error("standard errors.Is cannot reach the original filesystem failure")
		}
	}
	t.Logf("preserved technical Cause type: %T", err.Cause)
}

func resolverRegressionCheckLoop(t *testing.T, paths []string) {
	t.Helper()
	var resolver Resolver
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := os.Stat(path); err == nil {
				t.Fatalf("fixture %q must be a real unresolvable filesystem loop", path)
			}
			t.Run("Inspect", func(t *testing.T) {
				_, err := resolver.Inspect(path)
				resolverRegressionLoopCause(t, path, err)
			})
			t.Run("resolveExisting", func(t *testing.T) {
				_, _, err := resolveExisting(path)
				resolverRegressionLoopCause(t, path, err)
			})
		})
	}
}

func TestRegressionResolverSelfLoopPreservesTechnicalCause(t *testing.T) {
	path := filepath.Join(t.TempDir(), "петля с пробелом")
	if err := os.Symlink(filepath.Base(path), path); err != nil {
		t.Fatalf("cannot create required self-loop symlink fixture: %v", err)
	}
	resolverRegressionCheckLoop(t, []string{path})
}

func TestRegressionResolverMultiLinkLoopPreservesTechnicalCause(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "первая ссылка"),
		filepath.Join(root, "вторая ссылка"),
		filepath.Join(root, "третья ссылка"),
	}
	for i, path := range paths {
		target := paths[(i+1)%len(paths)]
		if err := os.Symlink(filepath.Base(target), path); err != nil {
			t.Fatalf("cannot create required multi-link loop fixture: %v", err)
		}
	}
	resolverRegressionCheckLoop(t, paths)
}
