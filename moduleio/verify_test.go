package moduleio_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

func TestVerifyClosedValidArchive(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		unit     bool
		method   uint16
		fileName string
	}{
		{"flat-deflate", false, zip.Deflate, "l007.json"},
		{"unit-sections-store-large-number", true, zip.Store, "l1000.json"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			entries := VerifyFixture(t, scenario.unit)
			entries[0].Name = scenario.fileName
			entries[1].Raw = bytes.ReplaceAll(entries[1].Raw, []byte("l007.json"), []byte(scenario.fileName))
			for i := range entries {
				entries[i].Method = scenario.method
			}
			path := VerifyPath(t, VerifyZIP(t, entries))
			VerifyReadResult(t, path)
		})
	}
}

func TestVerifyRejectsInvalidCandidates(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func([]VerifyEntry) []VerifyEntry
		codes  []string
	}{
		{"missing-manifest", func(e []VerifyEntry) []VerifyEntry { return e[:1] }, []string{moduleio.CodeManifest}},
		{"duplicate-lesson", func(e []VerifyEntry) []VerifyEntry { return append(e, e[0], e[0]) }, []string{moduleio.CodeDuplicateEntry}},
		{"duplicate-manifest", func(e []VerifyEntry) []VerifyEntry { return append(e, e[1]) }, []string{moduleio.CodeDuplicateEntry}},
		{"root-path", func(e []VerifyEntry) []VerifyEntry { e[0].Name = "../l007.json"; return e }, []string{moduleio.CodeEntryName}},
		{"backslash", func(e []VerifyEntry) []VerifyEntry { e[0].Name = `dir\l007.json`; return e }, []string{moduleio.CodeEntryName}},
		{"wrong-case", func(e []VerifyEntry) []VerifyEntry { e[0].Name = "L007.json"; return e }, []string{moduleio.CodeEntryName}},
		{"directory", func(e []VerifyEntry) []VerifyEntry {
			e[0].Name = "dir/"
			e[0].Mode = fs.ModeDir | 0700
			e[0].Raw = nil
			return e
		}, []string{moduleio.CodeEntryType}},
		{"symlink", func(e []VerifyEntry) []VerifyEntry { e[0].Mode = fs.ModeSymlink | 0700; return e }, []string{moduleio.CodeEntryType}},
		{"manifest-bom", func(e []VerifyEntry) []VerifyEntry { e[1].Raw = append([]byte{239, 187, 191}, e[1].Raw...); return e }, []string{moduleio.CodeEncoding}},
		{"lesson-invalid-utf8", func(e []VerifyEntry) []VerifyEntry { e[0].Raw = append(e[0].Raw, 255); return e }, []string{moduleio.CodeEncoding}},
		{"manifest-syntax", func(e []VerifyEntry) []VerifyEntry { e[1].Raw = []byte(`{"course":`); return e }, []string{moduleio.CodeJSON}},
		{"manifest-second-value", func(e []VerifyEntry) []VerifyEntry { e[1].Raw = append(e[1].Raw, []byte(" {}")...); return e }, []string{moduleio.CodeJSON}},
		{"lesson-trailing-garbage", func(e []VerifyEntry) []VerifyEntry { e[0].Raw = append(e[0].Raw, []byte(" !")...); return e }, []string{lesson.CodeJSON}},
		{"lesson-required-field", func(e []VerifyEntry) []VerifyEntry {
			e[0].Raw = bytes.Replace(e[0].Raw, []byte(`"title": "Проверка архива",`), nil, 1)
			return e
		}, []string{lesson.CodeJSON}},
		{"lesson-schema", func(e []VerifyEntry) []VerifyEntry {
			e[0].Raw = bytes.Replace(e[0].Raw, []byte(`"checkable": false`), []byte(`"speaking": false`), 1)
			return e
		}, []string{lesson.CodeContract}},
		{"manifest-shape", func(e []VerifyEntry) []VerifyEntry {
			e[1].Raw = bytes.Replace(e[1].Raw, []byte(`"subtitle": null,`), nil, 1)
			return e
		}, []string{manifest.CodeShape}},
		{"manifest-extra-field", func(e []VerifyEntry) []VerifyEntry {
			e[1].Raw = bytes.Replace(e[1].Raw, []byte(`"formatVersion":`), []byte(`"extra": false, "formatVersion":`), 1)
			return e
		}, []string{manifest.CodeShape}},
		{"manifest-schema", func(e []VerifyEntry) []VerifyEntry {
			e[1].Raw = bytes.Replace(e[1].Raw, []byte(`"formatVersion": "1.0"`), []byte(`"formatVersion": "2.0"`), 1)
			return e
		}, []string{manifest.CodeVersion}},
		{"links", func(e []VerifyEntry) []VerifyEntry {
			e[1].Raw = bytes.ReplaceAll(e[1].Raw, []byte("l007.json"), []byte("l008.json"))
			return e
		}, []string{module.CodeMissingFile, module.CodeUnlistedFile}},
		{"metadata", func(e []VerifyEntry) []VerifyEntry {
			e[1].Raw = bytes.Replace(e[1].Raw, []byte("Проверка архива"), []byte("Другое название"), 1)
			return e
		}, []string{module.CodeMetadata}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			entries := scenario.change(VerifyFixture(t, false))
			path := VerifyPath(t, VerifyZIP(t, entries))
			problems := VerifyReadResult(t, path, scenario.codes...)
			if scenario.name == "wrong-case" {
				problem := VerifyHasCode(t, problems, moduleio.CodeEntryName)
				var owner *diagnostic.Error
				if !errors.As(problem.Cause, &owner) || owner.Code != identity.CodeInvalidFileName {
					t.Fatal("identity Cause was lost")
				}
			}
			if scenario.name == "duplicate-lesson" {
				problem := VerifyHasCode(t, problems, moduleio.CodeDuplicateEntry)
				if len(problem.Related) != 3 || len(problem.Details) < 3 {
					t.Fatalf("all physical duplicates and ZIP positions are required: %#v", problem)
				}
				for _, source := range problem.Related {
					if source.Path != path || source.FileName != "l007.json" {
						t.Fatalf("wrong duplicate source: %#v", source)
					}
				}
			}
			if scenario.name == "lesson-schema" {
				problem := VerifyHasCode(t, problems, lesson.CodeContract)
				if problem.Context.LessonID != "QA7" || problem.Context.PageID != "qa7-p001" || len(problem.Details) == 0 {
					t.Fatalf("lesson owner context or pointer lost: %#v", problem)
				}
			}
		})
	}
}

