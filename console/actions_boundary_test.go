package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestActionsLoopPassesOriginalShownTokenToOwner(t *testing.T) {
	op, _, source := actionsTestCreate(t)
	before := actionsTestRead(t, source)
	initial := op.Snapshot()
	got, _ := actionsTestLoop(t, op,
		actionsTestPick(t, actionCourse, func(text string) {
			actionsTestShown(t, text, initial)
			// Владелец меняется после показа, до ответа на старое меню.
			actionsTestCourse(t, op, "Внешняя правка")
		}),
		actionsTestAnswer("1"), actionsTestAnswer("Устаревший ответ"),
		actionsTestPick(t, actionExit, func(text string) {
			s := op.Snapshot()
			if s.Catalog.CourseTitle != "Внешняя правка" {
				t.Fatal("stale answer bypassed owner token check")
			}
			actionsTestRequireText(t, text, app.CodeStaleReview)
			actionsTestShown(t, text, s)
		}),
	)
	if got != routeExit {
		t.Fatalf("route=%v", got)
	}
	actionsTestNoResult(t, op)
	actionsTestUnchanged(t, source, before)
}

func TestActionsLoopShowsFreshSnapshotAfterRevisionReturnsBlockers(t *testing.T) {
	op, archive, source := actionsTestUpdate(t, nil)
	before, sourceBefore := actionsTestRead(t, archive), actionsTestRead(t, source)
	edit, issues := op.ProposeSection(op.Snapshot().Token, "Пустая секция")
	if edit == nil {
		t.Fatalf("no real section candidate: %v", issues)
	}
	// Принятие рабочей секции допустимо, запись ждёт распределения.
	if len(op.AcceptEdit(edit.Token)) == 0 {
		t.Fatal("fixture should have placement blockers")
	}
	initial := op.Snapshot()
	if !initial.Actions.PrepareRevision || len(initial.Blockers) == 0 {
		t.Fatal("missing blocked revision precondition")
	}
	got, _ := actionsTestLoop(t, op,
		actionsTestPick(t, actionRevision, func(text string) { actionsTestShown(t, text, initial) }),
		actionsTestPick(t, actionExit, func(text string) {
			s := op.Snapshot()
			if !s.RevisionReady || len(s.Blockers) == 0 || s.NoChanges && len(s.Blockers) == 0 {
				t.Fatal(actionsTestSnapshotDescription(s))
			}
			actionsTestShown(t, text, s)
			for _, issue := range s.Blockers {
				actionsTestRequireText(t, text, issue.Code)
			}
			if strings.Contains(text, "Содержательных изменений не обнаружено") {
				t.Fatal("blocked plan treated as terminal no-change")
			}
			if strings.Contains(text, actionLabel(actionSave)) {
				t.Fatal("write offered with owner blockers")
			}
		}),
	)
	if got != routeExit {
		t.Fatalf("route=%v", got)
	}
	actionsTestNoResult(t, op)
	actionsTestUnchanged(t, archive, before)
	actionsTestUnchanged(t, source, sourceBefore)
}

func TestActionsLoopFailedPreparationDoesNotAutomaticallyRepeat(t *testing.T) {
	op, archive, _ := actionsTestUpdate(t, nil)
	before := actionsTestRead(t, archive)
	shown := op.Snapshot()
	got, _ := actionsTestLoop(t, op,
		actionsTestPick(t, actionRevision, func(text string) {
			actionsTestShown(t, text, shown)
			// Публичная отмена делает выбранный показанный token недействительным.
			op.Cancel()
		}),
		actionsTestPick(t, actionExit, func(text string) {
			s := op.Snapshot()
			if s.RevisionReady || s.Actions.PrepareRevision {
				t.Fatal("failed revision became ready/retryable")
			}
			actionsTestRequireText(t, text, app.CodeState)
			actionsTestShown(t, text, s)
			if strings.Contains(text, actionLabel(actionRevision)) {
				t.Fatal("reused previous action list")
			}
		}),
	)
	if got != routeExit {
		t.Fatalf("route=%v", got)
	}
	actionsTestNoResult(t, op)
	actionsTestUnchanged(t, archive, before)
}

