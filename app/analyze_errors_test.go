package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestAnalyzePreservesParsedOwnerDiagnostics(t *testing.T) {
	for _, scenario := range []string{"missing-id", "format", "parser-flag", "invalid-line", "unparsed", "parent", "utf8", "json"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			path := analyzeFixtureSource(t, root, "source.JSON", 2, func(result *model.Result) {
				switch scenario {
				case "missing-id":
					result.Document.Metadata.DocumentID = nil
				case "format":
					result.Format = "99"
				case "parser-flag":
					result.Document.HasErrors = true
				case "invalid-line":
					result.Lines[0].LineType = model.LineTypeInvalid
				case "unparsed":
					result.Lines[0].Elements[0].ElementType = model.ElementTypeUnparsed
				case "parent":
					parent := 999
					result.Lines[0].ParentLine = &parent
				}
			})
			if scenario == "utf8" {
				analyzeFixtureWrite(t, path, []byte{0xff, 0x80})
			}
			if scenario == "json" {
				analyzeFixtureWrite(t, path, []byte("{"))
			}
			_, expected := parsed.Load(path)
			if len(expected) == 0 {
				t.Fatal("fixture must be rejected by real parsed.Load")
			}
			op, issues := NewCreate(root, root, "Курс", "")
			analyzeFixtureOK(t, issues)
			actual := op.Analyze(analyzeFixtureTime())
			if !bytes.Equal(analyzeFixtureJSON(t, expected), analyzeFixtureJSON(t, actual)) {
				t.Fatalf("parsed diagnostics changed: %v != %v", actual, expected)
			}
			if scenario == "json" {
				var syntax *json.SyntaxError
				if !errors.As(actual[0], &syntax) {
					t.Fatal("JSON syntax Cause was flattened")
				}
			}
			if op.analyzed || op.current != nil || len(op.converted) != 0 {
				t.Fatal("invalid source published a candidate")
			}
		})
	}
}

func TestAnalyzePreservesEveryOwnerDiagnosticWithinAFile(t *testing.T) {
	root := t.TempDir()
	path := analyzeFixtureSource(t, root, "source.json", 2, func(result *model.Result) {
		result.Document.HasErrors = true
		result.Lines[0].LineType = model.LineTypeInvalid
		result.Lines[1].Elements[0].ElementType = model.ElementTypeUnparsed
	})
	_, expected := parsed.Load(path)
	if len(expected) < 3 {
		t.Fatalf("fixture lacks independent parser markers: %v", expected)
	}
	op, issues := NewCreate(root, filepath.Dir(path), "Курс", "")
	analyzeFixtureOK(t, issues)
	actual := op.Analyze(analyzeFixtureTime())
	if !bytes.Equal(analyzeFixtureJSON(t, expected), analyzeFixtureJSON(t, actual)) {
		t.Fatalf("owner errors reordered or truncated: %v != %v", actual, expected)
	}
}

func TestAnalyzeDeclaredResourceDirectoryErrorKeepsCause(t *testing.T) {
	root := t.TempDir()
	path := analyzeFixtureSource(t, root, "source.json", 2, func(result *model.Result) { result.Document.Metadata.ResourceDirs = []string{"missing-resources"} })
	input := analyzeFixtureLoad(t, path)
	_, _, expected := convert.Build(input, &filetree.Resolver{}, analyzeFixtureTime(), diagnostic.Context{FileName: "l001.json"})
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	actual := op.Analyze(analyzeFixtureTime())
	if len(expected) == 0 || !bytes.Equal(analyzeFixtureJSON(t, actual), analyzeFixtureJSON(t, expected)) {
		t.Fatalf("resource-directory owner error changed: %v != %v", actual, expected)
	}
	if !errors.Is(actual[0], os.ErrNotExist) {
		t.Fatal("underlying filesystem Cause lost")
	}
	if op.current != nil || op.analyzed {
		t.Fatal("resource directory error published module")
	}
}
