package structure_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/structure"
)

func task26String(value string) *string { return &value }

// task26Element задаёт семантический элемент, не разбирая DSL.
func task26Element(kind model.ElementType, value string) model.Element {
	return model.Element{ElementType: kind, Raw: value, Value: task26String(value), ErrorIDs: []string{}}
}

func task26Line(number, parent int, kind model.LineType, elements ...model.Element) model.Line {
	line := model.Line{Number: number, LineType: kind, LineEnding: model.LineEndingCRLF, Elements: []model.Element{}}
	if parent != 0 {
		line.ParentLine = &parent
	}
	for _, element := range elements {
		if len(line.Elements) != 0 {
			line.Raw += " "
		}
		element.Start = len([]rune(line.Raw)) + 1
		line.Raw += element.Raw
		element.End = len([]rune(line.Raw)) + 1
		line.Elements = append(line.Elements, element)
	}
	return line
}

func task26Tag(number, parent int, kind model.LineType, tag string, elements ...model.Element) model.Line {
	first := task26Element(model.ElementTypeTag, tag)
	first.Raw = "@" + tag
	parts := append([]model.Element{first}, elements...)
	if kind == model.LineTypeBlockStart {
		parts = append(parts, task26Element(model.ElementTypeBlockOpen, "{"))
	}
	return task26Line(number, parent, kind, parts...)
}

func task26Text(number, parent int, text string) model.Line {
	return task26Line(number, parent, model.LineTypeContent, task26Element(model.ElementTypeContent, text))
}

func task26End(number, parent int) model.Line {
	return task26Line(number, parent, model.LineTypeBlockEnd, task26Element(model.ElementTypeBlockClose, "}"))
}

func task26Heading(number int, level, title string) model.Line {
	element := task26Element(model.ElementTypeHeadingLevel, level)
	element.Raw = strings.Repeat("#", int(level[0]-'0'))
	return task26Line(number, 0, model.LineTypeHeading, element, task26Element(model.ElementTypeTitle, title))
}

func task26Load(t *testing.T, lines []model.Line) *parsed.Input {
	t.Helper()
	if lines == nil {
		lines = []model.Line{}
	}
	for i := range lines {
		if lines[i].ParentLine != nil {
			parent := *lines[i].ParentLine
			for j := 0; j < i; j++ {
				if lines[j].Number == parent {
					lines[i].NestingLevel = lines[j].NestingLevel + 1
				}
			}
		}
	}
	count := len(lines)
	result := model.Result{Format: model.FormatVersion1, Lines: lines, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Document: model.Document{LineCount: &count, Metadata: model.DocumentMetadata{
			DocumentID: task26String("QA26"), Title: task26String("Не название страницы"), ResourceDirs: []string{},
		}},
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "технический урок.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	input, problems := parsed.Load(path)
	if len(problems) != 0 {
		t.Fatalf("Load: %+v", problems)
	}
	return input
}

func task26Expand(t *testing.T, input *parsed.Input) []*fragments.Entry {
	t.Helper()
	entries, problems := fragments.Expand(input)
	if len(problems) != 0 {
		t.Fatalf("Expand: %+v", problems)
	}
	return entries
}

func task26Build(t *testing.T, lines []model.Line, want string) ([]lesson.Page, []structure.Binding) {
	t.Helper()
	input := task26Load(t, lines)
	entries := task26Expand(t, input)
	pages, bindings, problems := structure.Build(input, entries)
	if len(problems) != 0 {
		t.Fatalf("Build: %+v", problems)
	}
	task26Invariants(t, pages, bindings)
	if got := task26Shape(pages); got != want {
		t.Fatalf("shape=%s; want %s", got, want)
	}
	return pages, bindings
}

func task26Shape(pages []lesson.Page) string {
	var describe func([]lesson.Node) string
	describe = func(nodes []lesson.Node) string {
		kinds := make([]string, len(nodes))
		for i, node := range nodes {
			kinds[i] = string(node.Type)
			if len(node.Nodes) != 0 {
				kinds[i] += "(" + describe(node.Nodes) + ")"
			}
		}
		return strings.Join(kinds, ",")
	}
	parts := make([]string, len(pages))
	for i, page := range pages {
		parts[i] = "[" + describe(page.Nodes) + "]"
	}
	return strings.Join(parts, "|")
}

func task26Invariants(t *testing.T, pages []lesson.Page, bindings []structure.Binding) {
	t.Helper()
	if pages == nil || bindings == nil {
		t.Fatal("successful slices must be non-nil")
	}
	next := 0
	var visit func(int, []int, []lesson.Node)
	visit = func(page int, path []int, nodes []lesson.Node) {
		for i := range nodes {
			node := &nodes[i]
			current := append(slices.Clone(path), i)
			if next >= len(bindings) {
				t.Fatal("node has no binding")
			}
			binding := bindings[next]
			next++
			if binding.Page != page || !slices.Equal(binding.NodePath, current) || binding.Node(pages) != node {
				t.Fatalf("binding %d does not address preorder node %d/%v: %+v", next-1, page, current, binding)
			}
			if len(binding.Entries) == 0 {
				t.Fatalf("empty Entries at %v", current)
			}
			if node.ID != "" || node.Data == nil || len(node.Data) != 0 || node.Nodes == nil {
				t.Fatalf("premature data/ID or nil slices: %+v", node)
			}
			if node.Flags.Checkable != nil {
				t.Fatal("structure assigned checkable")
			}
			if node.Type == lesson.NodeTask {
				if node.Flags.Speaking == nil || node.Flags.Stepped == nil {
					t.Fatal("task flags must be explicit")
				}
			} else if node.Flags.Speaking != nil || node.Flags.Stepped != nil {
				t.Fatal("task flags on another type")
			}
			lines := binding.Lines()
			if len(lines) != len(binding.Entries) {
				t.Fatal("incomplete Lines projection")
			}
			for j, entry := range binding.Entries {
				a, err := json.Marshal(lines[j])
				if err != nil {
					t.Fatal(err)
				}
				b, err := json.Marshal(entry.Source)
				if err != nil {
					t.Fatal(err)
				}
				if string(a) != string(b) {
					t.Fatalf("Lines changed Source %d", j)
				}
			}
			visit(page, current, node.Nodes)
		}
	}
	for i, page := range pages {
		if page.ID != "" || page.Nodes == nil {
			t.Fatalf("invalid draft page %d", i)
		}
		visit(i, nil, page.Nodes)
	}
	if next != len(bindings) {
		t.Fatalf("%d nodes but %d bindings", next, len(bindings))
	}
}

func task26Numbers(binding structure.Binding) []int {
	numbers := make([]int, len(binding.Entries))
	for i, entry := range binding.Entries {
		numbers[i] = entry.Source.Number
	}
	return numbers
}

func task26Binding(t *testing.T, bindings []structure.Binding, line int, kind lesson.NodeType, pages []lesson.Page) structure.Binding {
	t.Helper()
	for _, binding := range bindings {
		if binding.Entries[0].Source.Number == line && binding.Node(pages).Type == kind {
			return binding
		}
	}
	t.Fatal(fmt.Sprintf("no binding for %s at line %d", kind, line))
	return structure.Binding{}
}
