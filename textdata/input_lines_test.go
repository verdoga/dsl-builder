package textdata

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func inputQABody(number int, kind model.LineType) model.Line {
	parent := 20
	line := model.Line{Number: number, LineType: kind, NestingLevel: 1,
		ParentLine: &parent, LineEnding: model.LineEndingCRLF, Elements: []model.Element{}}
	switch kind {
	case model.LineTypeContent:
		line.Raw = "  Привет🙂"
		value := "Привет🙂"
		line.Elements = append(line.Elements, inputQAElement(model.ElementTypeContent, value, &value, 3))
	case model.LineTypeSeparator:
		line.Raw = "\t--- "
	case model.LineTypeBlockEnd:
		line.Raw = "}"
		value := "}"
		line.Elements = append(line.Elements, inputQAElement(model.ElementTypeBlockClose, value, &value, 1))
	case model.LineTypeBlank:
		line.Raw = "\t "
	}
	return line
}

func inputQACheckLines(t *testing.T, kind lesson.NodeType, declaration *model.Line, lines []model.Line, wantError bool) {
	t.Helper()
	before := inputQASnapshot(t, declaration, lines)
	err := checkOwnLines(kind, declaration, lines, inputQAContext())
	if wantError {
		inputQAProblem(t, err, CodeInput)
	} else if err != nil {
		t.Fatalf("допустимая собственная форма: %v", err)
	}
	if before != inputQASnapshot(t, declaration, lines) {
		t.Fatal("локальный вход изменён")
	}
}

func TestInputQATitleOnlyHasNoOwnLines(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeTask, lesson.NodeStep,
		lesson.NodeVariants, lesson.NodeVariant, lesson.NodeHeading} {
		t.Run(string(kind), func(t *testing.T) {
			declaration := inputQADeclaration(inputQATag(kind), kind == lesson.NodeVariants)
			inputQACheckLines(t, kind, declaration, nil, false)
			inputQACheckLines(t, kind, declaration, []model.Line{}, false)
			for _, lineType := range []model.LineType{model.LineTypeContent, model.LineTypeBlank, model.LineTypeSeparator} {
				t.Run(string(lineType), func(t *testing.T) {
					inputQACheckLines(t, kind, declaration, []model.Line{inputQABody(80, lineType)}, true)
				})
			}
			inputQACheckLines(t, kind, declaration, []model.Line{*inputQADeclaration("note", false)}, true)
		})
	}
}

func TestInputQATextBodyLineTypes(t *testing.T) {
	kinds := []lesson.NodeType{lesson.NodeText, lesson.NodeInstruction, lesson.NodeNote,
		lesson.NodeAlt, lesson.NodeScript, lesson.NodeHint, lesson.NodeAnswerKey,
		lesson.NodeExample, lesson.NodeWordList, lesson.NodeTable, lesson.NodeMultifill}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			declaration := inputQADeclaration(inputQATag(kind), true)
			inputQACheckLines(t, kind, declaration, nil, false)
			for _, lineType := range []model.LineType{model.LineTypeContent, model.LineTypeBlank, model.LineTypeSeparator} {
				t.Run(string(lineType), func(t *testing.T) {
					wantError := lineType == model.LineTypeSeparator && kind != lesson.NodeExample && kind != lesson.NodeTable
					inputQACheckLines(t, kind, declaration, []model.Line{inputQABody(80, lineType)}, wantError)
				})
			}
			for _, tag := range []string{"note", "answer", "instr", "multifill", "endtask"} {
				inputQACheckLines(t, kind, declaration, []model.Line{*inputQADeclaration(tag, false)}, true)
			}
			for _, lineType := range []model.LineType{model.LineTypeHeading, model.LineTypeInvalid, "future"} {
				line := inputQABody(80, model.LineTypeContent)
				line.LineType = lineType
				inputQACheckLines(t, kind, declaration, []model.Line{line}, true)
			}
			if kind != lesson.NodeInstruction {
				inputQACheckLines(t, kind, declaration, []model.Line{*inputQADeclaration("hint", true)}, true)
				inputQACheckLines(t, kind, declaration, []model.Line{inputQABody(80, model.LineTypeBlockEnd)}, true)
			}
		})
	}
	body := []model.Line{inputQABody(80, model.LineTypeContent), inputQABody(95, model.LineTypeBlank)}
	inputQACheckLines(t, lesson.NodeText, nil, body, false)
	inputQACheckLines(t, lesson.NodeText, nil, []model.Line{*inputQADeclaration("hint", false)}, true)
	inputQACheckLines(t, lesson.NodeText, nil, []model.Line{inputQABody(80, model.LineTypeSeparator)}, true)
}

func TestInputQAInlineDeclarationsRejectAdditionalBody(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeText, lesson.NodeInstruction,
		lesson.NodeNote, lesson.NodeAlt, lesson.NodeScript, lesson.NodeHint,
		lesson.NodeAnswerKey, lesson.NodeExample, lesson.NodeWordList, lesson.NodeTable, lesson.NodeMultifill} {
		t.Run(string(kind), func(t *testing.T) {
			declaration := inputQADeclaration(inputQATag(kind), false)
			inputQACheckLines(t, kind, declaration, []model.Line{}, false)
			for _, lineType := range []model.LineType{model.LineTypeContent, model.LineTypeBlank, model.LineTypeSeparator} {
				inputQACheckLines(t, kind, declaration, []model.Line{inputQABody(80, lineType)}, true)
			}
		})
	}
}

