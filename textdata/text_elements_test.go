package textdata

import (
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestTextQAComposePreservesValuesAndPlaceholderRaw(t *testing.T) {
	line := model.Line{Number: 17, LineType: model.LineTypeContent, Raw: "  Я\\@😀\t _____{ответ}_____!  "}
	line.Elements = []model.Element{
		textQAElement(model.ElementTypeContent, "Я\\@😀", "Я\\@😀", 3),
		textQAElement(model.ElementTypePlaceholder, "_____{ответ}", "ответ", 9),
		textQAElement(model.ElementTypePlaceholder, "_____", "", 21),
		textQAElement(model.ElementTypeContent, "!", "!", 26),
	}
	line.Elements[2].Value = nil
	before, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	got, problem := composeElements(line, line.Elements, textQAContext())
	if problem != nil || got != "Я\\@😀\t _____{ответ}_____!" {
		t.Fatalf("текст = %q, %v", got, problem)
	}
	after, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("элементы изменены")
	}
}

func TestTextQAComposeUsesSemanticValueOnce(t *testing.T) {
	cases := []struct{ raw, value string }{
		{"\\@hint", "@hint"},
		{"\\\\@hint", "\\@hint"},
		{"<b>a|b;c</b>", "<b>a|b;c</b>"},
		{"_____{это обычный content}", "_____{это обычный content}"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			line := textQAContent(12, tc.raw)
			line.Elements[0].Value = textQAPtr(tc.value)
			got, problem := composeElements(line, line.Elements, textQAContext())
			if problem != nil || got != tc.value {
				t.Fatalf("текст = %q, %v; нужен %q", got, problem, tc.value)
			}
		})
	}
	line := textQAContent(12, "не читать весь Raw")
	got, problem := composeElements(line, nil, textQAContext())
	if problem != nil || got != "" {
		t.Fatalf("пустой выбор = %q, %v", got, problem)
	}
}

func TestTextQAComposeAdjacentAndMissingValue(t *testing.T) {
	line := textQAContent(42, "АБ")
	line.Elements = []model.Element{textQAElement(model.ElementTypeContent, "А", "один", 1), textQAElement(model.ElementTypeContent, "Б", "два", 2)}
	got, problem := composeElements(line, line.Elements, textQAContext())
	if problem != nil || got != "одиндва" {
		t.Fatalf("текст = %q, %v", got, problem)
	}
	line.Elements[1].Value = nil
	_, problem = composeElements(line, line.Elements, textQAContext())
	textQAAssertError(t, problem, CodeMissingValue, 42)
	if problem.Context.Column != 2 {
		t.Fatalf("Unicode-колонка = %d", problem.Context.Column)
	}
}

func TestTextQAInlineSelectsContentAndPlaceholder(t *testing.T) {
	line := model.Line{Number: 7, LineType: model.LineTypeTag, Raw: "@text Текст _____{да}"}
	line.Elements = []model.Element{
		textQAElement(model.ElementTypeTag, "@text", "text", 1),
		textQAElement(model.ElementTypeContent, "Текст", "Текст", 7),
		textQAElement(model.ElementTypePlaceholder, "_____{да}", "да", 13),
	}
	got, present, problem := inlineText(line, textQAContext())
	if problem != nil || !present || got != "Текст _____{да}" {
		t.Fatalf("inline = %q, %v, %v", got, present, problem)
	}
	for _, kind := range []model.ElementType{model.ElementTypeTag, model.ElementTypeTitle, model.ElementTypeIdentifier, model.ElementTypeName, model.ElementTypeBlockOpen, model.ElementTypeBlockClose} {
		t.Run(string(kind), func(t *testing.T) {
			line := model.Line{Number: 7, Raw: "x", Elements: []model.Element{textQAElement(kind, "x", "НЕ ТЕКСТ", 1)}}
			got, present, problem := inlineText(line, textQAContext())
			if problem != nil || present || got != "" {
				t.Fatalf("inline = %q, %v, %v", got, present, problem)
			}
		})
	}
	line = model.Line{Number: 7, Raw: "@text ", Elements: []model.Element{textQAElement(model.ElementTypeTag, "@text", "text", 1), textQAElement(model.ElementTypeContent, "", "", 7)}}
	got, present, problem = inlineText(line, textQAContext())
	if problem != nil || !present || got != "" {
		t.Fatalf("пустой Value = %q, %v, %v", got, present, problem)
	}
	line.Elements[1].Value = nil
	_, _, problem = inlineText(line, textQAContext())
	textQAAssertError(t, problem, CodeMissingValue, 7)
}

