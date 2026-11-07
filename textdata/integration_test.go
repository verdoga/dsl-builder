package textdata_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/textdata"
)

type integrationQACase struct {
	Name        string
	Kind        lesson.NodeType
	NodeID      string
	Declaration *model.Line
	Lines       []model.Line
	Want        []lesson.Data
	Error       string
}

type integrationQAFixture struct {
	Lesson lesson.Lesson
	Cases  []integrationQACase
}

func integrationQALoad(t *testing.T) integrationQAFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/integration/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture integrationQAFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func integrationQAJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationQAData(t *testing.T, got, want []lesson.Data) {
	t.Helper()
	if got == nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Data = %#v; want %#v (nonnull slice)", got, want)
	}
	for i, datum := range got {
		if _, ok := datum.Value.(string); !ok || datum.Type != lesson.DataText || datum.Role == nil || datum.Answers == nil {
			t.Fatalf("Data[%d] violates text/role/answers contract: %#v", i, datum)
		}
		for j := 0; j < i; j++ {
			if datum.Role == got[j].Role {
				t.Fatalf("Data[%d] and Data[%d] share Role", i, j)
			}
		}
	}
}

func TestIntegrationQAPublicBuildContracts(t *testing.T) {
	fixture := integrationQALoad(t)
	seen := map[lesson.NodeType]bool{}
	context := diagnostic.Context{Path: `C:\уроки\вход.json`, LessonID: "EFX7", PageID: "efx7-p001"}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			before := integrationQAJSON(t, tc)
			got, problem := textdata.Build(tc.Kind, tc.Declaration, tc.Lines, context)
			if !bytes.Equal(before, integrationQAJSON(t, tc)) {
				t.Fatal("Build modified caller declaration or own lines")
			}
			if tc.Error != "" {
				if got != nil || problem == nil || problem.Code != tc.Error {
					t.Fatalf("Build = %#v, %v; want nil and %s", got, problem, tc.Error)
				}
				if problem.Reason == "" || problem.Context.Path != context.Path || problem.Context.LessonID != context.LessonID || problem.Context.PageID != context.PageID {
					t.Fatalf("diagnostic lost caller context or reason: %#v", problem)
				}
				return
			}
			if problem != nil {
				t.Fatal(problem)
			}
			seen[tc.Kind] = true
			integrationQAData(t, got, tc.Want)
			integrationQASerialization(t, got)
		})
	}
	for _, kind := range []lesson.NodeType{lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants, lesson.NodeVariant, lesson.NodeHeading, lesson.NodeText, lesson.NodeInstruction, lesson.NodeNote, lesson.NodeAlt, lesson.NodeScript, lesson.NodeHint, lesson.NodeAnswerKey, lesson.NodeExample, lesson.NodeWordList, lesson.NodeTable, lesson.NodeMultifill} {
		if !seen[kind] {
			t.Errorf("no successful public Build case for %s", kind)
		}
	}
}

func integrationQASerialization(t *testing.T, value []lesson.Data) {
	t.Helper()
	var fields []map[string]json.RawMessage
	if err := json.Unmarshal(integrationQAJSON(t, value), &fields); err != nil {
		t.Fatal(err)
	}
	for i, object := range fields {
		if len(object) != 5 {
			t.Fatalf("Data[%d] serialized extra/missing properties: %v", i, object)
		}
		for _, name := range []string{"type", "role", "value", "answers", "hint"} {
			if _, ok := object[name]; !ok {
				t.Fatalf("Data[%d] omitted %s", i, name)
			}
		}
		if string(object["answers"]) != "[]" {
			t.Fatalf("Data[%d] answers = %s", i, object["answers"])
		}
	}
	var roundTrip []lesson.Data
	if err := json.Unmarshal(integrationQAJSON(t, value), &roundTrip); err != nil {
		t.Fatal(err)
	}
	integrationQAData(t, roundTrip, value)
}

