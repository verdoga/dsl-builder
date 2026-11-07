package app

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/compare"
)

func TestDocUpdateLifecycleRequiresIndividualDecisionsAndOneRevision(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "old.json")
	docFixtureWrite(t, source, docFixtureInput("DU01", "Old title", "Old subtitle", "Old section", "10", "old body"))
	archive, original := docFixtureArchive(t, base, source, "l007.json")
	archiveBytes := docFixtureBytes(t, archive)
	incoming := filepath.Join(base, "incoming.json")
	inputBytes := docFixtureWrite(t, incoming, docFixtureInput("DU01", "New title", "New subtitle", "New section", "20", "new body"))
	op, issues := NewUpdate(base, archive, incoming)
	docFixtureOK(t, issues)
	at := docFixtureAt().Add(24 * time.Hour)
	docFixtureOK(t, op.Analyze(at))
	first := op.Snapshot()
	if first.Mode != Update || first.Plan == nil || len(first.Plan.Fields) != 2 || len(first.PlacementProposals) != 2 || !first.Actions.DecideFields || !first.Actions.DecidePlacement {
		t.Fatalf("missing separate proposals: %+v", first)
	}
	first.Plan.Fields[0].After = docFixtureText("forged field")
	first.Plan.Pages = nil
	first.Plan.ReplaceLesson = true
	first.FieldAnswered[0], first.FieldAccepted[0] = true, true
	first.PlacementProposals[0].After[0] = '!'
	first.PlacementAnswered[0] = true
	unchanged := op.Snapshot()
	if unchanged.Plan.ReplaceLesson || len(unchanged.Plan.Pages) == 0 || unchanged.FieldAnswered[0] || unchanged.PlacementAnswered[0] || *unchanged.Plan.Fields[0].After == "forged field" {
		t.Fatal("snapshot plan/decision mutation patched the operation")
	}
	if _, issues := op.ReviewSave(unchanged.Token); len(issues) == 0 {
		t.Fatal("review allowed unresolved decisions")
	}
	if issues := op.PrepareRevision(unchanged.Token); len(issues) == 0 {
		t.Fatal("revision allowed unresolved fields")
	}
	for index, proposal := range unchanged.Plan.Fields {
		accept := proposal.Field == compare.DocumentSubtitle
		docFixtureOK(t, op.DecideField(op.Snapshot().Token, index, accept))
		current := op.Snapshot()
		for later := index + 1; later < len(current.FieldAnswered); later++ {
			if current.FieldAnswered[later] {
				t.Fatal("field decision implicitly answered another field")
			}
		}
	}
	if !op.Snapshot().Actions.DecidePlacement {
		t.Fatal("field confirmations answered placement proposals")
	}
	for index, proposal := range unchanged.PlacementProposals {
		docFixtureOK(t, op.DecidePlacement(op.Snapshot().Token, index, proposal.Field == "order"))
	}
	docFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
	prepared := op.Snapshot()
	if !prepared.RevisionReady || prepared.NoChanges || !prepared.Actions.ReviewSave {
		t.Fatalf("final update not reviewable: %+v", prepared.Actions)
	}
	document := prepared.After.Lesson().Document
	if document.Title != "Old title" || document.Subtitle == nil || *document.Subtitle != "New subtitle" || document.Version.String() != "2" || document.CreatedAt != at.Format(time.RFC3339) {
		t.Fatalf("revision differs from individual decisions: %+v", document)
	}
	item := prepared.Catalog.Items[0]
	if *item.Section != "Old section" || *item.Entry.Order != "20" || item.Entry.Title != "Old title" || item.Entry.Version.String() != "2" {
		t.Fatalf("placement and lesson decisions were mixed: %+v", item)
	}
	docFixtureOK(t, op.PrepareRevision(prepared.Token))
	if repeated := op.Snapshot().After.Lesson().Document; repeated.Version.String() != "2" || repeated.CreatedAt != document.CreatedAt {
		t.Fatal("preparing twice incremented from revised state")
	}
	if !bytes.Equal(archiveBytes, docFixtureBytes(t, archive)) || !bytes.Equal(inputBytes, docFixtureBytes(t, incoming)) {
		t.Fatal("analysis/decisions/preview wrote sources")
	}
	preview, issues := op.ReviewSave(op.Snapshot().Token)
	docFixtureOK(t, issues)
	if preview == nil || !preview.Destination.Exists || preview.Destination.Path != archive {
		t.Fatal("update did not show original path as overwrite")
	}
	docFixtureCode(t, op.ConfirmSave(preview.Token, true, false), CodeOverwrite)
	if _, issues := op.Save(at); len(issues) == 0 {
		t.Fatal("update wrote without separate overwrite permission")
	}
	if !bytes.Equal(archiveBytes, docFixtureBytes(t, archive)) {
		t.Fatal("denied overwrite changed original archive")
	}
	loaded := docFixtureSave(t, op, true)
	if len(loaded.Files) != 1 || loaded.Files[0].FileName() != original.Files[0].FileName() || loaded.Files[0].Lesson().Document.Version.String() != "2" || loaded.Files[0].Lesson().Document.Title != "Old title" {
		t.Fatal("real Read did not recover accepted update")
	}
}

