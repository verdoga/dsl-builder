package app

import (
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
)

func TestCatalogEditFullLessonGroupIsAtomicAndAllowsIntermediateConflict(t *testing.T) {
	op := catalogEditCreate(t, []string{"Alpha", "Alpha", "Beta"})
	before := op.Snapshot()
	section := "Alpha"
	for _, test := range []struct {
		name    string
		weights map[string]string
	}{
		{"nil", nil},
		{"missing", map[string]string{"l001.json": "4"}},
		{"foreign", map[string]string{"l001.json": "4", "l003.json": "5"}},
		{"extra", map[string]string{"l001.json": "4", "l002.json": "5", "l003.json": "6"}},
		{"case", map[string]string{"L001.json": "4", "l002.json": "5"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			preview, issues := op.ProposeLessonWeights(before.Token, &section, test.weights)
			catalogEditCode(t, issues, CodeIncompleteWeights)
			if preview != nil || op.Snapshot().Token != before.Token || catalogEditJSON(t, op.Snapshot().Catalog) != catalogEditJSON(t, before.Catalog) {
				t.Fatal("incomplete group changed accepted state")
			}
		})
	}
	preview, issues := op.ProposeLessonWeights(before.Token, &section, map[string]string{"l001.json": "4", "l002.json": "0"})
	if preview != nil || len(issues) == 0 || catalogEditJSON(t, op.Snapshot().Catalog) != catalogEditJSON(t, before.Catalog) {
		t.Fatal("invalid later input retained an earlier valid step")
	}
	weights := map[string]string{"l001.json": "2", "l002.json": "0001"}
	preview, issues = op.ProposeLessonWeights(before.Token, &section, weights)
	catalogEditOK(t, issues)
	if preview == nil || len(preview.Blockers) != 0 {
		t.Fatal("intermediate equal weights escaped instead of final Refresh")
	}
	weights["l001.json"] = "999"
	section = "Beta"
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	after := op.Snapshot()
	if *catalogEditItem(t, after.Catalog, "l001.json").Entry.Order != "2" ||
		*catalogEditItem(t, after.Catalog, "l002.json").Entry.Order != "0001" ||
		*catalogEditItem(t, after.Catalog, "l003.json").Entry.Order != "3" {
		t.Fatal("full-group result lost spelling or changed another section")
	}
	if after.Catalog.Items[0].Entry.FileName != "l002.json" {
		t.Fatal("final group was not numerically sorted")
	}
}

func TestCatalogEditFlatGroupLargeWeightsAndNumericDuplicates(t *testing.T) {
	op := catalogEditCreate(t, []string{"", ""})
	large := "000999999999999999999999999999999999999999999999999999999999999999999999"
	preview, issues := op.ProposeLessonWeights(op.Snapshot().Token, nil,
		map[string]string{"l001.json": large, "l002.json": "0003"})
	catalogEditOK(t, issues)
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	state := op.Snapshot()
	if *catalogEditItem(t, state.Catalog, "l001.json").Entry.Order != large || state.Catalog.Items[0].Entry.FileName != "l002.json" {
		t.Fatal("large weight was truncated or ordered lexically")
	}
	preview, issues = op.ProposeLessonWeights(state.Token, nil,
		map[string]string{"l001.json": "0003", "l002.json": "3"})
	catalogEditCode(t, issues, catalog.CodeWeightConflict)
	if preview == nil {
		t.Fatal("numeric duplicate is an editable candidate")
	}
	catalogEditCode(t, preview.Blockers, catalog.CodeWeightConflict)
	op.AcceptEdit(preview.Token)
	state = op.Snapshot()
	catalogEditCode(t, state.Blockers, catalog.CodeWeightConflict)
	if state.Actions.ReviewSave || state.Actions.Save {
		t.Fatal("conflict candidate enabled writing")
	}
}

