package exercisedata

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exMatchingRow(tag, value string) model.Line {
	kind := model.LineTypeContent
	raw := value
	elements := []model.Element{}
	if tag != "" {
		kind, raw = model.LineTypeTag, "@"+tag+" "+value
		name := tag
		elements = append(elements, model.Element{ElementType: model.ElementTypeTag, Raw: "@" + tag, Value: &name, Start: 1, End: len(tag) + 2})
	}
	start := len([]rune(raw)) - len([]rune(value)) + 1
	elements = append(elements, model.Element{ElementType: model.ElementTypeContent, Raw: value, Value: &value, Start: start, End: start + len([]rune(value))})
	return model.Line{LineType: kind, Raw: raw, Elements: elements}
}

func exMatchingFixture(instruction string, body ...model.Line) []model.Line {
	first := exMatchingRow("matching", instruction)
	first.Number, first.LineType = 100, model.LineTypeBlockStart
	if instruction == "" {
		first.Elements = first.Elements[:1]
	}
	first.Raw += " {"
	open := "{"
	first.Elements = append(first.Elements, model.Element{ElementType: model.ElementTypeBlockOpen, Raw: open, Value: &open, Start: len([]rune(first.Raw)), End: len([]rune(first.Raw)) + 1})
	lines := []model.Line{first}
	for i, row := range body {
		row.Number = 110 + i*10
		parent := 100
		if row.ParentLine == nil {
			row.ParentLine = &parent
		}
		row.NestingLevel = 1
		lines = append(lines, row)
	}
	parent, close := 100, "}"
	return append(lines, model.Line{Number: 110 + len(body)*10, ParentLine: &parent, LineType: model.LineTypeBlockEnd, Raw: close, Elements: []model.Element{{ElementType: model.ElementTypeBlockClose, Raw: close, Value: &close, Start: 1, End: 2}}})
}

func exMatchingSeparator() model.Line {
	return model.Line{LineType: model.LineTypeSeparator, Raw: "---"}
}

func exMatchingJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func exMatchingLeaves(t *testing.T, value lesson.Data, role, leafRole lesson.DataRole, count int) []lesson.Data {
	t.Helper()
	if value.Type != lesson.DataCollection || value.Role == nil || *value.Role != role || value.Answers == nil || len(value.Answers) != 0 || value.Hint != "" {
		t.Fatalf("invalid collection: %#v", value)
	}
	leaves, ok := value.Value.([]lesson.Data)
	if !ok || leaves == nil || len(leaves) != count {
		t.Fatalf("collection leaves = %#v, want %d", value.Value, count)
	}
	for _, leaf := range leaves {
		if leaf.Type != lesson.DataText || leaf.Role == nil || *leaf.Role != leafRole || leaf.Answers == nil {
			t.Fatalf("invalid leaf: %#v", leaf)
		}
		if _, ok := leaf.Value.(string); !ok {
			t.Fatalf("leaf value is %T", leaf.Value)
		}
	}
	return leaves
}

func TestExMatchingDataShapeAndOwnership(t *testing.T) {
	for _, instruction := range []string{"", "  Сопоставьте <b>\\текст</b>!  "} {
		t.Run(instruction, func(t *testing.T) {
			lines := exMatchingFixture(instruction, exMatchingRow("", " <i>Один</i>! "), exMatchingRow("answer", "ВаРиАнТ"), exMatchingRow("hint", "  <b>Подсказка</b>\\!  "), model.Line{LineType: model.LineTypeBlank}, exMatchingRow("", "Второй"), exMatchingRow("answer", "вариант"), exMatchingSeparator(), exMatchingRow("", "вариант"), exMatchingRow("", "Другой"))
			before := exMatchingJSON(t, lines)
			context := diagnostic.Context{Path: "Вход/technical.json", LessonID: "Урок"}
			got, checkable, err := matchingData(lines, context)
			if err != nil || !checkable {
				t.Fatalf("matchingData = %#v, %v, %v", got, checkable, err)
			}
			if len(got) != 2 {
				t.Fatalf("data length = %d, want 2", len(got))
			}
			for _, value := range got {
				if value.Role != nil && *value.Role == lesson.RoleInstruction {
					t.Fatalf("unexpected instruction = %#v", value)
				}
			}
			items := exMatchingLeaves(t, got[0], lesson.RoleItems, lesson.RoleItem, 2)
			options := exMatchingLeaves(t, got[1], lesson.RoleOptions, lesson.RoleOption, 2)
			if items[0].Value != " <i>Один</i>! " || items[0].Answers[0] != "ВаРиАнТ" || items[0].Hint != "  <b>Подсказка</b>\\!  " || items[1].Value != "Второй" || items[1].Answers[0] != "вариант" || items[1].Hint != "" {
				t.Fatalf("items = %#v", items)
			}
			for i, want := range []string{"вариант", "Другой"} {
				if options[i].Value != want || len(options[i].Answers) != 0 || options[i].Hint != "" {
					t.Fatalf("option = %#v", options[i])
				}
			}
			snapshot := exMatchingJSON(t, got)
			again, _, againErr := matchingData(lines, context)
			if againErr != nil || !bytes.Equal(snapshot, exMatchingJSON(t, again)) || !bytes.Equal(before, exMatchingJSON(t, lines)) {
				t.Fatal("non-deterministic output or mutated input")
			}
			items[0].Answers[0], items[0].Hint, items[0].Value = "changed", "changed", "changed"
			*items[0].Role = lesson.RoleTitle
			options[0].Value = "changed"
			if !bytes.Equal(snapshot, exMatchingJSON(t, again)) || !bytes.Equal(before, exMatchingJSON(t, lines)) {
				t.Fatal("outputs or input share mutable storage")
			}
		})
	}
}

