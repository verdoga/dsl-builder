package module

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
)

func ValidateLessonFixture(t *testing.T, id string) *lesson.Lesson {
	t.Helper()
	role := lesson.RoleBody
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: id, Title: "Урок «" + id + "»", Version: "1", LineCount: "12",
		CreatedAt: "2026-11-04T09:00:00Z", Pages: []lesson.Page{{
			ID: strings.ToLower(id) + "-p001", Nodes: []lesson.Node{{
				ID: strings.ToLower(id) + "-n001", Type: lesson.NodeText,
				Data: []lesson.Data{{Type: lesson.DataText, Role: &role,
					Value: "Текст\n\n_____ {answer}", Answers: []string{}, Hint: ""}},
				Nodes: []lesson.Node{},
			}},
		}},
	}}
	ValidateWantCodes(t, lesson.Validate(value, diagnostic.Context{}))
	return value
}

func ValidateFileFixture(t *testing.T, name, id string) *File {
	t.Helper()
	file, problems := NewFile(name, ValidateLessonFixture(t, id), diagnostic.Context{Path: "исходники/" + id + ".json"})
	ValidateWantCodes(t, problems)
	if file == nil {
		t.Fatal("NewFile returned nil without diagnostics")
	}
	return file
}

func ValidateItemFixture(t *testing.T, file *File, weight string) catalog.Item {
	t.Helper()
	entry := catalog.EntryFromLesson(file.Lesson().Document, file.FileName(), &weight)
	return catalog.Item{Entry: entry, Context: diagnostic.Context{
		Path: "модуль.zip", FileName: entry.FileName, LessonID: entry.ID,
	}}
}

func ValidateFlatFixture(t *testing.T, entries ...manifest.Entry) manifest.Manifest {
	t.Helper()
	sections := []manifest.Section{}
	return manifest.Manifest{FormatVersion: manifest.FormatVersion, CreatedAt: "2026-11-07T12:00:00Z",
		Course: manifest.Course{Title: "Курс", Lessons: &entries, Sections: &sections}}
}

