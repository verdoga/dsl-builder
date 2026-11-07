package fragments_test

import (
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task20CheckSources(t *testing.T, input *parsed.Input, entries []*fragments.Entry) {
	t.Helper()
	for _, entry := range entries {
		source := &input.Result.Lines[input.Index.ByNumber[entry.Source.Number]]
		if task20JSON(t, entry.Source) != task20JSON(t, source) {
			t.Fatalf("source line %d changed: %+v", source.Number, entry.Source)
		}
		if source.ParentLine != nil && entry.Source.ParentLine == source.ParentLine {
			t.Fatalf("line %d shares ParentLine with input", source.Number)
		}
		if len(source.Elements) > 0 && &entry.Source.Elements[0] == &source.Elements[0] {
			t.Fatalf("line %d shares Elements with input", source.Number)
		}
		for i, element := range source.Elements {
			copy := entry.Source.Elements[i]
			if element.Value != nil && element.Value == copy.Value {
				t.Fatalf("line %d element %d shares Value with input", source.Number, i)
			}
			if len(element.ErrorIDs) > 0 && &element.ErrorIDs[0] == &copy.ErrorIDs[0] {
				t.Fatalf("line %d element %d shares ErrorIDs with input", source.Number, i)
			}
		}
		if entry.Included != nil {
			task20CheckSources(t, input, entry.Included)
		} else if len(entry.Source.Elements) > 0 && entry.Source.Elements[0].Value != nil && *entry.Source.Elements[0].Value == "include" {
			t.Fatal("include lost its boundary")
		}
	}
}

func task20Distinct(t *testing.T, first, second []*fragments.Entry) {
	t.Helper()
	if len(first) != len(second) {
		t.Fatal("independent expansions have different lengths")
	}
	for i, a := range first {
		b := second[i]
		if a == b || a.Source.ParentLine != nil && a.Source.ParentLine == b.Source.ParentLine {
			t.Fatalf("line %d shares Entry or ParentLine between instances", a.Source.Number)
		}
		if len(a.Source.Elements) > 0 && &a.Source.Elements[0] == &b.Source.Elements[0] {
			t.Fatalf("line %d shares Elements between instances", a.Source.Number)
		}
		for j, element := range a.Source.Elements {
			other := b.Source.Elements[j]
			if element.Value != nil && element.Value == other.Value {
				t.Fatalf("line %d shares element Value between instances", a.Source.Number)
			}
			if len(element.ErrorIDs) > 0 && &element.ErrorIDs[0] == &other.ErrorIDs[0] {
				t.Fatalf("line %d shares ErrorIDs between instances", a.Source.Number)
			}
		}
		if a.Included != nil {
			if b.Included == nil {
				t.Fatal("independent expansion lost Included")
			}
			task20Distinct(t, a.Included, b.Included)
		}
	}
}

func TestTask20LoadExpandPreservesSourceAndOwnsEveryInstance(t *testing.T) {
	lines := []model.Line{
		task20Tag(1, 0, 0, "task", "A", false), task20Tag(2, 1, 1, "include", "A", false),
		task20Tag(3, 1, 1, "include", "a", false), task20Tag(4, 1, 1, "instr", "", false),
		task20Tag(5, 0, 0, "endtask", "", false), task20Tag(6, 0, 0, "fragment", "A", true),
		task20Tag(7, 6, 1, "script", "", true), task20Content(8, 7, 2, ` Ю\@ <b> `, ` Ю@ <b> `),
		task20Line(9, 7, 2, model.LineTypeBlank, " \t "), task20End(10, 7, 2),
		task20Tag(11, 6, 1, "multifill", "", true), task20Content(12, 11, 2, "", ""),
		task20Line(13, 11, 2, model.LineTypeContent, "_____{x} _____",
			task20Element(model.ElementTypePlaceholder, "_____{x}", "x", 1),
			task20Element(model.ElementTypePlaceholder, "_____", "", 10)), task20End(14, 11, 2),
		task20Line(15, 6, 1, model.LineTypeTag, `@media audio "asset\name.mp3"`,
			task20Element(model.ElementTypeTag, "@media", "media", 1),
			task20Element(model.ElementTypeMediaType, "audio", "audio", 8),
			task20Element(model.ElementTypeSource, `"asset\name.mp3"`, `asset\name.mp3`, 14)), task20End(16, 6, 1),
	}
	lines[7].LineEnding = model.LineEndingCRLF
	lines[12].Elements[1].Value = nil
	lines[15].LineEnding = model.LineEndingNone
	lines[7].Elements[0].ErrorIDs = []string{"warning", "recommendation"}
	result := task20Document(lines...)
	result.Processing = []model.Processing{{ID: "parse", Tool: "dsl-parser"}}
	for _, severity := range []model.Severity{model.SeverityWarning, model.SeverityRecommendation} {
		result.Diagnostics = append(result.Diagnostics, model.Diagnostic{ID: string(severity), Source: "parse",
			DiagnosticCode: "nonblocking", SeverityLevel: severity, DiagnosticScope: model.ScopeElement,
			Message: "Не выводить", Location: &model.Location{Start: model.Position{Line: 8, Column: 1},
				End: model.Position{Line: 8, Column: lines[7].Elements[0].End}}, RelatedLocations: []model.Location{}})
	}
	input := task20Load(t, result)
	snapshot := task20JSON(t, input)
	bytesBefore, err := os.ReadFile(input.Path)
	if err != nil {
		t.Fatal(err)
	}
	first := task20Expand(t, input)
	task20Numbers(t, first, 1, 2, 3, 4, 5)
	for _, site := range []*fragments.Entry{first[1], first[2]} {
		task20Numbers(t, site.Included, 7, 8, 9, 10, 11, 12, 13, 14, 15)
		body := site.Included
		if site.Parent != first[0] || body[0].Parent != site || body[4].Parent != site || body[8].Parent != site ||
			body[1].Parent != body[0] || body[2].Parent != body[0] || body[3].Parent != body[0] ||
			body[5].Parent != body[4] || body[6].Parent != body[4] || body[7].Parent != body[4] {
			t.Fatal("include boundary or internal block parents are incorrect")
		}
		for _, ordinary := range body {
			if ordinary.Included != nil {
				t.Fatalf("ordinary line %d has Included", ordinary.Source.Number)
			}
		}
	}
	if first[3].Parent != first[0] || first[4].Parent != nil {
		t.Fatal("following instruction/endtask crossed the include boundary")
	}
	task20CheckSources(t, input, first)
	task20Distinct(t, first[1].Included, first[2].Included)
	bytesAfter, err := os.ReadFile(input.Path)
	if err != nil || string(bytesBefore) != string(bytesAfter) {
		t.Fatalf("technical JSON bytes changed: %v", err)
	}
	if err := os.Remove(input.Path); err != nil {
		t.Fatal(err)
	}
	second := task20Expand(t, input)
	task20Distinct(t, first, second)
	task20CheckSources(t, input, second)
	if task20JSON(t, input) != snapshot || input.Path == *input.Result.Document.FilePath || *input.Result.Document.LineCount != 16 {
		t.Fatal("Input, metadata, resource base, indices or physical lineCount changed")
	}
}

func TestTask20LoadExpandPreservesUnknownPhysicalLineCount(t *testing.T) {
	result := task20Document(task20Line(3, 0, 0, model.LineTypeBlank, ""))
	result.Document.LineCount = nil
	result.Document.Metadata.ResourceDirs = nil
	input := task20Load(t, result)
	entries := task20Expand(t, input)
	task20Numbers(t, entries, 3)
	task20CheckSources(t, input, entries)
	if input.Result.Document.LineCount != nil || input.Result.Document.Metadata.ResourceDirs != nil {
		t.Fatal("unknown metadata must remain null")
	}
}
