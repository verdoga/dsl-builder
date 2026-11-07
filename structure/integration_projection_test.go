package structure_test

import (
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/textdata"
)

func TestTask26WholeLeafBlocksAndOpaqueEditor(t *testing.T) {
	pages, bindings := task26Build(t, []model.Line{
		task26Tag(1, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "A")),
		task26Tag(2, 1, model.LineTypeBlockStart, "editor"),
		task26Tag(3, 2, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "must stay opaque")),
		task26Tag(4, 2, model.LineTypeTag, "endtask"),
		task26End(5, 2),
		task26Tag(6, 1, model.LineTypeBlockStart, "text", task26Element(model.ElementTypeTitle, "Title")),
		task26Text(7, 6, "<b> literal \\ and & </b>"),
		task26Line(8, 6, model.LineTypeBlank),
		task26Text(9, 6, "second"),
		task26End(10, 6),
		task26Tag(11, 1, model.LineTypeTag, "media", task26Element(model.ElementTypeMediaType, "audio"), task26Element(model.ElementTypeSource, "clip")),
		task26Tag(12, 1, model.LineTypeBlockStart, "script"),
		task26Text(13, 12, "transcript"),
		task26End(14, 12),
		task26Tag(15, 1, model.LineTypeTag, "hint", task26Element(model.ElementTypeContent, "separate")),
		task26Text(16, 1, "standalone"),
	}, "[task(text,media,script,hint,text)]")
	text := task26Binding(t, bindings, 6, lesson.NodeText, pages)
	if !slices.Equal(task26Numbers(text), []int{6, 7, 8, 9, 10}) {
		t.Fatalf("leaf projection=%v", task26Numbers(text))
	}
	declaration, lines := text.TextSource()
	if declaration == nil || declaration.Number != 6 || len(lines) != 3 || lines[1].LineType != model.LineTypeBlank {
		t.Fatal("TextSource did not retain own body")
	}
	data, problem := textdata.Build(lesson.NodeText, declaration, lines, diagnostic.Context{})
	if problem != nil {
		t.Fatal(problem)
	}
	if len(data) != 2 || data[1].Value != "<b> literal \\ and & </b>\nsecond" {
		t.Fatalf("real textdata result=%+v", data)
	}
	standalone := task26Binding(t, bindings, 16, lesson.NodeText, pages)
	declaration, lines = standalone.TextSource()
	if declaration != nil || len(lines) != 1 || lines[0].Number != 16 {
		t.Fatal("standalone content was treated as declaration")
	}
	if _, problem := textdata.Build(lesson.NodeText, declaration, lines, diagnostic.Context{}); problem != nil {
		t.Fatal(problem)
	}
}

func TestTask26InstructionRetainsNestedHintAndOwnLastBoundary(t *testing.T) {
	pages, bindings := task26Build(t, []model.Line{
		task26Tag(1, 0, model.LineTypeBlockStart, "instr"),
		task26Text(2, 1, "first"),
		task26Tag(3, 1, model.LineTypeBlockStart, "hint"),
		task26Text(4, 3, "подсказка"),
		task26End(5, 3),
		task26Text(6, 1, "last"),
		task26End(7, 1),
		task26Tag(8, 0, model.LineTypeTag, "hint", task26Element(model.ElementTypeContent, "independent")),
	}, "[instruction,hint]")
	binding := task26Binding(t, bindings, 1, lesson.NodeInstruction, pages)
	declaration, lines := binding.TextSource()
	if declaration == nil || len(lines) != 5 || lines[3].LineType != model.LineTypeBlockEnd || lines[3].Number != 5 {
		t.Fatal("nested hint boundary lost")
	}
	data, problem := textdata.Build(lesson.NodeInstruction, declaration, lines, diagnostic.Context{})
	if problem != nil {
		t.Fatal(problem)
	}
	if len(data) != 1 || data[0].Value != "first\nlast" || data[0].Hint != "подсказка" {
		t.Fatalf("data=%+v", data)
	}
}