func ValidateWantCodes(t *testing.T, problems []*diagnostic.Error, want ...string) {
	t.Helper()
	got := make([]string, 0, len(problems))
	for _, problem := range problems {
		if problem == nil {
			t.Fatal("nil diagnostic")
		}
		got = append(got, problem.Code)
		if !strings.ContainsFunc(problem.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
			t.Errorf("diagnostic %s lacks a Russian reason: %q", problem.Code, problem.Reason)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("codes = %v, want %v; diagnostics: %v", got, want, problems)
	}
}

func ValidateWantRelated(t *testing.T, problem *diagnostic.Error, want ...diagnostic.Context) {
	t.Helper()
	if !slices.Equal(problem.Related, want) {
		t.Errorf("%s Related = %+v, want %+v", problem.Code, problem.Related, want)
	}
}

func ValidateSnapshot(t *testing.T, value *Module) []byte {
	t.Helper()
	snapshot := struct {
		Manifest  manifest.Manifest
		Names     []string
		Lessons   []*lesson.Lesson
		Originals [][]byte
		Contexts  []diagnostic.Context
	}{Manifest: value.Manifest}
	for _, file := range value.Files {
		snapshot.Names = append(snapshot.Names, file.FileName())
		snapshot.Lessons = append(snapshot.Lessons, file.Lesson())
		snapshot.Originals = append(snapshot.Originals, file.OriginalBytes())
		snapshot.Contexts = append(snapshot.Contexts, file.Context())
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestValidateNilAndEmpty(t *testing.T) {
	context := diagnostic.Context{Path: "пустой.zip"}
	problems := Validate(nil, context)
	ValidateWantCodes(t, problems, CodeModule)
	if problems[0].Context != context {
		t.Errorf("nil Module context = %+v", problems[0].Context)
	}
	file := ValidateFileFixture(t, "l001.json", "Alpha")
	entry := ValidateItemFixture(t, file, "1").Entry
	for _, files := range [][]*File{nil, {}} {
		ValidateWantCodes(t, Validate(&Module{Manifest: ValidateFlatFixture(t, entry), Files: files}, context), CodeEmpty)
	}
}

func TestValidateSchemaStopsPlacementAndLinks(t *testing.T) {
	first := ValidateFileFixture(t, "l001.json", "Alpha")
	second := ValidateFileFixture(t, "l002.json", "Beta")
	a, b := ValidateItemFixture(t, first, "20"), ValidateItemFixture(t, second, "10")
	a.Entry.Title = "Несогласованное название"
	b.Entry.FileName = "l099.json"
	value := &Module{Manifest: ValidateFlatFixture(t, a.Entry, b.Entry), Files: []*File{second, first}}
	value.Manifest.FormatVersion = "9.0"
	context := diagnostic.Context{Path: "архив.zip", FileName: ManifestFileName}
	want := manifest.Validate(value.Manifest, context)
	ValidateWantCodes(t, want, manifest.CodeVersion)
	got := Validate(value, context)
	ValidateWantCodes(t, got, manifest.CodeVersion)
	if got[0].Reason != want[0].Reason || !slices.Equal(got[0].Details, want[0].Details) {
		t.Errorf("schema owner diagnostic changed: got %+v, want %+v", got[0], want[0])
	}
}

func TestValidatePlacementErrorsStillAllowLinks(t *testing.T) {
	first := ValidateFileFixture(t, "l001.json", "Alpha")
	second := ValidateFileFixture(t, "l002.json", "Beta")
	a, b := ValidateItemFixture(t, first, "1"), ValidateItemFixture(t, second, "01")
	a.Entry.Title = "Другое название"
	b.Entry.FileName = "l099.json"
	value := &Module{Manifest: ValidateFlatFixture(t, a.Entry, b.Entry), Files: []*File{second, first}}
	ValidateWantCodes(t, manifest.Validate(value.Manifest, diagnostic.Context{}))
	before := ValidateSnapshot(t, value)
	problems := Validate(value, diagnostic.Context{Path: "модуль.zip"})
	ValidateWantCodes(t, problems, catalog.CodeWeightConflict, CodeMissingFile, CodeUnlistedFile, CodeMetadata)
	if !bytes.Equal(before, ValidateSnapshot(t, value)) || value.Files[0] != second || value.Files[1] != first {
		t.Fatal("Validate changed a rejected module")
	}
}

func TestValidateDelegatesPlacementWithoutRepair(t *testing.T) {
	a := ValidateFileFixture(t, "l001.json", "Alpha")
	b := ValidateFileFixture(t, "l002.json", "Beta")
	for _, scenario := range []struct {
		name    string
		weights []string
		want    string
	}{
		{"descending", []string{"20", "10"}, catalog.CodeLessonOrder},
		{"numeric duplicate", []string{"01", "1"}, catalog.CodeWeightConflict},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			value := &Module{Files: []*File{b, a}, Manifest: ValidateFlatFixture(t,
				ValidateItemFixture(t, a, scenario.weights[0]).Entry, ValidateItemFixture(t, b, scenario.weights[1]).Entry)}
			before := ValidateSnapshot(t, value)
			ValidateWantCodes(t, manifest.Validate(value.Manifest, diagnostic.Context{}))
			ValidateWantCodes(t, Validate(value, diagnostic.Context{}), scenario.want)
			if !bytes.Equal(before, ValidateSnapshot(t, value)) {
				t.Fatal("Validate sorted entries or normalized weights")
			}
		})
	}
	t.Run("single null weight is schema valid but incomplete", func(t *testing.T) {
		entry := ValidateItemFixture(t, a, "1").Entry
		entry.Order = nil
		value := &Module{Files: []*File{a}, Manifest: ValidateFlatFixture(t, entry)}
		ValidateWantCodes(t, manifest.Validate(value.Manifest, diagnostic.Context{}))
		ValidateWantCodes(t, Validate(value, diagnostic.Context{}), catalog.CodeMissingWeight)
		if (*value.Manifest.Course.Lessons)[0].Order != nil {
			t.Fatal("Validate synthesized a missing weight")
		}
	})
}

func TestValidateMixedPlacementAndLinksAreIndependent(t *testing.T) {
	a := ValidateFileFixture(t, "l001.json", "Alpha")
	b := ValidateFileFixture(t, "l002.json", "Beta")
	aEntry, bEntry := ValidateItemFixture(t, a, "1").Entry, ValidateItemFixture(t, b, "1").Entry
	bEntry.Title = "Другое название"
	value := &Module{Files: []*File{b, a}, Manifest: ValidateFlatFixture(t, aEntry)}
	*value.Manifest.Course.Sections = []manifest.Section{{Title: "Секция", Lessons: []manifest.Entry{bEntry}}}
	ValidateWantCodes(t, manifest.Validate(value.Manifest, diagnostic.Context{}))
	ValidateWantCodes(t, Validate(value, diagnostic.Context{}), catalog.CodeMixedSections, catalog.CodeUnassigned, CodeMetadata)
}

func TestValidateValidFlatAndUnitPreserveLoadedBytes(t *testing.T) {
	for _, unit := range []bool{false, true} {
		name := "flat"
		if unit {
			name = "unit with sections"
		}
		t.Run(name, func(t *testing.T) {
			original := ValidateLessonFixture(t, "Alpha")
			raw, err := json.MarshalIndent(original, "", "   ")
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, '\n', '\t')
			context := diagnostic.Context{Path: "исходный.zip", FileName: "l003.json", LessonID: "Alpha"}
			ValidateWantCodes(t, lesson.ValidateJSON(raw, context))
			var decoded lesson.Lesson
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			a, problems := LoadedFile("l003.json", &decoded, raw, context)
			ValidateWantCodes(t, problems)
			b := ValidateFileFixture(t, "l001.json", "Beta")
			aEntry, bEntry := ValidateItemFixture(t, a, "0001").Entry, ValidateItemFixture(t, b, "002").Entry
			value := &Module{Files: []*File{b, a}, Manifest: ValidateFlatFixture(t, aEntry, bEntry)}
			if unit {
				value.Manifest.Course = manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит", Lessons: []manifest.Entry{},
					Sections: []manifest.Section{{Title: "Z", Lessons: []manifest.Entry{aEntry}}, {Title: "A", Lessons: []manifest.Entry{bEntry}}}}}
			}
			before := ValidateSnapshot(t, value)
			ValidateWantCodes(t, Validate(value, diagnostic.Context{Path: "кандидат.zip"}))
			if !bytes.Equal(before, ValidateSnapshot(t, value)) || !bytes.Equal(a.OriginalBytes(), raw) || value.Files[0] != b || value.Files[1] != a {
				t.Fatal("Validate changed order, lesson, metadata or exact loaded bytes")
			}
		})
	}
}