func TestTextQAComposeDoesNotBridgeTag(t *testing.T) {
	line := model.Line{Number: 15, LineType: model.LineTypeContent, Raw: "a @hint b"}
	left := textQAElement(model.ElementTypeContent, "a", "a", 1)
	right := textQAElement(model.ElementTypeContent, "b", "b", 9)
	line.Elements = []model.Element{left, textQAElement(model.ElementTypeTag, "@hint", "hint", 3), right}
	_, problem := composeElements(line, []model.Element{left, right}, textQAContext())
	textQAAssertError(t, problem, CodeInput, 15)
}

func TestTextQAElementGapUnicodeAndExclusiveEnd(t *testing.T) {
	cases := []struct {
		name, raw   string
		left, right model.Element
		want        string
	}{
		{"adjacent", "Я😀", textQAElement(model.ElementTypeContent, "Я", "Я", 1), textQAElement(model.ElementTypeContent, "😀", "😀", 2), ""},
		{"cyrillic-and-non-BMP", "Я😀 \t Б𐐀", textQAElement(model.ElementTypeContent, "Я😀", "Я😀", 1), textQAElement(model.ElementTypeContent, "Б𐐀", "Б𐐀", 6), " \t "},
		{"empty-insertion-at-end", "Я ", textQAElement(model.ElementTypeContent, "Я", "Я", 1), textQAElement(model.ElementTypeContent, "", "", 3), " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, problem := elementGap(model.Line{Number: 8, Raw: tc.raw}, tc.left, tc.right, textQAContext())
			if problem != nil || got != tc.want {
				t.Fatalf("gap = %q, %v; нужен %q", got, problem, tc.want)
			}
		})
	}
}

func TestTextQAElementGapRejectsNonSpaceAndInvalidRanges(t *testing.T) {
	for _, gap := range []string{"x", "@hint", "\u00a0", "\u2003", "\n", "\r", "\v", "\f", "😀"} {
		t.Run("gap-"+gap, func(t *testing.T) {
			left := textQAElement(model.ElementTypeContent, "Я", "Я", 1)
			right := textQAElement(model.ElementTypeContent, "Б", "Б", 2+len([]rune(gap)))
			_, problem := elementGap(model.Line{Number: 8, Raw: "Я" + gap + "Б"}, left, right, textQAContext())
			textQAAssertError(t, problem, CodeInput, 8)
		})
	}
	cases := []struct {
		name                                     string
		leftStart, leftEnd, rightStart, rightEnd int
	}{
		{"overlap", 1, 3, 2, 4}, {"reversed", 3, 4, 1, 2},
		{"left-before-line", 0, 2, 3, 4}, {"left-reversed", 2, 1, 3, 4},
		{"left-after-line", 1, 5, 5, 5}, {"right-after-line", 1, 2, 3, 5},
		{"right-reversed", 1, 2, 4, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left := model.Element{Start: tc.leftStart, End: tc.leftEnd}
			right := model.Element{Start: tc.rightStart, End: tc.rightEnd}
			_, problem := elementGap(model.Line{Number: 8, Raw: "a b"}, left, right, textQAContext())
			textQAAssertError(t, problem, CodeInput, 8)
		})
	}
}
