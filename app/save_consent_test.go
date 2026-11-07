package app

import (
	"bytes"
	"testing"
)

func TestSavePreviewIsIndependentAndUsesSeparateToken(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	snapshot := op.Snapshot()
	before := savePrefixModel(t, snapshot)
	preview := savePrefixReview(t, op)
	token := preview.Token
	if token == snapshot.Token || token == (ReviewToken{}) || preview.Snapshot.Token != snapshot.Token {
		t.Fatal("SavePreview must have own token bound to accepted base")
	}
	preview.Destination.Path = target + ".foreign"
	preview.Destination.Exists = true
	preview.Snapshot.Target.Path = target + ".snapshot"
	preview.Snapshot.Catalog.CourseTitle = "tampered"
	*preview.Snapshot.Catalog.Items[0].Entry.Order = "999"
	preview.Snapshot.Module.Manifest.Course.Title = "tampered"
	preview.Snapshot.Module.Files[0] = nil
	preview.Token = ReviewToken{}
	if !bytes.Equal(before, savePrefixModel(t, op.Snapshot())) {
		t.Fatal("SavePreview leaked mutable state")
	}
	savePrefixOK(t, op.ConfirmSave(token, true, false))
	result, issues := op.Save(savePrefixTime())
	savePrefixOK(t, issues)
	if !result.Installed || result.TargetPath != target {
		t.Fatalf("preview mutation changed saved target: %+v", result)
	}
	savePrefixAbsent(t, target+".foreign")
	savePrefixAbsent(t, target+".snapshot")
}

func TestSaveDeclinedConsentPreservesAcceptedModel(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	snapshot := op.Snapshot()
	before := savePrefixModel(t, snapshot)
	preview := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, false, true))
	after := op.Snapshot()
	if !bytes.Equal(before, savePrefixModel(t, after)) || snapshot.Token != after.Token || after.Actions.Save {
		t.Fatal("declined Save changed model, base token or granted consent")
	}
	_, issues := op.Save(savePrefixTime())
	savePrefixCode(t, issues, CodeUnconfirmed)
	savePrefixAbsent(t, target)
	if len(op.ConfirmSave(preview.Token, true, true)) == 0 {
		t.Fatal("discarded preview token was reusable")
	}
	fresh := savePrefixReview(t, op)
	if fresh.Token == preview.Token {
		t.Fatal("fresh preview reused discarded identity")
	}
}

func TestSaveRequiresSeparateOverwriteConsent(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	original := []byte("already existing user file")
	savePrefixWrite(t, target, original)
	// Create может повторно показать появившуюся цель, получив новое согласие.
	savePrefixOK(t, op.SetDestination(op.Snapshot().Token, op.Snapshot().Target.Directory, "Результат"))
	preview := savePrefixReview(t, op)
	if !preview.Destination.Exists {
		t.Fatal("existing target not shown")
	}
	issues := op.ConfirmSave(preview.Token, true, false)
	savePrefixCode(t, issues, CodeOverwrite)
	_, issues = op.Save(savePrefixTime())
	if len(issues) == 0 {
		t.Fatal("Save overwrote without separate permission")
	}
	if !bytes.Equal(original, savePrefixRead(t, target)) {
		t.Fatal("target changed before overwrite consent")
	}
	preview = savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, true))
	result, issues := op.Save(savePrefixTime())
	savePrefixOK(t, issues)
	if !result.Installed || bytes.Equal(original, savePrefixRead(t, target)) {
		t.Fatal("explicitly confirmed replacement did not install ZIP")
	}
}

func TestSaveRejectsWrongForeignStaleAndCopiedOwnerTokens(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	other, _, _ := savePrefixCreate(t)
	old := savePrefixReview(t, op)
	current := savePrefixReview(t, op)
	foreign := savePrefixReview(t, other)
	for name, token := range map[string]ReviewToken{
		"zero": {}, "snapshot": op.Snapshot().Token, "foreign": foreign.Token, "stale": old.Token,
	} {
		t.Run(name, func(t *testing.T) {
			savePrefixCode(t, op.ConfirmSave(token, true, true), CodeStaleReview)
			savePrefixAbsent(t, target)
		})
	}
	copyOp := *op
	before := op.Snapshot()
	if len(copyOp.ConfirmSave(current.Token, true, false)) == 0 {
		t.Fatal("copied operation accepted original save token")
	}
	result, issues := copyOp.Save(savePrefixTime())
	if len(issues) == 0 || result.Installed || result.TargetPath != "" || result.TemporaryPath != "" || result.OriginalPath != "" {
		t.Fatal("copied operation attempted a write")
	}
	savePrefixAbsent(t, target)
	after := op.Snapshot()
	if before.Token != after.Token || !bytes.Equal(savePrefixModel(t, before), savePrefixModel(t, after)) || after.Actions.Save {
		t.Fatal("copied receiver changed original accepted state or consent")
	}
	savePrefixOK(t, op.ConfirmSave(current.Token, true, false))
	if !op.Snapshot().Actions.Save {
		t.Fatal("invalid decisions poisoned original consent")
	}
}

func TestSaveAcceptedEditRevokesConsent(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	preview := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Изменённый курс")
	savePrefixOK(t, issues)
	if edit == nil {
		t.Fatal("nil edit")
	}
	_, issues = op.Save(savePrefixTime())
	if len(issues) == 0 {
		t.Fatal("Save accepted active edit")
	}
	savePrefixAbsent(t, target)
	savePrefixOK(t, op.AcceptEdit(edit.Token))
	if len(op.ConfirmSave(preview.Token, true, false)) == 0 {
		t.Fatal("stale base accepted old save token")
	}
	_, issues = op.Save(savePrefixTime())
	if len(issues) == 0 {
		t.Fatal("accepted edit retained old final consent")
	}
	savePrefixAbsent(t, target)
	fresh := savePrefixReview(t, op)
	if fresh.Snapshot.Catalog.CourseTitle != "Изменённый курс" {
		t.Fatal("fresh preview omitted accepted edit")
	}
}
