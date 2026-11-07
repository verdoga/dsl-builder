package compare

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func TestTask21ValidationPreservesBothSnapshotsDiagnostics(t *testing.T) {
	oldContext := diagnostic.Context{Path: "старый.zip", FileName: "l002.json", LessonID: "known-old", PageID: "known-page", Line: 7, Column: 3}
	newContext := diagnostic.Context{Path: "новый.json", FileName: "candidate.json", LessonID: "known-new", PageID: "new-page", Line: 11, Column: 5}
	for _, name := range []string{"nil-old", "nil-candidate", "both-nil", "invalid-old", "invalid-candidate", "both-invalid"} {
		t.Run(name, func(t *testing.T) {
			old, candidate := task21Lesson(t, "old"), task21Lesson(t, "new")
			switch name {
			case "nil-old":
				old = nil
			case "nil-candidate":
				candidate = nil
			case "both-nil":
				old, candidate = nil, nil
			case "invalid-old":
				old.Document.Pages[0].Nodes[0].Data[0].Value = 21
				old.Document.Pages[0].ID = "invalid-page"
			case "invalid-candidate":
				candidate.Document.Pages[0].Nodes = nil
				candidate.Document.CreatedAt = "invalid-date"
			case "both-invalid":
				old.FormatVersion = "unsupported"
				old.Document.Pages[0].Nodes[0].ID = "invalid-node"
				candidate.Document.Version = json.Number("0")
				candidate.Document.Pages[0].Nodes[0].Data[0].Answers = nil
			}
			before := task21JSON(t, []*lesson.Lesson{old, candidate})
			want := append(lesson.Validate(old, oldContext), lesson.Validate(candidate, newContext)...)
			if len(want) == 0 {
				t.Fatal("invalid fixture unexpectedly validates")
			}
			if got := validateInputs(old, candidate, oldContext, newContext); !reflect.DeepEqual(got, want) {
				t.Fatalf("validateInputs diagnostics = %#v; want original ordered diagnostics %#v", got, want)
			}
			plan, got := Build(old, candidate, oldContext, newContext)
			if plan != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Build = %#v, %#v; want nil plan and original ordered diagnostics %#v", plan, got, want)
			}
			if after := task21JSON(t, []*lesson.Lesson{old, candidate}); after != before {
				t.Fatal("validation mutated a snapshot")
			}
		})
	}
}

func TestTask21ServiceOnlyAndEmptyInputsHaveEmptyPlans(t *testing.T) {
	for _, texts := range [][]string{{}, {"same", "same"}} {
		t.Run(fmt.Sprintf("pages-%d", len(texts)), func(t *testing.T) {
			old, candidate := task21Lesson(t, texts...), task21Lesson(t, texts...)
			candidate.Document.ID = "t21"
			candidate.Document.Version = json.Number("99999999999999999999999999999999999999")
			candidate.Document.LineCount = json.Number("1e100")
			candidate.Document.CreatedAt = "2026-11-05T12:34:56Z"
			for i := range candidate.Document.Pages {
				// Полные ID валидны, но повтор базового номера и будущие суффиксы
				// относятся к ValidateHistory потребителя, а не к compare.
				old.Document.Pages[i].Nodes[0].ID = fmt.Sprintf("t21-n001-v%02d", 99-i)
				candidate.Document.Pages[i].ID = fmt.Sprintf("t21-p%d", 1000+i)
				candidate.Document.Pages[i].Nodes[0].ID = fmt.Sprintf("t21-n%d-v99", 2000+i)
			}
			if got := validateInputs(old, candidate, diagnostic.Context{}, diagnostic.Context{}); len(got) != 0 {
				t.Fatalf("valid inputs diagnostics: %v", got)
			}
			plan := task21Build(t, old, candidate)
			if plan.ReplaceLesson || len(plan.Fields)+len(plan.Pages)+len(plan.Reasons) != 0 || plan.HasContentChanges() {
				t.Fatalf("service-only plan = %#v", plan)
			}
		})
	}
}

