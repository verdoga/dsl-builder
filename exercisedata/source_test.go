package exercisedata

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exSourceInt(n int) *int        { return &n }
func exSourceText(s string) *string { return &s }

func exSourceLine(n int, kind model.LineType, parent *int, tag string) model.Line {
	l := model.Line{Number: n, LineType: kind, ParentLine: parent, Elements: []model.Element{}}
	if tag != "" {
		l.Elements = append(l.Elements, model.Element{ElementType: model.ElementTypeTag, Raw: "@" + strings.ToUpper(tag), Value: exSourceText(tag), Start: 3, End: 4 + len(tag)})
	}
	if kind == model.LineTypeBlockStart {
		l.Elements = append(l.Elements, model.Element{ElementType: model.ElementTypeBlockOpen, Raw: "{", Value: exSourceText("{"), Start: 30, End: 31})
	}
	if kind == model.LineTypeBlockEnd {
		l.Elements = append(l.Elements, model.Element{ElementType: model.ElementTypeBlockClose, Raw: "}", Value: exSourceText("}"), Start: 1, End: 2})
	}
	return l
}

func exSourceContent(n int, parent *int, value string) model.Line {
	l := exSourceLine(n, model.LineTypeContent, parent, "")
	l.Raw = "  " + value
	l.Elements = []model.Element{{ElementType: model.ElementTypeContent, Raw: value, Value: exSourceText(value), Start: 3, End: 3 + len([]rune(value))}}
	return l
}

