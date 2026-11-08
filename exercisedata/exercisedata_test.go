package exercisedata_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exBuildLine(number, parent int, kind model.LineType, tag, content string) model.Line {
	l := model.Line{Number: number, LineType: kind, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
	if parent != 0 {
		l.ParentLine = &parent
		l.NestingLevel = 1
	}
	add := func(kind model.ElementType, raw, value string) {
		start := utf8.RuneCountInString(l.Raw) + 1
		l.Raw += raw
		l.Elements = append(l.Elements, model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}})
	}
	if tag != "" {
		add(model.ElementTypeTag, "@"+tag, tag)
		l.Raw += " "
	}
	if content != "" || kind == model.LineTypeContent {
		add(model.ElementTypeContent, strings.NewReplacer("\\", "\\\\", "{", "\\{", "}", "\\}").Replace(content), content)
	}
	if kind == model.LineTypeBlockStart {
		if content != "" {
			l.Raw += " "
		}
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

func exBuildText(role lesson.DataRole, value string, answers []string, hint string) lesson.Data {
	if answers == nil {
		answers = []string{}
	}
	return lesson.Data{Type: lesson.DataText, Role: &role, Value: value, Answers: answers, Hint: hint}
}

func exBuildCollection(role lesson.DataRole, children ...lesson.Data) lesson.Data {
	return lesson.Data{Type: lesson.DataCollection, Role: &role, Value: children, Answers: []string{}}
}

func exBuildJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func exBuildEqual(t *testing.T, got, want any) {
	t.Helper()
	if a, b := exBuildJSON(t, got), exBuildJSON(t, want); !bytes.Equal(a, b) {
		t.Fatalf("got %s, want %s", a, b)
	}
}

func exBuildFixture(kind lesson.NodeType, answered bool) ([]model.Line, []lesson.Data) {
	line := exBuildLine
	const instruction = " <b>Выберите</b> \\{точно\\}! "
	if kind == lesson.NodeQuestion {
		ls := []model.Line{line(101, 77, model.LineTypeTag, "question", instruction)}
		body := exBuildText(lesson.RoleBody, instruction, nil, "")
		if answered {
			ls = append(ls, line(107, 77, model.LineTypeTag, "answer", " Да! "), line(113, 77, model.LineTypeTag, "hint", "<i>Ё</i>"))
			body.Answers, body.Hint = []string{" Да! "}, "<i>Ё</i>"
		}
		return ls, []lesson.Data{body}
	}
	tag := map[lesson.NodeType]string{lesson.NodeSingleChoice: "choice", lesson.NodeMultipleChoice: "multichoice", lesson.NodeMatching: "matching", lesson.NodeOrdering: "ordering"}[kind]
	ls := []model.Line{line(101, 77, model.LineTypeBlockStart, tag, instruction)}
	var want []lesson.Data
	if kind == lesson.NodeSingleChoice || kind == lesson.NodeMultipleChoice {
		want = append(want, exBuildText(lesson.RoleInstruction, instruction, nil, ""))
	}
	if kind == lesson.NodeSingleChoice || kind == lesson.NodeMultipleChoice {
		first := line(107, 101, model.LineTypeContent, "", "А!")
		a, b := exBuildText(lesson.RoleOption, "А!", nil, ""), exBuildText(lesson.RoleOption, "Б?", nil, "")
		if answered && kind == lesson.NodeMultipleChoice {
			first = line(107, 101, model.LineTypeTag, "answer", "А!")
			a.Answers = []string{"А!"}
		}
		ls = append(ls, first)
		if answered {
			ls = append(ls, line(113, 101, model.LineTypeTag, "answer", "Б?"), line(119, 101, model.LineTypeTag, "hint", "<br>"))
			b.Answers, b.Hint = []string{"Б?"}, "<br>"
		} else {
			ls = append(ls, line(113, 101, model.LineTypeContent, "", "Б?"))
		}
		want = append(want, exBuildCollection(lesson.RoleOptions, a, b))
	} else {
		ls = append(ls, line(107, 101, model.LineTypeContent, "", "А!"))
		a, b := exBuildText(lesson.RoleItem, "А!", nil, ""), exBuildText(lesson.RoleItem, "Б?", nil, "")
		if answered {
			answer := "2"
			if kind == lesson.NodeMatching {
				answer = "Вариант"
			}
			ls = append(ls, line(113, 101, model.LineTypeTag, "answer", answer), line(119, 101, model.LineTypeTag, "hint", "Подсказка"))
			a.Answers, a.Hint = []string{answer}, "Подсказка"
		}
		if kind == lesson.NodeOrdering {
			ls = append(ls, line(125, 101, model.LineTypeContent, "", "Б?"))
			if answered {
				ls = append(ls, line(131, 101, model.LineTypeTag, "answer", "1"))
				b.Answers = []string{"1"}
			}
			want = append(want, exBuildCollection(lesson.RoleItems, a, b))
		} else {
			ls = append(ls, line(125, 101, model.LineTypeSeparator, "", ""), line(131, 101, model.LineTypeContent, "", "Вариант"), line(137, 101, model.LineTypeContent, "", "Другой"))
			want = append(want, exBuildCollection(lesson.RoleItems, a), exBuildCollection(lesson.RoleOptions, exBuildText(lesson.RoleOption, "Вариант", nil, ""), exBuildText(lesson.RoleOption, "Другой", nil, "")))
		}
	}
	return append(ls, line(149, 101, model.LineTypeBlockEnd, "", "")), want
}

func TestBuildDispatchPreservesSemanticProjection(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeQuestion, lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering} {
		for _, answered := range []bool{false, true} {
			t.Run(string(kind)+map[bool]string{false: "/uncheckable", true: "/checkable"}[answered], func(t *testing.T) {
				lines, want := exBuildFixture(kind, answered)
				before := exBuildJSON(t, lines)
				got, checkable, err := exercisedata.Build(kind, lines, diagnostic.Context{Path: "вход/технический.json"})
				if err != nil || checkable != answered {
					t.Fatalf("checkable=%v, err=%v", checkable, err)
				}
				exBuildEqual(t, got, want)
				if kind == lesson.NodeMatching || kind == lesson.NodeOrdering {
					for _, data := range got {
						if data.Role != nil && *data.Role == lesson.RoleInstruction {
							t.Fatal("текст объявления не должен становиться локальной инструкцией")
						}
					}
				}
				if !bytes.Equal(before, exBuildJSON(t, lines)) {
					t.Fatal("Build изменил исходную проекцию")
				}
			})
		}
	}
}

