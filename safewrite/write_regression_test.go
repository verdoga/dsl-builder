package safewrite_test

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestWriteForbiddenOverwriteReportsPreservedOriginal(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "исходный архив.bin")
	writeStore(t, target, "original\x00bytes\xff")
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	writeCalls, verifyCalls := 0, 0
	result, failures := safewrite.Write(target, false, func(io.Writer) error {
		writeCalls++
		return nil
	}, func(string) error {
		verifyCalls++
		return nil
	})
	if result != (safewrite.Result{TargetPath: target, OriginalPath: target}) {
		t.Errorf("forbidden overwrite result=%+v; want preserved original at %q", result, target)
	}
	if writeCalls != 0 || verifyCalls != 0 {
		t.Errorf("callbacks called: write=%d verify=%d", writeCalls, verifyCalls)
	}
	if len(failures) != 1 {
		t.Fatalf("want one overwrite refusal, got %v", failures)
	}
	writeDiagnostic(t, failures[0], target)
	if !slices.ContainsFunc(failures[0].Related, func(c diagnostic.Context) bool { return c.Path == target }) {
		t.Errorf("known original missing from diagnostic: %#v", failures[0].Related)
	}
	writeContents(t, target, "original\x00bytes\xff")
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("forbidden overwrite replaced the original file")
	}
	if !slices.Equal(writeNames(t, dir), []string{filepath.Base(target)}) {
		t.Error("forbidden overwrite left service files")
	}
}
