package fragments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task18Element(kind model.ElementType, raw, value string) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: &value, ErrorIDs: []string{}}
}

func task18Line(parent int, kind model.LineType, tag, name string) model.Line {
	line := model.Line{LineType: kind, LineEnding: model.LineEndingCRLF, Elements: []model.Element{}}
	if parent != 0 {
		line.ParentLine = &parent
	}
	if tag != "" {
		line.Elements = append(line.Elements, task18Element(model.ElementTypeTag, "@"+tag, tag))
	}
	if name != "" {
		kind := model.ElementTypeIdentifier
		if line.LineType == model.LineTypeContent {
			kind = model.ElementTypeContent
		}
		line.Elements = append(line.Elements, task18Element(kind, name, name))
	}
	if kind == model.LineTypeBlockStart {
		line.Elements = append(line.Elements, task18Element(model.ElementTypeBlockOpen, "{", "{"))
	}
	if kind == model.LineTypeBlockEnd {
		line.Elements = append(line.Elements, task18Element(model.ElementTypeBlockClose, "}", "}"))
	}
	for i := range line.Elements {
		if i != 0 {
			line.Raw += " "
		}
		e := &line.Elements[i]
		e.Start = utf8.RuneCountInString(line.Raw) + 1
		line.Raw += e.Raw
		e.End = utf8.RuneCountInString(line.Raw) + 1
	}
	return line
}