func exSourceSnapshot(t *testing.T, lines []model.Line) string {
	t.Helper()
	b, err := json.Marshal(lines)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exSourceError(t *testing.T, err *diagnostic.Error) {
	t.Helper()
	if err == nil || err.Code != CodeSource || err.Reason == "" || err.Context.Path != "source.json" {
		t.Fatalf("want source diagnostic retaining path, got %#v", err)
	}
}

func TestExSourceCheckSourceLocalForms(t *testing.T) {
	ctx := diagnostic.Context{Path: "source.json"}
	for _, tc := range []struct {
		kind lesson.NodeType
		tag  string
	}{{lesson.NodeQuestion, "question"}, {lesson.NodeSingleChoice, "choice"}, {lesson.NodeMultipleChoice, "multichoice"}, {lesson.NodeMatching, "matching"}, {lesson.NodeOrdering, "ordering"}} {
		t.Run(tc.tag, func(t *testing.T) {
			form := model.LineTypeBlockStart
			if tc.kind == lesson.NodeQuestion {
				form = model.LineTypeTag
			}
			lines := []model.Line{exSourceLine(101, form, exSourceInt(8), tc.tag)}
			if form == model.LineTypeBlockStart {
				lines = append(lines, exSourceContent(104, exSourceInt(101), "текст"), exSourceLine(110, model.LineTypeBlockEnd, exSourceInt(101), ""))
			}
			before := exSourceSnapshot(t, lines)
			if err := checkSource(tc.kind, lines, ctx); err != nil {
				t.Fatalf("valid local projection: %v", err)
			}
			if exSourceSnapshot(t, lines) != before {
				t.Fatal("input changed")
			}
			wrong := append([]model.Line(nil), lines...)
			wrong[0].LineType = model.LineTypeTag
			if form == model.LineTypeTag {
				wrong[0].LineType = model.LineTypeBlockStart
			}
			exSourceError(t, checkSource(tc.kind, wrong, ctx))
			if form == model.LineTypeBlockStart {
				exSourceError(t, checkSource(tc.kind, lines[:len(lines)-1], ctx))
				exSourceError(t, checkSource(tc.kind, append(lines, exSourceContent(112, exSourceInt(8), "хвост")), ctx))
			}
		})
	}
	exSourceError(t, checkSource(lesson.NodeQuestion, nil, ctx))
	for _, tag := range []string{"unknown", "answer", "Question"} {
		exSourceError(t, checkSource(lesson.NodeQuestion, []model.Line{exSourceLine(77, model.LineTypeTag, nil, tag)}, ctx))
	}
	line := exSourceLine(78, model.LineTypeTag, nil, "question")
	line.Elements[0].Value = nil
	exSourceError(t, checkSource(lesson.NodeQuestion, []model.Line{line}, ctx))
}

func TestExSourceBlockEndUsesPhysicalParentAndType(t *testing.T) {
	ctx := diagnostic.Context{Path: "source.json"}
	lines := []model.Line{exSourceLine(9, model.LineTypeBlank, nil, ""), exSourceLine(40, model.LineTypeBlockStart, exSourceInt(8), "answer"), exSourceLine(43, model.LineTypeBlockStart, exSourceInt(40), "hint"), exSourceLine(46, model.LineTypeBlockEnd, exSourceInt(43), ""), exSourceContent(49, exSourceInt(40), "}"), exSourceLine(55, model.LineTypeBlockEnd, exSourceInt(40), "")}
	before := exSourceSnapshot(t, lines)
	if got, err := blockEnd(lines, 1, ctx); err != nil || got != 5 {
		t.Fatalf("end=%d, err=%v", got, err)
	}
	exSourceError(t, func() *diagnostic.Error { _, err := blockEnd(lines[:5], 1, ctx); return err }())
	lines[5].ParentLine = exSourceInt(8)
	exSourceError(t, func() *diagnostic.Error { _, err := blockEnd(lines, 1, ctx); return err }())
	lines[5].ParentLine = exSourceInt(40)
	if exSourceSnapshot(t, lines) != before {
		t.Fatal("input changed")
	}
	long := []model.Line{exSourceLine(100, model.LineTypeBlockStart, nil, "hint")}
	for n := 0; n < 1500; n++ {
		long = append(long, exSourceContent(101+n, exSourceInt(100), "строка"))
	}
	long = append(long, exSourceLine(1601, model.LineTypeBlockEnd, exSourceInt(100), ""))
	if end, err := blockEnd(long, 0, ctx); err != nil || end != len(long)-1 {
		t.Fatalf("long block end=%d, err=%v", end, err)
	}
}

func TestExSourceValuesPreserveSemanticTextAndCoordinates(t *testing.T) {
	ctx := diagnostic.Context{Path: "source.json", FileName: "entry.json", LessonID: "ID", PageID: "PAGE", Line: 1, Column: 99}
	line := exSourceLine(307, model.LineTypeTag, nil, "question")
	line.Elements = append(line.Elements, exSourceContent(307, nil, "  Ё <b>\\---;</b>  ").Elements...)
	line.Elements[1].Raw = "different raw"
	before := exSourceSnapshot(t, []model.Line{line})
	if tag, err := tagValue(line, ctx); err != nil || tag != "question" {
		t.Fatalf("tag=%q err=%v", tag, err)
	}
	v, found, source, err := contentValue(line, ctx)
	want := ctx
	want.Line = 307
	want.Column = 3
	if err != nil || !found || v != "  Ё <b>\\---;</b>  " || source != want {
		t.Fatalf("content=(%q,%v,%#v,%v)", v, found, source, err)
	}
	if exSourceSnapshot(t, []model.Line{line}) != before {
		t.Fatal("input changed")
	}
	empty := exSourceContent(308, nil, "")
	if v, found, _, err := contentValue(empty, ctx); err != nil || !found || v != "" {
		t.Fatalf("empty value=(%q,%v,%v)", v, found, err)
	}
	absent := exSourceLine(309, model.LineTypeBlank, nil, "")
	if tag, err := tagValue(absent, ctx); err != nil || tag != "" {
		t.Fatalf("absent tag=%q err=%v", tag, err)
	}
	if _, found, _, err := contentValue(absent, ctx); err != nil || found {
		t.Fatalf("absent content found=%v err=%v", found, err)
	}
	line.Elements[0].Value = nil
	_, err = tagValue(line, ctx)
	exSourceError(t, err)
	if err.Context.Line != 307 || err.Context.Column != 3 {
		t.Fatalf("tag location=%#v", err.Context)
	}
	line.Elements[1].Value = nil
	_, _, _, err = contentValue(line, ctx)
	exSourceError(t, err)
	if err.Context.Line != 307 || err.Context.Column != 3 {
		t.Fatalf("content location=%#v", err.Context)
	}
	line = exSourceContent(310, nil, "one")
	line.Elements = append(line.Elements, model.Element{ElementType: model.ElementTypeContent, Value: exSourceText("two"), Start: 10, End: 13})
	_, _, _, err = contentValue(line, ctx)
	exSourceError(t, err)
}

func TestExSourceNextSignificantHonorsEditorAndBoundaries(t *testing.T) {
	ctx := diagnostic.Context{Path: "source.json"}
	lines := []model.Line{exSourceLine(20, model.LineTypeBlank, nil, ""), exSourceLine(21, model.LineTypeTag, nil, "editor"), exSourceLine(22, model.LineTypeBlockStart, nil, "editor"), exSourceLine(23, model.LineTypeBlockStart, exSourceInt(22), "text"), exSourceContent(24, exSourceInt(23), "@answer ignored"), exSourceLine(25, model.LineTypeBlockEnd, exSourceInt(23), ""), exSourceLine(26, model.LineTypeBlockEnd, exSourceInt(22), ""), exSourceLine(27, model.LineTypeBlank, nil, ""), exSourceLine(28, model.LineTypeTag, nil, "answer")}
	before := exSourceSnapshot(t, lines)
	for _, tc := range []struct {
		at     int
		editor bool
		want   int
	}{{0, false, 1}, {0, true, 8}, {2, false, 2}, {6, true, 6}, {8, true, 8}, {9, true, 9}} {
		if got, err := nextSignificant(lines, tc.at, tc.editor, ctx); err != nil || got != tc.want {
			t.Errorf("at=%d editor=%v: got %d err=%v want %d", tc.at, tc.editor, got, err, tc.want)
		}
	}
	if exSourceSnapshot(t, lines) != before {
		t.Fatal("input changed")
	}
	_, err := nextSignificant(lines[:6], 2, true, ctx)
	exSourceError(t, err)
	for _, tag := range []string{"hint", "include", "task", "step", "variant", "unknown"} {
		one := []model.Line{exSourceLine(30, model.LineTypeTag, nil, tag)}
		if got, err := nextSignificant(one, 0, true, ctx); err != nil || got != 0 {
			t.Errorf("must stop at %s: %d %v", tag, got, err)
		}
	}
	if got, err := nextSignificant(lines[:1], 0, true, ctx); err != nil || got != 1 {
		t.Fatalf("blank tail=%d %v", got, err)
	}
}

func TestExSourceContentRowsPreserveOrderAndExactValues(t *testing.T) {
	ctx := diagnostic.Context{Path: "source.json", FileName: "entry.json"}
	lines := []model.Line{exSourceLine(50, model.LineTypeBlockStart, exSourceInt(7), "hint"), exSourceContent(51, exSourceInt(50), "<b>Ё</b>"), exSourceLine(52, model.LineTypeBlank, exSourceInt(50), ""), exSourceContent(53, exSourceInt(50), ""), exSourceContent(54, exSourceInt(50), "  \t"), exSourceContent(55, exSourceInt(50), "---"), exSourceContent(56, exSourceInt(50), "<b>Ё</b>"), exSourceLine(57, model.LineTypeBlockEnd, exSourceInt(50), "")}
	lines[5].Raw = "\\---"
	lines[5].Elements[0].Raw = "\\---"
	before := exSourceSnapshot(t, lines)
	rows, sources, err := contentRows(lines, 1, 7, ctx)
	if err != nil || len(rows) != 4 || len(sources) != len(rows) {
		t.Fatalf("rows=%q sources=%#v err=%v", rows, sources, err)
	}
	want := []string{"<b>Ё</b>", "  \t", "---", "<b>Ё</b>"}
	if len(rows) != len(want) {
		t.Fatalf("rows=%q", rows)
	}
	for i, text := range want {
		if rows[i] != text {
			t.Errorf("row %d=%q want %q", i, rows[i], text)
		}
		physical := []int{51, 54, 55, 56}[i]
		if sources[i].Path != ctx.Path || sources[i].FileName != ctx.FileName || sources[i].Line != physical || sources[i].Column != 3 {
			t.Errorf("source %d=%#v", i, sources[i])
		}
	}
	if exSourceSnapshot(t, lines) != before {
		t.Fatal("input changed")
	}
	rows[0] = "changed"
	sources[0].Line = 999
	again, _, err := contentRows(lines, 1, 7, ctx)
	if err != nil || again[0] != want[0] {
		t.Fatalf("repeated rows=%q err=%v", again, err)
	}
}

func TestExSourceContentRowsRejectForeignOrUnknownContent(t *testing.T) {
	ctx := diagnostic.Context{Path: "source.json"}
	bad := []model.Line{exSourceContent(61, exSourceInt(999), "foreign"), exSourceContent(61, nil, "root"), exSourceLine(61, model.LineTypeTag, exSourceInt(60), "editor"), exSourceLine(61, model.LineTypeBlockStart, exSourceInt(60), "answer"), exSourceLine(61, model.LineTypeSeparator, exSourceInt(60), "")}
	nilValue := exSourceContent(61, exSourceInt(60), "known")
	nilValue.Elements[0].Value = nil
	ambiguous := exSourceContent(61, exSourceInt(60), "first")
	ambiguous.Elements = append(ambiguous.Elements, exSourceContent(61, exSourceInt(60), "second").Elements...)
	bad = append(bad, nilValue, ambiguous)
	for i, row := range bad {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			lines := []model.Line{exSourceLine(60, model.LineTypeBlockStart, nil, "answer"), row, exSourceLine(62, model.LineTypeBlockEnd, exSourceInt(60), "")}
			before := exSourceSnapshot(t, lines)
			_, _, err := contentRows(lines, 1, 2, ctx)
			exSourceError(t, err)
			if err.Context.Line != 61 {
				t.Fatalf("wrong physical location: %#v", err.Context)
			}
			if exSourceSnapshot(t, lines) != before {
				t.Fatal("failed read changed input")
			}
		})
	}
}

