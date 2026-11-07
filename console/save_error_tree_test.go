package console

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func consoleSaveTestWalkErrors(err error, visit func(error)) {
	if err == nil {
		return
	}
	if issue, ok := err.(*diagnostic.Error); ok && issue == nil {
		return
	}
	visit(err)
	switch tree := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range tree.Unwrap() {
			consoleSaveTestWalkErrors(child, visit)
		}
	case interface{ Unwrap() error }:
		consoleSaveTestWalkErrors(tree.Unwrap(), visit)
	}
}

func consoleSaveTestLeafCounts(err error) map[string]int {
	counts := make(map[string]int)
	consoleSaveTestWalkErrors(err, func(node error) {
		switch tree := node.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range tree.Unwrap() {
				if child != nil {
					return
				}
			}
		case interface{ Unwrap() error }:
			if tree.Unwrap() != nil {
				return
			}
		}
		counts[node.Error()]++
	})
	return counts
}

func consoleSaveTestPresentationCauses(t *testing.T, err error, causes ...error) {
	t.Helper()
	want := make(map[string]int)
	for _, cause := range causes {
		if cause == nil {
			t.Fatal("fixture did not break an independent preview block")
		}
		for text, count := range consoleSaveTestLeafCounts(cause) {
			want[text] += count
		}
	}
	got := consoleSaveTestLeafCounts(err)
	if len(got) != len(want) {
		t.Fatalf("cause multiset=%v; want %v", got, want)
	}
	for text, count := range want {
		if got[text] != count {
			t.Errorf("leaf %q occurs %d times; want %d", text, got[text], count)
		}
	}
	wrappers := 0
	consoleSaveTestWalkErrors(err, func(node error) {
		if issue, ok := node.(*diagnostic.Error); ok {
			if issue.Code != CodePreview {
				t.Errorf("unexpected diagnostic wrapper: %s", issue.Code)
			}
			wrappers++
		}
	})
	if wrappers == 0 {
		t.Error("missing presentation wrapper")
	}
}

func consoleSaveTestSnapshotUnchanged(t *testing.T, before, after app.Snapshot) {
	t.Helper()
	encode := func(value app.Snapshot) []byte {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		// File скрывает модель и исходные байты; проверяем их публичные копии.
		if value.Module != nil {
			for _, file := range value.Module.Files {
				lesson, err := json.Marshal(file.Lesson())
				if err != nil {
					t.Fatal(err)
				}
				raw = append(raw, lesson...)
				raw = append(raw, file.OriginalBytes()...)
			}
		}
		return raw
	}
	if before.Token != after.Token || !bytes.Equal(encode(before), encode(after)) {
		t.Error("review changed owner snapshot, decisions, target, files or consent")
	}
}

func TestConsoleSaveErrorOracleTraversesBothUnwrapFormsWithoutDeduplication(t *testing.T) {
	leaf := errors.New("repeated leaf")
	stream := errors.New("original stream failure")
	blocker := &diagnostic.Error{Code: "source.blocker", Reason: "Исходная блокировка", Cause: leaf}
	err := errors.Join(fmt.Errorf("context: %w", blocker), leaf,
		&diagnostic.Error{Code: CodeOutput, Reason: "Ошибка вывода", Cause: stream})
	counts := consoleSaveTestLeafCounts(err)
	if len(counts) != 2 || counts[leaf.Error()] != 2 || counts[stream.Error()] != 1 {
		t.Fatalf("leaf multiset lost branches or duplicates: %v", counts)
	}
	wrappers, sourceIdentity, streamIdentity := 0, 0, 0
	consoleSaveTestWalkErrors(err, func(node error) {
		if _, ok := node.(*diagnostic.Error); ok {
			wrappers++
		}
		if node == blocker {
			sourceIdentity++
		}
		if node == stream {
			streamIdentity++
		}
	})
	if wrappers != 2 || sourceIdentity != 1 || streamIdentity != 1 {
		t.Fatalf("wrappers=%d blocker identity=%d stream identity=%d", wrappers, sourceIdentity, streamIdentity)
	}
}

type consoleSaveTestBlockerWriter struct {
	consoleSaveTestWriter
	marker string
}

func (w *consoleSaveTestBlockerWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), w.marker) {
		w.failAt = w.calls + 1
	}
	return w.consoleSaveTestWriter.Write(p)
}

func (w *consoleSaveTestBlockerWriter) WriteString(text string) (int, error) {
	return w.Write([]byte(text))
}

func TestConsoleSavePreviewPreservesSourceBlockerAndStreamIdentity(t *testing.T) {
	f := consoleSaveTestCreate(t, false)
	unchanged := consoleSaveTestUnchanged(t, f.target)
	value := consoleSaveTestPreview(t, f)
	before := f.op.Snapshot()
	source := errors.New("original source cause")
	blocker := &diagnostic.Error{Code: "save.fixture.blocker", Reason: "Исходная блокировка", Cause: source}
	value.Snapshot.Blockers = []*diagnostic.Error{blocker}
	fault := errors.New("original output cause")
	w := &consoleSaveTestBlockerWriter{marker: blocker.Code}
	w.cause = fault
	r := &consoleSaveTestReader{lines: []string{"1\n"}}
	err := consoleSaveTestDialog(f.op, r, w).showSavePreview(value)
	if !w.failed || w.afterFailure != 0 || r.reads != 0 {
		t.Fatalf("fault not reached or output/input continued: failed=%v writes=%d reads=%d", w.failed, w.afterFailure, r.reads)
	}
	counts := consoleSaveTestLeafCounts(err)
	blockers, streams, outputs := 0, 0, 0
	consoleSaveTestWalkErrors(err, func(node error) {
		if node == blocker {
			blockers++
		}
		if node == fault {
			streams++
		}
		if issue, ok := node.(*diagnostic.Error); ok && issue.Code == CodeOutput {
			outputs++
		}
	})
	if len(counts) != 2 || counts[source.Error()] != blockers || counts[fault.Error()] != 1 {
		t.Errorf("source/output leaf multiset changed: %v; blocker occurrences=%d", counts, blockers)
	}
	if blockers == 0 || streams != 1 || outputs != 1 || !errors.Is(err, source) || !errors.Is(err, fault) || errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("source identity=%d stream identity=%d output wrappers=%d; err=%v", blockers, streams, outputs, err)
	}
	consoleSaveTestSnapshotUnchanged(t, before, f.op.Snapshot())
	unchanged()
}
