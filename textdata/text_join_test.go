package textdata

import (
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestTextQABodyLineUsesClassifiedText(t *testing.T) {
	cases := []struct {
		name string
		line model.Line
		want string
	}{
		{"blank", model.Line{Number: 3, LineType: model.LineTypeBlank, Raw: " \t"}, ""},
		{"separator", model.Line{Number: 4, LineType: model.LineTypeSeparator, Raw: " \t---\t "}, "---"},
		{"html", textQAContent(5, "<br><b>x</b>"), "<br><b>x</b>"},
		{"table", textQAContent(6, "a | b ; c"), "a | b ; c"},
		{"empty-content", textQAContent(7, ""), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, problem := bodyLine(tc.line, textQAContext())
			if problem != nil || got != tc.want {
				t.Fatalf("bodyLine = %q, %v; нужен %q", got, problem, tc.want)
			}
		})
	}
	line := textQAContent(12, "\\@hint")
	line.Elements[0].Value = textQAPtr("@hint")
	got, problem := bodyLine(line, textQAContext())
	if problem != nil || got != "@hint" {
		t.Fatalf("bodyLine = %q, %v", got, problem)
	}
	line.Elements[0].Value = nil
	_, problem = bodyLine(line, textQAContext())
	textQAAssertError(t, problem, CodeMissingValue, 12)
}

func TestTextQABodyLineRejectsNonTextLines(t *testing.T) {
	for _, kind := range []model.LineType{model.LineTypeTag, model.LineTypeHeading, model.LineTypeBlockStart, model.LineTypeBlockEnd, model.LineTypeInvalid, "unknown"} {
		t.Run(string(kind), func(t *testing.T) {
			line := textQAContent(14, "не превращать в текст")
			line.LineType = kind
			_, problem := bodyLine(line, textQAContext())
			textQAAssertError(t, problem, CodeInput, 14)
		})
	}
}

func TestTextQABodyLineDoesNotFallBackToRaw(t *testing.T) {
	line := model.Line{Number: 18, LineType: model.LineTypeContent, Raw: "неподтверждённый текст", Elements: []model.Element{}}
	got, problem := bodyLine(line, textQAContext())
	if got != "" {
		t.Fatalf("невыбранный Raw попал в результат: %q", got)
	}
	if problem != nil {
		textQAAssertError(t, problem, CodeInput, 18)
	}
}

func TestTextQAJoinBodyPhysicalLines(t *testing.T) {
	blank := model.Line{Number: 10, LineType: model.LineTypeBlank, Raw: " \t", LineEnding: model.LineEndingCRLF}
	first, second := textQAContent(20, "А😀"), textQAContent(90, "<br>")
	first.LineEnding = model.LineEndingCRLF
	second.LineEnding = model.LineEndingNone
	cases := []struct {
		name    string
		lines   []model.Line
		keep    bool
		want    string
		present bool
	}{
		{"nil", nil, false, "", false},
		{"empty", []model.Line{}, true, "", false},
		{"skip-blank", []model.Line{blank}, false, "", false},
		{"keep-blank", []model.Line{blank}, true, "", true},
		{"skip-empty-value", []model.Line{textQAContent(11, "")}, false, "", false},
		{"keep-empty-value", []model.Line{textQAContent(11, "")}, true, "", true},
		{"LF-not-EOL-or-line-numbers", []model.Line{first, second}, false, "А😀\n<br>", true},
		{"no-trailing-LF", []model.Line{first}, false, "А😀", true},
		{"drop-all-empty-lines", []model.Line{blank, first, blank, blank, textQAContent(21, ""), second, blank}, false, "А😀\n<br>", true},
		{"keep-edge-and-consecutive-empty", []model.Line{blank, first, blank, blank, second, blank}, true, "\nА😀\n\n\n<br>\n", true},
		{"two-empty-physical-lines", []model.Line{blank, textQAContent(21, "")}, true, "\n", true},
		{"table-separator", []model.Line{first, {Number: 21, LineType: model.LineTypeSeparator, Raw: "---"}, second}, false, "А😀\n---\n<br>", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, err := json.Marshal(tc.lines)
			if err != nil {
				t.Fatal(err)
			}
			got, present, problem := joinBody(tc.lines, tc.keep, textQAContext())
			if problem != nil || got != tc.want || present != tc.present {
				t.Fatalf("joinBody = %q, %v, %v; нужны %q, %v", got, present, problem, tc.want, tc.present)
			}
			after, err := json.Marshal(tc.lines)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("срез строк изменён")
			}
		})
	}
}

func TestTextQAJoinBodyReportsLaterFailure(t *testing.T) {
	bad := textQAContent(93, "неизвестно")
	bad.Elements[0].Value = nil
	for _, keep := range []bool{false, true} {
		_, _, problem := joinBody([]model.Line{textQAContent(11, "готово"), bad}, keep, textQAContext())
		textQAAssertError(t, problem, CodeMissingValue, 93)
	}
	bad.LineType = model.LineTypeTag
	_, _, problem := joinBody([]model.Line{textQAContent(11, "готово"), bad}, false, textQAContext())
	textQAAssertError(t, problem, CodeInput, 93)
}
