package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

func TestSaveFreezeChangesOnlyRootTimeAndOwnsContainers(t *testing.T) {
	op, _, _ := savePrefixCreate(t)
	before := savePrefixModel(t, op.Snapshot())
	preview := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
	at := savePrefixTime().Add(48 * time.Hour)
	frozen, issues := op.freezeForWrite(at)
	savePrefixOK(t, issues)
	if frozen == nil || frozen.Manifest.CreatedAt != "2026-11-06T07:11:12Z" {
		t.Fatal("incorrect UTC freeze")
	}
	if !bytes.Equal(before, savePrefixModel(t, op.Snapshot())) {
		t.Fatal("freeze changed accepted state")
	}
	expected := op.Snapshot().Module
	expected.Manifest.CreatedAt = frozen.Manifest.CreatedAt
	if !bytes.Equal(savePrefixModel(t, Snapshot{Module: expected}), savePrefixModel(t, Snapshot{Module: frozen})) {
		t.Fatal("freeze changed content beyond root createdAt")
	}
	if len(frozen.Files) != len(expected.Files) {
		t.Fatal("freeze lost files")
	}
	for index, file := range frozen.Files {
		old := expected.Files[index].Lesson().Document
		actual := file.Lesson().Document
		if actual.CreatedAt != old.CreatedAt || actual.Version != old.Version || actual.LineCount != old.LineCount {
			t.Fatal("freeze revised a lesson")
		}
	}
	frozen.Files[0] = nil
	frozen.Manifest.Course.Title = "tampered"
	frozen.Manifest.Course.Unit.Lessons[0].Title = "tampered"
	if !bytes.Equal(before, savePrefixModel(t, op.Snapshot())) {
		t.Fatal("frozen containers alias accepted state")
	}
	second, issues := op.freezeForWrite(at.Add(time.Hour))
	savePrefixOK(t, issues)
	if second.Files[0] == nil || second.Manifest.Course.Title != "Курс" || second.Manifest.Course.Unit.Lessons[0].Title != "TITLE" {
		t.Fatal("freeze did not own independent containers")
	}
}

func TestSaveFrozenCallbackKeepsModelAndTimestamp(t *testing.T) {
	op, _, _ := savePrefixCreate(t)
	preview := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
	frozen, issues := op.freezeForWrite(savePrefixTime().Add(24 * time.Hour))
	savePrefixOK(t, issues)
	before := savePrefixModel(t, Snapshot{Module: frozen})
	callback := newWriteCallback(frozen, diagnostic.Context{})
	for attempt := 0; attempt < 2; attempt++ {
		var dst bytes.Buffer
		if err := callback(&dst); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, savePrefixModel(t, Snapshot{Module: frozen})) {
			t.Fatal("callback modified frozen module")
		}
		path := filepath.Join(t.TempDir(), "candidate.zip")
		savePrefixWrite(t, path, dst.Bytes())
		actual, issues := moduleio.Read(path)
		savePrefixOK(t, issues)
		if actual.Manifest.CreatedAt != "2026-11-05T07:11:12Z" {
			t.Fatal("callback changed frozen manifest timestamp")
		}
	}
}

func TestSaveWriteFrozenPreservesJoinedOwnerDiagnostics(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	preview := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
	// Негодный module — прямой вход теста private writeFrozen, реальный
	// moduleio и реальная safewrite-транзакция остаются владельцами отказа.
	result, issues := op.writeFrozen(&module.Module{})
	if result.Installed || len(issues) == 0 || result.TemporaryPath != "" {
		t.Fatalf("unexpected rejected candidate result: %+v %v", result, issues)
	}
	foundEmpty := false
	var visit func(error)
	visit = func(err error) {
		if err == nil {
			return
		}
		if issue, ok := err.(*diagnostic.Error); ok {
			if issue.Code == module.CodeEmpty {
				foundEmpty = true
			}
		}
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			for _, child := range joined.Unwrap() {
				visit(child)
			}
		} else {
			visit(errors.Unwrap(err))
		}
	}
	for _, issue := range issues {
		visit(issue)
	}
	if !foundEmpty {
		t.Fatalf("joined module owner diagnostic missing: %v", issues)
	}
	// module.Validate может вернуть несколько независимых ошибок; каждая
	// должна остаться структурированной в callback Cause, а не стать текстом.
	ownerIssues := module.Validate(&module.Module{}, diagnostic.Context{})
	for _, expected := range ownerIssues {
		seen := false
		var search func(error)
		search = func(err error) {
			if err == nil {
				return
			}
			if actual, ok := err.(*diagnostic.Error); ok && actual.Code == expected.Code {
				seen = true
			}
			if joined, ok := err.(interface{ Unwrap() []error }); ok {
				for _, child := range joined.Unwrap() {
					search(child)
				}
			} else {
				search(errors.Unwrap(err))
			}
		}
		for _, issue := range issues {
			search(issue)
		}
		if !seen {
			t.Fatalf("owner code %s lost", expected.Code)
		}
	}
	savePrefixAbsent(t, target)
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "источники" {
		t.Fatal("failed callback left a recovery artifact")
	}
}
