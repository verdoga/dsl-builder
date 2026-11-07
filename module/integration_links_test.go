package module_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func integrationLinksSetup(t *testing.T) ([]*module.File, []catalog.Item) {
	t.Helper()
	var files []*module.File
	var items []catalog.Item
	for index, id := range []string{"Link+A", "Link:B", "Link-C"} {
		name := []string{"l003.json", "l001.json", "l007.json"}[index]
		weight := []string{"01", "02", "03"}[index]
		context := diagnostic.Context{Path: "source-" + id + ".json", FileName: name, LessonID: id}
		value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
			ID: id, Title: "Название " + id, Version: "9007199254740993", LineCount: "9007199254740995",
			CreatedAt: "2026-11-01T01:02:03Z", Pages: []lesson.Page{},
		}}
		if problems := lesson.Validate(value, context); len(problems) != 0 {
			t.Fatalf("lesson setup: %v", problems)
		}
		file, problems := module.NewFile(name, value, context)
		if len(problems) != 0 || file == nil {
			t.Fatalf("file setup: %v", problems)
		}
		files = append(files, file)
		items = append(items, catalog.Item{Entry: catalog.EntryFromLesson(value.Document, name, &weight), Context: context})
	}
	return files, items
}

func integrationLinksCodes(t *testing.T, problems []*diagnostic.Error) []string {
	t.Helper()
	codes := make([]string, len(problems))
	for index, problem := range problems {
		if problem == nil || problem.Reason == "" {
			t.Fatal("nil diagnostic or empty reason")
		}
		codes[index] = problem.Code
	}
	return codes
}

func integrationLinksJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestIntegrationLinksAllSixMetadataFieldsAreExactAndOrdered(t *testing.T) {
	files, items := integrationLinksSetup(t)
	fields := []string{"id", "title", "subtitle", "lineCount", "createdAt", "version"}
	changes := []func(*manifest.Entry){
		func(e *manifest.Entry) { e.ID = "link+a" },
		func(e *manifest.Entry) { e.Title = "Другое название" },
		func(e *manifest.Entry) { empty := ""; e.Subtitle = &empty },
		func(e *manifest.Entry) { e.LineCount = "9007199254740994" },
		func(e *manifest.Entry) { e.CreatedAt = "2026-11-02T01:02:03Z" },
		func(e *manifest.Entry) { e.Version = "9007199254740992" },
	}
	for selected := -1; selected < len(changes); selected++ {
		entry := items[0].Entry
		var expected []string
		for index, change := range changes {
			if selected == -1 || selected == index {
				change(&entry)
				expected = append(expected, fields[index])
			}
		}
		candidate := []catalog.Item{{Entry: entry, Context: diagnostic.Context{Path: "манифест.zip", FileName: entry.FileName, LessonID: entry.ID}}}
		before := integrationLinksJSON(t, candidate)
		problems := module.ValidateLinks(files[:1], candidate, diagnostic.Context{Path: "модуль.zip"})
		if len(problems) != len(expected) {
			t.Fatalf("case %d: want fields %v, got %v", selected, expected, problems)
		}
		for index, problem := range problems {
			if problem.Code != module.CodeMetadata || len(problem.Details) < 3 {
				t.Fatalf("metadata must identify field and both values: %+v", problem)
			}
			fieldFound := false
			for _, detail := range problem.Details {
				fieldFound = fieldFound || strings.Contains(strings.ToLower(detail.Value), strings.ToLower(expected[index]))
				if strings.Contains(detail.Value, "AAEC//4=") {
					t.Fatal("diagnostic leaked embedded content")
				}
			}
			if !fieldFound {
				t.Fatalf("metadata field order must follow Entry: want %q, got %+v", expected[index], problem.Details)
			}
			for _, path := range []string{files[0].Context().Path, candidate[0].Context.Path} {
				found := false
				for _, related := range problem.Related {
					found = found || related.Path == path
				}
				if !found {
					t.Fatalf("metadata mismatch omitted participant %q: %+v", path, problem.Related)
				}
			}
		}
		if !bytes.Equal(before, integrationLinksJSON(t, candidate)) {
			t.Fatal("ValidateLinks repaired its input")
		}
	}
}

