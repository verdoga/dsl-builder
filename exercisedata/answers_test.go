package exercisedata

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exAnswerInt(n int) *int          { return &n }
func exAnswerString(s string) *string { return &s }
func exAnswerContext(line, column int) diagnostic.Context {
	return diagnostic.Context{Path: "вход/урок.parsed.json", FileName: "lesson.json", LessonID: "Ba12", PageID: "Ba12-p001", Line: line, Column: column}
}
func exAnswerLine(number int, kind model.LineType, parent *int, tag string, value *string) model.Line {
	l := model.Line{Number: number, LineType: kind, ParentLine: parent, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
	if tag != "" {
		l.Elements = append(l.Elements, model.Element{ElementType: model.ElementTypeTag, Value: &tag, Start: 3, End: 10, Raw: "@" + tag})
	}
	if value != nil {
		l.Elements = append(l.Elements, model.Element{ElementType: model.ElementTypeContent, Value: value, Start: 13, End: 30, Raw: "не семантический текст"})
	}
	return l
}
func exAnswerError(t *testing.T, err *diagnostic.Error, code string, context diagnostic.Context) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s", code)
	}
	if err.Code != code || err.Context != context {
		t.Errorf("error = %#v, want %s at %#v", err, code, context)
	}
	if !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("reason is not Russian: %q", err.Reason)
	}
}
func exAnswerSnapshot(t *testing.T, lines []model.Line) string {
	t.Helper()
	raw, err := json.Marshal(lines)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestExAnswerUnicodeValidation(t *testing.T) {
	base, source := exAnswerContext(8, 3), exAnswerContext(90, 13)
	for _, value := range []string{"", " \t\r\n", "\u00a0\u2003\u202f\u3000"} {
		t.Run("blank_"+value, func(t *testing.T) {
			exAnswerError(t, validateAnswerValues([]string{value}, []diagnostic.Context{source}, base), CodeAnswer, source)
		})
	}
	exAnswerError(t, validateAnswerValues(nil, nil, base), CodeAnswer, base)
	values := []string{"  Да?!  ", "ё", "e\u0301", "É", "\\---", "<br>", "\u200b", "  Да?!  "}
	before := slices.Clone(values)
	sources := make([]diagnostic.Context, len(values))
	for i := range sources {
		sources[i] = source
	}
	if err := validateAnswerValues(values, sources, base); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(values, before) {
		t.Fatalf("answers normalized: %q", values)
	}
	exAnswerError(t, validateAnswerValues([]string{"valid", "\u00a0"}, []diagnostic.Context{base, source}, base), CodeAnswer, source)
}

func TestExAnswerBlockAlternativesAndHint(t *testing.T) {
	base := exAnswerContext(0, 0)
	lines := []model.Line{
		exAnswerLine(101, model.LineTypeBlockStart, exAnswerInt(9), "answer", nil),
		exAnswerLine(102, model.LineTypeContent, exAnswerInt(101), "", exAnswerString("  Да?! <b>\\x</b>  ")),
		exAnswerLine(103, model.LineTypeBlank, exAnswerInt(101), "", nil),
		exAnswerLine(104, model.LineTypeContent, exAnswerInt(101), "", exAnswerString("")),
		exAnswerLine(105, model.LineTypeContent, exAnswerInt(101), "", exAnswerString("e\u0301")),
		exAnswerLine(106, model.LineTypeContent, exAnswerInt(101), "", exAnswerString("e\u0301")),
		exAnswerLine(107, model.LineTypeBlockEnd, exAnswerInt(101), "", nil),
		exAnswerLine(108, model.LineTypeBlank, exAnswerInt(9), "", nil),
		exAnswerLine(109, model.LineTypeBlockStart, exAnswerInt(9), "hint", nil),
		exAnswerLine(110, model.LineTypeContent, exAnswerInt(109), "", exAnswerString(" <b>подсказка</b> ")),
		exAnswerLine(111, model.LineTypeBlank, exAnswerInt(109), "", nil),
		exAnswerLine(112, model.LineTypeContent, exAnswerInt(109), "", exAnswerString("")),
		exAnswerLine(113, model.LineTypeContent, exAnswerInt(109), "", exAnswerString("<br>")),
		exAnswerLine(114, model.LineTypeBlockEnd, exAnswerInt(109), "", nil),
		exAnswerLine(115, model.LineTypeTag, exAnswerInt(9), "answer", exAnswerString("следующий")),
	}
	before := exAnswerSnapshot(t, lines)
	group, next, err := readAnswer(lines, 0, false, base)
	if err != nil {
		t.Fatal(err)
	}
	if next != 14 || !slices.Equal(group.values, []string{"  Да?! <b>\\x</b>  ", "e\u0301", "e\u0301"}) || group.hint != " <b>подсказка</b> \n<br>" {
		t.Fatalf("group=%#v next=%d", group, next)
	}
	if group.source != exAnswerContext(101, 3) || !slices.Equal(group.sources, []diagnostic.Context{exAnswerContext(102, 13), exAnswerContext(105, 13), exAnswerContext(106, 13)}) {
		t.Fatalf("source coordinates: %#v", group)
	}
	group.values[0] = "changed"
	group.sources[0].Path = "changed"
	second, _, err := readAnswer(lines, 0, false, base)
	if err != nil || second.values[0] != "  Да?! <b>\\x</b>  " || second.sources[0].Path != base.Path {
		t.Fatalf("calls share state: %#v %v", second, err)
	}
	if exAnswerSnapshot(t, lines) != before {
		t.Fatal("input mutated")
	}
}

func TestExAnswerExplicitInvalidContent(t *testing.T) {
	base := exAnswerContext(0, 0)
	for _, tag := range []string{"answer", "hint"} {
		for _, mode := range []string{"missing", "nil", "ambiguous", "empty"} {
			t.Run(tag+"_"+mode, func(t *testing.T) {
				line := exAnswerLine(80, model.LineTypeTag, nil, tag, nil)
				if mode != "missing" {
					line = exAnswerLine(80, model.LineTypeTag, nil, tag, exAnswerString(""))
				}
				if mode == "nil" {
					line.Elements[1].Value = nil
				}
				if mode == "ambiguous" {
					line.Elements = append(line.Elements, line.Elements[1])
				}
				var err *diagnostic.Error
				if tag == "answer" {
					_, _, err = readAnswer([]model.Line{line}, 0, false, base)
				} else {
					_, _, err = readLinkedHint([]model.Line{line}, 0, nil, false, base)
				}
				if tag == "hint" && mode == "empty" {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				code := CodeSource
				if tag == "answer" && (mode == "missing" || mode == "empty") {
					code = CodeAnswer
				}
				if err == nil || err.Code != code || err.Context.Path != base.Path || err.Context.Line != 80 {
					t.Fatalf("error=%#v, want %s at source line 80", err, code)
				}
			})
		}
	}
	for _, mode := range []string{"empty", "whitespace", "wrong_parent", "nested_editor"} {
		t.Run("block_"+mode, func(t *testing.T) {
			body := exAnswerLine(82, model.LineTypeContent, exAnswerInt(81), "", exAnswerString(""))
			code := CodeAnswer
			if mode == "whitespace" {
				body.Elements[0].Value = exAnswerString("\u00a0\t")
			}
			if mode == "wrong_parent" {
				body.Elements[0].Value = exAnswerString("text")
				body.ParentLine = exAnswerInt(7)
				code = CodeSource
			}
			if mode == "nested_editor" {
				body = exAnswerLine(82, model.LineTypeTag, exAnswerInt(81), "editor", exAnswerString("comment"))
				code = CodeSource
			}
			_, _, err := readAnswer([]model.Line{exAnswerLine(81, model.LineTypeBlockStart, nil, "answer", nil), body, exAnswerLine(83, model.LineTypeBlockEnd, exAnswerInt(81), "", nil)}, 0, true, base)
			if err == nil || err.Code != code {
				t.Fatalf("error=%#v want %s", err, code)
			}
		})
	}
}

func TestExAnswerLinkedHintBoundaries(t *testing.T) {
	base := exAnswerContext(0, 0)
	for _, tc := range []struct {
		name   string
		parent *int
		tag    string
		allow  bool
		editor bool
		want   string
	}{
		{"equal_parent_values", exAnswerInt(4), "hint", false, false, "подсказка"},
		{"both_root", nil, "hint", false, false, "подсказка"},
		{"foreign_parent", exAnswerInt(3), "hint", false, false, ""},
		{"next_answer", exAnswerInt(4), "answer", false, false, ""},
		{"include_boundary", exAnswerInt(4), "include", true, false, ""},
		{"editor_allowed", exAnswerInt(4), "hint", true, true, "подсказка"},
		{"editor_forbidden", exAnswerInt(4), "hint", false, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := exAnswerInt(4)
			if tc.name == "both_root" {
				parent = nil
			}
			lines := []model.Line{}
			if tc.editor {
				lines = append(lines, exAnswerLine(20, model.LineTypeBlockStart, parent, "editor", nil), exAnswerLine(21, model.LineTypeContent, exAnswerInt(20), "", exAnswerString("hidden")), exAnswerLine(22, model.LineTypeBlockEnd, exAnswerInt(20), "", nil))
			}
			lines = append(lines, exAnswerLine(23, model.LineTypeTag, tc.parent, tc.tag, exAnswerString("подсказка")), exAnswerLine(24, model.LineTypeTag, tc.parent, "hint", exAnswerString("повтор")))
			hint, next, err := readLinkedHint(lines, 0, parent, tc.allow, base)
			if err != nil || hint != tc.want {
				t.Fatalf("hint=%q next=%d err=%v", hint, next, err)
			}
			if tc.want != "" && next != len(lines)-1 {
				t.Fatalf("second hint swallowed: next=%d", next)
			}
			if tc.want == "" && next != 0 {
				t.Fatalf("unrelated construction swallowed: next=%d", next)
			}
		})
	}
	if hint, next, err := readLinkedHint(nil, 0, nil, true, base); err != nil || hint != "" || next != 0 {
		t.Fatalf("absent hint: %q %d %v", hint, next, err)
	}
}

func TestExAnswerSingleAndOwnerRejectsDuplicates(t *testing.T) {
	base := exAnswerContext(0, 0)
	answer := exAnswerLine(41, model.LineTypeTag, exAnswerInt(7), "answer", exAnswerString("  Ё! \\n  "))
	group, next, err := readAnswer([]model.Line{answer, exAnswerLine(42, model.LineTypeTag, exAnswerInt(7), "answer", exAnswerString("second"))}, 0, true, base)
	if err != nil || next != 1 || !slices.Equal(group.values, []string{"  Ё! \\n  "}) || group.source != exAnswerContext(41, 3) || !slices.Equal(group.sources, []diagnostic.Context{exAnswerContext(41, 13)}) {
		t.Fatalf("single: %#v %d %v", group, next, err)
	}
	for _, duplicate := range []string{"answer", "hint"} {
		lines := []model.Line{exAnswerLine(40, model.LineTypeTag, exAnswerInt(7), "question", exAnswerString("Q")), answer, exAnswerLine(42, model.LineTypeTag, exAnswerInt(7), "hint", exAnswerString("H")), exAnswerLine(43, model.LineTypeTag, exAnswerInt(7), duplicate, exAnswerString("duplicate"))}
		_, err := questionAnswer(lines, base)
		if err == nil || err.Code != CodeBinding {
			t.Fatalf("duplicate %s silently accepted: %#v", duplicate, err)
		}
	}
}

func TestExAnswerAttachOwnsMutableState(t *testing.T) {
	role := lesson.RoleItem
	item := entry{data: lesson.Data{Type: lesson.DataText, Role: &role, Value: "item", Answers: []string{}}, source: exAnswerContext(10, 13)}
	group := answerGroup{values: []string{" A ", "B"}, sources: []diagnostic.Context{exAnswerContext(11, 13), exAnswerContext(12, 13)}, source: exAnswerContext(11, 3), hint: "<b>H</b>"}
	one, err := attachAnswer(item, group)
	if err != nil {
		t.Fatal(err)
	}
	two, err := attachAnswer(item, group)
	if err != nil {
		t.Fatal(err)
	}
	if one.source != item.source || one.answerSource == nil || *one.answerSource != group.source || !slices.Equal(one.answerSources, group.sources) || !slices.Equal(one.data.Answers, group.values) || one.data.Hint != group.hint || one.data.Value != "item" {
		t.Fatalf("attached=%#v", one)
	}
	one.data.Answers[0] = "changed"
	one.answerSources[0].Path = "changed"
	one.answerSource.Path = "changed"
	*one.data.Role = lesson.RoleBody
	if group.values[0] != " A " || group.sources[0].Path == "changed" || group.source.Path == "changed" || two.data.Answers[0] != " A " || two.answerSources[0].Path == "changed" || two.answerSource.Path == "changed" || *two.data.Role != lesson.RoleItem || role != lesson.RoleItem || item.answerSource != nil || len(item.data.Answers) != 0 {
		t.Fatal("attach aliases mutable input or sibling output")
	}
	_, err = attachAnswer(two, group)
	if err == nil || err.Code != CodeBinding {
		t.Fatalf("second attach=%#v", err)
	}
}

func TestExAnswerCoverageAllNonePartial(t *testing.T) {
	base := exAnswerContext(5, 3)
	for _, flags := range [][]bool{nil, {false, false, false}, {true, true, true}, {true, false, true}, {false, true, false}} {
		entries := make([]entry, len(flags))
		related := make([]diagnostic.Context, len(flags))
		for i, yes := range flags {
			entries[i] = entry{source: exAnswerContext(10+i*3, 13), data: lesson.Data{Answers: []string{}}}
			related[i] = entries[i].source
			if yes {
				source := exAnswerContext(11+i*3, 3)
				entries[i].answerSource = &source
				entries[i].data.Answers = []string{"A"}
			}
		}
		got, err := answerCoverage(entries, base)
		all, any := len(flags) > 0, false
		for _, flag := range flags {
			all = all && flag
			any = any || flag
		}
		if any && !all {
			exAnswerError(t, err, CodeIncompleteAnswers, base)
			if got || !slices.Equal(err.Related, related) {
				t.Errorf("partial %v: checkable=%v Related=%#v want %#v", flags, got, err.Related, related)
			}
		} else if err != nil || got != all {
			t.Errorf("coverage %v = %v, %v", flags, got, err)
		}
		for i, yes := range flags {
			if (entries[i].answerSource != nil) != yes || (len(entries[i].data.Answers) > 0) != yes || entries[i].source != related[i] {
				t.Fatal("coverage mutated input")
			}
		}
	}
}
