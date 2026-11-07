package app_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
)

func TestIntegrationCreateSortedSnapshotEditsAndZIP(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "Вход с пробелами")
	relatives := []string{"z/lesson.json", "a/deep/next/lesson.JSON", "A/lesson.json"}
	ids := []string{"LAST3", "MIDDLE2", "FIRST1"}
	inputs := map[string][]byte{}
	for index, relative := range relatives {
		path := filepath.Join(source, filepath.FromSlash(relative))
		integrationWriteInput(t, path, integrationInput(ids[index], []string{"body"}))
		inputs[path] = integrationRead(t, path)
	}
	op := integrationCreate(t, base, `"Вход с пробелами"`)
	snapshot := op.Snapshot()
	if !snapshot.ManifestReady || snapshot.NoChanges || len(snapshot.Module.Files) != 3 || len(snapshot.NewFileNames) != 0 {
		t.Fatalf("bad create snapshot: %+v", snapshot)
	}
	// Полный lowercase-путь a/deep/... предшествует a/lesson.json (§5.1).
	orderedIDs := []string{"MIDDLE2", "FIRST1", "LAST3"}
	orderedPaths := []string{relatives[1], relatives[2], relatives[0]}
	for index, id := range orderedIDs {
		name := fmt.Sprintf("l%03d.json", index+1)
		file := integrationFile(t, snapshot.Module, name)
		if file.Lesson().Document.ID != id {
			t.Fatalf("%s got %s", name, file.Lesson().Document.ID)
		}
		if file.Context().FileName != name || file.Context().Path != filepath.Join(source, filepath.FromSlash(orderedPaths[index])) {
			t.Fatalf("source/name not preserved through conversion: %+v", file.Context())
		}
		item := snapshot.Catalog.Items[index]
		if item.Entry.ID != id || item.Entry.FileName != name || item.RelativePath != orderedPaths[index] || item.Entry.Order == nil || *item.Entry.Order != fmt.Sprint(index+1) {
			t.Fatalf("sorted source/name/weight mapping differs: %+v", item)
		}
	}
	oldToken := snapshot.Token
	snapshot.Catalog.CourseTitle = "forged"
	snapshot.Catalog.Items[0].Entry.Title = "forged"
	*snapshot.Catalog.Items[0].Entry.Order = "99"
	snapshot.Module.Manifest.Course.Title = "forged"
	snapshot.Module.Files[0] = nil
	snapshot.Blockers = nil
	snapshot.Actions.Save = true
	if op.Snapshot().Catalog.CourseTitle != "Курс интеграции" || op.Snapshot().Module.Files[0] == nil {
		t.Fatal("snapshot mutated operation")
	}
	edit, issues := op.ProposeCourseTitle(oldToken, "Rejected course")
	integrationOK(t, issues)
	if edit == nil {
		t.Fatal("nil proposal")
	}
	if _, issues := op.ReviewSave(oldToken); len(issues) == 0 {
		t.Fatal("active edit permitted save review")
	}
	edit.Catalog.CourseTitle = "forged proposal"
	integrationOK(t, op.RejectEdit(edit.Token))
	if op.Snapshot().Token != oldToken || op.Snapshot().Catalog.CourseTitle != "Курс интеграции" {
		t.Fatal("reject mutated accepted state")
	}
	edit, issues = op.ProposeCourseTitle(oldToken, "Accepted course")
	integrationOK(t, issues)
	edit.Catalog.CourseTitle = "untrusted replacement"
	integrationOK(t, op.AcceptEdit(edit.Token))
	integrationCode(t, op.SetDestination(oldToken, base, "stale"), app.CodeStaleReview)
	if op.Snapshot().Catalog.CourseTitle != "Accepted course" {
		t.Fatal("accepted external proposal mutation")
	}
	edit, issues = op.ProposeUnitTitle(op.Snapshot().Token, "Unit")
	integrationOK(t, issues)
	integrationOK(t, op.AcceptEdit(edit.Token))
	integrationOK(t, op.SetDestination(op.Snapshot().Token, ".", "Модуль.ZIP"))
	target := filepath.Join(base, "Модуль.ZIP")
	integrationAbsent(t, target)
	review, issues := op.ReviewSave(op.Snapshot().Token)
	integrationOK(t, issues)
	if review.Destination.Path != target || review.Destination.Exists || !review.ManifestTimeWillChange {
		t.Fatalf("wrong destination: %+v", review)
	}
	review.Snapshot.Catalog.CourseTitle = "forged final"
	review.Snapshot.Module.Files = nil
	review.Destination.Path = filepath.Join(base, "wrong.zip")
	integrationOK(t, op.ConfirmSave(review.Token, false, false))
	_, issues = op.Save(integrationAt())
	integrationCode(t, issues, app.CodeUnconfirmed)
	integrationAbsent(t, target)
	value := integrationSave(t, op, false)
	if value.Manifest.Course.Title != "Accepted course" || value.Manifest.Course.Unit == nil || value.Manifest.Course.Unit.Title != "Unit" {
		t.Fatalf("incorrect manifest: %+v", value.Manifest)
	}
	if value.Manifest.CreatedAt != integrationAt().Add(24*time.Hour).UTC().Format("2006-01-02T15:04:05Z") {
		t.Fatal("wrong final timestamp")
	}
	for index, entry := range value.Manifest.Course.Unit.Lessons {
		if entry.ID != orderedIDs[index] || entry.FileName != fmt.Sprintf("l%03d.json", index+1) || entry.Order == nil || *entry.Order != fmt.Sprint(index+1) || integrationFile(t, value, entry.FileName).Lesson().Document.ID != entry.ID {
			t.Fatalf("ZIP lost sorted source/name/weight mapping: %+v", entry)
		}
	}
	for _, file := range value.Files {
		if file.Lesson().Document.CreatedAt != integrationAt().UTC().Format("2006-01-02T15:04:05Z") {
			t.Fatal("Save restamped lesson")
		}
	}
	for path, raw := range inputs {
		integrationSameBytes(t, path, raw)
	}
	installed := integrationRead(t, target)
	_, issues = op.Save(integrationAt())
	if len(issues) == 0 {
		t.Fatal("second Save succeeded")
	}
	if issues = op.Retry(integrationAt()); len(issues) == 0 {
		t.Fatal("installed operation retried")
	}
	op.Cancel()
	integrationSameBytes(t, target, installed)
	result := op.Snapshot().WriteResult
	if result == nil || !result.Installed {
		t.Fatal("installed result lost")
	}
}

