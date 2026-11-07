package revision

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func validateTestLesson(t *testing.T) *lesson.Lesson {
	t.Helper()
	node := func(id string) lesson.Node {
		return lesson.Node{ID: id, Type: lesson.NodeText, Data: []lesson.Data{}, Nodes: []lesson.Node{}}
	}
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EFX7", Version: "5", LineCount: "0", CreatedAt: "2026-01-02T03:04:05Z",
		Pages: []lesson.Page{{ID: "efx7-p001", Nodes: []lesson.Node{node("efx7-n001"), node("efx7-n010")}}, {ID: "efx7-p009", Nodes: []lesson.Node{node("efx7-n1000")}}},
	}}
	value.Document.Pages[0].Nodes[0].Nodes = []lesson.Node{node("efx7-n003")}
	validateTestValid(t, value)
	return value
}

func validateTestValid(t *testing.T, value *lesson.Lesson) {
	t.Helper()
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("не выполнено предусловие lesson.Validate: %v", problems)
	}
}

func validateTestErrors(t *testing.T, problems []*diagnostic.Error, code string) {
	t.Helper()
	if code == "" {
		if len(problems) != 0 {
			t.Fatalf("неожиданные ошибки: %v", problems)
		}
		return
	}
	if len(problems) == 0 {
		t.Fatalf("нет ошибки %s", code)
	}
	for _, problem := range problems {
		if problem == nil || problem.Code != code || problem.Reason == "" {
			t.Fatalf("ожидалась диагностика %s: %#v", code, problem)
		}
	}
}

func TestValidateTestUpdateInputs(t *testing.T) {
	oldContext, newContext := diagnostic.Context{Path: "old.zip", FileName: "l001.json"}, diagnostic.Context{Path: "candidate.json"}
	for _, name := range []string{"valid", "case", "mismatch", "old history", "candidate history", "invalid both", "nil both"} {
		t.Run(name, func(t *testing.T) {
			old, candidate := validateTestLesson(t), validateTestLesson(t)
			switch name {
			case "case":
				candidate.Document.ID = "efx7"
			case "mismatch":
				candidate.Document.ID = "OTHER"
				candidate.Document.Pages = []lesson.Page{}
			case "old history":
				old.Document.Pages[0].Nodes[0].ID = "efx7-n001-v06"
			case "candidate history":
				candidate.Document.Pages[0].Nodes[0].ID = "efx7-n001-v06"
			case "invalid both":
				old.Document.Version = "0"
				candidate.Document.Pages[0].ID = "bad"
			case "nil both":
				old, candidate = nil, nil
			}
			if name != "invalid both" && name != "nil both" {
				validateTestValid(t, old)
				validateTestValid(t, candidate)
			}
			problems := validateUpdateInputs(old, candidate, oldContext, newContext)
			switch name {
			case "mismatch":
				validateTestErrors(t, problems, CodeLessonMismatch)
			case "old history":
				validateTestErrors(t, problems, CodeHistory)
			case "invalid both", "nil both":
				want := append(lesson.Validate(old, oldContext), lesson.Validate(candidate, newContext)...)
				if len(problems) != len(want) {
					t.Fatalf("каскад или потеря ошибок: %v, ожидалось %v", problems, want)
				}
				for i := range want {
					if problems[i].Code != want[i].Code || problems[i].Context != want[i].Context {
						t.Errorf("ошибка %d: %#v, ожидалось %#v", i, problems[i], want[i])
					}
				}
			default:
				validateTestErrors(t, problems, "")
			}
		})
	}
}

func TestValidateTestPlanShape(t *testing.T) {
	reasons := []compare.Reason{{Code: compare.PageCountChanged}}
	for _, tc := range []struct {
		name      string
		plan      *compare.Plan
		decisions []Decision
		fewer     bool
		code      string
	}{
		{"nil", nil, nil, false, CodePlan},
		{"empty", &compare.Plan{}, nil, false, ""},
		{"partial count", &compare.Plan{}, nil, true, CodePlan},
		{"full count", &compare.Plan{ReplaceLesson: true, Reasons: reasons}, nil, true, ""},
		{"full without reasons", &compare.Plan{ReplaceLesson: true}, nil, false, CodePlan},
		{"partial reasons", &compare.Plan{Reasons: reasons}, nil, false, CodePlan},
		{"full pages", &compare.Plan{ReplaceLesson: true, Reasons: reasons, Pages: []compare.PageChange{{Index: 0, ReplaceNodes: true, Reasons: reasons}}}, nil, false, CodePlan},
		{"extra decision", &compare.Plan{}, []Decision{Accept}, false, CodeDecision},
		{"missing decision", &compare.Plan{Fields: []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1}}}, nil, false, CodeDecision},
		{"empty decision", &compare.Plan{Fields: []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1}}}, []Decision{""}, false, CodeDecision},
		{"unknown decision", &compare.Plan{Fields: []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1}}}, []Decision{"yes"}, false, CodeDecision},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, candidate := validateTestLesson(t), validateTestLesson(t)
			if tc.fewer {
				candidate.Document.Pages = candidate.Document.Pages[:1]
			}
			validateTestValid(t, candidate)
			validateTestErrors(t, validatePlan(old, candidate, tc.plan, tc.decisions, diagnostic.Context{}), tc.code)
		})
	}
}

