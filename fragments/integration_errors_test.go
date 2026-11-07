package fragments_test

import (
	"slices"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task20Problem(t *testing.T, input *parsed.Input, code, name string) *diagnostic.Error {
	t.Helper()
	before := task20JSON(t, input)
	entries, issues := fragments.Expand(input)
	if entries != nil || len(issues) == 0 {
		t.Fatalf("blocking failure must return nil output: entries=%v, issues=%v", entries, issues)
	}
	if before != task20JSON(t, input) {
		t.Fatal("failed expansion mutated its checked Input")
	}
	for _, issue := range issues {
		if issue == nil {
			t.Fatal("nil diagnostic")
		}
		if issue.Code != code {
			continue
		}
		if issue.Context.Path != input.Path || issue.Context.LessonID != "QA20" || issue.Context.PageID != "" {
			t.Fatalf("diagnostic lost physical input context: %+v", issue)
		}
		russian := false
		for _, r := range issue.Reason {
			russian = russian || unicode.Is(unicode.Cyrillic, r)
		}
		if !russian {
			t.Fatalf("Reason must explain failure in Russian: %+v", issue)
		}
		if name != "" {
			found := false
			for _, detail := range issue.Details {
				found = found || detail.Value == name
			}
			if !found {
				t.Fatalf("Details lost original fragment name %q: %+v", name, issue)
			}
		}
		return issue
	}
	t.Fatalf("missing diagnostic code %s: %v", code, issues)
	return nil
}

func task20Related(t *testing.T, input *parsed.Input, issue *diagnostic.Error, lines ...int) {
	t.Helper()
	var got []int
	for _, context := range issue.Related {
		if context.Path != input.Path || context.LessonID != "QA20" || context.PageID != "" {
			t.Fatalf("related source context is incorrect: %+v", context)
		}
		got = append(got, context.Line)
	}
	for _, line := range lines {
		if !slices.Contains(got, line) {
			t.Fatalf("Related=%v lacks participant line %d", got, line)
		}
	}
}

func TestTask20LoadExpandMissingUsesPhysicalUnicodeCoordinates(t *testing.T) {
	line := task20Line(1, 0, 0, model.LineTypeTag, "Ж @INCLUDE  Нет",
		task20Element(model.ElementTypeTag, "@INCLUDE", "include", 3),
		task20Element(model.ElementTypeIdentifier, "Нет", "Нет", 13))
	input := task20Load(t, task20Document(line))
	issue := task20Problem(t, input, fragments.CodeMissing, "Нет")
	if issue.Context.Line != 1 || issue.Context.Column != 13 {
		t.Fatalf("expected physical Unicode 1:13, got %+v", issue.Context)
	}
}

func TestTask20LoadExpandDoesNotReuseDefinitionsAcrossDocuments(t *testing.T) {
	first := task20Load(t, task20Document(task20Tag(1, 0, 0, "include", "Local", false),
		task20Tag(2, 0, 0, "fragment", "Local", true), task20Content(3, 2, 1, "local", "local"), task20End(4, 2, 1)))
	_ = task20Expand(t, first)
	second := task20Load(t, task20Document(task20Tag(1, 0, 0, "include", "Local", false)))
	task20Problem(t, second, fragments.CodeMissing, "Local")
	task20Numbers(t, task20Expand(t, first)[0].Included, 3)
}

func TestTask20LoadExpandBlocksUnresolvedAndAmbiguousUnusedMaterial(t *testing.T) {
	cases := []struct {
		name, code, identifier string
		lines                  []model.Line
		related                []int
	}{
		{"unused reference", fragments.CodeMissing, "Lost", []model.Line{
			task20Tag(1, 0, 0, "fragment", "Unused", true), task20Tag(2, 1, 1, "include", "Lost", false), task20End(3, 1, 1)}, nil},
		{"unused duplicate", fragments.CodeAmbiguous, "Name", []model.Line{
			task20Tag(1, 0, 0, "fragment", "Name", true), task20End(2, 1, 1),
			task20Tag(3, 0, 0, "fragment", "NAME", true), task20End(4, 3, 1)}, []int{1, 3}},
		{"ambiguous included", fragments.CodeAmbiguous, "Name", []model.Line{
			task20Tag(1, 0, 0, "include", "Name", false), task20Tag(2, 0, 0, "fragment", "Name", true), task20End(3, 2, 1),
			task20Tag(4, 0, 0, "fragment", "Name", true), task20End(5, 4, 1)}, []int{2, 4}},
		{"no trimming", fragments.CodeMissing, "Name ", []model.Line{
			task20Tag(1, 0, 0, "include", "Name ", false), task20Tag(2, 0, 0, "fragment", "Name", true), task20End(3, 2, 1)}, nil},
		{"no Unicode normalization", fragments.CodeMissing, "e\u0301", []model.Line{
			task20Tag(1, 0, 0, "include", "e\u0301", false), task20Tag(2, 0, 0, "fragment", "é", true), task20End(3, 2, 1)}, nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := task20Load(t, task20Document(test.lines...))
			issue := task20Problem(t, input, test.code, test.identifier)
			task20Related(t, input, issue, test.related...)
		})
	}
}

