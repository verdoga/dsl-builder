package safewrite_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestWritePreflightDoesNotCreateFilesOrCallCallbacks(t *testing.T) {
	for _, scenario := range []string{"nil write", "nil verify", "both nil", "empty", "relative", "directory", "overwrite refused"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "archive.bin")
			writeStore(t, target, "original")
			calls := 0
			var write safewrite.WriteFunc = func(io.Writer) error { calls++; return nil }
			var verify safewrite.VerifyFunc = func(string) error { calls++; return nil }
			replace := true
			switch scenario {
			case "nil write":
				write = nil
			case "nil verify":
				verify = nil
			case "both nil":
				write, verify = nil, nil
			case "empty":
				target = ""
			case "relative":
				target = filepath.Join("relative", "archive.bin")
			case "directory":
				target = dir
			case "overwrite refused":
				replace = false
			}
			before := writeNames(t, dir)
			result, failures := safewrite.Write(target, replace, write, verify)
			if calls != 0 || len(failures) == 0 || result.Installed || result.TemporaryPath != "" {
				t.Fatalf("calls=%d result=%+v errors=%v", calls, result, failures)
			}
			writeDiagnostic(t, failures[0], target)
			if !slices.Equal(before, writeNames(t, dir)) {
				t.Fatal("preflight created or removed files")
			}
			writeContents(t, filepath.Join(dir, "archive.bin"), "original")
		})
	}
}

func TestWriteInstallsOnlyAfterSuccessfulCallbacks(t *testing.T) {
	for _, scenario := range []struct{ existing, replace bool }{{false, false}, {false, true}, {true, true}} {
		t.Run(fmt.Sprint(scenario), func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "archive.bin")
			if scenario.existing {
				writeStore(t, target, "original")
			}
			writeStore(t, filepath.Join(dir, "unrelated"), "keep")
			var events []string
			checkOriginal := func() { t.Helper(); writeTarget(t, target, scenario.existing, "original") }
			result, failures := safewrite.Write(target, scenario.replace, func(dst io.Writer) error {
				events = append(events, "write")
				checkOriginal()
				_, err := io.WriteString(dst, "complete candidate")
				checkOriginal()
				return err
			}, func(candidate string) error {
				events = append(events, "verify")
				checkOriginal()
				writeCandidatePath(t, target, candidate)
				writeContents(t, candidate, "complete candidate")
				return nil
			})
			if len(failures) != 0 || result != (safewrite.Result{TargetPath: target, Installed: true}) {
				t.Fatalf("result=%+v errors=%v", result, failures)
			}
			if !slices.Equal(events, []string{"write", "verify"}) {
				t.Fatalf("callback order/count=%v", events)
			}
			writeContents(t, target, "complete candidate")
			writeContents(t, filepath.Join(dir, "unrelated"), "keep")
			if !slices.Equal(writeNames(t, dir), []string{"archive.bin", "unrelated"}) {
				t.Fatal("successful transaction left service files")
			}
		})
	}
}

func TestWriteCallbackFailuresPreserveTargetAndErrorChains(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, stage := range []string{"write", "verify"} {
			t.Run(fmt.Sprintf("existing=%t/%s", existing, stage), func(t *testing.T) {
				dir := t.TempDir()
				target := filepath.Join(dir, "archive.bin")
				if existing {
					writeStore(t, target, "original")
				}
				root := &writeCause{message: "callback root"}
				callbackErr := &diagnostic.Error{Code: "format.invalid", Reason: "Ошибка формата", Context: diagnostic.Context{Path: "source.json", FileName: "lesson.json", LessonID: "001", PageID: "001.002", Line: 17, Column: 3}, Cause: fmt.Errorf("wrapped: %w", root)}
				var events []string
				candidate := ""
				result, failures := safewrite.Write(target, true, func(dst io.Writer) error {
					events = append(events, "write")
					if _, err := io.WriteString(dst, "partial candidate"); err != nil {
						return err
					}
					if stage == "write" {
						return callbackErr
					}
					return nil
				}, func(path string) error { events = append(events, "verify"); candidate = path; return callbackErr })
				wantEvents := []string{"write"}
				if stage == "verify" {
					wantEvents = append(wantEvents, "verify")
				}
				if !slices.Equal(events, wantEvents) || len(failures) != 1 {
					t.Fatalf("events=%v errors=%v", events, failures)
				}
				failure := failures[0]
				if candidate == "" {
					candidate = failure.Context.Path
				}
				writeCandidatePath(t, target, candidate)
				writeDiagnostic(t, failure, candidate)
				if !slices.ContainsFunc(failure.Related, func(c diagnostic.Context) bool { return c.Path == candidate }) || (existing && !slices.ContainsFunc(failure.Related, func(c diagnostic.Context) bool { return c.Path == target })) {
					t.Fatalf("known recovery paths missing: %#v", failure.Related)
				}
				var nested *diagnostic.Error
				var typed *writeCause
				if !errors.Is(failure, root) || !errors.Is(failure, callbackErr) || !errors.As(failure, &typed) || typed != root || !errors.As(failure.Cause, &nested) || nested != callbackErr {
					t.Fatalf("callback chain lost: %#v", failure)
				}
				wantOriginal := ""
				if existing {
					wantOriginal = target
				}
				if result != (safewrite.Result{TargetPath: target, OriginalPath: wantOriginal}) {
					t.Fatalf("failure result=%+v", result)
				}
				writeTarget(t, target, existing, "original")
				if len(writeNames(t, dir)) != map[bool]int{false: 0, true: 1}[existing] {
					t.Fatal("callback failure left service files")
				}
			})
		}
	}
}

