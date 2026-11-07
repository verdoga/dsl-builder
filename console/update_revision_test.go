package console

import (
	"bytes"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/preview"
	"github.com/verdoga/dsl-builder/revision"
)

func TestConsoleUpdatePrepareOnlyWithExplicitPermission(t *testing.T) {
	before := consoleUpdateTestPlain()
	after := consoleUpdateTestResult("Новое", nil, []*string{nil}, [][]string{{"Материал", "Второй"}})
	op := consoleUpdateTestOperation(t, before, after, false)
	snapshot := op.Snapshot()
	if snapshot.Actions.PrepareRevision {
		t.Fatal("fixture unexpectedly allows preparation before field decision")
	}
	var out bytes.Buffer
	if err := consoleUpdateTestDialog(op, "", &out).prepareRevision(snapshot); err != nil {
		t.Fatal(err)
	}
	if op.Snapshot().RevisionReady || op.Snapshot().Token != snapshot.Token || out.Len() != 0 {
		t.Fatal("disabled helper called owner or produced preparation diagnostics")
	}
}

func TestConsoleUpdatePrepareRealNodePageAndLessonRevision(t *testing.T) {
	for _, kind := range []string{"node", "page", "lesson"} {
		t.Run(kind, func(t *testing.T) {
			before := consoleUpdateTestPlain()
			after := consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Изменённый", "Второй"}})
			if kind == "page" {
				after = consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Материал", "Второй", "Третий"}})
			}
			if kind == "lesson" {
				after = consoleUpdateTestResult("Прежнее", nil, []*string{nil, nil}, [][]string{{"Материал", "Второй"}, {"Новая страница"}})
			}
			op := consoleUpdateTestOperation(t, before, after, false)
			snapshot := op.Snapshot()
			if snapshot.Plan == nil || snapshot.RevisionReady {
				t.Fatal("fixture needs an unprepared owner plan")
			}
			switch kind {
			case "node":
				if snapshot.Plan.ReplaceLesson || len(snapshot.Plan.Pages) != 1 || snapshot.Plan.Pages[0].ReplaceNodes {
					t.Fatal("fixture did not request node replacement")
				}
			case "page":
				if snapshot.Plan.ReplaceLesson || len(snapshot.Plan.Pages) != 1 || !snapshot.Plan.Pages[0].ReplaceNodes {
					t.Fatal("fixture did not request page replacement")
				}
			case "lesson":
				if !snapshot.Plan.ReplaceLesson {
					t.Fatal("fixture did not request full lesson replacement")
				}
			}
			// Подготовка fixture через owner API не является циклом диалога.
			for index := range snapshot.Plan.Fields {
				consoleUpdateTestOK(t, op.DecideField(op.Snapshot().Token, index, false))
			}
			snapshot = op.Snapshot()
			if !snapshot.Actions.PrepareRevision {
				t.Fatal("fixture has not obtained explicit prepare permission")
			}
			var out bytes.Buffer
			if err := consoleUpdateTestDialog(op, "", &out).prepareRevision(snapshot); err != nil {
				t.Fatal(err)
			}
			got := op.Snapshot()
			if !got.RevisionReady || got.Token == snapshot.Token || got.After.Lesson().Document.Version != "6" ||
				got.Before.Lesson().Document.Version != "5" || got.Actions.Save || out.Len() != 0 {
				t.Fatalf("one successful call must publish one revision without retry or save: %q", out.String())
			}
			text, err := preview.Replacements(got.Before, got.After, got.Plan)
			if err != nil || text == "" {
				t.Fatalf("real prepared IDs are not presentable: %v", err)
			}
		})
	}
}

func TestConsoleUpdatePrepareRefusedTitleKeepsOldBytesAndNoChanges(t *testing.T) {
	before := consoleUpdateTestPlain()
	after := consoleUpdateTestResult("Новое", nil, []*string{nil}, [][]string{{"Материал", "Второй"}})
	op := consoleUpdateTestOperation(t, before, after, false)
	var out bytes.Buffer
	if err := consoleUpdateTestDialog(op, "2\n", &out).decideField(op.Snapshot()); err != nil {
		t.Fatal(err)
	}
	snapshot := op.Snapshot()
	if snapshot.NoChanges || snapshot.RevisionReady || !snapshot.Actions.PrepareRevision {
		t.Fatal("field refusal must still wait for explicit revision preparation")
	}
	out.Reset()
	if err := consoleUpdateTestDialog(op, "", &out).prepareRevision(snapshot); err != nil {
		t.Fatal(err)
	}
	got := op.Snapshot()
	if !got.NoChanges || !got.RevisionReady || got.Actions.ReviewSave || got.Actions.Save ||
		got.After.Lesson().Document.Title != "Прежнее" || got.After.Lesson().Document.Version != "5" ||
		!bytes.Equal(got.After.OriginalBytes(), snapshot.Before.OriginalBytes()) || out.Len() != 0 {
		t.Fatal("explicit no-op preparation did not retain old lesson and bytes")
	}
}

