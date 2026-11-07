package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/revision"
	"github.com/verdoga/dsl-builder/safewrite"
)

func snapshotIndependenceBytes(t *testing.T, value Snapshot) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSnapshotEveryMutablePreviewContainerIsIndependent(t *testing.T) {
	for _, unit := range []bool{false, true} {
		t.Run(map[bool]string{false: "course arrays", true: "unit arrays"}[unit], func(t *testing.T) {
			op := snapshotFixtureOperation(t, Add, unit, true)
			op.placement.Sections[0].Weight = snapshotFixtureText("10")
			op.placement.CourseTitle = "Новый курс"
			snapshotFixtureOK(t, op.refreshPlan())
			op.converted = []*module.File{op.current.Files[1]}
			used, unused := filepath.Join(op.baseDir, "used.wav"), filepath.Join(op.baseDir, "unused.wav")
			op.usage.MarkUsed([]string{used})
			op.usage.Register([]string{unused})
			op.proposals, _ = catalog.ProposePlacement(op.placement, "l001.json", snapshotFixtureText("Иная"), snapshotFixtureText("3"))
			op.placementDecisions = []revision.Decision{revision.Accept, revision.Reject}
			inner := &diagnostic.Error{Code: "test.inner", Reason: "Причина",
				Related: []diagnostic.Context{{Path: "related"}}, Details: []diagnostic.Detail{{Label: "поле", Value: "значение"}}}
			op.issues = []*diagnostic.Error{{Code: "test.outer", Reason: "Блокировка",
				Context: diagnostic.Context{Path: "source"}, Related: []diagnostic.Context{{Path: "outer-related"}},
				Details: []diagnostic.Detail{{Label: "причина", Value: "исходная"}},
				Cause:   errors.Join(inner, &os.PathError{Op: "open", Path: "missing", Err: os.ErrNotExist})}}
			op.result = &safewrite.Result{TargetPath: "target", OriginalPath: "backup", TemporaryPath: "temporary"}
			s := op.Snapshot()
			if len(s.NewFileNames) != 1 || s.NewFileNames[0] != "l002.json" {
				t.Fatalf("added file names = %v, want exactly [l002.json]", s.NewFileNames)
			}
			want := snapshotIndependenceBytes(t, s)
			s.Catalog.CourseTitle = "mutated"
			if s.Catalog.UnitTitle != nil {
				*s.Catalog.UnitTitle = "mutated"
			}
			*s.Catalog.Items[0].Section = "mutated"
			*s.Catalog.Items[0].Entry.Order = "999"
			*s.Catalog.Items[0].Entry.Subtitle = "mutated"
			s.Catalog.Items[0].Context.Path = "mutated"
			*s.Catalog.Sections[0].Weight = "999"
			s.Catalog.Sections[0].Title = "mutated"
			if unit {
				s.Module.Manifest.Course.Unit.Title = "mutated"
				s.Module.Manifest.Course.Unit.Sections[0].Title = "mutated"
				*s.Module.Manifest.Course.Unit.Sections[0].Lessons[0].Order = "999"
				*s.Module.Manifest.Course.Unit.Sections[0].Lessons[0].Subtitle = "mutated"
				s.Module.Manifest.Course.Unit.Lessons = append(s.Module.Manifest.Course.Unit.Lessons, s.Catalog.Items[0].Entry)
			} else {
				(*s.Module.Manifest.Course.Sections)[0].Title = "mutated"
				*(*s.Module.Manifest.Course.Sections)[0].Lessons[0].Order = "999"
				*(*s.Module.Manifest.Course.Sections)[0].Lessons[0].Subtitle = "mutated"
				*s.Module.Manifest.Course.Lessons = append(*s.Module.Manifest.Course.Lessons, s.Catalog.Items[0].Entry)
			}
			s.Module.Manifest.Course.Title = "mutated"
			s.Module.Files[0] = nil
			s.NewFileNames[0] = "mutated"
			s.Resources.UsedPaths[0], s.Resources.UnusedPaths[0] = "mutated", "mutated"
			s.ManifestChanges[0].After[0] = '!'
			s.PlacementProposals[0].After[0] = '!'
			s.PlacementAnswered[0] = false
			s.Target.Path = "mutated"
			s.WriteResult.TemporaryPath = "mutated"
			s.Blockers[0].Reason = "mutated"
			s.Blockers[0].Context.Path = "mutated"
			s.Blockers[0].Related[0].Path = "mutated"
			s.Blockers[0].Details[0].Value = "mutated"
			var clonedInner *diagnostic.Error
			if !errors.As(s.Blockers[0].Cause, &clonedInner) || clonedInner == inner {
				t.Fatal("mutable diagnostic cause was not cloned")
			}
			clonedInner.Reason, clonedInner.Related[0].Path, clonedInner.Details[0].Value = "mutated", "mutated", "mutated"
			var pathError *os.PathError
			if !errors.As(s.Blockers[0], &pathError) {
				t.Fatal("file cause lost")
			}
			pathError.Path = "mutated"
			again := op.Snapshot()
			if !bytes.Equal(want, snapshotIndependenceBytes(t, again)) {
				t.Fatal("snapshot mutation changed operation or next snapshot")
			}
			if !errors.Is(again.Blockers[0], os.ErrNotExist) || again.Resources.Total != 2 ||
				again.Resources.Used != 1 || again.Resources.Unused != 1 || again.Module.Files[0] == nil {
				t.Fatal("resource, file, or cause information lost")
			}
		})
	}
}

