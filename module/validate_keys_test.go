package module

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func TestValidateEntryKeysEmpty(t *testing.T) {
	ValidateWantCodes(t, validateEntryKeys(nil, diagnostic.Context{}))
	ValidateWantCodes(t, validateEntryKeys([]catalog.Item{}, diagnostic.Context{}))
}

func TestValidateEntryKeysCaseInsensitiveAcrossSections(t *testing.T) {
	ids := []string{"Урок+1", "урок+1", "УРок+1"}
	names := []string{"l002.json", "l001.json", "l1000.json"}
	sections := []string{"Z", "A", "B"}
	var items []catalog.Item
	for i := range ids {
		file := ValidateFileFixture(t, names[i], ids[i])
		item := ValidateItemFixture(t, file, "1")
		item.Section = &sections[i]
		items = append(items, item)
	}
	before, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	problems := validateEntryKeys(items, diagnostic.Context{Path: "другой.zip"})
	ValidateWantCodes(t, problems, CodeDuplicateID, CodeDuplicateID)
	for _, problem := range problems {
		ValidateWantRelated(t, problem, items[0].Context, items[1].Context, items[2].Context)
	}
	after, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("entry keys changed ID spelling, sections or order")
	}
}

func TestValidateEntryKeysReferencesAreGlobal(t *testing.T) {
	var items []catalog.Item
	for i, id := range []string{"Alpha", "Beta", "Gamma"} {
		item := ValidateItemFixture(t, ValidateFileFixture(t, "l001.json", id), "1")
		section := []string{"Section", "section", "Other"}[i]
		item.Section = &section
		items = append(items, item)
	}
	problems := validateEntryKeys(items, diagnostic.Context{})
	if len(problems) == 0 {
		t.Fatal("same technical name across sections accepted")
	}
	for _, problem := range problems {
		if problem.Code != CodeDuplicateReference {
			t.Errorf("reference conflict code = %s", problem.Code)
		}
		ValidateWantRelated(t, problem, items[0].Context, items[1].Context, items[2].Context)
	}
}

func TestValidateEntryKeysDoesNotNormalizeIdentityOrFileNames(t *testing.T) {
	items := []catalog.Item{
		ValidateItemFixture(t, ValidateFileFixture(t, "l001.json", "é"), "1"),
		ValidateItemFixture(t, ValidateFileFixture(t, "l1000.json", "e\u0301"), "1"),
		ValidateItemFixture(t, ValidateFileFixture(t, "l1000000000000000000000000000000000.json", "Other"), "1"),
	}
	ValidateWantCodes(t, validateEntryKeys(items, diagnostic.Context{}))
}

func TestValidateEntryKeysUniqueItemsDoesNotEnsureUniqueKeys(t *testing.T) {
	a := ValidateItemFixture(t, ValidateFileFixture(t, "l001.json", "Alpha"), "1")
	b := ValidateItemFixture(t, ValidateFileFixture(t, "l002.json", "aLPHA"), "2")
	b.Entry.Title = "Другое название с тем же ключом ID"
	value := ValidateFlatFixture(t, a.Entry, b.Entry)
	ValidateWantCodes(t, manifest.Validate(value, diagnostic.Context{}))
	problems := validateEntryKeys([]catalog.Item{a, b}, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeDuplicateID)
	ValidateWantRelated(t, problems[0], a.Context, b.Context)

	b.Entry.ID = "Beta"
	b.Context.LessonID = "Beta"
	b.Entry.FileName = a.Entry.FileName
	b.Context.FileName = a.Entry.FileName
	value = ValidateFlatFixture(t, a.Entry, b.Entry)
	ValidateWantCodes(t, manifest.Validate(value, diagnostic.Context{}))
	problems = validateEntryKeys([]catalog.Item{a, b}, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeDuplicateReference)
	ValidateWantRelated(t, problems[0], a.Context, b.Context)
}

func TestValidateEntryKeysSuppliesKnownFallback(t *testing.T) {
	a := ValidateItemFixture(t, ValidateFileFixture(t, "l001.json", "Alpha"), "1")
	b := ValidateItemFixture(t, ValidateFileFixture(t, "l002.json", "aLPHA"), "2")
	a.Context, b.Context = diagnostic.Context{}, diagnostic.Context{}
	context := diagnostic.Context{Path: "модуль.zip"}
	problems := validateEntryKeys([]catalog.Item{a, b}, context)
	ValidateWantCodes(t, problems, CodeDuplicateID)
	ValidateWantRelated(t, problems[0],
		diagnostic.Context{Path: context.Path, FileName: a.Entry.FileName, LessonID: a.Entry.ID},
		diagnostic.Context{Path: context.Path, FileName: b.Entry.FileName, LessonID: b.Entry.ID})
}

func TestValidateEntryKeysIDAndReferenceConflictsAreIndependent(t *testing.T) {
	a := ValidateItemFixture(t, ValidateFileFixture(t, "l001.json", "Alpha"), "1")
	b := ValidateItemFixture(t, ValidateFileFixture(t, "l001.json", "ALPHA"), "2")
	problems := validateEntryKeys([]catalog.Item{a, b}, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeDuplicateID, CodeDuplicateReference)
	for _, problem := range problems {
		ValidateWantRelated(t, problem, a.Context, b.Context)
	}
}
