package safewrite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestCleanupRemovesOnlyUnneededOwnedFiles(t *testing.T) {
	for _, state := range []string{"new target", "original unchanged", "original restored", "installed", "rollback failed"} {
		t.Run(state, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "Урок ёж с пробелами.zip")
			candidate := cleanupTestFile(t, dir, "candidate", "новая версия")
			backup := cleanupTestFile(t, dir, "backup", "")
			unrelated := cleanupTestFile(t, dir, "backup чужой", "чужие байты")
			tx := transaction{target: target, candidatePath: candidate, backupPath: backup}
			keepTarget, keepCandidate, keepBackup := false, false, false
			switch state {
			case "original unchanged", "original restored":
				cleanupTestFile(t, dir, filepath.Base(target), "исходная версия")
				tx.original = cleanupTestStat(t, target)
				keepTarget = true
			case "installed", "rollback failed":
				if err := os.WriteFile(backup, []byte("исходная версия"), 0600); err != nil {
					t.Fatal(err)
				}
				tx.original = cleanupTestStat(t, backup)
				tx.originalMoved, tx.verified = true, true
				if state == "installed" {
					if err := os.Rename(candidate, target); err != nil {
						t.Fatal(err)
					}
					tx.candidatePath, tx.installed = "", true
					keepTarget = true
				} else {
					keepCandidate, keepBackup = true, true
				}
			}
			if tx.candidatePath != "" {
				f, err := os.OpenFile(candidate, os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = f.Close() })
				tx.candidate = f
			}
			openCandidate := tx.candidate
			if issues := tx.cleanup(); len(issues) != 0 {
				t.Fatalf("cleanup: %v", issues)
			}
			if openCandidate != nil {
				if _, err := openCandidate.Write([]byte("!")); !errors.Is(err, os.ErrClosed) {
					t.Errorf("дескриптор кандидата не закрыт: %v", err)
				}
			}
			if tx.candidate != nil {
				t.Error("транзакция сохраняет владение закрытым дескриптором")
			}
			cleanupTestContents(t, unrelated, "чужие байты", true)
			cleanupTestContents(t, candidate, "новая версия", keepCandidate)
			cleanupTestContents(t, backup, "исходная версия", keepBackup)
			wantTarget := "исходная версия"
			if state == "installed" {
				wantTarget = "новая версия"
			}
			cleanupTestContents(t, target, wantTarget, keepTarget)
			if !keepCandidate && tx.candidatePath != "" {
				t.Error("после удаления сохранён путь кандидата")
			}
			if !keepBackup && tx.backupPath != "" {
				t.Error("после удаления сохранён путь резервы")
			}
			if keepBackup && (tx.backupPath != backup || tx.candidatePath != candidate || !tx.originalMoved) {
				t.Error("после неудачного отката потеряны сведения о сохранённых версиях")
			}
			if state == "installed" && !tx.installed {
				t.Error("очистка отменила факт установки")
			}
			if issues := tx.cleanup(); len(issues) != 0 {
				t.Errorf("повторная очистка: %v", issues)
			}
		})
	}
}

func TestCleanupContinuesAfterFilesystemRemovalErrors(t *testing.T) {
	for _, blocked := range []string{"candidate", "backup", "both"} {
		t.Run(blocked, func(t *testing.T) {
			dir := t.TempDir()
			candidate := cleanupTestFile(t, dir, "candidate", "частичный результат")
			backup := cleanupTestFile(t, dir, "backup", "")
			target := cleanupTestFile(t, dir, "цель.zip", "исходник")
			tx := transaction{target: target, original: cleanupTestStat(t, target), candidatePath: candidate, backupPath: backup}
			want := map[string]bool{}
			for _, item := range []struct{ name, path string }{{"candidate", candidate}, {"backup", backup}} {
				if blocked == item.name || blocked == "both" {
					// Непустой каталог даёт реальную ошибку Remove даже при запуске от root.
					if err := os.Remove(item.path); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(item.path, 0700); err != nil {
						t.Fatal(err)
					}
					cleanupTestFile(t, item.path, "не удалять", "препятствие")
					want[item.path] = true
				}
			}
			issues := tx.cleanup()
			if len(issues) != len(want) {
				t.Fatalf("ошибок %d, ожидается %d: %v", len(issues), len(want), issues)
			}
			for _, issue := range issues {
				if issue == nil || !want[issue.Context.Path] {
					t.Fatalf("неточная или повторная ошибка оставшегося пути: %#v", issue)
				}
				delete(want, issue.Context.Path)
				if !strings.HasPrefix(issue.Code, "safewrite.") || !strings.Contains(strings.ToLower(issue.Code), "cleanup") || !cleanupTestRussian(issue.Reason) {
					t.Errorf("нет кода этапа очистки или русской причины: %#v", issue)
				}
				var pathErr *os.PathError
				if !errors.As(issue, &pathErr) || pathErr.Path != issue.Context.Path || pathErr.Op != "remove" {
					t.Errorf("не сохранена техническая ошибка удаления: %v", issue)
				}
				if tx.candidatePath != issue.Context.Path && tx.backupPath != issue.Context.Path {
					t.Error("оставшийся файл потерян в состоянии транзакции")
				}
			}
			cleanupTestContents(t, target, "исходник", true)
			if blocked == "candidate" {
				cleanupTestContents(t, backup, "", false)
			}
			if blocked == "backup" {
				cleanupTestContents(t, candidate, "", false)
			}
		})
	}
}

