package exercisedata_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestExChoiceSpacingRepeatedHintKeepsEvidence(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeSingleChoice, lesson.NodeMultipleChoice} {
		for forms := 0; forms < 8; forms++ {
			for _, indent := range []string{"", " \t"} {
				for _, gap := range []string{"none", "blank", "empty-content"} {
					t.Run(fmt.Sprintf("%s/forms=%d/indent=%d/gap=%s", kind, forms, len(indent), gap), func(t *testing.T) {
						base := exChoiceSpacingContext()
						lines, want := exChoiceSpacingProjection(kind, forms, indent, gap, base)
						before := exChoiceSpacingJSON(t, lines)
						first := exChoiceSpacingConflict(t, kind, lines, base, want)
						second := exChoiceSpacingConflict(t, kind, lines, base, want)
						stable := exChoiceSpacingJSON(t, second)
						if first == second || !bytes.Equal(exChoiceSpacingJSON(t, first), stable) {
							t.Fatal("diagnostics must be deterministic independent objects")
						}
						first.Context.Path = "изменено"
						for i := range first.Related {
							first.Related[i].Line = -1
							first.Related[i].PageID = "изменено"
						}
						for i := range first.Details {
							first.Details[i].Value = "изменено"
						}
						third := exChoiceSpacingConflict(t, kind, lines, base, want)
						if third == first || third == second || !bytes.Equal(stable, exChoiceSpacingJSON(t, second)) || !bytes.Equal(stable, exChoiceSpacingJSON(t, third)) {
							t.Fatal("diagnostic mutation affected another call")
						}
						if !bytes.Equal(before, exChoiceSpacingJSON(t, lines)) {
							t.Fatal("Build or diagnostic mutation changed input")
						}
					})
				}
			}
		}
	}
}

func exChoiceSpacingConflict(t *testing.T, kind lesson.NodeType, lines []model.Line, base diagnostic.Context, want []diagnostic.Context) *diagnostic.Error {
	t.Helper()
	data, checkable, problem := exercisedata.Build(kind, lines, base)
	if problem == nil || data != nil || checkable || problem.Code != exercisedata.CodeBinding {
		t.Fatalf("expected atomic CodeBinding: data=%+v checkable=%v error=%+v", data, checkable, problem)
	}
	if problem.Context != want[2] {
		t.Errorf("Context=%+v; want=%+v", problem.Context, want[2])
	}
	if !slices.Equal(problem.Related, want) {
		t.Errorf("Related=%+v; want declarations in source order=%+v", problem.Related, want)
	}
	hasRussian := false
	for _, r := range problem.Reason {
		hasRussian = hasRussian || unicode.Is(unicode.Cyrillic, r)
	}
	if !hasRussian || problem.Error() == "" {
		t.Error("expected a Russian reason and usable error")
	}
	return problem
}

func exChoiceSpacingProjection(kind lesson.NodeType, forms int, indent, gap string, base diagnostic.Context) ([]model.Line, []diagnostic.Context) {
	tag := "choice"
	if kind == lesson.NodeMultipleChoice {
		tag = "multichoice"
	}
	lines := []model.Line{exChoiceSpacingLine(100, 0, model.LineTypeBlockStart, tag, nil, indent)}
	appendLine := func(parent int, typ model.LineType, tag string, text *string) int {
		n := 100 + 10*len(lines)
		lines = append(lines, exChoiceSpacingLine(n, parent, typ, tag, text, indent))
		return n
	}
	ordinary := "Обычный"
	appendLine(100, model.LineTypeContent, "", &ordinary)
	var want []diagnostic.Context
	for i, name := range []string{"answer", "hint", "hint"} {
		if i == 2 && gap != "none" {
			typ, empty := model.LineTypeBlank, ""
			var content *string
			if gap == "empty-content" {
				typ, content = model.LineTypeContent, &empty
			}
			appendLine(100, typ, "", content)
		}
		text := []string{"Правильный", "Первая", "Повторная"}[i]
		typ, content := model.LineTypeTag, &text
		if forms&(1<<i) != 0 {
			typ, content = model.LineTypeBlockStart, nil
		}
		n := appendLine(100, typ, name, content)
		place := base
		place.Line, place.Column = n, utf8.RuneCountInString(indent)+1
		want = append(want, place)
		if typ == model.LineTypeBlockStart {
			appendLine(n, model.LineTypeContent, "", &text)
			appendLine(n, model.LineTypeBlockEnd, "", nil)
		}
	}
	appendLine(100, model.LineTypeBlockEnd, "", nil)
	return lines, want
}

