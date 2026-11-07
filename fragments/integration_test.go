package fragments_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task20String(value string) *string { return &value }

func task20Element(kind model.ElementType, raw, value string, start int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: task20String(value), Start: start,
		End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}}
}

func task20Line(number, parent, depth int, kind model.LineType, raw string, elements ...model.Element) model.Line {
	line := model.Line{Number: number, LineType: kind, NestingLevel: depth, Raw: raw,
		LineEnding: model.LineEndingLF, Elements: append([]model.Element{}, elements...)}
	if parent != 0 {
		line.ParentLine = &parent
	}
	return line
}

// task20Tag создаёт typed Contract fixture; raw не разбирается как DSL.
func task20Tag(number, parent, depth int, tag, identifier string, block bool) model.Line {
	raw := "@" + strings.ToUpper(tag)
	elements := []model.Element{task20Element(model.ElementTypeTag, raw, tag, 1)}
	if identifier != "" {
		elements = append(elements, task20Element(model.ElementTypeIdentifier, identifier, identifier, utf8.RuneCountInString(raw)+2))
		raw += " " + identifier
	}
	kind := model.LineTypeTag
	if block {
		kind = model.LineTypeBlockStart
		elements = append(elements, task20Element(model.ElementTypeBlockOpen, "{", "{", utf8.RuneCountInString(raw)+2))
		raw += " {"
	}
	return task20Line(number, parent, depth, kind, raw, elements...)
}

func task20End(number, parent, depth int) model.Line {
	return task20Line(number, parent, depth, model.LineTypeBlockEnd, "  }  ",
		task20Element(model.ElementTypeBlockClose, "}", "}", 3))
}

func task20Content(number, parent, depth int, raw, value string) model.Line {
	return task20Line(number, parent, depth, model.LineTypeContent, raw,
		task20Element(model.ElementTypeContent, raw, value, 1))
}

func task20Document(lines ...model.Line) model.Result {
	count := len(lines)
	return model.Result{Format: model.FormatVersion1, Document: model.Document{
		DSLVersion: task20String("1.2"), FileName: task20String("source.dsl"),
		FilePath: task20String(`C:\unavailable\source.dsl`), LineCount: &count,
		Metadata: model.DocumentMetadata{DocumentID: task20String("QA20"),
			Title: task20String("Фрагменты"), ResourceDirs: []string{`missing\resources`}}},
		Lines: append([]model.Line{}, lines...), Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{}}
}

func task20JSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func task20Load(t *testing.T, result model.Result) *parsed.Input {
	t.Helper()
	path := filepath.Join(t.TempDir(), "технический вход.json")
	if err := os.WriteFile(path, []byte(task20JSON(t, result)), 0600); err != nil {
		t.Fatal(err)
	}
	input, issues := parsed.Load(path)
	if input == nil || issues != nil {
		t.Fatalf("parsed.Load rejected typed fixture: input=%v, issues=%v", input, issues)
	}
	return input
}

func task20Expand(t *testing.T, input *parsed.Input) []*fragments.Entry {
	t.Helper()
	entries, issues := fragments.Expand(input)
	if entries == nil || issues != nil {
		t.Fatalf("Expand: entries=%v, issues=%v", entries, issues)
	}
	return entries
}

func task20Numbers(t *testing.T, entries []*fragments.Entry, want ...int) {
	t.Helper()
	got := make([]int, len(entries))
	for i, entry := range entries {
		if entry == nil {
			t.Fatalf("nil Entry at %d", i)
		}
		got[i] = entry.Source.Number
	}
	if !slices.Equal(got, want) {
		t.Fatalf("physical sequence = %v, want %v", got, want)
	}
}

