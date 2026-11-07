package compare_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func task23Text(value string) *string { return &value }

func task23Data(value string) lesson.Data {
	role := lesson.RoleBody
	return lesson.Data{Type: lesson.DataText, Role: &role, Value: value, Answers: []string{}}
}

func task23Node(value string, children ...lesson.Node) lesson.Node {
	if children == nil {
		children = []lesson.Node{}
	}
	return lesson.Node{Type: lesson.NodeText, Data: []lesson.Data{task23Data(value)}, Nodes: children}
}

func task23Lesson(labels ...string) *lesson.Lesson {
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EFX7", Version: "1", LineCount: "0", CreatedAt: "2026-11-04T09:00:00Z", Pages: []lesson.Page{},
	}}
	for _, label := range labels {
		nodes := []lesson.Node{}
		if label != "" {
			nodes = append(nodes, task23Node(label))
		}
		value.Document.Pages = append(value.Document.Pages, lesson.Page{Nodes: nodes})
	}
	task23IDs(value, 0)
	return value
}

func task23IDs(value *lesson.Lesson, offset int) {
	number := offset
	var assign func([]lesson.Node)
	assign = func(nodes []lesson.Node) {
		for i := range nodes {
			number++
			nodes[i].ID = fmt.Sprintf("efx7-n%03d", number)
			assign(nodes[i].Nodes)
		}
	}
	for i := range value.Document.Pages {
		value.Document.Pages[i].ID = fmt.Sprintf("efx7-p%03d", offset+i+1)
		assign(value.Document.Pages[i].Nodes)
	}
}

func task23JSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func task23Build(t *testing.T, old, candidate *lesson.Lesson) *compare.Plan {
	t.Helper()
	for _, value := range []*lesson.Lesson{old, candidate} {
		if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
			t.Fatalf("invalid fixture: %+v", problems)
		}
	}
	beforeOld, beforeNew := task23JSON(t, old), task23JSON(t, candidate)
	plan, problems := compare.Build(old, candidate, diagnostic.Context{}, diagnostic.Context{})
	if len(problems) != 0 || plan == nil {
		t.Fatalf("Build: plan=%+v diagnostics=%+v", plan, problems)
	}
	if task23JSON(t, old) != beforeOld || task23JSON(t, candidate) != beforeNew {
		t.Fatal("Build mutated its inputs")
	}
	return plan
}

func task23Field(t *testing.T, got compare.FieldChange, field compare.Field, oldPage, newPage int, before, after *string) {
	t.Helper()
	want := compare.FieldChange{Field: field, OldPage: oldPage, NewPage: newPage, Before: before, After: after}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("field: got %+v want %+v", got, want)
	}
}

func TestTask23BuildReturnsBothSnapshotsDiagnosticsInOrder(t *testing.T) {
	oldContext := diagnostic.Context{Path: "old.zip", FileName: "l001.json", Line: 3, Column: 2}
	newContext := diagnostic.Context{Path: "candidate.json", FileName: "new.json", Line: 5, Column: 1}
	invalidOld, invalidNew := task23Lesson("old"), task23Lesson("new")
	invalidOld.Document.Pages[0].ID = "bad-page"
	invalidOld.Document.Version = "0"
	invalidNew.Document.Pages[0].Nodes[0].Data = nil
	invalidNew.Document.Pages[0].Nodes[0].ID = "bad-node"
	for _, tc := range []struct {
		name           string
		old, candidate *lesson.Lesson
	}{
		{"both nil", nil, nil}, {"old nil", nil, task23Lesson()},
		{"candidate nil", task23Lesson(), nil}, {"both invalid", invalidOld, invalidNew},
		{"old invalid", invalidOld, task23Lesson()}, {"candidate invalid", task23Lesson(), invalidNew},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := append(lesson.Validate(tc.old, oldContext), lesson.Validate(tc.candidate, newContext)...)
			beforeOld, beforeNew := task23JSON(t, tc.old), task23JSON(t, tc.candidate)
			plan, got := compare.Build(tc.old, tc.candidate, oldContext, newContext)
			if plan != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("plan=%+v diagnostics=%+v want=%+v", plan, got, want)
			}
			if task23JSON(t, tc.old) != beforeOld || task23JSON(t, tc.candidate) != beforeNew {
				t.Fatal("invalid inputs mutated")
			}
		})
	}
}