func exChoiceSpacingLine(number, parent int, typ model.LineType, tag string, content *string, indent string) model.Line {
	line := model.Line{Number: number, LineType: typ, Raw: indent, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
	if parent != 0 {
		line.ParentLine, line.NestingLevel = &parent, 1
		if parent != 100 {
			line.NestingLevel = 2
		}
	}
	appendElement := func(typ model.ElementType, raw, value string) {
		start := utf8.RuneCountInString(line.Raw) + 1
		line.Raw += raw
		line.Elements = append(line.Elements, model.Element{ElementType: typ, Raw: raw, Value: &value, Start: start, End: utf8.RuneCountInString(line.Raw) + 1, ErrorIDs: []string{}})
	}
	if tag != "" {
		appendElement(model.ElementTypeTag, "@"+tag, tag)
	}
	if content != nil {
		if tag != "" {
			line.Raw += " "
		}
		appendElement(model.ElementTypeContent, *content, *content)
	}
	if typ == model.LineTypeBlockStart {
		line.Raw += " "
		appendElement(model.ElementTypeBlockOpen, "{", "{")
	}
	if typ == model.LineTypeBlockEnd {
		appendElement(model.ElementTypeBlockClose, "}", "}")
	}
	return line
}

func TestExChoiceSpacingIgnoredRowsKeepGoldenData(t *testing.T) {
	raw, err := os.ReadFile("../golden/3. parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	var input model.Result
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	original := exChoiceSpacingJSON(t, input.Lines[17:22])
	// Ожидаемые data взяты из первого singleChoice golden3 без старых ID.
	const expected = `[{"type":"collection","role":"options","value":[{"type":"text","role":"option","value":"OPTION A","answers":[],"hint":""},{"type":"text","role":"option","value":"ANSWER B","answers":["ANSWER B"],"hint":"HINT TEXT"}],"answers":[],"hint":""}]`
	var want []lesson.Data
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	wantJSON := exChoiceSpacingJSON(t, want)
	for _, kind := range []lesson.NodeType{lesson.NodeSingleChoice, lesson.NodeMultipleChoice} {
		for _, gap := range []string{"none", "blank", "empty-content"} {
			t.Run(fmt.Sprintf("%s/gap=%s", kind, gap), func(t *testing.T) {
				var lines []model.Line
				if err := json.Unmarshal(original, &lines); err != nil {
					t.Fatal(err)
				}
				if kind == lesson.NodeMultipleChoice {
					tag := "multichoice"
					lines[0].Raw = "@multichoice {"
					lines[0].Elements[0].Value, lines[0].Elements[0].Raw, lines[0].Elements[0].End = &tag, "@multichoice", 13
					lines[0].Elements[1].Start, lines[0].Elements[1].End = 14, 15
				}
				if gap != "none" {
					empty, parent := "", 18
					row := exChoiceSpacingLine(22, 100, model.LineTypeBlank, "", nil, "")
					row.ParentLine, row.NestingLevel = &parent, 2
					if gap == "empty-content" {
						row.LineType = model.LineTypeContent
						row.Elements = []model.Element{{ElementType: model.ElementTypeContent, Value: &empty, Start: 1, End: 1, ErrorIDs: []string{}}}
					}
					end := lines[4]
					end.Number = 23
					lines = append(lines[:4], row, end)
				}
				before := exChoiceSpacingJSON(t, lines)
				build := func() []lesson.Data {
					data, checkable, problem := exercisedata.Build(kind, lines, exChoiceSpacingContext())
					if problem != nil || !checkable || !bytes.Equal(exChoiceSpacingJSON(t, data), wantJSON) {
						t.Fatalf("golden data/checkable changed: data=%+v checkable=%v error=%+v", data, checkable, problem)
					}
					value := lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{ID: "BA83", Version: "1", Title: "Golden control", LineCount: "23", CreatedAt: "2026-11-06T00:00:00Z", Pages: []lesson.Page{{ID: "ba83-p001", Nodes: []lesson.Node{{ID: "ba83-n001", Type: kind, Flags: lesson.NodeFlags{Checkable: &checkable}, Data: data, Nodes: []lesson.Node{}}}}}}}
					if issues := lesson.Validate(&value, exChoiceSpacingContext()); len(issues) != 0 {
						t.Fatal(issues)
					}
					return data
				}
				first, second := build(), build()
				options := first[0].Value.([]lesson.Data)
				options[1].Answers[0], options[1].Hint, *options[1].Role = "изменено", "изменено", lesson.RoleBody
				if *options[0].Role != lesson.RoleOption || options[0].Hint != "" || len(options[0].Answers) != 0 || !bytes.Equal(wantJSON, exChoiceSpacingJSON(t, second)) {
					t.Fatal("option or call results share mutable storage")
				}
				build()
				if !bytes.Equal(before, exChoiceSpacingJSON(t, lines)) || !bytes.Equal(original, exChoiceSpacingJSON(t, input.Lines[17:22])) {
					t.Fatal("output mutation or Build changed golden input")
				}
			})
		}
	}
}

func exChoiceSpacingContext() diagnostic.Context {
	return diagnostic.Context{Path: "C:/Вход с пробелами/вход.json", FileName: "l123.json", LessonID: "BA83", PageID: "ba83-p001", Line: 900, Column: 800}
}

func exChoiceSpacingJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
