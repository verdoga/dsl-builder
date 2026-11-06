package safewrite

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func transactionTestFile(t *testing.T, path, content string) os.FileInfo {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func transactionTestError(t *testing.T, err *diagnostic.Error, path string) {
	t.Helper()
	if err == nil {
		t.Fatal("ожидалась диагностическая ошибка")
	}
	if err.Code == "" || err.Reason == "" || err.Context.Path != path {
		t.Fatalf("неполная диагностика: %#v", err)
	}
}

func TestTransactionInspectTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Модуль с пробелами.zip")
	for _, replace := range []bool{false, true} {
		info, err := inspectTarget(path, replace)
		if info != nil || err != nil {
			t.Fatalf("отсутствующая цель, replace=%v: %v, %v", replace, info, err)
		}
	}
	original := transactionTestFile(t, path, "исходные байты")
	info, err := inspectTarget(path, true)
	if err != nil || info == nil {
		t.Fatalf("обычная цель: %v, %v", info, err)
	}
	if !os.SameFile(original, info) || info.Size() != original.Size() || !info.ModTime().Equal(original.ModTime()) {
		t.Fatal("сведения цели не соответствуют исходнику")
	}
	_, err = inspectTarget(path, false)
	transactionTestError(t, err, path)
	bytes, readErr := os.ReadFile(path)
	if readErr != nil || string(bytes) != "исходные байты" {
		t.Fatalf("исходник изменён: %q, %v", bytes, readErr)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil || len(entries) != 1 {
		t.Fatalf("inspectTarget создал служебные файлы: %v, %v", entries, readErr)
	}
}

func TestTransactionInspectRetainsOriginalWhenOverwriteRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Исходный модуль.zip")
	const content = "исходные байты\x00без потери"
	original := transactionTestFile(t, path, content)
	info, err := inspectTarget(path, false)
	transactionTestError(t, err, path)
	if info == nil {
		t.Error("при отказе перезаписи потеряны известные сведения об исходнике")
	} else {
		if !os.SameFile(original, info) || !info.Mode().IsRegular() || info.Size() != original.Size() || !info.ModTime().Equal(original.ModTime()) {
			t.Error("отказ перезаписи вернул сведения другого или изменённого файла")
		}
		tx := transaction{target: path, original: info}
		if got, want := tx.result(), (Result{TargetPath: path, OriginalPath: path}); got != want {
			t.Errorf("известное состояние после отказа=%#v, требуется %#v", got, want)
		}
	}
	after, statErr := os.Lstat(path)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if !os.SameFile(original, after) || after.Mode() != original.Mode() || after.Size() != original.Size() || !after.ModTime().Equal(original.ModTime()) {
		t.Error("отказ перезаписи изменил исходный файл")
	}
	bytes, readErr := os.ReadFile(path)
	if readErr != nil || string(bytes) != content {
		t.Errorf("исходные байты изменены: %q, %v", bytes, readErr)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Errorf("отказ перезаписи оставил служебные файлы: %v, %v", entries, readErr)
	}
}

