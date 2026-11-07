package app_test

import (
	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"testing"
	"time"
)

func integrationCreate(t *testing.T, base, source string) *app.Operation {
	t.Helper()
	op, issues := app.NewCreate(base, source, "Курс интеграции", "")
	integrationOK(t, issues)
	integrationOK(t, op.Analyze(integrationAt()))
	return op
}

func integrationSave(t *testing.T, op *app.Operation, replace bool) *module.Module {
	t.Helper()
	review, issues := op.ReviewSave(op.Snapshot().Token)
	integrationOK(t, issues)
	if review == nil {
		t.Fatal("nil save review")
	}
	integrationOK(t, op.ConfirmSave(review.Token, true, replace))
	result, issues := op.Save(integrationAt().Add(24 * time.Hour))
	integrationOK(t, issues)
	if !result.Installed || result.TargetPath != review.Destination.Path || result.TemporaryPath != "" || result.OriginalPath != "" {
		t.Fatalf("incorrect result: %+v", result)
	}
	value, issues := moduleio.Read(result.TargetPath)
	integrationOK(t, issues)
	return value
}
