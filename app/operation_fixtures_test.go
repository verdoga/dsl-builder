package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func operationWriteInput(t *testing.T, path, id, title string, order *string, dirs []string) []byte {
	t.Helper()
	lineCount := 1
	text := "Учебный текст"
	if dirs == nil {
		dirs = []string{}
	}
	value := model.Result{
		Format: "1.0",
		Document: model.Document{
			LineCount: &lineCount,
			Metadata: model.DocumentMetadata{
				DocumentID: &id, Title: &title, Order: order, ResourceDirs: dirs,
			},
		},
		Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Lines: []model.Line{{
			Number: 1, LineType: "content", Raw: text,
			Elements: []model.Element{{
				ElementType: "content", Raw: text, Value: &text,
				Start: 1, End: len([]rune(text)) + 1, ErrorIDs: []string{},
			}},
		}},
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, issues := parsed.Load(path); len(issues) != 0 {
		t.Fatalf("fixture parsed.Load: %v", issues)
	}
	return raw
}

func operationWriteArchive(t *testing.T, directory, id string) string {
	t.Helper()
	inputPath := filepath.Join(directory, "baseline-source.json")
	operationWriteInput(t, inputPath, id, "Прежнее название", nil, nil)
	input, issues := parsed.Load(inputPath)
	operationNoIssues(t, issues)
	at := time.Date(2026, 11, 1, 10, 20, 30, 0, time.UTC)
	value, _, issues := convert.Build(input, &filetree.Resolver{}, at, diagnostic.Context{})
	operationNoIssues(t, issues)
	file, issues := module.NewFile("l001.json", value, diagnostic.Context{Path: inputPath})
	operationNoIssues(t, issues)
	entry := catalog.EntryFromLesson(value.Document, file.FileName(), nil)
	state, issues := catalog.Create("Исходный курс", "", []catalog.Item{{Entry: entry}})
	operationNoIssues(t, issues)
	manifest, issues := catalog.Export(state, value.Document.CreatedAt, diagnostic.Context{})
	operationNoIssues(t, issues)
	path := filepath.Join(directory, "baseline.zip")
	dst, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	issues = moduleio.Write(dst, &module.Module{Manifest: *manifest, Files: []*module.File{file}}, diagnostic.Context{Path: path})
	closeErr := dst.Close()
	operationNoIssues(t, issues)
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	_, issues = moduleio.Read(path)
	operationNoIssues(t, issues)
	return path
}

func operationNoIssues(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %v", issues)
	}
}

func operationHasCode(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return
		}
	}
	t.Fatalf("diagnostics %v do not contain %q", issues, code)
}

func operationRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOperationFixturesAreOwnerValidated(t *testing.T) {
	operationWriteArchive(t, t.TempDir(), "FIXTURE1")
}
