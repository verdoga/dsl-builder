package app

import (
	"bytes"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
)

func catalogEditAdd(t *testing.T) (*Operation, string) {
	t.Helper()
	archive := catalogEditArchive(t, []string{"", ""})
	dir := t.TempDir()
	source := catalogEditSource(t, dir, "new.json", "CE99", "Новый урок", nil, catalogEditString("3"))
	op, issues := NewAdd(dir, archive, source)
	catalogEditOK(t, issues)
	catalogEditOK(t, op.Analyze(catalogEditTime()))
	return op, archive
}

func TestCatalogEditSectionAndMoveNormalizeThroughOwnerWithoutFlattening(t *testing.T) {
	op, archive := catalogEditAdd(t)
	bytesBefore, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	before := op.Snapshot()
	raw := " \tПрактика  A\r\n "
	preview, issues := op.ProposeSection(before.Token, raw)
	catalogEditCode(t, issues, catalog.CodeUnassigned)
	if preview == nil || preview.Catalog.Sections[0].Title != "Практика A" || !preview.Catalog.Sectioned {
		t.Fatal("section candidate was not normalized or was discarded due to blockers")
	}
	if op.Snapshot().Catalog.Sectioned {
		t.Fatal("section was accepted before confirmation")
	}
	preview.Blockers[0].Reason = "external"
	if len(preview.Blockers[0].Related) != 0 {
		preview.Blockers[0].Related[0].FileName = "external"
	}
	op.AcceptEdit(preview.Token)
	snapshot := op.Snapshot()
	catalogEditCode(t, snapshot.Blockers, catalog.CodeUnassigned)
	for _, issue := range snapshot.Blockers {
		if issue.Reason == "external" {
			t.Fatal("blocker leaked from preview into accepted state")
		}
	}
	raw = "\tПрактика   A\n"
	for _, name := range []string{"l001.json", "l002.json", "l003.json"} {
		preview, issues = op.ProposeMove(op.Snapshot().Token, name, &raw)
		if preview == nil {
			t.Fatalf("move to normalized section failed: %+v", issues)
		}
		if *catalogEditItem(t, preview.Catalog, name).Section != "Практика A" {
			t.Fatal("move did not normalize its section")
		}
		op.AcceptEdit(preview.Token)
	}
	snapshot = op.Snapshot()
	catalogEditOK(t, snapshot.Blockers)
	if !snapshot.ManifestReady {
		t.Fatal("completed distribution did not prepare the manifest")
	}
	preview, issues = op.ProposeFlatPlacement(snapshot.Token)
	if preview != nil || len(issues) == 0 || op.Snapshot().Token != snapshot.Token {
		t.Fatal("flat placement silently removed assignments")
	}
	for _, name := range []string{"l001.json", "l002.json", "l003.json"} {
		preview, issues = op.ProposeMove(op.Snapshot().Token, name, nil)
		if preview == nil {
			t.Fatalf("explicit move out of section failed: %+v", issues)
		}
		op.AcceptEdit(preview.Token)
	}
	preview, issues = op.ProposeFlatPlacement(op.Snapshot().Token)
	catalogEditOK(t, issues)
	if preview == nil || preview.Catalog.Sectioned || len(preview.Catalog.Sections) != 0 {
		t.Fatal("explicitly unassigned composition did not flatten")
	}
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	for _, file := range op.Snapshot().Module.Files[:2] {
		if len(file.OriginalBytes()) == 0 || file.Lesson().Document.Version != "1" {
			t.Fatal("catalog edits replaced an unchanged baseline lesson")
		}
	}
	bytesAfter, err := os.ReadFile(archive)
	if err != nil || !bytes.Equal(bytesBefore, bytesAfter) {
		t.Fatal("catalog editing wrote the archive")
	}
}

func TestCatalogEditModeAndInvalidSectionInputPreserveSnapshot(t *testing.T) {
	create := catalogEditCreate(t, []string{"", ""})
	token := create.Snapshot().Token
	if preview, issues := create.ProposeSection(token, "Section"); preview != nil || len(issues) == 0 {
		t.Fatal("Create permits introduction of sections")
	}
	if preview, issues := create.ProposeMove(token, "l001.json", catalogEditString("Section")); preview != nil || len(issues) == 0 {
		t.Fatal("Create permits a manual section move")
	}
	op, _ := catalogEditAdd(t)
	before := op.Snapshot()
	if preview, issues := op.ProposeSection(before.Token, " \t\n"); preview != nil || len(issues) == 0 {
		t.Fatal("blank normalized section became a candidate")
	}
	if preview, issues := op.ProposeMove(before.Token, "l001.json", catalogEditString(" \r ")); preview != nil || len(issues) == 0 {
		t.Fatal("blank move target became a candidate")
	}
	if op.Snapshot().Token != before.Token || catalogEditJSON(t, op.Snapshot().Catalog) != catalogEditJSON(t, before.Catalog) {
		t.Fatal("invalid section input changed accepted catalog")
	}
	preview, issues := op.ProposeSection(before.Token, "Alpha")
	if preview == nil {
		t.Fatalf("valid section failed after invalid input: %+v", issues)
	}
	catalogEditOK(t, op.RejectEdit(preview.Token))
}

func TestCatalogEditSingleWeightRepairsMissingAddedWeight(t *testing.T) {
	archive := catalogEditArchive(t, []string{"", ""})
	dir := t.TempDir()
	source := catalogEditSource(t, dir, "missing.json", "CE99", "Без веса", nil, nil)
	op, issues := NewAdd(dir, archive, source)
	catalogEditOK(t, issues)
	op.Analyze(catalogEditTime())
	before := op.Snapshot()
	catalogEditCode(t, before.Blockers, catalog.CodeMissingWeight)
	if before.Catalog == nil || len(before.Module.Files) != 3 {
		t.Fatal("editable missing-weight plan lost its complete composition")
	}
	preview, issues := op.ProposeLessonWeight(before.Token, "l003.json", "0004")
	catalogEditOK(t, issues)
	if preview == nil || len(preview.Blockers) != 0 || catalogEditItem(t, op.Snapshot().Catalog, "l003.json").Entry.Order != nil {
		t.Fatal("missing weight was not staged separately")
	}
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	after := op.Snapshot()
	catalogEditOK(t, after.Blockers)
	if !after.ManifestReady || *catalogEditItem(t, after.Catalog, "l003.json").Entry.Order != "0004" {
		t.Fatal("accepted weight did not repair the complete plan")
	}
}