func TestWriteRejectsTargetChangedDuringVerify(t *testing.T) {
	for _, change := range []string{"appeared", "disappeared", "size", "mtime", "identity"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "archive.bin")
			if change != "appeared" {
				writeStore(t, target, "original")
			}
			calls := 0
			result, failures := safewrite.Write(target, true, func(dst io.Writer) error { calls++; _, err := io.WriteString(dst, "candidate"); return err }, func(candidate string) error {
				calls++
				writeContents(t, candidate, "candidate")
				if change == "mtime" || change == "identity" {
					info, err := os.Stat(target)
					if err != nil {
						t.Fatal(err)
					}
					stamp := info.ModTime().Add(time.Hour)
					if change == "identity" {
						if err := os.Rename(target, filepath.Join(dir, "external-original")); err != nil {
							t.Fatal(err)
						}
						writeStore(t, target, "original")
						stamp = info.ModTime()
					}
					return os.Chtimes(target, stamp, stamp)
				}
				if change == "disappeared" {
					if err := os.Remove(target); err != nil {
						t.Fatal(err)
					}
				}
				if change != "disappeared" {
					writeStore(t, target, "external replacement")
				}
				return nil
			})
			if calls != 2 || len(failures) == 0 || result.Installed || result.TargetPath != target || result.TemporaryPath != "" {
				t.Fatalf("calls=%d result=%+v errors=%v", calls, result, failures)
			}
			writeDiagnostic(t, failures[0], target)
			contents := "external replacement"
			if change == "mtime" || change == "identity" {
				contents = "original"
			}
			writeTarget(t, target, change != "disappeared", contents)
			if (change == "appeared" && result.OriginalPath != "") || (change != "appeared" && result.OriginalPath != target) {
				t.Fatalf("original location lost: %+v", result)
			}
			wantCount := 1
			if change == "identity" {
				wantCount = 2
				writeContents(t, filepath.Join(dir, "external-original"), "original")
			}
			if change == "disappeared" {
				wantCount = 0
			}
			if len(writeNames(t, dir)) != wantCount {
				t.Fatal("changed target left service files")
			}
		})
	}
}

func TestWriteMissingParentPreservesFilesystemCause(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing", "archive.bin")
	calls := 0
	result, failures := safewrite.Write(target, false, func(io.Writer) error { calls++; return nil }, func(string) error { calls++; return nil })
	if calls != 0 || len(failures) == 0 || result.Installed || result.TemporaryPath != "" || len(writeNames(t, dir)) != 0 {
		t.Fatalf("calls=%d result=%+v errors=%v", calls, result, failures)
	}
	var pathErr *os.PathError
	if !errors.Is(failures[0], os.ErrNotExist) || !errors.As(failures[0], &pathErr) {
		t.Fatalf("filesystem cause lost: %v", failures)
	}
}

type writeCause struct{ message string }

func (e *writeCause) Error() string { return e.message }

func writeStore(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeContents(t *testing.T, path, contents string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != contents {
		t.Fatalf("read %q = %q, %v; want %q", path, got, err, contents)
	}
}

func writeTarget(t *testing.T, path string, exists bool, contents string) {
	t.Helper()
	if exists {
		writeContents(t, path, contents)
		return
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target %q unexpectedly exists: %v", path, err)
	}
}

func writeNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func writeCandidatePath(t *testing.T, target, candidate string) {
	t.Helper()
	if !filepath.IsAbs(candidate) || candidate == target || filepath.Dir(candidate) != filepath.Dir(target) {
		t.Fatalf("candidate %q is not beside target %q", candidate, target)
	}
}

func writeDiagnostic(t *testing.T, failure *diagnostic.Error, path string) {
	t.Helper()
	if failure == nil || !strings.Contains(failure.Code, "safewrite") || failure.Context.Path != path || len(failure.Details) == 0 || !strings.ContainsFunc(failure.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Fatalf("incomplete diagnostic: %#v; path %q", failure, path)
	}
}
