package safewrite

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func candidateQANew(t *testing.T, target string) *transaction {
	t.Helper()
	tx := &transaction{target: target}
	if err := tx.createCandidate(); err != nil {
		t.Fatalf("createCandidate: %v", err)
	}
	t.Cleanup(func() {
		if tx.candidate != nil {
			_ = tx.candidate.Close()
		}
	})
	return tx
}

func candidateQADiagnostic(t *testing.T, err *diagnostic.Error, path string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected diagnostic")
	}
	if !strings.Contains(strings.ToLower(err.Code), "safewrite") {
		t.Errorf("stage code must identify safewrite: %q", err.Code)
	}
	if !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("expected Russian reason: %q", err.Reason)
	}
	if err.Context.Path == "" || !filepath.IsAbs(err.Context.Path) {
		t.Errorf("expected concrete full error path: %q", err.Context.Path)
	}
	if path != "" && err.Context.Path != path {
		t.Errorf("path = %q, want %q", err.Context.Path, path)
	}
	if len(err.Details) == 0 {
		t.Error("missing file-state details")
	}
}

func candidateQABytes(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("ReadFile(%q) = %q, %v; want %q", path, got, err, want)
	}
}

func TestCandidateQACreateExclusiveNeighbors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Каталог с пробелами 日本語")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "Архив урока.zip")
	neighbor := filepath.Join(dir, "посторонний.tmp")
	for _, path := range []string{target, neighbor} {
		if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, second := candidateQANew(t, target), candidateQANew(t, target)
	if first.candidatePath == second.candidatePath {
		t.Fatal("two live transactions own the same candidate")
	}
	for _, tx := range []*transaction{first, second} {
		if !filepath.IsAbs(tx.candidatePath) || filepath.Dir(tx.candidatePath) != dir || tx.candidatePath == target || tx.candidatePath == neighbor {
			t.Errorf("candidate is not a distinct full neighboring path: %q", tx.candidatePath)
		}
		if tx.candidate == nil {
			t.Fatal("missing owned descriptor")
		}
		info, err := tx.candidate.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
			t.Fatalf("new candidate: info=%v, err=%v", info, err)
		}
		file, err := os.OpenFile(tx.candidatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if file != nil {
			_ = file.Close()
		}
		if !errors.Is(err, os.ErrExist) {
			t.Errorf("exclusive creation at existing candidate: %v", err)
		}
		if tx.verified || tx.installed {
			t.Error("creation prematurely reports prepared/installed result")
		}
	}
	candidateQABytes(t, target, "original")
	candidateQABytes(t, neighbor, "original")
}

func TestCandidateQACreateErrorsDoNotCreateDirectories(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "обычный файл")
	if err := os.WriteFile(blocker, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	var code string
	for _, dir := range []string{filepath.Join(root, "нет каталога"), filepath.Join(blocker, "вложенный")} {
		tx := &transaction{target: filepath.Join(dir, "result.zip")}
		err := tx.createCandidate()
		candidateQADiagnostic(t, err, "")
		if err.Cause == nil {
			t.Error("creation lost filesystem cause")
		}
		if code != "" && err.Code != code {
			t.Errorf("creation stage code varies by OS cause: %q versus %q", code, err.Code)
		}
		code = err.Code
		if tx.candidate != nil || tx.verified || tx.installed {
			t.Error("failed creation retained descriptor or reported success")
		}
		if _, statErr := os.Stat(dir); statErr == nil {
			t.Errorf("creation made a missing directory: %q", dir)
		}
	}
	candidateQABytes(t, blocker, "untouched")
}