func TestConsoleUpdatePrepareOriginalTokenAndCopiedOperation(t *testing.T) {
	for _, kind := range []string{"stale", "copied"} {
		t.Run(kind, func(t *testing.T) {
			before := consoleUpdateTestPlain()
			after := consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Новый материал", "Второй"}})
			op := consoleUpdateTestOperation(t, before, after, false)
			snapshot := op.Snapshot()
			if !snapshot.Actions.PrepareRevision {
				t.Fatal("fixture must permit preparation")
			}
			if kind == "stale" {
				edit, issues := op.ProposeCourseTitle(snapshot.Token, "Другой курс")
				consoleUpdateTestOK(t, issues)
				consoleUpdateTestOK(t, op.AcceptEdit(edit.Token))
			} else {
				copied := *op
				op = &copied
			}
			var out bytes.Buffer
			if err := consoleUpdateTestDialog(op, "", &out).prepareRevision(snapshot); err != nil {
				t.Fatal(err)
			}
			if op.Snapshot().RevisionReady || !strings.Contains(out.String(), app.CodeStaleReview) {
				t.Fatal("owner rejection bypassed by obtaining a new token")
			}
		})
	}
}

func TestConsoleUpdatePrepareOwnerFailureDoesNotRetry(t *testing.T) {
	before := consoleUpdateTestPlain()
	after := consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Новый материал", "Второй"}})
	op := consoleUpdateTestOperation(t, before, after, false, true)
	snapshot := op.Snapshot()
	if !snapshot.Actions.PrepareRevision {
		t.Fatal("fixture requires explicit preparation permission")
	}
	var out bytes.Buffer
	if err := consoleUpdateTestDialog(op, "", &out).prepareRevision(snapshot); err != nil {
		t.Fatal(err)
	}
	got := op.Snapshot()
	if got.RevisionReady || got.Token != snapshot.Token || !got.Actions.PrepareRevision ||
		strings.Count(out.String(), revision.CodeHistory) != 1 || got.WriteResult != nil {
		t.Fatalf("owner failure must be shown once without retry: %q", out.String())
	}
}

func TestConsoleUpdatePrepareShowsAllOwnerBlockers(t *testing.T) {
	op := consoleUpdateTestOperation(t, consoleUpdateTestPlain(), consoleUpdateTestPlain(), false)
	edit, _ := op.ProposeSection(op.Snapshot().Token, "Новая секция")
	if edit == nil {
		t.Fatal("owner refused editable section candidate")
	}
	if issues := op.AcceptEdit(edit.Token); len(issues) == 0 {
		t.Fatal("fixture should require distribution")
	}
	edit, _ = op.ProposeMove(op.Snapshot().Token, "l001.json", consoleUpdateTestText("Новая секция"))
	if edit == nil {
		t.Fatal("owner refused editable move candidate")
	}
	op.AcceptEdit(edit.Token)
	snapshot := op.Snapshot()
	want := catalog.Validate(snapshot.Catalog)
	if len(want) < 2 || !snapshot.Actions.PrepareRevision {
		t.Fatalf("fixture needs multiple owner blockers and preparation permission: %+v", want)
	}
	var out bytes.Buffer
	if err := consoleUpdateTestDialog(op, "", &out).prepareRevision(snapshot); err != nil {
		t.Fatal(err)
	}
	for _, issue := range want {
		if !strings.Contains(out.String(), issue.Code) || !strings.Contains(out.String(), issue.Reason) {
			t.Fatalf("owner diagnostic omitted: %+v in %q", issue, out.String())
		}
	}
	if !op.Snapshot().RevisionReady || op.Snapshot().Actions.Save {
		t.Fatal("editable blocked revision became saveable or lost owner readiness")
	}
}
