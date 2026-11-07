package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func destinationCreate(t *testing.T) (*Operation, string, []byte) {
	t.Helper()
	base := t.TempDir()
	source := filepath.Join(base, "Исходные JSON")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/destination/source.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(source, "source.json")
	if err := os.WriteFile(path, raw, 0444); err != nil {
		t.Fatal(err)
	}
	op, problems := NewCreate(base, `"Исходные JSON"`, "Курс назначения", "")
	if len(problems) != 0 || op == nil {
		t.Fatalf("NewCreate: %v", problems)
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if problems := op.Analyze(at); len(problems) != 0 {
		t.Fatalf("Analyze real JSON: %v", problems)
	}
	return op, path, raw
}

func destinationModuleBytes(t *testing.T, snapshot Snapshot) []byte {
	t.Helper()
	if snapshot.Module == nil {
		t.Fatal("analyzed Create has no module")
	}
	raw, err := json.Marshal(snapshot.Module.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range snapshot.Module.Files {
		lesson, err := json.Marshal(file.Lesson())
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, file.FileName()...)
		raw = append(raw, lesson...)
	}
	return raw
}

func TestDestinationSetPublishesNewReviewAndNeverWrites(t *testing.T) {
	op, source, raw := destinationCreate(t)
	base := filepath.Dir(filepath.Dir(source))
	dir := filepath.Join(base, "Результат с пробелами")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	before := op.Snapshot()
	if problems := op.SetDestination(before.Token, dir, "Курс.ZiP"); len(problems) != 0 {
		t.Fatalf("SetDestination: %v", problems)
	}
	after := op.Snapshot()
	want := Destination{Directory: dir, FileName: "Курс.ZiP", Path: filepath.Join(dir, "Курс.ZiP")}
	if after.Target == nil || *after.Target != want || after.Token == before.Token {
		t.Fatalf("new accepted destination/token were not published: %#v", after.Target)
	}
	if problems := op.SetDestination(before.Token, dir, "stale"); len(problems) != 1 || problems[0].Code != CodeStaleReview {
		t.Fatalf("old accepted token was not rejected: %v", problems)
	}
	preview, problems := op.ReviewSave(after.Token)
	if len(problems) != 0 || preview == nil {
		t.Fatalf("ReviewSave: %v", problems)
	}
	if problems := op.ConfirmSave(preview.Token, true, false); len(problems) != 0 {
		t.Fatalf("ConfirmSave: %v", problems)
	}
	if problems := op.SetDestination(op.Snapshot().Token, "Результат с пробелами", "Другая цель"); len(problems) != 0 {
		t.Fatalf("change destination: %v", problems)
	}
	changed := op.Snapshot()
	if changed.Target == nil || changed.Target.Path != filepath.Join(dir, "Другая цель.zip") || changed.Actions.Save || op.saving != nil {
		t.Fatal("destination change retained final consent or failed to publish new path")
	}
	if problems := op.ConfirmSave(preview.Token, true, false); len(problems) == 0 {
		t.Fatal("old final-save token remained usable after destination change")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("destination selection/review wrote files: %v, %v", entries, err)
	}
	destinationAssertBytes(t, source, raw)
}

func TestDestinationInvalidInputPreservesAcceptedTargetTokenModelAndConsent(t *testing.T) {
	op, source, raw := destinationCreate(t)
	dir := filepath.Dir(filepath.Dir(source))
	path := filepath.Join(dir, "existing.zip")
	destinationWrite(t, path, []byte("original target bytes"))
	if problems := op.SetDestination(op.Snapshot().Token, dir, "existing.zip"); len(problems) != 0 {
		t.Fatalf("initial destination: %v", problems)
	}
	preview, problems := op.ReviewSave(op.Snapshot().Token)
	if len(problems) != 0 || preview == nil {
		t.Fatalf("ReviewSave: %v", problems)
	}
	if problems := op.ConfirmSave(preview.Token, true, true); len(problems) != 0 {
		t.Fatalf("ConfirmSave: %v", problems)
	}
	before := op.Snapshot()
	catalogBefore, err := json.Marshal(before.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	consent, target := op.saving, op.target
	moduleBefore := destinationModuleBytes(t, before)
	for _, tc := range []struct{ directory, name, code string }{
		{dir, "bad/name", CodeArchiveName},
		{dir, "NUL.zip", CodeArchiveName},
		{dir, "bad.zip ", CodeArchiveName},
		{filepath.Join(dir, "absent-directory"), "valid", CodeDestination},
		{source, "valid", CodeDestination},
	} {
		t.Run(tc.name+"/"+tc.directory, func(t *testing.T) {
			problems := op.SetDestination(before.Token, tc.directory, tc.name)
			if len(problems) != 1 || problems[0].Code != tc.code {
				t.Fatalf("invalid input = %v; want %s", problems, tc.code)
			}
			after := op.Snapshot()
			catalogAfter, err := json.Marshal(after.Catalog)
			if err != nil {
				t.Fatal(err)
			}
			if after.Token != before.Token || after.Target == nil || *after.Target != *before.Target ||
				op.target != target || op.saving != consent || !after.Actions.Save ||
				!bytes.Equal(catalogBefore, catalogAfter) || !bytes.Equal(moduleBefore, destinationModuleBytes(t, after)) || len(after.Blockers) != len(before.Blockers) {
				t.Fatal("invalid input changed accepted target/token/catalog/consent/blockers")
			}
		})
	}
	destinationAssertBytes(t, source, raw)
	destinationAssertBytes(t, path, []byte("original target bytes"))
	if _, err := os.Lstat(filepath.Join(dir, "absent-directory")); !os.IsNotExist(err) {
		t.Fatalf("invalid input created destination directory: %v", err)
	}
}

func TestDestinationSetIsCreateOnlyAndRejectsForeignTokens(t *testing.T) {
	dir := t.TempDir()
	var nilOperation *Operation
	for _, op := range []*Operation{nilOperation, {}} {
		if problems := op.SetDestination(ReviewToken{}, dir, "result"); len(problems) != 1 || problems[0].Code != CodeState {
			t.Fatalf("nil/zero receiver: %v", problems)
		}
	}
	for _, mode := range []Mode{Update, Add} {
		var op *Operation
		if mode == Update {
			op, _ = NewUpdate(dir, "existing.zip", "source.json")
		} else {
			op, _ = NewAdd(dir, "existing.zip", "source.json")
		}
		if op == nil {
			t.Fatal("constructor rejected syntactically valid paths before analysis")
		}
		if problems := op.SetDestination(op.Snapshot().Token, dir, "new"); len(problems) != 1 || problems[0].Code != CodeState {
			t.Fatalf("SetDestination in %s: %v", mode, problems)
		}
	}
	op, source, raw := destinationCreate(t)
	foreign, _, _ := destinationCreate(t)
	for _, token := range []ReviewToken{{}, foreign.Snapshot().Token} {
		if problems := op.SetDestination(token, dir, "new"); len(problems) != 1 || problems[0].Code != CodeStaleReview {
			t.Fatalf("foreign/zero token: %v", problems)
		}
	}
	op.Cancel()
	if problems := op.SetDestination(op.Snapshot().Token, dir, "new"); len(problems) != 1 || problems[0].Code != CodeState {
		t.Fatalf("finished operation: %v", problems)
	}
	destinationAssertBytes(t, source, raw)
}
