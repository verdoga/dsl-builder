package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
)

func catalogEditTime() time.Time {
	return time.Date(2026, 11, 7, 10, 11, 12, 0, time.UTC)
}

func catalogEditOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", issues)
	}
}

func catalogEditCode(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return
		}
	}
	t.Fatalf("missing code %q in %+v", code, issues)
}

func catalogEditString(value string) *string { return &value }

func catalogEditSource(t *testing.T, dir, name, id, title string, section, weight *string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/catalog_edit/source.json")
	if err != nil {
		t.Fatal(err)
	}
	encode := func(value *string) string {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	raw = []byte(strings.NewReplacer("$ID", encode(&id), "$TITLE", encode(&title),
		"$SECTION", encode(section), "$ORDER", encode(weight)).Replace(string(raw)))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func catalogEditSources(t *testing.T, sections []string) string {
	t.Helper()
	dir := t.TempDir()
	for i, section := range sections {
		var selected *string
		if section != "" {
			selected = catalogEditString(section)
		}
		catalogEditSource(t, dir, fmt.Sprintf("%02d.json", i), fmt.Sprintf("CE%02d", i),
			fmt.Sprintf("Урок %d", i), selected, catalogEditString(fmt.Sprint(i+1)))
	}
	return dir
}

func catalogEditOwnerModule(t *testing.T, dir string) (*module.Module, *catalog.State) {
	t.Helper()
	tree := new(filetree.Resolver)
	entries, problem := tree.JSONFiles(dir)
	if problem != nil {
		t.Fatal(problem)
	}
	value := &module.Module{}
	var items []catalog.Item
	for i, entry := range entries {
		input, issues := parsed.Load(entry.Path)
		catalogEditOK(t, issues)
		name := fmt.Sprintf("l%03d.json", i+1)
		context := diagnostic.Context{Path: entry.Path, FileName: name}
		lesson, _, issues := convert.Build(input, tree, catalogEditTime(), context)
		catalogEditOK(t, issues)
		file, issues := module.NewFile(name, lesson, context)
		catalogEditOK(t, issues)
		value.Files = append(value.Files, file)
		metadata := input.Result.Document.Metadata
		items = append(items, catalog.Item{Entry: catalog.EntryFromLesson(lesson.Document, name, metadata.Order),
			Section: metadata.Section, RelativePath: filepath.Base(entry.Path), Context: file.Context()})
	}
	state, issues := catalog.Create("Курс", "Юнит", items)
	catalogEditOK(t, issues)
	manifest, issues := catalog.Export(state, catalogEditTime().Format(time.RFC3339), diagnostic.Context{})
	catalogEditOK(t, issues)
	value.Manifest = *manifest
	catalogEditOK(t, module.Validate(value, diagnostic.Context{}))
	return value, state
}

func catalogEditArchive(t *testing.T, sections []string) string {
	t.Helper()
	value, _ := catalogEditOwnerModule(t, catalogEditSources(t, sections))
	var zip bytes.Buffer
	catalogEditOK(t, moduleio.Write(&zip, value, diagnostic.Context{}))
	path := filepath.Join(t.TempDir(), "исходный.zip")
	if err := os.WriteFile(path, zip.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, issues := moduleio.Read(path)
	catalogEditOK(t, issues)
	if len(loaded.Files) != len(sections) || len(loaded.Files[0].OriginalBytes()) == 0 {
		t.Fatal("owner archive lost files or original bytes")
	}
	return path
}

func TestCatalogEditFixtureOwners(t *testing.T) {
	catalogEditArchive(t, []string{"", ""})
	catalogEditArchive(t, []string{"Alpha", "Alpha", "Beta"})
}
