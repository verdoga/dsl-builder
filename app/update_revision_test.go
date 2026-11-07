package app

import (
	"bytes"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/revision"
)

func TestUpdateRevisionUsesOwnersForNodePageAndLessonReplacement(t *testing.T) {
	for _, kind := range []string{"node", "page", "lesson"} {
		t.Run(kind, func(t *testing.T) {
			before := updateFixtureResult("Прежнее", nil, []*string{updateFixtureText("Первая")}, [][]string{{"Раз", "Два"}})
			after := updateFixtureResult("Новое", nil, []*string{updateFixtureText("Первая")}, [][]string{{"Изменён", "Два"}})
			if kind == "page" {
				after = updateFixtureResult("Новое", nil, []*string{updateFixtureText("Первая")}, [][]string{{"Раз", "Два", "Три"}})
			}
			if kind == "lesson" {
				after = updateFixtureResult("Новое", nil, []*string{updateFixtureText("Первая"), nil}, [][]string{{"Раз", "Два"}, {"Три"}})
			}
			after.Document.Metadata.DocumentID = updateFixtureText("eFX7")
			op, loaded, candidate, _ := updateFlowStart(t, before, after, []*string{updateFixtureText("Alpha"), updateFixtureText("Beta")}, []string{"010", "20"})
			plan := op.Snapshot().Plan
			switch kind {
			case "node":
				if plan.ReplaceLesson || len(plan.Pages) != 1 || plan.Pages[0].ReplaceNodes || len(plan.Pages[0].Nodes) != 1 {
					t.Fatalf("node scenario did not exercise one node: %+v", plan)
				}
			case "page":
				if plan.ReplaceLesson || len(plan.Pages) != 1 || !plan.Pages[0].ReplaceNodes {
					t.Fatalf("page scenario did not exercise page replacement: %+v", plan)
				}
			case "lesson":
				if !plan.ReplaceLesson || len(plan.Pages) != 0 {
					t.Fatal("lesson scenario did not exercise full replacement")
				}
			}
			for index := range op.Snapshot().PlacementProposals {
				updateFixtureOK(t, op.DecidePlacement(op.Snapshot().Token, index, false))
			}
			choices := make([]bool, len(plan.Fields))
			decisions := make([]revision.Decision, len(plan.Fields))
			for index := range choices {
				choices[index], decisions[index] = true, revision.Accept
			}
			updateFlowDecide(t, op, choices)
			old := op.oldFile
			untouched := op.current.Files[1]
			oldModel, newModel := old.Lesson(), op.newFile.Lesson()
			want, changed, issues := revision.Apply(oldModel, candidate, plan, decisions, updateFixtureAt(), old.Context(), op.newFile.Context())
			updateFixtureOK(t, issues)
			if !changed || want.Document.Version != "6" {
				t.Fatal("owner oracle did not produce version five to six")
			}
			wantFile, issues := module.ReplaceFile(old, want, op.newFile.Context())
			updateFixtureOK(t, issues)
			wantCatalog, issues := catalog.SyncLesson(catalog.Clone(op.placement), old.FileName(), want.Document)
			updateFixtureOK(t, issues)
			updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
			got := op.Snapshot()
			updateFlowEqual(t, got.After.Lesson(), wantFile.Lesson())
			updateFlowEqual(t, got.Catalog, wantCatalog)
			if got.After.FileName() != loaded.Files[0].FileName() || got.After.Lesson().Document.ID != "Efx7" ||
				got.After.OriginalBytes() != nil || op.current.Files[1] != untouched || op.oldFile != old ||
				op.baseline.Files[0] != old || op.current == op.baseline {
				t.Fatal("replacement changed name/case, retained obsolete bytes or replaced an unrelated file")
			}
			updateFlowEqual(t, op.oldFile.Lesson(), oldModel)
			updateFlowEqual(t, op.newFile.Lesson(), newModel)
			updateFlowEqual(t, op.inputs[0].Result, after)
			updateFlowEqual(t, got.Plan, plan)
			updateFixtureOK(t, op.DecideField(got.Token, 0, false))
			if op.Snapshot().RevisionReady {
				t.Fatal("changed decision retained prepared revision")
			}
			updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
			rejected := op.Snapshot().After.Lesson()
			if rejected.Document.Version != "6" || rejected.Document.Title != oldModel.Document.Title {
				t.Fatal("second preparation used previous revised model")
			}
			file, changed, issues := op.applyRevision()
			updateFixtureOK(t, issues)
			if !changed {
				t.Fatal("content replacement disappeared on repeat")
			}
			updateFlowEqual(t, file.Lesson(), rejected)
			updateFlowEqual(t, op.plan, plan)
		})
	}
}

