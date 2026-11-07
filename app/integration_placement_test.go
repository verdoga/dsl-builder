package app_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
)

func TestIntegrationUpdateAcceptedSectionAndRejectedWeightDoNotReviseLessons(t *testing.T) {
	base := t.TempDir()
	archive, rawFiles := integrationArchive(t, base)
	input := integrationInput("OLD1", []string{"old body", "unchanged"})
	input.Document.Metadata.Section = integrationText("New section")
	input.Document.Metadata.Order = integrationText("99")
	path := filepath.Join(base, "placement.json")
	integrationWriteInput(t, path, input)
	op, issues := app.NewUpdate(base, archive, path)
	integrationOK(t, issues)
	integrationOK(t, op.Analyze(integrationAt()))
	snapshot := op.Snapshot()
	if len(snapshot.Plan.Fields) != 0 || len(snapshot.PlacementProposals) != 2 {
		t.Fatal("placement fixture produced wrong proposals")
	}
	for index, change := range snapshot.PlacementProposals {
		issues = op.DecidePlacement(op.Snapshot().Token, index, change.Field == "section")
		if change.Field == "section" {
			integrationCode(t, issues, catalog.CodeUnassigned)
		} else {
			integrationOK(t, issues)
		}
	}
	integrationCode(t, op.PrepareRevision(op.Snapshot().Token), catalog.CodeUnassigned)
	snapshot = op.Snapshot()
	if !snapshot.RevisionReady || snapshot.After.Lesson().Document.Version != "1" || !bytes.Equal(snapshot.After.OriginalBytes(), rawFiles["l001.json"]) {
		t.Fatal("placement-only operation changed lesson")
	}
	for _, item := range snapshot.Catalog.Items {
		if item.Entry.FileName == "l001.json" && (*item.Entry.Order != "10" || item.Section == nil || *item.Section != "New section") {
			t.Fatal("section/weight decisions not separate")
		}
	}
	edit, issues := op.ProposeMove(snapshot.Token, "l003.json", integrationText("New section"))
	integrationOK(t, issues)
	integrationOK(t, op.AcceptEdit(edit.Token))
	value := integrationSave(t, op, true)
	for name, raw := range rawFiles {
		if !bytes.Equal(integrationFile(t, value, name).OriginalBytes(), raw) {
			t.Fatalf("placement changed bytes: %s", name)
		}
	}
}

func TestIntegrationChangingFieldDecisionInvalidatesPreparedRevisionAndConsent(t *testing.T) {
	base := t.TempDir()
	archive, _ := integrationArchive(t, base)
	input := integrationInput("OLD1", []string{"changed", "unchanged"})
	input.Document.Metadata.Subtitle = integrationText("Proposed subtitle")
	path := filepath.Join(base, "field.json")
	integrationWriteInput(t, path, input)
	op, issues := app.NewUpdate(base, archive, path)
	integrationOK(t, issues)
	integrationOK(t, op.Analyze(integrationAt()))
	snapshot := op.Snapshot()
	if len(snapshot.Plan.Fields) != 1 {
		t.Fatal("subtitle fixture must produce one field")
	}
	integrationOK(t, op.DecideField(snapshot.Token, 0, true))
	integrationOK(t, op.PrepareRevision(op.Snapshot().Token))
	review, issues := op.ReviewSave(op.Snapshot().Token)
	integrationOK(t, issues)
	integrationOK(t, op.ConfirmSave(review.Token, true, true))
	integrationOK(t, op.DecideField(op.Snapshot().Token, 0, false))
	snapshot = op.Snapshot()
	if snapshot.RevisionReady || snapshot.Actions.Save || snapshot.Actions.ReviewSave {
		t.Fatal("changed field kept prepared/confirmed revision")
	}
	_, issues = op.Save(integrationAt())
	if len(issues) == 0 {
		t.Fatal("obsolete revision saved")
	}
	integrationOK(t, op.PrepareRevision(snapshot.Token))
	after := op.Snapshot().After.Lesson()
	if after.Document.Version != "2" || after.Document.Subtitle != nil || after.Document.Pages[0].Nodes[0].Nodes[0].ID != "old1-n002-v02" {
		t.Fatal("second preparation was not derived from immutable old")
	}
	value := integrationSave(t, op, true)
	if integrationFile(t, value, "l001.json").Lesson().Document.Version != "2" {
		t.Fatal("one operation increased revision twice")
	}
}
