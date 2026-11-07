package app

import (
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/revision"
)

func TestSnapshotRefreshPreservesFullCompositionWhenExportIsBlocked(t *testing.T) {
	op := snapshotFixtureOperation(t, Add, false, false)
	op.placement.CourseTitle = "Изменённый курс"
	op.placement.Items[1].Entry.Order = nil
	op.issues = []*diagnostic.Error{{Code: "test.obsolete", Reason: "Устаревшая блокировка"}}
	issues := op.refreshPlan()
	if len(issues) == 0 {
		t.Fatal("missing weight did not block refresh")
	}
	s := op.Snapshot()
	if s.ManifestReady || s.Module == nil || len(s.Module.Files) != 2 || s.Catalog == nil || len(s.Catalog.Items) != 2 {
		t.Fatalf("blocked export hid complete editable composition: %+v", s)
	}
	if len(s.ManifestChanges) < 2 || len(s.Blockers) == 0 || s.Actions.ReviewSave || s.Actions.Save {
		t.Fatal("blocked export hid changes/diagnostics or enabled writing")
	}
	for _, issue := range s.Blockers {
		if issue.Code == "test.obsolete" {
			t.Fatal("refresh retained obsolete blocker")
		}
	}
	op.placement.Items[1].Entry.Order = snapshotFixtureText("2")
	snapshotFixtureOK(t, op.refreshPlan())
	ready := op.Snapshot()
	if !ready.ManifestReady || len(ready.Blockers) != 0 || ready.NoChanges {
		t.Fatalf("fixed plan did not refresh readiness/blockers: %+v", ready)
	}
}

func TestSnapshotRefreshUsesStableBaselineTimeAndIgnoresInternalWeights(t *testing.T) {
	op := snapshotFixtureOperation(t, Update, true, true)
	op.plan = &compare.Plan{}
	op.oldFile, op.revised = op.current.Files[0], op.current.Files[0]
	op.current.Manifest.CreatedAt = "2026-11-07T23:59:59Z"
	op.placement.Sections[0].Weight = snapshotFixtureText("000001")
	snapshotFixtureOK(t, op.refreshPlan())
	s := op.Snapshot()
	if !s.ManifestReady || !s.NoChanges || len(s.ManifestChanges) != 0 || s.Actions.ReviewSave || s.Actions.Save {
		t.Fatalf("service-only change became meaningful: %+v", s)
	}
	if s.Module.Manifest.CreatedAt != op.baseline.Manifest.CreatedAt {
		t.Fatal("preview used current time rather than stable baseline time")
	}
	if s.Before.Lesson().Document.CreatedAt != "2026-11-01T10:00:00Z" {
		t.Fatal("preview changed original lesson date")
	}
}

func TestSnapshotNoChangesDoesNotImplyExportReadiness(t *testing.T) {
	op := snapshotFixtureOperation(t, Update, false, false)
	op.plan = &compare.Plan{}
	op.placement.Sectioned = true
	op.placement.Sections = []catalog.SectionOrder{{Title: "Пустая секция"}}
	if len(op.refreshPlan()) == 0 {
		t.Fatal("unassigned sectioned catalog must block export")
	}
	s := op.Snapshot()
	if !s.NoChanges || s.ManifestReady || len(s.ManifestChanges) != 0 || s.Actions.ReviewSave || s.Actions.Save {
		t.Fatalf("meaningful changes were conflated with readiness: %+v", s)
	}
}

func TestSnapshotNoChangesWaitsForEveryUpdateDecisionAndRevision(t *testing.T) {
	op := snapshotFixtureOperation(t, Update, false, false)
	op.oldFile, op.revised = op.current.Files[0], op.current.Files[0]
	candidate := op.oldFile.Lesson()
	candidate.Document.Title = "Новое название"
	var issues []*diagnostic.Error
	op.newFile, issues = module.NewFile("l001.json", candidate, diagnostic.Context{})
	snapshotFixtureOK(t, issues)
	op.plan, issues = compare.Build(op.oldFile.Lesson(), candidate, diagnostic.Context{}, diagnostic.Context{})
	snapshotFixtureOK(t, issues)
	op.fields = []revision.Decision{""}
	op.revisionReady = false
	if op.Snapshot().NoChanges {
		t.Fatal("pending field declared no changes")
	}
	op.fields[0] = revision.Reject
	if op.Snapshot().NoChanges {
		t.Fatal("unprepared revision declared no changes")
	}
	op.revisionReady = true
	op.proposals, issues = catalog.ProposePlacement(op.placement, "l001.json", nil, snapshotFixtureText("3"))
	snapshotFixtureOK(t, issues)
	op.placementDecisions = []revision.Decision{""}
	if op.Snapshot().NoChanges {
		t.Fatal("pending placement declared no changes")
	}
	op.placementDecisions[0] = revision.Reject
	if !op.Snapshot().NoChanges {
		t.Fatal("all rejections with unchanged model must report no changes")
	}
	op.lessonChanged = true
	if op.Snapshot().NoChanges {
		t.Fatal("changed lesson declared no changes")
	}
	create := snapshotFixtureOperation(t, Create, false, false)
	create.lessonChanged = false
	if create.Snapshot().NoChanges {
		t.Fatal("creation is never an unchanged existing module")
	}
}

func TestSnapshotRefreshValidatesLinksAfterSuccessfulCatalogExport(t *testing.T) {
	op := snapshotFixtureOperation(t, Create, false, false)
	op.current.Files = append(op.current.Files, nil)
	if len(op.refreshPlan()) == 0 {
		t.Fatal("valid placement hid invalid complete module")
	}
	s := op.Snapshot()
	if s.ManifestReady || s.Module == nil || len(s.Module.Files) != 3 || s.Actions.ReviewSave || s.Actions.Save {
		t.Fatal("failed module validation lost full composition or permitted write")
	}
}

func TestSnapshotCreateManifestUsesSuppliedUTCAnalysisTime(t *testing.T) {
	op := snapshotFixtureOperation(t, Create, false, false)
	snapshotFixtureOK(t, op.refreshPlan())
	s := op.Snapshot()
	if !s.ManifestReady || s.Module.Manifest.CreatedAt != "2026-11-04T09:30:00Z" || len(s.ManifestChanges) != 0 {
		t.Fatalf("create preview time/changes mismatch: %+v", s)
	}
}