func TestFailurePreservesCallbackDiagnosticAndRecoveryState(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Результат ёж.zip")
	backup := cleanupTestFile(t, dir, "исходник", "исходная версия")
	candidate := cleanupTestFile(t, dir, "кандидат", "новая версия")
	_, readErr := os.Open(filepath.Join(dir, "отсутствующий исходник.json"))
	if !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("не получена реальная ошибка чтения: %v", readErr)
	}
	inner := &diagnostic.Error{
		Code: "moduleio.verify", Reason: "Не согласован состав модуля",
		Context: diagnostic.Context{Path: candidate, FileName: "l001.json", LessonID: "000ЁЖ", PageID: "000ЁЖ.P01", Line: 35, Column: 7},
		Related: []diagnostic.Context{{Path: "исходный JSON.json", LessonID: "000ЁЖ", Line: 4}},
		Details: []diagnostic.Detail{{Label: "Исходное значение", Value: " 00.12 ёж "}},
		Cause:   readErr,
	}
	wantContext, wantRelated, wantDetails := inner.Context, slices.Clone(inner.Related), slices.Clone(inner.Details)
	cause := fmt.Errorf("проверка callback: %w", inner)
	tx := transaction{target: target, original: cleanupTestStat(t, backup), backupPath: backup, originalMoved: true, candidatePath: candidate, verified: true}
	issue := tx.failure("safewrite.verify", "Не удалось проверить результат", candidate, cause)
	if issue == nil {
		t.Fatal("failure вернул nil")
	}
	if issue.Code != "safewrite.verify" || issue.Reason != "Не удалось проверить результат" || issue.Context.Path != candidate || issue.Cause != cause {
		t.Fatalf("потеряны поля ошибки этапа: %#v", issue)
	}
	if !errors.Is(issue, inner) || !errors.Is(issue, os.ErrNotExist) {
		t.Error("Cause потерял исходную callback-диагностику или её техническую причину")
	}
	var nested *diagnostic.Error
	var pathErr *os.PathError
	if !errors.As(errors.Unwrap(issue), &nested) || nested != inner || !errors.As(issue, &pathErr) {
		t.Fatal("errors.As потерял исходные типы ошибок")
	}
	if nested.Context != wantContext || !slices.Equal(nested.Related, wantRelated) || !slices.Equal(nested.Details, wantDetails) {
		t.Error("утрачены ID, координаты, связанные места или подробности callback")
	}
	for _, path := range []string{backup, candidate} {
		if !slices.ContainsFunc(issue.Related, func(c diagnostic.Context) bool { return c.Path == path }) {
			t.Errorf("Related не сообщает путь сохранённой версии %q", path)
		}
		if !slices.ContainsFunc(issue.Details, func(d diagnostic.Detail) bool { return strings.Contains(d.Value, path) }) {
			t.Errorf("Details не поясняет место версии %q", path)
		}
	}
	flag := cleanupTestInstalledDetail(t, issue)
	tx.installed, tx.candidatePath = true, ""
	installed := tx.failure("safewrite.cleanup", "Не удалось удалить резерву", backup, nil)
	if installed.Cause != nil || cleanupTestInstalledDetail(t, installed) == flag {
		t.Error("Details не различает установленный и неустановленный результат")
	}
	cleanupTestContents(t, backup, "исходная версия", true)
	cleanupTestContents(t, candidate, "новая версия", true)
}

func cleanupTestFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func cleanupTestStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func cleanupTestContents(t *testing.T, path, want string, exists bool) {
	t.Helper()
	got, err := os.ReadFile(path)
	if !exists {
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("ненужный файл %q не удалён: %v", path, err)
		}
	} else if err != nil || string(got) != want {
		t.Errorf("содержимое %q = %q, %v; ожидается %q", path, got, err, want)
	}
}

func cleanupTestRussian(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) })
}

func cleanupTestInstalledDetail(t *testing.T, issue *diagnostic.Error) string {
	t.Helper()
	for _, detail := range issue.Details {
		if strings.Contains(strings.ToLower(detail.Label), "установ") && detail.Value != "" {
			return detail.Value
		}
	}
	t.Errorf("Details не поясняет, установлен ли результат: %#v", issue.Details)
	return ""
}