func TestSnapshotFlatEntriesAndImmutableFilesCannotBeChanged(t *testing.T) {
	op := snapshotFixtureOperation(t, Update, false, false)
	op.oldFile, op.revised = op.current.Files[0], op.current.Files[0]
	s := op.Snapshot()
	if s.Before == nil || s.After == nil {
		t.Fatal("missing stable before/after files")
	}
	want := snapshotIndependenceBytes(t, s)
	*(*s.Module.Manifest.Course.Lessons)[0].Order = "999"
	*(*s.Module.Manifest.Course.Lessons)[0].Subtitle = "mutated"
	(*s.Module.Manifest.Course.Lessons)[0].Title = "mutated"
	for _, file := range []*module.File{s.Before, s.After, s.Module.Files[0]} {
		value := file.Lesson()
		value.Document.Title = "mutated"
		*value.Document.Subtitle = "mutated"
		value.Document.Pages[0].Nodes[0].Data[0].Value = "mutated"
		raw := file.OriginalBytes()
		if len(raw) == 0 {
			t.Fatal("real loaded file lost original bytes")
		}
		raw[0] = '!'
		if file.Lesson().Document.Title != "Урок snap-a" || file.OriginalBytes()[0] == '!' {
			t.Fatal("immutable File leaked its lesson or original bytes")
		}
	}
	if !bytes.Equal(want, snapshotIndependenceBytes(t, op.Snapshot())) {
		t.Fatal("flat manifest entries leaked accepted module")
	}
}

func TestSnapshotFullReplacementKeepsUnmatchedNullFieldAndReasons(t *testing.T) {
	op := snapshotFixtureOperation(t, Update, false, false)
	old, candidate := snapshotFixtureLesson("snap-a"), snapshotFixtureLesson("snap-a")
	candidate.Document.Pages = append(candidate.Document.Pages, lesson.Page{ID: "snap-a-p002", Nodes: []lesson.Node{}})
	var issues []*diagnostic.Error
	op.plan, issues = compare.Build(old, candidate, diagnostic.Context{}, diagnostic.Context{})
	snapshotFixtureOK(t, issues)
	op.fields = make([]revision.Decision, len(op.plan.Fields))
	op.revisionReady = false
	s := op.Snapshot()
	if !s.Plan.ReplaceLesson || len(s.Plan.Reasons) == 0 || len(s.Plan.Pages) != 0 {
		t.Fatalf("incomplete full replacement plan: %+v", s.Plan)
	}
	found := false
	for _, field := range s.Plan.Fields {
		if field.Field == compare.PageTitle && field.OldPage == -1 && field.NewPage == 1 && field.After == nil {
			found = true
		}
	}
	if !found || len(s.FieldAnswered) != len(s.Plan.Fields) {
		t.Fatal("unmatched null page title lost its independent decision")
	}
	want := snapshotIndependenceBytes(t, s)
	s.Plan.Reasons[0].Description = "mutated"
	s.Plan.Reasons[0].OldPointer = "mutated"
	if !bytes.Equal(want, snapshotIndependenceBytes(t, op.Snapshot())) {
		t.Fatal("full replacement reasons alias original plan")
	}
}