func TestValidateTestFieldAddresses(t *testing.T) {
	text := "значение, связь со снимками гарантирует compare.Build"
	for _, tc := range []struct {
		name   string
		full   bool
		fields []compare.FieldChange
		bad    bool
	}{
		{"title", false, []compare.FieldChange{{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: &text, After: &text}}, false},
		{"title before nil", false, []compare.FieldChange{{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, After: &text}}, true},
		{"title after nil", false, []compare.FieldChange{{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: &text}}, true},
		{"subtitle null", false, []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1}}, false},
		{"document old index", false, []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: 0, NewPage: -1}}, true},
		{"document new index", false, []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: 0}}, true},
		{"page null", false, []compare.FieldChange{{Field: compare.PageTitle, OldPage: 0, NewPage: 0}}, false},
		{"page unmatched null", true, []compare.FieldChange{{Field: compare.PageTitle, OldPage: -1, NewPage: 0}}, false},
		{"page matched moved", true, []compare.FieldChange{{Field: compare.PageTitle, OldPage: 1, NewPage: 0, Before: &text, After: &text}}, false},
		{"page unmatched before", true, []compare.FieldChange{{Field: compare.PageTitle, OldPage: -1, NewPage: 0, Before: &text}}, true},
		{"page partial unmatched", false, []compare.FieldChange{{Field: compare.PageTitle, OldPage: -1, NewPage: 0}}, true},
		{"page partial moved", false, []compare.FieldChange{{Field: compare.PageTitle, OldPage: 1, NewPage: 0}}, true},
		{"old below -1", true, []compare.FieldChange{{Field: compare.PageTitle, OldPage: -2, NewPage: 0}}, true},
		{"old outside", true, []compare.FieldChange{{Field: compare.PageTitle, OldPage: 2, NewPage: 0}}, true},
		{"new negative", true, []compare.FieldChange{{Field: compare.PageTitle, OldPage: 0, NewPage: -1}}, true},
		{"new outside", true, []compare.FieldChange{{Field: compare.PageTitle, OldPage: 0, NewPage: 2}}, true},
		{"unknown", false, []compare.FieldChange{{Field: "other", OldPage: -1, NewPage: -1}}, true},
		{"duplicate document", false, []compare.FieldChange{{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1}, {Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1}}, true},
		{"duplicate page", true, []compare.FieldChange{{Field: compare.PageTitle, OldPage: 0, NewPage: 0}, {Field: compare.PageTitle, OldPage: 1, NewPage: 0}}, true},
		{"distinct pages", false, []compare.FieldChange{{Field: compare.PageTitle, OldPage: 0, NewPage: 0}, {Field: compare.PageTitle, OldPage: 1, NewPage: 1}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &compare.Plan{ReplaceLesson: tc.full, Fields: tc.fields}
			code := ""
			if tc.bad {
				code = CodePlan
			}
			validateTestErrors(t, validateFieldAddresses(validateTestLesson(t), validateTestLesson(t), plan, diagnostic.Context{}), code)
		})
	}
}

func TestValidateTestPageChanges(t *testing.T) {
	reasons := []compare.Reason{{Code: compare.NodeNestingChanged}}
	nodes := []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}
	for _, tc := range []struct {
		name    string
		changes []compare.PageChange
		bad     bool
	}{
		{"none", nil, false}, {"point", []compare.PageChange{{Index: 0, Nodes: nodes}}, false},
		{"replacement", []compare.PageChange{{Index: 0, ReplaceNodes: true, Reasons: reasons}}, false},
		{"replacement missing reasons", []compare.PageChange{{Index: 0, ReplaceNodes: true}}, true},
		{"replacement with nodes", []compare.PageChange{{Index: 0, ReplaceNodes: true, Reasons: reasons, Nodes: nodes}}, true},
		{"point empty", []compare.PageChange{{Index: 0}}, true},
		{"point reasons", []compare.PageChange{{Index: 0, Nodes: nodes, Reasons: reasons}}, true},
		{"negative", []compare.PageChange{{Index: -1, Nodes: nodes}}, true},
		{"outside", []compare.PageChange{{Index: 2, Nodes: nodes}}, true},
		{"duplicate", []compare.PageChange{{Index: 0, Nodes: nodes}, {Index: 0, Nodes: nodes}}, true},
		{"descending", []compare.PageChange{{Index: 1, Nodes: nodes}, {Index: 0, Nodes: nodes}}, true},
		{"ascending", []compare.PageChange{{Index: 0, Nodes: nodes}, {Index: 1, Nodes: nodes}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := ""
			if tc.bad {
				code = CodePlan
			}
			validateTestErrors(t, validatePageChanges(validateTestLesson(t), validateTestLesson(t), tc.changes, diagnostic.Context{}), code)
		})
	}
	old, candidate := validateTestLesson(t), validateTestLesson(t)
	candidate.Document.Pages = candidate.Document.Pages[:1]
	validateTestValid(t, candidate)
	validateTestErrors(t, validatePageChanges(old, candidate, []compare.PageChange{{Index: 1, Nodes: nodes}}, diagnostic.Context{}), CodePlan)
}