func TestCatalogEditGroupHelpersUseItemsOrderAndDiscardNilStep(t *testing.T) {
	_, state := catalogEditOwnerModule(t, catalogEditSources(t, []string{"", ""}))
	original := catalogEditJSON(t, state)
	if issue := checkWeightMembers(state, nil, map[string]string{"l001.json": "2", "l002.json": "1"}); issue != nil {
		t.Fatal(issue)
	}
	for range 20 {
		candidate, issues := applyGroupWeights(state, map[string]string{"l001.json": "0", "l002.json": "bad"})
		if candidate != nil || len(issues) == 0 || issues[0].Context.FileName != state.Items[0].Entry.FileName {
			t.Fatal("group steps did not follow Items order")
		}
	}
	candidate, issues := applyGroupWeights(state, map[string]string{"l001.json": "2", "l002.json": "0"})
	if candidate != nil || len(issues) == 0 || catalogEditJSON(t, state) != original {
		t.Fatal("nil later step returned or leaked a partial candidate")
	}
	candidate, issues = applyGroupWeights(state, map[string]string{"l001.json": "2", "l002.json": "1"})
	catalogEditOK(t, issues)
	if candidate == nil || catalogEditJSON(t, state) != original || len(catalog.Conflicts(candidate)) != 0 {
		t.Fatal("complete valid swap failed or changed its input")
	}
}

func TestCatalogEditSectionWeightsRequireWholeIndependentSet(t *testing.T) {
	op := catalogEditCreate(t, []string{"Alpha", "Beta"})
	before := op.Snapshot()
	for _, test := range []struct {
		name    string
		weights []catalog.SectionOrder
	}{
		{"missing", []catalog.SectionOrder{{Title: "Alpha", Weight: catalogEditString("2")}}},
		{"duplicate", []catalog.SectionOrder{{Title: "Alpha", Weight: catalogEditString("2")}, {Title: "Alpha", Weight: catalogEditString("1")}}},
		{"unknown", []catalog.SectionOrder{{Title: "Alpha", Weight: catalogEditString("2")}, {Title: "beta", Weight: catalogEditString("1")}}},
		{"nil weight", []catalog.SectionOrder{{Title: "Alpha"}, {Title: "Beta", Weight: catalogEditString("1")}}},
		{"zero", []catalog.SectionOrder{{Title: "Alpha", Weight: catalogEditString("0")}, {Title: "Beta", Weight: catalogEditString("1")}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			preview, issues := op.ProposeSectionWeights(before.Token, test.weights)
			if preview != nil || len(issues) == 0 || op.Snapshot().Token != before.Token || catalogEditJSON(t, op.Snapshot().Catalog) != catalogEditJSON(t, before.Catalog) {
				t.Fatal("invalid section set was partially applied")
			}
		})
	}
	weights := []catalog.SectionOrder{{Title: "Alpha", Weight: catalogEditString("999999999999999999999999999999999999999")}, {Title: "Beta", Weight: catalogEditString("0001")}}
	preview, issues := op.ProposeSectionWeights(before.Token, weights)
	catalogEditOK(t, issues)
	*weights[1].Weight = "999999999999999999999999999999999999999999"
	weights[1].Title = "mutated"
	*preview.Catalog.Sections[0].Weight = "666"
	catalogEditOK(t, op.AcceptEdit(preview.Token))
	after := op.Snapshot()
	if after.Catalog.Sections[0].Title != "Beta" || *after.Catalog.Sections[0].Weight != "0001" {
		t.Fatal("section weights shared exposed input or preview memory")
	}
	preview, issues = op.ProposeSectionWeights(after.Token, []catalog.SectionOrder{
		{Title: "Beta", Weight: catalogEditString("01")},
		{Title: "Alpha", Weight: catalogEditString("1")},
	})
	catalogEditOK(t, issues)
	if preview == nil || preview.Catalog.Sections[0].Title != "Alpha" || len(preview.Blockers) != 0 {
		t.Fatal("equal section weights were treated as lesson conflicts or not sorted by title")
	}
}
