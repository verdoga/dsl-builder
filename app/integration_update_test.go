package app_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/compare"
)

func TestIntegrationUpdateDecisionsRevisionAndOriginalBytes(t *testing.T) {
	base := t.TempDir()
	archive, rawFiles := integrationArchive(t, base)
	before := integrationRead(t, archive)
	input := integrationInput("old1", []string{"changed body", "unchanged"})
	input.Document.Metadata.Title = integrationText("New title rejected")
	input.Document.Metadata.Subtitle = integrationText("New subtitle accepted")
	input.Document.Metadata.Section = integrationText("New section rejected")
	input.Document.Metadata.Order = integrationText("030")
	path := filepath.Join(base, "update.json")
	integrationWriteInput(t, path, input)
	sourceBytes := integrationRead(t, path)
	op, issues := app.NewUpdate(base, archive, path)
	integrationOK(t, issues)
	integrationOK(t, op.Analyze(integrationAt()))
	initial := op.Snapshot()
	if initial.Before == nil || initial.Plan == nil || len(initial.Plan.Fields) != 2 || len(initial.PlacementProposals) != 2 || initial.RevisionReady {
		t.Fatalf("incomplete update preview: %+v", initial)
	}
	if !bytes.Equal(initial.Before.OriginalBytes(), rawFiles["l001.json"]) {
		t.Fatal("baseline bytes lost")
	}
	integrationCode(t, op.PrepareRevision(initial.Token), app.CodeDecision)
	for index, change := range initial.Plan.Fields {
		integrationOK(t, op.DecideField(op.Snapshot().Token, index, change.Field != compare.DocumentTitle))
	}
	for index, change := range initial.PlacementProposals {
		integrationOK(t, op.DecidePlacement(op.Snapshot().Token, index, change.Field == "order"))
	}
	integrationOK(t, op.PrepareRevision(op.Snapshot().Token))
	prepared := op.Snapshot()
	after := prepared.After.Lesson()
	if after.Document.ID != "OLD1" || after.Document.Version != "2" || after.Document.Title != "Lesson OLD1" || after.Document.Subtitle == nil || *after.Document.Subtitle != "New subtitle accepted" {
		t.Fatalf("wrong accepted metadata: %+v", after.Document)
	}
	nodes := after.Document.Pages[0].Nodes
	if nodes[0].ID != "old1-n001" || nodes[0].Nodes[0].ID != "old1-n002-v02" || nodes[0].Nodes[1].ID != "old1-n003" {
		t.Fatalf("wrong node IDs: %+v", nodes)
	}
	for _, item := range prepared.Catalog.Items {
		if item.Entry.FileName == "l001.json" && (item.Section != nil || item.Entry.Order == nil || *item.Entry.Order != "030") {
			t.Fatalf("unconfirmed placement applied: %+v", item)
		}
	}
	integrationOK(t, op.PrepareRevision(prepared.Token))
	if op.Snapshot().After.Lesson().Document.Version != "2" {
		t.Fatal("PrepareRevision incremented twice")
	}
	prepared = op.Snapshot()
	prepared.Plan.Fields[0].After = integrationText("forged")
	prepared.Plan.Pages[0].Nodes[0].Path[0] = 999
	prepared.FieldAccepted[0] = true
	prepared.Module.Manifest.Course.Title = "forged"
	prepared.Before.OriginalBytes()[0] = 'X'
	prepared.After.Lesson().Document.Title = "forged"
	fresh := op.Snapshot()
	if *fresh.Plan.Fields[0].After == "forged" || fresh.Plan.Pages[0].Nodes[0].Path[0] == 999 || fresh.After.Lesson().Document.Title != "Lesson OLD1" {
		t.Fatal("update preview mutated operation")
	}
	integrationSameBytes(t, archive, before)
	value := integrationSave(t, op, true)
	changed := integrationFile(t, value, "l001.json").Lesson().Document
	if changed.Version != "2" || changed.Title != "Lesson OLD1" || changed.CreatedAt != integrationAt().UTC().Format("2006-01-02T15:04:05Z") {
		t.Fatalf("saved wrong lesson: %+v", changed)
	}
	if !bytes.Equal(integrationFile(t, value, "l003.json").OriginalBytes(), rawFiles["l003.json"]) {
		t.Fatal("untouched file was reserialized")
	}
	integrationSameBytes(t, path, sourceBytes)
}

