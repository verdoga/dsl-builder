package structure_test

import (
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestTask26NestedSemanticOwnersAndContainerExit(t *testing.T) {
	lines := []model.Line{
		task26Tag(1, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "same")),
		task26Tag(2, 1, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "outer")),
		task26Tag(3, 1, model.LineTypeBlockStart, "variants"),
		task26Tag(4, 3, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "A")),
		task26Tag(5, 3, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "same")),
		task26Tag(6, 5, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "inner")),
		task26Text(7, 5, "inner step"),
		task26Tag(8, 3, model.LineTypeTag, "endtask"),
		task26Text(9, 3, "branch A after inner task"),
		task26Tag(10, 3, model.LineTypeBlockStart, "variants"),
		task26Tag(11, 10, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "inner")),
		task26Tag(12, 10, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "same")),
		task26Text(13, 12, "deep"),
		task26Tag(14, 10, model.LineTypeTag, "endtask"),
		task26Text(15, 10, "deep branch"),
		task26End(16, 10),
		task26Text(17, 3, "outer branch resumes"),
		task26Tag(18, 3, model.LineTypeTag, "variant", task26Element(model.ElementTypeName, "B")),
		task26Tag(19, 3, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "same")),
		task26Text(20, 19, "branch B task"),
		task26Tag(21, 3, model.LineTypeTag, "endtask"),
		task26End(22, 3),
		task26Text(23, 1, "outer step after variants"),
		task26Tag(24, 0, model.LineTypeTag, "endtask"),
		task26Text(25, 0, "outside"),
	}
	pages, bindings := task26Build(t, lines, "[task(step(variants(variant(task(step(text)),text,variants(variant(task(text),text)),text),variant(task(text))),text)),text]")
	for _, number := range []int{1, 5, 12, 19} {
		node := task26Binding(t, bindings, number, lesson.NodeTask, pages).Node(pages)
		wantStepped := number == 1 || number == 5
		if *node.Flags.Stepped != wantStepped || *node.Flags.Speaking {
			t.Fatalf("wrong flags for task line %d", number)
		}
	}
}

func TestTask26NestedTaskDoesNotStepOrSpeakForOuterTask(t *testing.T) {
	pages, bindings := task26Build(t, []model.Line{
		task26Tag(1, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "outer")),
		task26Tag(2, 1, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "inner")),
		task26Tag(3, 2, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "inner step")),
		task26Tag(4, 2, model.LineTypeTag, "speaking"),
		task26Text(5, 2, "inside"),
		task26Tag(6, 1, model.LineTypeTag, "endtask"),
		task26Text(7, 1, "outer again"),
	}, "[task(task(step(text)),text)]")
	outer := task26Binding(t, bindings, 1, lesson.NodeTask, pages).Node(pages)
	inner := task26Binding(t, bindings, 2, lesson.NodeTask, pages).Node(pages)
	if *outer.Flags.Speaking || *outer.Flags.Stepped || !*inner.Flags.Speaking || !*inner.Flags.Stepped {
		t.Fatal("nested task flags leaked")
	}
}

func TestTask26IncludesPreserveExternalStepAndIndependentOccurrences(t *testing.T) {
	pages, bindings := task26Build(t, []model.Line{
		task26Tag(1, 0, model.LineTypeBlockStart, "fragment", task26Element(model.ElementTypeIdentifier, "body")),
		task26Tag(2, 1, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "same")),
		task26Text(3, 2, "fragment task"),
		task26End(4, 1),
		task26Tag(5, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "outer")),
		task26Tag(6, 5, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "stage")),
		task26Tag(7, 5, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "body")),
		task26Text(8, 5, "after first include"),
		task26Tag(9, 5, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "body")),
		task26Text(10, 5, "after second include"),
	}, "[task(step(task(text),text,task(text),text))]")
	var repeated []int
	for i, binding := range bindings {
		if binding.Entries[0].Source.Number == 2 {
			repeated = append(repeated, i)
		}
	}
	if len(repeated) != 2 || bindings[repeated[0]].Entries[0] == bindings[repeated[1]].Entries[0] {
		t.Fatal("include occurrences collapsed")
	}
	first := bindings[repeated[0]].Node(pages)
	second := bindings[repeated[1]].Node(pages)
	first.ID = "caller mutation"
	first.Data = append(first.Data, lesson.Data{Value: "caller data"})
	if second.ID != "" || len(second.Data) != 0 {
		t.Fatal("independent include nodes alias")
	}
}

func TestTask26PageInsideIncludeDoesNotResurrectClosedTask(t *testing.T) {
	pages, _ := task26Build(t, []model.Line{
		task26Tag(1, 0, model.LineTypeBlockStart, "fragment", task26Element(model.ElementTypeIdentifier, "body")),
		task26Text(2, 1, "before boundary"),
		task26Tag(3, 1, model.LineTypeTag, "newpage"),
		task26Text(4, 1, "after boundary"),
		task26End(5, 1),
		task26Tag(6, 0, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "outer")),
		task26Tag(7, 6, model.LineTypeTag, "step", task26Element(model.ElementTypeTitle, "stage")),
		task26Tag(8, 6, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "body")),
		task26Text(9, 6, "outside completed task"),
	}, "[task(step(text))]|[text,text]")
	if pages[0].Title != nil || pages[1].Title != nil {
		t.Fatal("newpage must not copy document title")
	}
}

func TestTask26PagesKeepExplicitEmptyBoundariesAndHeader(t *testing.T) {
	cases := []struct {
		name   string
		lines  []model.Line
		shape  string
		titles []*string
	}{
		{"no material", nil, "[]", []*string{nil}},
		{"first heading", []model.Line{task26Heading(1, "1", "Document"), task26Heading(2, "2", "Sub"), task26Heading(3, "3", "  A  ")}, "[]", []*string{task26String("  A  ")}},
		{"first newpage", []model.Line{task26Tag(1, 0, model.LineTypeTag, "newpage")}, "[]|[]", []*string{nil, nil}},
		{"consecutive", []model.Line{task26Text(1, 0, "intro"), task26Heading(2, "3", "A"), task26Heading(3, "3", "B"), task26Tag(4, 0, model.LineTypeTag, "newpage")}, "[text]|[]|[]|[]", []*string{nil, task26String("A"), task26String("B"), nil}},
		{"header and metadata", []model.Line{task26Tag(1, 0, model.LineTypeTag, "section", task26Element(model.ElementTypeName, "S")), task26Tag(2, 0, model.LineTypeTag, "header", task26Element(model.ElementTypeTitle, "H")), task26Text(3, 0, "body")}, "[heading,text]", []*string{nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pages, _ := task26Build(t, tc.lines, tc.shape)
			if len(pages) != len(tc.titles) {
				t.Fatalf("pages=%d", len(pages))
			}
			for i, want := range tc.titles {
				if want == nil {
					if pages[i].Title != nil {
						t.Fatalf("title[%d]=%q", i, *pages[i].Title)
					}
					continue
				}
				if pages[i].Title == nil || *pages[i].Title != *want {
					t.Fatalf("title[%d] lost", i)
				}
			}
		})
	}
}
