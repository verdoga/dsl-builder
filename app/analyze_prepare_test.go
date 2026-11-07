package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestAnalyzeRejectsDuplicateIDsBeforeConversion(t *testing.T) {
	root := t.TempDir()
	first := analyzeFixtureSource(t, root, "a.json", 1, func(result *model.Result) { id := "Ba82"; result.Document.Metadata.DocumentID = &id })
	second := analyzeFixtureSource(t, root, "z.json", 2, func(result *model.Result) { id := "bA82"; result.Document.Metadata.DocumentID = &id })
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	got := op.Analyze(analyzeFixtureTime())
	expected := module.CheckNewIDs([]diagnostic.Context{{Path: first, LessonID: "Ba82"}, {Path: second, LessonID: "bA82"}}, nil)
	if !bytes.Equal(analyzeFixtureJSON(t, got), analyzeFixtureJSON(t, expected)) {
		t.Fatalf("owner diagnostics changed: %v != %v", got, expected)
	}
	if len(op.converted) != 0 || op.current != nil || op.analyzed {
		t.Fatal("duplicate IDs allowed conversion/publication")
	}
}

func TestAnalyzeInitialPlacementPrecedesConversion(t *testing.T) {
	for _, scenario := range []string{"partial-order", "mixed-sections"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			analyzeFixtureSource(t, root, "a.json", 1, func(result *model.Result) {
				text := "001"
				if scenario == "partial-order" {
					result.Document.Metadata.Order = &text
				} else {
					text = "Секция"
					result.Document.Metadata.Section = &text
				}
			})
			analyzeFixtureSource(t, root, "z.json", 2, nil)
			op, issues := NewCreate(root, root, "Курс", "")
			analyzeFixtureOK(t, issues)
			code := catalog.CodePartialWeight
			if scenario == "mixed-sections" {
				code = catalog.CodeMixedSections
			}
			got := op.Analyze(analyzeFixtureTime())
			issue := analyzeFixtureIssues(t, got, code)
			if len(issue.Related) != 2 || issue.Related[0].FileName != "l001.json" || issue.Related[1].FileName != "l002.json" {
				t.Fatalf("partial Entry lacks names/context: %+v", issue)
			}
			for _, problem := range got {
				if strings.HasPrefix(problem.Code, "resourcefind.") {
					t.Fatalf("conversion preceded placement: %v", got)
				}
			}
			if len(op.converted) != 0 || op.current != nil || op.analyzed {
				t.Fatal("invalid placement was converted/published")
			}
		})
	}
}

func TestAnalyzeIncomingItemsPreserveMetadataAndLoadedInput(t *testing.T) {
	root := t.TempDir()
	path := analyzeFixtureSource(t, root, "подкаталог/урок.JSON", 5, func(result *model.Result) {
		section, order, title, subtitle := "Studentbook", "0009999999999999999999999999999", "Точное название", ""
		result.Document.Metadata.Section, result.Document.Metadata.Order = &section, &order
		result.Document.Metadata.Title, result.Document.Metadata.Subtitle = &title, &subtitle
	})
	expectedInput := analyzeFixtureJSON(t, analyzeFixtureLoad(t, path))
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	before := analyzeFixtureJSON(t, op.inputs[0])
	if !bytes.Equal(expectedInput, before) {
		t.Fatal("Analyze changed loaded input")
	}
	items, issues := op.incomingItems()
	analyzeFixtureOK(t, issues)
	if len(items) != 1 {
		t.Fatal("wrong item count")
	}
	item := items[0]
	if item.RelativePath != "подкаталог/урок.JSON" || item.Context != (diagnostic.Context{Path: path, FileName: "l001.json", LessonID: "BA85"}) {
		t.Fatalf("wrong source: %+v", item)
	}
	if item.Section == nil || *item.Section != "Studentbook" || item.Entry.Order == nil || *item.Entry.Order != "0009999999999999999999999999999" {
		t.Fatalf("placement metadata changed: %+v", item)
	}
	document := op.converted[0].Lesson().Document
	expected := catalog.EntryFromLesson(document, "l001.json", op.inputs[0].Result.Document.Metadata.Order)
	if !bytes.Equal(analyzeFixtureJSON(t, item.Entry), analyzeFixtureJSON(t, expected)) {
		t.Fatalf("EntryFromLesson fields changed: %+v", item.Entry)
	}
	if document.CreatedAt != analyzeFixtureTime().UTC().Format("2006-01-02T15:04:05Z") || document.LineCount.String() != "29" || document.Version.String() != "1" {
		t.Fatalf("converted metadata: %+v", document)
	}
	if !bytes.Equal(before, analyzeFixtureJSON(t, op.inputs[0])) {
		t.Fatal("incomingItems changed loaded input")
	}
}

func TestAnalyzeAddSingleFileUsesContainingDirectoryForRelativePath(t *testing.T) {
	root := t.TempDir()
	archive, _ := analyzeFixtureArchive(t, root)
	source := analyzeFixtureSource(t, root, "new/deep/source.JSON", 7, func(result *model.Result) { order := "3"; result.Document.Metadata.Order = &order })
	op, issues := NewAdd(root, archive, source)
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	items, issues := op.incomingItems()
	analyzeFixtureOK(t, issues)
	if len(items) != 1 || items[0].RelativePath != filepath.Base(source) || items[0].Context.Path != source {
		t.Fatalf("single-file base: %+v", items)
	}
	if op.names[0] != "l010.json" {
		t.Fatalf("filled old numbering gap: %v", op.names)
	}
}
