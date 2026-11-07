package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestAnalyzeOwnerFixtureMetadataVariantsLoadBeforeUse(t *testing.T) {
	for _, scenario := range []string{"case-id", "order", "section", "title", "media"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			golden := 2
			if scenario == "media" {
				golden = 1
			}
			path := analyzeFixtureSource(t, root, "docs/source.json", golden, func(result *model.Result) {
				switch scenario {
				case "case-id":
					id := "ba82"
					result.Document.Metadata.DocumentID = &id
				case "order":
					order := "0009999999999999999999999999999"
					result.Document.Metadata.Order = &order
				case "section":
					section := "Studentbook"
					result.Document.Metadata.Section = &section
				case "title":
					title, subtitle := "Изменено на диске", ""
					result.Document.Metadata.Title, result.Document.Metadata.Subtitle = &title, &subtitle
				case "media":
					result.Document.Metadata.ResourceDirs = []string{"../resources", "../resources"}
				}
			})
			if scenario == "media" {
				analyzeFixtureWrite(t, filepath.Join(root, "resources", "track12.01.mp3"), []byte{0, 0xff, 1, 2, 3})
			}
			input := analyzeFixtureLoad(t, path)
			_, _, issues := convert.Build(input, &filetree.Resolver{}, analyzeFixtureTime(), diagnostic.Context{FileName: "l001.json"})
			analyzeFixtureOK(t, issues)
		})
	}
}

func TestAnalyzeOwnerFixtureParserMarkersProduceAllDiagnostics(t *testing.T) {
	root := t.TempDir()
	path := analyzeFixtureSource(t, root, "source.json", 2, func(result *model.Result) {
		result.Document.HasErrors = true
		result.Lines[0].LineType = model.LineTypeInvalid
		result.Lines[1].Elements[0].ElementType = model.ElementTypeUnparsed
	})
	_, issues := parsed.Load(path)
	if len(issues) < 3 {
		t.Fatalf("not enough parser marker diagnostics: %v", issues)
	}
}

func TestAnalyzeOwnerFixtureSharedMediaIdentity(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"track12.01.mp3", "track12.01.txt", "track12.01.vtt", "unused.bin"} {
		original := filepath.Join(root, "resources", name)
		analyzeFixtureWrite(t, original, []byte{0, 1, 2, 3})
		if err := os.MkdirAll(filepath.Join(root, "alias"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(original, filepath.Join(root, "alias", name)); err != nil {
			t.Fatal(err)
		}
	}
	var tree filetree.Resolver
	var registry resourceusage.Registry
	for i, directory := range []string{"resources", "alias"} {
		path := analyzeFixtureSource(t, root, fmt.Sprintf("docs/%d.json", i), 1, func(result *model.Result) {
			id := fmt.Sprintf("EFX%d", 7+i)
			result.Document.Metadata.DocumentID = &id
			result.Document.Metadata.ResourceDirs = []string{"../" + directory}
		})
		_, usage, issues := convert.Build(analyzeFixtureLoad(t, path), &tree, analyzeFixtureTime(), diagnostic.Context{FileName: fmt.Sprintf("l00%d.json", i+1)})
		analyzeFixtureOK(t, issues)
		registry.Merge(usage)
	}
	report := registry.Snapshot()
	if report.Total != 4 || report.Used != 3 || report.Unused != 1 {
		t.Fatalf("invalid shared identity fixture: %+v", report)
	}
}
