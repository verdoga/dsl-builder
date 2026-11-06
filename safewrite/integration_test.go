package safewrite_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestIntegrationWriteInstallsCompleteZIP(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing bool
		replace  bool
	}{
		{"new", false, false},
		{"new_with_permission", false, true},
		{"overwrite", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for range 4 {
				dir = filepath.Join(dir, "Длинный каталог "+strings.Repeat("segment", 8)+" 世界")
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, "Учебный модуль 世界 №1.zip")
			original := []byte("исходный архив\x00\xff\r\n")
			if tc.existing {
				if err := os.WriteFile(target, original, 0600); err != nil {
					t.Fatal(err)
				}
			}
			unrelated := filepath.Join(dir, "чужая резервная копия.zip")
			if err := os.WriteFile(unrelated, original, 0600); err != nil {
				t.Fatal(err)
			}
			payload := bytes.Repeat([]byte("Сохранённый урок\r\n\x00\xff"), 4096)
			for b := range 256 {
				payload = append(payload, byte(b))
			}
			entries := []struct {
				name string
				body []byte
			}{{"manifest.json", []byte("{\"title\":\"Курс 世界\"}\n")}, {"lesson1.json", payload}}
			var written bytes.Buffer
			writes, verifies := 0, 0
			candidate := ""
			result, errs := safewrite.Write(target, tc.replace, func(dst io.Writer) error {
				writes++
				integrationTargetState(t, target, tc.existing, original)
				archive := zip.NewWriter(io.MultiWriter(dst, &written))
				for _, entry := range entries {
					stream, err := archive.Create(entry.name)
					if err != nil {
						return errors.Join(err, archive.Close())
					}
					if _, err := stream.Write(entry.body); err != nil {
						return errors.Join(err, archive.Close())
					}
				}
				err := archive.Close()
				integrationTargetState(t, target, tc.existing, original)
				return err
			}, func(path string) error {
				verifies++
				candidate = path
				integrationCandidatePath(t, target, path)
				integrationTargetState(t, target, tc.existing, original)
				integrationFileBytes(t, path, written.Bytes())
				archive, err := zip.OpenReader(path)
				if err != nil {
					t.Fatalf("verify cannot read completed ZIP: %v", err)
				}
				if len(archive.File) != len(entries) {
					t.Fatalf("ZIP entries = %d, want %d", len(archive.File), len(entries))
				}
				for i, entry := range archive.File {
					if entry.Name != entries[i].name {
						t.Errorf("entry = %q, want %q", entry.Name, entries[i].name)
					}
					stream, err := entry.Open()
					if err != nil {
						t.Fatal(err)
					}
					body, readErr := io.ReadAll(stream)
					if err := errors.Join(readErr, stream.Close()); err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(body, entries[i].body) {
						t.Errorf("entry %q bytes differ", entry.Name)
					}
				}
				err = archive.Close()
				integrationTargetState(t, target, tc.existing, original)
				return err
			})
			if len(errs) != 0 {
				t.Fatalf("Write errors: %v", errs)
			}
			if result != (safewrite.Result{TargetPath: target, Installed: true}) {
				t.Errorf("Result = %+v", result)
			}
			if writes != 1 || verifies != 1 {
				t.Errorf("callback calls = %d/%d, want 1/1", writes, verifies)
			}
			integrationFileBytes(t, target, written.Bytes())
			integrationFileBytes(t, unrelated, original)
			integrationTargetState(t, candidate, false, nil)
			integrationDirectoryNames(t, dir, filepath.Base(target), filepath.Base(unrelated))
		})
	}
}

