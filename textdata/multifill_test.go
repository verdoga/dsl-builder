package textdata

import (
	"encoding/json"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestMultifillQAPreservesPhysicalEmptyLines(t *testing.T) {
	blank := model.Line{Number: 8, LineType: model.LineTypeBlank, Raw: " \t", LineEnding: model.LineEndingCRLF}
	empty := multifillQALine(9, "", "")
	text := multifillQALine(12, "текст", "текст")
	for _, tc := range []struct {
		name  string
		lines []model.Line
		want  string
	}{
		{"nil", nil, ""},
		{"empty slice", []model.Line{}, ""},
		{"one blank", []model.Line{blank}, ""},
		{"one empty content", []model.Line{empty}, ""},
		{"all empty", []model.Line{blank, empty, blank}, "\n\n"},
		{"leading", []model.Line{blank, empty, text}, "\n\nтекст"},
		{"trailing", []model.Line{text, empty, blank}, "текст\n\n"},
		{"consecutive middle", []model.Line{text, blank, empty, text}, "текст\n\n\nтекст"},
		{"both edges", []model.Line{empty, text, blank}, "\nтекст\n"},
		{"non successive numbers", []model.Line{multifillQALine(3, "A", "A"), multifillQALine(900, "B", "B")}, "A\nB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.lines {
				tc.lines[i].Number = (i + 1) * 41
			}
			before := multifillQAJSON(t, tc.lines)
			got, err := buildMultifill(tc.lines, diagnostic.Context{Path: "урок.parsed.json"})
			if err != nil {
				t.Fatal(err)
			}
			multifillQABody(t, got, tc.want)
			if after := multifillQAJSON(t, tc.lines); after != before {
				t.Fatalf("input changed: before %s, after %s", before, after)
			}
		})
	}
}

func TestMultifillQAPreservesOpaqueParserValuesAndPlaceholderRaw(t *testing.T) {
	answer := " am | есть "
	raw := "Я\t_____{ am | есть }  _____!"
	line := model.Line{
		Number: 41, LineType: model.LineTypeContent, Raw: raw, LineEnding: model.LineEndingCRLF,
		Elements: []model.Element{
			{ElementType: model.ElementTypeContent, Raw: "Я", Value: multifillQAString("Я"), Start: 1, End: 2},
			{ElementType: model.ElementTypePlaceholder, Raw: "_____{ am | есть }", Value: &answer, Start: 3, End: 21},
			{ElementType: model.ElementTypePlaceholder, Raw: "_____", Start: 23, End: 28},
			{ElementType: model.ElementTypeContent, Raw: "!", Value: multifillQAString("!"), Start: 28, End: 29},
		},
	}
	for _, tc := range []struct {
		name string
		line model.Line
		want string
	}{
		{"answered and unanswered", line, raw},
		{"answered alone", model.Line{Number: 2, LineType: model.LineTypeContent, Raw: "_____{ответ}", Elements: []model.Element{
			{ElementType: model.ElementTypePlaceholder, Raw: "_____{ответ}", Value: multifillQAString("ответ"), Start: 1, End: 13},
		}}, "_____{ответ}"},
		{"unanswered alone", model.Line{Number: 2, LineType: model.LineTypeContent, Raw: "_____", Elements: []model.Element{
			{ElementType: model.ElementTypePlaceholder, Raw: "_____", Start: 1, End: 6},
		}}, "_____"},
		{"parser already unescaped", multifillQALine(5, `\@hint \{ literal \}`, `@hint { literal }`), `@hint { literal }`},
		{"literal backslashes", multifillQALine(6, `\\path\name`, `\path\name`), `\path\name`},
		{"html and tag like text", multifillQALine(7, `<br><b>Я</b> @answer _____{x}; a|b`, `<br><b>Я</b> @answer _____{x}; a|b`), `<br><b>Я</b> @answer _____{x}; a|b`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []model.Line{tc.line}
			before := multifillQAJSON(t, lines)
			got, err := buildMultifill(lines, diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			multifillQABody(t, got, tc.want)
			if multifillQAJSON(t, lines) != before {
				t.Fatal("input changed")
			}
		})
	}
}

