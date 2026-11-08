package textdata

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func instructionQAString(value string) *string { return &value }
func instructionQAInt(value int) *int          { return &value }

func instructionQAElement(kind model.ElementType, raw, value string, start int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: instructionQAString(value),
		Start: start, End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}}
}

func instructionQADeclaration(number int, tag string, block bool, content *string) model.Line {
	raw := "@" + tag
	line := model.Line{Number: number, LineType: model.LineTypeTag, LineEnding: model.LineEndingCRLF,
		Raw: raw, Elements: []model.Element{instructionQAElement(model.ElementTypeTag, raw, tag, 1)}}
	if content != nil {
		line.Elements = append(line.Elements, instructionQAElement(model.ElementTypeContent, *content, *content, len(raw)+2))
		line.Raw += " " + *content
	}
	if block {
		line.LineType = model.LineTypeBlockStart
		line.Elements = append(line.Elements, instructionQAElement(model.ElementTypeBlockOpen, "{", "{", utf8.RuneCountInString(line.Raw)+2))
		line.Raw += " {"
	}
	return line
}

func instructionQABody(number, parent int, value string) model.Line {
	return model.Line{Number: number, ParentLine: instructionQAInt(parent), LineType: model.LineTypeContent,
		Raw: value, LineEnding: model.LineEndingCRLF,
		Elements: []model.Element{instructionQAElement(model.ElementTypeContent, value, value, 1)}}
}

func instructionQAClose(number, parent int) model.Line {
	return model.Line{Number: number, ParentLine: instructionQAInt(parent), LineType: model.LineTypeBlockEnd,
		Raw: "}", Elements: []model.Element{instructionQAElement(model.ElementTypeBlockClose, "}", "}", 1)}}
}

func instructionQABlank(number, parent int) model.Line {
	return model.Line{Number: number, ParentLine: instructionQAInt(parent), LineType: model.LineTypeBlank,
		Raw: "\t ", Elements: []model.Element{}}
}

func instructionQAAssertBody(t *testing.T, got []lesson.Data, value, hint string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("data = %#v, want exactly one body", got)
	}
	d := got[0]
	if d.Type != lesson.DataText || d.Role == nil || *d.Role != lesson.RoleBody || d.Value != value || d.Hint != hint {
		t.Errorf("body = %#v, want value %q, hint %q", d, value, hint)
	}
	if d.Answers == nil || len(d.Answers) != 0 {
		t.Errorf("answers = %#v, want non-nil empty slice", d.Answers)
	}
}

func instructionQAGolden(t *testing.T, file string) model.Result {
	t.Helper()
	raw, err := os.ReadFile("../golden/" + file + ". parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	var result model.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInstructionQAGoldenOwnSlices(t *testing.T) {
	for _, tc := range []struct {
		file        string
		declaration int
		own         []int
		value, hint string
	}{
		{"2", 7, []int{8, 9}, "INSTRUCTION TEXT A\nINSTRUCTION TEXT B", ""},
		{"3", 8, []int{9, 10, 11}, "INSTRUCTION TEXT A\nINSTRUCTION TEXT B", "HINT TEXT"},
		{"3", 41, nil, "INSTRUCTION TEXT", ""},
		{"3", 44, nil, "INSTRUCTION TEXT", ""},
		{"7", 8, nil, "INSTRUCTION TEXT", ""},
		{"7", 9, nil, "INSTRUCTION TEXT", ""},
		{"7", 16, nil, "INSTRUCTION TEXT", ""},
		{"7", 27, nil, "INSTRUCTION TEXT", ""},
		{"7", 28, nil, "INSTRUCTION TEXT", ""},
	} {
		t.Run(tc.file+"/"+strconv.Itoa(tc.declaration), func(t *testing.T) {
			result := instructionQAGolden(t, tc.file)
			declaration := result.Lines[tc.declaration-1]
			own := make([]model.Line, 0, len(tc.own))
			for _, number := range tc.own {
				own = append(own, result.Lines[number-1])
			}
			got, err := buildInstruction(declaration, own, diagnostic.Context{Path: tc.file + ". parsed.json"})
			if err != nil {
				t.Fatal(err)
			}
			instructionQAAssertBody(t, got, tc.value, tc.hint)
		})
	}
}

func TestInstructionQAInlineSemanticText(t *testing.T) {
	for _, tag := range []string{"instr", "multifill", "ordering", "matching"} {
		for _, tc := range []struct{ name, raw, value string }{
			{"exact", " \t<b>Я😀|ты;он</b>  ", " \t<b>Я😀|ты;он</b>  "},
			{"escaped", `Текст \{x\} \\@hint`, `Текст {x} \@hint`},
			{"placeholders", `Я\@ _____{ответ} _____!`, `Я@ _____{ответ} _____!`},
		} {
			t.Run(tag+"/"+tc.name, func(t *testing.T) {
				declaration := instructionQADeclaration(10, tag, tag != "instr", instructionQAString(tc.raw))
				declaration.Elements[1].Value = instructionQAString(tc.value)
				if tc.name == "placeholders" {
					start := len(tag) + 3
					text := []model.Element{
						instructionQAElement(model.ElementTypeContent, `Я\@`, "Я@", start),
						instructionQAElement(model.ElementTypePlaceholder, "_____{ответ}", "ответ", start+4),
						instructionQAElement(model.ElementTypePlaceholder, "_____", "", start+17),
						instructionQAElement(model.ElementTypeContent, "!", "!", start+22),
					}
					text[2].Value = nil
					boundaries := declaration.Elements
					declaration.Elements = append([]model.Element{boundaries[0]}, text...)
					declaration.Elements = append(declaration.Elements, boundaries[2:]...)
				}
				before, _ := json.Marshal(declaration)
				got, err := buildInstruction(declaration, nil, diagnostic.Context{})
				if err != nil {
					t.Fatal(err)
				}
				instructionQAAssertBody(t, got, tc.value, "")
				after, _ := json.Marshal(declaration)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("input mutated")
				}
			})
		}
	}
}

