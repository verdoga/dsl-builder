package console

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/preview"
)

func TestConsolePreviewUpdateWaitsForRevisionAndDoesNotInventIDs(t *testing.T) {
	for _, mode := range []app.Mode{app.Create, app.Add, app.Update} {
		t.Run(string(mode), func(t *testing.T) {
			// Недоступные обязательные параметры выявят преждевременный вызов preview.
			snapshot := app.Snapshot{Mode: mode, RevisionReady: false}
			var output bytes.Buffer
			if err := previewTestDialog(&output, "").showUpdate(snapshot); err != nil {
				t.Fatalf("unprepared update called mandatory preview: %v", err)
			}
			if mode == app.Update && output.Len() == 0 {
				t.Fatal("update must explain that final versions and IDs are not prepared")
			}
			if mode != app.Update && output.Len() != 0 {
				t.Fatal("update block appeared for another mode")
			}
			previewTestAbsent(t, output.String(), "Версия урока:", "Замены нод не запланированы", "Будет заменена нода:", "-v01", "-v02")
		})
	}
	op := previewTestUpdate(t)
	var output bytes.Buffer
	if err := previewTestDialog(&output, "").showUpdate(op.Snapshot()); err != nil {
		t.Fatal(err)
	}
	previewTestAbsent(t, output.String(), "ba82-n002", "Версия урока:", "Prepared", "FRESH")
}

func TestConsolePreviewUpdateUsesActualReadyFilesAndAcceptedSubset(t *testing.T) {
	op := previewTestReadyUpdate(t)
	snapshot := op.Snapshot()
	if len(snapshot.AcceptedFields) != 1 || snapshot.After.Lesson().Document.Title != "FRESH" || *snapshot.After.Lesson().Document.Subtitle != "Subtitle" {
		t.Fatal("expected title accepted and subtitle rejected in owner result")
	}
	var output bytes.Buffer
	if err := previewTestDialog(&output, "").showUpdate(snapshot); err != nil {
		t.Fatal(err)
	}
	heading, err := preview.UpdateHeading(snapshot.ArchivePath, snapshot.Before, snapshot.After)
	previewTestContains(t, output.String(), previewTestBlock(t, heading, err))
	fields, err := preview.AcceptedFields(snapshot.Before, snapshot.After, snapshot.AcceptedFields)
	previewTestContains(t, output.String(), previewTestBlock(t, fields, err))
	replacements, err := preview.Replacements(snapshot.Before, snapshot.After, snapshot.Plan)
	previewTestContains(t, output.String(), previewTestBlock(t, replacements, err), snapshot.ArchivePath, "BA82", "FRESH", "Subtitle", "Версия урока: 1 -> 2", "ba82-p001", "ba82-n002", "ba82-n002-v02")
	previewTestAbsent(t, output.String(), "Prepared", "INSTRUCTION TEXT Z", "EXAMPLE TEXT", "document.subtitle")
}

func TestConsolePreviewUpdatePreservesPreparedReplacementSelectors(t *testing.T) {
	snapshot := previewTestReadyUpdate(t).Snapshot()
	// Проекция подготовленного DTO: эти планы не объявляются результатом app.
	for _, test := range []struct {
		name string
		plan *compare.Plan
	}{
		{"page", &compare.Plan{Pages: []compare.PageChange{{Index: 0, ReplaceNodes: true, Reasons: []compare.Reason{{Code: compare.NodeCountChanged, Description: "Причина страницы один", OldPointer: "/document/pages/0/nodes", NewPointer: "/document/pages/0/nodes"}, {Code: compare.NodeNestingChanged, Description: "Причина страницы два"}}}}}},
		{"lesson", &compare.Plan{ReplaceLesson: true, Reasons: []compare.Reason{{Code: compare.PageCountChanged, Description: "Причина урока один", OldPointer: "/document/pages", NewPointer: "/document/pages"}, {Code: compare.PageCorrespondenceAmbiguous, Description: "Причина урока два"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot.Plan = test.plan
			var output bytes.Buffer
			if err := previewTestDialog(&output, "").showUpdate(snapshot); err != nil {
				t.Fatal(err)
			}
			block, err := preview.Replacements(snapshot.Before, snapshot.After, test.plan)
			previewTestContains(t, output.String(), previewTestBlock(t, block, err), "ba82-n001", "ba82-n007", "ba82-n002-v02")
			for _, reason := range test.plan.Reasons {
				previewTestContains(t, output.String(), reason.Description)
			}
			for _, page := range test.plan.Pages {
				for _, reason := range page.Reasons {
					previewTestContains(t, output.String(), reason.Description)
				}
			}
		})
	}
}

func TestConsolePreviewManifestShowsEveryProvidedFieldInOrder(t *testing.T) {
	changes := []catalog.Change{
		{Field: "course.title", Before: json.RawMessage(`"Старый курс"`), After: json.RawMessage(`"Новый курс"`)},
		{Field: "course.unit", Before: json.RawMessage(`null`), After: json.RawMessage(`"Юнит"`)},
		{Field: "sections", Before: json.RawMessage(`["Б","А"]`), After: json.RawMessage(`["А","Б"]`)},
		{Field: "lessons", Section: previewTestString("А"), Before: json.RawMessage(`["l002.json","l001.json"]`), After: json.RawMessage(`["l001.json","l002.json"]`)},
		{Field: "section", FileName: "l001.json", Before: json.RawMessage(`null`), After: json.RawMessage(`"А"`)},
		{Field: "id", FileName: "l001.json", Before: json.RawMessage(`"BA82"`), After: json.RawMessage(`"BA83"`)},
		{Field: "fileName", FileName: "l001.json", Before: json.RawMessage(`"l001.json"`), After: json.RawMessage(`"l1000.json"`)},
		{Field: "order", FileName: "l001.json", Before: json.RawMessage(`"000123456789012345678901234567890"`), After: json.RawMessage(`"0002"`)},
		{Field: "title", FileName: "l001.json", Before: json.RawMessage(`"Название"`), After: json.RawMessage(`" Другое "`)},
		{Field: "subtitle", FileName: "l001.json", Before: json.RawMessage(`null`), After: json.RawMessage(`""`)},
		{Field: "lineCount", FileName: "l001.json", Before: json.RawMessage(`123456789012345678901234567890`), After: json.RawMessage(`23`)},
		{Field: "createdAt", FileName: "l001.json", Before: json.RawMessage(`"2026-11-04T00:00:00Z"`), After: json.RawMessage(`"2026-11-05T00:00:00Z"`)},
		{Field: "version", FileName: "l001.json", Before: json.RawMessage(`1`), After: json.RawMessage(`123456789012345678901234567890`)},
		{Field: "entry", FileName: "l002.json", After: json.RawMessage(`{"id":"BA83","fileName":"l002.json","order":"002","title":"Вторая","subtitle":null,"lineCount":44,"createdAt":"2026-11-04T00:00:00Z","version":1}`)},
	}
	var output bytes.Buffer
	if err := previewTestDialog(&output, "").showManifest(app.Snapshot{ManifestChanges: changes}); err != nil {
		t.Fatal(err)
	}
	block, err := preview.ManifestChanges(changes)
	previewTestContains(t, output.String(), previewTestBlock(t, block, err), "000123456789012345678901234567890", "123456789012345678901234567890", `" Другое "`, `"Вторая"`, `"lineCount":44`, `"subtitle":null`)
	previewTestAbsent(t, output.String(), "Дата манифеста будет обновлена")
}
