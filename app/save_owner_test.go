package app

import (
	"bytes"
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

func savePrefixTime() time.Time {
	return time.Date(2026, 11, 4, 10, 11, 12, 987654321, time.FixedZone("test", 3*3600))
}

func savePrefixOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %v", issues)
	}
}

func savePrefixCode(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return
		}
	}
	t.Fatalf("missing code %q: %v", code, issues)
}

func savePrefixWrite(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func savePrefixRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func savePrefixSource(t *testing.T, path, title string, order *string) {
	t.Helper()
	raw := savePrefixRead(t, "testdata/save/source.json")
	var value model.Result
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	value.Document.Metadata.Order = order
	if title != "" {
		value.Document.Metadata.Title = &title
		line := &value.Lines[2]
		line.Raw = "# " + title
		line.Elements[1].Raw = title
		line.Elements[1].Value = &title
		line.Elements[1].End = 3 + len([]rune(title))
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	savePrefixWrite(t, path, raw)
}

func savePrefixArchive(t *testing.T, base, source string) string {
	t.Helper()
	input, issues := parsed.Load(source)
	savePrefixOK(t, issues)
	value, _, issues := convert.Build(input, &filetree.Resolver{}, savePrefixTime(), diagnostic.Context{})
	savePrefixOK(t, issues)
	file, issues := module.NewFile("l001.json", value, diagnostic.Context{Path: source})
	savePrefixOK(t, issues)
	weight := "1"
	state, issues := catalog.Create("Курс", "Юнит", []catalog.Item{{Entry: catalog.EntryFromLesson(value.Document, file.FileName(), &weight)}})
	savePrefixOK(t, issues)
	manifest, issues := catalog.Export(state, "2026-11-04T07:11:12Z", diagnostic.Context{})
	savePrefixOK(t, issues)
	var dst bytes.Buffer
	savePrefixOK(t, moduleio.Write(&dst, &module.Module{Manifest: *manifest, Files: []*module.File{file}}, diagnostic.Context{}))
	path := filepath.Join(base, "исходный.zip")
	savePrefixWrite(t, path, dst.Bytes())
	_, issues = moduleio.Read(path)
	savePrefixOK(t, issues)
	return path
}

func savePrefixAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no target %q, got %v", path, err)
	}
}

func TestSaveOwnerFixtureRoundtrip(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source.json")
	savePrefixSource(t, source, "", nil)
	archive := savePrefixArchive(t, base, source)
	value, issues := moduleio.Read(archive)
	savePrefixOK(t, issues)
	if len(value.Files) != 1 || value.Files[0].Lesson().Document.Version.String() != "1" || value.Files[0].Lesson().Document.LineCount.String() != "23" {
		t.Fatal("invalid baseline fixture")
	}
	weight := "2"
	savePrefixSource(t, source, "Другое название", &weight)
	input, issues := parsed.Load(source)
	savePrefixOK(t, issues)
	converted, _, issues := convert.Build(input, &filetree.Resolver{}, savePrefixTime(), diagnostic.Context{})
	savePrefixOK(t, issues)
	if converted.Document.Title != "Другое название" || *input.Result.Document.Metadata.Order != "2" {
		t.Fatal("changed input fixture lost metadata")
	}
}
