package app

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/moduleio"
)

func TestSaveConfirmedCreatesReadableZIPOnce(t *testing.T) {
	op, source, target := savePrefixCreate(t)
	originalSource := savePrefixRead(t, source)
	before := op.Snapshot()
	expectedLesson := before.Module.Files[0].Lesson()
	preview := savePrefixReview(t, op)
	if preview.Destination.Path != target || preview.Destination.Exists || !preview.ManifestTimeWillChange {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	savePrefixAbsent(t, target)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
	savePrefixAbsent(t, target)
	at := time.Date(2026, 12, 3, 14, 15, 16, 999999999, time.FixedZone("save", -5*3600))
	result, issues := op.Save(at)
	savePrefixOK(t, issues)
	if !result.Installed || result.TargetPath != target || result.TemporaryPath != "" || result.OriginalPath != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	actual, issues := moduleio.Read(target)
	savePrefixOK(t, issues)
	if actual.Manifest.CreatedAt != "2026-12-03T19:15:16Z" || len(actual.Files) != 1 {
		t.Fatalf("incorrect saved manifest: %+v", actual.Manifest)
	}
	got := actual.Files[0].Lesson()
	if got.Document.CreatedAt != expectedLesson.Document.CreatedAt || got.Document.Version != expectedLesson.Document.Version || got.Document.LineCount != expectedLesson.Document.LineCount {
		t.Fatal("Save changed lesson date, version or lineCount")
	}
	saved := savePrefixRead(t, target)
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	_, issues = op.Save(at.Add(time.Hour))
	if len(issues) == 0 {
		t.Fatal("second Save accepted consumed consent")
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, savePrefixRead(t, target)) || !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) {
		t.Fatal("second Save attempted another installation")
	}
	if !bytes.Equal(originalSource, savePrefixRead(t, source)) {
		t.Fatal("Save modified source")
	}
	snapshot := op.Snapshot()
	if snapshot.WriteResult == nil || *snapshot.WriteResult != result || snapshot.Actions.Save || snapshot.Actions.Retry || snapshot.Actions.ReviewSave {
		t.Fatalf("installed operation remains writable: %+v", snapshot.Actions)
	}
	if issues = op.Retry(at); len(issues) == 0 {
		t.Fatal("Retry allowed after install")
	}
}

func TestSaveManifestOnlyPreservesOriginalLessonBytes(t *testing.T) {
	op, source, archive := savePrefixUpdate(t, "", nil)
	sourceBytes := savePrefixRead(t, source)
	old, issues := moduleio.Read(archive)
	savePrefixOK(t, issues)
	savePrefixDecide(t, op, false)
	edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Новое название")
	savePrefixOK(t, issues)
	if edit == nil {
		t.Fatal("nil edit")
	}
	savePrefixOK(t, op.AcceptEdit(edit.Token))
	preview := savePrefixReview(t, op)
	if !preview.Destination.Exists || preview.Destination.Path != archive {
		t.Fatal("original path was not shown as overwrite")
	}
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, true))
	result, issues := op.Save(savePrefixTime().Add(48 * time.Hour))
	savePrefixOK(t, issues)
	if !result.Installed {
		t.Fatal("not installed")
	}
	actual, issues := moduleio.Read(archive)
	savePrefixOK(t, issues)
	if actual.Manifest.Course.Title != "Новое название" || actual.Manifest.CreatedAt != "2026-11-06T07:11:12Z" {
		t.Fatalf("manifest not updated: %+v", actual.Manifest)
	}
	if !bytes.Equal(old.Files[0].OriginalBytes(), actual.Files[0].OriginalBytes()) {
		t.Fatal("manifest-only Save reserialized lesson bytes")
	}
	if !bytes.Equal(sourceBytes, savePrefixRead(t, source)) {
		t.Fatal("source changed")
	}
}

func TestSaveRejectingEveryChangeDoesNotWrite(t *testing.T) {
	op, _, archive := savePrefixUpdate(t, "Другое название", nil)
	before := savePrefixRead(t, archive)
	savePrefixDecide(t, op, false)
	if !op.Snapshot().NoChanges {
		t.Fatal("rejected title created artificial changes")
	}
	preview, issues := op.ReviewSave(op.Snapshot().Token)
	if preview != nil || len(issues) == 0 {
		t.Fatal("unchanged operation obtained SavePreview")
	}
	_, issues = op.Save(savePrefixTime().Add(24 * time.Hour))
	if len(issues) == 0 {
		t.Fatal("unchanged operation wrote without consent")
	}
	if !bytes.Equal(before, savePrefixRead(t, archive)) {
		t.Fatal("unchanged ZIP was rewritten")
	}
}
