package exercisedata_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestExChoiceBindingRepeatedHintKeepsKnownDeclarations(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeQuestion, lesson.NodeMatching, lesson.NodeOrdering} {
		for _, block := range []bool{false, true} {
			for _, indent := range []string{"", " \t"} {
				t.Run(fmt.Sprintf("%s/block=%v/indent=%d", kind, block, len(indent)), func(t *testing.T) {
					lines, evidence := exChoiceBindingFixture(kind, block, true, indent)
					before := exChoiceBindingSnapshot(t, lines)
					base := evidence[0]
					base.Line, base.Column = 999, 999
					data, checkable, first := exercisedata.Build(kind, lines, base)
					if data != nil || checkable || first == nil || first.Code != exercisedata.CodeBinding {
						t.Fatalf("Build = %v, %v, %+v; want nil, false, CodeBinding", data, checkable, first)
					}
					exChoiceBindingEvidence(t, first, evidence)
					initial, err := json.Marshal(first)
					if err != nil {
						t.Fatal(err)
					}
					_, _, second := exercisedata.Build(kind, lines, base)
					other, err := json.Marshal(second)
					if err != nil || !bytes.Equal(initial, other) {
						t.Fatalf("repeated Build changed diagnostic: %+v, %v", second, err)
					}
					if first == second {
						t.Fatal("independent calls share the diagnostic object")
					}
					first.Context.Path = "изменено вызывающим кодом"
					for i := range first.Related {
						first.Related[i].Line = -1
					}
					for i := range first.Details {
						first.Details[i].Value = "изменено"
					}
					unchanged, err := json.Marshal(second)
					if err != nil || !bytes.Equal(other, unchanged) {
						t.Fatal("mutating one diagnostic changed another call's diagnostic")
					}
					_, _, third := exercisedata.Build(kind, lines, base)
					fresh, err := json.Marshal(third)
					if err != nil || !bytes.Equal(initial, fresh) {
						t.Fatal("caller mutation leaked into a later Build")
					}
					if !bytes.Equal(before, exChoiceBindingSnapshot(t, lines)) {
						t.Fatal("Build or diagnostic mutation changed input")
					}
				})
			}
		}
	}
}

func TestExChoiceBindingSingleLinkedHintStillBuilds(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeQuestion, lesson.NodeMatching, lesson.NodeOrdering} {
		for _, block := range []bool{false, true} {
			for _, indent := range []string{"", " \t"} {
				t.Run(fmt.Sprintf("%s/block=%v/indent=%d", kind, block, len(indent)), func(t *testing.T) {
					lines, _ := exChoiceBindingFixture(kind, block, false, indent)
					before := exChoiceBindingSnapshot(t, lines)
					data, checkable, err := exercisedata.Build(kind, lines, diagnostic.Context{})
					if err != nil || !checkable {
						t.Fatalf("valid linked hint: checkable=%v err=%+v", checkable, err)
					}
					leaf := data[0]
					if kind != lesson.NodeQuestion {
						leaves, ok := data[0].Value.([]lesson.Data)
						if !ok || len(leaves) == 0 {
							t.Fatalf("expected nonempty collection: %+v", data)
						}
						leaf = leaves[0]
						if kind == lesson.NodeSingleChoice || kind == lesson.NodeMultipleChoice {
							if len(leaves) != 2 || leaves[0].Value != "Элемент" || leaves[0].Answers == nil || len(leaves[0].Answers) != 0 || leaves[0].Hint != "" {
								t.Fatalf("ordinary option changed: %+v", leaves)
							}
							leaf = leaves[1]
						}
					}
					answer := "Правильный"
					if kind == lesson.NodeMatching {
						answer = "X"
					}
					if kind == lesson.NodeOrdering {
						answer = "1"
					}
					if !slices.Equal(leaf.Answers, []string{answer}) || leaf.Hint != "Первая" {
						t.Errorf("linked answer/hint changed: %+v", leaf)
					}
					value := lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
						ID: "BA83", Version: "1", Title: "Проверка", LineCount: "210", CreatedAt: "2026-11-06T00:00:00Z",
						Pages: []lesson.Page{{ID: "ba83-p001", Nodes: []lesson.Node{{ID: "ba83-n001", Type: kind,
							Flags: lesson.NodeFlags{Checkable: &checkable}, Data: data, Nodes: []lesson.Node{}}}}},
					}}
					if problems := lesson.Validate(&value, diagnostic.Context{}); len(problems) != 0 {
						t.Fatalf("lesson.Validate: %+v", problems)
					}
					if !bytes.Equal(before, exChoiceBindingSnapshot(t, lines)) {
						t.Fatal("successful Build changed input")
					}
				})
			}
		}
	}
}