type actionsTestFaultWriter struct {
	cause  error
	writes int
	short  bool
}

func (w *actionsTestFaultWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.short {
		return len(p) - 1, nil
	}
	return 0, w.cause
}

type actionsTestReadFault struct {
	cause error
	reads int
}

func (r *actionsTestReadFault) Read([]byte) (int, error) { r.reads++; return 0, r.cause }

func TestActionsLoopStreamFailuresStopFurtherIOAndDecision(t *testing.T) {
	for _, kind := range []string{"read", "write", "short write"} {
		t.Run(kind, func(t *testing.T) {
			op, _, source := actionsTestCreate(t)
			raw := actionsTestRead(t, source)
			cause := errors.New("actions stream failure")
			reader := &actionsTestReadFault{cause: cause}
			var output bytes.Buffer
			var dst io.Writer = &output
			writer := &actionsTestFaultWriter{cause: cause, short: kind == "short write"}
			if kind != "read" {
				dst = writer
			}
			d := dialog{input: bufio.NewReader(reader), output: dst, operation: op}
			_, err := d.operationLoop()
			want := cause
			if kind == "short write" {
				want = io.ErrShortWrite
			}
			if !errors.Is(err, want) || !isStreamFailure(err) {
				t.Fatalf("wrong stream error: %v", err)
			}
			code := CodeOutput
			if kind == "read" {
				code = CodeInput
				if reader.reads != 1 {
					t.Fatalf("read repeated %d times", reader.reads)
				}
			} else {
				if reader.reads != 0 || writer.writes != 1 {
					t.Fatalf("continued IO reads=%d writes=%d", reader.reads, writer.writes)
				}
			}
			var issue interface{ Error() string }
			issue = err
			if !strings.Contains(issue.Error(), code) {
				t.Errorf("missing stream code %q: %v", code, err)
			}
			actionsTestNoResult(t, op)
			actionsTestUnchanged(t, source, raw)
		})
	}
}

func TestActionsLoopInstalledResultIsTerminal(t *testing.T) {
	op, base, source := actionsTestCreate(t)
	archive := actionsTestSave(t, op, base)
	before, sourceBefore := actionsTestRead(t, archive), actionsTestRead(t, source)
	got, text := actionsTestLoop(t, op, func(text string) string {
		actionsTestRequireText(t, text, archive, "Модуль успешно сохранён")
		for _, a := range []action{actionRetry, actionRevision, actionSave, actionDestination, actionCourse} {
			if strings.Contains(text, actionLabel(a)) {
				t.Errorf("nonterminal action %q after Installed", a)
			}
		}
		return actionsTestNumber(t, text, "Выход")
	})
	if got != routeExit || !op.Snapshot().WriteResult.Installed {
		t.Fatalf("wrong installed result route: %v\n%s", got, text)
	}
	actionsTestUnchanged(t, archive, before)
	actionsTestUnchanged(t, source, sourceBefore)
}

func TestActionsDispatchNavigationCancelsWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		a    action
		want route
	}{{actionRestart, routeRestart}, {actionMain, routeMain}, {actionExit, routeExit}} {
		t.Run(string(tc.a), func(t *testing.T) {
			op, base, source := actionsTestCreate(t)
			raw := actionsTestRead(t, source)
			s := op.Snapshot()
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader("")), output: &output, operation: op}
			got, err := d.dispatchAction(s, tc.a)
			if err != nil || got != tc.want {
				t.Fatalf("dispatch=(%v,%v)", got, err)
			}
			if _, issues := op.ProposeCourseTitle(s.Token, "После ухода"); len(issues) == 0 {
				t.Fatal("navigation did not cancel")
			}
			paths, err := filepath.Glob(filepath.Join(base, "*.zip"))
			if err != nil || len(paths) != 0 {
				t.Fatalf("navigation wrote archive: %v, %v", paths, err)
			}
			actionsTestNoResult(t, op)
			actionsTestUnchanged(t, source, raw)
			if _, err := os.Stat(source); err != nil {
				t.Fatal(err)
			}
		})
	}
}
