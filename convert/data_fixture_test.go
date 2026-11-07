package convert

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
	"github.com/verdoga/dsl-builder/structure"
	"github.com/verdoga/dsl-builder/textdata"
)

func dataLoad(t *testing.T, fixture string) (*parsed.Input, []lesson.Page, []structure.Binding) {
	t.Helper()
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source input.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	tree := new(filetree.Resolver)
	entry, problem := tree.Inspect(path)
	if problem != nil {
		t.Fatal(problem)
	}
	input, problems := parsed.Load(entry.Path)
	if len(problems) != 0 {
		t.Fatalf("Load(%s): %v", fixture, problems)
	}
	entries, problems := fragments.Expand(input)
	if len(problems) != 0 {
		t.Fatalf("Expand: %v", problems)
	}
	pages, bindings, problems := structure.Build(input, entries)
	if len(problems) != 0 {
		t.Fatalf("structure.Build: %v", problems)
	}
	return input, pages, bindings
}

func dataResources(t *testing.T, input *parsed.Input) (*resourcefind.Finder, *resourceusage.Registry) {
	t.Helper()
	for name, raw := range map[string][]byte{
		"track.mp3": {0, 1, 254, 255, '\n'},
		"track.txt": []byte("\xef\xbb\xbfFile transcript\r\n"),
		"track.vtt": []byte("WEBVTT\r\n\r\n"),
	} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(input.Path), name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	finder, paths, problem := resourcefind.Prepare(new(filetree.Resolver), input.Path,
		input.Result.Document.Metadata.ResourceDirs, dataBase(input))
	if problem != nil {
		t.Fatal(problem)
	}
	usage := new(resourceusage.Registry)
	usage.Register(paths)
	return finder, usage
}

func dataBase(input *parsed.Input) diagnostic.Context {
	return diagnostic.Context{Path: input.Path, FileName: "l013.json", LessonID: *input.Result.Document.Metadata.DocumentID}
}

func dataJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func dataEqual(t *testing.T, got, want any) {
	t.Helper()
	actual, expected := dataJSON(t, got), dataJSON(t, want)
	if !bytes.Equal(actual, expected) {
		t.Fatalf("got %s\nwant %s", actual, expected)
	}
}

func dataOwner(t *testing.T, node *lesson.Node, binding structure.Binding, base diagnostic.Context) ([]lesson.Data, *bool) {
	t.Helper()
	switch node.Type {
	case lesson.NodeQuestion, lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering:
		values, checkable, problem := exercisedata.Build(node.Type, binding.Lines(), base)
		if problem != nil {
			t.Fatalf("owner %s: %v", node.Type, problem)
		}
		return values, &checkable
	default:
		declaration, lines := binding.TextSource()
		values, problem := textdata.Build(node.Type, declaration, lines, base)
		if problem != nil {
			t.Fatalf("owner %s: %v", node.Type, problem)
		}
		return values, nil
	}
}

func dataRole(t *testing.T, values []lesson.Data, role lesson.DataRole) lesson.Data {
	t.Helper()
	for _, value := range values {
		if value.Role != nil && *value.Role == role {
			return value
		}
	}
	t.Fatalf("role %s missing in %s", role, dataJSON(t, values))
	return lesson.Data{}
}

func dataText(t *testing.T, value lesson.Data, want string, answers []string, hint string) {
	t.Helper()
	if value.Type != lesson.DataText || value.Value != want || value.Hint != hint {
		t.Fatalf("text got %s, want %q hint %q", dataJSON(t, value), want, hint)
	}
	dataEqual(t, value.Answers, answers)
}

func dataCollection(t *testing.T, value lesson.Data, role lesson.DataRole) []lesson.Data {
	t.Helper()
	values, ok := value.Value.([]lesson.Data)
	if !ok || value.Type != lesson.DataCollection || values == nil {
		t.Fatalf("collection: %s", dataJSON(t, value))
	}
	for _, item := range values {
		if item.Role == nil || *item.Role != role || item.Type != lesson.DataText || item.Answers == nil {
			t.Fatalf("item role/type/answers: %s", dataJSON(t, item))
		}
	}
	return values
}
