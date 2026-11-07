package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveNilReceiverReportsState(t *testing.T) {
	var op *Operation
	preview, issues := op.ReviewSave(ReviewToken{})
	if preview != nil {
		t.Fatal("nil operation produced preview")
	}
	savePrefixCode(t, issues, CodeState)
	savePrefixCode(t, op.ConfirmSave(ReviewToken{}, true, true), CodeState)
	result, issues := op.Save(savePrefixTime())
	savePrefixCode(t, issues, CodeState)
	if result.Installed {
		t.Fatal("nil operation installed result")
	}
}

func TestSaveReviewRequiresCurrentAcceptedToken(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	other, _, _ := savePrefixCreate(t)
	preview := savePrefixReview(t, op)
	for name, token := range map[string]ReviewToken{
		"zero": {}, "foreign snapshot": other.Snapshot().Token, "save token": preview.Token,
	} {
		t.Run(name, func(t *testing.T) {
			candidate, issues := op.ReviewSave(token)
			if candidate != nil {
				t.Fatal("wrong token produced preview")
			}
			savePrefixCode(t, issues, CodeStaleReview)
			savePrefixAbsent(t, target)
		})
	}
}

func TestSaveReviewAloneDoesNotAuthorizeAttempt(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	for attempt := 0; attempt < 2; attempt++ {
		result, issues := op.Save(savePrefixTime())
		savePrefixCode(t, issues, CodeUnconfirmed)
		if result.Installed {
			t.Fatal("unconfirmed Save installed result")
		}
		savePrefixAbsent(t, target)
		savePrefixReview(t, op)
	}
}

func TestSaveRetryAndCancelRevokeFinalConsent(t *testing.T) {
	t.Run("clean retry is inapplicable", func(t *testing.T) {
		op, _, target := savePrefixCreate(t)
		preview := savePrefixReview(t, op)
		savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
		before := op.Snapshot()
		savePrefixCode(t, op.Retry(savePrefixTime()), CodeState)
		after := op.Snapshot()
		if before.Token != after.Token || !bytes.Equal(savePrefixModel(t, before), savePrefixModel(t, after)) || !after.Actions.Save {
			t.Fatal("inapplicable Retry changed accepted state or consent")
		}
		savePrefixAbsent(t, target)
	})
	t.Run("retry after baseline failure", func(t *testing.T) {
		op, source, archive := savePrefixUpdate(t, "Новое название", nil)
		savePrefixDecide(t, op, true)
		accepted := op.Snapshot().Token
		preview := savePrefixReview(t, op)
		savePrefixOK(t, op.ConfirmSave(preview.Token, true, true))
		original := savePrefixRead(t, archive)
		input := savePrefixRead(t, source)
		info, err := os.Stat(archive)
		if err != nil {
			t.Fatal(err)
		}
		changed := info.ModTime().Add(24 * time.Hour)
		if err := os.Chtimes(archive, changed, changed); err != nil {
			t.Fatal(err)
		}
		result, issues := op.Save(savePrefixTime())
		savePrefixCode(t, issues, CodeTargetChanged)
		if result.Installed || !op.Snapshot().Actions.Retry {
			t.Fatal("baseline failure did not make Retry applicable")
		}
		savePrefixOK(t, op.Retry(savePrefixTime()))
		savePrefixCode(t, op.ConfirmSave(preview.Token, true, true), CodeStaleReview)
		candidate, issues := op.ReviewSave(accepted)
		savePrefixCode(t, issues, CodeStaleReview)
		if candidate != nil || op.Snapshot().Token == accepted || op.Snapshot().Actions.Save {
			t.Fatal("reanalysis retained previous authorization")
		}
		result, issues = op.Save(savePrefixTime())
		savePrefixCode(t, issues, CodeUnconfirmed)
		if result.Installed || op.Snapshot().WriteResult != nil {
			t.Fatal("reset operation attempted an unconfirmed write")
		}
		savePrefixDecide(t, op, true)
		fresh := savePrefixReview(t, op)
		if fresh.Token == preview.Token {
			t.Fatal("reanalysis reused previous save token")
		}
		savePrefixOK(t, op.ConfirmSave(fresh.Token, true, true))
		if !op.Snapshot().Actions.Save || !bytes.Equal(original, savePrefixRead(t, archive)) || !bytes.Equal(input, savePrefixRead(t, source)) {
			t.Fatal("fresh consent unavailable or lifecycle reset changed input bytes")
		}
	})
	t.Run("retry with weight conflicts", func(t *testing.T) {
		base := t.TempDir()
		first := filepath.Join(base, "first.json")
		second := filepath.Join(base, "second.json")
		weight := "10"
		savePrefixSource(t, first, "", &weight)
		savePrefixSource(t, second, "", &weight)
		savePrefixWrite(t, second, bytes.ReplaceAll(savePrefixRead(t, second), []byte("BA82"), []byte("ZZ99")))
		op, issues := NewCreate(base, base, "Курс", "")
		savePrefixOK(t, issues)
		issues = op.Analyze(savePrefixTime())
		blocked := op.Snapshot()
		if len(issues) == 0 || blocked.Catalog == nil || len(blocked.Conflicts) != 1 || !blocked.Actions.Retry {
			t.Fatal("fixture did not reach editable weight conflict")
		}
		weight = "20"
		savePrefixSource(t, second, "", &weight)
		savePrefixWrite(t, second, bytes.ReplaceAll(savePrefixRead(t, second), []byte("BA82"), []byte("ZZ99")))
		savePrefixOK(t, op.Retry(savePrefixTime()))
		fresh := op.Snapshot()
		if !fresh.ManifestReady || fresh.Target != nil || len(fresh.Blockers) != 0 || len(fresh.Module.Files) != 2 || fresh.Token == blocked.Token {
			t.Fatal("Retry did not rebuild corrected source placement")
		}
		candidate, issues := op.ReviewSave(blocked.Token)
		savePrefixCode(t, issues, CodeStaleReview)
		if candidate != nil {
			t.Fatal("preanalysis token authorized a new plan")
		}
		savePrefixAbsent(t, filepath.Join(base, "Результат.zip"))
	})
	t.Run("cancel", func(t *testing.T) {
		op, _, target := savePrefixCreate(t)
		preview := savePrefixReview(t, op)
		savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
		op.Cancel()
		if len(op.ConfirmSave(preview.Token, true, true)) == 0 {
			t.Fatal("old consent survived lifecycle reset")
		}
		result, issues := op.Save(savePrefixTime())
		if len(issues) == 0 || result.Installed {
			t.Fatal("reset operation reused final consent")
		}
		savePrefixAbsent(t, target)
	})
}