func TestInputQAExtractedInstructionsNeverReceiveExerciseBody(t *testing.T) {
	for _, tag := range []string{"multifill", "ordering"} {
		for _, block := range []bool{false, true} {
			t.Run(tag+"/block="+strconv.FormatBool(block), func(t *testing.T) {
				declaration := inputQADeclaration(tag, block)
				inputQACheckLines(t, lesson.NodeInstruction, declaration, nil, false)
				for _, lineType := range []model.LineType{model.LineTypeContent, model.LineTypeBlank, model.LineTypeSeparator} {
					inputQACheckLines(t, lesson.NodeInstruction, declaration, []model.Line{inputQABody(80, lineType)}, true)
				}
			})
		}
	}
}

func TestInputQAInstructionAcceptsInlineAndBlockHintForms(t *testing.T) {
	declaration := inputQADeclaration("instr", true)
	before, after := inputQABody(21, model.LineTypeContent), inputQABody(27, model.LineTypeContent)
	for _, block := range []bool{false, true} {
		t.Run(strconv.FormatBool(block), func(t *testing.T) {
			hint := inputQADeclaration("hint", block)
			hint.Number, hint.ParentLine, hint.NestingLevel = 22, &declaration.Number, 1
			lines := []model.Line{before, *hint}
			if block {
				body := inputQABody(23, model.LineTypeContent)
				blank := inputQABody(24, model.LineTypeBlank)
				close := inputQABody(25, model.LineTypeBlockEnd)
				body.ParentLine, blank.ParentLine, close.ParentLine = &hint.Number, &hint.Number, &hint.Number
				body.NestingLevel, blank.NestingLevel, close.NestingLevel = 2, 2, 2
				lines = append(lines, body, blank, close)
			}
			lines = append(lines, after)
			inputQACheckLines(t, lesson.NodeInstruction, declaration, lines, false)
		})
	}
	// Принадлежность обычного текста уже определена вызывающим конвейером.
	ancestor := 7
	before.ParentLine, before.NestingLevel = &ancestor, 2
	inputQACheckLines(t, lesson.NodeInstruction, declaration, []model.Line{before, after}, false)
}

func TestInputQAGoldenLocalSlices(t *testing.T) {
	cases := []struct {
		golden, declaration int
		kind                lesson.NodeType
		own                 []int
	}{
		{1, 7, lesson.NodeTask, nil}, {1, 8, lesson.NodeMultifill, []int{9, 10, 11, 12, 13, 14, 15, 16, 17, 18}},
		{1, 8, lesson.NodeInstruction, nil}, {1, 21, lesson.NodeAnswerKey, []int{22, 23, 24, 25, 26, 27, 28, 29, 30, 31}},
		{2, 7, lesson.NodeInstruction, []int{8, 9}}, {2, 11, lesson.NodeExample, nil},
		{2, 12, lesson.NodeAlt, nil}, {2, 13, lesson.NodeMultifill, nil},
		{2, 16, lesson.NodeNote, []int{17, 18}}, {3, 5, lesson.NodeHeading, nil},
		{3, 8, lesson.NodeInstruction, []int{9, 10, 11}}, {3, 13, lesson.NodeExample, []int{14, 15, 16}},
		{3, 35, lesson.NodeStep, nil}, {3, 37, lesson.NodeScript, []int{38, 39}},
		{3, 43, lesson.NodeWordList, nil}, {3, 44, lesson.NodeInstruction, nil},
		{4, 6, lesson.NodeWordList, []int{7, 8, 9}}, {4, 14, lesson.NodeTable, []int{15, 16, 17, 18}},
		{4, 23, lesson.NodeText, []int{24, 25, 26}}, {5, 16, lesson.NodeMultifill, []int{17, 18}},
		{6, 11, lesson.NodeHint, nil}, {6, 31, lesson.NodeHint, []int{32, 33}},
		{7, 5, lesson.NodeVariants, nil}, {7, 6, lesson.NodeVariant, nil},
		{7, 9, lesson.NodeInstruction, nil},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.golden)+"/"+strconv.Itoa(tc.declaration)+"/"+string(tc.kind), func(t *testing.T) {
			raw, err := os.ReadFile("../golden/" + strconv.Itoa(tc.golden) + ". parsed.json")
			if err != nil {
				t.Fatal(err)
			}
			var result model.Result
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			declaration := result.Lines[tc.declaration-1]
			lines := make([]model.Line, 0, len(tc.own))
			for _, number := range tc.own {
				lines = append(lines, result.Lines[number-1])
			}
			if err := checkDeclaration(tc.kind, &declaration, inputQAContext()); err != nil {
				t.Fatal(err)
			}
			inputQACheckLines(t, tc.kind, &declaration, lines, false)
		})
	}
}