func TestBuildRejectsUnsupportedKindsAtomically(t *testing.T) {
	lines, _ := exBuildFixture(lesson.NodeQuestion, true)
	for _, kind := range []lesson.NodeType{"", "unknown", lesson.NodeMultifill, lesson.NodeText, lesson.NodeTask, lesson.NodeInstruction} {
		t.Run(string(kind), func(t *testing.T) {
			got, checkable, err := exercisedata.Build(kind, lines, diagnostic.Context{Path: "input.json"})
			if got != nil || checkable || err == nil || err.Code != exercisedata.CodeKind {
				t.Fatalf("data=%v checkable=%v err=%v", got, checkable, err)
			}
		})
	}
}

func TestBuildErrorsDiscardPartialDataAndKeepSource(t *testing.T) {
	lines, _ := exBuildFixture(lesson.NodeQuestion, true)
	lines[1].Elements[1].Value = nil
	before := exBuildJSON(t, lines)
	base := diagnostic.Context{Path: "C:/JSON/урок.json", FileName: "7.json", LessonID: "Lesson", PageID: "Page", Line: 999, Column: 888}
	got, checkable, err := exercisedata.Build(lesson.NodeQuestion, lines, base)
	if got != nil || checkable || err == nil {
		t.Fatalf("data=%v checkable=%v err=%v", got, checkable, err)
	}
	if !bytes.Equal(before, exBuildJSON(t, lines)) {
		t.Fatal("ошибка Build изменила вход")
	}
	want := base
	want.Line, want.Column = 107, lines[1].Elements[1].Start
	if err.Code != exercisedata.CodeSource || err.Context != want {
		t.Fatalf("diagnostic=%+v, want context=%+v", err, want)
	}
	russian := false
	for _, r := range err.Reason {
		if unicode.In(r, unicode.Cyrillic) {
			russian = true
		}
	}
	if !russian {
		t.Fatalf("причина не на русском: %q", err.Reason)
	}
	for _, kind := range []lesson.NodeType{lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering} {
		t.Run(string(kind), func(t *testing.T) {
			ls, _ := exBuildFixture(kind, true)
			ls = append(ls[:len(ls)-1], exBuildLine(143, 101, model.LineTypeTag, "instr", "посторонняя"), ls[len(ls)-1])
			d, c, e := exercisedata.Build(kind, ls, base)
			if d != nil || c || e == nil {
				t.Fatalf("partial result: %v %v %v", d, c, e)
			}
			if e.Context.Path != base.Path || e.Context.Line != 143 {
				t.Fatalf("source=%+v", e.Context)
			}
		})
	}
}

