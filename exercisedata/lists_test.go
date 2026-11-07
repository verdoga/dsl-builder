package exercisedata

import (
	"encoding/json"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"strings"
	"testing"
	"unicode"
)

func exListContext(line, column int) diagnostic.Context {
	return diagnostic.Context{Path: "C:/уроки/input.parsed.json", FileName: "lesson.json", LessonID: "Ab12", PageID: "Ab12-p001", Line: line, Column: column}
}
func exListLine(number, parent int, kind model.LineType, tag, value string) model.Line {
	l := model.Line{Number: number, LineType: kind, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
	if parent != 0 {
		l.ParentLine = &parent
		l.NestingLevel = 1
		if parent > 100 {
			l.NestingLevel = 2
		}
	}
	add := func(kind model.ElementType, raw, semantic string) {
		start := len([]rune(l.Raw)) + 1
		l.Raw += raw
		l.Elements = append(l.Elements, model.Element{ElementType: kind, Raw: raw, Value: &semantic, Start: start, End: len([]rune(l.Raw)) + 1, ErrorIDs: []string{}})
	}
	if tag != "" {
		add(model.ElementTypeTag, "@"+tag, tag)
	}
	if value != "" || kind == model.LineTypeContent {
		if tag != "" {
			l.Raw += " "
		}
		add(model.ElementTypeContent, value, value)
	}
	if kind == model.LineTypeBlockStart {
		l.Raw += " "
		add(model.ElementTypeBlockOpen, "{", "{")
	}
	if kind == model.LineTypeBlockEnd {
		add(model.ElementTypeBlockClose, "}", "}")
	}
	if kind == model.LineTypeSeparator {
		l.Raw = "---"
	}
	return l
}
func exListFrame(tag string, body ...model.Line) []model.Line {
	lines := append([]model.Line{exListLine(100, 0, model.LineTypeBlockStart, tag, "")}, body...)
	last := body[len(body)-1].Number
	return append(lines, exListLine(last+1, 100, model.LineTypeBlockEnd, "", ""))
}
func exListJSON(t *testing.T, value any) string {
	t.Helper()
	if entries, ok := value.([]entry); ok {
		var snapshot strings.Builder
		for _, e := range entries {
			snapshot.WriteString(exListJSON(t, e.data))
			snapshot.WriteString(exListJSON(t, e.source))
			snapshot.WriteString(exListJSON(t, e.answerSource))
			snapshot.WriteString(exListJSON(t, e.answerSources))
		}
		return snapshot.String()
	}
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func exListError(t *testing.T, err *diagnostic.Error, code string) {
	t.Helper()
	if err == nil || err.Code != code {
		t.Fatalf("error = %+v; want %s", err, code)
	}
	russian := strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) })
	if !russian || err.Context.Path != exListContext(0, 0).Path {
		t.Fatalf("reason/context = %+v", err)
	}
}
func TestExListSplitDirectSeparators(t *testing.T) {
	row := func(n int, text string) model.Line { return exListLine(n, 100, model.LineTypeContent, "", text) }
	sep := exListLine(102, 100, model.LineTypeSeparator, "", "")
	escaped := row(102, "---")
	escaped.Raw = `\---`
	escaped.Elements[0].Raw = escaped.Raw
	escaped.Elements[0].End = 5
	for _, tc := range []struct {
		name  string
		kind  lesson.NodeType
		lines []model.Line
		index int
		code  string
	}{
		{"matching direct", lesson.NodeMatching, exListFrame("matching", row(101, "A"), sep, row(103, "B")), 2, ""},
		{"legacy separator", lesson.NodeMatching, exListFrame("matching", row(101, "A"), row(102, "---")), 2, ""},
		{"empty lower is present", lesson.NodeOrdering, exListFrame("ordering", row(101, "A"), sep), 2, ""},
		{"short ordering", lesson.NodeOrdering, exListFrame("ordering", row(101, "A"), escaped), -1, ""},
		{"matching needs separator", lesson.NodeMatching, exListFrame("matching", row(101, "A")), -1, CodeCollection},
		{"embedded dashes", lesson.NodeOrdering, exListFrame("ordering", row(101, "a---b")), -1, ""},
		{"duplicate", lesson.NodeMatching, exListFrame("matching", sep, exListLine(103, 100, model.LineTypeSeparator, "", "")), -1, CodeCollection},
		{"ordering duplicate", lesson.NodeOrdering, exListFrame("ordering", sep, exListLine(103, 100, model.LineTypeSeparator, "", "")), -1, CodeCollection},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := exListJSON(t, tc.lines)
			index, err := splitLists(tc.kind, tc.lines, exListContext(100, 1))
			if tc.code != "" {
				exListError(t, err, tc.code)
			} else if err != nil || index != tc.index {
				t.Fatalf("split = %d, %v; want %d", index, err, tc.index)
			}
			if exListJSON(t, tc.lines) != before {
				t.Fatal("input mutated")
			}
		})
	}
	for _, tag := range []string{"answer", "hint"} {
		t.Run("skip "+tag+" block", func(t *testing.T) {
			lines := exListFrame("ordering", row(101, "A"), exListLine(102, 100, model.LineTypeBlockStart, tag, ""), exListLine(103, 102, model.LineTypeContent, "", "---"), exListLine(104, 102, model.LineTypeBlockEnd, "", ""), row(105, "B"))
			index, err := splitLists(lesson.NodeOrdering, lines, exListContext(100, 1))
			if err != nil || index != -1 {
				t.Fatalf("nested separator = %d, %v", index, err)
			}
		})
	}
}
func TestExListItemsBindOnlyAdjacentAnswerAndHint(t *testing.T) {
	lines := exListFrame("matching",
		exListLine(101, 100, model.LineTypeContent, "", " A <b>\\x</b> "),
		exListLine(102, 100, model.LineTypeBlank, "", ""),
		exListLine(103, 100, model.LineTypeBlockStart, "answer", ""),
		exListLine(104, 103, model.LineTypeContent, "", "Option A"),
		exListLine(105, 103, model.LineTypeContent, "", "OPTION B"),
		exListLine(106, 103, model.LineTypeBlockEnd, "", ""),
		exListLine(107, 100, model.LineTypeBlockStart, "hint", ""),
		exListLine(108, 107, model.LineTypeContent, "", "<i>hint</i>"),
		exListLine(109, 107, model.LineTypeBlank, "", ""),
		exListLine(110, 107, model.LineTypeContent, "", "tail"),
		exListLine(111, 107, model.LineTypeBlockEnd, "", ""),
		exListLine(112, 100, model.LineTypeContent, "", "B"),
		exListLine(113, 100, model.LineTypeContent, "", ""))
	before := exListJSON(t, lines)
	items, err := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %+v, %v", items, err)
	}
	if items[0].data.Value != " A <b>\\x</b> " || strings.Join(items[0].data.Answers, "|") != "Option A|OPTION B" || items[0].data.Hint != "<i>hint</i>\ntail" {
		t.Fatalf("first = %+v", items[0])
	}
	if items[0].source != exListContext(101, 1) || items[0].answerSource == nil || *items[0].answerSource != exListContext(103, 1) || len(items[0].answerSources) != 2 || items[0].answerSources[1] != exListContext(105, 1) {
		t.Fatalf("sources = %+v", items[0])
	}
	for _, item := range items {
		if item.data.Type != lesson.DataText || item.data.Role == nil || *item.data.Role != lesson.RoleItem || item.data.Answers == nil {
			t.Fatalf("leaf = %+v", item)
		}
	}
	if len(items[1].data.Answers) != 0 || items[1].data.Hint != "" {
		t.Fatalf("second = %+v", items[1])
	}
	items[0].data.Answers[0] = "changed"
	items[0].answerSources[0].Line = 999
	*items[0].answerSource = diagnostic.Context{}
	again, err := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
	if err != nil || again[0].data.Answers[0] != "Option A" || again[0].answerSources[0] != exListContext(104, 1) || *again[0].answerSource != exListContext(103, 1) || exListJSON(t, lines) != before {
		t.Fatal("input or later result aliases prior output")
	}
	bounded, err := listItems(lines, 12, 13, exListContext(100, 1))
	if err != nil || len(bounded) != 1 || bounded[0].data.Value != "B" {
		t.Fatalf("exclusive bounds = %+v, %v", bounded, err)
	}
}
func TestExListItemsRejectOrphanRepeatedAndForeignConstructs(t *testing.T) {
	item := exListLine(101, 100, model.LineTypeContent, "", "item")
	answer := exListLine(102, 100, model.LineTypeTag, "answer", "A")
	for _, tc := range []struct {
		name string
		body []model.Line
	}{
		{"orphan answer", []model.Line{answer}},
		{"orphan hint", []model.Line{item, exListLine(102, 100, model.LineTypeTag, "hint", "h")}},
		{"repeat answer", []model.Line{item, answer, exListLine(103, 100, model.LineTypeTag, "answer", "B")}},
		{"repeat hint", []model.Line{item, answer, exListLine(103, 100, model.LineTypeTag, "hint", "h"), exListLine(104, 100, model.LineTypeTag, "hint", "h2")}},
		{"foreign parent", []model.Line{item, exListLine(102, 99, model.LineTypeTag, "answer", "A")}},
		{"editor interrupts", []model.Line{item, exListLine(102, 100, model.LineTypeTag, "editor", "comment"), exListLine(103, 100, model.LineTypeTag, "answer", "A")}},
		{"unknown tag", []model.Line{item, exListLine(102, 100, model.LineTypeTag, "note", "text")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := exListFrame("matching", tc.body...)
			_, err := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
			exListError(t, err, CodeBinding)
		})
	}
	lines := exListFrame("ordering", item, exListLine(102, 100, model.LineTypeContent, "", "new item"), exListLine(103, 100, model.LineTypeTag, "answer", "A"))
	items, err := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
	if err != nil || len(items) != 2 || len(items[0].data.Answers) != 0 || strings.Join(items[1].data.Answers, "") != "A" {
		t.Fatalf("adjacent target = %+v, %v", items, err)
	}
}
func TestExListLabelsPreserveTextAndForbidTags(t *testing.T) {
	for _, role := range []lesson.DataRole{lesson.RoleOption, lesson.RolePosition} {
		t.Run(string(role), func(t *testing.T) {
			lines := exListFrame("matching", exListLine(101, 100, model.LineTypeContent, "", " 01. <b>A</b> "), exListLine(102, 100, model.LineTypeBlank, "", ""), exListLine(103, 100, model.LineTypeContent, "", ""), exListLine(104, 100, model.LineTypeContent, "", "B!"))
			labels, err := listLabels(lines, 1, len(lines)-1, role, exListContext(100, 1))
			if err != nil || len(labels) != 2 {
				t.Fatalf("labels = %+v, %v", labels, err)
			}
			for i, want := range []string{" 01. <b>A</b> ", "B!"} {
				d := labels[i].data
				if d.Type != lesson.DataText || d.Role == nil || *d.Role != role || d.Value != want || d.Answers == nil || len(d.Answers) != 0 || d.Hint != "" {
					t.Fatalf("label %d = %+v", i, d)
				}
			}
			for _, tag := range []string{"answer", "hint", "note"} {
				for _, kind := range []model.LineType{model.LineTypeTag, model.LineTypeBlockStart} {
					bad := []model.Line{exListLine(101, 100, kind, tag, "text")}
					if kind == model.LineTypeBlockStart {
						bad = append(bad, exListLine(102, 101, model.LineTypeContent, "", "nested"), exListLine(103, 101, model.LineTypeBlockEnd, "", ""))
					}
					full := exListFrame("matching", bad...)
					_, err := listLabels(full, 1, len(full)-1, role, exListContext(100, 1))
					exListError(t, err, CodeBinding)
				}
			}
		})
	}
}