func TestIntegrationLinksIntegerEqualityUsesMathematicalValues(t *testing.T) {
	for _, pair := range [][2]string{{"1", "1.0"}, {"1", "1e0"}, {"9007199254740993", "9007199254740993.00"}, {"123456789012345678901234567890", "12345678901234567890123456789e1"}, {"1e100000000000000000000000000", "10e99999999999999999999999999"}} {
		files, items := integrationLinksSetup(t)
		value := files[0].Lesson()
		value.Document.LineCount, value.Document.Version = json.Number(pair[0]), json.Number(pair[0])
		file, problems := module.NewFile(files[0].FileName(), value, files[0].Context())
		if len(problems) != 0 || file == nil {
			t.Fatalf("valid exact integer rejected: %v", problems)
		}
		entry := catalog.EntryFromLesson(value.Document, file.FileName(), items[0].Entry.Order)
		entry.LineCount, entry.Version = json.Number(pair[1]), json.Number(pair[1])
		if problems := module.ValidateLinks([]*module.File{file}, []catalog.Item{{Entry: entry}}, diagnostic.Context{}); len(problems) != 0 {
			t.Fatalf("equal integer spellings %q and %q: %v", pair[0], pair[1], problems)
		}
	}
	files, items := integrationLinksSetup(t)
	value := files[0].Lesson()
	value.Document.LineCount = "-0.0e100000000000000000000"
	file, problems := module.NewFile(files[0].FileName(), value, files[0].Context())
	if len(problems) != 0 || file == nil {
		t.Fatalf("valid zero: %v", problems)
	}
	items[0].Entry.LineCount = "0"
	if problems := module.ValidateLinks([]*module.File{file}, items[:1], diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("mathematical zero compared lexically: %v", problems)
	}
}

func TestIntegrationLinksCoverageAndGlobalEntryKeysKeepEveryParticipant(t *testing.T) {
	files, items := integrationLinksSetup(t)
	items[0].Entry.FileName = "l099.json"
	problems := module.ValidateLinks(files, items, diagnostic.Context{Path: "модуль.zip"})
	if !slices.Equal(integrationLinksCodes(t, problems), []string{module.CodeMissingFile, module.CodeUnlistedFile}) || problems[0].Context.FileName != "l099.json" || problems[1].Context.FileName != files[0].FileName() {
		t.Fatalf("coverage must check Items then Files: %+v", problems)
	}
	for _, duplicateID := range []bool{true, false} {
		files, items = integrationLinksSetup(t)
		for index := range items {
			section := []string{"Секция", "секция", "Другая"}[index]
			items[index].Section = &section
			if duplicateID {
				items[index].Entry.ID = []string{"Link+A", "link+a", "LINK+A"}[index]
			} else {
				items[index].Entry.FileName = files[0].FileName()
			}
		}
		before := integrationLinksJSON(t, items)
		problems = module.ValidateLinks(files, items, diagnostic.Context{})
		code := module.CodeDuplicateReference
		if duplicateID {
			code = module.CodeDuplicateID
		}
		count := 0
		for _, problem := range problems {
			if problem.Code != code {
				continue
			}
			count++
			if len(problem.Related) != len(items) {
				t.Fatalf("duplicate across sections omitted participants: %+v", problem.Related)
			}
			for index, related := range problem.Related {
				if related.Path != items[index].Context.Path || related.FileName != items[index].Entry.FileName || related.LessonID != items[index].Entry.ID {
					t.Fatalf("participant context/order: %+v", problem.Related)
				}
			}
		}
		if count == 0 || duplicateID && count != 2 || !bytes.Equal(before, integrationLinksJSON(t, items)) {
			t.Fatalf("duplicate count or input mutation: %v", problems)
		}
		if !duplicateID && slices.Contains(integrationLinksCodes(t, problems), module.CodeMetadata) {
			t.Fatal("ambiguous reference selected an arbitrary metadata pair")
		}
	}
}