func TestCandidateQAWriteCloseVerifyLifecycle(t *testing.T) {
	target := filepath.Join(t.TempDir(), "новый результат.zip")
	tx := candidateQANew(t, target)
	owned := tx.candidate
	writes, verifies := 0, 0
	if err := tx.writeCandidate(func(dst io.Writer) error {
		writes++
		_, err := io.WriteString(dst, "complete payload\x00\n")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || tx.candidate != owned || tx.verified {
		t.Fatalf("write ownership/state: calls=%d, same file=%v, verified=%v", writes, tx.candidate == owned, tx.verified)
	}
	if _, err := owned.Stat(); err != nil {
		t.Fatalf("write closed transaction's file: %v", err)
	}
	if errs := tx.closeCandidate(); len(errs) != 0 {
		t.Fatalf("closeCandidate: %v", errs)
	}
	if tx.candidate != nil || tx.verified {
		t.Fatal("close did not release ownership or verified before callback")
	}
	if _, err := owned.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Errorf("owned file still open after close: %v", err)
	}
	if errs := tx.closeCandidate(); len(errs) != 0 {
		t.Errorf("second close must not close released descriptor: %v", errs)
	}
	if err := tx.verifyCandidate(func(path string) error {
		verifies++
		if path != tx.candidatePath || !filepath.IsAbs(path) || tx.candidate != nil || tx.verified {
			t.Errorf("verification ran with invalid path/ownership/state: %q", path)
		}
		candidateQABytes(t, path, "complete payload\x00\n")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if verifies != 1 || !tx.verified || tx.installed {
		t.Errorf("verification state: calls=%d verified=%v installed=%v", verifies, tx.verified, tx.installed)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("preparation changed absent target: %v", err)
	}
}

func TestCandidateQACallbackErrorsPreserveExactCause(t *testing.T) {
	for _, stage := range []string{"write", "verify"} {
		t.Run(stage, func(t *testing.T) {
			var stableCode string
			for range 2 {
				target := filepath.Join(t.TempDir(), "исходник.zip")
				if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
					t.Fatal(err)
				}
				tx := candidateQANew(t, target)
				cause := &diagnostic.Error{Code: "format.invalid", Reason: "Ошибка формата", Context: diagnostic.Context{LessonID: "001.02", PageID: "001.02.003", Line: 7, Column: 4}}
				calls := 0
				callback := func(dst io.Writer) error {
					calls++
					if _, err := io.WriteString(dst, "partial"); err != nil {
						t.Fatal(err)
					}
					if stage == "write" {
						return cause
					}
					return nil
				}
				err := tx.writeCandidate(callback)
				if stage == "write" {
					candidateQADiagnostic(t, err, tx.candidatePath)
				} else if err != nil {
					t.Fatal(err)
				}
				owned := tx.candidate
				if errs := tx.closeCandidate(); len(errs) != 0 {
					t.Fatalf("close after %s callback: %v", stage, errs)
				}
				if _, closeErr := owned.Stat(); !errors.Is(closeErr, os.ErrClosed) {
					t.Errorf("descriptor not closed after callback: %v", closeErr)
				}
				if stage == "verify" {
					err = tx.verifyCandidate(func(path string) error { calls++; return cause })
					candidateQADiagnostic(t, err, tx.candidatePath)
				}
				if err.Cause != cause || !errors.Is(err, cause) {
					t.Error("callback error was replaced or lost from Cause chain")
				}
				var nested *diagnostic.Error
				if !errors.As(err.Cause, &nested) || nested != cause {
					t.Error("callback ID and coordinates unavailable through errors.As")
				}
				if stableCode != "" && stableCode != err.Code {
					t.Errorf("unstable %s code: %q versus %q", stage, stableCode, err.Code)
				}
				stableCode = err.Code
				wantCalls := 1
				if stage == "verify" {
					wantCalls = 2
				}
				if calls != wantCalls || tx.verified || tx.installed {
					t.Errorf("callback failure reported success or wrong calls: %d, verified=%v installed=%v", calls, tx.verified, tx.installed)
				}
				candidateQABytes(t, target, "original")
				candidateQABytes(t, tx.candidatePath, "partial")
			}
		})
	}
}

func TestCandidateQACloseReportsSyncAndCloseErrors(t *testing.T) {
	tx := candidateQANew(t, filepath.Join(t.TempDir(), "result.zip"))
	if err := tx.writeCandidate(func(dst io.Writer) error { _, err := io.WriteString(dst, "bytes"); return err }); err != nil {
		t.Fatal(err)
	}
	// Закрытый реальный дескриптор даёт воспроизводимый сбой обоих завершений.
	if err := tx.candidate.Close(); err != nil {
		t.Fatal(err)
	}
	errs := tx.closeCandidate()
	if len(errs) != 2 {
		t.Fatalf("expected Sync and Close errors, got %v", errs)
	}
	for index, err := range errs {
		candidateQADiagnostic(t, err, tx.candidatePath)
		if !errors.Is(err, os.ErrClosed) {
			t.Errorf("completion error lost closed-file cause: %v", err)
		}
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) || pathErr.Op != []string{"sync", "close"}[index] {
			t.Errorf("completion errors must preserve Sync-before-Close order: %v", err)
		}
	}
	if errs[0].Code == errs[1].Code {
		t.Error("Sync and Close must have distinguishable stage codes")
	}
	if tx.candidate != nil || tx.verified || tx.installed {
		t.Error("failed completion retained ownership or reported success")
	}
	if repeated := tx.closeCandidate(); len(repeated) != 0 {
		t.Errorf("completion retried already released file: %v", repeated)
	}
}

func TestCandidateQAVerifyRejectsOpenCandidate(t *testing.T) {
	tx := candidateQANew(t, filepath.Join(t.TempDir(), "result.zip"))
	if err := tx.writeCandidate(func(dst io.Writer) error {
		_, err := io.WriteString(dst, "unfinished lifecycle")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := tx.verifyCandidate(func(string) error { calls++; return nil })
	candidateQADiagnostic(t, err, tx.candidatePath)
	if calls != 0 || tx.verified || tx.installed {
		t.Errorf("open candidate was verified: calls=%d verified=%v installed=%v", calls, tx.verified, tx.installed)
	}
	if errs := tx.closeCandidate(); len(errs) != 0 {
		t.Fatalf("close after rejected premature verification: %v", errs)
	}
}
