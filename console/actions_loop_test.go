package console

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestActionsLoopShowsInitialSnapshotAndNeverPreparesAutomatically(t *testing.T) {
	op, archive, source := actionsTestUpdate(t, nil)
	archiveBefore, sourceBefore := actionsTestRead(t, archive), actionsTestRead(t, source)
	initial := op.Snapshot()
	got, _ := actionsTestLoop(t, op, actionsTestPick(t, actionMain, func(text string) {
		actionsTestShown(t, text, initial)
		actionsTestRequireMenu(t, text, actionRevision)
		if op.Snapshot().RevisionReady {
			t.Fatal("revision prepared before explicit choice")
		}
	}))
	if got != routeMain || op.Snapshot().RevisionReady {
		t.Fatal("navigation prepared revision or wrong route")
	}
	if _, issues := op.ProposeCourseTitle(initial.Token, "После отмены"); len(issues) == 0 {
		t.Fatal("navigation did not Cancel")
	}
	actionsTestNoResult(t, op)
	actionsTestUnchanged(t, archive, archiveBefore)
	actionsTestUnchanged(t, source, sourceBefore)
}

func TestActionsLoopExplicitRevisionBeforeNoChanges(t *testing.T) {
	for _, rejectFields := range []bool{false, true} {
		t.Run(map[bool]string{false: "no fields", true: "reject all fields"}[rejectFields], func(t *testing.T) {
			var edit func(*model.Result)
			if rejectFields {
				edit = func(r *model.Result) {
					title, subtitle := "OTHER", "New text"
					r.Document.Metadata.Title = &title
					r.Document.Metadata.Subtitle = &subtitle
				}
			}
			op, archive, source := actionsTestUpdate(t, edit)
			archiveBefore, sourceBefore := actionsTestRead(t, archive), actionsTestRead(t, source)
			steps := []func(string) string{}
			for range op.Snapshot().Plan.Fields {
				steps = append(steps, actionsTestPick(t, actionField, func(text string) {
					s := op.Snapshot()
					actionsTestShown(t, text, s)
					if s.RevisionReady {
						t.Fatal("automatic preparation between fields")
					}
				}), actionsTestAnswer("2"))
			}
			steps = append(steps, actionsTestPick(t, actionRevision, func(text string) {
				s := op.Snapshot()
				actionsTestShown(t, text, s)
				if s.RevisionReady || !s.Actions.PrepareRevision {
					t.Fatal(actionsTestSnapshotDescription(s))
				}
				if strings.Contains(text, "Содержательных изменений не обнаружено") {
					t.Fatal("premature no-change terminal")
				}
			}), func(text string) string {
				s := op.Snapshot()
				if !s.RevisionReady || !s.NoChanges || len(s.Blockers) != 0 {
					t.Fatal(actionsTestSnapshotDescription(s))
				}
				actionsTestShown(t, text, s)
				actionsTestRequireText(t, text, "Содержательных изменений не обнаружено")
				if strings.Contains(text, "Записать показанный модуль?") {
					t.Fatal("asked save for no-change")
				}
				return actionsTestNumber(t, text, "Выход")
			})
			got, _ := actionsTestLoop(t, op, steps...)
			if got != routeExit {
				t.Fatalf("route=%v", got)
			}
			s := op.Snapshot()
			if s.After.Lesson().Document.Version != "1" || s.After.Lesson().Document.Title != "TITLE" {
				t.Fatal("rejected/unchanged lesson revised")
			}
			actionsTestNoResult(t, op)
			actionsTestUnchanged(t, archive, archiveBefore)
			actionsTestUnchanged(t, source, sourceBefore)
		})
	}
}