func TestTransactionInspectRejectsInvalidTargets(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "file")
	transactionTestFile(t, regular, "original")
	for _, path := range []string{"", "relative.zip", filepath.Join("relative", "file.zip"), filepath.Join(dir, "bad\x00.zip"), dir, filepath.Join(regular, "child.zip")} {
		t.Run(path, func(t *testing.T) {
			for _, replace := range []bool{false, true} {
				_, err := inspectTarget(path, replace)
				transactionTestError(t, err, path)
			}
		})
	}
	for _, destination := range []string{regular, filepath.Join(dir, "missing")} {
		link := filepath.Join(dir, "link")
		if err := os.Symlink(destination, link); err != nil {
			t.Skipf("создание symlink недоступно: %v", err)
		}
		_, err := inspectTarget(link, true)
		transactionTestError(t, err, link)
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTransactionCheckTargetDetectsChanges(t *testing.T) {
	for _, change := range []string{"unchanged", "disappeared", "identity", "size", "mtime", "directory", "symlink"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "цель.zip")
			original := transactionTestFile(t, path, "old")
			tx := transaction{target: path, original: original}
			switch change {
			case "disappeared", "directory", "symlink":
				other := filepath.Join(dir, "other")
				if err := os.Rename(path, other); err != nil {
					t.Fatal(err)
				}
				if change == "directory" {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
				if change == "symlink" {
					if err := os.Symlink(other, path); err != nil {
						t.Skipf("symlink недоступен: %v", err)
					}
				}
			case "identity":
				// Сохраняем старый inode, чтобы новый файл не получил его повторно.
				if err := os.Rename(path, filepath.Join(dir, "old")); err != nil {
					t.Fatal(err)
				}
				transactionTestFile(t, path, "new")
				if err := os.Chtimes(path, original.ModTime(), original.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "size":
				transactionTestFile(t, path, "longer content")
				if err := os.Chtimes(path, original.ModTime(), original.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "mtime":
				later := original.ModTime().Add(2 * time.Hour)
				if err := os.Chtimes(path, later, later); err != nil {
					t.Fatal(err)
				}
			}
			err := tx.checkTarget()
			if change == "unchanged" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				transactionTestError(t, err, path)
			}
			if tx.target != path || tx.original != original || tx.installed || tx.originalMoved {
				t.Fatal("проверка изменила состояние транзакции")
			}
		})
	}
	for _, appeared := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "new.zip")
		tx := transaction{target: path}
		if appeared {
			transactionTestFile(t, path, "foreign")
			transactionTestError(t, tx.checkTarget(), path)
			bytes, err := os.ReadFile(path)
			if err != nil || string(bytes) != "foreign" {
				t.Fatalf("посторонний файл изменён: %q, %v", bytes, err)
			}
		} else if err := tx.checkTarget(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTransactionTargetAccessErrors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "closed")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "target")
	original := transactionTestFile(t, path, "old")
	if err := os.Chmod(dir, 0); err != nil {
		t.Skipf("изменение доступа недоступно: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrPermission) {
		t.Skip("среда не воспроизводит запрет доступа к каталогу (например root)")
	}
	_, err := inspectTarget(path, true)
	transactionTestError(t, err, path)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("потеряна техническая причина: %v", err)
	}
	for _, info := range []os.FileInfo{nil, original} {
		tx := transaction{target: path, original: info}
		err := tx.checkTarget()
		transactionTestError(t, err, path)
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("ошибка доступа принята за отсутствие: %v", err)
		}
	}
}

func TestTransactionResultUsesLastKnownState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	original := transactionTestFile(t, path, "original")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                   string
		original               os.FileInfo
		candidate, backup      string
		moved, installed       bool
		wantOriginal, wantTemp string
	}{
		{"new", nil, "", "", false, false, "", ""},
		{"new candidate", nil, "candidate", "", false, false, "", "candidate"},
		{"original untouched", original, "candidate", "placeholder", false, false, path, "candidate"},
		{"original moved", original, "candidate", "backup", true, false, "backup", "candidate"},
		{"restored", original, "candidate", "placeholder", false, false, path, "candidate"},
		{"installed backup remains", original, "", "backup", true, true, "backup", ""},
		{"installed cleaned", original, "", "", false, true, "", ""},
		{"new installed", nil, "", "", false, true, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate, backup, wantOriginal, wantTemp := tc.candidate, tc.backup, tc.wantOriginal, tc.wantTemp
			if candidate != "" {
				candidate = filepath.Join(dir, candidate)
			}
			if backup != "" {
				backup = filepath.Join(dir, backup)
			}
			if wantOriginal != "" && wantOriginal != path {
				wantOriginal = filepath.Join(dir, wantOriginal)
			}
			if wantTemp != "" {
				wantTemp = filepath.Join(dir, wantTemp)
			}
			tx := transaction{target: path, original: tc.original, candidatePath: candidate, backupPath: backup, originalMoved: tc.moved, installed: tc.installed}
			before := tx
			want := Result{TargetPath: path, Installed: tc.installed, OriginalPath: wantOriginal, TemporaryPath: wantTemp}
			for i := 0; i < 2; i++ {
				if got := tx.result(); got != want {
					t.Fatalf("result=%#v, требуется %#v", got, want)
				}
			}
			if tx != before {
				t.Fatal("result изменил состояние транзакции")
			}
		})
	}
}
