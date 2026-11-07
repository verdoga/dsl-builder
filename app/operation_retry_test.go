package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/parsed"
)

func TestOperationRetryRebuildsCreateFromOriginalParameters(t *testing.T) {
	base := t.TempDir()
	sources := filepath.Join(base, "Источники")
	resources := filepath.Join(sources, "res")
	if err := os.MkdirAll(resources, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resources, "unused.bin"), []byte{1, 2, 3}, 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sources, "lesson.json")
	operationWriteInput(t, source, "RETRY1", "Первое название", nil, []string{"res"})
	peer := filepath.Join(sources, "peer.json")
	peerRaw := operationWriteInput(t, peer, "RETRY2", "Второй урок", nil, nil)
	op, issues := NewCreate(base, `"Источники"`, "Первоначальный курс", "Первоначальный юнит")
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 1, 2, 3, 0, time.UTC)
	operationNoIssues(t, op.Analyze(at))
	oldTree, oldUsage := op.tree, op.usage
	if op.Snapshot().Resources.Total != 1 {
		t.Fatal("fixture did not populate resource registry")
	}
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Изменённый курс")
	operationNoIssues(t, issues)
	operationNoIssues(t, op.AcceptEdit(edit.Token))
	operationNoIssues(t, op.SetDestination(op.Snapshot().Token, base, "discarded"))
	oldToken := op.Snapshot().Token
	save, issues := op.ReviewSave(oldToken)
	operationNoIssues(t, issues)
	operationNoIssues(t, op.ConfirmSave(save.Token, true, false))
	if !op.Snapshot().Actions.Save {
		t.Fatal("fixture did not obtain real final consent")
	}
	// Принятая правка создаёт достижимую блокировку весов по ТЗ 16.5.
	conflict, issues := op.ProposeLessonWeight(op.Snapshot().Token, "l002.json", "1")
	operationHasCode(t, issues, catalog.CodeWeightConflict)
	if conflict == nil {
		t.Fatal("weight conflict must remain editable")
	}
	operationHasCode(t, op.AcceptEdit(conflict.Token), catalog.CodeWeightConflict)
	oldToken = op.Snapshot().Token
	pending, issues := op.ProposeUnitTitle(oldToken, "Непринятый юнит")
	operationHasCode(t, issues, catalog.CodeWeightConflict)
	if pending == nil || !op.Snapshot().Actions.Retry {
		t.Fatal("fixture did not retain a pending edit over a blocked catalog")
	}
	raw := operationWriteInput(t, source, "RETRY1", "Прочитано повторно", nil, nil)
	later := at.Add(25 * time.Hour)
	operationNoIssues(t, op.Retry(later))
	if !op.at.Equal(later) || op.tree == nil || op.tree == oldTree || op.usage == nil || op.usage == oldUsage {
		t.Fatal("Retry did not start a fresh analysis with the supplied time")
	}
	if oldUsage.Snapshot().Total != 1 || op.Snapshot().Resources.Total != 0 {
		t.Fatal("Retry reused or mutated the previous registry")
	}
	snapshot := op.Snapshot()
	if !op.analyzed || snapshot.Catalog == nil || snapshot.Module == nil || len(snapshot.Module.Files) != 2 {
		t.Fatal("Retry did not publish the complete new analysis")
	}
	if snapshot.Catalog.CourseTitle != "Первоначальный курс" || snapshot.Catalog.UnitTitle == nil || *snapshot.Catalog.UnitTitle != "Первоначальный юнит" {
		t.Fatal("Retry used accepted edits instead of constructor titles")
	}
	document := snapshot.Module.Files[0].Lesson().Document
	if document.Title != "Прочитано повторно" || document.CreatedAt != later.UTC().Format("2006-01-02T15:04:05Z") {
		t.Fatalf("Retry did not reread/reconvert source: %+v", document)
	}
	if snapshot.Target != nil || snapshot.WriteResult != nil || op.saving != nil || op.edit != nil || snapshot.Actions.Save || !snapshot.Actions.ChooseDestination {
		t.Fatal("Retry retained destination, edit, result or final consent")
	}
	if issues := op.SetDestination(oldToken, base, "stale"); len(issues) == 0 {
		t.Fatal("Retry kept the previous snapshot token valid")
	}
	if issues := op.ConfirmSave(save.Token, true, false); len(issues) == 0 {
		t.Fatal("Retry kept final consent token valid")
	}
	if issues := op.AcceptEdit(pending.Token); len(issues) == 0 {
		t.Fatal("Retry kept pending catalog edit token valid")
	}
	if !bytes.Equal(operationRead(t, source), raw) {
		t.Fatal("Retry modified source JSON")
	}
	if !bytes.Equal(operationRead(t, peer), peerRaw) {
		t.Fatal("Retry modified the second source JSON")
	}
	if _, err := os.Stat(filepath.Join(base, "discarded.zip")); !os.IsNotExist(err) {
		t.Fatalf("Retry wrote the old destination: %v", err)
	}
}

