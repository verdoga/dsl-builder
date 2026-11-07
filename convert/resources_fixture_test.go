package convert

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/structure"
)

func resourcesString(value string) *string { return &value }

func resourcesElement(t *testing.T, kind model.ElementType, raw, value string) model.Element {
	t.Helper()
	return model.Element{ElementType: kind, Raw: raw, Value: resourcesString(value), ErrorIDs: []string{}}
}

func resourcesLine(t *testing.T, number int, kind model.LineType, elements ...model.Element) model.Line {
	t.Helper()
	line := model.Line{Number: number, LineType: kind, LineEnding: model.LineEndingLF, Elements: elements}
	column := 1
	var parts []string
	for i := range line.Elements {
		line.Elements[i].Start = column
		column += utf8.RuneCountInString(line.Elements[i].Raw)
		line.Elements[i].End = column
		column++
		parts = append(parts, line.Elements[i].Raw)
	}
	line.Raw = strings.Join(parts, " ")
	return line
}

func resourcesMediaLine(t *testing.T, number int, kind, raw, value string) model.Line {
	t.Helper()
	return resourcesLine(t, number, model.LineTypeTag,
		resourcesElement(t, model.ElementTypeTag, "@media", "media"),
		resourcesElement(t, model.ElementTypeMediaType, kind, kind),
		resourcesElement(t, model.ElementTypeSource, raw, value))
}

func resourcesWrite(t *testing.T, path string, bytes []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes, 0600); err != nil {
		t.Fatal(err)
	}
}

func resourcesJSON(t *testing.T, value any) string {
	t.Helper()
	bytes, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

func resourcesLoad(t *testing.T, root string, dirs []string, lines ...model.Line) (*parsed.Input, *filetree.Resolver) {
	t.Helper()
	if lines == nil {
		lines = []model.Line{}
	}
	count := len(lines)
	result := model.Result{
		Format: model.FormatVersion1,
		Document: model.Document{
			FileName: resourcesString("old.txt"), FilePath: resourcesString(`Z:\wrong-base\old.txt`), LineCount: &count,
			Metadata: model.DocumentMetadata{DocumentID: resourcesString("RES1"), Title: resourcesString("Ресурсы"), ResourceDirs: dirs},
		},
		Processing: []model.Processing{}, Lines: lines, Diagnostics: []model.Diagnostic{},
	}
	path := filepath.Join(root, "input.json")
	resourcesWrite(t, path, []byte(resourcesJSON(t, result)))
	tree := &filetree.Resolver{}
	entry, problem := tree.Inspect(path)
	if problem != nil {
		t.Fatal(problem)
	}
	input, problems := parsed.Load(entry.Path)
	if len(problems) != 0 {
		t.Fatalf("parsed.Load: %+v", problems)
	}
	return input, tree
}

func resourcesBindings(t *testing.T, input *parsed.Input) ([]lesson.Page, []structure.Binding) {
	t.Helper()
	entries, problems := fragments.Expand(input)
	if len(problems) != 0 {
		t.Fatalf("fragments.Expand: %+v", problems)
	}
	pages, bindings, problems := structure.Build(input, entries)
	if len(problems) != 0 {
		t.Fatalf("structure.Build: %+v", problems)
	}
	return pages, bindings
}

func resourcesContext(input *parsed.Input) diagnostic.Context {
	return diagnostic.Context{Path: input.Path, FileName: "l007.json", LessonID: *input.Result.Document.Metadata.DocumentID}
}

func resourcesIdentity(t *testing.T, tree *filetree.Resolver, path string) string {
	t.Helper()
	entry, problem := tree.Inspect(path)
	if problem != nil {
		t.Fatal(problem)
	}
	return entry.IdentityPath
}
