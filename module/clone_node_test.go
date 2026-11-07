package module

import (
	"fmt"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
)

func TestClonePageIndependentOrderedTree(t *testing.T) {
	for _, direction := range []string{"copy", "input"} {
		t.Run(direction, func(t *testing.T) {
			input := cloneTestFixture(t).Document.Pages[0]
			want := cloneTestFixture(t).Document.Pages[0]
			first, second := clonePage(input), clonePage(input)
			cloneTestEqual(t, first, want)
			cloneTestEqual(t, second, want)
			cloneTestEqual(t, input, want)
			if direction == "copy" {
				cloneTestMutatePage(t, &first)
				cloneTestEqual(t, input, want)
			} else {
				cloneTestMutatePage(t, &input)
				cloneTestEqual(t, first, want)
			}
			cloneTestEqual(t, second, want)
		})
	}
}

func TestClonePagePreservesOptionalTitleAndNilOrEmptyNodes(t *testing.T) {
	for _, title := range []*string{nil, new(string), cloneTestString(t, " \nНазвание\t ")} {
		for _, nodes := range [][]lesson.Node{nil, {}} {
			input := lesson.Page{ID: "clone-p001", Title: title, Nodes: nodes}
			got := clonePage(input)
			cloneTestEqual(t, got, input)
			if title != nil {
				*got.Title += "изменено"
				if *got.Title == *title {
					t.Fatal("page title shares a pointer")
				}
			}
		}
	}
	input := lesson.Page{Nodes: make([]lesson.Node, 0, 1)}
	got := clonePage(input)
	input.Nodes = append(input.Nodes, lesson.Node{ID: "source"})
	got.Nodes = append(got.Nodes, lesson.Node{ID: "copy"})
	if input.Nodes[0].ID != "source" {
		t.Fatal("empty node slices share backing storage")
	}
}

func TestCloneNodeIndependentFlagsDataAndChildren(t *testing.T) {
	for _, direction := range []string{"copy", "input"} {
		t.Run(direction, func(t *testing.T) {
			input := cloneTestFixture(t).Document.Pages[0].Nodes[0]
			want := cloneTestFixture(t).Document.Pages[0].Nodes[0]
			first, second := cloneNode(input), cloneNode(input)
			cloneTestEqual(t, input, want)
			cloneTestEqual(t, first, want)
			cloneTestEqual(t, second, want)
			if direction == "copy" {
				cloneTestMutateNode(t, &first)
				cloneTestEqual(t, input, want)
			} else {
				cloneTestMutateNode(t, &input)
				cloneTestEqual(t, first, want)
			}
			cloneTestEqual(t, second, want)
		})
	}
}

func TestCloneNodeEmptySlicesAndAbsentFlags(t *testing.T) {
	value := cloneTestFixture(t)
	node := lesson.Node{ID: "clone+урок-n999", Type: lesson.NodeText,
		Data: make([]lesson.Data, 0, 1), Nodes: make([]lesson.Node, 0, 1)}
	value.Document.Pages[0].Nodes = []lesson.Node{node}
	cloneTestValid(t, value)
	got := cloneNode(node)
	cloneTestEqual(t, got, node)
	if got.Data == nil || got.Nodes == nil {
		t.Fatal("empty required arrays became nil")
	}
	node.Data = append(node.Data, lesson.Data{Hint: "source"})
	got.Data = append(got.Data, lesson.Data{Hint: "copy"})
	node.Nodes = append(node.Nodes, lesson.Node{ID: "source"})
	got.Nodes = append(got.Nodes, lesson.Node{ID: "copy"})
	if node.Data[0].Hint != "source" || node.Nodes[0].ID != "source" {
		t.Fatal("empty node arrays share storage")
	}
}

func TestCloneNodeNoArtificialDepthLimit(t *testing.T) {
	const depth = 512
	makeTree := func() lesson.Node {
		branch := lesson.Node{ID: fmt.Sprintf("clone+урок-n%03d", depth), Type: lesson.NodeText,
			Data: []lesson.Data{cloneTestText(t, cloneTestRole(t, lesson.RoleBody), "глубокий лист")}, Nodes: []lesson.Node{}}
		for index := depth - 1; index > 0; index-- {
			branch = lesson.Node{ID: fmt.Sprintf("clone+урок-n%03d", index), Type: lesson.NodeTask,
				Flags: lesson.NodeFlags{Speaking: cloneTestBool(t, false)}, Data: []lesson.Data{}, Nodes: []lesson.Node{branch}}
		}
		return branch
	}
	value := cloneTestFixture(t)
	input, want := makeTree(), makeTree()
	value.Document.Pages[0].Nodes = []lesson.Node{input}
	cloneTestValid(t, value)
	got := cloneNode(input)
	cloneTestEqual(t, got, want)
	leaf := &got
	for index := 1; index < depth; index++ {
		if len(leaf.Nodes) != 1 {
			t.Fatalf("tree cut off at depth %d", index)
		}
		leaf = &leaf.Nodes[0]
	}
	leaf.Data[0].Answers = append(leaf.Data[0].Answers, "copy only")
	leaf.Data[0].Value = "changed"
	cloneTestEqual(t, input, want)
}

func cloneTestMutatePage(t *testing.T, value *lesson.Page) {
	t.Helper()
	value.ID += "changed"
	if value.Title != nil {
		*value.Title += "changed"
	}
	for index := range value.Nodes {
		cloneTestMutateNode(t, &value.Nodes[index])
	}
	if len(value.Nodes) > 1 {
		value.Nodes[0], value.Nodes[1] = value.Nodes[1], value.Nodes[0]
	}
}

func cloneTestMutateNode(t *testing.T, value *lesson.Node) {
	t.Helper()
	value.ID += "changed"
	value.Type = lesson.NodeNote
	for _, flag := range []*bool{value.Flags.Speaking, value.Flags.Stepped, value.Flags.Checkable} {
		if flag != nil {
			*flag = !*flag
		}
	}
	for index := range value.Data {
		cloneTestMutateData(t, &value.Data[index])
	}
	for index := range value.Nodes {
		cloneTestMutateNode(t, &value.Nodes[index])
	}
	if len(value.Data) > 1 {
		value.Data[0], value.Data[1] = value.Data[1], value.Data[0]
	}
	if len(value.Nodes) > 1 {
		value.Nodes[0], value.Nodes[1] = value.Nodes[1], value.Nodes[0]
	}
}
