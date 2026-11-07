package moduleio_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func TestGoldenHistoricalLessonsApplicable(t *testing.T) {
	cases := []struct{ id, title, subtitle, lines, created string }{
		{"EFX7", "12A I'M GOING TO HAVE A LONG HOT BATH", "", "36", "2026-11-03T08:34:59Z"},
		{"BA82", "TITLE", "Subtitle", "23", "2026-11-03T17:32:07Z"},
		{"BA83", "TITLE", "", "44", "2026-11-03T17:36:18Z"},
		{"BA84", "TITLE", "", "27", "2026-11-03T17:54:17Z"},
		{"BA85", "TITLE", "Subtitle", "29", "2026-11-03T18:05:25Z"},
		{"BA86", "TITLE", "", "45", "2026-11-03T18:07:54Z"},
		{"BA87", "TITLE", "", "39", "2026-11-03T18:11:31Z"},
	}
	for i, want := range cases {
		t.Run(want.id, func(t *testing.T) {
			path := filepath.Join("..", "golden", fmt.Sprintf("%d. lesson.json", i+1))
			value := GoldenLesson(t, GoldenRead(t, path))
			d := value.Document
			if d.ID != want.id || d.Title != want.title || string(d.LineCount) != want.lines || d.CreatedAt != want.created || d.Version != "1" {
				t.Fatalf("six metadata do not match historical source: %+v", d)
			}
			if want.subtitle == "" {
				if d.Subtitle != nil {
					t.Fatal("historical null subtitle changed")
				}
			} else if d.Subtitle == nil || *d.Subtitle != want.subtitle {
				t.Fatal("historical subtitle changed")
			}
			GoldenDataTypes(t, value)
		})
	}
}

func TestGoldenManifestExamplesApplicability(t *testing.T) {
	for _, kind := range []string{"flat", "unit"} {
		t.Run(kind, func(t *testing.T) {
			value := GoldenManifest(t, GoldenRead(t, "../Manifest "+kind+" example.json"))
			candidate := GoldenExampleModule(t, value)
			problems := module.Validate(candidate, diagnostic.Context{})
			if kind == "flat" {
				if value.Course.Unit != nil || len(*value.Course.Lessons) != 1 || len(*value.Course.Sections) != 0 || (*value.Course.Lessons)[0].Order != nil {
					t.Fatal("flat example lost its exact nullable-weight shape")
				}
				GoldenRequireCode(t, problems, "catalog.missing_weight")
			} else {
				if value.Course.Lessons != nil || value.Course.Sections != nil || value.Course.Unit == nil || len(candidate.Files) != 11 || len(value.Course.Unit.Lessons) != 0 || len(value.Course.Unit.Sections) != 3 {
					t.Fatal("unit example lost its full hierarchy")
				}
				GoldenNoProblems(t, problems)
			}
		})
	}
}

func TestGoldenUnitExampleIsNestedTechnicalInput(t *testing.T) {
	reader, err := zip.OpenReader("../Unit example.ZIP")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	if len(reader.File) != 20 {
		t.Fatalf("technical ZIP inventory: got %d entries", len(reader.File))
	}
	var documents int
	for _, entry := range reader.File {
		if !strings.HasPrefix(entry.Name, "U1/") || entry.Name == module.ManifestFileName {
			t.Fatalf("unexpected technical input entry %q", entry.Name)
		}
		if strings.HasSuffix(entry.Name, ".json") {
			documents++
		}
	}
	if documents != 11 {
		t.Fatalf("got %d technical JSON files", documents)
	}
}

func GoldenRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func GoldenLesson(t *testing.T, raw []byte) *lesson.Lesson {
	t.Helper()
	GoldenNoProblems(t, lesson.ValidateJSON(raw, diagnostic.Context{}))
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	GoldenNoProblems(t, lesson.Validate(&value, diagnostic.Context{}))
	return &value
}

func GoldenManifest(t *testing.T, raw []byte) manifest.Manifest {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var shape any
	if err := decoder.Decode(&shape); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("manifest is not one complete JSON value: %v", err)
	}
	GoldenNoProblems(t, manifest.ValidateShape(shape, diagnostic.Context{}))
	var value manifest.Manifest
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	GoldenNoProblems(t, manifest.Validate(value, diagnostic.Context{}))
	return value
}

func GoldenExampleModule(t *testing.T, value manifest.Manifest) *module.Module {
	t.Helper()
	candidate := &module.Module{Manifest: value}
	lessons, sections := value.Course.Lessons, value.Course.Sections
	if value.Course.Unit != nil {
		lessons, sections = &value.Course.Unit.Lessons, &value.Course.Unit.Sections
	}
	entries := append([]manifest.Entry{}, (*lessons)...)
	for _, section := range *sections {
		entries = append(entries, section.Lessons...)
	}
	for _, entry := range entries {
		value := &lesson.Lesson{FormatVersion: "1.0", Document: lesson.Document{
			ID: entry.ID, Version: entry.Version, Title: entry.Title, Subtitle: entry.Subtitle,
			LineCount: entry.LineCount, CreatedAt: entry.CreatedAt, Pages: []lesson.Page{},
		}}
		file, problems := module.NewFile(entry.FileName, value, diagnostic.Context{})
		GoldenNoProblems(t, problems)
		candidate.Files = append(candidate.Files, file)
	}
	return candidate
}

func GoldenDataTypes(t *testing.T, value *lesson.Lesson) {
	t.Helper()
	var data func([]lesson.Data)
	data = func(values []lesson.Data) {
		for _, item := range values {
			switch item.Type {
			case lesson.DataText:
				if _, ok := item.Value.(string); !ok {
					t.Fatalf("text value decoded as %T", item.Value)
				}
			case lesson.DataMedia:
				if _, ok := item.Value.(lesson.EmbeddedFile); !ok {
					t.Fatalf("media value decoded as %T", item.Value)
				}
			case lesson.DataCollection:
				children, ok := item.Value.([]lesson.Data)
				if !ok {
					t.Fatalf("collection value decoded as %T", item.Value)
				}
				data(children)
			default:
				t.Fatalf("unknown typed data %q", item.Type)
			}
		}
	}
	var nodes func([]lesson.Node)
	nodes = func(values []lesson.Node) {
		for _, node := range values {
			data(node.Data)
			nodes(node.Nodes)
		}
	}
	for _, page := range value.Document.Pages {
		nodes(page.Nodes)
	}
}

func GoldenNoProblems(t *testing.T, problems []*diagnostic.Error) {
	t.Helper()
	if len(problems) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", problems)
	}
}

func GoldenRequireCode(t *testing.T, problems []*diagnostic.Error, code string) {
	t.Helper()
	for _, problem := range problems {
		if problem != nil && problem.Code == code {
			return
		}
	}
	t.Fatalf("missing diagnostic %q: %+v", code, problems)
}
