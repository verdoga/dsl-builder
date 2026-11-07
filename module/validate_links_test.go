package module

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func ValidateMetadataDetail(t *testing.T, problem *diagnostic.Error, field string, values ...string) {
	t.Helper()
	foundField := false
	for _, detail := range problem.Details {
		if strings.Contains(strings.ToLower(detail.Value), strings.ToLower(field)) {
			foundField = true
		}
	}
	if !foundField {
		t.Errorf("metadata diagnostic lacks field %s: %+v", field, problem.Details)
	}
	for _, value := range values {
		found := false
		for _, detail := range problem.Details {
			if detail.Value == value || detail.Value == strconv.Quote(value) {
				found = true
			}
		}
		if !found {
			t.Errorf("metadata %s lacks exact value %q: %+v", field, value, problem.Details)
		}
	}
}

func TestValidateLinksValidArbitraryFileAndSectionOrder(t *testing.T) {
	a := ValidateFileFixture(t, "l1000.json", "Alpha")
	b := ValidateFileFixture(t, "l001.json", "Beta")
	c := ValidateFileFixture(t, "l003.json", "Gamma")
	items := []catalog.Item{ValidateItemFixture(t, a, "0009"), ValidateItemFixture(t, b, "1"), ValidateItemFixture(t, c, "1")}
	sectionA, sectionB := "Z", "A"
	items[0].Section, items[1].Section, items[2].Section = &sectionA, &sectionB, &sectionA
	items[0].RelativePath, items[1].RelativePath = "z/исходник.json", "a/исходник.json"
	ValidateWantCodes(t, validateFiles([]*File{c, b, a}, diagnostic.Context{}))
	for _, item := range items {
		ValidateWantCodes(t, manifest.Validate(ValidateFlatFixture(t, item.Entry), diagnostic.Context{}))
	}
	before, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	ValidateWantCodes(t, ValidateLinks([]*File{c, b, a}, items, diagnostic.Context{}))
	after, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("ValidateLinks changed items or sorted their placement")
	}
}

func TestValidateLinksSixMetadataFieldsInEntryOrder(t *testing.T) {
	file := ValidateFileFixture(t, "l001.json", "Alpha")
	item := ValidateItemFixture(t, file, "1")
	subtitle := "Новый подзаголовок"
	item.Entry.ID, item.Entry.Title, item.Entry.Subtitle = "aLPHA", "Новое название", &subtitle
	item.Entry.LineCount, item.Entry.CreatedAt, item.Entry.Version = "13", "2026-11-05T10:00:00Z", "2"
	item.Context.LessonID = item.Entry.ID
	ValidateWantCodes(t, manifest.Validate(ValidateFlatFixture(t, item.Entry), diagnostic.Context{}))
	problems := ValidateLinks([]*File{file}, []catalog.Item{item}, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeMetadata, CodeMetadata, CodeMetadata, CodeMetadata, CodeMetadata, CodeMetadata)
	fields := []string{"id", "title", "subtitle", "lineCount", "createdAt", "version"}
	for i, field := range fields {
		ValidateMetadataDetail(t, problems[i], field)
		if problems[i].Context.FileName != file.FileName() {
			t.Errorf("metadata %s lost technical name: %+v", field, problems[i].Context)
		}
		foundFile, foundEntry := false, false
		for _, related := range problems[i].Related {
			foundFile = foundFile || related == file.Context()
			foundEntry = foundEntry || related == item.Context
		}
		if !foundFile || !foundEntry {
			t.Errorf("metadata %s lacks both source contexts: %+v", field, problems[i].Related)
		}
	}
	ValidateMetadataDetail(t, problems[0], "id", "aLPHA", "Alpha")
	ValidateMetadataDetail(t, problems[1], "title", item.Entry.Title, file.Lesson().Document.Title)
	ValidateMetadataDetail(t, problems[3], "lineCount", "13", "12")
	ValidateMetadataDetail(t, problems[4], "createdAt", item.Entry.CreatedAt, file.Lesson().Document.CreatedAt)
	ValidateMetadataDetail(t, problems[5], "version", "2", "1")
}

func TestValidateLinksSubtitleNullAndEmptyDiffer(t *testing.T) {
	for _, documentHasSubtitle := range []bool{false, true} {
		name := "null document, empty entry"
		if documentHasSubtitle {
			name = "empty document, null entry"
		}
		t.Run(name, func(t *testing.T) {
			value := ValidateLessonFixture(t, "Alpha")
			empty := ""
			if documentHasSubtitle {
				value.Document.Subtitle = &empty
			}
			file, problems := NewFile("l001.json", value, diagnostic.Context{})
			ValidateWantCodes(t, problems)
			item := ValidateItemFixture(t, file, "1")
			if documentHasSubtitle {
				item.Entry.Subtitle = nil
			} else {
				item.Entry.Subtitle = &empty
			}
			ValidateWantCodes(t, manifest.Validate(ValidateFlatFixture(t, item.Entry), diagnostic.Context{}))
			problems = ValidateLinks([]*File{file}, []catalog.Item{item}, diagnostic.Context{})
			ValidateWantCodes(t, problems, CodeMetadata)
			ValidateMetadataDetail(t, problems[0], "subtitle")
		})
	}
}

