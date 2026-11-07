package app

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/revision"
)

func updateFlowEqual(t *testing.T, got, want any) {
	t.Helper()
	left, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatalf("models differ:\ngot %s\nwant %s", left, right)
	}
}

func updateFlowCode(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return
		}
	}
	t.Fatalf("missing diagnostic %s: %+v", code, issues)
}

func updateFlowStart(t *testing.T, before, after model.Result, sections []*string, orders []string) (*Operation, *module.Module, *lesson.Lesson, string) {
	t.Helper()
	directory, archive, source, loaded, candidate := updateFixtureScenario(t, before, after, sections, orders)
	op, issues := NewUpdate(directory, archive, source)
	updateFixtureOK(t, issues)
	updateFixtureOK(t, op.Analyze(updateFixtureAt()))
	return op, loaded, candidate, archive
}

func updateFlowDecide(t *testing.T, op *Operation, choices []bool) {
	t.Helper()
	if len(op.Snapshot().Plan.Fields) != len(choices) {
		t.Fatalf("field count does not match explicit choices: %+v", op.Snapshot().Plan.Fields)
	}
	for index, choice := range choices {
		updateFixtureOK(t, op.DecideField(op.Snapshot().Token, index, choice))
	}
}

func TestUpdateComparePreservesWholePlanAndPendingNullField(t *testing.T) {
	before := updateFixtureResult("Старое", updateFixtureText("Прежний"), []*string{updateFixtureText("Известная")}, [][]string{{"Первый"}})
	after := updateFixtureResult("Новое", nil, []*string{updateFixtureText("Известная"), nil}, [][]string{{"Первый"}, {"Второй"}})
	op, loaded, candidate, _ := updateFlowStart(t, before, after, []*string{nil, nil}, []string{"010", "20"})
	want, issues := compare.Build(loaded.Files[0].Lesson(), candidate, loaded.Files[0].Context(), op.newFile.Context())
	updateFixtureOK(t, issues)
	snapshot := op.Snapshot()
	updateFlowEqual(t, snapshot.Plan, want)
	if !snapshot.Plan.ReplaceLesson || len(snapshot.Plan.Pages) != 0 {
		t.Fatal("added page must keep complete lesson replacement plan")
	}
	unknown := -1
	for index, field := range snapshot.Plan.Fields {
		if field.Field == compare.PageTitle && field.NewPage == 1 {
			if field.OldPage != -1 || field.Before != nil || field.After != nil {
				t.Fatalf("new unnamed page must retain explicit unmatched null field: %+v", field)
			}
			unknown = index
		}
		if snapshot.FieldAnswered[index] || snapshot.FieldAccepted[index] {
			t.Fatal("pending field was implicitly answered")
		}
	}
	if unknown < 0 || len(snapshot.FieldAnswered) != len(want.Fields) {
		t.Fatal("unmatched field was removed")
	}
	updateFlowCode(t, op.PrepareRevision(snapshot.Token), CodeDecision)
	updateFixtureOK(t, op.DecideField(op.Snapshot().Token, unknown, false))
	next := op.Snapshot()
	for index, answered := range next.FieldAnswered {
		if answered != (index == unknown) || next.FieldAccepted[index] {
			t.Fatal("one rejection answered another field or became acceptance")
		}
	}
	updateFlowEqual(t, next.Plan, want)
	updateFlowCode(t, op.PrepareRevision(next.Token), CodeDecision)
	preview, issues := op.ReviewSave(next.Token)
	if preview != nil || len(issues) == 0 {
		t.Fatal("pending metadata allowed save review")
	}
	result, issues := op.Save(updateFixtureAt())
	if result.Installed || len(issues) == 0 {
		t.Fatal("pending metadata allowed save")
	}
	for index, answered := range op.Snapshot().FieldAnswered {
		if !answered {
			updateFixtureOK(t, op.DecideField(op.Snapshot().Token, index, false))
		}
	}
	updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
	final := op.Snapshot()
	if final.After.Lesson().Document.Pages[1].Title != nil || !final.FieldAnswered[unknown] || final.FieldAccepted[unknown] {
		t.Fatal("explicit unmatched rejection restored a guessed old title")
	}
	updateFlowEqual(t, final.Plan, want)
}