func TestExMatchingDataUncheckableDuplicatesAndMinimum(t *testing.T) {
	lines := exMatchingFixture("", model.Line{LineType: model.LineTypeBlank}, exMatchingRow("", ""), exMatchingRow("", "Item"), exMatchingSeparator(), exMatchingRow("", "Dup"), exMatchingRow("", "dUP"))
	got, checkable, err := matchingData(lines, diagnostic.Context{})
	if err != nil || checkable || len(got) != 2 {
		t.Fatalf("minimum uncheckable matching = %#v, %v, %v", got, checkable, err)
	}
	items := exMatchingLeaves(t, got[0], lesson.RoleItems, lesson.RoleItem, 1)
	exMatchingLeaves(t, got[1], lesson.RoleOptions, lesson.RoleOption, 2)
	if items[0].Value != "Item" || len(items[0].Answers) != 0 || items[0].Hint != "" {
		t.Fatalf("unanswered item = %#v", items[0])
	}
}

func TestExMatchingDataRejectsInvalidConfiguration(t *testing.T) {
	i, a, sep, x, y := exMatchingRow("", "I"), exMatchingRow("answer", "X"), exMatchingSeparator(), exMatchingRow("", "X"), exMatchingRow("", "Y")
	tests := []struct {
		name, code string
		body       []model.Line
	}{
		{"no item", CodeCollection, []model.Line{sep, x, y}},
		{"no option", CodeCollection, []model.Line{i, sep}},
		{"one option", CodeCollection, []model.Line{i, sep, x}},
		{"no separator", CodeCollection, []model.Line{i, x, y}},
		{"two separators", CodeCollection, []model.Line{i, sep, x, sep, y}},
		{"partial answers", CodeIncompleteAnswers, []model.Line{i, a, exMatchingRow("", "II"), sep, x, y}},
		{"missing reference", CodeReference, []model.Line{i, exMatchingRow("answer", "Z"), sep, x, y}},
		{"space significant", CodeReference, []model.Line{i, exMatchingRow("answer", " X"), sep, x, y}},
		{"punctuation significant", CodeReference, []model.Line{i, exMatchingRow("answer", "X!"), sep, x, y}},
		{"unused duplicate", CodeReference, []model.Line{i, a, sep, x, y, exMatchingRow("", "y")}},
		{"ambiguous reference", CodeReference, []model.Line{i, a, sep, x, exMatchingRow("", "x")}},
		{"empty answer", CodeAnswer, []model.Line{i, exMatchingRow("answer", "\u2003"), sep, x, y}},
		{"orphan answer", CodeBinding, []model.Line{a, i, sep, x, y}},
		{"repeated answer", CodeBinding, []model.Line{i, a, a, sep, x, y}},
		{"orphan hint", CodeBinding, []model.Line{i, exMatchingRow("hint", "H"), sep, x, y}},
		{"answer below separator", CodeBinding, []model.Line{i, sep, x, a, y}},
		{"editor inside lists", CodeBinding, []model.Line{i, exMatchingRow("editor", "Note"), sep, x, y}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := exMatchingFixture("", tt.body...)
			context := diagnostic.Context{Path: "Exact/Исходник.json", FileName: "entry", LessonID: "ID", PageID: "Page"}
			before := exMatchingJSON(t, lines)
			_, checkable, err := matchingData(lines, context)
			if err == nil || err.Code != tt.code || checkable {
				t.Fatalf("checkable=%v error=%#v, want %s", checkable, err, tt.code)
			}
			if err.Context.Path != context.Path || err.Context.Line < 100 || !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
				t.Fatalf("diagnostic context/reason = %#v", err)
			}
			for n := 0; n < 4; n++ {
				_, _, repeated := matchingData(lines, context)
				if !bytes.Equal(exMatchingJSON(t, err), exMatchingJSON(t, repeated)) {
					t.Fatal("diagnostic is not deterministic")
				}
			}
			if !bytes.Equal(before, exMatchingJSON(t, lines)) {
				t.Fatal("failed conversion mutated input")
			}
		})
	}
}