func TestMultifillQACallsOwnTheirData(t *testing.T) {
	lines := []model.Line{multifillQALine(21, "исходный", "исходный")}
	before := multifillQAJSON(t, lines)
	first, err := buildMultifill(lines, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildMultifill(lines, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	multifillQABody(t, first, "исходный")
	multifillQABody(t, second, "исходный")
	if first[0].Role == second[0].Role {
		t.Fatal("calls share Role pointer")
	}
	*first[0].Role = lesson.RoleTitle
	first[0].Value = "changed"
	first[0].Answers = append(first[0].Answers, "answer")
	first[0].Hint = "hint"
	multifillQABody(t, second, "исходный")
	if multifillQAJSON(t, lines) != before {
		t.Fatal("output mutation changed input")
	}
	*lines[0].Elements[0].Value = "input changed"
	multifillQABody(t, second, "исходный")
	third, err := buildMultifill(lines, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	multifillQABody(t, third, "input changed")
}

func TestMultifillQAReportsBodyFailureWithoutPartialData(t *testing.T) {
	missing := multifillQALine(72, "неизвестно", "")
	missing.Elements[0].Value = nil
	context := diagnostic.Context{Path: "технический.json", LessonID: "EFX7", PageID: "efx7-p001"}
	lines := []model.Line{multifillQALine(70, "до", "до"), missing}
	before := multifillQAJSON(t, lines)
	got, err := buildMultifill(lines, context)
	if err == nil {
		t.Fatal("nil content Value accepted")
	}
	if got != nil {
		t.Fatalf("partial data returned: %#v", got)
	}
	if err.Code != CodeMissingValue || err.Context.Path != context.Path || err.Context.Line != 72 || err.Context.Column != 1 {
		t.Fatalf("wrong diagnostic: %#v", err)
	}
	if err.Context.LessonID != context.LessonID || err.Context.PageID != context.PageID || err.Reason == "" {
		t.Fatalf("lost error context or reason: %#v", err)
	}
	if multifillQAJSON(t, lines) != before {
		t.Fatal("failed call changed input")
	}
}

func TestMultifillQAGoldenOwnBodySlices(t *testing.T) {
	for _, tc := range []struct {
		file string
		from int
		to   int
		want string
	}{
		{"1", 9, 18, "Conversation 1\n" +
			"A: Is there an email from Stephen? He going to / He's going to / He's going write to you with information about the boat trip.\n" +
			"B: I don't know. I'm going not to / I'm not going to / I'm not going check my emails today or tomorrow.\n" +
			"A: Oh, OK. Well, I'm going to phone / phoning / phone him this evening, so I can ask him then.\n\nConversation 2\n" +
			"A: It's the holidays! I go to / I'm going / I'm going to sleep for ten hours every night! I going / I'm going / I'm go to watch TV every day!\n" +
			"B: Really? That's so boring! My brother and I am going / is going / are going to work on a farm near the mountains. The people there are going / is going / am going to teach us how to ride horses.\n" +
			"A: Hmm, sounds quite interesting. What about your sister?\n" +
			"B: She's going not / She not going / She isn't going to come with us. She wants to stay at home."},
		{"2", 14, 13, ""},
		{"3", 45, 44, ""},
		{"5", 17, 18, "CONTENT A\nCONTENT B"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile("../golden/" + tc.file + ". parsed.json")
			if err != nil {
				t.Fatal(err)
			}
			var parsed model.Result
			if err := json.Unmarshal(raw, &parsed); err != nil {
				t.Fatal(err)
			}
			lines := parsed.Lines[tc.from-1 : tc.to]
			before := multifillQAJSON(t, parsed)
			got, problem := buildMultifill(lines, diagnostic.Context{Path: tc.file + ". parsed.json"})
			if problem != nil {
				t.Fatal(problem)
			}
			multifillQABody(t, got, tc.want)
			if multifillQAJSON(t, parsed) != before {
				t.Fatal("golden input changed")
			}
		})
	}
}

func multifillQABody(t *testing.T, got []lesson.Data, want string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("want exactly one body, got %#v", got)
	}
	datum := got[0]
	value, ok := datum.Value.(string)
	if datum.Type != lesson.DataText || datum.Role == nil || *datum.Role != lesson.RoleBody || !ok || value != want {
		t.Fatalf("want text body %q, got %#v", want, datum)
	}
	if datum.Answers == nil || len(datum.Answers) != 0 || datum.Hint != "" {
		t.Fatalf("want non-nil empty Answers and empty Hint, got %#v", datum)
	}
}

func multifillQALine(number int, raw, value string) model.Line {
	return model.Line{Number: number, LineType: model.LineTypeContent, Raw: raw, LineEnding: model.LineEndingLF,
		Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: raw, Value: multifillQAString(value), Start: 1, End: utf8.RuneCountInString(raw) + 1}}}
}

func multifillQAString(value string) *string { return &value }

func multifillQAJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
