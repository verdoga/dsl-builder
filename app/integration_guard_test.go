package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestIntegrationOpaqueTokensAndChangedPlanRevokeConsent(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	integrationWriteInput(t, filepath.Join(source, "one.json"), integrationInput("ONE1", []string{"body"}))
	op := integrationCreate(t, base, source)
	other := integrationCreate(t, base, source)
	snapshot := op.Snapshot()
	for _, token := range []app.ReviewToken{{}, other.Snapshot().Token} {
		integrationCode(t, op.SetDestination(token, base, "foreign"), app.CodeStaleReview)
	}
	copied := *op
	integrationCode(t, copied.SetDestination(snapshot.Token, base, "copied"), app.CodeStaleReview)
	integrationOK(t, op.SetDestination(snapshot.Token, base, "one"))
	review, issues := op.ReviewSave(op.Snapshot().Token)
	integrationOK(t, issues)
	integrationCode(t, op.ConfirmSave(op.Snapshot().Token, true, false), app.CodeStaleReview)
	integrationCode(t, other.ConfirmSave(review.Token, true, false), app.CodeStaleReview)
	integrationOK(t, op.ConfirmSave(review.Token, true, false))
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Changed")
	integrationOK(t, issues)
	integrationOK(t, op.AcceptEdit(edit.Token))
	integrationCode(t, op.ConfirmSave(review.Token, true, false), app.CodeStaleReview)
	_, issues = op.Save(integrationAt())
	integrationCode(t, issues, app.CodeUnconfirmed)
	integrationAbsent(t, filepath.Join(base, "one.zip"))
	integrationSave(t, op, false)
}

func TestIntegrationExistingDestinationRequiresSeparateOverwrite(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	integrationWriteInput(t, filepath.Join(source, "one.json"), integrationInput("ONE1", []string{"body"}))
	op := integrationCreate(t, base, source)
	target := filepath.Join(base, "existing.zip")
	original := []byte("existing arbitrary bytes")
	integrationWrite(t, target, original)
	integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "existing"))
	review, issues := op.ReviewSave(op.Snapshot().Token)
	integrationOK(t, issues)
	if !review.Destination.Exists || review.Destination.Path != target {
		t.Fatal("existing target not shown")
	}
	integrationCode(t, op.ConfirmSave(review.Token, true, false), app.CodeOverwrite)
	_, issues = op.Save(integrationAt())
	if len(issues) == 0 {
		t.Fatal("missing overwrite allowed write")
	}
	integrationSameBytes(t, target, original)
	integrationSave(t, op, true)
}

func TestIntegrationChangedTargetIsNeverRebaselinedByReview(t *testing.T) {
	for _, phase := range []string{"after_load", "after_review", "after_confirm"} {
		t.Run(phase, func(t *testing.T) {
			base := t.TempDir()
			archive, _ := integrationArchive(t, base)
			path := filepath.Join(base, "update.json")
			integrationWriteInput(t, path, integrationInput("OLD1", []string{"changed", "unchanged"}))
			op, issues := app.NewUpdate(base, archive, path)
			integrationOK(t, issues)
			integrationOK(t, op.Analyze(integrationAt()))
			integrationOK(t, op.PrepareRevision(op.Snapshot().Token))
			var token app.ReviewToken
			if phase != "after_load" {
				review, issues := op.ReviewSave(op.Snapshot().Token)
				integrationOK(t, issues)
				token = review.Token
				if phase == "after_confirm" {
					integrationOK(t, op.ConfirmSave(token, true, true))
				}
			}
			replacement := append([]byte{}, integrationRead(t, archive)...)
			replacement = append(replacement, []byte("external change")...)
			integrationWrite(t, archive+".external", replacement)
			if err := os.Rename(archive+".external", archive); err != nil {
				t.Fatal(err)
			}
			if phase == "after_load" {
				_, issues = op.ReviewSave(op.Snapshot().Token)
				integrationCode(t, issues, app.CodeTargetChanged)
			} else if phase == "after_review" {
				issues = op.ConfirmSave(token, true, true)
				if len(issues) > 0 {
					integrationCode(t, issues, app.CodeTargetChanged)
				}
			}
			_, issues = op.Save(integrationAt())
			if len(issues) == 0 {
				t.Fatal("changed target overwritten")
			}
			if phase == "after_confirm" {
				integrationCode(t, issues, app.CodeTargetChanged)
			}
			integrationSameBytes(t, archive, replacement)
			_, issues = op.ReviewSave(op.Snapshot().Token)
			if len(issues) == 0 {
				t.Fatal("fresh ReviewSave rebaselined foreign ZIP")
			}
			integrationSameBytes(t, archive, replacement)
		})
	}
}

func TestIntegrationCreateTargetAppearanceAndInvalidNameDoNotWrite(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	integrationWriteInput(t, filepath.Join(source, "one.json"), integrationInput("ONE1", []string{"body"}))
	op := integrationCreate(t, base, source)
	for _, name := range []string{"CON", "bad/name", "trail.", "LPT¹.zip", "name "} {
		integrationCode(t, op.SetDestination(op.Snapshot().Token, base, name), app.CodeArchiveName)
	}
	integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "appeared"))
	review, issues := op.ReviewSave(op.Snapshot().Token)
	integrationOK(t, issues)
	integrationOK(t, op.ConfirmSave(review.Token, true, false))
	target := filepath.Join(base, "appeared.zip")
	raw := []byte("external")
	integrationWrite(t, target, raw)
	_, issues = op.Save(integrationAt())
	integrationCode(t, issues, app.CodeTargetChanged)
	integrationSameBytes(t, target, raw)
	integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "appeared"))
	integrationSave(t, op, true)
}