func TestIntegrationLinksValidateDelegatesSchemaPlacementAndKeepsIndependentErrors(t *testing.T) {
	for _, scenario := range []string{"missing-weight", "conflicting-weights", "unsorted", "mixed", "duplicate-section", "schema"} {
		t.Run(scenario, func(t *testing.T) {
			files, items := integrationLinksSetup(t)
			files = files[:2]
			entries := []manifest.Entry{items[0].Entry, items[1].Entry}
			sections := []manifest.Section{}
			value := manifest.Manifest{FormatVersion: manifest.FormatVersion, CreatedAt: "2026-11-07T00:00:00Z", Course: manifest.Course{Title: "Курс", Lessons: &entries, Sections: &sections}}
			switch scenario {
			case "missing-weight":
				entries = entries[:1]
				files = files[:1]
				entries[0].Order = nil
			case "conflicting-weights":
				weight := "001"
				entries[1].Order = &weight
			case "unsorted":
				entries[0], entries[1] = entries[1], entries[0]
			case "mixed":
				sections = []manifest.Section{{Title: "Секция", Lessons: entries[1:]}}
				entries = entries[:1]
			case "duplicate-section":
				sections = []manifest.Section{{Title: "Секция", Lessons: entries[:1]}, {Title: "Секция", Lessons: entries[1:]}}
				entries = []manifest.Entry{}
			case "schema":
				value.CreatedAt = "2026-02-30T00:00:00Z"
			}
			if len(entries) > 0 {
				entries[0].Title = "Рассогласование"
			} else {
				sections[0].Lessons[0].Title = "Рассогласование"
			}
			context := diagnostic.Context{Path: "модуль.zip", FileName: module.ManifestFileName}
			owner := manifest.Validate(value, context)
			if len(owner) == 0 {
				_, owner = catalog.Import(value, context)
			}
			if len(owner) == 0 {
				t.Fatal("scenario did not reach a real dependency error")
			}
			candidate := &module.Module{Manifest: value, Files: files}
			before := integrationLinksJSON(t, value)
			problems := module.Validate(candidate, context)
			codes := integrationLinksCodes(t, problems)
			if len(problems) < len(owner) || !slices.Equal(codes[:len(owner)], integrationLinksCodes(t, owner)) {
				t.Fatalf("dependency error order/codes changed: %v, owner %v", problems, owner)
			}
			for index := range owner {
				if problems[index].Reason != owner[index].Reason {
					t.Fatal("dependency reason was replaced")
				}
			}
			if scenario != "schema" && !slices.Contains(codes, module.CodeMetadata) || !bytes.Equal(before, integrationLinksJSON(t, value)) {
				t.Fatalf("independent links hidden or invalid placement repaired: %v", problems)
			}
		})
	}
}

func TestIntegrationLinksFilesAndManifestBranchesFailIndependently(t *testing.T) {
	files, items := integrationLinksSetup(t)
	state := &catalog.State{CourseTitle: "Курс", Items: items}
	exported, problems := catalog.Export(state, "2026-11-07T00:00:00Z", diagnostic.Context{})
	if len(problems) != 0 || exported == nil {
		t.Fatalf("Export setup: %v", problems)
	}
	exported.FormatVersion = "wrong"
	candidate := &module.Module{Manifest: *exported, Files: append([]*module.File{nil}, files...)}
	before := integrationLinksJSON(t, candidate.Manifest)
	context := diagnostic.Context{Path: "архив.zip", FileName: module.ManifestFileName}
	problems = module.Validate(candidate, context)
	codes := integrationLinksCodes(t, problems)
	if len(codes) != 2 || codes[0] != module.CodeFile || codes[1] != manifest.CodeVersion {
		t.Fatalf("file failure hid schema failure or allowed dependent links: %v", problems)
	}
	if !bytes.Equal(before, integrationLinksJSON(t, candidate.Manifest)) || candidate.Files[0] != nil || len(candidate.Files) != 4 {
		t.Fatal("failed validation repaired a partial candidate")
	}
	problems = module.Validate(nil, context)
	if !slices.Equal(integrationLinksCodes(t, problems), []string{module.CodeModule}) {
		t.Fatalf("nil module: %v", problems)
	}
	entry := items[0].Entry
	entry.Order = nil
	if problems := module.ValidateLinks(files[:1], []catalog.Item{{Entry: entry}}, context); len(problems) != 0 {
		t.Fatalf("links must leave final weight checks with catalog: %v", problems)
	}
}