func TestIntegrationCreateConflictsRequireAcceptedCompleteWeights(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "sources")
	for index, weight := range []string{"1", "01"} {
		input := integrationInput(fmt.Sprintf("DUP%d", index), []string{"body"})
		input.Document.Metadata.Order = &weight
		integrationWriteInput(t, filepath.Join(source, fmt.Sprintf("%d.json", index)), input)
	}
	op, issues := app.NewCreate(base, source, "Course", "")
	integrationOK(t, issues)
	issues = op.Analyze(integrationAt())
	integrationCode(t, issues, catalog.CodeWeightConflict)
	snapshot := op.Snapshot()
	if snapshot.Catalog == nil || snapshot.Module == nil || len(snapshot.Module.Files) != 2 || snapshot.ManifestReady || snapshot.Actions.ReviewSave || len(snapshot.Conflicts) != 1 {
		t.Fatalf("conflict not editable: %+v", snapshot)
	}
	conflict := snapshot.Conflicts[0]
	conflict.Items[0].Entry.Title = "forged"
	snapshot.Blockers[0].Reason = "forged"
	if op.Snapshot().Blockers[0].Reason == "forged" || op.Snapshot().Conflicts[0].Items[0].Entry.Title == "forged" {
		t.Fatal("mutable diagnostics/conflict leaked")
	}
	_, issues = op.ProposeLessonWeights(snapshot.Token, nil, map[string]string{"l001.json": "2"})
	integrationCode(t, issues, app.CodeIncompleteWeights)
	if op.Snapshot().Token != snapshot.Token {
		t.Fatal("invalid group changed token")
	}
	edit, issues := op.ProposeLessonWeights(snapshot.Token, nil, map[string]string{"l001.json": "02", "l002.json": "1"})
	integrationOK(t, issues)
	if edit == nil || len(edit.Blockers) != 0 {
		t.Fatal("complete valid group rejected")
	}
	integrationOK(t, op.AcceptEdit(edit.Token))
	if len(op.Snapshot().Blockers) != 0 || !op.Snapshot().ManifestReady {
		t.Fatal("conflict not cleared")
	}
	integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "weights"))
	value := integrationSave(t, op, false)
	entries := *value.Manifest.Course.Lessons
	if entries[0].FileName != "l002.json" || *entries[1].Order != "02" {
		t.Fatalf("weight sorting/spelling wrong: %+v", entries)
	}
}

func TestIntegrationCreateRejectAndCancelNeverWrite(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	integrationWriteInput(t, filepath.Join(source, "one.json"), integrationInput("ONLY1", []string{"body"}))
	op := integrationCreate(t, base, source)
	integrationOK(t, op.SetDestination(op.Snapshot().Token, base, "cancelled"))
	review, issues := op.ReviewSave(op.Snapshot().Token)
	integrationOK(t, issues)
	integrationOK(t, op.ConfirmSave(review.Token, true, false))
	op.Cancel()
	op.Cancel()
	_, issues = op.Save(integrationAt())
	if len(issues) == 0 {
		t.Fatal("cancelled Save succeeded")
	}
	integrationAbsent(t, filepath.Join(base, "cancelled.zip"))
	if op.Snapshot().Actions.Save || op.Snapshot().Actions.Retry {
		t.Fatal("cancel left operation usable")
	}
}