func task18JSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func task18Load(t *testing.T, lines ...model.Line) *parsed.Input {
	t.Helper()
	if lines == nil {
		lines = []model.Line{}
	}
	for i := range lines {
		lines[i].Number = i + 1
		if lines[i].ParentLine != nil {
			lines[i].NestingLevel = lines[*lines[i].ParentLine-1].NestingLevel + 1
		}
	}
	id, source, name, count := "QA18", `C:\old\source.txt`, "source.txt", len(lines)
	result := model.Result{Format: model.FormatVersion1, Lines: lines, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Document: model.Document{FileName: &name, FilePath: &source, LineCount: &count,
			Metadata: model.DocumentMetadata{DocumentID: &id, ResourceDirs: []string{"missing media"}}}}
	path := filepath.Join(t.TempDir(), "технический JSON.json")
	if err := os.WriteFile(path, []byte(task18JSON(t, result)), 0600); err != nil {
		t.Fatal(err)
	}
	input, problems := parsed.Load(path)
	if input == nil || problems != nil {
		t.Fatalf("Load: %v", problems)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	return input
}

func task18Sources(t *testing.T, input *parsed.Input, entries []*Entry, want []int) {
	t.Helper()
	if entries == nil || len(entries) != len(want) {
		t.Fatalf("entries length=%d, want nonnil length=%d", len(entries), len(want))
	}
	for i, number := range want {
		if entries[i] == nil || task18JSON(t, entries[i].Source) != task18JSON(t, input.Result.Lines[input.Index.ByNumber[number]]) {
			t.Fatalf("entry %d: source differs from physical line %d", i, number)
		}
	}
}

func TestTask18ExpandGlobalDefinitionsAndIndependentCalls(t *testing.T) {
	lines := []model.Line{
		task18Line(0, "tag", "task", "T"), task18Line(1, "tag", "include", "А"),
		task18Line(1, "content", "", "сосед"), task18Line(1, "tag", "include", "а"),
		task18Line(0, "block-start", "variants", ""), task18Line(5, "block-start", "fragment", "А"),
		task18Line(6, "block-start", "script", ""), task18Line(7, "content", "", `текст\{x}`),
		task18Line(7, "blank", "", ""), task18Line(7, "content", "", "_____"),
		task18Line(7, "block-end", "", ""), task18Line(6, "tag", "include", "B"),
		task18Line(6, "block-end", "", ""), task18Line(5, "block-end", "", ""),
		task18Line(0, "block-start", "fragment", "B"), task18Line(15, "block-end", "", ""),
		task18Line(0, "tag", "include", "B")}
	semantic := "текст{x}"
	lines[7].Elements[0].Value = &semantic
	lines[9].Elements[0].ElementType = model.ElementTypePlaceholder
	lines[9].Elements[0].Value = nil
	lines[16].LineEnding = model.LineEndingNone
	input := task18Load(t, lines...)
	before := task18JSON(t, input)
	var previous []*Entry
	for call := 0; call < 2; call++ {
		out, problems := Expand(input)
		if problems != nil {
			t.Fatal(problems)
		}
		task18Sources(t, input, out, []int{1, 2, 3, 4, 5, 14, 17})
		if out[0].Parent != nil || out[1].Parent != out[0] || out[2].Parent != out[0] || out[3].Parent != out[0] || out[5].Parent != out[4] {
			t.Fatal("document parents or neighboring line changed")
		}
		for _, position := range []int{1, 3} {
			site := out[position]
			task18Sources(t, input, site.Included, []int{7, 8, 9, 10, 11, 12})
			body := site.Included
			if body[0].Parent != site || body[1].Parent != body[0] || body[2].Parent != body[0] || body[3].Parent != body[0] || body[4].Parent != body[0] || body[5].Parent != site {
				t.Fatal("body sourceRoot/boundary parents differ")
			}
			if body[5].Included == nil || len(body[5].Included) != 0 {
				t.Fatal("nested empty inclusion must be nonnil")
			}
			for _, entry := range body[:5] {
				if entry.Included != nil {
					t.Fatal("ordinary body line acquired Included")
				}
			}
		}
		for _, position := range []int{0, 2, 4, 5} {
			if out[position].Included != nil {
				t.Fatal("ordinary line acquired Included")
			}
		}
		if out[6].Included == nil || len(out[6].Included) != 0 {
			t.Fatal("empty inclusion must be nonnil")
		}
		a, b := out[1].Included, out[3].Included
		for i := range a {
			source := &input.Result.Lines[input.Index.ByNumber[a[i].Source.Number]]
			if a[i] == b[i] || a[i].Source.ParentLine == b[i].Source.ParentLine || a[i].Source.ParentLine == source.ParentLine {
				t.Fatal("inclusions share mutable entry/parent pointer")
			}
			if len(a[i].Source.Elements) > 0 {
				x, y, original := &a[i].Source.Elements[0], &b[i].Source.Elements[0], &source.Elements[0]
				if x == y || x == original || x.Value != nil && (x.Value == y.Value || x.Value == original.Value) {
					t.Fatal("inclusions share mutable element/value")
				}
			}
		}
		if previous != nil && (out[0] == previous[0] || a[0] == previous[1].Included[0]) {
			t.Fatal("calls reuse entries")
		}
		previous = out
		if task18JSON(t, input) != before {
			t.Fatal("input, resource base, lineCount or index changed")
		}
	}
}

func TestTask18ExpandEmptyAndBlockingStages(t *testing.T) {
	for _, lines := range [][]model.Line{nil, {task18Line(0, "block-start", "fragment", "A"), task18Line(1, "block-end", "", "")}} {
		out, problems := Expand(task18Load(t, lines...))
		if out == nil || len(out) != 0 || problems != nil {
			t.Fatalf("empty success: %v, %v", out, problems)
		}
	}
	for _, tc := range []struct {
		name, code string
		lines      []model.Line
	}{
		{"missing", CodeMissing, []model.Line{task18Line(0, "content", "", "prefix"), task18Line(0, "tag", "include", "absent")}},
		{"unused reference", CodeMissing, []model.Line{task18Line(0, "block-start", "fragment", "A"), task18Line(1, "tag", "include", "absent"), task18Line(1, "block-end", "", "")}},
		{"unused duplicate", CodeAmbiguous, []model.Line{task18Line(0, "block-start", "fragment", "A"), task18Line(1, "block-end", "", ""), task18Line(0, "block-start", "fragment", "a"), task18Line(3, "block-end", "", "")}},
		{"unused cycle", CodeCycle, []model.Line{task18Line(0, "block-start", "fragment", "A"), task18Line(1, "tag", "include", "A"), task18Line(1, "block-end", "", "")}},
		{"shape", CodeShape, []model.Line{task18Line(0, "tag", "fragment", "A")}},
		{"missing boundary", CodeShape, []model.Line{task18Line(0, "block-start", "fragment", "A")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := task18Load(t, tc.lines...)
			before := task18JSON(t, input)
			out, problems := Expand(input)
			if out != nil || len(problems) == 0 {
				t.Fatalf("partial result or missing error: %v, %v", out, problems)
			}
			if !slices.ContainsFunc(problems, func(e *diagnostic.Error) bool { return e != nil && e.Code == tc.code }) {
				t.Fatalf("missing %s: %v", tc.code, problems)
			}
			if task18JSON(t, input) != before {
				t.Fatal("failed expansion mutated input")
			}
		})
	}
}

func TestTask18SequenceAndInclusionHelpers(t *testing.T) {
	input := task18Load(t, task18Line(0, "tag", "include", "A"), task18Line(0, "content", "", "neighbor"),
		task18Line(0, "block-start", "fragment", "A"), task18Line(3, "block-start", "multifill", ""),
		task18Line(4, "content", "", "тело"), task18Line(4, "block-end", "", ""), task18Line(3, "block-end", "", ""))
	definitions, problems := collectDefinitions(input)
	if problems != nil {
		t.Fatal(problems)
	}
	refs, problems := resolveIncludes(input, definitions)
	if problems != nil || len(checkIncludeCycles(input, definitions, refs)) != 0 {
		t.Fatalf("helper preconditions: %v", problems)
	}
	entries := copyEntries(input, []int{1, 2})
	if problems := bindParents(input, entries, 0, nil); problems != nil {
		t.Fatal(problems)
	}
	neighbor, original := entries[1], task18JSON(t, entries[1].Source)
	if problems := expandInclusions(input, entries, refs); problems != nil {
		t.Fatal(problems)
	}
	if entries[1] != neighbor || neighbor.Parent != nil || neighbor.Included != nil || task18JSON(t, neighbor.Source) != original {
		t.Fatal("expandInclusions changed neighboring entry")
	}
	for call := 0; call < 2; call++ {
		body, problems := expandInclusion(input, entries[0], refs[1], refs)
		if problems != nil {
			t.Fatal(problems)
		}
		task18Sources(t, input, body, []int{4, 5, 6})
		if body[0] == entries[0].Included[0] || body[0].Parent != entries[0] || body[1].Parent != body[0] || body[2].Parent != body[0] {
			t.Fatal("expandInclusion reused an instance or lost boundary")
		}
	}
	sequence, problems := expandSequence(input, []int{1, 2}, 0, nil, refs)
	if problems != nil {
		t.Fatal(problems)
	}
	task18Sources(t, input, sequence, []int{1, 2})
	task18Sources(t, input, sequence[0].Included, []int{4, 5, 6})
	// Все номера существуют; намеренно отсутствует локальная копия родителя 4.
	broken := *refs[1]
	broken.Body = []int{5}
	bad, problems := expandInclusion(input, entries[0], &broken, refs)
	if bad != nil || len(problems) == 0 {
		t.Fatalf("parent failure returned partial body: %v, %v", bad, problems)
	}
	if !slices.ContainsFunc(problems, func(e *diagnostic.Error) bool {
		return e.Code == CodeParent && e.Context.Path == input.Path && e.Context.Line == 5 && slices.ContainsFunc(e.Related, func(c diagnostic.Context) bool {
			return c.Path == input.Path && c.LessonID == "QA18" && c.PageID == "" && c.Line == 1 && c.Column == 10
		})
	}) {
		t.Fatalf("missing physical parent/include locations: %v", problems)
	}
}

func TestTask18ExpandNoSmallDepthOrCountLimit(t *testing.T) {
	const depth = 1024
	lines := []model.Line{task18Line(0, "tag", "include", "F0")}
	for i := 0; i < depth; i++ {
		header := len(lines) + 1
		child := task18Line(header, "tag", "include", "F"+strconv.Itoa(i+1))
		if i == depth-1 {
			child = task18Line(header, "content", "", "конец")
		}
		lines = append(lines, task18Line(0, "block-start", "fragment", "F"+strconv.Itoa(i)), child, task18Line(header, "block-end", "", ""))
	}
	out, problems := Expand(task18Load(t, lines...))
	if problems != nil || len(out) != 1 {
		t.Fatalf("deep expansion: %v", problems)
	}
	for i, site := 0, out[0]; i < depth; i++ {
		if len(site.Included) != 1 || site.Included[0].Parent != site || site.Included[0].Source.Number != 3+3*i {
			t.Fatalf("lost inclusion at depth %d", i)
		}
		site = site.Included[0]
	}
}
