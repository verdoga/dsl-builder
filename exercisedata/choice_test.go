package exercisedata

import (
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exChoiceLine(number, parent int, kind model.LineType, tag string, value ...string) model.Line {
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
	if len(value) != 0 {
		if tag != "" {
			l.Raw += " "
		}
		add(model.ElementTypeContent, value[0], value[0])
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

func exChoiceProjection(tag string, body ...model.Line) []model.Line {
	return append(append([]model.Line{exChoiceLine(10, 0, model.LineTypeBlockStart, tag)}, body...), exChoiceLine(99, 10, model.LineTypeBlockEnd, ""))
}

func exChoiceLeaf(t *testing.T, d lesson.Data, role lesson.DataRole, value, hint string, answers ...string) {
	t.Helper()
	if d.Type != lesson.DataText || d.Role == nil || *d.Role != role || d.Value != value || d.Hint != hint || d.Answers == nil || !slices.Equal(d.Answers, answers) {
		t.Fatalf("leaf = %#v, want %s %q hint=%q answers=%q", d, role, value, hint, answers)
	}
}

func exChoiceOptionsData(t *testing.T, d lesson.Data, count int) []lesson.Data {
	t.Helper()
	options, ok := d.Value.([]lesson.Data)
	if !ok || options == nil || len(options) != count || d.Type != lesson.DataCollection || d.Role == nil || *d.Role != lesson.RoleOptions || d.Answers == nil || len(d.Answers) != 0 || d.Hint != "" {
		t.Fatalf("options collection = %#v, want %d leaves", d, count)
	}
	return options
}

func exChoiceEntry(value string, correct bool, line int) entry {
	role := lesson.RoleOption
	e := entry{data: lesson.Data{Type: lesson.DataText, Role: &role, Value: value, Answers: []string{}}, source: diagnostic.Context{Path: "input.json", Line: line, Column: 3}}
	if correct {
		e.data.Answers = []string{value}
		source := e.source
		e.answerSource = &source
		e.answerSources = []diagnostic.Context{source}
	}
	return e
}

func TestExChoiceDataInstructionAndUncheckable(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeSingleChoice, lesson.NodeMultipleChoice} {
		for _, instruction := range []bool{false, true} {
			t.Run(string(kind)+"/instruction="+map[bool]string{false: "no", true: "yes"}[instruction], func(t *testing.T) {
				tag := "choice"
				if kind == lesson.NodeMultipleChoice {
					tag = "multichoice"
				}
				lines := exChoiceProjection(tag, exChoiceLine(12, 10, model.LineTypeContent, "", "  <b>A</b>!  "), exChoiceLine(13, 10, model.LineTypeBlank, ""), exChoiceLine(14, 10, model.LineTypeContent, "", ""), exChoiceLine(15, 10, model.LineTypeContent, "", "A"))
				if instruction {
					lines[0] = exChoiceLine(10, 0, model.LineTypeBlockStart, tag, "Выберите \\*вариант*.")
				}
				data, checkable, err := choiceData(kind, lines, diagnostic.Context{Path: "input.json"})
				count := 1
				if instruction {
					count++
				}
				if err != nil || checkable || len(data) != count {
					t.Fatalf("choiceData = %#v, %v, %v", data, checkable, err)
				}
				if instruction {
					exChoiceLeaf(t, data[0], lesson.RoleInstruction, "Выберите \\*вариант*.", "")
				}
				opts := exChoiceOptionsData(t, data[count-1], 2)
				exChoiceLeaf(t, opts[0], lesson.RoleOption, "  <b>A</b>!  ", "")
				exChoiceLeaf(t, opts[1], lesson.RoleOption, "A", "")
			})
		}
	}
}

func TestExChoiceCorrectOptionsPreserveInstancesAndSources(t *testing.T) {
	group := answerGroup{values: []string{"  A!  ", "  A!  ", "a!"}, sources: []diagnostic.Context{{Line: 23, Column: 2}, {Line: 24, Column: 4}, {Line: 25, Column: 6}}, source: diagnostic.Context{Path: "input.json", Line: 22, Column: 1}, hint: "<b>Hint</b>\n\\text"}
	opts := correctOptions(group)
	if len(opts) != 3 {
		t.Fatalf("correctOptions length = %d", len(opts))
	}
	for i, e := range opts {
		exChoiceLeaf(t, e.data, lesson.RoleOption, group.values[i], group.hint, group.values[i])
		if e.source != group.sources[i] || e.answerSource == nil || *e.answerSource != group.source || !slices.Equal(e.answerSources, group.sources[i:i+1]) {
			t.Fatalf("sources[%d] = %#v", i, e)
		}
	}
	opts[0].data.Answers[0] = "changed"
	if group.values[0] != "  A!  " || opts[1].data.Answers[0] != "  A!  " {
		t.Fatal("answer storage aliases group or another option")
	}
}

func TestExChoiceOptionsReadGroupsAndNestedBodies(t *testing.T) {
	lines := exChoiceProjection("multichoice", exChoiceLine(12, 10, model.LineTypeContent, "", "wrong"), exChoiceLine(20, 10, model.LineTypeBlockStart, "answer"), exChoiceLine(21, 20, model.LineTypeContent, "", "Yes"), exChoiceLine(22, 20, model.LineTypeBlank, ""), exChoiceLine(23, 20, model.LineTypeContent, "", "Yes"), exChoiceLine(24, 20, model.LineTypeBlockEnd, ""), exChoiceLine(25, 10, model.LineTypeBlockStart, "hint"), exChoiceLine(26, 25, model.LineTypeContent, "", "<b>one</b>"), exChoiceLine(27, 25, model.LineTypeContent, "", ""), exChoiceLine(28, 25, model.LineTypeContent, "", "two"), exChoiceLine(29, 25, model.LineTypeBlockEnd, ""), exChoiceLine(30, 10, model.LineTypeTag, "answer", "Last"))
	opts, groups, err := choiceOptions(lines, diagnostic.Context{Path: "input.json"})
	if err != nil || len(opts) != 4 || len(groups) != 2 {
		t.Fatalf("choiceOptions = %#v, %#v, %v", opts, groups, err)
	}
	if groups[0].Line != 20 || groups[1].Line != 30 {
		t.Fatalf("group order = %#v", groups)
	}
	exChoiceLeaf(t, opts[0].data, lesson.RoleOption, "wrong", "")
	for _, i := range []int{1, 2} {
		exChoiceLeaf(t, opts[i].data, lesson.RoleOption, "Yes", "<b>one</b>\ntwo", "Yes")
	}
	exChoiceLeaf(t, opts[3].data, lesson.RoleOption, "Last", "", "Last")
	data, checkable, err := choiceData(lesson.NodeMultipleChoice, lines, diagnostic.Context{})
	if err != nil || !checkable || len(data) != 1 {
		t.Fatalf("choiceData = %#v, %v, %v", data, checkable, err)
	}
	exChoiceOptionsData(t, data[0], 4)
}

func TestExChoiceAnswerCardinality(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      lesson.NodeType
		options   []entry
		groups    int
		checkable bool
		code      string
	}{
		{"none", lesson.NodeSingleChoice, []entry{exChoiceEntry("A", false, 12)}, 0, false, ""},
		{"single", lesson.NodeSingleChoice, []entry{exChoiceEntry("A", true, 12)}, 1, true, ""},
		{"single alternatives", lesson.NodeSingleChoice, []entry{exChoiceEntry("A", true, 12), exChoiceEntry("B", true, 13)}, 1, false, CodeAnswer},
		{"single groups", lesson.NodeSingleChoice, []entry{exChoiceEntry("A", true, 12), exChoiceEntry("B", true, 13)}, 2, false, CodeAnswer},
		{"multiple alternatives", lesson.NodeMultipleChoice, []entry{exChoiceEntry("A", true, 12), exChoiceEntry("A", true, 13)}, 1, true, ""},
		{"multiple groups", lesson.NodeMultipleChoice, []entry{exChoiceEntry("A", true, 12), exChoiceEntry("A", true, 13)}, 2, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := make([]diagnostic.Context, tc.groups)
			got, err := checkChoiceAnswers(tc.kind, tc.options, groups, diagnostic.Context{})
			if got != tc.checkable || (tc.code == "" && err != nil) || (tc.code != "" && (err == nil || err.Code != tc.code)) {
				t.Fatalf("checkChoiceAnswers = %v, %v", got, err)
			}
		})
	}
}