func TestSnapshotPlanAndDecisionCopiesKeepCompleteOriginalPlan(t *testing.T) {
	op := snapshotFixtureOperation(t, Update, false, false)
	old, candidate := snapshotFixtureLesson("snap-a"), snapshotFixtureLesson("snap-a")
	for _, value := range []*lesson.Lesson{old, candidate} {
		value.Document.Pages = append(value.Document.Pages, lesson.Page{ID: "snap-a-p002", Nodes: []lesson.Node{}})
	}
	candidate.Document.Title = "Новое название"
	candidate.Document.Subtitle = nil
	candidate.Document.Pages[0].Title = snapshotFixtureText("Новое название страницы")
	candidate.Document.Pages[0].Nodes[0].Data[0].Value = "Новый текст"
	candidate.Document.Pages[1].Nodes = []lesson.Node{{ID: "snap-a-n002", Type: lesson.NodeText, Data: []lesson.Data{}, Nodes: []lesson.Node{}}}
	var issues []*diagnostic.Error
	op.plan, issues = compare.Build(old, candidate, diagnostic.Context{}, diagnostic.Context{})
	snapshotFixtureOK(t, issues)
	if len(op.plan.Fields) != 3 || len(op.plan.Pages) != 2 {
		t.Fatalf("fixture did not produce intended real plan: %+v", op.plan)
	}
	op.fields = []revision.Decision{revision.Accept, revision.Reject, ""}
	op.oldFile, issues = module.NewFile("l001.json", old, diagnostic.Context{})
	snapshotFixtureOK(t, issues)
	op.revised = op.oldFile
	op.revisionReady = false
	s := op.Snapshot()
	if len(s.Plan.Fields) != 3 || len(s.AcceptedFields) != 1 || !s.FieldAnswered[0] ||
		!s.FieldAnswered[1] || s.FieldAnswered[2] || !s.FieldAccepted[0] || s.FieldAccepted[1] || s.RevisionReady {
		t.Fatalf("field decisions/complete plan lost: %+v", s)
	}
	want := snapshotIndependenceBytes(t, s)
	*s.Plan.Fields[0].Before, *s.Plan.Fields[0].After = "mutated", "mutated"
	s.Plan.Fields[1].After = snapshotFixtureText("mutated")
	s.Plan.Pages[0].Nodes[0].Path[0] = 900
	s.Plan.Pages[0].Nodes[0].Fields[0] = compare.NodeTypeChanged
	s.Plan.Pages[1].Reasons[0].Description = "mutated"
	*s.AcceptedFields[0].After = "mutated accepted"
	s.FieldAnswered[0], s.FieldAccepted[0] = false, false
	if !bytes.Equal(want, snapshotIndependenceBytes(t, op.Snapshot())) {
		t.Fatal("plan, field pointers, reasons, or decisions alias internal state")
	}
}

func TestSnapshotConflictsOwnTheirParticipantPointers(t *testing.T) {
	op := snapshotFixtureOperation(t, Add, true, true)
	var issues []*diagnostic.Error
	op.placement, issues = catalog.SetLessonWeight(op.placement, "l002.json", "01")
	if op.placement == nil || len(issues) == 0 {
		t.Fatal("fixture must be an editable conflicting catalog")
	}
	op.issues = op.refreshPlan()
	s := op.Snapshot()
	if len(s.Conflicts) != 1 || len(s.Conflicts[0].Items) != 2 || s.Conflicts[0].Weight != "1" {
		t.Fatalf("incomplete conflict report: %+v", s.Conflicts)
	}
	want := snapshotIndependenceBytes(t, s)
	*s.Conflicts[0].Section = "mutated"
	*s.Conflicts[0].Items[0].Section = "mutated"
	*s.Conflicts[0].Items[0].Entry.Order = "900"
	*s.Conflicts[0].Items[0].Entry.Subtitle = "mutated"
	s.Conflicts[0].Items[0].Context.Path = "mutated"
	if !bytes.Equal(want, snapshotIndependenceBytes(t, op.Snapshot())) {
		t.Fatal("conflict participant leaked accepted catalog")
	}
}

func TestSnapshotManifestGroupChangesPreserveIndependentSourceValues(t *testing.T) {
	op := snapshotFixtureOperation(t, Update, false, true)
	var issues []*diagnostic.Error
	op.placement, issues = catalog.SetLessonWeight(op.placement, "l001.json", "3")
	snapshotFixtureOK(t, issues)
	snapshotFixtureOK(t, op.refreshPlan())
	s := op.Snapshot()
	want := snapshotIndependenceBytes(t, s)
	found := false
	for i := range s.ManifestChanges {
		change := &s.ManifestChanges[i]
		if change.Section != nil {
			found = true
			*change.Section = "mutated"
		}
		if len(change.Before) != 0 {
			change.Before[0] = '!'
		}
		if len(change.After) != 0 {
			change.After[0] = '!'
		}
		change.Field, change.FileName = "mutated", "mutated"
	}
	if !found {
		t.Fatal("fixture did not produce section-addressed group order change")
	}
	if !bytes.Equal(want, snapshotIndependenceBytes(t, op.Snapshot())) {
		t.Fatal("manifest changes leaked raw values or section pointers")
	}
}
