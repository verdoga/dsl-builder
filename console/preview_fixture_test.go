package console

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/moduleio"
)

func previewTestIssues(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("fixture owner rejected input: %v", issues)
	}
}

func previewTestCreate(t *testing.T, count int) (*app.Operation, string) {
	t.Helper()
	base := t.TempDir()
	sources := filepath.Join(base, "исходники")
	if err := os.Mkdir(sources, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../golden/2. parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"A.json", "B.json", "C.json"}[:count] {
		data := raw
		if index != 0 {
			data = bytes.ReplaceAll(raw, []byte("BA82"), []byte([]string{"BA82", "BA83", "BA84"}[index]))
		}
		if err := os.WriteFile(filepath.Join(sources, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	op, issues := app.NewCreate(base, sources, "Курс preview", "Юнит preview")
	previewTestIssues(t, issues)
	previewTestIssues(t, op.Analyze(time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)))
	t.Cleanup(op.Cancel)
	return op, base
}

func previewTestArchive(t *testing.T) (string, string) {
	t.Helper()
	op, base := previewTestCreate(t, 1)
	path := filepath.Join(base, "исходный модуль.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	issues := moduleio.Write(file, op.Snapshot().Module, diagnostic.Context{Path: path})
	closeErr := file.Close()
	previewTestIssues(t, issues)
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	previewTestIssues(t, moduleio.Verify(path))
	return path, base
}

func previewTestUpdate(t *testing.T) *app.Operation {
	t.Helper()
	archive, base := previewTestArchive(t)
	raw, err := os.ReadFile("../golden/2. parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"TITLE", "FRESH"}, {"Subtitle", "Prepared"}, {"INSTRUCTION TEXT A", "INSTRUCTION TEXT Z"}} {
		raw = bytes.ReplaceAll(raw, []byte(pair[0]), []byte(pair[1]))
	}
	path := filepath.Join(base, "обновление.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	op, issues := app.NewUpdate(base, archive, path)
	previewTestIssues(t, issues)
	previewTestIssues(t, op.Analyze(time.Date(2026, 11, 5, 11, 0, 0, 0, time.UTC)))
	t.Cleanup(op.Cancel)
	return op
}

func previewTestReadyUpdate(t *testing.T) *app.Operation {
	t.Helper()
	op := previewTestUpdate(t)
	snapshot := op.Snapshot()
	if len(snapshot.Plan.Fields) != 2 {
		t.Fatalf("fixture fields = %v; want title and subtitle", snapshot.Plan.Fields)
	}
	previewTestIssues(t, op.DecideField(snapshot.Token, 0, true))
	previewTestIssues(t, op.DecideField(op.Snapshot().Token, 1, false))
	previewTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
	if !op.Snapshot().RevisionReady {
		t.Fatal("owner did not prepare revision")
	}
	return op
}

func previewTestDialog(output *bytes.Buffer, input string) *dialog {
	return &dialog{input: bufio.NewReader(strings.NewReader(input)), output: output}
}

func previewTestContains(t *testing.T, output string, expected ...string) {
	t.Helper()
	for _, value := range expected {
		if value == "" || !strings.Contains(output, value) {
			t.Errorf("output missing %q\n%s", value, output)
		}
	}
}

func previewTestAbsent(t *testing.T, output string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if strings.Contains(output, value) {
			t.Errorf("output leaked %q\n%s", value, output)
		}
	}
}

func previewTestBlock(t *testing.T, block string, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("expected representable owner block: %v", err)
	}
	return block
}

func previewTestString(value string) *string { return &value }