func TestExMatchingDataSourceAndRelated(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(map[bool]string{false: "nil value", true: "ambiguous content"}[ambiguous], func(t *testing.T) {
			row := exMatchingRow("", "Item")
			if ambiguous {
				row.Elements = append(row.Elements, row.Elements[0])
			} else {
				row.Elements[0].Value = nil
			}
			_, _, err := matchingData(exMatchingFixture("", row, exMatchingSeparator(), exMatchingRow("", "X"), exMatchingRow("", "Y")), diagnostic.Context{Path: "input.json"})
			if err == nil || err.Code != CodeSource || err.Context.Path != "input.json" || err.Context.Line != 110 || err.Context.Column != 1 {
				t.Fatalf("source error = %#v", err)
			}
		})
	}
	for _, partial := range []bool{false, true} {
		body := []model.Line{exMatchingRow("", "I"), exMatchingRow("answer", "X"), exMatchingRow("", "II")}
		want := []int{110, 130}
		if partial {
			body = append(body, exMatchingSeparator(), exMatchingRow("", "X"), exMatchingRow("", "Y"))
		} else {
			body = append(body[:2], exMatchingSeparator(), exMatchingRow("", "X"), exMatchingRow("", "Y"), exMatchingRow("", "y"))
			want = []int{150, 160}
		}
		_, _, err := matchingData(exMatchingFixture("", body...), diagnostic.Context{Path: "input.json"})
		if err == nil {
			t.Fatal("expected conflict")
		}
		previous := 0
		for _, related := range err.Related {
			if related.Line < previous {
				t.Fatalf("related locations are not in source order: %#v", err.Related)
			}
			previous = related.Line
		}
		for _, line := range want {
			found := false
			for _, related := range err.Related {
				found = found || related.Path == "input.json" && related.Line == line && related.Column == 1
			}
			if !found {
				t.Fatalf("missing related line %d: %#v", line, err)
			}
		}
	}
}

func TestExMatchingDataNestedSeparatorAndEscapedOption(t *testing.T) {
	answer := exMatchingRow("answer", "")
	answer.LineType, answer.Elements = model.LineTypeBlockStart, answer.Elements[:1]
	parent := 120
	nested := exMatchingRow("", "---")
	nested.ParentLine = &parent
	close := model.Line{LineType: model.LineTypeBlockEnd, ParentLine: &parent, Raw: "}"}
	escaped := exMatchingRow("", "---")
	escaped.Raw, escaped.Elements[0].Raw = "\\---", "\\---"
	lines := exMatchingFixture("", exMatchingRow("", "Item"), answer, nested, close, exMatchingRow("hint", "---"), exMatchingSeparator(), escaped, exMatchingRow("", "Other"))
	got, checkable, err := matchingData(lines, diagnostic.Context{})
	if err != nil || !checkable || len(got) != 2 {
		t.Fatalf("nested separator = %#v, %v, %v", got, checkable, err)
	}
	items := exMatchingLeaves(t, got[0], lesson.RoleItems, lesson.RoleItem, 1)
	options := exMatchingLeaves(t, got[1], lesson.RoleOptions, lesson.RoleOption, 2)
	if len(items[0].Answers) != 1 || items[0].Answers[0] != "---" || items[0].Hint != "---" || options[0].Value != "---" {
		t.Fatalf("nested/escaped semantic values changed: %#v, %#v", items, options)
	}
}
