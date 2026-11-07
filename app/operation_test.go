package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOperationConstructorsResolveExplicitQuotedPathsWithoutReading(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join("Исходники с пробелами", "..", "Уроки")
	archive := filepath.Join("Модули", "Курс.zip")
	jsonPath := filepath.Join("Уроки", "Тема.JSON")
	cases := []struct {
		name string
		mode Mode
		new  func() *Operation
	}{
		{"create", Create, func() *Operation {
			op, issues := NewCreate(base, `"`+source+`"`, "  Курс  ", "Юнит")
			operationNoIssues(t, issues)
			return op
		}},
		{"update", Update, func() *Operation {
			op, issues := NewUpdate(base, `"`+archive+`"`, `"`+jsonPath+`"`)
			operationNoIssues(t, issues)
			return op
		}},
		{"add", Add, func() *Operation {
			op, issues := NewAdd(base, `"`+archive+`"`, `"`+source+`"`)
			operationNoIssues(t, issues)
			return op
		}},
	}
	var previous *reviewKey
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := tc.new()
			if op == nil {
				t.Fatal("constructor returned nil")
			}
			wantSource := filepath.Join(base, source)
			if tc.mode == Update {
				wantSource = filepath.Join(base, jsonPath)
			}
			if op.mode != tc.mode || op.baseDir != base || op.sourcePath != wantSource {
				t.Fatalf("mode/base/source = %q %q %q", op.mode, op.baseDir, op.sourcePath)
			}
			wantArchive := ""
			if tc.mode != Create {
				wantArchive = filepath.Join(base, archive)
			}
			if op.archivePath != wantArchive {
				t.Fatalf("archive = %q, want %q", op.archivePath, wantArchive)
			}
			if tc.mode == Create && (op.courseTitle != "  Курс  " || op.unitTitle != "Юнит") {
				t.Fatal("constructor changed original titles before catalog validation")
			}
			if op.key == nil || op.key.owner != op || op.key == previous {
				t.Fatal("constructor did not create an independent owner-bound key")
			}
			previous = op.key
			if op.analyzed || op.finished || op.current != nil || op.baseline != nil {
				t.Fatal("constructor performed analysis")
			}
		})
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 0 {
		t.Fatalf("constructors wrote to filesystem: %v, %v", entries, err)
	}
}

func TestOperationConstructorsRejectInvalidPathsAtomically(t *testing.T) {
	base := t.TempDir()
	for _, bad := range []string{"", `""`, "bad\x00path"} {
		t.Run("source/"+bad, func(t *testing.T) {
			op, issues := NewCreate(base, bad, "Курс", "")
			if op != nil || len(issues) == 0 {
				t.Fatal("invalid create path returned an operation")
			}
			op, issues = NewUpdate(base, "archive.zip", bad)
			if op != nil || len(issues) == 0 {
				t.Fatal("invalid update source returned a partial operation")
			}
			op, issues = NewAdd(base, "archive.zip", bad)
			if op != nil || len(issues) == 0 {
				t.Fatal("invalid add source returned a partial operation")
			}
		})
		t.Run("archive/"+bad, func(t *testing.T) {
			op, issues := NewUpdate(base, bad, "source.json")
			if op != nil || len(issues) == 0 {
				t.Fatal("invalid update archive returned a partial operation")
			}
			op, issues = NewAdd(base, bad, "source.json")
			if op != nil || len(issues) == 0 {
				t.Fatal("invalid add archive returned a partial operation")
			}
		})
	}
	for _, badBase := range []string{"", "relative-base", "bad\x00base"} {
		op, issues := NewCreate(badBase, "source", "Курс", "")
		if op != nil || len(issues) == 0 {
			t.Errorf("relative source accepted invalid explicit base %q", badBase)
		}
	}
	op, issues := NewCreate(base, `"`+filepath.Join(base, "absolute source")+`"`, "", "")
	operationNoIssues(t, issues)
	if op == nil || op.sourcePath != filepath.Join(base, "absolute source") {
		t.Fatal("absolute quoted input was not retained")
	}
}