func TestIntegrationQACallsOwnTheirData(t *testing.T) {
	fixture := integrationQALoad(t)
	for _, tc := range fixture.Cases {
		if tc.Error != "" || len(tc.Want) == 0 {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			first, problem := textdata.Build(tc.Kind, tc.Declaration, tc.Lines, diagnostic.Context{})
			if problem != nil {
				t.Fatal(problem)
			}
			second, problem := textdata.Build(tc.Kind, tc.Declaration, tc.Lines, diagnostic.Context{})
			if problem != nil {
				t.Fatal(problem)
			}
			integrationQAData(t, first, tc.Want)
			integrationQAData(t, second, tc.Want)
			for i := range first {
				for j := range second {
					if first[i].Role == second[j].Role {
						t.Fatal("independent calls share Role storage")
					}
				}
				*first[i].Role = lesson.RoleHint
				first[i].Answers = append(first[i].Answers, "changed")
				second[i].Answers = append(second[i].Answers, "second")
				first[i].Answers[0] = string(*tc.Want[i].Role)
				if second[i].Answers[0] != "second" {
					t.Fatal("independent calls share Answers storage")
				}
				second[i].Answers = second[i].Answers[:0]
			}
			for i := range first {
				if first[i].Answers[0] != string(*tc.Want[i].Role) {
					t.Fatal("data elements share Answers storage")
				}
			}
			integrationQAData(t, second, tc.Want)
			third, problem := textdata.Build(tc.Kind, tc.Declaration, tc.Lines, diagnostic.Context{})
			if problem != nil {
				t.Fatal(problem)
			}
			integrationQAData(t, third, tc.Want)
		})
	}
}

func TestIntegrationQARealLessonValidation(t *testing.T) {
	fixture := integrationQALoad(t)
	context := diagnostic.Context{Path: "integration.lesson.json"}
	for _, tc := range fixture.Cases {
		if tc.Error != "" {
			continue
		}
		data, problem := textdata.Build(tc.Kind, tc.Declaration, tc.Lines, context)
		if problem != nil {
			t.Fatalf("%s: %v", tc.Name, problem)
		}
		fixture.Lesson.Document.Pages[0].Nodes = append(fixture.Lesson.Document.Pages[0].Nodes, lesson.Node{
			ID: tc.NodeID, Type: tc.Kind, Flags: lesson.NodeFlags{}, Data: data, Nodes: []lesson.Node{},
		})
	}
	before := integrationQAJSON(t, fixture.Lesson)
	if problems := lesson.Validate(&fixture.Lesson, context); len(problems) != 0 {
		t.Fatalf("real lesson.Validate rejected assembled explicit fixture: %v", problems)
	}
	if !bytes.Equal(before, integrationQAJSON(t, fixture.Lesson)) {
		t.Fatal("lesson.Validate modified the assembled lesson")
	}
	var decoded lesson.Lesson
	if err := json.Unmarshal(before, &decoded); err != nil {
		t.Fatal(err)
	}
	if problems := lesson.Validate(&decoded, context); len(problems) != 0 {
		t.Fatalf("serialized lesson failed validation: %v", problems)
	}
	*decoded.Document.Pages[0].Nodes[0].Data[0].Role = lesson.RoleInstruction
	if problems := lesson.Validate(&decoded, context); len(problems) == 0 {
		t.Fatal("real validator accepted instruction role in task data")
	}
}

// Совпадающие исходные номера не объединяют два выбранных caller экземпляра.
func TestIntegrationQASameNumberInclusionsStaySeparate(t *testing.T) {
	fixture := integrationQALoad(t)
	var firstCase, secondCase integrationQACase
	for _, tc := range fixture.Cases {
		if tc.Name == "named text title precedes body" {
			firstCase = tc
		}
		if tc.Name == "second inclusion same source numbers" {
			secondCase = tc
		}
	}
	if firstCase.Declaration == nil || secondCase.Declaration == nil || firstCase.Declaration.Number != secondCase.Declaration.Number || firstCase.Lines[0].Number != secondCase.Lines[0].Number {
		t.Fatal("fixture must contain two caller-selected inclusions with equal source numbers")
	}
	first, problem := textdata.Build(firstCase.Kind, firstCase.Declaration, firstCase.Lines, diagnostic.Context{Path: "one.json"})
	if problem != nil {
		t.Fatal(problem)
	}
	second, problem := textdata.Build(secondCase.Kind, secondCase.Declaration, secondCase.Lines, diagnostic.Context{Path: "two.json"})
	if problem != nil {
		t.Fatal(problem)
	}
	integrationQAData(t, first, firstCase.Want)
	integrationQAData(t, second, secondCase.Want)
	*first[0].Role = lesson.RoleHint
	first[1].Value = "изменение только первого включения"
	integrationQAData(t, second, secondCase.Want)
	again, problem := textdata.Build(firstCase.Kind, firstCase.Declaration, firstCase.Lines, diagnostic.Context{Path: "one.json"})
	if problem != nil {
		t.Fatal(problem)
	}
	integrationQAData(t, again, firstCase.Want)
}