func TestTask20LoadExpandBlocksCyclesEvenWhenUnused(t *testing.T) {
	for _, indirect := range []bool{false, true} {
		name := "A"
		if indirect {
			name = "B"
		}
		lines := []model.Line{task20Tag(1, 0, 0, "fragment", "A", true), task20Tag(2, 1, 1, "include", name, false), task20End(3, 1, 1)}
		participants := []int{1, 2}
		if indirect {
			lines = append(lines, task20Tag(4, 0, 0, "fragment", "B", true), task20Tag(5, 4, 1, "include", "A", false), task20End(6, 4, 1))
			participants = append(participants, 4, 5)
		}
		input := task20Load(t, task20Document(lines...))
		issue := task20Problem(t, input, fragments.CodeCycle, "")
		task20Related(t, input, issue, participants...)
	}
}

func TestTask20LoadExpandRejectsFragmentShapesAcceptedByContract(t *testing.T) {
	missingID := task20Tag(1, 0, 0, "include", "", false)
	emptyID := task20Line(1, 0, 0, model.LineTypeTag, "@include ",
		task20Element(model.ElementTypeTag, "@include", "include", 1), task20Element(model.ElementTypeIdentifier, "", "", 10))
	duplicateID := task20Line(1, 0, 0, model.LineTypeTag, "@include A B",
		task20Element(model.ElementTypeTag, "@include", "include", 1),
		task20Element(model.ElementTypeIdentifier, "A", "A", 10), task20Element(model.ElementTypeIdentifier, "B", "B", 12))
	cases := []struct {
		name  string
		lines []model.Line
	}{
		{"include missing identifier", []model.Line{missingID}},
		{"include empty identifier", []model.Line{emptyID}},
		{"include multiple identifiers", []model.Line{duplicateID}},
		{"fragment missing identifier", []model.Line{task20Tag(1, 0, 0, "fragment", "", true), task20End(2, 1, 1)}},
		{"fragment wrong line type", []model.Line{task20Tag(1, 0, 0, "fragment", "A", false)}},
		{"include wrong line type", []model.Line{task20Tag(1, 0, 0, "include", "A", true), task20End(2, 1, 1)}},
		{"missing closing boundary", []model.Line{task20Tag(1, 0, 0, "fragment", "A", true), task20Content(2, 1, 1, "body", "body")}},
		{"duplicate closing boundary", []model.Line{task20Tag(1, 0, 0, "fragment", "A", true), task20End(2, 1, 1), task20End(3, 1, 1)}},
		{"descendant after closing boundary", []model.Line{task20Tag(1, 0, 0, "fragment", "A", true), task20End(2, 1, 1), task20Content(3, 1, 1, "late", "late")}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := task20Load(t, task20Document(test.lines...))
			task20Problem(t, input, fragments.CodeShape, "")
		})
	}
}

func TestTask20LoadExpandDuplicateGroupsHavePhysicalOrder(t *testing.T) {
	input := task20Load(t, task20Document(
		task20Tag(1, 0, 0, "fragment", "Zulu", true), task20End(2, 1, 1),
		task20Tag(3, 0, 0, "fragment", "zulu", true), task20End(4, 3, 1),
		task20Tag(5, 0, 0, "fragment", "Alpha", true), task20End(6, 5, 1),
		task20Tag(7, 0, 0, "fragment", "alpha", true), task20End(8, 7, 1)))
	entries, issues := fragments.Expand(input)
	if entries != nil || len(issues) != 2 {
		t.Fatalf("one error per duplicate group required: %v, %v", entries, issues)
	}
	for i, group := range [][]int{{1, 3}, {5, 7}} {
		if issues[i] == nil || issues[i].Code != fragments.CodeAmbiguous {
			t.Fatalf("incorrect duplicate diagnostic: %v", issues)
		}
		task20Related(t, input, issues[i], group...)
		if len(issues[i].Related) != 2 || issues[i].Related[0].Line != group[0] || issues[i].Related[1].Line != group[1] {
			t.Fatalf("duplicate groups or candidates reordered: %+v", issues[i])
		}
	}
}
