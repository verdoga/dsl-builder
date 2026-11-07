package console

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestResultTerminalOffersOnlyAllowedNavigationAndCancels(t *testing.T) {
	for _, test := range []struct {
		name       string
		main, exit bool
		input      string
		want       route
		labels     []string
	}{
		{"both_main", true, true, "1\n", routeMain, []string{"main", "exit"}},
		{"both_exit", true, true, "2\n", routeExit, []string{"main", "exit"}},
		{"main_only", true, false, "1\n", routeMain, []string{"main"}},
		{"exit_only", false, true, "1\n", routeExit, []string{"exit"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			op, source, _ := resultTestCreate(t)
			before := resultTestBytes(t, source)
			snapshot := op.Snapshot()
			snapshot.Actions.MainMenu, snapshot.Actions.Exit = test.main, test.exit
			var output bytes.Buffer
			d := resultTestDialog(t, strings.NewReader(test.input), &output, op)
			got, err := d.terminalChoice(snapshot)
			if err != nil || got != test.want {
				t.Fatalf("route=%v, want=%v; error=%v", got, test.want, err)
			}
			resultTestMenu(t, output.String(), test.labels...)
			resultTestCanceled(t, op, snapshot.Token)
			resultTestUnchanged(t, source, before)
		})
	}
}

func TestResultTerminalInvalidInputRepeatsTheSameMenu(t *testing.T) {
	op, _, _ := resultTestCreate(t)
	snapshot := op.Snapshot()
	var output bytes.Buffer
	d := resultTestDialog(t, strings.NewReader("\n0\n3\nда\n2\n"), &output, op)
	got, err := d.terminalChoice(snapshot)
	if err != nil || got != routeExit {
		t.Fatalf("route=%v error=%v", got, err)
	}
	if strings.Count(output.String(), "1. Вернуться в главное меню") != 5 || strings.Count(output.String(), "2. Выход") != 5 {
		t.Errorf("invalid input did not repeat same terminal menu: %s", output.String())
	}
	resultTestCanceled(t, op, snapshot.Token)
}

func TestResultTerminalLeavesInstalledArchiveAndRecoveryPathsUntouched(t *testing.T) {
	for _, input := range []string{"1\n", "2\n"} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			op, source, archive := resultTestSaved(t)
			sourceBefore, archiveBefore := resultTestBytes(t, source), resultTestBytes(t, archive)
			original, temporary := filepath.Join(filepath.Dir(archive), "резерва.zip"), filepath.Join(filepath.Dir(archive), "кандидат.zip")
			for _, path := range []string{original, temporary} {
				if err := os.WriteFile(path, []byte("Не трогать этот файл"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := op.Snapshot()
			// Проекция известных путей не внедряется во внутреннее состояние app.
			snapshot.WriteResult.OriginalPath, snapshot.WriteResult.TemporaryPath = original, temporary
			var output bytes.Buffer
			d := resultTestDialog(t, strings.NewReader(input), &output, op)
			if _, err := d.terminalChoice(snapshot); err != nil {
				t.Fatal(err)
			}
			resultTestCanceled(t, op, snapshot.Token)
			resultTestUnchanged(t, archive, archiveBefore)
			resultTestUnchanged(t, source, sourceBefore)
			for _, path := range []string{original, temporary} {
				resultTestUnchanged(t, path, []byte("Не трогать этот файл"))
			}
			if result := op.Snapshot().WriteResult; result == nil || !result.Installed || result.TargetPath != archive {
				t.Error("terminal navigation rolled back installed result")
			}
		})
	}
}

func TestResultPresentationFailureOffersOnlySafeActions(t *testing.T) {
	for _, test := range []struct {
		selected string
		want     route
	}{
		{"restart", routeRestart}, {"main", routeMain}, {"exit", routeExit},
	} {
		t.Run(test.selected, func(t *testing.T) {
			op, source, directory := resultTestCreate(t)
			before := resultTestBytes(t, source)
			path := resultTestConsent(t, op, directory)
			snapshot := op.Snapshot()
			if !snapshot.Actions.Save || !snapshot.Actions.ReviewSave {
				t.Fatal("fixture lacks positive consent to challenge safe menu")
			}
			var output bytes.Buffer
			d := resultTestDialog(t, resultTestMenuInput(t, &output, test.selected), &output, op)
			got, err := d.presentationFailureChoice(snapshot)
			if err != nil || got != test.want {
				t.Fatalf("route=%v, want=%v; error=%v", got, test.want, err)
			}
			resultTestMenu(t, output.String(), "restart", "main", "exit")
			resultTestCanceled(t, op, snapshot.Token)
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || op.Snapshot().WriteResult != nil {
				t.Fatalf("presentation failure wrote target: %v", err)
			}
			resultTestUnchanged(t, source, before)
		})
	}
}

