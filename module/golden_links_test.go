package module_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func TestGoldenLinksManifestExamples(t *testing.T) {
	for _, kind := range []string{"flat", "unit"} {
		t.Run(kind, func(t *testing.T) {
			value := goldenLinksManifest(t, kind)
			before, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			context := diagnostic.Context{Path: kind + ".zip", FileName: module.ManifestFileName}
			state, placement := catalog.Import(value, context)
			if state == nil {
				t.Fatal("Import returned nil State")
			}
			files := goldenLinksFiles(t, kind, state.Items)
			if kind == "flat" {
				if len(state.Items) != 1 || state.Items[0].Entry.Order != nil {
					t.Fatal("flat example must retain its single null order")
				}
				goldenLinksCodes(t, placement, catalog.CodeMissingWeight)
			} else {
				if len(state.Items) != 11 || len(state.Sections) != 3 {
					t.Fatal("unit example inventory changed")
				}
				goldenLinksCodes(t, placement)
			}
			goldenLinksCodes(t, module.ValidateLinks(files, state.Items, context))
			// Порядок Files не является ни ID, ни размещением уроков.
			for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
				files[i], files[j] = files[j], files[i]
			}
			candidate := &module.Module{Manifest: value, Files: files}
			problems := module.Validate(candidate, context)
			if kind == "flat" {
				goldenLinksCodes(t, problems, catalog.CodeMissingWeight)
				if problems[0].Reason != placement[0].Reason || problems[0].Context != placement[0].Context {
					t.Fatal("catalog diagnosis was not preserved")
				}
			} else {
				goldenLinksCodes(t, problems)
				candidate.Manifest.CreatedAt = "2026-11-07T03:04:05Z"
				goldenLinksCodes(t, module.Validate(candidate, context))
				candidate.Manifest.CreatedAt = value.CreatedAt
			}
			after, err := json.Marshal(candidate.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("module mutated manifest metadata/order/sections")
			}
		})
	}
}

func TestGoldenLinksSixMetadataFields(t *testing.T) {
	value := goldenLinksManifest(t, "unit")
	state, problems := catalog.Import(value, diagnostic.Context{Path: "unit.zip"})
	goldenLinksCodes(t, problems)
	files := goldenLinksFiles(t, "unit", state.Items)
	cases := []struct {
		name   string
		change func(*manifest.Entry)
	}{
		{"id", func(e *manifest.Entry) { e.ID = strings.ToLower(e.ID) }},
		{"title", func(e *manifest.Entry) { e.Title = "Иное название" }},
		{"subtitle-null-vs-empty", func(e *manifest.Entry) { v := ""; e.Subtitle = &v }},
		{"lineCount", func(e *manifest.Entry) { e.LineCount = "505" }},
		{"createdAt", func(e *manifest.Entry) { e.CreatedAt = "2026-11-05T09:00:00Z" }},
		{"version", func(e *manifest.Entry) { e.Version = "2" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := catalog.Clone(state)
			tc.change(&changed.Items[0].Entry)
			got := module.ValidateLinks(files, changed.Items, diagnostic.Context{Path: "unit.zip"})
			goldenLinksCodes(t, got, module.CodeMetadata)
			if got[0].Context.FileName != "l001.json" || got[0].Context.Path != "unit.zip" || len(got[0].Details) < 3 {
				t.Fatalf("incomplete mismatch diagnosis: %+v", got[0])
			}
			goldenLinksCodes(t, module.ValidateLinks(files, state.Items, diagnostic.Context{}))
		})
	}
}

func TestGoldenLinksExactFileNamesAndCoverage(t *testing.T) {
	value := goldenLinksManifest(t, "unit")
	state, problems := catalog.Import(value, diagnostic.Context{Path: "unit.zip"})
	goldenLinksCodes(t, problems)
	files := goldenLinksFiles(t, "unit", state.Items)
	t.Run("filename-case-is-invalid", func(t *testing.T) {
		value.Course.Unit.Sections[0].Lessons[0].FileName = "L001.json"
		got := module.Validate(&module.Module{Manifest: value, Files: files}, diagnostic.Context{Path: "unit.zip"})
		goldenLinksCodes(t, got, identity.CodeInvalidFileName)
	})
	t.Run("exact-valid-name-required", func(t *testing.T) {
		changed := catalog.Clone(state)
		changed.Items[0].Entry.FileName = "l999.json"
		got := module.ValidateLinks(files, changed.Items, diagnostic.Context{Path: "unit.zip"})
		goldenLinksCodes(t, got, module.CodeMissingFile, module.CodeUnlistedFile)
	})
	t.Run("duplicate-reference-across-sections", func(t *testing.T) {
		changed := catalog.Clone(state)
		changed.Items[4].Entry.FileName = changed.Items[0].Entry.FileName
		got := module.ValidateLinks(files, changed.Items, diagnostic.Context{Path: "unit.zip"})
		goldenLinksCodes(t, got, module.CodeDuplicateReference, module.CodeUnlistedFile)
		if len(got[0].Related) != 2 {
			t.Fatalf("both references required: %+v", got[0])
		}
	})
}

