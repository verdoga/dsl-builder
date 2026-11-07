package moduleio_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

func TestGoldenSevenLessonZIPAndManifestOrdersRemainIndependent(t *testing.T) {
	entries := []manifest.Entry{}
	sections := []manifest.Section{}
	value := &module.Module{Manifest: manifest.Manifest{
		FormatVersion: "1.0", CreatedAt: "2026-11-04T09:00:00Z",
		Course: manifest.Course{Title: "Семь готовых уроков", Lessons: &entries, Sections: &sections},
	}}
	var sources []GoldenEntry
	for i := 1; i <= 7; i++ {
		raw := GoldenRead(t, fmt.Sprintf("../golden/%d. lesson.json", i))
		lesson := GoldenLesson(t, raw)
		name := fmt.Sprintf("l%03d.json", i)
		file, problems := module.NewFile(name, lesson, diagnostic.Context{})
		GoldenNoProblems(t, problems)
		value.Files = append([]*module.File{file}, value.Files...)
		entries = append(entries, GoldenEntryFor(t, name, lesson.Document, fmt.Sprintf("%04d", i)))
		sources = append([]GoldenEntry{{name, raw}}, sources...)
	}
	GoldenNoProblems(t, module.Validate(value, diagnostic.Context{}))
	input := GoldenZIP(t, append(sources, GoldenEntry{module.ManifestFileName, GoldenJSON(t, value.Manifest)}))
	loaded := GoldenLoaded(t, input)
	var expectedRaw [][]byte
	for i, source := range sources {
		if loaded.Files[i].FileName() != source.name {
			t.Fatal("Read sorted Files by manifest instead of ZIP order")
		}
		GoldenEqualLesson(t, loaded.Files[i].Lesson(), value.Files[i].Lesson())
		expectedRaw = append(expectedRaw, source.raw)
	}
	GoldenAssertOutput(t, GoldenWritten(t, loaded), value, expectedRaw)
}

func TestGoldenFullManifestUnitExampleStorage(t *testing.T) {
	example := GoldenManifest(t, GoldenRead(t, "../Manifest unit example.json"))
	value := GoldenExampleModule(t, example)
	GoldenNoProblems(t, module.Validate(value, diagnostic.Context{}))
	var expectedRaw [][]byte
	for _, file := range value.Files {
		raw, err := json.MarshalIndent(file.Lesson(), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		expectedRaw = append(expectedRaw, raw)
	}
	path := GoldenWritten(t, value)
	GoldenAssertOutput(t, path, value, expectedRaw)
	loaded := GoldenLoaded(t, path)
	if len(loaded.Files) != 11 || len(loaded.Manifest.Course.Unit.Sections) != 3 {
		t.Fatal("full unit example was shortened during storage")
	}
	for i := range value.Files {
		GoldenEqualLesson(t, loaded.Files[i].Lesson(), value.Files[i].Lesson())
	}
}

func TestGoldenFlatManifestNullWeightIsNotAReadyModule(t *testing.T) {
	raw := GoldenRead(t, "../Manifest flat example.json")
	value := GoldenExampleModule(t, GoldenManifest(t, raw))
	var buffer bytes.Buffer
	GoldenRequireCode(t, moduleio.Write(&buffer, value, diagnostic.Context{}), "catalog.missing_weight")
	if buffer.Len() != 0 {
		t.Fatal("Write emitted bytes for an unfinished final weight")
	}
	lessonRaw, err := json.Marshal(value.Files[0].Lesson())
	if err != nil {
		t.Fatal(err)
	}
	path := GoldenZIP(t, []GoldenEntry{{module.ManifestFileName, raw}, {"l001.json", lessonRaw}})
	loaded, problems := moduleio.Read(path)
	if loaded != nil {
		t.Fatal("schema-valid flat example bypassed final weight validation")
	}
	GoldenRequireCode(t, problems, "catalog.missing_weight")
	GoldenRequireCode(t, moduleio.Verify(path), "catalog.missing_weight")
}

func TestGoldenSixManifestMetadataMismatchesBlockStorage(t *testing.T) {
	cases := []struct {
		field  string
		change func(*manifest.Entry)
	}{
		{"id", func(e *manifest.Entry) { e.ID = strings.ToLower(e.ID) }},
		{"title", func(e *manifest.Entry) { e.Title = "Другой заголовок" }},
		{"subtitle", func(e *manifest.Entry) { e.Subtitle = nil }},
		{"lineCount", func(e *manifest.Entry) { e.LineCount = "9007199254740996.0" }},
		{"createdAt", func(e *manifest.Entry) { e.CreatedAt = "2020-02-29T12:34:57Z" }},
		{"version", func(e *manifest.Entry) { e.Version = "9007199254740994e0" }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			raw := GoldenRead(t, "testdata/golden/storage.lesson.json")
			value := GoldenPrepared(t, raw)
			tc.change(&(*value.Manifest.Course.Lessons)[0])
			GoldenNoProblems(t, manifest.Validate(value.Manifest, diagnostic.Context{}))
			GoldenRequireCode(t, module.Validate(value, diagnostic.Context{}), module.CodeMetadata)
			var buffer bytes.Buffer
			GoldenRequireCode(t, moduleio.Write(&buffer, value, diagnostic.Context{}), module.CodeMetadata)
			if buffer.Len() != 0 {
				t.Fatal("invalid metadata caused partial output")
			}
			path := GoldenZIP(t, []GoldenEntry{
				{module.ManifestFileName, GoldenJSON(t, value.Manifest)}, {"l001.json", raw},
			})
			loaded, problems := moduleio.Read(path)
			if loaded != nil {
				t.Fatal("Read returned a module with conflicting metadata")
			}
			GoldenRequireCode(t, problems, module.CodeMetadata)
			for _, problem := range problems {
				if problem.Code == module.CodeMetadata && (problem.Context.Path != path || problem.Context.FileName != "l001.json") {
					t.Fatal("metadata owner error lost archive/entry context")
				}
			}
			GoldenRequireCode(t, moduleio.Verify(path), module.CodeMetadata)
		})
	}
}

func TestGoldenNestedTechnicalZIPCannotBeReadAsReadyModule(t *testing.T) {
	path := "../Unit example.ZIP"
	before := GoldenRead(t, path)
	value, problems := moduleio.Read(path)
	if value != nil {
		t.Fatal("nested technical input was accepted as a finished flat module")
	}
	GoldenRequireCode(t, problems, moduleio.CodeManifest)
	GoldenRequireCode(t, moduleio.Verify(path), moduleio.CodeManifest)
	if !bytes.Equal(before, GoldenRead(t, path)) {
		t.Fatal("rejection changed the technical input ZIP")
	}
}