func TestResultPresentationFailureHonorsNavigationActions(t *testing.T) {
	for _, test := range []struct {
		selected string
		want     route
	}{
		{"exit", routeExit},
		{"main", routeMain},
	} {
		t.Run(test.selected, func(t *testing.T) {
			op, source, directory := resultTestCreate(t)
			before := resultTestBytes(t, source)
			resultTestIssues(t, op.SetDestination(op.Snapshot().Token, directory, "Результат.zip"))
			// Разрешения берутся у владельца; независимая копия не задаёт их заново.
			snapshot := op.Snapshot()
			if !snapshot.Actions.MainMenu || !snapshot.Actions.Exit || snapshot.Actions.Retry || !snapshot.Actions.ReviewSave || snapshot.Actions.Save || snapshot.Target == nil || snapshot.Target.Exists {
				t.Fatal("fixture lacks current navigation and a target without save consent")
			}
			var output bytes.Buffer
			d := resultTestDialog(t, resultTestMenuInput(t, &output, test.selected), &output, op)
			got, err := d.presentationFailureChoice(snapshot)
			if err != nil || got != test.want {
				t.Fatalf("route=%v want=%v err=%v", got, test.want, err)
			}
			resultTestMenu(t, output.String(), "restart", "main", "exit")
			resultTestCanceled(t, op, snapshot.Token)
			if _, err := os.Stat(snapshot.Target.Path); !errors.Is(err, os.ErrNotExist) || op.Snapshot().WriteResult != nil {
				t.Fatalf("presentation navigation wrote target: %v", err)
			}
			resultTestUnchanged(t, source, before)
		})
	}
}

func TestResultPresentationRetryUsesTheRealFailedOperation(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "Исправляемые исходники")
	op, issues := app.NewCreate(base, source, "Курс", "")
	resultTestIssues(t, issues)
	if issues := op.Analyze(resultTestTime(t)); len(issues) == 0 || !op.Snapshot().Actions.Retry {
		t.Fatal("fixture did not produce a real retryable analysis failure")
	}
	snapshot := op.Snapshot()
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	raw := resultTestBytes(t, "testdata/result/source.json")
	path := filepath.Join(source, "урок.json")
	resultTestWriteSource(t, path, raw)
	var output bytes.Buffer
	d := resultTestDialog(t, resultTestMenuInput(t, &output, "retry"), &output, op)
	got, err := d.presentationFailureChoice(snapshot)
	if err != nil || got != routePreview || d.operation != op {
		t.Fatalf("retry route=%v, error=%v, operation replaced=%t", got, err, d.operation != op)
	}
	resultTestMenu(t, output.String(), "retry", "restart", "main", "exit")
	after := op.Snapshot()
	if after.Token == snapshot.Token || len(after.Blockers) != 0 || after.Catalog == nil || after.Actions.Retry || after.WriteResult != nil {
		t.Fatal("selected retry did not analyze corrected source exactly as the same real operation")
	}
	resultTestUnchanged(t, path, raw)
}

func TestResultNavigationIncompleteEOFDoesNotSelectAction(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "presentation", true: "terminal"}[terminal], func(t *testing.T) {
			op, source, directory := resultTestCreate(t)
			path := resultTestConsent(t, op, directory)
			before := resultTestBytes(t, source)
			var output bytes.Buffer
			d := resultTestDialog(t, strings.NewReader("1"), &output, op)
			var err error
			if terminal {
				_, err = d.terminalChoice(op.Snapshot())
			} else {
				_, err = d.presentationFailureChoice(op.Snapshot())
			}
			if !errors.Is(err, io.EOF) {
				t.Fatalf("partial response accepted: %v", err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || op.Snapshot().WriteResult != nil {
				t.Error("EOF executed a file operation")
			}
			resultTestUnchanged(t, source, before)
		})
	}
}

func TestResultPresentationNavigationDoesNotRetryAutomatically(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "После ошибки")
	op, issues := app.NewCreate(base, source, "Курс", "")
	resultTestIssues(t, issues)
	if issues := op.Analyze(resultTestTime(t)); len(issues) == 0 {
		t.Fatal("missing source did not fail analysis")
	}
	snapshot := op.Snapshot()
	if !snapshot.Actions.Retry || snapshot.Catalog != nil {
		t.Fatal("fixture is not an analysis failure without catalog")
	}
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	raw := resultTestBytes(t, "testdata/result/source.json")
	resultTestWriteSource(t, filepath.Join(source, "урок.json"), raw)
	var output bytes.Buffer
	d := resultTestDialog(t, resultTestMenuInput(t, &output, "main"), &output, op)
	got, err := d.presentationFailureChoice(snapshot)
	if err != nil || got != routeMain {
		t.Fatalf("route=%v error=%v", got, err)
	}
	resultTestCanceled(t, op, snapshot.Token)
	if after := op.Snapshot(); after.Catalog != nil || len(after.Blockers) == 0 || after.WriteResult != nil {
		t.Error("navigation performed unselected analysis or write")
	}
}