func TestTask23ServiceFieldsDoNotCreateChanges(t *testing.T) {
	old, candidate := task23Lesson("same", ""), task23Lesson("same", "")
	candidate.Document.ID = "efx7"
	candidate.Document.Version = json.Number("999999999999999999999999999999999999999999")
	candidate.Document.LineCount = json.Number("1234567890123456789012345678901234567890")
	candidate.Document.CreatedAt = "2026-11-05T18:20:00Z"
	task23IDs(candidate, 1000)
	candidate.Document.Pages[0].Nodes[0].ID += "-v99"
	plan := task23Build(t, old, candidate)
	if plan.HasContentChanges() || plan.ReplaceLesson || len(plan.Fields)+len(plan.Pages)+len(plan.Reasons) != 0 {
		t.Fatalf("service-only plan: %+v", plan)
	}
}

func TestTask23FieldsRemainIndependentProposals(t *testing.T) {
	old, candidate := task23Lesson("same", "same", ""), task23Lesson("same", "same", "")
	before, after := task23Text("old"), task23Text("new")
	old.Document.Title, candidate.Document.Title = *before, *after
	old.Document.Subtitle, candidate.Document.Subtitle = before, after
	old.Document.Pages[0].Title, candidate.Document.Pages[0].Title = before, after
	old.Document.Pages[1].Title, candidate.Document.Pages[1].Title = before, after
	candidate.Document.Pages[2].Title = task23Text("")
	plan, independent := task23Build(t, old, candidate), task23Build(t, old, candidate)
	if plan.HasContentChanges() || plan.ReplaceLesson || len(plan.Pages)+len(plan.Reasons) != 0 || len(plan.Fields) != 5 {
		t.Fatalf("fields-only plan: %+v", plan)
	}
	task23Field(t, plan.Fields[0], compare.DocumentTitle, -1, -1, before, after)
	task23Field(t, plan.Fields[1], compare.DocumentSubtitle, -1, -1, before, after)
	for i := 0; i < 2; i++ {
		task23Field(t, plan.Fields[i+2], compare.PageTitle, i, i, before, after)
	}
	task23Field(t, plan.Fields[4], compare.PageTitle, 2, 2, nil, task23Text(""))
	oldJSON, newJSON, otherJSON := task23JSON(t, old), task23JSON(t, candidate), task23JSON(t, independent)
	*plan.Fields[0].Before, *plan.Fields[0].After = "changed old", "changed new"
	if *plan.Fields[1].Before != "old" || *plan.Fields[1].After != "new" {
		t.Fatal("different field decisions share pointers")
	}
	if task23JSON(t, old) != oldJSON || task23JSON(t, candidate) != newJSON || task23JSON(t, independent) != otherJSON {
		t.Fatal("result shares mutable memory with input or another result")
	}
	*before, *after = "input old changed", "input new changed"
	if *independent.Fields[2].Before != "old" || *independent.Fields[2].After != "new" {
		t.Fatal("input mutation changed retained result")
	}
}

func TestTask23EmptyDocumentTitleIsAValidProposal(t *testing.T) {
	old, candidate := task23Lesson(), task23Lesson()
	candidate.Document.Title = "Title"
	plan := task23Build(t, old, candidate)
	if len(plan.Fields) != 1 || plan.HasContentChanges() {
		t.Fatalf("plan: %+v", plan)
	}
	task23Field(t, plan.Fields[0], compare.DocumentTitle, -1, -1, task23Text(""), task23Text("Title"))
}
