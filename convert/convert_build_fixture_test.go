package convert_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func convertBuildAt() time.Time {
	return time.Date(2024, 2, 29, 0, 15, 16, 987654321, time.FixedZone("test", 5*3600+30*60))
}

func convertBuildContext() diagnostic.Context {
	return diagnostic.Context{Path: "stale.zip", FileName: "l016.json", LessonID: "STALE", PageID: "stale-p999", Line: 909, Column: 808}
}

func convertBuildJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func convertBuildWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
}

func convertBuildRead(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func convertBuildLoad(t *testing.T, path string, tree *filetree.Resolver) *parsed.Input {
	t.Helper()
	entry, problem := tree.Inspect(path)
	if problem != nil {
		t.Fatal(problem)
	}
	input, problems := parsed.Load(entry.Path)
	if len(problems) != 0 || input == nil {
		t.Fatalf("Load: input=%v errors=%v", input, problems)
	}
	return input
}

func convertBuildFixture(t *testing.T, name string) (*parsed.Input, *filetree.Resolver) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "source.json")
	convertBuildWrite(t, path, convertBuildRead(t, filepath.Join("testdata", "convert", name+".parsed.json")))
	convertBuildWrite(t, filepath.Join(root, "assets", "Clip 003.mp4"), []byte{0, 255, 1, 13, 10, 42})
	convertBuildWrite(t, filepath.Join(root, "assets", "Clip 003.txt"), []byte{239, 187, 191, 255, 13, 10})
	convertBuildWrite(t, filepath.Join(root, "assets", "Clip 003.vtt"), []byte("WEBVTT\r\n\r\n"))
	convertBuildWrite(t, filepath.Join(root, "assets", "unused.bin"), []byte("unused"))
	tree := &filetree.Resolver{}
	return convertBuildLoad(t, path, tree), tree
}

func convertBuildFailed(t *testing.T, value *lesson.Lesson, usage *resourceusage.Registry, problems []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	if value != nil || usage != nil || len(problems) == 0 {
		t.Fatalf("failure must be atomic: value=%v usage=%v errors=%v", value, usage, problems)
	}
	if problems[0] == nil || problems[0].Code != code {
		t.Fatalf("first error=%v; want code %q", problems, code)
	}
	return problems[0]
}

func convertBuildUnchanged(t *testing.T, input *parsed.Input, before string, fileBefore []byte) {
	t.Helper()
	if after := convertBuildJSON(t, input); after != before {
		t.Fatal("Build changed loaded Input, index or nested data")
	}
	if string(convertBuildRead(t, input.Path)) != string(fileBefore) {
		t.Fatal("Build changed technical JSON bytes")
	}
}
