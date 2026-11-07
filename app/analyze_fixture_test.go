package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
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

func analyzeFixtureTime() time.Time {
	return time.Date(2026, 9, 17, 12, 13, 14, 876, time.FixedZone("test", 3*3600))
}

func analyzeFixtureBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func analyzeFixtureWrite(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
}

func analyzeFixtureSource(t *testing.T, root, relative string, golden int, edit func(*model.Result)) string {
	t.Helper()
	raw := analyzeFixtureBytes(t, filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", golden)))
	if edit != nil {
		var result model.Result
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		edit(&result)
		var err error
		raw, err = json.MarshalIndent(result, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	analyzeFixtureWrite(t, path, raw)
	return path
}

func analyzeFixtureLoad(t *testing.T, path string) *parsed.Input {
	t.Helper()
	input, issues := parsed.Load(path)
	if len(issues) != 0 || input == nil {
		t.Fatalf("parsed.Load(%s): %v", path, issues)
	}
	return input
}

func analyzeFixtureJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func analyzeFixtureIssues(t *testing.T, issues []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return issue
		}
	}
	t.Fatalf("missing diagnostic %s: %v", code, issues)
	return nil
}

func analyzeFixtureOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %v", issues)
	}
}

func analyzeFixtureArchive(t *testing.T, root string) (string, *module.Module) {
	t.Helper()
	var tree filetree.Resolver
	var files []*module.File
	var items []catalog.Item
	for i, golden := range []int{2, 4} {
		path := analyzeFixtureSource(t, root, fmt.Sprintf("baseline-input/%d.json", i), golden, nil)
		input := analyzeFixtureLoad(t, path)
		context := diagnostic.Context{Path: path}
		value, _, issues := convert.Build(input, &tree, analyzeFixtureTime(), context)
		analyzeFixtureOK(t, issues)
		name := []string{"l002.json", "l009.json"}[i]
		file, issues := module.NewFile(name, value, context)
		analyzeFixtureOK(t, issues)
		files = append(files, file)
		items = append(items, catalog.Item{Entry: catalog.EntryFromLesson(value.Document, name, nil)})
	}
	state, issues := catalog.Create("Исходный курс", "Юнит", items)
	analyzeFixtureOK(t, issues)
	manifest, issues := catalog.Export(state, analyzeFixtureTime().UTC().Format("2006-01-02T15:04:05Z"), diagnostic.Context{})
	analyzeFixtureOK(t, issues)
	value := &module.Module{Manifest: *manifest, Files: files}
	var initial bytes.Buffer
	analyzeFixtureOK(t, moduleio.Write(&initial, value, diagnostic.Context{}))
	reader, err := zip.NewReader(bytes.NewReader(initial.Bytes()), int64(initial.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var final bytes.Buffer
	writer := zip.NewWriter(&final)
	for _, entry := range reader.File {
		source, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		var raw bytes.Buffer
		if _, err := raw.ReadFrom(source); err != nil {
			t.Fatal(err)
		}
		if err := source.Close(); err != nil {
			t.Fatal(err)
		}
		target, err := writer.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := target.Write(append([]byte(" \n\t"), raw.Bytes()...)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "исходный модуль.zip")
	analyzeFixtureWrite(t, path, final.Bytes())
	loaded, issues := moduleio.Read(path)
	analyzeFixtureOK(t, issues)
	if loaded == nil {
		t.Fatal("nil loaded fixture")
	}
	return path, loaded
}

func TestAnalyzeFixtureRealOwners(t *testing.T) {
	root := t.TempDir()
	archive, value := analyzeFixtureArchive(t, root)
	if len(value.Files) != 2 || len(value.Files[0].OriginalBytes()) == 0 {
		t.Fatal("invalid real ZIP fixture")
	}
	analyzeFixtureOK(t, moduleio.Verify(archive))
	for _, n := range []int{2, 4, 5, 6, 7} {
		path := analyzeFixtureSource(t, root, fmt.Sprintf("golden/%d.json", n), n, nil)
		input := analyzeFixtureLoad(t, path)
		_, _, issues := convert.Build(input, &filetree.Resolver{}, analyzeFixtureTime(), diagnostic.Context{})
		analyzeFixtureOK(t, issues)
	}
}
