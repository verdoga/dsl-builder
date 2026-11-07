package compare_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
)

func TestTask3HasContentChanges(t *testing.T) {
	before, after, empty := "Прежнее название", "Новое название", ""
	documentTitle := compare.FieldChange{
		Field: compare.DocumentTitle, OldPage: -1, NewPage: -1,
		Before: &before, After: &after,
	}
	subtitle := compare.FieldChange{
		Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1,
		Before: nil, After: &empty,
	}
	pageTitle := compare.FieldChange{
		Field: compare.PageTitle, OldPage: 2, NewPage: 2,
		Before: &before, After: nil,
	}
	unknownPageTitle := compare.FieldChange{
		Field: compare.PageTitle, OldPage: -1, NewPage: 0,
		Before: nil, After: nil,
	}
	lessonReasons := []compare.Reason{{
		Code: compare.PageCountChanged, Description: "Число страниц изменилось с 1 до 2.",
		OldPointer: "/document/pages", NewPointer: "/document/pages",
	}}
	pageChanges := []compare.PageChange{{
		Index: 2,
		Nodes: []compare.NodeChange{{
			Path:   []int{1, 0, 3},
			Fields: []compare.NodeField{compare.NodeTypeChanged, compare.NodeFlagsChanged, compare.NodeDataChanged},
		}},
	}}
	structuralChanges := []compare.PageChange{{
		Index: 0, ReplaceNodes: true,
		Reasons: []compare.Reason{{
			Code: compare.NodeNestingChanged, Description: "Число дочерних нод изменилось с 1 до 2.",
			OldPointer: "/document/pages/0/nodes/1/nodes", NewPointer: "/document/pages/0/nodes/1/nodes",
		}},
	}}

	cases := []struct {
		name string
		plan compare.Plan
		want bool
	}{
		{name: "zero plan"},
		{name: "allocated empty slices", plan: compare.Plan{
			Fields: []compare.FieldChange{}, Reasons: []compare.Reason{}, Pages: []compare.PageChange{},
		}},
		{name: "document title awaits decision", plan: compare.Plan{Fields: []compare.FieldChange{documentTitle}}},
		{name: "subtitle null to empty awaits decision", plan: compare.Plan{Fields: []compare.FieldChange{subtitle}}},
		{name: "page title removal awaits decision", plan: compare.Plan{Fields: []compare.FieldChange{pageTitle}}},
		{name: "all proposed fields await decisions", plan: compare.Plan{
			Fields: []compare.FieldChange{documentTitle, subtitle, pageTitle},
		}},
		// Проверяем представление, а не валидность плана: Reasons не заменяет ReplaceLesson.
		{name: "reasons alone are not content changes", plan: compare.Plan{Reasons: lessonReasons}},
		{name: "fields and reasons alone are not content changes", plan: compare.Plan{
			Fields: []compare.FieldChange{documentTitle}, Reasons: lessonReasons,
		}},
		{name: "whole lesson replacement", plan: compare.Plan{
			ReplaceLesson: true, Reasons: lessonReasons,
		}, want: true},
		{name: "whole replacement still retains pending fields", plan: compare.Plan{
			ReplaceLesson: true, Reasons: lessonReasons,
			Fields: []compare.FieldChange{documentTitle, subtitle, unknownPageTitle},
		}, want: true},
		// Метод читает признак и наличие страниц, не валидирует содержимое представления.
		{name: "replacement flag alone", plan: compare.Plan{ReplaceLesson: true}, want: true},
		{name: "nonempty pages alone", plan: compare.Plan{Pages: []compare.PageChange{{}}}, want: true},
		{name: "individual nested node changes", plan: compare.Plan{Pages: pageChanges}, want: true},
		{name: "all nodes of a page replaced", plan: compare.Plan{Pages: structuralChanges}, want: true},
		{name: "node changes with pending field", plan: compare.Plan{
			Fields: []compare.FieldChange{documentTitle}, Pages: pageChanges,
		}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original, err := json.Marshal(tc.plan)
			if err != nil {
				t.Fatal(err)
			}
			for call := 0; call < 2; call++ {
				if got := tc.plan.HasContentChanges(); got != tc.want {
					t.Errorf("HasContentChanges() = %v, want %v", got, tc.want)
				}
			}
			current, err := json.Marshal(tc.plan)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(current, original) {
				t.Error("HasContentChanges changed the plan or applied proposed fields")
			}
		})
	}
}

func TestTask3NodeFieldValues(t *testing.T) {
	cases := []struct {
		field compare.NodeField
		want  string
	}{
		{compare.NodeTypeChanged, "type"},
		{compare.NodeFlagsChanged, "flags"},
		{compare.NodeDataChanged, "data"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := string(tc.field); got != tc.want {
				t.Errorf("NodeField = %q, want %q", got, tc.want)
			}
		})
	}
}
