package app_test

import (
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestIntegrationUnmatchedNullPageTitleStillRequiresSeparateDecision(t *testing.T) {
	base := t.TempDir()
	archive, _ := integrationArchive(t, base)
	input := integrationInput("OLD1", []string{"old body", "unchanged"}, []string{"new content"})
	input.Lines[4] = model.Line{Number: 5, LineType: model.LineTypeTag, Raw: "@newpage", LineEnding: model.LineEndingLF, Elements: []model.Element{
		{ElementType: model.ElementTypeTag, Raw: "@newpage", Value: integrationText("newpage"), Start: 1, End: 9, ErrorIDs: []string{}},
	}}
	path := filepath.Join(base, "newpage.json")
	integrationWriteInput(t, path, input)
	op, issues := app.NewUpdate(base, archive, path)
	integrationOK(t, issues)
	integrationOK(t, op.Analyze(integrationAt()))
	snapshot := op.Snapshot()
	if !snapshot.Plan.ReplaceLesson {
		t.Fatal("added page did not replace lesson")
	}
	found := false
	for index, field := range snapshot.Plan.Fields {
		if field.Field == compare.PageTitle && field.OldPage == -1 && field.NewPage == 1 && field.Before == nil && field.After == nil {
			found = true
			integrationCode(t, op.PrepareRevision(op.Snapshot().Token), app.CodeDecision)
		}
		integrationOK(t, op.DecideField(op.Snapshot().Token, index, false))
	}
	if !found {
		t.Fatal("unmatched null page-title proposal was suppressed")
	}
	integrationOK(t, op.PrepareRevision(op.Snapshot().Token))
	if op.Snapshot().After.Lesson().Document.Pages[1].Title != nil {
		t.Fatal("rejected unknown page title borrowed old value")
	}
	value := integrationSave(t, op, true)
	after := integrationFile(t, value, "l001.json").Lesson()
	if after.Document.Version != "2" || after.Document.Pages[1].Title != nil {
		t.Fatal("null-title decision not persisted")
	}
}
