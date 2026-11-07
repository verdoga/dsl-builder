package app

import (
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
)

func copyPlanFixture() *compare.Plan {
	before, after, empty := "Прежнее", "Новое", ""
	return &compare.Plan{
		Fields: []compare.FieldChange{
			{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: &before, After: &after},
			{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1, Before: &empty},
			{Field: compare.PageTitle, OldPage: -1, NewPage: 2},
			{Field: compare.PageTitle, OldPage: 1, NewPage: 1, After: &empty},
		},
		Reasons: []compare.Reason{},
		Pages: []compare.PageChange{
			{Index: 0, ReplaceNodes: true, Reasons: []compare.Reason{{Code: compare.NodeNestingChanged,
				Description: "Изменилось число детей", OldPointer: "/document/pages/0/nodes/2/nodes", NewPointer: "/document/pages/0/nodes/2/nodes"}}},
			{Index: 1, Nodes: []compare.NodeChange{
				{Path: []int{2, 1, 4}, Fields: []compare.NodeField{compare.NodeTypeChanged, compare.NodeFlagsChanged, compare.NodeDataChanged}},
				{Path: []int{3}, Fields: []compare.NodeField{compare.NodeDataChanged}},
			}},
		},
	}
}

func copyPlanMutate(value *compare.Plan) {
	*value.Fields[0].Before = "изменено"
	*value.Fields[0].After = "изменено"
	*value.Fields[1].Before = "изменено"
	*value.Fields[3].After = "изменено"
	value.Fields[2].OldPage, value.Fields[2].NewPage = 99, 99
	value.Fields[2].Field = compare.DocumentTitle
	value.Pages[0].Reasons[0] = compare.Reason{Description: "изменено"}
	value.Pages[1].Nodes[0].Path[2] = 99
	value.Pages[1].Nodes[0].Fields[0] = compare.NodeDataChanged
	value.Pages[1].Nodes[1] = compare.NodeChange{}
	value.Pages[0].Index = 99
	value.Pages[0].ReplaceNodes = false
}

func TestCopyPlanPreservesFullPlanAndIndependence(t *testing.T) {
	source := copyPlanFixture()
	first, second := clonePlan(source), clonePlan(source)
	if !reflect.DeepEqual(first, source) || !reflect.DeepEqual(second, source) {
		t.Fatal("потеряны поля, индексы, причины или пути исходного плана")
	}
	copyPlanMutate(first)
	if !reflect.DeepEqual(source, copyPlanFixture()) || !reflect.DeepEqual(second, source) {
		t.Fatal("правка копии повредила источник или вторую копию")
	}
	copyPlanMutate(source)
	if !reflect.DeepEqual(second, copyPlanFixture()) {
		t.Fatal("копия разделяет память с источником")
	}
}

func TestCopyPlanFullReplacementKeepsUnmatchedNullableFields(t *testing.T) {
	source := copyPlanFixture()
	source.ReplaceLesson, source.Pages = true, nil
	source.Reasons = []compare.Reason{{Code: compare.PageCorrespondenceAmbiguous,
		Description: "Не установлено соответствие", OldPointer: "/document/pages", NewPointer: "/document/pages"}}
	got, other := clonePlan(source), clonePlan(source)
	if !reflect.DeepEqual(got, source) {
		t.Fatal("полная замена изменила отдельные предложения полей")
	}
	got.Reasons[0].Description = "правка"
	got.Reasons[0].OldPointer = "правка"
	got.Fields[2].OldPage = 0
	if source.Reasons[0].Description != "Не установлено соответствие" || !reflect.DeepEqual(other, source) {
		t.Fatal("причины или несопоставленные поля разделяют память")
	}
	source.Reasons[0].NewPointer = "правка источника"
	if other.Reasons[0].NewPointer != "/document/pages" {
		t.Fatal("правка источника повредила ранее выданные причины")
	}
}

func TestCopyPlanPreservesNilEmptyAtEveryLevel(t *testing.T) {
	if clonePlan(nil) != nil {
		t.Fatal("nil plan")
	}
	for _, value := range []*compare.Plan{
		{},
		{Fields: []compare.FieldChange{}, Reasons: []compare.Reason{}, Pages: []compare.PageChange{}},
		{Pages: []compare.PageChange{{Reasons: []compare.Reason{}, Nodes: []compare.NodeChange{}}}},
		{Pages: []compare.PageChange{{Nodes: []compare.NodeChange{{}, {Path: []int{}, Fields: []compare.NodeField{}}}}}},
	} {
		if got := clonePlan(value); !reflect.DeepEqual(got, value) {
			t.Fatalf("nil/empty преобразованы: %#v", got)
		}
	}
}
