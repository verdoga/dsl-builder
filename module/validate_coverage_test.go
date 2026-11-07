package module

import (
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestValidateCoverageEmpty(t *testing.T) {
	ValidateWantCodes(t, validateCoverage(nil, nil, diagnostic.Context{}))
	ValidateWantCodes(t, validateCoverage([]*File{}, []catalog.Item{}, diagnostic.Context{}))
}

func TestValidateCoverageIsBidirectionalAndOrdered(t *testing.T) {
	a := ValidateFileFixture(t, "l002.json", "Alpha")
	b := ValidateFileFixture(t, "l001.json", "Beta")
	missingFirst := ValidateItemFixture(t, ValidateFileFixture(t, "l1000.json", "Gamma"), "1")
	missingSecond := ValidateItemFixture(t, ValidateFileFixture(t, "l003.json", "Delta"), "2")
	problems := validateCoverage([]*File{a, b}, []catalog.Item{missingFirst, missingSecond}, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeMissingFile, CodeMissingFile, CodeUnlistedFile, CodeUnlistedFile)
	want := []diagnostic.Context{missingFirst.Context, missingSecond.Context, a.Context(), b.Context()}
	for i, problem := range problems {
		if problem.Context != want[i] {
			t.Errorf("diagnostic %d context = %+v, want %+v", i, problem.Context, want[i])
		}
	}
}

func TestValidateCoverageMatchesExactFileNamesNotIDsOrPositions(t *testing.T) {
	a := ValidateFileFixture(t, "l002.json", "Alpha")
	b := ValidateFileFixture(t, "l001.json", "Beta")
	aItem, bItem := ValidateItemFixture(t, a, "1"), ValidateItemFixture(t, b, "2")
	aItem.Entry.ID, bItem.Entry.ID = bItem.Entry.ID, aItem.Entry.ID
	ValidateWantCodes(t, validateCoverage([]*File{b, a}, []catalog.Item{aItem, bItem}, diagnostic.Context{}))

	aItem.Entry.FileName = "l003.json"
	problems := validateCoverage([]*File{b, a}, []catalog.Item{bItem, aItem}, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeMissingFile, CodeUnlistedFile)
	if problems[0].Context.FileName != "l003.json" || problems[1].Context.FileName != a.FileName() {
		t.Errorf("coverage used identity instead of exact technical names: %+v, %+v", problems[0], problems[1])
	}
}

func TestValidateCoverageOneSideOnly(t *testing.T) {
	a := ValidateFileFixture(t, "l001.json", "Alpha")
	item := ValidateItemFixture(t, a, "1")
	ValidateWantCodes(t, validateCoverage(nil, []catalog.Item{item}, diagnostic.Context{}), CodeMissingFile)
	ValidateWantCodes(t, validateCoverage([]*File{a}, nil, diagnostic.Context{}), CodeUnlistedFile)
}

func TestValidateCoverageDoesNotReplaceKeyValidation(t *testing.T) {
	a := ValidateFileFixture(t, "l001.json", "Alpha")
	first := ValidateItemFixture(t, a, "1")
	second := first
	second.Entry.ID, second.Entry.Title = "Beta", "Вторая запись"
	second.Context.LessonID = "Beta"
	ValidateWantCodes(t, validateCoverage([]*File{a}, []catalog.Item{first, second}, diagnostic.Context{}))
	ValidateWantCodes(t, validateEntryKeys([]catalog.Item{first, second}, diagnostic.Context{}), CodeDuplicateReference)
}

func TestValidateCoverageFallbackUsesArchiveAndKnownEntry(t *testing.T) {
	a := &File{name: "l001.json", value: ValidateLessonFixture(t, "Alpha")}
	item := ValidateItemFixture(t, ValidateFileFixture(t, "l002.json", "Beta"), "1")
	item.Context = diagnostic.Context{}
	context := diagnostic.Context{Path: "архив.zip"}
	problems := validateCoverage([]*File{a}, []catalog.Item{item}, context)
	ValidateWantCodes(t, problems, CodeMissingFile, CodeUnlistedFile)
	want := []diagnostic.Context{
		{Path: context.Path, FileName: item.Entry.FileName, LessonID: item.Entry.ID},
		{Path: context.Path, FileName: a.FileName(), LessonID: "Alpha"},
	}
	for i, problem := range problems {
		if problem.Context != want[i] {
			t.Errorf("fallback diagnostic %d = %+v, want %+v", i, problem.Context, want[i])
		}
	}
}