func TestTask26InstructionPairsAndInlineExerciseInstructionsUseRealConsumers(t *testing.T) {
	pages, bindings := task26Build(t, []model.Line{
		task26Tag(1, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "A")),
		task26Tag(2, 1, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "stage")),
		task26Tag(3, 1, model.LineTypeTag, "instr", task26Element(model.ElementTypeContent, "same")),
		task26Tag(4, 1, model.LineTypeBlockStart, "multifill", task26Element(model.ElementTypeContent, "same")),
		task26Text(5, 4, "I _____{am}"), task26Text(6, 4, ""), task26End(7, 4),
		task26Tag(8, 1, model.LineTypeBlockStart, "ordering", task26Element(model.ElementTypeContent, "same")),
		task26Text(9, 8, "A"), task26Text(10, 8, "B"), task26End(11, 8),
		task26Tag(12, 1, model.LineTypeBlockStart, "choice", task26Element(model.ElementTypeContent, "same")),
		task26Text(13, 12, "A"), task26Tag(14, 12, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "B")),
		task26Tag(15, 12, model.LineTypeTag, "hint", task26Element(model.ElementTypeContent, "hint")), task26End(16, 12),
		task26Tag(17, 1, model.LineTypeBlockStart, "multichoice", task26Element(model.ElementTypeContent, "same")),
		task26Tag(18, 17, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "A")),
		task26Tag(19, 17, model.LineTypeTag, "answer", task26Element(model.ElementTypeContent, "B")), task26End(20, 17),
		task26Tag(21, 1, model.LineTypeBlockStart, "matching", task26Element(model.ElementTypeContent, "same")),
		task26Text(22, 21, "item"), task26Line(23, 21, model.LineTypeSeparator, task26Element(model.ElementTypeContent, "---")),
		task26Text(24, 21, "A"), task26Text(25, 21, "B"), task26End(26, 21),
	}, "[task(step(instruction,instruction,multifill,instruction,ordering,singleChoice,multipleChoice,matching))]")
	for _, binding := range bindings {
		node := binding.Node(pages)
		switch node.Type {
		case lesson.NodeInstruction, lesson.NodeMultifill:
			declaration, lines := binding.TextSource()
			data, problem := textdata.Build(node.Type, declaration, lines, diagnostic.Context{})
			if problem != nil {
				t.Fatalf("%s: %v", node.Type, problem)
			}
			if node.Type == lesson.NodeInstruction {
				if len(data) != 1 || data[0].Value != "same" {
					t.Fatalf("instruction data=%+v", data)
				}
				if declaration.Number == 4 || declaration.Number == 8 {
					if len(binding.Entries) != 1 || len(lines) != 0 {
						t.Fatal("synthetic instruction consumed exercise body")
					}
				}
			} else if len(data) != 1 || data[0].Value != "I _____{am}\n" || len(data[0].Answers) != 0 {
				t.Fatalf("multifill was interpreted: %+v", data)
			}
		case lesson.NodeOrdering, lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching:
			data, _, problem := exercisedata.Build(node.Type, binding.Lines(), diagnostic.Context{})
			if problem != nil {
				t.Fatalf("%s: %v", node.Type, problem)
			}
			wantInline := node.Type != lesson.NodeOrdering
			gotInline := len(data) > 0 && data[0].Role != nil && *data[0].Role == lesson.RoleInstruction
			if gotInline != wantInline {
				t.Fatalf("%s inline instruction=%v", node.Type, gotInline)
			}
			if gotInline && data[0].Value != "same" {
				t.Fatal("inline value lost")
			}
		}
	}
}

func TestTask26QuestionTailFullBlocksAndUnrelatedHint(t *testing.T) {
	pages, bindings := task26Build(t, []model.Line{
		task26Tag(1, 0, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "Q")),
		task26Line(2, 0, model.LineTypeBlank),
		task26Tag(3, 0, model.LineTypeBlockStart, "editor"), task26Text(4, 3, "ignored"), task26End(5, 3),
		task26Tag(6, 0, model.LineTypeBlockStart, "answer"), task26Text(7, 6, "A"), task26Text(8, 6, "B"), task26End(9, 6),
		task26Tag(10, 0, model.LineTypeBlockStart, "hint"), task26Text(11, 10, "H"), task26End(12, 10),
		task26Tag(13, 0, model.LineTypeTag, "hint", task26Element(model.ElementTypeContent, "separate")),
		task26Tag(14, 0, model.LineTypeTag, "question", task26Element(model.ElementTypeContent, "without answer")),
		task26Tag(15, 0, model.LineTypeTag, "hint", task26Element(model.ElementTypeContent, "also separate")),
	}, "[question,hint,question,hint]")
	binding := task26Binding(t, bindings, 1, lesson.NodeQuestion, pages)
	numbers := task26Numbers(binding)
	for _, number := range []int{1, 6, 7, 8, 9, 10, 11, 12} {
		if !slices.Contains(numbers, number) {
			t.Fatalf("missing line %d from %v", number, numbers)
		}
	}
	if slices.Contains(numbers, 13) {
		t.Fatal("second hint swallowed")
	}
	data, checkable, problem := exercisedata.Build(lesson.NodeQuestion, binding.Lines(), diagnostic.Context{})
	if problem != nil {
		t.Fatal(problem)
	}
	if !checkable || len(data) != 1 || data[0].Value != "Q" || !slices.Equal(data[0].Answers, []string{"A", "B"}) || data[0].Hint != "H" {
		t.Fatalf("question data=%+v checkable=%v", data, checkable)
	}
}

func TestTask26EmptyDeclaredInstructionAndNullablePlaceholder(t *testing.T) {
	placeholder := task26Element(model.ElementTypePlaceholder, "_____")
	placeholder.Value = nil
	task26Build(t, []model.Line{
		task26Tag(1, 0, model.LineTypeTag, "multifill", task26Element(model.ElementTypeContent, "")),
		task26Tag(2, 0, model.LineTypeTag, "multifill", placeholder),
	}, "[multifill,instruction,multifill]")
}

func TestTask26SemanticValuesAndRawReachConsumersWithoutReescaping(t *testing.T) {
	element := task26Element(model.ElementTypeContent, `\@literal & <b>текст</b>`)
	element.Raw = `\\@literal & <b>текст</b>`
	pages, bindings := task26Build(t, []model.Line{task26Line(1, 0, model.LineTypeContent, element)}, "[text]")
	binding := task26Binding(t, bindings, 1, lesson.NodeText, pages)
	declaration, lines := binding.TextSource()
	if lines[0].Elements[0].Raw != element.Raw || *lines[0].Elements[0].Value != *element.Value {
		t.Fatal("source encoding changed")
	}
	data, problem := textdata.Build(lesson.NodeText, declaration, lines, diagnostic.Context{})
	if problem != nil {
		t.Fatal(problem)
	}
	if len(data) != 1 || data[0].Value != *element.Value {
		t.Fatalf("semantic Value escaped again: %+v", data)
	}
}
