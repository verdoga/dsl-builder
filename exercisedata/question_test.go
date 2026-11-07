package exercisedata

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exQuestionLine(t *testing.T, number int, kind model.LineType, parent int, tag string, content ...string) model.Line {
	t.Helper()
	l := model.Line{Number: number, LineType: kind, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
	if parent != 0 {
		l.ParentLine = &parent
		l.NestingLevel = 1
	}
	add := func(kind model.ElementType, raw, value string) {
		start := utf8.RuneCountInString(l.Raw) + 1
		l.Raw += raw
		l.Elements = append(l.Elements, model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: utf8.RuneCountInString(l.Raw) + 1, ErrorIDs: []string{}})
	}
	if tag != "" {
		add(model.ElementTypeTag, "@"+tag, tag)
	}
	if len(content) != 0 {
		if tag != "" {
			l.Raw += " "
		}
		add(model.ElementTypeContent, content[0], content[0])
	}
	if kind == model.LineTypeBlockStart {
		l.Raw += " "
		add(model.ElementTypeBlockOpen, "{", "{")
	}
	if kind == model.LineTypeBlockEnd {
		add(model.ElementTypeBlockClose, "}", "}")
	}
	return l
}

func exQuestionSnapshot(t *testing.T, lines []model.Line) string {
	t.Helper()
	raw, err := json.Marshal(lines)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func exQuestionLevels(t *testing.T, lines []model.Line) {
	t.Helper()
	for i := range lines {
		for j := 0; j < i; j++ {
			if lines[i].ParentLine != nil && *lines[i].ParentLine == lines[j].Number {
				lines[i].NestingLevel = lines[j].NestingLevel + 1
			}
		}
	}
}

func exQuestionBody(t *testing.T, got []lesson.Data, value string, answers []string, hint string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("data count = %d, want one body", len(got))
	}
	d := got[0]
	text, ok := d.Value.(string)
	if d.Type != lesson.DataText || d.Role == nil || *d.Role != lesson.RoleBody || !ok || text != value || d.Answers == nil || !slices.Equal(d.Answers, answers) || d.Hint != hint {
		t.Fatalf("body = %#v, want value=%q answers=%q hint=%q", d, value, answers, hint)
	}
}

func TestQuestionDataAndAnswerPreserveValues(t *testing.T) {
	line := func(n int, kind model.LineType, p int, tag string, content ...string) model.Line {
		return exQuestionLine(t, n, kind, p, tag, content...)
	}
	body, answer, hint := `  <b>Вопрос \?</b>!  `, ` Да, \{точно\}! `, " <i>Подсказка</i> "
	for _, tc := range []struct {
		name    string
		lines   []model.Line
		answers []string
		hint    string
	}{
		{"root unanswered", []model.Line{line(41, model.LineTypeTag, 0, "question", body)}, []string{}, ""},
		{"ignored tail", []model.Line{line(41, model.LineTypeTag, 7, "question", body), line(42, model.LineTypeBlank, 7, ""), line(43, model.LineTypeTag, 7, "editor", "comment")}, []string{}, ""},
		{"same parent value separate pointers", []model.Line{line(41, model.LineTypeTag, 7, "question", body), line(44, model.LineTypeTag, 7, "answer", answer), line(45, model.LineTypeTag, 7, "hint", hint)}, []string{answer}, hint},
		{"root answer", []model.Line{line(41, model.LineTypeTag, 0, "question", body), line(44, model.LineTypeTag, 0, "answer", answer)}, []string{answer}, ""},
		{"blocks blank editor", []model.Line{
			line(41, model.LineTypeTag, 7, "question", body), line(42, model.LineTypeBlank, 7, ""),
			line(43, model.LineTypeBlockStart, 7, "editor"), line(44, model.LineTypeContent, 43, "", "answer is not here"), line(45, model.LineTypeBlockEnd, 43, ""),
			line(46, model.LineTypeBlockStart, 7, "answer"), line(47, model.LineTypeContent, 46, "", answer), line(48, model.LineTypeBlank, 46, ""), line(49, model.LineTypeContent, 46, "", ""), line(50, model.LineTypeContent, 46, "", "ДА!"), line(51, model.LineTypeContent, 46, "", answer), line(52, model.LineTypeBlockEnd, 46, ""),
			line(53, model.LineTypeTag, 7, "editor", "between"), line(54, model.LineTypeBlank, 7, ""), line(55, model.LineTypeBlockStart, 7, "hint"), line(56, model.LineTypeContent, 55, "", hint), line(57, model.LineTypeBlank, 55, ""), line(58, model.LineTypeContent, 55, "", ""), line(59, model.LineTypeContent, 55, "", "<br>Ещё"), line(60, model.LineTypeBlockEnd, 55, ""), line(61, model.LineTypeBlank, 7, ""), line(62, model.LineTypeTag, 7, "editor", "tail"),
		}, []string{answer, "ДА!", answer}, hint + "\n<br>Ещё"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exQuestionLevels(t, tc.lines)
			before := exQuestionSnapshot(t, tc.lines)
			context := diagnostic.Context{Path: "вход/parsed.json", LessonID: "AB12"}
			got, checkable, err := questionData(tc.lines, context)
			if err != nil || checkable != (len(tc.answers) != 0) {
				t.Fatalf("questionData: checkable=%v err=%v", checkable, err)
			}
			exQuestionBody(t, got, body, tc.answers, tc.hint)
			group, err := questionAnswer(tc.lines, context)
			if err != nil {
				t.Fatal(err)
			}
			if len(tc.answers) == 0 {
				if group != nil {
					t.Fatalf("absent answer = %#v", group)
				}
			} else if group == nil || !slices.Equal(group.values, tc.answers) || group.hint != tc.hint || len(group.sources) != len(tc.answers) {
				t.Fatalf("answer group = %#v", group)
			}
			if exQuestionSnapshot(t, tc.lines) != before {
				t.Fatal("input changed")
			}
		})
	}
}