func TestExChoiceLabelsUseExactTextAndCorrectness(t *testing.T) {
	for _, tc := range []struct {
		name, second      string
		correct, conflict bool
	}{
		{"same wrong", "Same", false, false}, {"different correctness", "Same", true, true}, {"case", "same", true, false}, {"space", " Same", true, false}, {"punctuation", "Same!", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := []entry{exChoiceEntry("Same", false, 12), exChoiceEntry(tc.second, tc.correct, 15)}
			err := checkChoiceLabels(opts, diagnostic.Context{Path: "input.json"})
			if !tc.conflict {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Code != CodeContradiction || err.Reason == "" {
				t.Fatalf("contradiction = %v", err)
			}
			for _, line := range []int{12, 15} {
				found := false
				for _, c := range err.Related {
					if c.Path == "input.json" && c.Line == line && c.Column == 3 {
						found = true
					}
				}
				if !found {
					t.Fatalf("conflict participant %d missing: %#v", line, err)
				}
			}
			ok, answerErr := checkChoiceAnswers(lesson.NodeMultipleChoice, opts, []diagnostic.Context{{Line: 15}}, diagnostic.Context{})
			if ok || answerErr == nil || answerErr.Code != CodeContradiction {
				t.Fatalf("checkChoiceAnswers = %v,%v", ok, answerErr)
			}
		})
	}
	if err := checkChoiceLabels([]entry{exChoiceEntry("same", true, 12), exChoiceEntry("same", true, 15)}, diagnostic.Context{}); err != nil {
		t.Fatal(err)
	}
}