func TestExListRepeatedHintRetainsAllSourceEvidence(t *testing.T) {
	for _, indent := range []string{"", " \t"} {
		t.Run("indent="+indent, func(t *testing.T) {
			lines := exListFrame("matching",
				exListLine(101, 100, model.LineTypeContent, "", "item"),
				exListLine(102, 100, model.LineTypeTag, "answer", "A"),
				exListLine(103, 100, model.LineTypeTag, "hint", "first"),
				exListLine(104, 100, model.LineTypeTag, "hint", "second"))
			column := len([]rune(indent)) + 1
			for i := 1; i <= 4; i++ {
				lines[i].Raw = indent + lines[i].Raw
				for j := range lines[i].Elements {
					lines[i].Elements[j].Start += column - 1
					lines[i].Elements[j].End += column - 1
				}
			}
			before := exListJSON(t, lines)
			_, err := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
			exListError(t, err, CodeBinding)
			if err.Context != exListContext(104, column) {
				t.Fatalf("repeated hint source = %+v", err.Context)
			}
			if len(err.Related) != 4 {
				t.Fatalf("conflict evidence = %+v; want item, answer and both hints", err.Related)
			}
			for i, related := range err.Related {
				if related != exListContext(101+i, column) {
					t.Fatalf("participant %d = %+v; want source %d:%d", i, related, 101+i, column)
				}
			}
			diagnosticBefore := exListJSON(t, err)
			_, repeated := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
			if exListJSON(t, repeated) != diagnosticBefore {
				t.Fatal("nondeterministic conflict diagnostic")
			}
			err.Related[0].Line = 999
			_, fresh := listItems(lines, 1, len(lines)-1, exListContext(100, 1))
			if exListJSON(t, fresh) != diagnosticBefore || exListJSON(t, repeated) != diagnosticBefore || exListJSON(t, lines) != before {
				t.Fatal("conflict diagnostic aliases input or another call")
			}
		})
	}
}