func TestGoldenLinksIntegerSpellingAndPrecision(t *testing.T) {
	for _, numbers := range [][2]string{{"1", "1.0"}, {"1.0", "1e0"}, {"1e0", "1"}, {"9007199254740993", "9007199254740993.0"}, {"123456789012345678901234567890", "123456789012345678901234567890e0"}} {
		t.Run(fmt.Sprint(numbers), func(t *testing.T) {
			document := lesson.Document{ID: "Число+1", Title: "Число", Version: json.Number(numbers[0]), LineCount: json.Number(numbers[0]), CreatedAt: "2026-11-03T08:00:00Z", Pages: []lesson.Page{}}
			file, problems := module.NewFile("l001.json", &lesson.Lesson{FormatVersion: "1.0", Document: document}, diagnostic.Context{})
			goldenLinksCodes(t, problems)
			weight := "01"
			entry := catalog.EntryFromLesson(document, "l001.json", &weight)
			entry.Version = json.Number(numbers[1])
			entry.LineCount = json.Number(numbers[1])
			goldenLinksCodes(t, module.ValidateLinks([]*module.File{file}, []catalog.Item{{Entry: entry}}, diagnostic.Context{}))
			if file.Lesson().Document.Version != json.Number(numbers[0]) || entry.Version != json.Number(numbers[1]) {
				t.Fatal("metadata comparison normalized numbers")
			}
			entry.LineCount = "9007199254740992"
			goldenLinksCodes(t, module.ValidateLinks([]*module.File{file}, []catalog.Item{{Entry: entry}}, diagnostic.Context{}), module.CodeMetadata)
		})
	}
}

func goldenLinksManifest(t *testing.T, kind string) manifest.Manifest {
	t.Helper()
	raw, err := os.ReadFile("../Manifest " + kind + " example.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	context := diagnostic.Context{FileName: module.ManifestFileName}
	goldenLinksCodes(t, manifest.ValidateShape(tree, context))
	var value manifest.Manifest
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	goldenLinksCodes(t, manifest.Validate(value, context))
	return value
}

func goldenLinksFiles(t *testing.T, kind string, items []catalog.Item) []*module.File {
	t.Helper()
	raw, err := os.ReadFile("testdata/golden/normative." + kind + ".lessons.json")
	if err != nil {
		t.Fatal(err)
	}
	var lessons []lesson.Lesson
	if err := json.Unmarshal(raw, &lessons); err != nil {
		t.Fatal(err)
	}
	if len(lessons) != len(items) {
		t.Fatalf("fixture lesson count %d, entries %d", len(lessons), len(items))
	}
	files := make([]*module.File, 0, len(lessons))
	for i := range lessons {
		goldenLinksCodes(t, lesson.Validate(&lessons[i], diagnostic.Context{}))
		expected := items[i].Entry
		got := catalog.EntryFromLesson(lessons[i].Document, expected.FileName, expected.Order)
		gotRaw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		expectedRaw, err := json.Marshal(expected)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(gotRaw, expectedRaw) {
			t.Fatalf("fixture metadata %d differs from exact example", i)
		}
		file, problems := module.NewFile(expected.FileName, &lessons[i], diagnostic.Context{Path: kind + ".zip"})
		goldenLinksCodes(t, problems)
		if file == nil {
			t.Fatal("NewFile returned nil")
		}
		files = append(files, file)
	}
	return files
}

func goldenLinksCodes(t *testing.T, got []*diagnostic.Error, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v; want codes %v", got, want)
	}
	for i, problem := range got {
		if problem == nil || problem.Code != want[i] {
			t.Fatalf("problem %d: %+v, want %s", i, problem, want[i])
		}
	}
}
