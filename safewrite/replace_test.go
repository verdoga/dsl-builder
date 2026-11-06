package safewrite

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func replaceTestWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
}

func replaceTestBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("ReadFile(%q) = %q, %v; want %q", path, got, err, want)
	}
}

func replaceTestAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(%q) = %v; want absence", path, err)
	}
}

func replaceTestTransaction(t *testing.T, existing bool) (*transaction, []byte, []byte) {
	t.Helper()
	dir := t.TempDir()
	tx := &transaction{target: filepath.Join(dir, "Архив с пробелом 🐈.zip"), candidatePath: filepath.Join(dir, "кандидат.tmp"), verified: true}
	original, candidate := []byte{0, 255, 13, 10, 1, 128}, []byte("новая версия\x00\xff")
	if existing {
		replaceTestWrite(t, tx.target, original)
		var err error
		tx.original, err = os.Stat(tx.target)
		if err != nil {
			t.Fatal(err)
		}
	}
	replaceTestWrite(t, tx.candidatePath, candidate)
	return tx, original, candidate
}

func replaceTestError(t *testing.T, err *diagnostic.Error) {
	t.Helper()
	if err == nil {
		t.Fatal("want diagnostic error")
	}
	if !strings.HasPrefix(err.Code, "safewrite.") || !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Fatalf("missing stage code or Russian reason: %#v", err)
	}
}

func replaceTestReportedPath(t *testing.T, err *diagnostic.Error, path string) {
	t.Helper()
	if err.Context.Path == path {
		return
	}
	for _, related := range err.Related {
		if related.Path == path {
			return
		}
	}
	for _, detail := range err.Details {
		if detail.Value == path {
			return
		}
	}
	t.Fatalf("exact path %q absent from diagnostic: %#v", path, err)
}

func TestReplaceReserveBackupUniqueNeighborAndClosed(t *testing.T) {
	tx, original, candidate := replaceTestTransaction(t, true)
	other := filepath.Join(filepath.Dir(tx.target), "посторонняя резерва.bak")
	replaceTestWrite(t, other, []byte("чужой файл"))
	second := &transaction{target: tx.target}
	for _, current := range []*transaction{tx, second} {
		if err := current.reserveBackup(); err != nil {
			t.Fatal(err)
		}
		path := current.backupPath
		if path == "" || !filepath.IsAbs(path) || filepath.Dir(path) != filepath.Dir(tx.target) || path == tx.target || path == tx.candidatePath || path == other {
			t.Fatalf("invalid backup path %q", path)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("backup placeholder: %v, %v", info, err)
		}
		// Linux позволяет непосредственно увидеть утечку открытого дескриптора;
		// удаление placeholder дополнительно проверяет закрытие в Windows.
		if runtime.GOOS == "linux" {
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if name, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name())); err == nil && name == path {
					t.Fatalf("backup descriptor still open: %s", path)
				}
			}
		}
	}
	if tx.backupPath == second.backupPath {
		t.Fatal("backup names are not unique")
	}
	for _, path := range []string{tx.backupPath, second.backupPath} {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove closed placeholder: %v", err)
		}
	}
	replaceTestBytes(t, tx.target, original)
	replaceTestBytes(t, tx.candidatePath, candidate)
	replaceTestBytes(t, other, []byte("чужой файл"))
}

func TestReplacePreserveRequiresVerifiedAndUnchangedTarget(t *testing.T) {
	for _, scenario := range []string{"unverified", "changed target"} {
		t.Run(scenario, func(t *testing.T) {
			tx, original, candidate := replaceTestTransaction(t, true)
			if err := tx.reserveBackup(); err != nil {
				t.Fatal(err)
			}
			if scenario == "unverified" {
				tx.verified = false
			} else {
				original = []byte("постороннее изменение другого размера")
				replaceTestWrite(t, tx.target, original)
			}
			replaceTestError(t, tx.preserveOriginal())
			if tx.originalMoved || tx.installed {
				t.Fatal("failed preservation advanced state")
			}
			replaceTestBytes(t, tx.target, original)
			replaceTestBytes(t, tx.candidatePath, candidate)
		})
	}
}

