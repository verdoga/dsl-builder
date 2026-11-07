package app_test

import (
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestDestinationConsoleRegressionSuccessfulChangeRevokesConsent(t *testing.T) {
	root := t.TempDir()
	source := destinationConsoleRegressionSource(t, root, "source")
	out := destinationConsoleRegressionDirectory(t, root, "out")
	destinationConsoleRegressionDirectory(t, root, "Другое назначение")
	destinationConsoleRegressionWrite(t, filepath.Join(out, "existing.zip"), []byte("сохранить исходные байты цели"))
	op := destinationConsoleRegressionNewCreate(t, root, source)
	destinationConsoleRegressionNoIssues(t, op.SetDestination(op.Snapshot().Token, "out", "existing"))
	before := op.Snapshot()
	model := destinationConsoleRegressionModel(t, before)
	preview, issues := op.ReviewSave(before.Token)
	destinationConsoleRegressionNoIssues(t, issues)
	if preview == nil || !preview.Destination.Exists {
		t.Fatalf("существующая цель не показана: %+v", preview)
	}
	destinationConsoleRegressionNoIssues(t, op.ConfirmSave(preview.Token, true, true))
	if !op.Snapshot().Actions.Save {
		t.Fatal("fixture не получил согласие для прежней цели")
	}
	destinationConsoleRegressionNoIssues(t, op.SetDestination(before.Token, "Другое назначение", "новый"))
	after := op.Snapshot()
	destinationConsoleRegressionWantTarget(t, after, destinationConsoleRegressionTarget(t, root, "Другое назначение", "новый.zip", false))
	if after.Token == before.Token || destinationConsoleRegressionModel(t, after) != model {
		t.Fatal("смена цели не заменила token либо изменила предметную модель")
	}
	destinationConsoleRegressionIssue(t, op.ConfirmSave(preview.Token, true, true), app.CodeStaleReview)
	destinationConsoleRegressionIssue(t, op.SetDestination(before.Token, "out", "stale"), app.CodeStaleReview)
	destinationConsoleRegressionWantTarget(t, op.Snapshot(), *after.Target)
	destinationConsoleRegressionNoConsent(t, op)
}

func TestDestinationConsoleRegressionTokensCannotSelectTarget(t *testing.T) {
	root := t.TempDir()
	source := destinationConsoleRegressionSource(t, root, "source")
	destinationConsoleRegressionDirectory(t, root, "out")
	op := destinationConsoleRegressionNewCreate(t, root, source)
	other := destinationConsoleRegressionNewCreate(t, root, source)
	before := op.Snapshot()
	for _, test := range []struct {
		name  string
		token app.ReviewToken
	}{
		{name: "zero"},
		{name: "foreign", token: other.Snapshot().Token},
	} {
		t.Run(test.name, func(t *testing.T) {
			destinationConsoleRegressionIssue(t, op.SetDestination(test.token, "out", "result"), app.CodeStaleReview)
			after := op.Snapshot()
			if after.Token != before.Token || after.Target != nil || after.Actions.Save {
				t.Fatal("непринадлежащий token изменил цель либо разрешил запись")
			}
		})
	}
	destinationConsoleRegressionNoConsent(t, op)
}

func TestDestinationConsoleRegressionSeparateOverwriteConsent(t *testing.T) {
	for _, test := range []struct {
		name   string
		accept bool
	}{
		{name: "explicit refusal", accept: false},
		{name: "write consent without overwrite", accept: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := destinationConsoleRegressionSource(t, root, "source")
			out := destinationConsoleRegressionDirectory(t, root, "out")
			destinationConsoleRegressionWrite(t, filepath.Join(out, "result.zip"), []byte("исходный файл не изменяется"))
			op := destinationConsoleRegressionNewCreate(t, root, source)
			destinationConsoleRegressionNoIssues(t, op.SetDestination(op.Snapshot().Token, "out", "result"))
			snapshot := op.Snapshot()
			preview, issues := op.ReviewSave(snapshot.Token)
			destinationConsoleRegressionNoIssues(t, issues)
			want := destinationConsoleRegressionTarget(t, root, "out", "result.zip", true)
			if preview == nil || preview.Destination != want {
				t.Fatalf("неверное наблюдение существующей цели: %+v", preview)
			}
			destinationConsoleRegressionIssue(t, op.ConfirmSave(snapshot.Token, true, true), app.CodeStaleReview)
			issues = op.ConfirmSave(preview.Token, test.accept, false)
			if test.accept {
				destinationConsoleRegressionIssue(t, issues, app.CodeOverwrite)
			} else {
				destinationConsoleRegressionNoIssues(t, issues)
			}
			destinationConsoleRegressionWantTarget(t, op.Snapshot(), want)
			destinationConsoleRegressionNoConsent(t, op)
		})
	}
}

func TestDestinationConsoleRegressionInvalidArchiveNameKeepsTarget(t *testing.T) {
	root := t.TempDir()
	source := destinationConsoleRegressionSource(t, root, "source")
	destinationConsoleRegressionDirectory(t, root, "out")
	op := destinationConsoleRegressionNewCreate(t, root, source)
	destinationConsoleRegressionNoIssues(t, op.SetDestination(op.Snapshot().Token, "out", "previous"))
	before := op.Snapshot()
	for _, name := range []string{"", "sub/result", `sub\result`, "CON", "name.", `"name"`} {
		t.Run(name, func(t *testing.T) {
			destinationConsoleRegressionIssue(t, op.SetDestination(before.Token, "out", name), app.CodeArchiveName)
			after := op.Snapshot()
			destinationConsoleRegressionWantTarget(t, after, *before.Target)
			if after.Token != before.Token || after.Actions.Save {
				t.Fatal("неверное имя изменило принятый token либо согласие")
			}
		})
	}
	destinationConsoleRegressionNoConsent(t, op)
}
