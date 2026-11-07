package structure_test

import (
	"slices"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/structure"
)

func TestTask26AtomicStructuralErrorsAfterRealLoadExpand(t *testing.T) {
	cases := []struct {
		name, code string
		lines      []model.Line
		line       int
	}{
		{"step without task", structure.CodeContext, []model.Line{task26Text(1, 0, "valid prefix"), task26Tag(2, 0, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "orphan"))}, 2},
		{"variant without set", structure.CodeContext, []model.Line{task26Text(1, 0, "valid prefix"), task26Tag(2, 0, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "orphan"))}, 2},
		{"speaking without task", structure.CodeContext, []model.Line{task26Text(1, 0, "valid prefix"), task26Tag(2, 0, model.LineTypeTag, "speaking")}, 2},
		{"endtask without task", structure.CodeContext, []model.Line{task26Text(1, 0, "valid prefix"), task26Tag(2, 0, model.LineTypeTag, "endtask")}, 2},
		{"orphan answer", structure.CodeContext, []model.Line{task26Text(1, 0, "valid prefix"), task26Tag(2, 0, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A"))}, 2},
		{"unfinished leaf", structure.CodeBoundary, []model.Line{task26Text(1, 0, "valid prefix"), task26Tag(2, 0, model.LineTypeBlockStart, "text"), task26Text(3, 2, "body")}, 2},
		{"missing page title", structure.CodeSource, []model.Line{task26Text(1, 0, "valid prefix"), task26Line(2, 0, model.LineTypeHeading, model.Element{ElementType: model.ElementTypeHeadingLevel, Raw: "###", Value: task26String("3"), ErrorIDs: []string{}})}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := task26Load(t, tc.lines)
			entries := task26Expand(t, input)
			pages, bindings, problems := structure.Build(input, entries)
			if pages != nil || bindings != nil || len(problems) == 0 {
				t.Fatalf("non-atomic failure: pages=%v bindings=%v errors=%v", pages, bindings, problems)
			}
			first := problems[0]
			if first.Code != tc.code || first.Context.Path != input.Path || first.Context.LessonID != "QA26" || first.Context.Line != tc.line || first.Context.PageID != "" {
				t.Fatalf("wrong diagnostic: %+v", first)
			}
			if first.Context.Column < 0 || first.Context.Column > len([]rune(input.Result.Lines[tc.line-1].Raw))+1 {
				t.Fatalf("invented column: %+v", first.Context)
			}
			russian := false
			for _, r := range first.Reason {
				if unicode.In(r, unicode.Cyrillic) {
					russian = true
				}
			}
			if !russian {
				t.Fatalf("non-Russian reason %q", first.Reason)
			}
		})
	}
}

func TestTask26InnerBranchMustOwnItsTaskAndVariant(t *testing.T) {
	for _, tag := range []string{"step", "speaking", "endtask", "text"} {
		t.Run(tag, func(t *testing.T) {
			lines := []model.Line{
				task26Tag(1, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "outer")),
				task26Tag(2, 1, model.LineTypeBlockStart, "variants"),
				task26Tag(3, 2, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "outer branch")),
				task26Tag(4, 2, model.LineTypeBlockStart, "variants"),
			}
			// Снаружи есть variant и task; внутренний набор ещё не имеет своей ветви.
			if tag == "text" {
				lines = append(lines, task26Text(5, 4, "not in an inner branch"))
			} else {
				lines = append(lines, task26Tag(5, 4, model.LineTypeTag, tag, task26Element(model.ElementTypeTitle, "orphan")))
			}
			lines = append(lines, task26End(6, 4), task26End(7, 2))
			input := task26Load(t, lines)
			pages, bindings, problems := structure.Build(input, task26Expand(t, input))
			if pages != nil || bindings != nil || len(problems) == 0 || problems[0].Code != structure.CodeContext || problems[0].Context.Line != 5 {
				t.Fatalf("outer owner crossed inner barrier: %+v", problems)
			}
		})
	}
	for _, tag := range []string{"step", "speaking", "endtask"} {
		t.Run("own branch no task/"+tag, func(t *testing.T) {
			input := task26Load(t, []model.Line{
				task26Tag(1, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "outer")),
				task26Tag(2, 1, model.LineTypeBlockStart, "variants"),
				task26Tag(3, 2, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "branch")),
				task26Tag(4, 2, model.LineTypeTag, tag, task26Element(model.ElementTypeTitle, "orphan")), task26End(5, 2),
			})
			pages, bindings, problems := structure.Build(input, task26Expand(t, input))
			if pages != nil || bindings != nil || len(problems) == 0 || problems[0].Code != structure.CodeContext || problems[0].Context.Line != 4 {
				t.Fatalf("outer task crossed branch barrier: %+v", problems)
			}
		})
	}
}

