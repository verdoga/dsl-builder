package safewrite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResultContractKnownFileLocations(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Модуль 日本語 с пробелом.zip")
	candidate := filepath.Join(dir, ".кандидат.tmp")
	backup := filepath.Join(dir, ".исходник.bak")
	if err := os.WriteFile(target, []byte("исходный архив"), 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	// Снимок хранит последние известные места, а не повторяет Stat:
	// внешнее удаление не должно менять уже известное состояние транзакции.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		tx   transaction
		want Result
	}{
		{
			name: "new target before preparation",
			tx:   transaction{target: target},
			want: Result{TargetPath: target},
		},
		{
			name: "existing target retained after early failure",
			tx:   transaction{target: target, original: original},
			want: Result{TargetPath: target, OriginalPath: target},
		},
		{
			name: "partial candidate without original",
			tx:   transaction{target: target, candidatePath: candidate},
			want: Result{TargetPath: target, TemporaryPath: candidate},
		},
		{
			name: "partial candidate with retained original",
			tx:   transaction{target: target, original: original, candidatePath: candidate},
			want: Result{TargetPath: target, OriginalPath: target, TemporaryPath: candidate},
		},
		{
			name: "verified candidate is not installed",
			tx:   transaction{target: target, candidatePath: candidate, verified: true},
			want: Result{TargetPath: target, TemporaryPath: candidate},
		},
		{
			name: "reserved placeholder is not original",
			tx:   transaction{target: target, original: original, candidatePath: candidate, backupPath: backup, verified: true},
			want: Result{TargetPath: target, OriginalPath: target, TemporaryPath: candidate},
		},
		{
			name: "original moved before install or after failed rollback",
			tx:   transaction{target: target, original: original, candidatePath: candidate, backupPath: backup, originalMoved: true, verified: true},
			want: Result{TargetPath: target, OriginalPath: backup, TemporaryPath: candidate},
		},
		{
			name: "restored original and remaining candidate",
			tx:   transaction{target: target, original: original, candidatePath: candidate, backupPath: backup, verified: true},
			want: Result{TargetPath: target, OriginalPath: target, TemporaryPath: candidate},
		},
		{
			name: "restored original after candidate cleanup",
			tx:   transaction{target: target, original: original, verified: true},
			want: Result{TargetPath: target, OriginalPath: target},
		},
		{
			name: "failed new write after cleanup",
			tx:   transaction{target: target},
			want: Result{TargetPath: target},
		},
		{
			name: "new result installed",
			tx:   transaction{target: target, installed: true, verified: true},
			want: Result{TargetPath: target, Installed: true},
		},
		{
			name: "installed replacement with backup cleanup failure",
			tx:   transaction{target: target, original: original, backupPath: backup, originalMoved: true, installed: true, verified: true},
			want: Result{TargetPath: target, Installed: true, OriginalPath: backup},
		},
		{
			name: "replacement installed and backup removed",
			tx:   transaction{target: target, original: original, installed: true, verified: true},
			want: Result{TargetPath: target, Installed: true},
		},
		{
			name: "exact Windows path spelling",
			tx:   transaction{target: `C:\Модули и уроки\日本語.zip`, original: original, candidatePath: `C:\Модули и уроки\.Новый файл.tmp`},
			want: Result{TargetPath: `C:\Модули и уроки\日本語.zip`, OriginalPath: `C:\Модули и уроки\日本語.zip`, TemporaryPath: `C:\Модули и уроки\.Новый файл.tmp`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.tx
			if got := tc.tx.result(); got != tc.want {
				t.Errorf("result() = %+v, want %+v", got, tc.want)
			}
			if tc.tx != before {
				t.Error("result() changed transaction state")
			}
			if again := tc.tx.result(); again != tc.want {
				t.Errorf("repeated result() = %+v, want %+v", again, tc.want)
			}
		})
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("result() created files: %v", entries)
	}
}

func TestResultContractSnapshotDoesNotFollowTransactionChanges(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Цель.zip")
	candidate := filepath.Join(dir, ".частичный.tmp")
	tx := transaction{target: target, candidatePath: candidate}
	snapshot := tx.result()
	want := Result{TargetPath: target, TemporaryPath: candidate}
	if snapshot != want {
		t.Fatalf("initial snapshot = %+v, want %+v", snapshot, want)
	}

	tx.target = filepath.Join(dir, "Другая цель.zip")
	tx.candidatePath = ""
	tx.verified = true
	tx.installed = true
	if snapshot != want {
		t.Errorf("saved snapshot changed to %+v, want %+v", snapshot, want)
	}
	wantCurrent := Result{TargetPath: tx.target, Installed: true}
	if got := tx.result(); got != wantCurrent {
		t.Errorf("current snapshot = %+v, want %+v", got, wantCurrent)
	}

	snapshot.TargetPath = "изменённая копия"
	snapshot.OriginalPath = "чужой исходник"
	snapshot.TemporaryPath = "чужой кандидат"
	snapshot.Installed = false
	if got := tx.result(); got != wantCurrent {
		t.Errorf("mutating Result changed transaction snapshot to %+v", got)
	}
}