func TestIntegrationWriteCallbackFailuresPreserveFilesAndDiagnostic(t *testing.T) {
	codes := make(map[string]string)
	for _, stage := range []string{"write", "verify"} {
		for _, existing := range []bool{false, true} {
			name := stage + "_new"
			if existing {
				name = stage + "_existing"
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				target := filepath.Join(dir, "Модуль с пробелом 世界.zip")
				original := []byte("исходник\x00\xff\r\n")
				if existing {
					if err := os.WriteFile(target, original, 0600); err != nil {
						t.Fatal(err)
					}
				}
				first, second := errors.New("first callback cause"), errors.New("second callback cause")
				formatError := &diagnostic.Error{
					Code: "format.lesson", Reason: "Нарушен состав урока",
					Context: diagnostic.Context{Path: "исходный JSON.json", FileName: "lesson1.json", LessonID: "0007", PageID: "0007.002", Line: 17, Column: 9},
					Related: []diagnostic.Context{{Path: "ресурс 世界.bin"}},
					Details: []diagnostic.Detail{{Label: "Значение", Value: " 0007 "}}, Cause: second,
				}
				callbackError := errors.Join(first, formatError)
				payload := bytes.Repeat([]byte("полный кандидат\x00\xff"), 512)
				writes, verifies := 0, 0
				candidate := ""
				result, errs := safewrite.Write(target, true, func(dst io.Writer) error {
					writes++
					integrationTargetState(t, target, existing, original)
					_, err := dst.Write(payload)
					integrationTargetState(t, target, existing, original)
					if stage == "write" {
						return errors.Join(err, callbackError)
					}
					return err
				}, func(path string) error {
					verifies++
					candidate = path
					integrationCandidatePath(t, target, path)
					integrationFileBytes(t, path, payload)
					integrationTargetState(t, target, existing, original)
					return callbackError
				})
				if len(errs) != 1 || errs[0] == nil {
					t.Fatalf("errors = %v, want one callback-stage diagnostic", errs)
				}
				failure := errs[0]
				if !strings.HasPrefix(failure.Code, "safewrite.") || !strings.Contains(strings.ToLower(failure.Code), stage) || failure.Reason == "" {
					t.Errorf("stage diagnostic = %+v", failure)
				}
				if code := codes[stage]; code != "" && code != failure.Code {
					t.Errorf("unstable code: %q versus %q", code, failure.Code)
				}
				codes[stage] = failure.Code
				integrationCandidatePath(t, target, failure.Context.Path)
				if candidate != "" && failure.Context.Path != candidate {
					t.Errorf("diagnostic path = %q, verified %q", failure.Context.Path, candidate)
				}
				for _, cause := range []error{first, second, formatError, callbackError} {
					if !errors.Is(failure, cause) {
						t.Errorf("lost callback cause %v", cause)
					}
				}
				var nested *diagnostic.Error
				if !errors.As(failure.Cause, &nested) || nested != formatError {
					t.Errorf("lost diagnostic with ID, coordinates, related paths and details: %+v", nested)
				} else {
					wantContext := diagnostic.Context{Path: "исходный JSON.json", FileName: "lesson1.json", LessonID: "0007", PageID: "0007.002", Line: 17, Column: 9}
					if nested.Context != wantContext || nested.Code != "format.lesson" || nested.Reason != "Нарушен состав урока" || nested.Cause != second {
						t.Errorf("callback diagnostic changed: %+v", nested)
					}
					if len(nested.Related) != 1 || nested.Related[0].Path != "ресурс 世界.bin" || len(nested.Details) != 1 || nested.Details[0] != (diagnostic.Detail{Label: "Значение", Value: " 0007 "}) {
						t.Errorf("callback related paths or details changed: %+v", nested)
					}
				}
				if len(failure.Related) == 0 || len(failure.Details) == 0 {
					t.Errorf("missing transaction context: %+v", failure)
				}
				want := safewrite.Result{TargetPath: target}
				if existing {
					want.OriginalPath = target
				}
				if result != want {
					t.Errorf("Result = %+v, want %+v", result, want)
				}
				wantVerifies := 0
				if stage == "verify" {
					wantVerifies = 1
				}
				if writes != 1 || verifies != wantVerifies {
					t.Errorf("callback calls = %d/%d, want 1/%d", writes, verifies, wantVerifies)
				}
				integrationTargetState(t, target, existing, original)
				integrationTargetState(t, failure.Context.Path, false, nil)
				if existing {
					integrationDirectoryNames(t, dir, filepath.Base(target))
				} else {
					integrationDirectoryNames(t, dir)
				}
			})
		}
	}
}

func integrationCandidatePath(t *testing.T, target, candidate string) {
	t.Helper()
	if !filepath.IsAbs(candidate) || filepath.Dir(candidate) != filepath.Dir(target) || candidate == target {
		t.Errorf("candidate %q must be an absolute sibling of %q", candidate, target)
	}
}

func integrationTargetState(t *testing.T, path string, exists bool, want []byte) {
	t.Helper()
	if exists {
		integrationFileBytes(t, path, want)
		return
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("path %q should be absent, Lstat error = %v", path, err)
	}
}

func integrationFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("bytes at %q differ: got %d bytes, want %d", path, len(got), len(want))
	}
}

func integrationDirectoryNames(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(names) {
		t.Fatalf("directory %q has %d entries, want %d: %v", dir, len(entries), len(names), entries)
	}
	for _, entry := range entries {
		found := false
		for _, name := range names {
			found = found || entry.Name() == name
		}
		if !found {
			t.Errorf("unexpected file %q", entry.Name())
		}
	}
}
