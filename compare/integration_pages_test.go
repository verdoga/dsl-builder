package compare_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
)

func TestTask23FullReplacementPreservesIndependentFieldProposals(t *testing.T) {
	for _, tc := range []struct {
		name           string
		old, candidate []string
		oldPages       []int
		code           compare.ReasonCode
	}{
		{"added pages", []string{"A", "B"}, []string{"B", "C", "D"}, []int{1, -1, -1}, compare.PageCountChanged},
		{"removed pages", []string{"A", "B", "C"}, []string{"C", "A"}, []int{2, 0}, compare.PageCountChanged},
		{"unique move", []string{"A", "B", "C"}, []string{"C", "A", "B"}, []int{2, 0, 1}, compare.PageOrderChanged},
		{"ambiguous duplicates", []string{"A", "A", "X"}, []string{"Y", "A", "A"}, []int{-1, -1, -1}, compare.PageCorrespondenceAmbiguous},
		{"one old to several new", []string{"A", "B"}, []string{"A", "A", "C"}, []int{-1, -1, -1}, compare.PageCountChanged},
		{"several old to one new", []string{"A", "A"}, []string{"A", "B", "C"}, []int{-1, -1, -1}, compare.PageCountChanged},
		{"duplicate empty pages", []string{"", "", "A"}, []string{"", "A"}, []int{-1, 2}, compare.PageCountChanged},
		{"new empty page", []string{"A"}, []string{"A", ""}, []int{0, -1}, compare.PageCountChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, candidate := task23Lesson(tc.old...), task23Lesson(tc.candidate...)
			old.Document.Title, candidate.Document.Title = "Old document", "New document"
			candidate.Document.Subtitle = task23Text("")
			for i := range old.Document.Pages {
				old.Document.Pages[i].Title = task23Text(fmt.Sprintf("old %d", i))
			}
			for i := range candidate.Document.Pages {
				candidate.Document.Pages[i].Title = task23Text(fmt.Sprintf("new %d", i))
				if tc.oldPages[i] == -1 {
					candidate.Document.Pages[i].Title = nil
				}
			}
			plan := task23Build(t, old, candidate)
			if !plan.ReplaceLesson || !plan.HasContentChanges() || len(plan.Pages) != 0 || len(plan.Reasons) == 0 || len(plan.Fields) != len(tc.candidate)+2 {
				t.Fatalf("replacement plan: %+v", plan)
			}
			task23Field(t, plan.Fields[0], compare.DocumentTitle, -1, -1, task23Text("Old document"), task23Text("New document"))
			task23Field(t, plan.Fields[1], compare.DocumentSubtitle, -1, -1, nil, task23Text(""))
			for i, oldPage := range tc.oldPages {
				var before *string
				if oldPage >= 0 {
					before = old.Document.Pages[oldPage].Title
				}
				task23Field(t, plan.Fields[i+2], compare.PageTitle, oldPage, i, before, candidate.Document.Pages[i].Title)
			}
			for _, reason := range plan.Reasons {
				if reason.Code != tc.code || !strings.HasPrefix(reason.OldPointer, "/document/pages") || !strings.HasPrefix(reason.NewPointer, "/document/pages") {
					t.Fatalf("reason code/pointers: %+v", reason)
				}
				if !strings.ContainsAny(reason.Description, "абвгдежзийклмнопрстуфхцчшщэюя") {
					t.Fatalf("reason is not Russian: %+v", reason)
				}
			}
			if tc.code == compare.PageCountChanged {
				if len(plan.Reasons) != 1 || plan.Reasons[0].OldPointer != "/document/pages" || plan.Reasons[0].NewPointer != "/document/pages" {
					t.Fatalf("page count reason: %+v", plan.Reasons)
				}
			}
			if tc.code == compare.PageOrderChanged {
				if len(plan.Reasons) != len(tc.oldPages) {
					t.Fatalf("moved pairs: %+v", plan.Reasons)
				}
				for i, oldPage := range tc.oldPages {
					if plan.Reasons[i].OldPointer != fmt.Sprintf("/document/pages/%d", oldPage) || plan.Reasons[i].NewPointer != fmt.Sprintf("/document/pages/%d", i) {
						t.Fatalf("move order/addresses: %+v", plan.Reasons)
					}
				}
			}
			again := task23Build(t, old, candidate)
			if !reflect.DeepEqual(plan, again) {
				t.Fatalf("nondeterministic plan: %+v / %+v", plan, again)
			}
			plan.Reasons[0].Description = "changed result"
			if again.Reasons[0].Description == "changed result" {
				t.Fatal("full replacement reasons share result storage")
			}
		})
	}
}

