package console

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/preview"
)

func TestActionsDispatchReviewSaveRequiresSeparateConsent(t *testing.T) {
	op, base, source := actionsTestCreate(t)
	raw := actionsTestRead(t, source)
	actionsTestIssues(t, op.SetDestination(op.Snapshot().Token, base, "Отказ от записи"))
	shown := op.Snapshot()
	if !shown.Actions.ReviewSave || shown.Actions.Save {
		t.Fatal("fixture must permit review without consent")
	}
	var output bytes.Buffer
	input := &actionsTestInput{t: t, output: &output, steps: []func(string) string{func(text string) string {
		actionsTestShown(t, text, op.Snapshot())
		actionsTestRequireText(t, text, filepath.Join(base, "Отказ от записи.zip"))
		return actionsTestNumber(t, text, "Нет")
	}}}
	d := dialog{input: bufio.NewReader(input), output: &output, operation: op}
	got, err := d.dispatchAction(shown, actionSave)
	if err != nil || got != routePreview || input.reads != 1 {
		t.Fatalf("save review=(%v,%v), reads=%d\n%s", got, err, input.reads, output.String())
	}
	actionsTestNoResult(t, op)
	if _, err := os.Stat(shown.Target.Path); !os.IsNotExist(err) {
		t.Fatalf("refused save created ZIP: %v", err)
	}
	actionsTestUnchanged(t, source, raw)
	// Отказ не завершает операцию: следующее меню полностью показано заново.
	got, _ = actionsTestLoop(t, op, actionsTestPick(t, actionExit, func(text string) {
		actionsTestShown(t, text, op.Snapshot())
		actionsTestRequireMenu(t, text, actionSave)
	}))
	if got != routeExit {
		t.Fatalf("route=%v", got)
	}
}

func TestActionsDispatchUnusedReturnsToPreviewWithoutMutation(t *testing.T) {
	op, _, source := actionsTestCreate(t)
	raw := actionsTestRead(t, source)
	shown := op.Snapshot()
	var output bytes.Buffer
	d := dialog{input: bufio.NewReader(strings.NewReader("1\n")), output: &output, operation: op}
	got, err := d.dispatchAction(shown, actionUnused)
	if err != nil || got != routePreview {
		t.Fatalf("unused=(%v,%v)", got, err)
	}
	actionsTestRequireText(t, output.String(), preview.UnusedResources(shown.Resources))
	if op.Snapshot().Token != shown.Token {
		t.Fatal("resource view changed owner state")
	}
	actionsTestNoResult(t, op)
	actionsTestUnchanged(t, source, raw)
}

func TestActionsSafePresentationNavigationUsesRealOwnerPermissions(t *testing.T) {
	op, base, source := actionsTestCreate(t)
	raw := actionsTestRead(t, source)
	actionsTestIssues(t, op.SetDestination(op.Snapshot().Token, base, "Безопасное меню"))
	shown := op.Snapshot()
	if !shown.Actions.ReviewSave {
		t.Fatal("real owner must permit save review")
	}
	var output bytes.Buffer
	input := &actionsTestInput{t: t, output: &output, steps: []func(string) string{func(text string) string {
		// У реального готового Create есть ReviewSave, но safe меню его скрывает.
		for _, a := range []action{actionCourse, actionUnit, actionDestination, actionSave, actionRevision, actionRetry} {
			if strings.Contains(text, actionLabel(a)) {
				t.Errorf("unsafe/unavailable action %q in presentation-failure menu", a)
			}
		}
		actionsTestRequireMenu(t, text, actionRestart, actionMain, actionExit)
		return actionsTestNumber(t, text, actionLabel(actionExit))
	}}}
	d := dialog{input: bufio.NewReader(input), output: &output, operation: op}
	cause := presentationError(os.ErrInvalid)
	if err := PrintError(&output, cause); err != nil {
		t.Fatal(err)
	}
	input.cursor = output.Len()
	got, err := d.presentationFailureChoice(shown)
	if err != nil || got != routeExit {
		t.Fatalf("presentation navigation=(%v,%v)", got, err)
	}
	actionsTestRequireText(t, output.String(), CodePreview)
	actionsTestNoResult(t, op)
	actionsTestUnchanged(t, source, raw)
}
