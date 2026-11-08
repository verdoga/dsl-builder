package app_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func matchingInstructionInput(text string, standalone, repeated, checked bool) model.Result {
	result := integrationInput("MATCH1", []string{})
	add := func(kind model.LineType, parent int, parts ...model.Element) int {
		line := model.Line{Number: len(result.Lines) + 1, LineType: kind, LineEnding: model.LineEndingLF, Elements: parts}
		if parent != 0 {
			line.ParentLine = &parent
			line.NestingLevel = result.Lines[parent-1].NestingLevel + 1
		}
		for index := range line.Elements {
			part := &line.Elements[index]
			if index > 0 {
				line.Raw += " "
			}
			part.Start = len([]rune(line.Raw)) + 1
			line.Raw += part.Raw
			part.End = len([]rune(line.Raw)) + 1
		}
		result.Lines = append(result.Lines, line)
		return line.Number
	}
	element := func(kind model.ElementType, raw string) model.Element {
		return model.Element{ElementType: kind, Raw: raw, Value: integrationText(raw), ErrorIDs: []string{}}
	}
	tag := func(name string) model.Element {
		part := element(model.ElementTypeTag, "@"+name)
		part.Value = integrationText(name)
		return part
	}
	content := func(value string) model.Element { return element(model.ElementTypeContent, value) }
	add(model.LineTypeTag, 2, tag("step"), element(model.ElementTypeTitle, "First"))
	parent := 2
	if repeated {
		add(model.LineTypeTag, 2, tag("include"), element(model.ElementTypeIdentifier, "F"))
		add(model.LineTypeTag, 2, tag("include"), element(model.ElementTypeIdentifier, "F"))
		parent = add(model.LineTypeBlockStart, 0, tag("fragment"), element(model.ElementTypeIdentifier, "F"), element(model.ElementTypeBlockOpen, "{"))
	}
	if standalone {
		add(model.LineTypeTag, parent, tag("instr"), content(text))
	}
	parts := []model.Element{tag("matching")}
	if text != "" {
		parts = append(parts, content(text))
	}
	parts = append(parts, element(model.ElementTypeBlockOpen, "{"))
	block := add(model.LineTypeBlockStart, parent, parts...)
	add(model.LineTypeContent, block, content("item"))
	if checked {
		add(model.LineTypeTag, block, tag("answer"), content("A"))
		add(model.LineTypeTag, block, tag("hint"), content("item hint"))
	}
	add(model.LineTypeSeparator, block, content("---"))
	add(model.LineTypeContent, block, content("A"))
	add(model.LineTypeContent, block, content("B"))
	add(model.LineTypeBlockEnd, block, element(model.ElementTypeBlockClose, "}"))
	if repeated {
		add(model.LineTypeBlockEnd, parent, element(model.ElementTypeBlockClose, "}"))
	}
	count := len(result.Lines)
	result.Document.LineCount = &count
	return result
}

func matchingInstructionText(role lesson.DataRole, value string, answers []string, hint string) lesson.Data {
	return lesson.Data{Type: lesson.DataText, Role: &role, Value: value, Answers: answers, Hint: hint}
}

func matchingInstructionNode(id, text string) lesson.Node {
	return lesson.Node{ID: id, Type: lesson.NodeInstruction, Nodes: []lesson.Node{}, Data: []lesson.Data{
		matchingInstructionText(lesson.RoleBody, text, []string{}, ""),
	}}
}

func matchingExerciseNode(id string, checked bool) lesson.Node {
	answers, hint := []string{}, ""
	if checked {
		answers, hint = []string{"A"}, "item hint"
	}
	items, options := lesson.RoleItems, lesson.RoleOptions
	return lesson.Node{ID: id, Type: lesson.NodeMatching, Flags: lesson.NodeFlags{Checkable: &checked}, Nodes: []lesson.Node{}, Data: []lesson.Data{
		{Type: lesson.DataCollection, Role: &items, Answers: []string{}, Value: []lesson.Data{
			matchingInstructionText(lesson.RoleItem, "item", answers, hint),
		}},
		{Type: lesson.DataCollection, Role: &options, Answers: []string{}, Value: []lesson.Data{
			matchingInstructionText(lesson.RoleOption, "A", []string{}, ""),
			matchingInstructionText(lesson.RoleOption, "B", []string{}, ""),
		}},
	}}
}

func matchingInstructionTree(taskID, stepID string, children ...lesson.Node) []lesson.Node {
	speaking, stepped := false, true
	return []lesson.Node{{ID: taskID, Type: lesson.NodeTask, Flags: lesson.NodeFlags{Speaking: &speaking, Stepped: &stepped}, Data: []lesson.Data{matchingInstructionText(lesson.RoleTitle, "a", []string{}, "")}, Nodes: []lesson.Node{
		{ID: stepID, Type: lesson.NodeStep, Data: []lesson.Data{matchingInstructionText(lesson.RoleTitle, "First", []string{}, "")}, Nodes: children},
	}}}
}

