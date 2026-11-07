package structure_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/structure"
)

type task31EntrySnapshot struct {
	entry    *fragments.Entry
	source   string
	parent   *fragments.Entry
	included []*fragments.Entry
}

type task31GraphSnapshot struct {
	roots []*fragments.Entry
	nodes []task31EntrySnapshot
}

// task31SnapshotEntries сохраняет значения и точные связи графа с циклами include.
func task31SnapshotEntries(t *testing.T, entries []*fragments.Entry) task31GraphSnapshot {
	t.Helper()
	snapshot := task31GraphSnapshot{roots: slices.Clone(entries)}
	visited := make(map[*fragments.Entry]bool)
	var visit func(*fragments.Entry)
	visit = func(entry *fragments.Entry) {
		if entry == nil || visited[entry] {
			return
		}
		visited[entry] = true
		source, err := json.Marshal(entry.Source)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.nodes = append(snapshot.nodes, task31EntrySnapshot{
			entry: entry, source: string(source), parent: entry.Parent, included: slices.Clone(entry.Included),
		})
		visit(entry.Parent)
		for _, included := range entry.Included {
			visit(included)
		}
	}
	for _, entry := range entries {
		visit(entry)
	}
	return snapshot
}

func task31AssertEntriesUnchanged(t *testing.T, before task31GraphSnapshot, entries []*fragments.Entry) {
	t.Helper()
	after := task31SnapshotEntries(t, entries)
	if (before.roots == nil) != (after.roots == nil) || !slices.Equal(before.roots, after.roots) {
		t.Fatal("Build changed expanded entry roots or their order")
	}
	if !slices.EqualFunc(before.nodes, after.nodes, func(a, b task31EntrySnapshot) bool {
		return a.entry == b.entry && a.source == b.source && a.parent == b.parent &&
			(a.included == nil) == (b.included == nil) && slices.Equal(a.included, b.included)
	}) {
		t.Fatal("Build mutated expanded entry sources, identities, or ordered links")
	}
}

func TestTask26BuildRepeatedConcurrentAndDoesNotMutateInputs(t *testing.T) {
	input := task26Load(t, []model.Line{
		task26Tag(1, 0, model.LineTypeBlockStart, "fragment", task26Element(model.ElementTypeIdentifier, "body")),
		task26Tag(2, 1, model.LineTypeBlockStart, "multifill", task26Element(model.ElementTypeContent, "Do it")),
		task26Text(3, 2, "Слово _____{ответ}"), task26End(4, 2), task26End(5, 1),
		task26Tag(6, 0, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "body")),
		task26Tag(7, 0, model.LineTypeTag, "include", task26Element(model.ElementTypeIdentifier, "body")),
	})
	entries := task26Expand(t, input)
	beforeInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries := task31SnapshotEntries(t, entries)
	pages, bindings, problems := structure.Build(input, entries)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	task26Invariants(t, pages, bindings)
	expected, err := json.Marshal(pages)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		again, againBindings, problems := structure.Build(input, entries)
		if len(problems) != 0 {
			t.Fatal(problems)
		}
		task26Invariants(t, again, againBindings)
		raw, err := json.Marshal(again)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(expected) {
			t.Fatal("repeat is nondeterministic")
		}
		again[0].Nodes[0].ID = "caller mutation"
		again[0].Nodes[0].Data = append(again[0].Nodes[0].Data, lesson.Data{Value: "changed"})
		againBindings[0].NodePath[0] = 999
	}
	if pages[0].Nodes[0].ID != "" || len(pages[0].Nodes[0].Data) != 0 || bindings[0].NodePath[0] != 0 {
		t.Fatal("Build outputs alias")
	}
	var group sync.WaitGroup
	failures := make(chan string, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			result, links, errors := structure.Build(input, entries)
			if len(errors) != 0 {
				failures <- fmt.Sprint(errors)
				return
			}
			raw, err := json.Marshal(result)
			if err != nil || string(raw) != string(expected) || len(links) != len(bindings) {
				failures <- "concurrent result differs"
			}
		}()
	}
	group.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	afterInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeInput) != string(afterInput) {
		t.Fatal("Build mutated input")
	}
	task31AssertEntriesUnchanged(t, beforeEntries, entries)
}

func TestTask26NoArtificialNodePageOrDepthLimits(t *testing.T) {
	t.Run("wide pages and nodes", func(t *testing.T) {
		lines := make([]model.Line, 0, 2010)
		for i := 0; i < 1005; i++ {
			lines = append(lines, task26Heading(2*i+1, "3", fmt.Sprint(i)), task26Text(2*i+2, 0, "body"))
		}
		input := task26Load(t, lines)
		pages, bindings, problems := structure.Build(input, task26Expand(t, input))
		if len(problems) != 0 || len(pages) != 1005 || len(bindings) != 1005 {
			t.Fatalf("pages=%d bindings=%d errors=%v", len(pages), len(bindings), problems)
		}
		task26Invariants(t, pages, bindings)
		if bindings[1004].Page != 1004 || bindings[1004].Node(pages).Type != lesson.NodeText {
			t.Fatal("large page address truncated")
		}
	})
	t.Run("deep semantic tasks", func(t *testing.T) {
		const depth = 128
		lines := make([]model.Line, 0, depth+1)
		for i := 1; i <= depth; i++ {
			lines = append(lines, task26Tag(i, i-1, model.LineTypeTag, "task", task26Element(model.ElementTypeIdentifier, "same")))
		}
		lines = append(lines, task26Text(depth+1, depth, "deep leaf"))
		input := task26Load(t, lines)
		pages, bindings, problems := structure.Build(input, task26Expand(t, input))
		if len(problems) != 0 || len(bindings) != depth+1 {
			t.Fatalf("bindings=%d errors=%v", len(bindings), problems)
		}
		task26Invariants(t, pages, bindings)
		deepest := bindings[len(bindings)-1]
		if len(deepest.NodePath) != depth+1 || deepest.Node(pages).Type != lesson.NodeText {
			t.Fatal("deep binding path truncated")
		}
	})
}