func TestReplaceInstallNewAndExisting(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			tx, original, candidate := replaceTestTransaction(t, existing)
			if existing {
				if err := tx.reserveBackup(); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.preserveOriginal(); err != nil {
				t.Fatal(err)
			}
			if existing {
				if !tx.originalMoved {
					t.Fatal("originalMoved is false after preservation")
				}
				replaceTestBytes(t, tx.backupPath, original)
				preserved, err := os.Stat(tx.backupPath)
				if err != nil || !os.SameFile(tx.original, preserved) {
					t.Fatalf("original was not moved intact: %v", err)
				}
			}
			replaceTestAbsent(t, tx.target)
			candidatePath := tx.candidatePath
			if err := tx.installCandidate(); err != nil {
				t.Fatal(err)
			}
			if !tx.installed {
				t.Fatal("successful installation did not set installed")
			}
			replaceTestBytes(t, tx.target, candidate)
			replaceTestAbsent(t, candidatePath)
			if existing {
				replaceTestBytes(t, tx.backupPath, original)
			}
		})
	}
}

func TestReplaceInstallRejectsUnsafeStates(t *testing.T) {
	for _, scenario := range []string{"unverified", "unsaved original", "unexpected target"} {
		t.Run(scenario, func(t *testing.T) {
			tx, original, candidate := replaceTestTransaction(t, scenario == "unsaved original")
			if scenario == "unverified" {
				tx.verified = false
			}
			if scenario == "unexpected target" {
				original = []byte("посторонняя цель")
				replaceTestWrite(t, tx.target, original)
			}
			replaceTestError(t, tx.installCandidate())
			if tx.installed {
				t.Fatal("failed installation set installed")
			}
			replaceTestBytes(t, tx.candidatePath, candidate)
			if scenario == "unverified" {
				replaceTestAbsent(t, tx.target)
			} else {
				replaceTestBytes(t, tx.target, original)
			}
		})
	}
}

func TestReplaceActualInstallFailureAndRollback(t *testing.T) {
	tx, original, _ := replaceTestTransaction(t, true)
	if err := tx.reserveBackup(); err != nil {
		t.Fatal(err)
	}
	if err := tx.preserveOriginal(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tx.candidatePath); err != nil {
		t.Fatal(err)
	}
	err := tx.installCandidate()
	replaceTestError(t, err)
	if !errors.Is(err, os.ErrNotExist) || tx.installed {
		t.Fatalf("missing candidate must cause real FS failure, installed=false: %v", err)
	}
	replaceTestBytes(t, tx.backupPath, original)
	if err := tx.restoreOriginal(); err != nil {
		t.Fatal(err)
	}
	if tx.originalMoved || tx.installed {
		t.Fatal("rollback state is incorrect")
	}
	replaceTestBytes(t, tx.target, original)
	restored, statErr := os.Stat(tx.target)
	if statErr != nil || !os.SameFile(tx.original, restored) {
		t.Fatalf("rollback did not move original intact: %v", statErr)
	}
}

func TestReplaceFailedRollbackPreservesBothVersionsAndForeignTarget(t *testing.T) {
	tx, original, candidate := replaceTestTransaction(t, true)
	if err := tx.reserveBackup(); err != nil {
		t.Fatal(err)
	}
	if err := tx.preserveOriginal(); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("появившийся посторонний файл")
	replaceTestWrite(t, tx.target, foreign)
	replaceTestError(t, tx.installCandidate())
	err := tx.restoreOriginal()
	replaceTestError(t, err)
	replaceTestReportedPath(t, err, tx.backupPath)
	replaceTestReportedPath(t, err, tx.candidatePath)
	if !tx.originalMoved || tx.installed {
		t.Fatal("failed rollback lost original location or marked installed")
	}
	replaceTestBytes(t, tx.target, foreign)
	replaceTestBytes(t, tx.backupPath, original)
	replaceTestBytes(t, tx.candidatePath, candidate)
}

func TestReplaceRestoreDoesNotCreateOriginalForNewTarget(t *testing.T) {
	tx, _, candidate := replaceTestTransaction(t, false)
	tx.verified = false
	if err := tx.restoreOriginal(); err != nil {
		t.Fatal(err)
	}
	replaceTestAbsent(t, tx.target)
	replaceTestBytes(t, tx.candidatePath, candidate)
	if tx.installed || tx.originalMoved {
		t.Fatal("restore advanced absent original or unverified candidate")
	}
}