func TestUpdateNoChangesKeepsEntireOldFileAndNeverWrites(t *testing.T) {
	for _, kind := range []string{"unchanged", "time-only", "line-count-only", "metadata-rejected"} {
		t.Run(kind, func(t *testing.T) {
			before := updateFixtureResult("Прежнее", updateFixtureText("Подзаголовок"), []*string{nil}, [][]string{{"Материал"}})
			after := updateFixtureResult("Прежнее", updateFixtureText("Подзаголовок"), []*string{nil}, [][]string{{"Материал"}})
			if kind == "line-count-only" {
				blank := after.Lines[len(after.Lines)-1]
				blank.Number++
				blank.LineType, blank.Raw = "blank", ""
				blank.Elements = blank.Elements[:0]
				after.Lines = append(after.Lines, blank)
				count := len(after.Lines)
				after.Document.LineCount = &count
			}
			if kind == "metadata-rejected" {
				after = updateFixtureResult("Предложение", nil, []*string{updateFixtureText("Новая")}, [][]string{{"Материал"}})
			}
			op, _, _, archive := updateFlowStart(t, before, after, []*string{nil, nil}, []string{"010", "20"})
			raw, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			old := op.oldFile
			for index := range op.Snapshot().Plan.Fields {
				updateFixtureOK(t, op.DecideField(op.Snapshot().Token, index, false))
			}
			updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
			snapshot := op.Snapshot()
			if !snapshot.NoChanges || !snapshot.RevisionReady || snapshot.Actions.ReviewSave ||
				op.revised != old || op.current.Files[0] != old || op.lessonChanged {
				t.Fatal("unchanged owner result became a replacement or saveable change")
			}
			if !bytes.Equal(snapshot.After.OriginalBytes(), old.OriginalBytes()) {
				t.Fatal("unchanged original lesson bytes were discarded")
			}
			if snapshot.After.Lesson().Document.Version != "5" {
				t.Fatal("no-op incremented revision")
			}
			if snapshot.Module.Manifest.CreatedAt != op.baseline.Manifest.CreatedAt {
				t.Fatal("no-op introduced manifest timestamp change during preview")
			}
			preview, issues := op.ReviewSave(snapshot.Token)
			if preview != nil || len(issues) == 0 {
				t.Fatal("no-op allowed review")
			}
			result, issues := op.Save(updateFixtureAt().AddDate(1, 0, 0))
			if result.Installed || len(issues) == 0 {
				t.Fatal("no-op allowed write")
			}
			actual, err := os.ReadFile(archive)
			if err != nil || !bytes.Equal(actual, raw) {
				t.Fatal("no-op changed archive")
			}
		})
	}
}

func TestUpdateChangedDecisionRevokesActualSaveConsent(t *testing.T) {
	before := updateFixtureResult("До", nil, []*string{nil}, [][]string{{"Текст"}})
	after := updateFixtureResult("После", nil, []*string{nil}, [][]string{{"Новый текст"}})
	op, _, _, archive := updateFlowStart(t, before, after, []*string{nil, nil}, []string{"10", "20"})
	updateFlowDecide(t, op, []bool{true})
	updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
	preview, issues := op.ReviewSave(op.Snapshot().Token)
	updateFixtureOK(t, issues)
	updateFixtureOK(t, op.ConfirmSave(preview.Token, true, true))
	if !op.Snapshot().Actions.Save {
		t.Fatal("fixture did not obtain actual save consent")
	}
	updateFixtureOK(t, op.DecideField(op.Snapshot().Token, 0, false))
	if op.Snapshot().Actions.Save || op.Snapshot().RevisionReady {
		t.Fatal("changed answer retained consent/readiness")
	}
	result, issues := op.Save(updateFixtureAt())
	if result.Installed || len(issues) == 0 {
		t.Fatal("stale consent allowed write")
	}
	updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
	preview, issues = op.ReviewSave(op.Snapshot().Token)
	updateFixtureOK(t, issues)
	updateFixtureOK(t, op.ConfirmSave(preview.Token, true, true))
	result, issues = op.Save(updateFixtureAt())
	updateFixtureOK(t, issues)
	if !result.Installed {
		t.Fatal("current final revision was not saved")
	}
	loaded, issues := moduleio.Read(archive)
	updateFixtureOK(t, issues)
	if loaded.Files[0].Lesson().Document.Version != "6" || loaded.Files[0].Lesson().Document.Title != "До" {
		t.Fatal("real save wrote candidate title or incremented twice")
	}
}