func TestTask23FullReplacementDoesNotGuessUnknownOldTitle(t *testing.T) {
	old, candidate := task23Lesson("A", "A", ""), task23Lesson("A", "", "", "new")
	for i := range old.Document.Pages {
		old.Document.Pages[i].Title = task23Text("same title")
	}
	candidate.Document.Pages[0].Title = task23Text("same title")
	plan := task23Build(t, old, candidate)
	if !plan.ReplaceLesson || len(plan.Fields) != 4 {
		t.Fatalf("unknown title proposals: %+v", plan)
	}
	for i, field := range plan.Fields {
		task23Field(t, field, compare.PageTitle, -1, i, nil, candidate.Document.Pages[i].Title)
	}
}

func TestTask23SafeUnchangedTitleNeedsNoProposalDuringReplacement(t *testing.T) {
	old, candidate := task23Lesson("A", "B"), task23Lesson("B", "A", "new")
	old.Document.Pages[0].Title, candidate.Document.Pages[1].Title = task23Text("A title"), task23Text("A title")
	plan := task23Build(t, old, candidate)
	if !plan.ReplaceLesson || len(plan.Fields) != 1 {
		t.Fatalf("safe unchanged title: %+v", plan)
	}
	task23Field(t, plan.Fields[0], compare.PageTitle, -1, 2, nil, nil)
}

func TestTask23RenamingDuplicatesAndEmptyPagesKeepsPositions(t *testing.T) {
	for _, labels := range [][]string{{}, {"A", "A", "", ""}, {"", ""}} {
		old, candidate := task23Lesson(labels...), task23Lesson(labels...)
		for i := range candidate.Document.Pages {
			old.Document.Pages[i].Title = task23Text(fmt.Sprintf("old %d", i))
			candidate.Document.Pages[i].Title = task23Text(fmt.Sprintf("new %d", i))
		}
		plan := task23Build(t, old, candidate)
		if plan.ReplaceLesson || plan.HasContentChanges() || len(plan.Pages)+len(plan.Reasons) != 0 || len(plan.Fields) != len(labels) {
			t.Fatalf("renaming labels %q: %+v", labels, plan)
		}
		for i, field := range plan.Fields {
			task23Field(t, field, compare.PageTitle, i, i, old.Document.Pages[i].Title, candidate.Document.Pages[i].Title)
		}
	}
}

func TestTask23CompletelyEditedPagesDoNotProveMovement(t *testing.T) {
	old, candidate := task23Lesson("A", "B", "C"), task23Lesson("X", "Y", "Z")
	plan := task23Build(t, old, candidate)
	if plan.ReplaceLesson || len(plan.Reasons) != 0 || len(plan.Pages) != 3 {
		t.Fatalf("several fully edited pages: %+v", plan)
	}
	for i, page := range plan.Pages {
		if page.Index != i || page.ReplaceNodes || len(page.Reasons) != 0 || len(page.Nodes) != 1 || !reflect.DeepEqual(page.Nodes[0].Path, []int{0}) || !reflect.DeepEqual(page.Nodes[0].Fields, []compare.NodeField{compare.NodeDataChanged}) {
			t.Fatalf("positional change %d: %+v", i, page)
		}
	}
}