func TestOperationRetryClearsFailedAnalysisAndCanFailAgain(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "lesson.json")
	if err := os.WriteFile(source, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	op, issues := NewCreate(base, base, "Курс", "")
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if issues := op.Analyze(at); len(issues) == 0 {
		t.Fatal("invalid JSON unexpectedly analyzed")
	}
	failedTree, failedUsage := op.tree, op.usage
	if op.analyzed || op.current != nil {
		t.Fatal("failed analysis published a module")
	}
	operationWriteInput(t, source, "REPAIRED1", "Исправлен", nil, nil)
	peer := filepath.Join(base, "peer.json")
	operationWriteInput(t, peer, "REPAIRED2", "Второй урок", nil, nil)
	operationNoIssues(t, op.Retry(at.Add(time.Hour)))
	if !op.analyzed || op.current == nil || len(op.issues) != 0 || op.tree == failedTree || op.usage == failedUsage {
		t.Fatal("Retry did not clear failed analysis")
	}
	conflict, issues := op.ProposeLessonWeight(op.Snapshot().Token, "l001.json", "2")
	operationHasCode(t, issues, catalog.CodeWeightConflict)
	if conflict == nil {
		t.Fatal("weight conflict must remain editable")
	}
	operationHasCode(t, op.AcceptEdit(conflict.Token), catalog.CodeWeightConflict)
	if !op.Snapshot().Actions.Retry || len(op.Snapshot().Conflicts) != 1 {
		t.Fatal("fixture did not reach a blocked retry prerequisite")
	}
	oldToken := op.Snapshot().Token
	if err := os.WriteFile(source, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	operationHasCode(t, op.Retry(at.Add(2*time.Hour)), parsed.CodeJSON)
	if op.analyzed || op.current != nil || op.Snapshot().Actions.Save {
		t.Fatal("failed Retry left the previous successful candidate available")
	}
	if preview, issues := op.ProposeCourseTitle(oldToken, "Устаревшая правка"); preview != nil || len(issues) == 0 {
		t.Fatal("token from analysis before failed Retry remained usable")
	}
	operationWriteInput(t, source, "REPAIRED1", "Снова исправлен", nil, nil)
	operationNoIssues(t, op.Retry(at.Add(3*time.Hour)))
}

func TestOperationRetryAllowedAfterAnalyzedWeightConflict(t *testing.T) {
	base := t.TempDir()
	one, zeroOne, two := "1", "01", "2"
	operationWriteInput(t, filepath.Join(base, "a.json"), "WEIGHT1", "Первый", &one, nil)
	second := filepath.Join(base, "b.json")
	operationWriteInput(t, second, "WEIGHT2", "Второй", &zeroOne, nil)
	op, issues := NewCreate(base, base, "Курс", "")
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	op.Analyze(at)
	if !op.analyzed || op.placement == nil || op.current == nil || len(op.Snapshot().Blockers) == 0 {
		t.Fatal("fixture must finish analysis with an editable catalog conflict")
	}
	oldTree, oldUsage := op.tree, op.usage
	operationWriteInput(t, second, "WEIGHT2", "Второй", &two, nil)
	operationNoIssues(t, op.Retry(at.Add(time.Hour)))
	if !op.analyzed || len(op.Snapshot().Blockers) != 0 || len(op.Snapshot().Conflicts) != 0 || op.tree == oldTree || op.usage == oldUsage {
		t.Fatal("Retry did not resolve the completed analysis's source conflict")
	}
}

func TestOperationRetryUpdateResetsFieldAndPlacementDecisions(t *testing.T) {
	base := t.TempDir()
	archive := operationWriteArchive(t, base, "UPDATE1")
	archiveBytes := operationRead(t, archive)
	source := filepath.Join(base, "update.json")
	weight := "2"
	operationWriteInput(t, source, "UPDATE1", "Новое название", &weight, nil)
	op, issues := NewUpdate(base, `"baseline.zip"`, `"update.json"`)
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	operationNoIssues(t, op.Analyze(at))
	if len(op.fields) != 1 || len(op.placementDecisions) != 1 {
		t.Fatal("fixture did not produce one title and one weight proposal")
	}
	operationNoIssues(t, op.DecideField(op.Snapshot().Token, 0, true))
	operationNoIssues(t, op.DecidePlacement(op.Snapshot().Token, 0, true))
	operationNoIssues(t, op.PrepareRevision(op.Snapshot().Token))
	section, issues := op.ProposeSection(op.Snapshot().Token, "Новая секция")
	operationHasCode(t, issues, catalog.CodeUnassigned)
	if section == nil {
		t.Fatal("incomplete section assignment must remain editable")
	}
	operationHasCode(t, op.AcceptEdit(section.Token), catalog.CodeUnassigned)
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Непринятая правка")
	operationHasCode(t, issues, catalog.CodeUnassigned)
	if edit == nil || !op.Snapshot().Actions.Retry || !op.revisionReady {
		t.Fatal("fixture did not preserve revision and pending edit over a blocked catalog")
	}
	oldToken, oldTree, oldUsage := op.Snapshot().Token, op.tree, op.usage
	operationNoIssues(t, op.Retry(at.Add(time.Hour)))
	if len(op.fields) != 1 || op.fields[0] != "" || len(op.placementDecisions) != 1 || op.placementDecisions[0] != "" {
		t.Fatal("Retry reused previous field or placement decisions")
	}
	snapshot := op.Snapshot()
	if snapshot.RevisionReady || snapshot.FieldAnswered[0] || snapshot.PlacementAnswered[0] || snapshot.Actions.Save || op.edit != nil || op.saving != nil {
		t.Fatal("Retry retained revised readiness, proposal or consent")
	}
	if snapshot.Catalog.CourseTitle != "Исходный курс" || snapshot.Catalog.Items[0].Entry.Order == nil || *snapshot.Catalog.Items[0].Entry.Order != "1" {
		t.Fatal("Retry did not reload original placement")
	}
	if snapshot.Before == nil || len(snapshot.Before.OriginalBytes()) == 0 || op.tree == oldTree || op.usage == oldUsage {
		t.Fatal("Retry did not reload real archive and fresh analysis dependencies")
	}
	if issues := op.AcceptEdit(edit.Token); len(issues) == 0 {
		t.Fatal("Retry kept pending edit token valid")
	}
	if issues := op.DecideField(oldToken, 0, false); len(issues) == 0 {
		t.Fatal("Retry kept earlier accepted-state token valid")
	}
	if !bytes.Equal(operationRead(t, archive), archiveBytes) {
		t.Fatal("Retry or analysis changed original ZIP bytes")
	}
}
