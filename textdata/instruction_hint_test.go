package textdata

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestInstructionQASplitHintPreservesSurroundingLines(t *testing.T) {
	for _, block := range []bool{false, true} {
		t.Run(map[bool]string{false: "inline", true: "block"}[block], func(t *testing.T) {
			declaration := instructionQADeclaration(10, "instr", true, nil)
			before := instructionQABody(11, 10, "До <br>")
			after := instructionQABody(30, 10, "После")
			hint := instructionQADeclaration(12, "hint", block, nil)
			hint.ParentLine = instructionQAInt(10)
			hint.NestingLevel = 18 // Принадлежность устанавливает ParentLine.
			var contents []model.Line
			if block {
				contents = []model.Line{instructionQABody(13, 12, "Первая"), instructionQABlank(14, 12), instructionQABody(15, 12, "Вторая <b>строка</b>")}
			} else {
				hint = instructionQADeclaration(12, "hint", false, instructionQAString("Первая"))
				hint.ParentLine = instructionQAInt(10)
			}
			// Raw сохраняет исходный регистр, семантическое имя — tag.Value.
			hint.Raw = "@HINT" + hint.Raw[5:]
			hint.Elements[0].Raw = "@HINT"
			lines := []model.Line{before, hint}
			lines = append(lines, contents...)
			if block {
				lines = append(lines, instructionQAClose(16, 12))
			}
			lines = append(lines, after)
			snapshot, _ := json.Marshal(lines)
			body, found, ownHint, err := splitInstructionHint(declaration, lines, diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(body, []model.Line{before, after}) || found == nil || !reflect.DeepEqual(*found, hint) {
				t.Fatalf("split body=%#v hint=%#v", body, found)
			}
			if len(ownHint) != len(contents) || (len(contents) > 0 && !reflect.DeepEqual(ownHint, contents)) {
				t.Fatalf("hint body=%#v; boundaries must be excluded", ownHint)
			}
			got, err := buildInstruction(declaration, lines, diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			wantHint := "Первая"
			if block {
				wantHint = "Первая\nВторая <b>строка</b>"
			}
			instructionQAAssertBody(t, got, "До <br>\nПосле", wantHint)
			unchanged, _ := json.Marshal(lines)
			if string(unchanged) != string(snapshot) {
				t.Fatal("split/build changed input")
			}
		})
	}
}

func TestInstructionQASplitWithoutHint(t *testing.T) {
	declaration := instructionQADeclaration(10, "instr", true, nil)
	lines := []model.Line{instructionQABody(11, 10, "@hint — обычный текст"), instructionQABlank(13, 10), instructionQABody(20, 10, "Конец")}
	body, hint, hintLines, err := splitInstructionHint(declaration, lines, diagnostic.Context{})
	if err != nil || hint != nil || len(hintLines) != 0 || !reflect.DeepEqual(body, lines) {
		t.Fatalf("split without hint = %#v, %#v, %#v, %v", body, hint, hintLines, err)
	}
	got, err := buildInstruction(declaration, lines, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	instructionQAAssertBody(t, got, "@hint — обычный текст\nКонец", "")
}

func TestInstructionQAHintTextForms(t *testing.T) {
	inline := instructionQADeclaration(12, "hint", false, instructionQAString(`Подсказка \{x\}`))
	inline.Elements[1].Value = instructionQAString("Подсказка {x}")
	block := instructionQADeclaration(12, "hint", true, nil)
	for _, tc := range []struct {
		name        string
		declaration *model.Line
		lines       []model.Line
		want        string
	}{
		{"nil", nil, nil, ""},
		{"inline", &inline, nil, "Подсказка {x}"},
		{"block", &block, []model.Line{instructionQABlank(13, 12), instructionQABody(14, 12, "А"), instructionQABody(17, 12, ""), instructionQABody(20, 12, "<br>"), instructionQABlank(21, 12)}, "А\n<br>"},
		{"empty block", &block, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := instructionHint(tc.declaration, tc.lines, diagnostic.Context{})
			if err != nil || got != tc.want {
				t.Fatalf("hint=%q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func instructionQAAssertHintError(t *testing.T, err *diagnostic.Error, path string) {
	t.Helper()
	if err == nil {
		t.Fatal("want instruction hint error")
	}
	if err.Code != CodeInstructionHint || err.Context.Path != path || err.Reason == "" || err.Context.Line <= 0 {
		t.Fatalf("diagnostic = %#v", err)
	}
}

func TestInstructionQASplitRejectsAmbiguousLocalHints(t *testing.T) {
	declaration := instructionQADeclaration(10, "instr", true, nil)
	first := instructionQADeclaration(12, "hint", false, instructionQAString("Один"))
	first.ParentLine = instructionQAInt(10)
	second := instructionQADeclaration(14, "hint", false, instructionQAString("Два"))
	second.ParentLine = instructionQAInt(10)
	foreign := instructionQADeclaration(12, "note", false, instructionQAString("Чужой тег"))
	foreign.ParentLine = instructionQAInt(10)
	adjacent := instructionQADeclaration(12, "hint", false, instructionQAString("Сосед"))
	adjacent.ParentLine = instructionQAInt(5)
	adjacent.NestingLevel = declaration.NestingLevel + 1
	block := instructionQADeclaration(12, "hint", true, nil)
	block.ParentLine = instructionQAInt(10)
	for _, tc := range []struct {
		name string
		own  []model.Line
	}{
		{"second hint", []model.Line{first, second}},
		{"foreign nested tag", []model.Line{foreign}},
		{"adjacent standalone hint", []model.Line{adjacent}},
		{"root standalone hint", []model.Line{instructionQADeclaration(12, "hint", false, instructionQAString("Сосед"))}},
		{"foreign hint content parent", []model.Line{block, instructionQABody(13, 5, "Чужая строка"), instructionQAClose(14, 12)}},
		{"foreign closing boundary", []model.Line{block, instructionQABody(13, 12, "Подсказка"), instructionQAClose(14, 5)}},
		{"orphan hint content", []model.Line{instructionQABody(13, 12, "Нет локального объявления")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := append([]model.Line{instructionQABody(11, 10, "Инструкция")}, tc.own...)
			context := diagnostic.Context{Path: "источник.json", LessonID: "BA83"}
			_, _, _, err := splitInstructionHint(declaration, lines, context)
			instructionQAAssertHintError(t, err, context.Path)
			got, err := buildInstruction(declaration, lines, context)
			instructionQAAssertHintError(t, err, context.Path)
			if got != nil {
				t.Fatalf("partial data returned: %#v", got)
			}
			if tc.name == "second hint" {
				for _, number := range []int{12, 14} {
					found := false
					for _, related := range err.Related {
						found = found || related.Line == number && related.Path == context.Path
					}
					if !found {
						t.Errorf("related does not contain hint at line %d: %#v", number, err.Related)
					}
				}
			}
		})
	}
}

func TestInstructionQAHintWithoutInstructionFails(t *testing.T) {
	for _, empty := range [][]model.Line{nil, {instructionQABlank(11, 10)}, {instructionQABody(11, 10, "")}} {
		declaration := instructionQADeclaration(10, "instr", true, nil)
		hint := instructionQADeclaration(12, "hint", false, instructionQAString("Нельзя потерять"))
		hint.ParentLine = instructionQAInt(10)
		got, err := buildInstruction(declaration, append(empty, hint), diagnostic.Context{Path: "empty.json"})
		instructionQAAssertHintError(t, err, "empty.json")
		if got != nil {
			t.Errorf("hint without instruction returned data: %#v", got)
		}
	}
}

func TestInstructionQAMissingHintValue(t *testing.T) {
	hint := instructionQADeclaration(12, "hint", false, instructionQAString("Текст"))
	hint.Elements[1].Value = nil
	got, err := instructionHint(&hint, nil, diagnostic.Context{Path: "hint.json"})
	if err == nil || err.Code != CodeMissingValue || err.Context.Path != "hint.json" || err.Context.Line != 12 || err.Context.Column != 7 || got != "" {
		t.Fatalf("missing hint value = %q, %#v", got, err)
	}
}