func TestValidateTestNodeChanges(t *testing.T) {
	change := func(path ...int) compare.NodeChange {
		return compare.NodeChange{Path: path, Fields: []compare.NodeField{compare.NodeDataChanged}}
	}
	for _, tc := range []struct {
		name    string
		changes []compare.NodeChange
		bad     bool
	}{
		{"none", nil, false}, {"parent child sibling", []compare.NodeChange{change(0), change(0, 0), change(1)}, false},
		{"child alone", []compare.NodeChange{change(0, 0)}, false},
		{"empty path", []compare.NodeChange{change()}, true}, {"negative", []compare.NodeChange{change(-1)}, true},
		{"outside root", []compare.NodeChange{change(2)}, true}, {"outside child", []compare.NodeChange{change(0, 1)}, true},
		{"negative child", []compare.NodeChange{change(0, -1)}, true}, {"missing grandchild", []compare.NodeChange{change(0, 0, 0)}, true},
		{"duplicate", []compare.NodeChange{change(0), change(0)}, true},
		{"child before parent", []compare.NodeChange{change(0, 0), change(0)}, true},
		{"siblings descending", []compare.NodeChange{change(1), change(0, 0)}, true},
		{"empty fields", []compare.NodeChange{{Path: []int{0}}}, true},
		{"unknown field", []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{"id"}}}, true},
		{"duplicate field", []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged, compare.NodeDataChanged}}}, true},
		{"all fields", []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeTypeChanged, compare.NodeFlagsChanged, compare.NodeDataChanged}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, candidate := validateTestLesson(t), validateTestLesson(t)
			code := ""
			if tc.bad {
				code = CodePlan
			}
			validateTestErrors(t, validateNodeChanges(old.Document.Pages[0].Nodes, candidate.Document.Pages[0].Nodes, tc.changes, diagnostic.Context{}), code)
		})
	}
	for _, missing := range []string{"old", "candidate"} {
		t.Run("path missing in "+missing, func(t *testing.T) {
			old, candidate := validateTestLesson(t), validateTestLesson(t)
			if missing == "old" {
				old.Document.Pages[0].Nodes[0].Nodes = []lesson.Node{}
			} else {
				candidate.Document.Pages[0].Nodes[0].Nodes = []lesson.Node{}
			}
			validateTestValid(t, old)
			validateTestValid(t, candidate)
			validateTestErrors(t, validateNodeChanges(old.Document.Pages[0].Nodes, candidate.Document.Pages[0].Nodes, []compare.NodeChange{change(0, 0)}, diagnostic.Context{}), CodePlan)
		})
	}
}

func validateTestSnapshot(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestValidateTestBuildPlansAndImmutability(t *testing.T) {
	for _, mode := range []string{"field", "node", "page", "lesson"} {
		t.Run(mode, func(t *testing.T) {
			old, candidate := validateTestLesson(t), validateTestLesson(t)
			switch mode {
			case "field":
				candidate.Document.Title = "Новое название"
			case "node":
				candidate.Document.Pages[0].Nodes[0].Type = lesson.NodeNote
			case "page":
				candidate.Document.Pages[0].Nodes = []lesson.Node{}
			case "lesson":
				candidate.Document.Pages = candidate.Document.Pages[:1]
			}
			plan, problems := compare.Build(old, candidate, diagnostic.Context{}, diagnostic.Context{})
			validateTestErrors(t, problems, "")
			decisions := make([]Decision, len(plan.Fields))
			for i := range decisions {
				decisions[i] = Reject
			}
			before := validateTestSnapshot(t, old) + validateTestSnapshot(t, candidate) + validateTestSnapshot(t, plan)
			validateTestErrors(t, validatePlan(old, candidate, plan, decisions, diagnostic.Context{}), "")
			if before != validateTestSnapshot(t, old)+validateTestSnapshot(t, candidate)+validateTestSnapshot(t, plan) {
				t.Fatal("проверка изменила вход")
			}
		})
	}
}

func validateTestDetails(problems []*diagnostic.Error) string {
	var out strings.Builder
	for _, problem := range problems {
		for _, detail := range problem.Details {
			fmt.Fprintln(&out, detail.Value)
		}
	}
	return out.String()
}