func TestIntegrationUpdateReplacementLevels(t *testing.T) {
	for _, scenario := range []string{"page", "lesson"} {
		t.Run(scenario, func(t *testing.T) {
			base := t.TempDir()
			archive, rawFiles := integrationArchive(t, base)
			input := integrationInput("OLD1", []string{"old body", "unchanged", "inserted"})
			if scenario == "lesson" {
				input = integrationInput("OLD1", []string{"old body", "unchanged"}, []string{"new page"})
			}
			path := filepath.Join(base, "replacement.json")
			integrationWriteInput(t, path, input)
			op, issues := app.NewUpdate(base, archive, path)
			integrationOK(t, issues)
			integrationOK(t, op.Analyze(integrationAt()))
			snapshot := op.Snapshot()
			if scenario == "page" && (snapshot.Plan.ReplaceLesson || len(snapshot.Plan.Pages) != 1 || !snapshot.Plan.Pages[0].ReplaceNodes || len(snapshot.Plan.Pages[0].Reasons) == 0) {
				t.Fatalf("wrong page replacement plan: %+v", snapshot.Plan)
			}
			if scenario == "lesson" && (!snapshot.Plan.ReplaceLesson || len(snapshot.Plan.Pages) != 0 || len(snapshot.Plan.Reasons) == 0) {
				t.Fatalf("wrong lesson replacement plan: %+v", snapshot.Plan)
			}
			for index, change := range snapshot.Plan.Fields {
				if change.Field == compare.PageTitle && change.OldPage == -1 && change.Before != nil {
					t.Fatal("invented old page title")
				}
				integrationOK(t, op.DecideField(op.Snapshot().Token, index, false))
			}
			integrationOK(t, op.PrepareRevision(op.Snapshot().Token))
			after := op.Snapshot().After.Lesson()
			if after.Document.Version != "2" {
				t.Fatal("wrong revision")
			}
			if scenario == "page" {
				if after.Document.Pages[0].ID != "old1-p001" || after.Document.Pages[0].Nodes[0].ID != "old1-n004-v02" {
					t.Fatalf("wrong page replacement IDs: %+v", after.Document.Pages)
				}
			} else {
				if after.Document.Pages[0].ID != "old1-p002" || after.Document.Pages[1].ID != "old1-p003" {
					t.Fatalf("wrong full replacement page IDs: %+v", after.Document.Pages)
				}
				for _, change := range snapshot.Plan.Fields {
					if change.Field == compare.PageTitle && change.OldPage == -1 && after.Document.Pages[change.NewPage].Title != nil {
						t.Fatal("rejected unmatched title did not become null")
					}
				}
			}
			for _, page := range after.Document.Pages {
				for _, node := range page.Nodes {
					if !strings.HasSuffix(node.ID, "-v02") {
						t.Fatalf("missing revision suffix: %s", node.ID)
					}
					for _, child := range node.Nodes {
						if !strings.HasSuffix(child.ID, "-v02") {
							t.Fatalf("missing child suffix: %s", child.ID)
						}
					}
				}
			}
			value := integrationSave(t, op, true)
			if !bytes.Equal(integrationFile(t, value, "l003.json").OriginalBytes(), rawFiles["l003.json"]) {
				t.Fatal("other lesson changed")
			}
		})
	}
}

func TestIntegrationUpdateRejectAllAndManifestOnlyKeepRaw(t *testing.T) {
	base := t.TempDir()
	archive, rawFiles := integrationArchive(t, base)
	before := integrationRead(t, archive)
	input := integrationInput("old1", []string{"old body", "unchanged"})
	input.Document.Metadata.Title = integrationText("Rejected")
	input.Document.Metadata.Subtitle = integrationText("Rejected subtitle")
	path := filepath.Join(base, "update.json")
	integrationWriteInput(t, path, input)
	op, issues := app.NewUpdate(base, archive, path)
	integrationOK(t, issues)
	integrationOK(t, op.Analyze(integrationAt()))
	for index := range op.Snapshot().Plan.Fields {
		integrationOK(t, op.DecideField(op.Snapshot().Token, index, false))
	}
	integrationOK(t, op.PrepareRevision(op.Snapshot().Token))
	snapshot := op.Snapshot()
	if !snapshot.NoChanges || snapshot.Actions.ReviewSave || snapshot.After.Lesson().Document.Version != "1" {
		t.Fatalf("no-change guard failed: %+v", snapshot)
	}
	if !bytes.Equal(snapshot.After.OriginalBytes(), rawFiles["l001.json"]) {
		t.Fatal("unchanged update lost original bytes")
	}
	if _, issues = op.ReviewSave(snapshot.Token); len(issues) == 0 {
		t.Fatal("no-change review permitted")
	}
	_, issues = op.Save(integrationAt())
	if len(issues) == 0 {
		t.Fatal("no-change Save permitted")
	}
	integrationSameBytes(t, archive, before)
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Manifest only")
	integrationOK(t, issues)
	integrationOK(t, op.AcceptEdit(edit.Token))
	value := integrationSave(t, op, true)
	for name, raw := range rawFiles {
		file := integrationFile(t, value, name)
		if !bytes.Equal(file.OriginalBytes(), raw) || file.Lesson().Document.Version != "1" {
			t.Fatalf("manifest-only changed %s", name)
		}
	}
	if value.Manifest.Course.Title != "Manifest only" {
		t.Fatal("manifest edit missing")
	}
}