func TestTask26NestedIncludeDiagnosticsKeepFullOccurrenceChain(t *testing.T) {
	input := task26Load(t, []model.Line{
		task26Tag(1, 0, model.LineTypeBlockStart, "fragment", task26Element(model.ElementTypeIdentifier, "outer")),
		task26Tag(2, 1, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "inner")), task26End(3, 1),
		task26Tag(4, 0, model.LineTypeBlockStart, "fragment", task26Element(model.ElementTypeIdentifier, "inner")),
		task26Tag(5, 4, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "orphan")), task26End(6, 4),
		task26Text(7, 0, "valid prefix"),
		task26Tag(8, 0, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "outer")),
	})
	pages, bindings, problems := structure.Build(input, task26Expand(t, input))
	if pages != nil || bindings != nil || len(problems) == 0 {
		t.Fatal("include error returned partial draft")
	}
	problem := problems[0]
	if problem.Code != structure.CodeContext || problem.Context.Path != input.Path || problem.Context.LessonID != "QA26" || problem.Context.Line != 5 || problem.Context.PageID != "" {
		t.Fatalf("wrong original coordinate: %+v", problem)
	}
	related := []int{}
	for _, place := range problem.Related {
		if place.Path != input.Path || place.LessonID != "QA26" || place.PageID != "" {
			t.Fatalf("lost include source: %+v", place)
		}
		related = append(related, place.Line)
	}
	if !slices.Contains(related, 2) || !slices.Contains(related, 8) {
		t.Fatalf("incomplete include chain: %v", related)
	}
}

func TestTask26QuestionAnswerCannotCrossIncludeOrSemanticBoundary(t *testing.T) {
	cases := []struct {
		name  string
		lines []model.Line
	}{
		{"step", []model.Line{
			task26Tag(1, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "A")),
			task26Tag(2, 1, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "first")),
			task26Tag(3, 1, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")),
			task26Tag(4, 1, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "next")),
			task26Tag(5, 1, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")),
		}},
		{"variant", []model.Line{
			task26Tag(1, 0, model.LineTypeBlockStart, "variants"),
			task26Tag(2, 1, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "A")),
			task26Tag(3, 1, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")),
			task26Tag(4, 1, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "B")),
			task26Tag(5, 1, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")), task26End(6, 1),
		}},
		{"block-end", []model.Line{
			task26Tag(1, 0, model.LineTypeBlockStart, "variants"),
			task26Tag(2, 1, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "A")),
			task26Tag(3, 1, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")), task26End(4, 1),
			task26Tag(5, 0, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")),
		}},
		{"into include", []model.Line{
			task26Tag(1, 0, model.LineTypeBlockStart, "fragment", task26Element(model.ElementTypeIdentifier, "answer")),
			task26Tag(2, 1, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")), task26End(3, 1),
			task26Tag(4, 0, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")),
			task26Tag(5, 0, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "answer")),
		}},
		{"out of include", []model.Line{
			task26Tag(1, 0, model.LineTypeBlockStart, "fragment", task26Element(model.ElementTypeIdentifier, "question")),
			task26Tag(2, 1, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")), task26End(3, 1),
			task26Tag(4, 0, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "question")),
			task26Tag(5, 0, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")),
		}},
		{"include", []model.Line{
			task26Tag(1, 0, model.LineTypeBlockStart, "fragment", task26Element(model.ElementTypeIdentifier, "empty")), task26End(2, 1),
			task26Tag(3, 0, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")),
			task26Tag(4, 0, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "empty")),
			task26Tag(5, 0, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")),
		}},
		{"task", []model.Line{
			task26Tag(1, 0, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")),
			task26Tag(2, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "A")),
			task26Tag(3, 2, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")),
		}},
		{"page", []model.Line{
			task26Tag(1, 0, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")), task26Heading(2, "3", "page"),
			task26Tag(3, 0, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := task26Load(t, tc.lines)
			pages, bindings, problems := structure.Build(input, task26Expand(t, input))
			if pages != nil || bindings != nil || len(problems) == 0 || problems[0].Code != structure.CodeContext {
				t.Fatalf("tail crossed %s: %+v", tc.name, problems)
			}
		})
	}
}