func TestValidateLinksExactIntegerMetadata(t *testing.T) {
	for _, scenario := range []struct {
		name, document, entry string
		equal                 bool
	}{
		{"fraction representation", "1", "1.000", true},
		{"exponent representation", "100", "1e2", true},
		{"negative exponent", "100", "10000e-2", true},
		{"beyond int64", "184467440737095516160000000001", "184467440737095516160000000001.0", true},
		{"adjacent beyond float64", "9007199254740992", "9007199254740993", false},
		{"huge exponent", "1e100000000000000000000", "10e99999999999999999999", true},
		{"different huge exponent", "1e100000000000000000000", "1e100000000000000000001", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			value := ValidateLessonFixture(t, "Alpha")
			value.Document.LineCount, value.Document.Version = json.Number(scenario.document), json.Number(scenario.document)
			file, problems := NewFile("l001.json", value, diagnostic.Context{})
			ValidateWantCodes(t, problems)
			item := ValidateItemFixture(t, file, "0007")
			item.Entry.LineCount, item.Entry.Version = json.Number(scenario.entry), json.Number(scenario.entry)
			ValidateWantCodes(t, manifest.Validate(ValidateFlatFixture(t, item.Entry), diagnostic.Context{}))
			problems = ValidateLinks([]*File{file}, []catalog.Item{item}, diagnostic.Context{})
			if scenario.equal {
				ValidateWantCodes(t, problems)
			} else {
				ValidateWantCodes(t, problems, CodeMetadata, CodeMetadata)
				ValidateMetadataDetail(t, problems[0], "lineCount", scenario.entry, scenario.document)
				ValidateMetadataDetail(t, problems[1], "version", scenario.entry, scenario.document)
			}
			if item.Entry.Version != json.Number(scenario.entry) || file.Lesson().Document.Version != json.Number(scenario.document) {
				t.Fatal("integer metadata normalized")
			}
		})
	}
}

func TestValidateLinksDoesNotSelectArbitraryDuplicateReference(t *testing.T) {
	a := ValidateFileFixture(t, "l001.json", "Alpha")
	b := ValidateFileFixture(t, "l002.json", "Beta")
	c := ValidateFileFixture(t, "l003.json", "Gamma")
	first, second, third := ValidateItemFixture(t, a, "1"), ValidateItemFixture(t, b, "2"), ValidateItemFixture(t, c, "3")
	second.Entry.FileName, second.Context.FileName = a.FileName(), a.FileName()
	first.Entry.Title, second.Entry.Title, third.Entry.Title = "Первая ошибка", "Вторая ошибка", "Третья ошибка"
	items := []catalog.Item{second, third, first}
	ValidateWantCodes(t, validateFiles([]*File{b, a, c}, diagnostic.Context{}))
	ValidateWantCodes(t, manifest.Validate(ValidateFlatFixture(t, second.Entry, third.Entry, first.Entry), diagnostic.Context{}))
	problems := ValidateLinks([]*File{b, a, c}, items, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeDuplicateReference, CodeUnlistedFile, CodeMetadata)
	ValidateWantRelated(t, problems[0], second.Context, first.Context)
	if problems[2].Context.FileName != c.FileName() {
		t.Errorf("ambiguous reference received metadata diagnostics: %+v", problems[2])
	}
}

func TestValidateLinksMissingAndMetadataErrorsRemainIndependent(t *testing.T) {
	a := ValidateFileFixture(t, "l001.json", "Alpha")
	b := ValidateFileFixture(t, "l002.json", "Beta")
	first, second := ValidateItemFixture(t, a, "1"), ValidateItemFixture(t, b, "2")
	first.Entry.FileName, first.Context.FileName = "l099.json", "l099.json"
	first.Entry.Title, second.Entry.Title = "Нет файла", "Другое название"
	problems := ValidateLinks([]*File{b, a}, []catalog.Item{first, second}, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeMissingFile, CodeUnlistedFile, CodeMetadata)
	if problems[2].Context.FileName != b.FileName() {
		t.Errorf("metadata compared an absent file: %+v", problems[2])
	}
}

func TestValidateLinksZeroLineCountAndNullSubtitle(t *testing.T) {
	value := ValidateLessonFixture(t, "Alpha")
	value.Document.LineCount = "-0.0"
	file, problems := NewFile("l001.json", value, diagnostic.Context{})
	ValidateWantCodes(t, problems)
	item := ValidateItemFixture(t, file, "1")
	item.Entry.LineCount = "0e999999999999999999999"
	ValidateWantCodes(t, manifest.Validate(ValidateFlatFixture(t, item.Entry), diagnostic.Context{}))
	ValidateWantCodes(t, ValidateLinks([]*File{file}, []catalog.Item{item}, diagnostic.Context{}))
}