func TestVerifyRetainsAllIndependentErrors(t *testing.T) {
	entries := VerifyFixture(t, false)
	entries[0].Raw = bytes.Replace(entries[0].Raw, []byte(`"pages": [{`), []byte(`"pages": null, "ignored": [{`), 1)
	entries[1].Raw = bytes.Replace(entries[1].Raw, []byte(`"subtitle": null,`), nil, 1)
	entries = append(entries, VerifyEntry{Name: "l1000.json", Raw: []byte(`{`), Method: zip.Deflate})
	problems := VerifyReadResult(t, VerifyPath(t, VerifyZIP(t, entries)), lesson.CodeJSON, manifest.CodeShape)
	for _, fileName := range []string{"l007.json", "l1000.json"} {
		found := false
		for _, problem := range problems {
			if problem.Context.FileName == fileName {
				found = true
			}
		}
		if !found {
			t.Fatalf("independent failure in %s was lost", fileName)
		}
	}
}

func TestVerifyOpenAndArchiveFailures(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "нет архива.tmp")
		problems := VerifyReadResult(t, path, moduleio.CodeOpen)
		if !errors.Is(VerifyHasCode(t, problems, moduleio.CodeOpen), os.ErrNotExist) {
			t.Fatal("missing filesystem Cause")
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("Verify created missing path")
		}
	})
	t.Run("invalid-zip", func(t *testing.T) {
		problems := VerifyReadResult(t, VerifyPath(t, []byte("not a ZIP")), moduleio.CodeArchive)
		if !errors.Is(VerifyHasCode(t, problems, moduleio.CodeArchive), zip.ErrFormat) {
			t.Fatal("missing ZIP Cause")
		}
	})
	t.Run("empty-zip", func(t *testing.T) {
		VerifyReadResult(t, VerifyPath(t, VerifyZIP(t, nil)), moduleio.CodeManifest)
	})
}
