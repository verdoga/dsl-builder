package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/parsed"
)

func TestAnalyzeDiscoveryModes(t *testing.T) {
	for _, mode := range []Mode{Create, Update, Add} {
		for _, directory := range []bool{false, true} {
			t.Run(string(mode)+map[bool]string{true: "/directory", false: "/file"}[directory], func(t *testing.T) {
				root := t.TempDir()
				archive, _ := analyzeFixtureArchive(t, root)
				path := analyzeFixtureSource(t, root, "new/a.JSON", 5, nil)
				if directory {
					path = filepath.Dir(path)
				}
				var op *Operation
				switch mode {
				case Create:
					op, _ = NewCreate(root, path, "Курс", "")
				case Update:
					op, _ = NewUpdate(root, archive, path)
				case Add:
					op, _ = NewAdd(root, archive, path)
				}
				if op == nil {
					t.Fatal("nil constructor")
				}
				op.tree = &filetree.Resolver{}
				issues := op.discoverSources()
				if mode == Create && !directory || mode == Update && directory {
					analyzeFixtureIssues(t, issues, filetree.CodeKind)
					return
				}
				analyzeFixtureOK(t, issues)
				if len(op.entries) != 1 || filepath.Ext(op.entries[0].Path) != ".JSON" {
					t.Fatalf("entries: %+v", op.entries)
				}
			})
		}
	}
}

func TestAnalyzeDiscoveryRecursesSortsAndKeepsEqualBaseNames(t *testing.T) {
	root := t.TempDir()
	expected := []string{"a.JSON", "b/deep/work.json", "z/work.json"}
	analyzeFixtureSource(t, root, expected[2], 7, nil)
	analyzeFixtureSource(t, root, expected[1], 4, nil)
	analyzeFixtureSource(t, root, expected[0], 2, nil)
	analyzeFixtureWrite(t, filepath.Join(root, "readme.txt"), []byte("ignore extension"))
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	if len(op.entries) != len(expected) || len(op.names) != len(expected) {
		t.Fatalf("entries/names: %v %v", op.entries, op.names)
	}
	for i, relative := range expected {
		if op.entries[i].Path != filepath.Join(root, filepath.FromSlash(relative)) {
			t.Fatalf("entry %d: %+v", i, op.entries[i])
		}
		if op.current.Files[i].FileName() != []string{"l001.json", "l002.json", "l003.json"}[i] {
			t.Fatal("names do not follow paths")
		}
	}
	if op.inputs[0].Result.Document.Metadata.Order != nil {
		t.Fatal("synthesis changed parsed metadata")
	}
}

func TestAnalyzeDiscoveryDeduplicatesFileIdentity(t *testing.T) {
	root := t.TempDir()
	original := analyzeFixtureSource(t, root, "a.json", 2, nil)
	if err := os.Link(original, filepath.Join(root, "b.JSON")); err != nil {
		t.Fatal(err)
	}
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	if len(op.entries) != 1 || len(op.inputs) != 1 || len(op.current.Files) != 1 {
		t.Fatal("same filesystem object was processed twice")
	}
}

func TestAnalyzeDiscoveryErrorsKeepOwnerDiagnostics(t *testing.T) {
	for _, scenario := range []string{"empty", "missing", "bad-json", "wrong-extension"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			path, code := root, filetree.CodeEmptyJSON
			if scenario == "missing" {
				path = filepath.Join(root, "missing")
				code = filetree.CodeAccess
			}
			if scenario == "bad-json" {
				analyzeFixtureSource(t, root, "a.json", 2, nil)
				analyzeFixtureWrite(t, filepath.Join(root, "z.JSON"), []byte("{}"))
				code = parsed.CodeContract
			}
			op, issues := NewCreate(root, path, "Курс", "")
			if scenario == "wrong-extension" {
				archive, _ := analyzeFixtureArchive(t, root)
				path = filepath.Join(root, "source.txt")
				analyzeFixtureWrite(t, path, []byte("{}"))
				op, issues = NewAdd(root, archive, path)
				code = filetree.CodeExtension
			}
			analyzeFixtureOK(t, issues)
			issue := analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), code)
			if issue.Reason == "" || issue.Context.Path == "" {
				t.Fatalf("missing owner context: %+v", issue)
			}
			if op.analyzed || op.current != nil || op.Snapshot().Actions.ReviewSave {
				t.Fatal("published incomplete input")
			}
		})
	}
}

func TestAnalyzeLoadsEveryJSONBeforeConvert(t *testing.T) {
	root := t.TempDir()
	first := analyzeFixtureSource(t, root, "a.json", 1, nil)
	bad := filepath.Join(root, "z.JSON")
	analyzeFixtureWrite(t, bad, []byte("{"))
	original := analyzeFixtureBytes(t, first)
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	issue := analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), parsed.CodeJSON)
	if issue.Context.Path != bad || issue.Cause == nil {
		t.Fatalf("source error not retained: %+v", issue)
	}
	if len(op.converted) != 0 || op.current != nil {
		t.Fatal("conversion ran before source validation")
	}
	if !bytes.Equal(original, analyzeFixtureBytes(t, first)) {
		t.Fatal("source modified")
	}
}
