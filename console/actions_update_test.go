package console

import (
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestActionsLoopAcceptedAndRejectedFieldsEachRequireNewChoice(t *testing.T) {
	op, archive, source := actionsTestUpdate(t, func(r *model.Result) {
		title, subtitle := "OTHER", "New text"
		r.Document.Metadata.Title = &title
		r.Document.Metadata.Subtitle = &subtitle
	})
	before, sourceBefore := actionsTestRead(t, archive), actionsTestRead(t, source)
	if len(op.Snapshot().Plan.Fields) != 2 {
		t.Fatal("fixture must have two separate fields")
	}
	got, _ := actionsTestLoop(t, op,
		actionsTestPick(t, actionField, func(text string) { actionsTestShown(t, text, op.Snapshot()) }),
		actionsTestAnswer("1"),
		actionsTestPick(t, actionField, func(text string) {
			s := op.Snapshot()
			if !s.FieldAnswered[0] || !s.FieldAccepted[0] || s.FieldAnswered[1] || s.RevisionReady {
				t.Fatal("answer affected more than first field")
			}
			actionsTestShown(t, text, s)
		}),
		actionsTestAnswer("2"),
		actionsTestPick(t, actionRevision, func(text string) {
			s := op.Snapshot()
			if !s.FieldAnswered[1] || s.FieldAccepted[1] || s.RevisionReady {
				t.Fatal("second field not separately rejected")
			}
			actionsTestShown(t, text, s)
			if strings.Contains(text, actionLabel(actionField)) {
				t.Fatal("all fields answered but old menu remains")
			}
		}),
		actionsTestPick(t, actionExit, func(text string) {
			s := op.Snapshot()
			lesson := s.After.Lesson()
			if !s.RevisionReady || lesson.Document.Title != "OTHER" || lesson.Document.Subtitle == nil || *lesson.Document.Subtitle != "Subtitle" || lesson.Document.Version != "2" {
				t.Fatal("accepted/rejected field decisions not preserved")
			}
			actionsTestShown(t, text, s)
		}),
	)
	if got != routeExit {
		t.Fatalf("route=%v", got)
	}
	actionsTestNoResult(t, op)
	actionsTestUnchanged(t, archive, before)
	actionsTestUnchanged(t, source, sourceBefore)
}

func TestActionsLoopPlacementAnswerRefreshesManualActions(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject", true: "accept"}[accept], func(t *testing.T) {
			op, archive, source := actionsTestUpdate(t, func(r *model.Result) { weight := "2"; r.Document.Metadata.Order = &weight })
			before, sourceBefore := actionsTestRead(t, archive), actionsTestRead(t, source)
			if s := op.Snapshot(); len(s.PlacementProposals) != 1 || !s.Actions.DecidePlacement || s.Actions.EditCatalog {
				t.Fatal("fixture must await order proposal")
			}
			answer := "2"
			if accept {
				answer = "1"
			}
			got, _ := actionsTestLoop(t, op,
				actionsTestPick(t, actionPlacement, func(text string) {
					actionsTestShown(t, text, op.Snapshot())
					if strings.Contains(text, actionLabel(actionCourse)) {
						t.Fatal("manual catalog edit before placement decision")
					}
				}),
				actionsTestAnswer(answer),
				actionsTestPick(t, actionExit, func(text string) {
					s := op.Snapshot()
					want := "1"
					if accept {
						want = "2"
					}
					if !s.PlacementAnswered[0] || !s.Actions.EditCatalog || s.Actions.DecidePlacement || s.RevisionReady || s.Catalog.Items[0].Entry.Order == nil || *s.Catalog.Items[0].Entry.Order != want {
						t.Fatal("placement answer not independently published")
					}
					actionsTestShown(t, text, s)
					actionsTestRequireMenu(t, text, actionCourse, actionRevision)
				}),
			)
			if got != routeExit {
				t.Fatalf("route=%v", got)
			}
			actionsTestNoResult(t, op)
			actionsTestUnchanged(t, archive, before)
			actionsTestUnchanged(t, source, sourceBefore)
		})
	}
}