func matchingInstructionAssert(t *testing.T, value *lesson.Lesson, version string, want []lesson.Node) {
	t.Helper()
	if value.Document.ID != "MATCH1" || string(value.Document.Version) != version || len(value.Document.Pages) != 1 || value.Document.Pages[0].ID != "match1-p001" {
		t.Fatalf("wrong lesson identity/pages: %+v", value.Document)
	}
	actual, err := json.Marshal(value.Document.Pages[0].Nodes)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("tree/data/IDs:\ngot  %s\nwant %s", actual, expected)
	}
}

func TestIntegrationMatchingInstructionCreateAndAddZIP(t *testing.T) {
	for _, operation := range []string{"create", "add"} {
		for _, scenario := range []struct {
			name, text                    string
			standalone, repeated, checked bool
		}{
			{"present", "Match", false, false, true},
			{"absent", "", false, false, false},
			{"identical_standalone", "Match", true, false, false},
			{"repeated_include", "Match", false, true, true},
		} {
			t.Run(operation+"/"+scenario.name, func(t *testing.T) {
				base := t.TempDir()
				path := filepath.Join(base, "source", "matching.json")
				input := matchingInstructionInput(scenario.text, scenario.standalone, scenario.repeated, scenario.checked)
				input.Document.Metadata.Order = integrationText("30")
				integrationWriteInput(t, path, input)
				var op *app.Operation
				name := "l001.json"
				if operation == "create" {
					op = integrationCreate(t, base, filepath.Dir(path))
					integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "matching"))
				} else {
					archive, _ := integrationArchive(t, base)
					added, issues := app.NewAdd(base, archive, path)
					integrationOK(t, issues)
					op = added
					integrationOK(t, op.Analyze(integrationAt()))
					name = "l004.json"
				}
				children := []lesson.Node{}
				switch scenario.name {
				case "present":
					children = append(children, matchingInstructionNode("match1-n003", "Match"), matchingExerciseNode("match1-n004", true))
				case "absent":
					children = append(children, matchingExerciseNode("match1-n003", false))
				case "identical_standalone":
					children = append(children, matchingInstructionNode("match1-n003", "Match"), matchingInstructionNode("match1-n004", "Match"), matchingExerciseNode("match1-n005", false))
				case "repeated_include":
					children = append(children, matchingInstructionNode("match1-n003", "Match"), matchingExerciseNode("match1-n004", true), matchingInstructionNode("match1-n005", "Match"), matchingExerciseNode("match1-n006", true))
				}
				want := matchingInstructionTree("match1-n001", "match1-n002", children...)
				matchingInstructionAssert(t, integrationFile(t, op.Snapshot().Module, name).Lesson(), "1", want)
				value := integrationSave(t, op, operation == "add")
				matchingInstructionAssert(t, integrationFile(t, value, name).Lesson(), "1", want)
			})
		}
	}
}

func TestIntegrationMatchingInstructionUpdateZIP(t *testing.T) {
	for _, scenario := range []struct {
		name, before, after string
		want                []lesson.Node
	}{
		{"text", "Match", "Changed", matchingInstructionTree("match1-n001", "match1-n002", matchingInstructionNode("match1-n003-v02", "Changed"), matchingExerciseNode("match1-n004", true))},
		{"removed", "Match", "", matchingInstructionTree("match1-n005-v02", "match1-n006-v02", matchingExerciseNode("match1-n007-v02", true))},
		{"added", "", "Match", matchingInstructionTree("match1-n004-v02", "match1-n005-v02", matchingInstructionNode("match1-n006-v02", "Match"), matchingExerciseNode("match1-n007-v02", true))},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			base := t.TempDir()
			path := filepath.Join(base, "source", "matching.json")
			integrationWriteInput(t, path, matchingInstructionInput(scenario.before, false, false, true))
			create := integrationCreate(t, base, filepath.Dir(path))
			integrationOK(t, create.SetDestination(create.Snapshot().Token, base, "matching"))
			integrationSave(t, create, false)
			integrationWriteInput(t, path, matchingInstructionInput(scenario.after, false, false, true))
			op, issues := app.NewUpdate(base, filepath.Join(base, "matching.zip"), path)
			integrationOK(t, issues)
			integrationOK(t, op.Analyze(integrationAt()))
			plan := op.Snapshot().Plan
			if plan.ReplaceLesson || len(plan.Fields) != 0 || len(plan.Pages) != 1 || plan.Pages[0].ReplaceNodes != (scenario.name != "text") {
				t.Fatalf("wrong revision plan: %+v", plan)
			}
			integrationOK(t, op.PrepareRevision(op.Snapshot().Token))
			matchingInstructionAssert(t, op.Snapshot().After.Lesson(), "2", scenario.want)
			value := integrationSave(t, op, true)
			matchingInstructionAssert(t, integrationFile(t, value, "l001.json").Lesson(), "2", scenario.want)
		})
	}
}
