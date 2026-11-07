package app

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestAnalyzeBaselineIsReadBeforeSourcesAndPreservesCause(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "invalid.zip")
	analyzeFixtureWrite(t, archive, []byte("invalid ZIP"))
	source := filepath.Join(root, "missing.json")
	op, issues := NewAdd(root, archive, source)
	analyzeFixtureOK(t, issues)
	_, ownerIssues := moduleio.Read(archive)
	got := op.Analyze(analyzeFixtureTime())
	if !bytes.Equal(analyzeFixtureJSON(t, ownerIssues), analyzeFixtureJSON(t, got)) {
		t.Fatalf("moduleio diagnostics replaced: %v != %v", got, ownerIssues)
	}
	issue := analyzeFixtureIssues(t, got, moduleio.CodeArchive)
	if !errors.Is(issue, zip.ErrFormat) || issue.Context.Path != archive {
		t.Fatalf("archive cause/context lost: %+v", issue)
	}
	if len(op.inputs) != 0 || op.current != nil || op.analyzed {
		t.Fatal("sources processed after invalid baseline")
	}
	if op.target == nil || op.target.info == nil {
		t.Fatal("baseline observation was not captured before Read")
	}
}

func TestAnalyzeBaselineKeepsInitialObservationAndImmutableRawFiles(t *testing.T) {
	root := t.TempDir()
	archive, old := analyzeFixtureArchive(t, root)
	original := analyzeFixtureBytes(t, archive)
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	source := analyzeFixtureSource(t, root, "new.json", 5, func(result *model.Result) { order := "3"; result.Document.Metadata.Order = &order })
	op, issues := NewAdd(root, archive, source)
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	if op.target == nil || !op.target.destination.Exists || !os.SameFile(info, op.target.info) || !info.ModTime().Equal(op.target.info.ModTime()) || info.Size() != op.target.info.Size() {
		t.Fatal("Read lost initial target observation")
	}
	if issue := op.checkDestination(); issue != nil {
		t.Fatal(issue)
	}
	if !bytes.Equal(original, analyzeFixtureBytes(t, archive)) {
		t.Fatal("Analyze wrote archive")
	}
	for i, file := range old.Files {
		if op.current.Files[i] != op.baseline.Files[i] {
			t.Fatal("old File was reconstructed")
		}
		if !bytes.Equal(file.OriginalBytes(), op.baseline.Files[i].OriginalBytes()) || !bytes.Equal(file.OriginalBytes(), op.current.Files[i].OriginalBytes()) {
			t.Fatal("old raw bytes changed")
		}
	}
	beforeCatalog := analyzeFixtureJSON(t, op.beforeCatalog)
	preview, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Другое название")
	analyzeFixtureOK(t, issues)
	if preview == nil {
		t.Fatal("missing editable proposal")
	}
	analyzeFixtureOK(t, op.AcceptEdit(preview.Token))
	if !bytes.Equal(beforeCatalog, analyzeFixtureJSON(t, op.beforeCatalog)) {
		t.Fatal("accepted edit mutated baseline catalog")
	}
	for i, file := range old.Files {
		if !bytes.Equal(file.OriginalBytes(), op.baseline.Files[i].OriginalBytes()) {
			t.Fatal("edit mutated baseline raw")
		}
	}
	replacement := filepath.Join(root, "replacement.zip")
	analyzeFixtureWrite(t, replacement, original)
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, archive); err != nil {
		t.Fatal(err)
	}
	_, issues = op.ReviewSave(op.Snapshot().Token)
	analyzeFixtureIssues(t, issues, CodeTargetChanged)
	if !os.SameFile(info, op.target.info) {
		t.Fatal("review legitimized replacement ZIP")
	}
}

func TestAnalyzeUpdateMatchesCaseWithoutChangingOldNameOrBytes(t *testing.T) {
	root := t.TempDir()
	archive, loaded := analyzeFixtureArchive(t, root)
	source := analyzeFixtureSource(t, root, "different-name.JSON", 2, func(result *model.Result) { id := "ba82"; result.Document.Metadata.DocumentID = &id })
	input := analyzeFixtureLoad(t, source)
	expected, issues := module.FindByID(loaded.Files, *input.Result.Document.Metadata.DocumentID, diagnostic.Context{Path: source, LessonID: "ba82"})
	analyzeFixtureOK(t, issues)
	op, issues := NewUpdate(root, archive, source)
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	if len(op.names) != 1 || op.names[0] != expected.FileName() || op.oldFile != op.baseline.Files[0] {
		t.Fatal("Update did not use owner match and old technical name")
	}
	if !bytes.Equal(op.oldFile.OriginalBytes(), expected.OriginalBytes()) || op.oldFile.Lesson().Document.ID != "BA82" {
		t.Fatal("old file rewritten")
	}
	if op.converted[0].FileName() != "l002.json" || op.converted[0].Lesson().Document.ID != "ba82" || len(op.converted[0].OriginalBytes()) != 0 {
		t.Fatal("invalid primary update candidate")
	}
	if op.current.Files[0] != op.oldFile || op.revised != op.oldFile {
		t.Fatal("analysis prematurely replaced old lesson")
	}
}

func TestAnalyzeRejectsExistingAddIDAndMissingUpdateIDBeforeConvert(t *testing.T) {
	for _, mode := range []Mode{Add, Update} {
		t.Run(string(mode), func(t *testing.T) {
			root := t.TempDir()
			archive, _ := analyzeFixtureArchive(t, root)
			id := "ba82"
			if mode == Update {
				id = "NOTFOUND"
			}
			source := analyzeFixtureSource(t, root, "source.json", 1, func(result *model.Result) { result.Document.Metadata.DocumentID = &id })
			var op *Operation
			var issues []*diagnostic.Error
			if mode == Add {
				op, issues = NewAdd(root, archive, source)
			} else {
				op, issues = NewUpdate(root, archive, source)
			}
			analyzeFixtureOK(t, issues)
			code := module.CodeExistingID
			if mode == Update {
				code = module.CodeMissingLesson
			}
			issue := analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), code)
			if issue.Context.Path != source || issue.Context.LessonID != id {
				t.Fatalf("owner context lost: %+v", issue)
			}
			if len(op.converted) != 0 || op.analyzed || op.current != nil {
				t.Fatal("failed ID matching reached conversion")
			}
		})
	}
}
