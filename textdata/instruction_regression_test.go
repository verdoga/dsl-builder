package textdata_test

import (
	"encoding/json"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/textdata"
)

func instructionRegressionQAString(value string) *string { return &value }

func instructionRegressionQAElement(kind model.ElementType, raw, value string, start int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: instructionRegressionQAString(value),
		Start: start, End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}}
}

func instructionRegressionQADeclaration(tag string, block bool, content *string) model.Line {
	line := model.Line{Number: 10, LineType: model.LineTypeTag, Raw: "@" + tag,
		LineEnding: model.LineEndingLF, Elements: []model.Element{
			instructionRegressionQAElement(model.ElementTypeTag, "@"+tag, tag, 1),
		}}
	if content != nil {
		start := utf8.RuneCountInString(line.Raw) + 2
		line.Raw += " " + *content
		line.Elements = append(line.Elements, instructionRegressionQAElement(model.ElementTypeContent, *content, *content, start))
	}
	if block {
		line.LineType = model.LineTypeBlockStart
		start := utf8.RuneCountInString(line.Raw) + 2
		line.Raw += " {"
		line.Elements = append(line.Elements, instructionRegressionQAElement(model.ElementTypeBlockOpen, "{", "{", start))
	}
	return line
}

func instructionRegressionQAJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func instructionRegressionQAAssert(t *testing.T, got []lesson.Data, body bool, value, hint string) {
	t.Helper()
	if got == nil {
		t.Fatal("successful data must not be nil")
	}
	if !body {
		if len(got) != 0 || instructionRegressionQAJSON(t, got) != "[]" {
			t.Fatalf("absent instruction data = %#v, want []", got)
		}
		return
	}
	if len(got) != 1 {
		t.Fatalf("instruction data = %#v, want one body", got)
	}
	datum := got[0]
	text, isString := datum.Value.(string)
	if datum.Type != lesson.DataText || datum.Role == nil || *datum.Role != lesson.RoleBody || !isString || text != value || datum.Hint != hint {
		t.Fatalf("body = %#v, want text %q, hint %q", datum, value, hint)
	}
	if datum.Answers == nil || len(datum.Answers) != 0 {
		t.Fatalf("answers = %#v, want non-nil []", datum.Answers)
	}
	var shapes []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(instructionRegressionQAJSON(t, got)), &shapes); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"type": `"text"`, "role": `"body"`, "value": instructionRegressionQAJSON(t, value), "answers": "[]", "hint": instructionRegressionQAJSON(t, hint)}
	if len(shapes[0]) != len(want) {
		t.Fatalf("unexpected JSON fields: %s", instructionRegressionQAJSON(t, got))
	}
	for key, expected := range want {
		if string(shapes[0][key]) != expected {
			t.Errorf("JSON %s = %s, want %s", key, shapes[0][key], expected)
		}
	}
}