func TestExSourceSeparatorRequiresExactLegacyRaw(t *testing.T) {
	for _, tc := range []struct {
		name, raw, element, value string
		want                      bool
	}{{"legacy", "---", "---", "---", true}, {"ascii margins", " \t---\t ", "---", "---", true}, {"escaped", "\\---", "\\---", "---", false}, {"escaped element", "---", "\\---", "---", false}, {"nbsp", "\u00a0---\u00a0", "---", "---", false}, {"newline", "---\n", "---", "---", false}, {"carriage return", "---\r", "---", "---", false}, {"substring", "x---", "---", "---", false}, {"semantic margins", "---", "---", " --- ", false}, {"different semantic", "---", "---", "other", false}} {
		t.Run(tc.name, func(t *testing.T) {
			l := exSourceContent(91, exSourceInt(80), tc.value)
			l.Raw = tc.raw
			l.Elements[0].Raw = tc.element
			before := exSourceSnapshot(t, []model.Line{l})
			if got := isSeparator(l); got != tc.want {
				t.Fatalf("separator=%v want %v", got, tc.want)
			}
			if exSourceSnapshot(t, []model.Line{l}) != before {
				t.Fatal("separator changed input")
			}
		})
	}
	l := exSourceContent(91, exSourceInt(80), "---")
	l.Raw = "---"
	l.Elements[0].Value = nil
	if isSeparator(l) {
		t.Fatal("nil semantic value became separator")
	}
	l.Elements[0].Value = exSourceText("---")
	l.Elements = append(l.Elements, l.Elements[0])
	if isSeparator(l) {
		t.Fatal("ambiguous content became separator")
	}
	l.LineType = model.LineTypeTag
	if isSeparator(l) {
		t.Fatal("tag became separator")
	}
	if !isSeparator(exSourceLine(92, model.LineTypeSeparator, exSourceInt(80), "")) {
		t.Fatal("classified separator not recognized")
	}
}
