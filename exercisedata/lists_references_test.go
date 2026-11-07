package exercisedata

import (
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func exListEntry(role lesson.DataRole, value string, line int, answers ...string) entry {
	source := exListContext(line, 3)
	e := entry{data: lesson.Data{Type: lesson.DataText, Role: &role, Value: value, Answers: append([]string{}, answers...)}, source: source}
	if len(answers) != 0 {
		a := exListContext(line+1, 1)
		e.answerSource = &a
		for range answers {
			e.answerSources = append(e.answerSources, exListContext(line+1, 9))
		}
	}
	return e
}
func TestExListReferencesEqualFoldWithoutNormalization(t *testing.T) {
	labels := []entry{exListEntry(lesson.RoleOption, "Σ", 201), exListEntry(lesson.RoleOption, " 01. <b>A</b>! ", 202)}
	for _, tc := range []struct {
		value string
		index int
	}{{"ς", 0}, {"σ", 0}, {" 01. <B>a</B>! ", 1}} {
		index, err := resolveLabel(tc.value, labels, exListContext(150, 9))
		if err != nil || index != tc.index {
			t.Fatalf("resolve %q = %d, %v", tc.value, index, err)
		}
	}
	for _, value := range []string{"01. <b>A</b>!", " 1. <b>A</b>! ", " 01. A! ", " 01. <b>A</b> ", "missing"} {
		_, err := resolveLabel(value, labels, exListContext(150, 9))
		exListError(t, err, CodeReference)
		if err.Context != exListContext(150, 9) {
			t.Fatalf("answer source = %+v", err.Context)
		}
	}
	for _, role := range []lesson.DataRole{lesson.RoleOption, lesson.RolePosition} {
		labels := []entry{exListEntry(role, "A", 201), exListEntry(role, "B", 202)}
		items := []entry{exListEntry(lesson.RoleItem, "one", 101, "a", "A"), exListEntry(lesson.RoleItem, "two", 103, "A")}
		before := exListJSON(t, items)
		if err := checkLabelReferences(items, labels, exListContext(100, 1)); err != nil {
			t.Fatalf("many-to-one %s: %v", role, err)
		}
		if exListJSON(t, items) != before {
			t.Fatal("references rewritten")
		}
		items[1].data.Answers[0] = "unresolved"
		err := checkLabelReferences(items, labels, exListContext(100, 1))
		exListError(t, err, CodeReference)
		if err.Context != items[1].answerSources[0] {
			t.Fatalf("reference coordinates = %+v", err.Context)
		}
	}
}
func TestExListDuplicatesIncludeUnusedLabelsAndAllCandidates(t *testing.T) {
	labels := []entry{exListEntry(lesson.RoleOption, "A", 201), exListEntry(lesson.RoleOption, "Σ", 202), exListEntry(lesson.RoleOption, "ς", 203), exListEntry(lesson.RoleOption, "σ", 204)}
	items := []entry{exListEntry(lesson.RoleItem, "item", 101, "A")}
	before := exListJSON(t, labels)
	for _, run := range []struct {
		name string
		call func() *diagnostic.Error
	}{
		{"distinct", func() *diagnostic.Error { return checkDistinctLabels(labels, exListContext(100, 1)) }},
		{"unused duplicates", func() *diagnostic.Error { return checkLabelReferences(items, labels, exListContext(100, 1)) }},
		{"resolve candidates", func() *diagnostic.Error { _, err := resolveLabel("σ", labels, exListContext(150, 9)); return err }},
	} {
		t.Run(run.name, func(t *testing.T) {
			err := run.call()
			exListError(t, err, CodeReference)
			if len(err.Related) != 3 {
				t.Fatalf("all duplicate candidates = %+v", err.Related)
			}
			for i, want := range labels[1:] {
				if err.Related[i] != want.source {
					t.Fatalf("candidate order = %+v", err.Related)
				}
			}
			if run.name == "resolve candidates" && err.Context != exListContext(150, 9) {
				t.Fatalf("source = %+v", err.Context)
			}
			if exListJSON(t, run.call()) != exListJSON(t, err) {
				t.Fatal("nondeterministic diagnostic")
			}
			err.Related[0].Line = 999
			if exListJSON(t, labels) != before || run.call().Related[0].Line != 202 {
				t.Fatal("diagnostic aliases labels or next call")
			}
		})
	}
	if err := checkDistinctLabels([]entry{exListEntry(lesson.RoleOption, " A", 201), exListEntry(lesson.RoleOption, "A", 202), exListEntry(lesson.RoleOption, "A!", 203)}, exListContext(100, 1)); err != nil {
		t.Fatalf("significant characters collapsed: %v", err)
	}
}