func TestTask21BuildCoordinatesMixedPagesAndFields(t *testing.T) {
	old, candidate := task21Lesson(t, "a", "b", "c"), task21Lesson(t, "A", "b", "C")
	old.Document.Pages[0].Title = task21String("before")
	candidate.Document.Title, candidate.Document.Subtitle = "new", task21String("")
	candidate.Document.Pages[0].Title, candidate.Document.Pages[1].Title = task21String("after"), task21String("")
	old.Document.Pages[0].Nodes[0].Nodes = []lesson.Node{task21Node("t21-n010", "child")}
	candidate.Document.Pages[0].Nodes[0].Nodes = []lesson.Node{task21Node("t21-n010", "CHILD")}
	candidate.Document.Pages[1].Nodes = append(candidate.Document.Pages[1].Nodes, task21Node("t21-n011", "extra"))
	flag := false
	candidate.Document.Pages[2].Nodes[0].Type = lesson.NodeQuestion
	candidate.Document.Pages[2].Nodes[0].Flags.Checkable = &flag
	plan := task21Build(t, old, candidate)
	wantFields := []FieldChange{
		{Field: DocumentTitle, OldPage: -1, NewPage: -1, Before: task21String(""), After: task21String("new")},
		{Field: DocumentSubtitle, OldPage: -1, NewPage: -1, After: task21String("")},
		{Field: PageTitle, OldPage: 0, NewPage: 0, Before: task21String("before"), After: task21String("after")},
		{Field: PageTitle, OldPage: 1, NewPage: 1, After: task21String("")},
	}
	if plan.ReplaceLesson || len(plan.Reasons) != 0 || !reflect.DeepEqual(plan.Fields, wantFields) || len(plan.Pages) != 3 || !plan.HasContentChanges() {
		t.Fatalf("mixed plan = %#v", plan)
	}
	wantNodes := [][]NodeChange{
		{{Path: []int{0}, Fields: []NodeField{NodeDataChanged}}, {Path: []int{0, 0}, Fields: []NodeField{NodeDataChanged}}},
		nil,
		{{Path: []int{0}, Fields: []NodeField{NodeTypeChanged, NodeFlagsChanged, NodeDataChanged}}},
	}
	for i, page := range plan.Pages {
		if page.Index != i || page.ReplaceNodes != (i == 1) || len(page.Nodes) != len(wantNodes[i]) {
			t.Fatalf("page %d = %#v", i, page)
		}
		if i == 1 {
			if len(page.Reasons) == 0 || page.Reasons[0].Code != NodeCountChanged || page.Reasons[0].OldPointer != "/document/pages/1/nodes" || page.Reasons[0].NewPointer != "/document/pages/1/nodes" {
				t.Fatalf("structural page reasons = %#v", page.Reasons)
			}
		} else if len(page.Reasons) != 0 || !reflect.DeepEqual(page.Nodes, wantNodes[i]) {
			t.Fatalf("point page %d = %#v", i, page)
		}
	}
}

func TestTask21FullReplacementKeepsIndependentFields(t *testing.T) {
	for _, name := range []string{"count", "moved", "ambiguous"} {
		t.Run(name, func(t *testing.T) {
			old, candidate := task21Lesson(t, "a", "b"), task21Lesson(t, "b", "a", "new")
			code := PageCountChanged
			if name == "moved" {
				candidate = task21Lesson(t, "b", "a")
				code = PageOrderChanged
			}
			if name == "ambiguous" {
				old, candidate = task21Lesson(t, "a", "a", "b"), task21Lesson(t, "a", "b", "b")
				code = PageCorrespondenceAmbiguous
			}
			old.Document.Title, old.Document.Subtitle = "old", task21String("subtitle")
			candidate.Document.Title = "new"
			old.Document.Pages[0].Title = task21String("A")
			if name != "ambiguous" {
				old.Document.Pages[1].Title = task21String("B")
				candidate.Document.Pages[0].Title = task21String("B")
				candidate.Document.Pages[1].Title = task21String("renamed A")
			}
			plan := task21Build(t, old, candidate)
			if !plan.ReplaceLesson || len(plan.Pages) != 0 || len(plan.Reasons) == 0 || plan.Reasons[0].Code != code || !plan.HasContentChanges() {
				t.Fatalf("full replacement = %#v", plan)
			}
			want := []FieldChange{
				{Field: DocumentTitle, OldPage: -1, NewPage: -1, Before: task21String("old"), After: task21String("new")},
				{Field: DocumentSubtitle, OldPage: -1, NewPage: -1, Before: task21String("subtitle")},
			}
			if name == "ambiguous" {
				for i := range candidate.Document.Pages {
					want = append(want, FieldChange{Field: PageTitle, OldPage: -1, NewPage: i})
				}
			} else {
				want = append(want, FieldChange{Field: PageTitle, OldPage: 0, NewPage: 1, Before: task21String("A"), After: task21String("renamed A")})
				if name == "count" {
					want = append(want, FieldChange{Field: PageTitle, OldPage: -1, NewPage: 2})
				}
			}
			if !reflect.DeepEqual(plan.Fields, want) {
				t.Fatalf("independent fields = %#v; want %#v", plan.Fields, want)
			}
		})
	}
}