func TestTask20LoadExpandForwardDefinitionsAcrossBranches(t *testing.T) {
	input := task20Load(t, task20Document(
		task20Tag(1, 0, 0, "include", "фРАГ/自由", false),
		task20Tag(2, 0, 0, "variants", "", true),
		task20Tag(3, 2, 1, "variant", "", false),
		task20Tag(4, 3, 2, "fragment", "Фраг/自由", true),
		task20Tag(5, 4, 3, "text", "", true),
		task20Content(6, 5, 4, `\@include literal`, "@include literal"),
		task20End(7, 5, 4), task20End(8, 4, 3),
		task20Tag(9, 2, 1, "variant", "", false),
		task20Tag(10, 9, 2, "include", "ФРАГ/自由", false), task20End(11, 2, 1),
		task20Content(12, 0, 0, "after", "after")))
	entries := task20Expand(t, input)
	task20Numbers(t, entries, 1, 2, 3, 9, 10, 11, 12)
	for _, site := range []*fragments.Entry{entries[0], entries[4]} {
		task20Numbers(t, site.Included, 5, 6, 7)
		if site.Included[0].Parent != site || site.Included[1].Parent != site.Included[0] || site.Included[2].Parent != site.Included[0] {
			t.Fatal("body and block-end parents must bind inside this inclusion")
		}
		if *site.Included[0].Source.ParentLine != 4 || site.Included[0].Source.NestingLevel != 3 {
			t.Fatal("effective boundary must not rewrite physical source coordinates")
		}
	}
	if entries[4].Parent != entries[3] || entries[3].Parent != entries[1] || entries[5].Parent != entries[1] {
		t.Fatal("outer branch and block-end parent links were lost")
	}
	if entries[0].Included[0] == entries[4].Included[0] || entries[6].Parent != nil {
		t.Fatal("instances must be independent; following root remains outside include")
	}
}

func TestTask20LoadExpandNestedDefinitionsAndInclusions(t *testing.T) {
	input := task20Load(t, task20Document(
		task20Tag(1, 0, 0, "include", "Outer", false), task20Tag(2, 0, 0, "include", "Inner", false),
		task20Tag(3, 0, 0, "fragment", "Outer", true), task20Content(4, 3, 1, "}", "}"),
		task20Tag(5, 3, 1, "fragment", "Inner", true), task20Content(6, 5, 2, "inner", "inner"),
		task20End(7, 5, 2), task20Tag(8, 3, 1, "include", "Inner", false), task20End(9, 3, 1),
		task20Content(10, 0, 0, "after", "after")))
	entries := task20Expand(t, input)
	task20Numbers(t, entries, 1, 2, 10)
	task20Numbers(t, entries[0].Included, 4, 8)
	nested := entries[0].Included[1]
	task20Numbers(t, nested.Included, 6)
	task20Numbers(t, entries[1].Included, 6)
	if nested.Parent != entries[0] || nested.Included[0].Parent != nested || entries[1].Included[0].Parent != entries[1] {
		t.Fatal("nested inclusion must keep both effective boundaries")
	}
	if nested.Included[0] == entries[1].Included[0] {
		t.Fatal("nested and direct use share an Entry")
	}
}

func TestTask20LoadExpandUsesSemanticNamesWithoutReparsingRaw(t *testing.T) {
	site := task20Tag(1, 0, 0, "include", "encoded", false)
	site.Elements[1].Value = task20String(`Имя\x`)
	definition := task20Tag(2, 0, 0, "fragment", "different", true)
	definition.Elements[1].Value = task20String(`имя\X`)
	input := task20Load(t, task20Document(site, definition,
		task20Content(3, 2, 1, `\@literal`, `\@literal`), task20End(4, 2, 1)))
	entries := task20Expand(t, input)
	task20Numbers(t, entries, 1)
	task20Numbers(t, entries[0].Included, 3)
	if task20JSON(t, entries[0].Source) != task20JSON(t, input.Result.Lines[0]) ||
		task20JSON(t, entries[0].Included[0].Source) != task20JSON(t, input.Result.Lines[2]) {
		t.Fatal("Raw, semantic values, or physical coordinates changed")
	}
}

func TestTask20LoadExpandEmptySequences(t *testing.T) {
	for _, result := range []model.Result{task20Document(), task20Document(
		task20Tag(1, 0, 0, "fragment", "Empty", true), task20End(2, 1, 1))} {
		if got := task20Expand(t, task20Load(t, result)); len(got) != 0 {
			t.Fatalf("definitions produce no output: %v", got)
		}
	}
	entries := task20Expand(t, task20Load(t, task20Document(
		task20Tag(1, 0, 0, "include", "Empty", false), task20Tag(2, 0, 0, "include", "Empty", false),
		task20Tag(3, 0, 0, "fragment", "Empty", true), task20End(4, 3, 1))))
	task20Numbers(t, entries, 1, 2)
	for _, site := range entries {
		if site.Included == nil || len(site.Included) != 0 {
			t.Fatal("empty include requires a nonnil empty Included")
		}
	}
}