func TestInstructionRegressionQABuildContentPresence(t *testing.T) {
	for _, tc := range []struct {
		name, tag string
		block     bool
		content   *string
		body      bool
		value     string
	}{
		{"standalone/present-empty", "instr", false, instructionRegressionQAString(""), true, ""},
		{"standalone/absent", "instr", false, nil, false, ""},
		{"standalone/present-text", "instr", false, instructionRegressionQAString("Текст"), true, "Текст"},
		{"standalone/empty-block", "instr", true, nil, false, ""},
		{"multifill/inline-absent", "multifill", false, nil, false, ""},
		{"multifill/inline-empty", "multifill", false, instructionRegressionQAString(""), false, ""},
		{"multifill/block-absent", "multifill", true, nil, false, ""},
		{"multifill/block-empty", "multifill", true, instructionRegressionQAString(""), false, ""},
		{"multifill/present-text", "multifill", true, instructionRegressionQAString("Текст"), true, "Текст"},
		{"ordering/inline-absent", "ordering", false, nil, false, ""},
		{"ordering/inline-empty", "ordering", false, instructionRegressionQAString(""), false, ""},
		{"ordering/block-absent", "ordering", true, nil, false, ""},
		{"ordering/block-empty", "ordering", true, instructionRegressionQAString(""), false, ""},
		{"ordering/present-text", "ordering", true, instructionRegressionQAString("Текст"), true, "Текст"},
		{"matching/inline-absent", "matching", false, nil, false, ""},
		{"matching/inline-empty", "matching", false, instructionRegressionQAString(""), false, ""},
		{"matching/inline-nonempty", "matching", false, instructionRegressionQAString("Текст"), true, "Текст"},
		{"matching/block-absent", "matching", true, nil, false, ""},
		{"matching/block-empty", "matching", true, instructionRegressionQAString(""), false, ""},
		{"matching/block-nonempty", "matching", true, instructionRegressionQAString("Текст"), true, "Текст"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declaration := instructionRegressionQADeclaration(tc.tag, tc.block, tc.content)
			before := instructionRegressionQAJSON(t, declaration)
			first, problem := textdata.Build(lesson.NodeInstruction, &declaration, nil, diagnostic.Context{Path: "first.json"})
			if problem != nil {
				t.Fatal(problem)
			}
			instructionRegressionQAAssert(t, first, tc.body, tc.value, "")
			second, problem := textdata.Build(lesson.NodeInstruction, &declaration, nil, diagnostic.Context{Path: "second.json"})
			if problem != nil {
				t.Fatal(problem)
			}
			instructionRegressionQAAssert(t, second, tc.body, tc.value, "")
			if tc.body {
				if &first[0] == &second[0] || first[0].Role == second[0].Role {
					t.Fatal("independent calls share Data or Role")
				}
				*first[0].Role = lesson.RoleTitle
				first[0].Value, first[0].Hint = "изменено", "изменено"
				first[0].Answers = append(first[0].Answers, "изменено")
				instructionRegressionQAAssert(t, second, true, tc.value, "")
			}
			if instructionRegressionQAJSON(t, declaration) != before {
				t.Fatal("Build or output mutation changed source")
			}
			if tc.content != nil {
				*declaration.Elements[1].Value = "новый источник"
				instructionRegressionQAAssert(t, second, tc.body, tc.value, "")
			}
		})
	}
}

func TestInstructionRegressionQABuildEmptyMultifillHasDifferentData(t *testing.T) {
	declaration := instructionRegressionQADeclaration("multifill", false, instructionRegressionQAString(""))
	instruction, problem := textdata.Build(lesson.NodeInstruction, &declaration, nil, diagnostic.Context{})
	if problem != nil {
		t.Fatal(problem)
	}
	instructionRegressionQAAssert(t, instruction, false, "", "")
	multifill, problem := textdata.Build(lesson.NodeMultifill, &declaration, nil, diagnostic.Context{})
	if problem != nil {
		t.Fatal(problem)
	}
	instructionRegressionQAAssert(t, multifill, true, "", "")
}

func TestInstructionRegressionQABuildEmptyPhysicalLinesAndHint(t *testing.T) {
	raw, err := os.ReadFile("../golden/3. parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	var source model.Result
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	declaration := source.Lines[7]
	lines := []model.Line{source.Lines[8], source.Lines[9], source.Lines[10]}
	before := instructionRegressionQAJSON(t, source)
	got, problem := textdata.Build(lesson.NodeInstruction, &declaration, lines, diagnostic.Context{Path: "3. parsed.json"})
	if problem != nil {
		t.Fatal(problem)
	}
	instructionRegressionQAAssert(t, got, true, "INSTRUCTION TEXT A\nINSTRUCTION TEXT B", "HINT TEXT")
	if instructionRegressionQAJSON(t, source) != before {
		t.Fatal("Build changed golden source")
	}

	// Пустые физические строки блока отбрасываются; это не inline content.
	parent := declaration.Number
	empty := model.Line{Number: 9, LineType: model.LineTypeContent, ParentLine: &parent,
		NestingLevel: declaration.NestingLevel + 1, LineEnding: model.LineEndingLF,
		Elements: []model.Element{instructionRegressionQAElement(model.ElementTypeContent, "", "", 1)}}
	blank := model.Line{Number: 10, LineType: model.LineTypeBlank, ParentLine: &parent,
		NestingLevel: declaration.NestingLevel + 1, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
	got, problem = textdata.Build(lesson.NodeInstruction, &declaration, []model.Line{empty, blank}, diagnostic.Context{})
	if problem != nil {
		t.Fatal(problem)
	}
	instructionRegressionQAAssert(t, got, false, "", "")
	got, problem = textdata.Build(lesson.NodeInstruction, &declaration, []model.Line{empty, blank, lines[2]}, diagnostic.Context{Path: "empty.json"})
	if got != nil || problem == nil || problem.Code != textdata.CodeInstructionHint || problem.Context.Path != "empty.json" {
		t.Fatalf("hint without body = %#v, %#v; want blocking hint error", got, problem)
	}
}