func TestTask21RenamesPreserveDuplicatePagePositionsAndOwnValues(t *testing.T) {
	old, candidate := task21Lesson(t, "duplicate", "duplicate"), task21Lesson(t, "duplicate", "duplicate")
	shared := task21String("shared")
	old.Document.Subtitle, old.Document.Pages[0].Title, old.Document.Pages[1].Title = shared, shared, shared
	candidate.Document.Subtitle = task21String("new")
	candidate.Document.Pages[0].Title, candidate.Document.Pages[1].Title = task21String("first"), task21String("")
	plan := task21Build(t, old, candidate)
	if plan.ReplaceLesson || len(plan.Pages)+len(plan.Reasons) != 0 || plan.HasContentChanges() || len(plan.Fields) != 3 {
		t.Fatalf("rename-only plan = %#v", plan)
	}
	for i := 0; i < 2; i++ {
		if plan.Fields[i+1].OldPage != i || plan.Fields[i+1].NewPage != i {
			t.Fatalf("duplicate title used non-positional pair: %#v", plan.Fields)
		}
	}
	before := task21JSON(t, []*lesson.Lesson{old, candidate})
	second := task21Build(t, old, candidate)
	secondBefore := task21JSON(t, second)
	*plan.Fields[0].Before, *plan.Fields[0].After = "mutated before", "mutated after"
	if *plan.Fields[1].Before != "shared" || *plan.Fields[2].Before != "shared" || *plan.Fields[1].After != "first" {
		t.Fatal("independent field proposals share mutable values")
	}
	if task21JSON(t, []*lesson.Lesson{old, candidate}) != before || task21JSON(t, second) != secondBefore {
		t.Fatal("plan values alias snapshots or another Build result")
	}
}

func task21Build(t *testing.T, old, candidate *lesson.Lesson) *Plan {
	t.Helper()
	for _, value := range []*lesson.Lesson{old, candidate} {
		if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
			t.Fatalf("valid fixture rejected by lesson.Validate: %v", problems)
		}
	}
	before := task21JSON(t, []*lesson.Lesson{old, candidate})
	plan, problems := Build(old, candidate, diagnostic.Context{}, diagnostic.Context{})
	if plan == nil || len(problems) != 0 {
		t.Fatalf("Build = %#v, %v; want nonnil plan without errors", plan, problems)
	}
	if task21JSON(t, []*lesson.Lesson{old, candidate}) != before {
		t.Fatal("Build mutated snapshots")
	}
	return plan
}

func task21Lesson(t *testing.T, texts ...string) *lesson.Lesson {
	t.Helper()
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "T21", Version: json.Number("1"), LineCount: json.Number("0"), CreatedAt: "2026-11-04T00:00:00Z", Pages: []lesson.Page{},
	}}
	for i, text := range texts {
		value.Document.Pages = append(value.Document.Pages, lesson.Page{ID: fmt.Sprintf("t21-p%03d", i+1), Nodes: []lesson.Node{task21Node(fmt.Sprintf("t21-n%03d", i+1), text)}})
	}
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("base fixture invalid: %v", problems)
	}
	return value
}

func task21Node(id, text string) lesson.Node {
	role := lesson.RoleBody
	return lesson.Node{ID: id, Type: lesson.NodeText, Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: text, Answers: []string{}}}, Nodes: []lesson.Node{}}
}

func task21String(value string) *string { return &value }

func task21JSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
