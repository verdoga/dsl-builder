package module_test

import (
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func TestModelManifestFileName(t *testing.T) {
	const fileName string = module.ManifestFileName
	if fileName != "manifest.json" {
		t.Fatalf("ManifestFileName = %q, want manifest.json", fileName)
	}
}

func TestModelStoresUnfinishedCandidate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []*module.File
	}{
		{name: "nil files"},
		{name: "empty files", files: []*module.File{}},
		{name: "unfinished file", files: []*module.File{nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := module.Module{
				Manifest: manifest.Manifest{FormatVersion: "candidate", Course: manifest.Course{Title: "Курс"}},
				Files:    tc.files,
			}
			// These assignments require the actual public dependency types.
			var storedManifest manifest.Manifest = value.Manifest
			var storedFiles []*module.File = value.Files
			if storedManifest.FormatVersion != "candidate" || storedManifest.Course.Title != "Курс" {
				t.Fatalf("unfinished manifest changed: %+v", storedManifest)
			}
			if (storedFiles == nil) != (tc.files == nil) || len(storedFiles) != len(tc.files) {
				t.Fatalf("unfinished Files changed: %#v", storedFiles)
			}
			for i, file := range tc.files {
				if storedFiles[i] != file {
					t.Fatalf("Files[%d] changed", i)
				}
			}
		})
	}
}

func TestModelFilesPreserveOrderAndRepeatedPointers(t *testing.T) {
	first := modelTestFile(t, "l001.json", "FIRST")
	second := modelTestFile(t, "l002.json", "SECOND")
	want := []*module.File{second, first, second, nil, first}
	value := module.Module{Files: want}
	if len(value.Files) != len(want) {
		t.Fatalf("Files length = %d, want %d", len(value.Files), len(want))
	}
	for i, file := range want {
		if value.Files[i] != file {
			t.Fatalf("Files[%d] = %p, want %p", i, value.Files[i], file)
		}
	}
	// app owns the separate slice for a candidate and may share immutable Files.
	candidate := module.Module{Manifest: value.Manifest, Files: append([]*module.File(nil), value.Files...)}
	candidate.Files[0] = first
	if value.Files[0] != second || candidate.Files[1] != value.Files[1] {
		t.Fatal("candidate did not retain an independent slice of the same File pointers")
	}
}

func TestModelFileOrderDoesNotDefineManifestPlacement(t *testing.T) {
	first := modelTestFile(t, "l001.json", "FIRST")
	second := modelTestFile(t, "l002.json", "SECOND")
	one, two := "01", "2"
	entries := []manifest.Entry{
		catalog.EntryFromLesson(second.Lesson().Document, second.FileName(), &one),
		catalog.EntryFromLesson(first.Lesson().Document, first.FileName(), &two),
	}
	sections := []manifest.Section{}
	value := module.Module{
		Manifest: manifest.Manifest{
			FormatVersion: manifest.FormatVersion,
			CreatedAt:     "2026-11-07T00:00:00Z",
			Course:        manifest.Course{Title: "Курс", Lessons: &entries, Sections: &sections},
		},
		Files: []*module.File{first, second},
	}
	if problems := manifest.Validate(value.Manifest, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("manifest fixture is invalid: %v", problems)
	}
	placement, problems := catalog.Import(value.Manifest, diagnostic.Context{})
	if len(problems) != 0 || placement == nil {
		t.Fatalf("Import: state=%v, problems=%v", placement, problems)
	}
	if len(placement.Items) != 2 || placement.Items[0].Entry.FileName != "l002.json" || placement.Items[1].Entry.FileName != "l001.json" {
		t.Fatalf("manifest placement changed: %+v", placement.Items)
	}
	if value.Files[0] != first || value.Files[1] != second || (*value.Manifest.Course.Lessons)[0].Order == nil || *(*value.Manifest.Course.Lessons)[0].Order != "01" {
		t.Fatal("storing the Module changed file order or manifest order spelling")
	}
}

func modelTestFile(t *testing.T, fileName, id string) *module.File {
	t.Helper()
	value := &lesson.Lesson{
		FormatVersion: lesson.FormatVersion,
		Document: lesson.Document{
			ID: id, Version: json.Number("1"), Title: "Урок " + id,
			LineCount: json.Number("0"), CreatedAt: "2026-11-04T09:00:00Z", Pages: []lesson.Page{},
		},
	}
	file, problems := module.NewFile(fileName, value, diagnostic.Context{Path: "Вход/" + id + ".json"})
	if len(problems) != 0 || file == nil {
		t.Fatalf("NewFile(%q): file=%v, problems=%v", fileName, file, problems)
	}
	return file
}