func TestBuildAcceptsOnlyOneExerciseProjection(t *testing.T) {
	for _, boundary := range []string{"question", "task", "step", "variant", "newpage", "include", "instr", "hint"} {
		t.Run(boundary, func(t *testing.T) {
			ls, _ := exBuildFixture(lesson.NodeQuestion, false)
			ls = append(ls, exBuildLine(157, 77, model.LineTypeTag, boundary, "внешняя конструкция"))
			d, c, e := exercisedata.Build(lesson.NodeQuestion, ls, diagnostic.Context{Path: "projection.json"})
			if d != nil || c || e == nil {
				t.Fatalf("принят хвост полного DSL: %v %v %v", d, c, e)
			}
		})
	}
}

func TestBuildOutputsOwnTheirMutableData(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeQuestion, lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering} {
		t.Run(string(kind), func(t *testing.T) {
			lines, want := exBuildFixture(kind, true)
			before := exBuildJSON(t, lines)
			first, _, err := exercisedata.Build(kind, lines, diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			second, _, err := exercisedata.Build(kind, lines, diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			for i := range first {
				*first[i].Role = lesson.RoleHint
				if children, ok := first[i].Value.([]lesson.Data); ok {
					for j := range children {
						*children[j].Role = lesson.RoleHint
						if len(children[j].Answers) > 0 {
							children[j].Answers[0] = "изменено"
						}
						children[j].Value = "изменено"
					}
				} else if len(first[i].Answers) > 0 {
					first[i].Answers[0] = "изменено"
				}
			}
			exBuildEqual(t, second, want)
			if !bytes.Equal(before, exBuildJSON(t, lines)) {
				t.Fatal("выход владеет указателями входа")
			}
			*lines[0].Elements[1].Value = "новый вход"
			exBuildEqual(t, second, want)
			third, _, err := exercisedata.Build(kind, exBuildClone(t, before), diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			exBuildEqual(t, third, want)
		})
	}
}

func exBuildClone(t *testing.T, raw []byte) []model.Line {
	t.Helper()
	var lines []model.Line
	if err := json.Unmarshal(raw, &lines); err != nil {
		t.Fatal(err)
	}
	return lines
}

func TestBuildGroupSiblingsAreIndependent(t *testing.T) {
	line := exBuildLine
	ls := []model.Line{line(101, 77, model.LineTypeBlockStart, "multichoice", ""), line(107, 101, model.LineTypeBlockStart, "answer", ""), line(113, 107, model.LineTypeContent, "", "Одинаково"), line(119, 107, model.LineTypeContent, "", "Одинаково"), line(125, 107, model.LineTypeBlockEnd, "", ""), line(131, 101, model.LineTypeTag, "hint", "Общая"), line(137, 101, model.LineTypeBlockEnd, "", "")}
	d, c, e := exercisedata.Build(lesson.NodeMultipleChoice, ls, diagnostic.Context{})
	if e != nil || !c || len(d) != 1 {
		t.Fatalf("%v %v %v", d, c, e)
	}
	children, ok := d[0].Value.([]lesson.Data)
	if !ok || len(children) != 2 {
		t.Fatalf("children=%v", d[0].Value)
	}
	want := exBuildText(lesson.RoleOption, "Одинаково", []string{"Одинаково"}, "Общая")
	exBuildEqual(t, children[0], want)
	exBuildEqual(t, children[1], want)
	*children[0].Role = lesson.RoleHint
	children[0].Answers[0] = "другой"
	children[0].Hint = "другая"
	children[0].Value = "другое"
	exBuildEqual(t, children[1], want)
}