func TestQuestionDataRejectsUnknownOrMissingBody(t *testing.T) {
	for _, name := range []string{"absent", "nil", "empty", "ambiguous"} {
		t.Run(name, func(t *testing.T) {
			q := exQuestionLine(t, 103, model.LineTypeTag, 8, "question", "body")
			switch name {
			case "absent":
				q.Elements = q.Elements[:1]
			case "nil":
				q.Elements[1].Value = nil
			case "empty":
				q = exQuestionLine(t, 103, model.LineTypeTag, 8, "question", "")
			case "ambiguous":
				q.Elements = append(q.Elements, q.Elements[1])
			}
			lines := []model.Line{q}
			before := exQuestionSnapshot(t, lines)
			got, checkable, err := questionData(lines, diagnostic.Context{Path: "technical.json"})
			if err == nil || err.Code != CodeSource || got != nil || checkable || err.Context.Path != "technical.json" || err.Context.Line != 103 || err.Reason == "" {
				t.Fatalf("data=%#v checkable=%v err=%#v", got, checkable, err)
			}
			if exQuestionSnapshot(t, lines) != before {
				t.Fatal("invalid input changed")
			}
		})
	}
}

func TestQuestionAnswerRejectsInvalidBindingAndAnswer(t *testing.T) {
	line := func(n int, kind model.LineType, p int, tag string, content ...string) model.Line {
		return exQuestionLine(t, n, kind, p, tag, content...)
	}
	q, a, h := line(101, model.LineTypeTag, 8, "question", "Body"), line(103, model.LineTypeTag, 8, "answer", "Answer"), line(105, model.LineTypeTag, 8, "hint", "Hint")
	nilAnswer := line(103, model.LineTypeTag, 8, "answer", "Answer")
	nilAnswer.Elements[1].Value = nil
	nilHint := line(105, model.LineTypeTag, 8, "hint", "Hint")
	nilHint.Elements[1].Value = nil
	for _, tc := range []struct {
		name  string
		lines []model.Line
		code  string
	}{
		{"orphan hint", []model.Line{q, h}, CodeBinding},
		{"wrong answer parent", []model.Line{q, line(103, model.LineTypeTag, 9, "answer", "Answer")}, CodeBinding},
		{"root answer under nested question", []model.Line{q, line(103, model.LineTypeTag, 0, "answer", "Answer")}, CodeBinding},
		{"wrong hint parent", []model.Line{q, a, line(105, model.LineTypeTag, 9, "hint", "Hint")}, CodeBinding},
		{"second answer", []model.Line{q, a, line(106, model.LineTypeTag, 8, "answer", "Second")}, CodeBinding},
		{"answer after hint", []model.Line{q, a, h, line(107, model.LineTypeTag, 8, "answer", "Second")}, CodeBinding},
		{"second hint", []model.Line{q, a, h, line(107, model.LineTypeTag, 8, "hint", "Second")}, CodeBinding},
		{"content tail", []model.Line{q, a, h, line(107, model.LineTypeContent, 8, "", "Extra")}, CodeBinding},
		{"intervening tag", []model.Line{q, line(102, model.LineTypeTag, 8, "text", "Other"), a}, CodeBinding},
		{"include boundary", []model.Line{q, line(102, model.LineTypeTag, 8, "include"), a}, CodeBinding},
		{"unknown tail", []model.Line{q, a, h, line(107, model.LineTypeTag, 8, "note", "Other")}, CodeBinding},
		{"empty answer", []model.Line{q, line(103, model.LineTypeTag, 8, "answer", "")}, CodeAnswer},
		{"unicode whitespace answer", []model.Line{q, line(103, model.LineTypeTag, 8, "answer", " \t\u2003")}, CodeAnswer},
		{"nil answer content", []model.Line{q, nilAnswer}, CodeSource},
		{"nil hint content", []model.Line{q, a, nilHint}, CodeSource},
		{"empty answer block", []model.Line{q, line(103, model.LineTypeBlockStart, 8, "answer"), line(104, model.LineTypeBlank, 103, ""), line(105, model.LineTypeBlockEnd, 103, "")}, CodeAnswer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exQuestionLevels(t, tc.lines)
			before := exQuestionSnapshot(t, tc.lines)
			context := diagnostic.Context{Path: "technical.json"}
			group, err := questionAnswer(tc.lines, context)
			if err == nil || err.Code != tc.code || group != nil || err.Context.Path != context.Path || err.Context.Line < 101 || err.Reason == "" {
				t.Fatalf("group=%#v err=%#v, want %s", group, err, tc.code)
			}
			got, checkable, err := questionData(tc.lines, context)
			if err == nil || err.Code != tc.code || got != nil || checkable {
				t.Fatalf("data=%#v checkable=%v err=%#v", got, checkable, err)
			}
			if exQuestionSnapshot(t, tc.lines) != before {
				t.Fatal("invalid input changed")
			}
		})
	}
}