func exChoiceBindingEvidence(t *testing.T, err *diagnostic.Error, evidence []diagnostic.Context) {
	t.Helper()
	if err.Context != evidence[len(evidence)-1] {
		t.Errorf("Context = %+v; want repeated hint declaration %+v", err.Context, evidence[len(evidence)-1])
	}
	previous := -1
	for _, want := range evidence {
		at := slices.Index(err.Related, want)
		if at < 0 {
			t.Errorf("missing declaration %+v; Related = %+v", want, err.Related)
		} else if at <= previous {
			t.Errorf("declarations not in source order: %+v", err.Related)
		}
		previous = at
	}
	for i := 1; i < len(err.Related); i++ {
		if err.Related[i].Line < err.Related[i-1].Line {
			t.Errorf("Related not in source order: %+v", err.Related)
		}
	}
}

func exChoiceBindingSnapshot(t *testing.T, lines []model.Line) []byte {
	t.Helper()
	raw, err := json.Marshal(lines)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func exChoiceBindingFixture(kind lesson.NodeType, block, repeat bool, indent string) ([]model.Line, []diagnostic.Context) {
	var lines []model.Line
	add := func(typ model.LineType, parent int, tag, content string) int {
		line := model.Line{Number: 100 + len(lines)*10, LineType: typ, Raw: indent, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
		if parent != 0 {
			line.ParentLine = &parent
			line.NestingLevel = 1
			if parent != 100 {
				line.NestingLevel = 2
			}
		}
		element := func(typ model.ElementType, raw, value string) {
			start := utf8.RuneCountInString(line.Raw) + 1
			line.Raw += raw
			line.Elements = append(line.Elements, model.Element{ElementType: typ, Raw: raw, Value: &value,
				Start: start, End: utf8.RuneCountInString(line.Raw) + 1, ErrorIDs: []string{}})
		}
		if tag != "" {
			element(model.ElementTypeTag, "@"+tag, tag)
		}
		if content != "" {
			if tag != "" {
				line.Raw += " "
			}
			element(model.ElementTypeContent, content, content)
		}
		if typ == model.LineTypeBlockStart {
			line.Raw += " "
			element(model.ElementTypeBlockOpen, "{", "{")
		}
		if typ == model.LineTypeBlockEnd {
			element(model.ElementTypeBlockClose, "}", "}")
		}
		if typ == model.LineTypeSeparator {
			line.Raw += "---"
		}
		lines = append(lines, line)
		return line.Number
	}
	parent := 100
	if kind == lesson.NodeQuestion {
		add(model.LineTypeTag, 0, "question", "Вопрос")
		parent = 0
	} else {
		tag := map[lesson.NodeType]string{lesson.NodeSingleChoice: "choice", lesson.NodeMultipleChoice: "multichoice", lesson.NodeMatching: "matching", lesson.NodeOrdering: "ordering"}[kind]
		add(model.LineTypeBlockStart, 0, tag, "")
		add(model.LineTypeContent, parent, "", "Элемент")
	}
	var evidence []diagnostic.Context
	declaration := func(tag, value string) {
		typ, inline := model.LineTypeTag, value
		if block {
			typ, inline = model.LineTypeBlockStart, ""
		}
		number := add(typ, parent, tag, inline)
		evidence = append(evidence, diagnostic.Context{Path: "C:/Вход с пробелами/урок.json", FileName: "l001.json",
			LessonID: "BA83", PageID: "ba83-p001", Line: number, Column: utf8.RuneCountInString(indent) + 1})
		if block {
			add(model.LineTypeContent, number, "", value)
			add(model.LineTypeBlockEnd, number, "", "")
		}
	}
	answer := "Правильный"
	if kind == lesson.NodeMatching {
		answer = "X"
	}
	if kind == lesson.NodeOrdering {
		answer = "1"
	}
	declaration("answer", answer)
	declaration("hint", "Первая")
	if repeat {
		declaration("hint", "Повторная")
	}
	if kind == lesson.NodeMatching {
		add(model.LineTypeSeparator, parent, "", "")
		add(model.LineTypeContent, parent, "", "X")
		add(model.LineTypeContent, parent, "", "Y")
	}
	if kind == lesson.NodeOrdering {
		add(model.LineTypeContent, parent, "", "Второй")
		add(model.LineTypeTag, parent, "answer", "2")
	}
	if kind != lesson.NodeQuestion {
		add(model.LineTypeBlockEnd, 100, "", "")
	}
	return lines, evidence
}
