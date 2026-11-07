package compare_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/lesson"
)

func TestTask23OwnChangesContinueWithoutUnchangedParent(t *testing.T) {
	old, candidate := task23Lesson("", "later"), task23Lesson("", "later changed")
	old.Document.Pages[0].Nodes = []lesson.Node{task23Node("parent", task23Node("first"), task23Node("second"))}
	candidate.Document.Pages[0].Nodes = []lesson.Node{task23Node("parent", task23Node("first changed"), task23Node("second changed"))}
	oldChild := &old.Document.Pages[0].Nodes[0].Nodes[0]
	newChild := &candidate.Document.Pages[0].Nodes[0].Nodes[0]
	oldChild.Type, newChild.Type = lesson.NodeQuestion, lesson.NodeMultifill
	flag := false
	newChild.Flags.Checkable = &flag
	task23IDs(old, 0)
	task23IDs(candidate, 100)
	plan := task23Build(t, old, candidate)
	want := []compare.PageChange{
		{Index: 0, Nodes: []compare.NodeChange{
			{Path: []int{0, 0}, Fields: []compare.NodeField{compare.NodeTypeChanged, compare.NodeFlagsChanged, compare.NodeDataChanged}},
			{Path: []int{0, 1}, Fields: []compare.NodeField{compare.NodeDataChanged}},
		}},
		{Index: 1, Nodes: []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}},
	}
	if plan.ReplaceLesson || !plan.HasContentChanges() || len(plan.Fields)+len(plan.Reasons) != 0 || len(plan.Pages) != len(want) {
		t.Fatalf("plan: %+v", plan)
	}
	for i := range want {
		got := plan.Pages[i]
		if got.Index != want[i].Index || got.ReplaceNodes || len(got.Reasons) != 0 || !reflect.DeepEqual(got.Nodes, want[i].Nodes) {
			t.Fatalf("page %d: got %+v want %+v", i, got, want[i])
		}
	}
	independent := task23Build(t, old, candidate)
	saved := task23JSON(t, independent)
	plan.Pages[0].Nodes[0].Path[1] = 99
	plan.Pages[0].Nodes[0].Fields[0] = compare.NodeDataChanged
	if !reflect.DeepEqual(plan.Pages[0].Nodes[1].Path, []int{0, 1}) || task23JSON(t, independent) != saved {
		t.Fatal("node changes or independent plans share mutable slices")
	}
}

func TestTask23PositionalNodeSwapProducesTwoOwnChanges(t *testing.T) {
	old, candidate := task23Lesson(""), task23Lesson("")
	old.Document.Pages[0].Nodes = []lesson.Node{task23Node("A"), task23Node("B")}
	candidate.Document.Pages[0].Nodes = []lesson.Node{task23Node("B"), task23Node("A")}
	task23IDs(old, 0)
	task23IDs(candidate, 0)
	plan := task23Build(t, old, candidate)
	want := []compare.NodeChange{
		{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}},
		{Path: []int{1}, Fields: []compare.NodeField{compare.NodeDataChanged}},
	}
	if plan.ReplaceLesson || len(plan.Pages) != 1 || plan.Pages[0].ReplaceNodes || !reflect.DeepEqual(plan.Pages[0].Nodes, want) {
		t.Fatalf("positional swap: %+v", plan)
	}
}

func TestTask23StructuralReplacementHasReasonsAndNoFalseNodePairs(t *testing.T) {
	for _, tc := range []struct {
		name           string
		old, candidate []lesson.Node
		code           compare.ReasonCode
		pointer        string
	}{
		{"root count", []lesson.Node{task23Node("A")}, []lesson.Node{task23Node("A"), task23Node("B")}, compare.NodeCountChanged, "/document/pages/0/nodes"},
		{"nesting same total", []lesson.Node{task23Node("A", task23Node("B")), task23Node("C")}, []lesson.Node{task23Node("A"), task23Node("C", task23Node("B"))}, compare.NodeNestingChanged, "/document/pages/0/nodes/0/nodes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, candidate := task23Lesson("", "stable"), task23Lesson("", "stable changed")
			old.Document.Pages[0].Nodes, candidate.Document.Pages[0].Nodes = tc.old, tc.candidate
			candidate.Document.Pages[0].Title = task23Text("New title")
			task23IDs(old, 0)
			task23IDs(candidate, 0)
			plan := task23Build(t, old, candidate)
			if plan.ReplaceLesson || !plan.HasContentChanges() || len(plan.Reasons) != 0 || len(plan.Pages) != 2 || len(plan.Fields) != 1 {
				t.Fatalf("plan: %+v", plan)
			}
			page := plan.Pages[0]
			if page.Index != 0 || !page.ReplaceNodes || len(page.Nodes) != 0 || len(page.Reasons) == 0 {
				t.Fatalf("structural page: %+v", page)
			}
			found := false
			for _, reason := range page.Reasons {
				if reason.Code == tc.code && reason.OldPointer == tc.pointer && reason.NewPointer == tc.pointer {
					found = true
				}
				if !strings.ContainsAny(reason.Description, "абвгдежзийклмнопрстуфхцчшщэюя") || strings.Contains(reason.Description, `"value"`) {
					t.Fatalf("reason must explain in Russian without node JSON: %+v", reason)
				}
			}
			if !found {
				t.Fatalf("missing code/pointers %s %s: %+v", tc.code, tc.pointer, page.Reasons)
			}
			if plan.Pages[1].Index != 1 || plan.Pages[1].ReplaceNodes || len(plan.Pages[1].Nodes) != 1 {
				t.Fatalf("later page not compared: %+v", plan.Pages[1])
			}
			independent := task23Build(t, old, candidate)
			saved := task23JSON(t, independent)
			plan.Pages[0].Reasons[0].NewPointer = "changed"
			if task23JSON(t, independent) != saved {
				t.Fatal("reason slices shared across Build results")
			}
		})
	}
}