func TestActionsLoopContentAndManifestOnlyPrepareThenShowFreshState(t *testing.T) {
	for _, content := range []bool{true, false} {
		t.Run(map[bool]string{true: "node content", false: "manifest only"}[content], func(t *testing.T) {
			var edit func(*model.Result)
			if content {
				edit = func(r *model.Result) {
					line := &r.Lines[10]
					line.Raw = strings.ReplaceAll(line.Raw, "EXAMPLE TEXT", "CHANGED TEXT")
					value := "CHANGED TEXT"
					line.Elements[1].Raw = value
					line.Elements[1].Value = &value
				}
			}
			op, archive, source := actionsTestUpdate(t, edit)
			if !content {
				actionsTestCourse(t, op, "Изменённый курс")
			}
			if len(op.Snapshot().Plan.Fields) != 0 {
				t.Fatal("fixture unexpectedly needs fields")
			}
			archiveBefore, sourceBefore := actionsTestRead(t, archive), actionsTestRead(t, source)
			got, _ := actionsTestLoop(t, op,
				actionsTestPick(t, actionRevision, func(text string) { actionsTestShown(t, text, op.Snapshot()) }),
				actionsTestPick(t, actionExit, func(text string) {
					s := op.Snapshot()
					if !s.RevisionReady || s.NoChanges || !s.Actions.ReviewSave {
						t.Fatal(actionsTestSnapshotDescription(s))
					}
					actionsTestShown(t, text, s)
					wantVersion := "1"
					if content {
						wantVersion = "2"
						actionsTestRequireText(t, text, "ba82-n003-v02")
					}
					if s.After.Lesson().Document.Version.String() != wantVersion {
						t.Fatal("unexpected final version")
					}
					if strings.Contains(text, actionLabel(actionRevision)) {
						t.Fatal("revision item remains after successful preparation")
					}
				}),
			)
			if got != routeExit {
				t.Fatalf("route=%v", got)
			}
			actionsTestNoResult(t, op)
			actionsTestUnchanged(t, archive, archiveBefore)
			actionsTestUnchanged(t, source, sourceBefore)
		})
	}
}

func TestActionsLoopRefreshesAfterAcceptRejectAndDestination(t *testing.T) {
	op, base, source := actionsTestCreate(t)
	before := actionsTestRead(t, source)
	initial := op.Snapshot()
	got, _ := actionsTestLoop(t, op,
		actionsTestPick(t, actionCourse, func(text string) { actionsTestShown(t, text, initial) }),
		actionsTestAnswer("1"), actionsTestAnswer("Новый курс"), actionsTestAnswer("1"),
		actionsTestPick(t, actionUnit, func(text string) {
			s := op.Snapshot()
			if s.Catalog.CourseTitle != "Новый курс" || s.Token == initial.Token {
				t.Fatal("accepted edit not published")
			}
			actionsTestShown(t, text, s)
		}),
		actionsTestAnswer("1"), actionsTestAnswer("Отклонённый юнит"), actionsTestAnswer("2"),
		actionsTestPick(t, actionDestination, func(text string) {
			s := op.Snapshot()
			if s.Catalog.UnitTitle != nil {
				t.Fatal("rejected unit accepted")
			}
			actionsTestShown(t, text, s)
		}),
		actionsTestAnswer("1"), actionsTestAnswer(base), actionsTestAnswer("1"), actionsTestAnswer("Новая цель"),
		actionsTestPick(t, actionExit, func(text string) {
			s := op.Snapshot()
			if s.Target == nil || s.Target.Path != filepath.Join(base, "Новая цель.zip") || !s.Actions.ReviewSave {
				t.Fatal("destination not refreshed")
			}
			actionsTestShown(t, text, s)
			actionsTestRequireMenu(t, text, actionSave)
		}),
	)
	if got != routeExit {
		t.Fatalf("route=%v", got)
	}
	actionsTestNoResult(t, op)
	actionsTestUnchanged(t, source, before)
	if paths, err := filepath.Glob(filepath.Join(base, "*.zip")); err != nil || len(paths) != 0 {
		t.Fatalf("unexpected ZIP: %v, %v", paths, err)
	}
}

func TestActionsLoopRetryShowsNewOwnerState(t *testing.T) {
	_, base, source := actionsTestCreate(t)
	// Ошибка повторного анализа создаётся реальным чтением повреждённого JSON.
	raw := actionsTestRead(t, source)
	actionsTestWrite(t, source, []byte("{"))
	failed, issues := app.NewCreate(base, filepath.Dir(source), "После Retry", "")
	actionsTestIssues(t, issues)
	if len(failed.Analyze(actionsTestAt())) == 0 || !failed.Snapshot().Actions.Retry {
		t.Fatal("missing owner read failure")
	}
	got, _ := actionsTestLoop(t, failed,
		actionsTestPick(t, actionRetry, func(text string) {
			s := failed.Snapshot()
			for _, issue := range s.Blockers {
				actionsTestRequireText(t, text, issue.Code)
			}
			actionsTestWrite(t, source, raw)
		}),
		actionsTestPick(t, actionExit, func(text string) {
			s := failed.Snapshot()
			if s.Catalog == nil || len(s.Blockers) != 0 || s.Actions.Retry {
				t.Fatal("Retry did not replace failed state")
			}
			actionsTestShown(t, text, s)
		}),
	)
	if got != routeExit {
		t.Fatalf("route=%v", got)
	}
	actionsTestUnchanged(t, source, raw)
	actionsTestNoResult(t, failed)
}
