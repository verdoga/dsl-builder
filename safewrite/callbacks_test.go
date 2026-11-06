package safewrite_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestCallbacksFunctionTypesPassArgumentsAndErrors(t *testing.T) {
	var zeroWrite safewrite.WriteFunc
	var zeroVerify safewrite.VerifyFunc
	if zeroWrite != nil || zeroVerify != nil {
		t.Fatal("zero callbacks must be nil")
	}

	var dst bytes.Buffer
	wantErr := errors.New("ошибка callback")
	writeCalled := false
	var write safewrite.WriteFunc = func(got io.Writer) error {
		writeCalled = true
		if got != &dst {
			t.Fatal("WriteFunc did not receive the original stream")
		}
		if _, err := io.WriteString(got, "точные байты\x00"); err != nil {
			return err
		}
		return wantErr
	}
	var plainWrite func(io.Writer) error = write
	if err := plainWrite(&dst); err != wantErr || !writeCalled {
		t.Fatalf("WriteFunc: error = %v, called = %v", err, writeCalled)
	}
	if got := dst.String(); got != "точные байты\x00" {
		t.Fatalf("stream contents = %q", got)
	}

	path := filepath.Join(t.TempDir(), "временный файл.zip")
	verifyCalled := false
	var verify safewrite.VerifyFunc = func(got string) error {
		verifyCalled = true
		if got != path {
			t.Fatalf("VerifyFunc path = %q, want %q", got, path)
		}
		return wantErr
	}
	var plainVerify func(string) error = verify
	if err := plainVerify(path); err != wantErr || !verifyCalled {
		t.Fatalf("VerifyFunc: error = %v, called = %v", err, verifyCalled)
	}
	if err := safewrite.WriteFunc(func(io.Writer) error { return nil })(&dst); err != nil {
		t.Fatalf("successful WriteFunc returned %v", err)
	}
	if err := safewrite.VerifyFunc(func(string) error { return nil })(path); err != nil {
		t.Fatalf("successful VerifyFunc returned %v", err)
	}
}

func TestCallbacksWriteCompletesFormatBeforeSynchronousVerification(t *testing.T) {
	target := filepath.Join(t.TempDir(), "модуль с пробелом.zip")
	payload := []byte("содержимое урока\x00\xff")
	var events []string
	writeReturned := false
	write := safewrite.WriteFunc(func(dst io.Writer) error {
		events = append(events, "write")
		defer func() {
			writeReturned = true
			events = append(events, "write returned")
		}()
		format := zip.NewWriter(dst)
		entry, err := format.Create("урок.txt")
		if err != nil {
			return errors.Join(err, format.Close())
		}
		_, writeErr := entry.Write(payload)
		closeErr := format.Close()
		events = append(events, "format closed")
		return errors.Join(writeErr, closeErr)
	})
	verify := safewrite.VerifyFunc(func(path string) error {
		events = append(events, "verify")
		defer func() { events = append(events, "verify returned") }()
		if !writeReturned {
			t.Error("verification started before write returned")
		}
		if !filepath.IsAbs(path) || filepath.Dir(path) != filepath.Dir(target) || path == target {
			t.Errorf("verification must receive a full temporary sibling path, got %q", path)
		}
		if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("target exists before verification finishes: %v", err)
		}
		archive, err := zip.OpenReader(path)
		if err != nil {
			return err
		}
		if len(archive.File) != 1 || archive.File[0].Name != "урок.txt" {
			t.Errorf("unexpected ZIP composition: %+v", archive.File)
			return archive.Close()
		}
		entry, err := archive.File[0].Open()
		if err != nil {
			return errors.Join(err, archive.Close())
		}
		got, readErr := io.ReadAll(entry)
		entryCloseErr := entry.Close()
		archiveCloseErr := archive.Close()
		if !bytes.Equal(got, payload) {
			t.Errorf("verified payload = %q, want %q", got, payload)
		}
		return errors.Join(readErr, entryCloseErr, archiveCloseErr)
	})

	result, errs := safewrite.Write(target, false, write, verify)
	if len(errs) != 0 || !result.Installed {
		t.Fatalf("Write = %+v, errors = %v", result, errs)
	}
	wantEvents := []string{"write", "format closed", "write returned", "verify", "verify returned"}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("callback order/count = %v, want %v", events, wantEvents)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("successful callbacks did not produce target: %v", err)
	}
}

func TestCallbacksWritePreservesOriginalCallbackError(t *testing.T) {
	rootCause := errors.New("исходная техническая ошибка")
	callbackDiagnostic := &diagnostic.Error{
		Code:   "format.invalid",
		Reason: "Ошибка состава формата",
		Context: diagnostic.Context{
			Path: "исходный архив.zip", FileName: "lesson.json",
			LessonID: "001", PageID: "001.002", Line: 7, Column: 3,
		},
		Related: []diagnostic.Context{{Path: "связанный файл"}},
		Details: []diagnostic.Detail{{Label: "Подробность", Value: "точное значение"}},
		Cause:   rootCause,
	}
	for _, stage := range []string{"write", "verify"} {
		for _, failure := range []struct {
			name string
			err  error
		}{
			{name: "technical", err: rootCause},
			{name: "diagnostic", err: callbackDiagnostic},
		} {
			t.Run(stage+"/"+failure.name, func(t *testing.T) {
				target := filepath.Join(t.TempDir(), "исходный модуль.zip")
				original := []byte("исходные байты\x00\xff")
				if err := os.WriteFile(target, original, 0600); err != nil {
					t.Fatal(err)
				}
				writeCalls, verifyCalls := 0, 0
				write := safewrite.WriteFunc(func(dst io.Writer) error {
					writeCalls++
					if _, err := io.WriteString(dst, "частичный или готовый кандидат"); err != nil {
						return err
					}
					if stage == "write" {
						return failure.err
					}
					return nil
				})
				verify := safewrite.VerifyFunc(func(string) error {
					verifyCalls++
					return failure.err
				})
				result, errs := safewrite.Write(target, true, write, verify)
				if result.Installed {
					t.Error("failed callback must prevent installation")
				}
				wantVerify := 0
				if stage == "verify" {
					wantVerify = 1
				}
				if writeCalls != 1 || verifyCalls != wantVerify {
					t.Errorf("callback counts = (%d, %d), want (1, %d)", writeCalls, verifyCalls, wantVerify)
				}
				if len(errs) != 1 || errs[0] == nil {
					t.Fatalf("expected one callback failure, got %v", errs)
				}
				if errs[0].Cause != failure.err {
					t.Errorf("Cause = %v, want original callback error %v", errs[0].Cause, failure.err)
				}
				if !errors.Is(errs[0], rootCause) {
					t.Error("callback's technical error was lost from the error chain")
				}
				if failure.err == callbackDiagnostic {
					var gotDiagnostic *diagnostic.Error
					if !errors.As(errs[0].Cause, &gotDiagnostic) || gotDiagnostic != callbackDiagnostic {
						t.Error("callback's structured diagnostic was lost or replaced")
					}
				}
				got, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(got, original) {
					t.Errorf("original after callback failure = %q, error = %v", got, err)
				}
			})
		}
	}
}