func TestInstructionQAEmptyForms(t *testing.T) {
	for _, tag := range []string{"instr", "multifill", "ordering", "matching"} {
		for _, block := range []bool{false, true} {
			t.Run(tag+map[bool]string{false: "/inline", true: "/block"}[block], func(t *testing.T) {
				declaration := instructionQADeclaration(10, tag, block, nil)
				got, err := buildInstruction(declaration, nil, diagnostic.Context{})
				if err != nil || got == nil || len(got) != 0 {
					t.Fatalf("empty instruction = %#v, %v; want non-nil empty data", got, err)
				}
			})
		}
	}
	for _, tag := range []string{"multifill", "ordering", "matching"} {
		for _, block := range []bool{false, true} {
			t.Run(tag+"/present-empty/block="+strconv.FormatBool(block), func(t *testing.T) {
				declaration := instructionQADeclaration(10, tag, block, instructionQAString(""))
				got, err := buildInstruction(declaration, nil, diagnostic.Context{})
				if err != nil || got == nil || len(got) != 0 {
					t.Fatalf("empty extracted instruction = %#v, %v", got, err)
				}
			})
		}
	}
}

func TestInstructionQAExplicitEmptyStandaloneContent(t *testing.T) {
	// Присутствующий пустой content отличается от отсутствия content;
	// исключение о пустой выделенной инструкции касается multifill/ordering/matching.
	declaration := instructionQADeclaration(10, "instr", false, instructionQAString(""))
	got, err := buildInstruction(declaration, nil, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	instructionQAAssertBody(t, got, "", "")
}

func TestInstructionQAMissingInstructionValue(t *testing.T) {
	for _, tag := range []string{"instr", "multifill", "ordering", "matching"} {
		for _, block := range []bool{false, true} {
			if tag == "instr" && block {
				continue // Блочный instr читает строки тела, а не content декларации.
			}
			t.Run(tag+"/block="+strconv.FormatBool(block), func(t *testing.T) {
				declaration := instructionQADeclaration(10, tag, block, instructionQAString("Текст"))
				declaration.Elements[1].Value = nil
				context := diagnostic.Context{Path: "instruction.json", FileName: "урок.json", LessonID: "AB12", PageID: "ab12-p001", Line: 3, Column: 4}
				before, _ := json.Marshal(declaration)
				got, err := buildInstruction(declaration, nil, context)
				context.Line, context.Column = 10, len(tag)+3
				if got != nil || err == nil || err.Code != CodeMissingValue || err.Reason == "" || err.Context != context {
					t.Fatalf("missing %s value = %#v, %#v", tag, got, err)
				}
				after, _ := json.Marshal(declaration)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("input mutated")
				}
			})
		}
	}
}

func TestInstructionQARepeatedInstancesAreIndependent(t *testing.T) {
	declaration := instructionQADeclaration(10, "instr", true, nil)
	hint := instructionQADeclaration(12, "hint", false, instructionQAString("Подсказка"))
	hint.ParentLine = instructionQAInt(10)
	lines := []model.Line{instructionQABody(11, 10, "Повтор"), hint, instructionQABody(15, 10, "Повтор")}
	before, _ := json.Marshal(struct {
		Declaration model.Line
		Lines       []model.Line
	}{declaration, lines})
	first, err := buildInstruction(declaration, lines, diagnostic.Context{Path: "first.json"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildInstruction(declaration, lines, diagnostic.Context{Path: "second.json"})
	if err != nil {
		t.Fatal(err)
	}
	instructionQAAssertBody(t, first, "Повтор\nПовтор", "Подсказка")
	instructionQAAssertBody(t, second, "Повтор\nПовтор", "Подсказка")
	if first[0].Role == second[0].Role || &first[0] == &second[0] {
		t.Fatal("independent calls share data or role")
	}
	*first[0].Role = lesson.RoleTitle
	first[0].Answers = append(first[0].Answers, "изменение")
	first[0].Value, first[0].Hint = "изменено", "изменено"
	instructionQAAssertBody(t, second, "Повтор\nПовтор", "Подсказка")
	after, _ := json.Marshal(struct {
		Declaration model.Line
		Lines       []model.Line
	}{declaration, lines})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("input mutated")
	}
	*lines[0].Elements[0].Value = "новый вход"
	instructionQAAssertBody(t, second, "Повтор\nПовтор", "Подсказка")
}
