package app

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestAnalyzeOriginalMediaGoldenDoesNotSearchNeighbours(t *testing.T) {
	for _, golden := range []int{1, 3} {
		t.Run(string(rune('0'+golden)), func(t *testing.T) {
			root := t.TempDir()
			source := analyzeFixtureSource(t, root, "source.json", golden, nil)
			analyzeFixtureWrite(t, filepath.Join(root, "12.01.mp3"), []byte{0, 1, 2, 3, 4})
			analyzeFixtureWrite(t, filepath.Join(root, "003.mp4"), []byte{5, 6, 7, 8})
			original := analyzeFixtureBytes(t, source)
			op, issues := NewCreate(root, root, "Курс", "")
			analyzeFixtureOK(t, issues)
			issue := analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), "resourcefind.not_found")
			if issue.Context.Path != source || issue.Context.FileName != "l001.json" || issue.Context.LessonID == "" || issue.Context.Line == 0 {
				t.Fatalf("missing source coordinates/name: %+v", issue)
			}
			if op.analyzed || op.current != nil {
				t.Fatal("missing mandatory media published partial module")
			}
			if !bytes.Equal(original, analyzeFixtureBytes(t, source)) {
				t.Fatal("golden input rewritten")
			}
		})
	}
}

func TestAnalyzeSharedResolverAndRegistryUnion(t *testing.T) {
	root := t.TempDir()
	content := []byte{0, 0xff, 0x80, 1, 2, 3, 4}
	for name, raw := range map[string][]byte{"track12.01.mp3": content, "track12.01.txt": []byte("Точная расшифровка\r\n"), "track12.01.vtt": []byte("WEBVTT\n"), "unused.bin": []byte{7, 8}} {
		path := filepath.Join(root, "resources", name)
		analyzeFixtureWrite(t, path, raw)
		if err := os.MkdirAll(filepath.Join(root, "alias"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(path, filepath.Join(root, "alias", name)); err != nil {
			t.Fatal(err)
		}
	}
	first := analyzeFixtureSource(t, root, "docs/a.json", 1, func(result *model.Result) {
		result.Document.Metadata.ResourceDirs = []string{"../resources", "../resources"}
	})
	second := analyzeFixtureSource(t, root, "docs/b.JSON", 1, func(result *model.Result) {
		id := "EFX8"
		result.Document.Metadata.DocumentID = &id
		result.Document.Metadata.ResourceDirs = []string{"../alias"}
	})
	firstRaw, secondRaw := analyzeFixtureBytes(t, first), analyzeFixtureBytes(t, second)
	op, issues := NewCreate(root, filepath.Join(root, "docs"), "Курс", "")
	analyzeFixtureOK(t, issues)
	analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
	report := op.Snapshot().Resources
	if report.Total != 4 || report.Used != 3 || report.Unused != 1 || len(report.UsedPaths) != 3 || len(report.UnusedPaths) != 1 {
		t.Fatalf("registry sums lessons/aliases: %+v", report)
	}
	if report.UnusedPaths[0] != filepath.Join(root, "resources", "unused.bin") {
		t.Fatalf("identity lost: %+v", report)
	}
	for _, file := range op.current.Files {
		document := file.Lesson().Document
		if document.CreatedAt != analyzeFixtureTime().UTC().Format("2006-01-02T15:04:05Z") {
			t.Fatal("unstable conversion timestamp")
		}
		found := false
		var walk func([]lesson.Node)
		walk = func(nodes []lesson.Node) {
			for _, node := range nodes {
				if node.Type == lesson.NodeMedia {
					for _, item := range node.Data {
						if item.Type != lesson.DataMedia || item.Role == nil || *item.Role != lesson.RoleBody {
							continue
						}
						media, ok := item.Value.(lesson.EmbeddedFile)
						if !ok {
							t.Fatal("wrong media shape")
						}
						decoded, err := base64.StdEncoding.DecodeString(media.Content)
						if err != nil || !bytes.Equal(decoded, content) {
							t.Fatal("media bytes changed")
						}
						found = true
					}
				}
				walk(node.Nodes)
			}
		}
		for _, page := range document.Pages {
			walk(page.Nodes)
		}
		if !found {
			t.Fatal("media not embedded")
		}
	}
	if !bytes.Equal(firstRaw, analyzeFixtureBytes(t, first)) || !bytes.Equal(secondRaw, analyzeFixtureBytes(t, second)) {
		t.Fatal("input JSON mutated")
	}
}

func TestAnalyzeSecondConversionFailureNeverPublishesPartialCurrent(t *testing.T) {
	root := t.TempDir()
	analyzeFixtureSource(t, root, "a.json", 2, nil)
	analyzeFixtureSource(t, root, "z.json", 1, nil)
	op, issues := NewCreate(root, root, "Курс", "")
	analyzeFixtureOK(t, issues)
	issue := analyzeFixtureIssues(t, op.Analyze(analyzeFixtureTime()), "resourcefind.not_found")
	if issue.Context.FileName != "l002.json" {
		t.Fatalf("technical names assigned after conversion: %+v", issue)
	}
	if op.current != nil || op.placement != nil || op.analyzed || op.Snapshot().Module != nil {
		t.Fatal("partial converted module escaped")
	}
}

func TestAnalyzeValidOriginalGoldenInputs(t *testing.T) {
	for _, golden := range []int{2, 4, 5, 6, 7} {
		t.Run(string(rune('0'+golden)), func(t *testing.T) {
			root := t.TempDir()
			path := analyzeFixtureSource(t, root, "source.json", golden, nil)
			input := analyzeFixtureLoad(t, path)
			before := analyzeFixtureBytes(t, path)
			op, issues := NewCreate(root, root, "Курс", "")
			analyzeFixtureOK(t, issues)
			analyzeFixtureOK(t, op.Analyze(analyzeFixtureTime()))
			snapshot := op.Snapshot()
			if !op.analyzed || !snapshot.ManifestReady || len(snapshot.Module.Files) != 1 {
				t.Fatal("valid golden analysis incomplete")
			}
			document := snapshot.Module.Files[0].Lesson().Document
			if document.ID != *input.Result.Document.Metadata.DocumentID || document.CreatedAt != analyzeFixtureTime().UTC().Format("2006-01-02T15:04:05Z") {
				t.Fatal("metadata not carried through real conversion")
			}
			if !bytes.Equal(before, analyzeFixtureBytes(t, path)) {
				t.Fatal("source modified")
			}
		})
	}
}