func TestDocUpdateNoChangesKeepsOldBytesAndManifestOnlySaveReusesThem(t *testing.T) {
	for _, rejectedTitle := range []bool{false, true} {
		t.Run(map[bool]string{false: "identical", true: "rejected-title"}[rejectedTitle], func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "old.json")
			docFixtureWrite(t, source, docFixtureInput("DU02", "Old title", "", "", "10", "body"))
			archive, original := docFixtureArchive(t, base, source, "l003.json")
			archiveBytes := docFixtureBytes(t, archive)
			title := "Old title"
			if rejectedTitle {
				title = "Rejected title"
			}
			incoming := filepath.Join(base, "incoming.json")
			docFixtureWrite(t, incoming, docFixtureInput("DU02", title, "", "", "", "body"))
			op, issues := NewUpdate(base, archive, incoming)
			docFixtureOK(t, issues)
			docFixtureOK(t, op.Analyze(docFixtureAt().Add(72*time.Hour)))
			for index := range op.Snapshot().Plan.Fields {
				docFixtureOK(t, op.DecideField(op.Snapshot().Token, index, false))
			}
			docFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
			snapshot := op.Snapshot()
			if !snapshot.NoChanges || snapshot.Actions.ReviewSave || snapshot.After.Lesson().Document.Version.String() != "1" {
				t.Fatal("time/rejected fields fabricated a revision")
			}
			oldRaw := original.Files[0].OriginalBytes()
			if !bytes.Equal(snapshot.After.OriginalBytes(), oldRaw) {
				t.Fatal("unchanged update discarded original bytes")
			}
			if preview, issues := op.ReviewSave(snapshot.Token); preview != nil || len(issues) == 0 {
				t.Fatal("no changes produced a write proposal")
			}
			if _, issues := op.Save(docFixtureAt()); len(issues) == 0 {
				t.Fatal("no changes wrote archive")
			}
			if !bytes.Equal(archiveBytes, docFixtureBytes(t, archive)) {
				t.Fatal("no changes altered archive bytes")
			}
			proposal, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Changed course")
			docFixtureOK(t, issues)
			docFixtureOK(t, op.AcceptEdit(proposal.Token))
			loaded := docFixtureSave(t, op, true)
			if loaded.Manifest.Course.Title != "Changed course" || !bytes.Equal(loaded.Files[0].OriginalBytes(), oldRaw) || loaded.Files[0].Lesson().Document.CreatedAt != original.Files[0].Lesson().Document.CreatedAt {
				t.Fatal("manifest-only save rewrote unchanged lesson")
			}
		})
	}
}