func TestExChoiceBuildRejectsMalformedProjectionWithoutPartialOutput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []model.Line
		codes []string
	}{
		{"empty", nil, []string{CodeSource}},
		{"no options", exChoiceProjection("choice"), []string{CodeCollection}},
		{"one option", exChoiceProjection("choice", exChoiceLine(12, 10, model.LineTypeContent, "", "A")), []string{CodeCollection}},
		{"orphan hint", exChoiceProjection("choice", exChoiceLine(12, 10, model.LineTypeTag, "hint", "H")), []string{CodeBinding}},
		{"editor", exChoiceProjection("choice", exChoiceLine(12, 10, model.LineTypeTag, "editor", "E")), []string{CodeBinding, CodeSource}},
		{"separator", exChoiceProjection("choice", exChoiceLine(12, 10, model.LineTypeSeparator, "")), []string{CodeBinding, CodeSource}},
		{"wrong parent", exChoiceProjection("choice", exChoiceLine(12, 8, model.LineTypeContent, "", "A")), []string{CodeBinding, CodeSource}},
		{"foreign tag", exChoiceProjection("choice", exChoiceLine(12, 10, model.LineTypeTag, "instr", "I")), []string{CodeBinding, CodeSource}},
		{"missing block end", []model.Line{exChoiceLine(10, 0, model.LineTypeBlockStart, "choice"), exChoiceLine(12, 10, model.LineTypeContent, "", "A")}, []string{CodeSource}},
		{"unclosed answer", exChoiceProjection("choice", exChoiceLine(12, 10, model.LineTypeBlockStart, "answer"), exChoiceLine(13, 12, model.LineTypeContent, "", "A")), []string{CodeSource}},
		{"two groups", exChoiceProjection("choice", exChoiceLine(12, 10, model.LineTypeTag, "answer", "A"), exChoiceLine(13, 10, model.LineTypeTag, "answer", "B")), []string{CodeAnswer}},
		{"contradiction", exChoiceProjection("choice", exChoiceLine(12, 10, model.LineTypeContent, "", "A"), exChoiceLine(13, 10, model.LineTypeTag, "answer", "A")), []string{CodeContradiction}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, checkable, err := Build(lesson.NodeSingleChoice, tc.lines, diagnostic.Context{Path: "input.json"})
			if data != nil || checkable || err == nil || !slices.Contains(tc.codes, err.Code) || err.Context.Path != "input.json" {
				t.Fatalf("Build = %#v,%v,%v", data, checkable, err)
			}
		})
	}
}