func TestUpdateEachMetadataDecisionIsIndependent(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		t.Run(string(rune('0'+mask)), func(t *testing.T) {
			before := updateFixtureResult("Старое", updateFixtureText("Прежний"), []*string{updateFixtureText("До")}, [][]string{{"Материал"}})
			after := updateFixtureResult("Новое", nil, []*string{updateFixtureText("После")}, [][]string{{"Материал"}})
			op, loaded, _, _ := updateFlowStart(t, before, after, []*string{nil, nil}, []string{"010", "20"})
			plan := op.Snapshot().Plan
			if len(plan.Fields) != 3 || plan.Fields[0].Field != compare.DocumentTitle ||
				plan.Fields[1].Field != compare.DocumentSubtitle || plan.Fields[2].Field != compare.PageTitle ||
				plan.Fields[2].OldPage != 0 || plan.Fields[2].NewPage != 0 {
				t.Fatalf("unexpected exact field addresses: %+v", plan.Fields)
			}
			for index := range plan.Fields {
				updateFixtureOK(t, op.DecideField(op.Snapshot().Token, index, mask&(1<<index) != 0))
				snapshot := op.Snapshot()
				for field := range plan.Fields {
					if snapshot.FieldAnswered[field] != (field <= index) ||
						snapshot.FieldAccepted[field] != (field <= index && mask&(1<<field) != 0) {
						t.Fatal("decision leaked to another field")
					}
				}
				updateFlowEqual(t, snapshot.Plan, plan)
				updateFlowEqual(t, snapshot.After.Lesson(), loaded.Files[0].Lesson())
			}
			updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
			got := op.Snapshot()
			decisions := make([]revision.Decision, len(plan.Fields))
			for index := range decisions {
				decisions[index] = revision.Reject
				if mask&(1<<index) != 0 {
					decisions[index] = revision.Accept
				}
			}
			expected, changed, issues := revision.Apply(op.oldFile.Lesson(), op.newFile.Lesson(), plan, decisions, op.at, op.oldFile.Context(), op.newFile.Context())
			updateFixtureOK(t, issues)
			if changed != (mask != 0) {
				t.Fatal("metadata choices did not exercise expected changed state")
			}
			updateFlowEqual(t, got.After.Lesson(), expected)
			want := loaded.Files[0].Lesson()
			if mask&1 != 0 {
				want.Document.Title = "Новое"
			}
			if mask&2 != 0 {
				want.Document.Subtitle = nil
			}
			if mask&4 != 0 {
				want.Document.Pages[0].Title = updateFixtureText("После")
			}
			actual := got.After.Lesson().Document
			if actual.Title != want.Document.Title {
				t.Fatal("title rejection/acceptance not retained")
			}
			updateFlowEqual(t, actual.Subtitle, want.Document.Subtitle)
			updateFlowEqual(t, actual.Pages[0].Title, want.Document.Pages[0].Title)
			entry := updatePlacementItem(t, got.Catalog, loaded.Files[0].FileName()).Entry
			if entry.Title != actual.Title || entry.Version != actual.Version {
				t.Fatal("catalog did not sync final accepted metadata")
			}
			updateFlowEqual(t, entry.Subtitle, actual.Subtitle)
			if !slices.Equal(got.FieldAnswered, []bool{true, true, true}) || !got.RevisionReady {
				t.Fatal("complete explicit decisions not ready")
			}
		})
	}
}

func TestUpdateExplicitAcceptanceOfUnmatchedNullTitleIsNotPending(t *testing.T) {
	before := updateFixtureResult("Урок", nil, []*string{nil}, [][]string{{"Прежняя"}})
	after := updateFixtureResult("Урок", nil, []*string{nil, nil}, [][]string{{"Прежняя"}, {"Новая"}})
	op, _, _, _ := updateFlowStart(t, before, after, []*string{nil, nil}, []string{"10", "20"})
	snapshot := op.Snapshot()
	if len(snapshot.Plan.Fields) != 1 || snapshot.Plan.Fields[0].Field != compare.PageTitle ||
		snapshot.Plan.Fields[0].OldPage != -1 || snapshot.Plan.Fields[0].NewPage != 1 || snapshot.Plan.Fields[0].After != nil {
		t.Fatalf("fixture did not produce exact unmatched null field: %+v", snapshot.Plan.Fields)
	}
	updateFixtureOK(t, op.DecideField(snapshot.Token, 0, true))
	answered := op.Snapshot()
	if !answered.FieldAnswered[0] || !answered.FieldAccepted[0] || !answered.Actions.PrepareRevision {
		t.Fatal("explicit acceptance of null was mistaken for a missing decision")
	}
	updateFixtureOK(t, op.PrepareRevision(answered.Token))
	if !op.Snapshot().RevisionReady || op.Snapshot().After.Lesson().Document.Pages[1].Title != nil {
		t.Fatal("accepted null title could not be prepared")
	}
}
