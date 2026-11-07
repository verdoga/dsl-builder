package app

import (
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/revision"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestSnapshotActionsFollowDecisionStages(t *testing.T) {
	op := snapshotFixtureOperation(t, Update, false, true)
	old, candidate := op.current.Files[0].Lesson(), op.current.Files[0].Lesson()
	candidate.Document.Title = "Новое название"
	var issues []*diagnostic.Error
	op.plan, issues = compare.Build(old, candidate, diagnostic.Context{}, diagnostic.Context{})
	snapshotFixtureOK(t, issues)
	op.oldFile, op.revised = op.current.Files[0], op.current.Files[0]
	op.fields = []revision.Decision{""}
	op.revisionReady = false
	a := op.Snapshot().Actions
	if !a.DecideFields || a.PrepareRevision || a.ReviewSave || a.Save {
		t.Fatalf("pending fields actions: %+v", a)
	}
	op.fields[0] = revision.Reject
	a = op.Snapshot().Actions
	if a.DecideFields || !a.PrepareRevision || a.ReviewSave || a.Save {
		t.Fatalf("answered fields actions: %+v", a)
	}
	op.revisionReady = true
	op.proposals, issues = catalog.ProposePlacement(op.placement, "l001.json", snapshotFixtureText("Иная секция"), nil)
	snapshotFixtureOK(t, issues)
	op.placementDecisions = []revision.Decision{""}
	a = op.Snapshot().Actions
	if !a.DecidePlacement || a.ReviewSave || a.Save || a.EditCatalog || a.EditSections {
		t.Fatalf("pending placement actions: %+v", a)
	}
	op.placementDecisions[0] = revision.Reject
	a = op.Snapshot().Actions
	if a.DecideFields || a.PrepareRevision || a.DecidePlacement || !a.EditCatalog || !a.EditSections || !a.SortSections {
		t.Fatalf("completed decisions actions: %+v", a)
	}
	if !op.Snapshot().NoChanges || a.ReviewSave || a.Save {
		t.Fatal("all rejected changes enabled an unchanged write")
	}
}

func TestSnapshotActionsExposeEditableConflictsAndRetry(t *testing.T) {
	for _, mode := range []Mode{Create, Add, Update} {
		t.Run(string(mode), func(t *testing.T) {
			op := snapshotFixtureOperation(t, mode, false, true)
			var issues []*diagnostic.Error
			op.placement, issues = catalog.SetLessonWeight(op.placement, "l002.json", "01")
			if op.placement == nil || len(issues) == 0 {
				t.Fatal("expected editable conflict")
			}
			op.refreshPlan()
			s := op.Snapshot()
			if len(s.Conflicts) != 1 || s.ManifestReady || s.Actions.ReviewSave || s.Actions.Save ||
				!s.Actions.EditCatalog || !s.Actions.SortSections || !s.Actions.Retry || !s.Actions.UnusedResources {
				t.Fatalf("conflict preview actions: %+v", s.Actions)
			}
			if s.Actions.EditSections != (mode != Create) || s.Actions.ChooseDestination != (mode == Create) {
				t.Fatalf("mode-specific actions: %+v", s.Actions)
			}
		})
	}
}

func TestSnapshotActionsTrackStagedEditAndSeparateSaveConsent(t *testing.T) {
	op := snapshotFixtureOperation(t, Create, false, false)
	a := op.Snapshot().Actions
	if !a.EditCatalog || a.EditSections || a.SortSections || !a.ChooseDestination || !a.ReviewSave || a.Save || !a.UnusedResources {
		t.Fatalf("ready create actions: %+v", a)
	}
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Кандидат")
	snapshotFixtureOK(t, issues)
	a = op.Snapshot().Actions
	if a.ReviewSave || a.Save || a.EditCatalog || a.EditSections {
		t.Fatalf("active edit actions: %+v", a)
	}
	snapshotFixtureOK(t, op.RejectEdit(edit.Token))
	preview, issues := op.ReviewSave(op.Snapshot().Token)
	snapshotFixtureOK(t, issues)
	if op.Snapshot().Actions.Save {
		t.Fatal("review alone enabled save")
	}
	snapshotFixtureOK(t, op.ConfirmSave(preview.Token, false, false))
	if op.Snapshot().Actions.Save || !op.Snapshot().Actions.ReviewSave {
		t.Fatal("rejected save did not return to preview")
	}
	preview, issues = op.ReviewSave(op.Snapshot().Token)
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, op.ConfirmSave(preview.Token, true, false))
	if !op.Snapshot().Actions.Save {
		t.Fatal("confirmed absent target did not enable save")
	}

	add := snapshotFixtureOperation(t, Add, false, false)
	add.lessonChanged = true
	preview, issues = add.ReviewSave(add.Snapshot().Token)
	snapshotFixtureOK(t, issues)
	if !preview.Destination.Exists {
		t.Fatal("fixture must overwrite its real archive")
	}
	if len(add.ConfirmSave(preview.Token, true, false)) == 0 || add.Snapshot().Actions.Save {
		t.Fatal("missing separate overwrite answer enabled save")
	}
	preview, issues = add.ReviewSave(add.Snapshot().Token)
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, add.ConfirmSave(preview.Token, true, true))
	if !add.Snapshot().Actions.Save {
		t.Fatal("separate overwrite confirmation did not enable save")
	}
}

func TestSnapshotActionsRestrictFinishedAndRecoveryStates(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    safewrite.Result
		finished  bool
		wantRetry bool
	}{
		{"installed", safewrite.Result{TargetPath: "target", Installed: true}, true, false},
		{"installed with cleanup error", safewrite.Result{TargetPath: "target", Installed: true, OriginalPath: "backup"}, true, false},
		{"temporary recovery", safewrite.Result{TargetPath: "target", TemporaryPath: "temporary"}, false, false},
		{"original recovery", safewrite.Result{TargetPath: "target", OriginalPath: "backup"}, false, false},
		{"completed rollback", safewrite.Result{TargetPath: "target", OriginalPath: "target"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op := snapshotFixtureOperation(t, Create, false, false)
			op.result, op.finished = &tc.result, tc.finished
			op.issues = []*diagnostic.Error{{Code: "test.write", Reason: "Ошибка записи"}}
			s := op.Snapshot()
			if !s.Actions.MainMenu || !s.Actions.Exit || s.Actions.Retry != tc.wantRetry || s.Actions.Save || s.Actions.ReviewSave {
				t.Fatalf("write outcome actions: %+v", s.Actions)
			}
			if s.WriteResult == nil || *s.WriteResult != tc.result || len(s.Blockers) != 1 {
				t.Fatal("actual write outcome or cleanup issue hidden")
			}
			if tc.finished && (s.Actions.Cancel || s.Actions.EditCatalog || s.Actions.ChooseDestination || s.Actions.DecideFields || s.Actions.PrepareRevision) {
				t.Fatalf("finished operation still offers mutation: %+v", s.Actions)
			}
		})
	}
}
