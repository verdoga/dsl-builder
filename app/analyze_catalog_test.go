package app

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestAnalyzeCreateConflictsRemainEditable(t *testing.T) {
	root := t.TempDir()
	for i, golden := range []int{2, 4} {
		analyzeFixtureSource(t, root, []string{"a.json", "z.json"}[i], golden, func(result *model.Result) {
			weight := []string{"01", "1"}[i]
			result.Document.Metadata.Order = &weight
		})
	}
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), catalog.CodeWeightConflict)
	snapshot := op.Snapshot()
	if !op.analyzed || op.placement == nil || op.current == nil || len(snapshot.Module.Files) != 2 || len(snapshot.Catalog.Items) != 2 || !snapshot.Actions.EditCatalog || snapshot.ManifestReady || snapshot.Actions.ReviewSave {
		t.Fatalf("conflicting catalog discarded: %+v", snapshot)
	}
	if len(snapshot.Conflicts) != 1 || len(snapshot.Conflicts[0].Items) != 2 {
		t.Fatal("conflict did not include all files")
	}
	edit, issues := op.ProposeLessonWeight(snapshot.Token, "l002.json", "2")
	analyzeFixtureOK(t, issues)
	if edit == nil {
		t.Fatal("repair proposal unavailable")
	}
	analyzeFixtureOK(t, op.AcceptEdit(edit.Token))
	if !op.Snapshot().ManifestReady || len(op.Snapshot().Blockers) != 0 {
		t.Fatal("valid repair not published")
	}
}

func TestAnalyzeCreateNilCatalogDoesNotCompleteAnalysis(t *testing.T) {
	root := t.TempDir()
	analyzeFixtureSource(t, root, "source.json", 2, nil)
	op, issues := NewCreate(root, root, " \t", "")
	analyzeFixtureOK(t, issues)
	if issues := op.Analyze(analyzeFixtureTime()); len(issues) == 0 {
		t.Fatal("blank course accepted")
	}
	if op.analyzed || op.placement != nil || op.Snapshot().Actions.EditCatalog || op.Snapshot().Actions.ReviewSave {
		t.Fatal("nil catalog presented as editable completion")
	}
}

func TestAnalyzeAddWorkingBlockersKeepAllFiles(t *testing.T) {
	for _, scenario := range []string{"missing-weight", "conflicting-weight", "introduce-section"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			archive, original := analyzeFixtureArchive(t, root)
			source := analyzeFixtureSource(t, root, "new/source.json", 5, func(result *model.Result) {
				if scenario == "conflicting-weight" {
					weight := "01"
					result.Document.Metadata.Order = &weight
				}
				if scenario == "introduce-section" {
					section, weight := "Новая секция", "3"
					result.Document.Metadata.Section, result.Document.Metadata.Order = &section, &weight
				}
			})
			op, issues := NewAdd(root, archive, source)
			analyzeFixtureOK(t, issues)
			code := catalog.CodeMissingWeight
			if scenario == "conflicting-weight" {
				code = catalog.CodeWeightConflict
			}
			if scenario == "introduce-section" {
				code = catalog.CodeUnassigned
			}
			analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), code)
			snapshot := op.Snapshot()
			if !op.analyzed || snapshot.Catalog == nil || snapshot.Module == nil || len(snapshot.Module.Files) != 3 || snapshot.ManifestReady || !snapshot.Actions.EditCatalog || snapshot.Actions.ReviewSave {
				t.Fatalf("editable Add lost: %+v", snapshot)
			}
			if len(snapshot.NewFileNames) != 1 || snapshot.NewFileNames[0] != "l010.json" {
				t.Fatalf("wrong added names: %v", snapshot.NewFileNames)
			}
			for i, file := range original.Files {
				if !bytes.Equal(file.OriginalBytes(), op.current.Files[i].OriginalBytes()) {
					t.Fatal("old raw rewritten with working blockers")
				}
			}
			if len(snapshot.ManifestChanges) == 0 {
				t.Fatal("blocked Add hid entry changes")
			}
		})
	}
}

func TestAnalyzeSeparateGroupsAndMissingWeightsUseOwnerCatalog(t *testing.T) {
	root := t.TempDir()
	for i, golden := range []int{2, 4, 5} {
		analyzeFixtureSource(t, root, []string{"a.json", "b.json", "c.json"}[i], golden, func(result *model.Result) {
			section := []string{"Z", "a", "Z"}[i]
			result.Document.Metadata.Section = &section
		})
	}
	op, issues := NewCreate(root, root, "Курс", "Юнит")
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	if len(op.placement.Sections) != 2 || op.placement.Sections[0].Title != "a" || op.placement.Sections[1].Title != "Z" {
		t.Fatalf("section order: %+v", op.placement.Sections)
	}
	weights := map[string]string{}
	for _, item := range op.placement.Items {
		if item.Entry.Order == nil {
			t.Fatal("missing synthesized weight")
		}
		weights[item.Entry.FileName] = *item.Entry.Order
		if item.RelativePath == "" || filepath.IsAbs(item.RelativePath) {
			t.Fatal("missing relative source")
		}
	}
	if weights["l001.json"] != "1" || weights["l002.json"] != "1" || weights["l003.json"] != "2" {
		t.Fatalf("synthesis not per group/path: %v", weights)
	}
	for _, input := range op.inputs {
		if input.Result.Document.Metadata.Order != nil {
			t.Fatal("synthesized weight leaked into source")
		}
	}
}

func TestAnalyzeAddDirectoryConvertsFullSortedNewSet(t *testing.T) {
	root := t.TempDir()
	archive, baseline := analyzeFixtureArchive(t, root)
	analyzeFixtureSource(t, root, "incoming/Z.json", 7, func(result *model.Result) { weight := "4"; result.Document.Metadata.Order = &weight })
	analyzeFixtureSource(t, root, "incoming/deep/a.JSON", 5, func(result *model.Result) { weight := "3"; result.Document.Metadata.Order = &weight })
	op, issues := NewAdd(root, archive, filepath.Join(root, "incoming"))
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	snapshot := op.Snapshot()
	if !snapshot.ManifestReady || !op.analyzed || len(snapshot.Module.Files) != 4 || len(snapshot.NewFileNames) != 2 {
		t.Fatal("directory Add incomplete")
	}
	if snapshot.NewFileNames[0] != "l010.json" || snapshot.NewFileNames[1] != "l011.json" || snapshot.Module.Files[2].Lesson().Document.ID != "BA85" || snapshot.Module.Files[3].Lesson().Document.ID != "BA87" {
		t.Fatal("directory Add lost relative order/name assignment")
	}
	for i, file := range baseline.Files {
		if !bytes.Equal(file.OriginalBytes(), op.current.Files[i].OriginalBytes()) {
			t.Fatal("directory Add rewrote old lesson")
		}
	}
}