func TestQuestionCallsOwnTheirResults(t *testing.T) {
	lines := []model.Line{exQuestionLine(t, 41, model.LineTypeTag, 7, "question", "Body"), exQuestionLine(t, 43, model.LineTypeTag, 7, "answer", "Answer")}
	before := exQuestionSnapshot(t, lines)
	first, _, err := questionData(lines, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := questionData(lines, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	first[0].Answers[0], first[0].Value, first[0].Hint, *first[0].Role = "Changed", "Changed", "Changed", lesson.RoleTitle
	exQuestionBody(t, second, "Body", []string{"Answer"}, "")
	group, err := questionAnswer(lines, diagnostic.Context{})
	if err != nil || group == nil {
		t.Fatalf("group=%#v err=%v", group, err)
	}
	otherGroup, err := questionAnswer(lines, diagnostic.Context{})
	if err != nil || otherGroup == nil {
		t.Fatalf("other group=%#v err=%v", otherGroup, err)
	}
	group.values[0] = "Changed"
	group.sources[0].Path = "Changed"
	if otherGroup.values[0] != "Answer" || otherGroup.sources[0].Path != "" || otherGroup.source.Line != 43 || otherGroup.sources[0].Line != 43 || otherGroup.sources[0].Column != 9 {
		t.Fatalf("independent group/source = %#v", otherGroup)
	}
	exQuestionBody(t, second, "Body", []string{"Answer"}, "")
	if exQuestionSnapshot(t, lines) != before {
		t.Fatal("output mutation changed input")
	}
	*lines[0].Elements[1].Value, *lines[1].Elements[1].Value = "New body", "New answer"
	exQuestionBody(t, second, "Body", []string{"Answer"}, "")
}

func TestQuestionUsesSemanticValueWithoutUnescapingAgain(t *testing.T) {
	lines := make([]model.Line, 0, 3)
	for i, tag := range []string{"question", "answer", "hint"} {
		l := exQuestionLine(t, 71+i, model.LineTypeTag, 7, tag, `\\@`)
		semantic := `\@`
		l.Elements[1].Value = &semantic
		lines = append(lines, l)
	}
	before := exQuestionSnapshot(t, lines)
	got, checkable, err := questionData(lines, diagnostic.Context{})
	if err != nil || !checkable {
		t.Fatalf("checkable=%v err=%v", checkable, err)
	}
	exQuestionBody(t, got, `\@`, []string{`\@`}, `\@`)
	if exQuestionSnapshot(t, lines) != before {
		t.Fatal("semantic input changed")
	}
}

func TestQuestionGoldenProjections(t *testing.T) {
	for _, tc := range []struct {
		path    string
		at      int
		body    string
		answers []string
	}{
		{"../golden/3. parsed.json", 27, "TEXT A", []string{}},
		{"../golden/3. parsed.json", 28, "TEXT B", []string{}},
		{"../golden/5. parsed.json", 8, "TEXT", []string{"ANSWER TEXT"}},
	} {
		t.Run(tc.body, func(t *testing.T) {
			raw, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			var parsed model.Result
			if err := json.Unmarshal(raw, &parsed); err != nil {
				t.Fatal(err)
			}
			end := tc.at + 1
			if len(tc.answers) != 0 {
				end++
			}
			got, checkable, problem := questionData(parsed.Lines[tc.at:end], diagnostic.Context{Path: tc.path})
			if problem != nil || checkable != (len(tc.answers) != 0) {
				t.Fatalf("checkable=%v err=%v", checkable, problem)
			}
			exQuestionBody(t, got, tc.body, tc.answers, "")
		})
	}
}
