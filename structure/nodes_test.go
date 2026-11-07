package structure

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task21Entry(number int, kind model.LineType, tag string, parent *fragments.Entry) *fragments.Entry {
	e := &fragments.Entry{Parent: parent, Source: model.Line{Number: number, LineType: kind, LineEnding: model.LineEndingLF, Elements: []model.Element{}}}
	if parent != nil {
		n := parent.Source.Number
		e.Source.ParentLine = &n
		e.Source.NestingLevel = parent.Source.NestingLevel + 1
	}
	add := func(kind model.ElementType, raw, value string) {
		start := len([]rune(e.Source.Raw)) + 1
		e.Source.Raw += raw
		e.Source.Elements = append(e.Source.Elements, model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: start + len([]rune(raw)), ErrorIDs: []string{}})
	}
	if tag != "" {
		add(model.ElementTypeTag, "@"+strings.ToUpper(tag), tag)
	}
	parameter := func(kind model.ElementType, value string) { e.Source.Raw += " "; add(kind, value, value) }
	switch tag {
	case "task", "include":
		parameter(model.ElementTypeIdentifier, "A")
	case "step", "header":
		parameter(model.ElementTypeTitle, "Название")
	case "variant":
		parameter(model.ElementTypeName, "Ветка")
	case "media":
		parameter(model.ElementTypeMediaType, "audio")
		parameter(model.ElementTypeSource, "001")
	}
	switch kind {
	case model.LineTypeBlockStart:
		e.Source.Raw += " "
		add(model.ElementTypeBlockOpen, "{", "{")
	case model.LineTypeBlockEnd:
		add(model.ElementTypeBlockClose, "}", "}")
	case model.LineTypeContent:
		add(model.ElementTypeContent, "текст", "текст")
	}
	return e
}

func task21Content(entry *fragments.Entry, value string) {
	start := len([]rune(entry.Source.Raw)) + 2
	entry.Source.Raw += " " + value
	entry.Source.Elements = append(entry.Source.Elements, model.Element{ElementType: model.ElementTypeContent, Raw: value, Value: &value, Start: start, End: start + len([]rune(value)), ErrorIDs: []string{}})
}

func task21State() *builder {
	id := "TASK21"
	return &builder{page: -1, input: &parsed.Input{Path: "/source/task21.json", Result: model.Result{Format: model.FormatVersion1, Document: model.Document{Metadata: model.DocumentMetadata{DocumentID: &id}}}}}
}

func task21Entries(t *testing.T, got, want []*fragments.Entry) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("проекция Entry: %p; нужна %p", got, want)
	}
}

func TestTask21ContentTypeSemanticMapping(t *testing.T) {
	cases := []struct {
		tag  string
		want lesson.NodeType
	}{
		{"header", lesson.NodeHeading}, {"instr", lesson.NodeInstruction}, {"key", lesson.NodeAnswerKey}, {"wordlist", lesson.NodeWordList},
		{"choice", lesson.NodeSingleChoice}, {"multichoice", lesson.NodeMultipleChoice}, {"text", lesson.NodeText}, {"note", lesson.NodeNote},
		{"alt", lesson.NodeAlt}, {"script", lesson.NodeScript}, {"hint", lesson.NodeHint}, {"example", lesson.NodeExample}, {"table", lesson.NodeTable},
		{"media", lesson.NodeMedia}, {"question", lesson.NodeQuestion}, {"multifill", lesson.NodeMultifill}, {"matching", lesson.NodeMatching}, {"ordering", lesson.NodeOrdering},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			line := task21Entry(1, model.LineTypeTag, tc.tag, nil).Source
			got, err := contentType(line, task21State())
			if err != nil || got != tc.want {
				t.Fatalf("contentType = %q, %v; нужно %q", got, err, tc.want)
			}
		})
	}
	line := task21Entry(1, model.LineTypeContent, "", nil).Source
	got, err := contentType(line, task21State())
	if err != nil || got != lesson.NodeText {
		t.Fatalf("content = %q, %v", got, err)
	}
	for _, tag := range []string{"answer", "endtask", "include"} {
		t.Run("source_"+tag, func(t *testing.T) {
			_, err := contentType(task21Entry(1, model.LineTypeTag, tag, nil).Source, task21State())
			if err == nil || err.Code != CodeSource {
				t.Fatalf("непредставимая форма: %v", err)
			}
		})
	}
}

func TestTask21AppendNodeStablePathsAndEmptySkeleton(t *testing.T) {
	state := task21State()
	root := task21Entry(1, model.LineTypeTag, "task", nil)
	if got := appendNode(state, nil, lesson.NodeTask, []*fragments.Entry{root}); got != 0 {
		t.Fatalf("binding = %d", got)
	}
	parent := []int{0}
	for i := range 1005 {
		entry := task21Entry(i+2, model.LineTypeContent, "", root)
		got := appendNode(state, parent, lesson.NodeText, []*fragments.Entry{entry})
		if got != i+1 {
			t.Fatalf("binding = %d; нужен %d", got, i+1)
		}
	}
	parent[0] = 99
	if len(state.pages) != 1 || len(state.pages[0].Nodes) != 1 || len(state.pages[0].Nodes[0].Nodes) != 1005 || len(state.bindings) != 1006 {
		t.Fatal("одна привязка на каждую ноду и правильный родитель не сохранены")
	}
	for i, binding := range state.bindings {
		want := []int{0}
		node := &state.pages[0].Nodes[0]
		if i > 0 {
			want = []int{0, i - 1}
			node = &node.Nodes[i-1]
		}
		if binding.Page != 0 || !slices.Equal(binding.NodePath, want) {
			t.Fatalf("путь %d = %v", i, binding)
		}
		if node.ID != "" || node.Data == nil || len(node.Data) != 0 || node.Nodes == nil || node.Flags != (lesson.NodeFlags{}) {
			t.Fatalf("каркас %d = %+v", i, node)
		}
		if binding.Entries[0].Source.Number != i+1 {
			t.Fatalf("источник %d", i)
		}
	}
	state.pages = append(state.pages, lesson.Page{Nodes: []lesson.Node{}})
	state.page = 1
	appendNode(state, nil, lesson.NodeHint, []*fragments.Entry{task21Entry(1007, model.LineTypeTag, "hint", nil)})
	last := state.bindings[len(state.bindings)-1]
	if last.Page != 1 || !slices.Equal(last.NodePath, []int{0}) {
		t.Fatalf("другая страница: %+v", last)
	}
}

func TestTask21AppendContentConsumesWholeLeaf(t *testing.T) {
	for _, tag := range []string{"text", "table", "multifill", "choice", "multichoice", "matching", "ordering", "instr"} {
		t.Run(tag, func(t *testing.T) {
			state := task21State()
			decl := task21Entry(1, model.LineTypeBlockStart, tag, nil)
			entries := []*fragments.Entry{decl, task21Entry(2, model.LineTypeContent, "", decl), task21Entry(3, model.LineTypeBlank, "", decl)}
			number := 4
			if tag == "choice" || tag == "multichoice" || tag == "matching" || tag == "ordering" {
				entries = append(entries, task21Entry(number, model.LineTypeTag, "answer", decl))
				number++
			}
			if tag == "instr" || tag == "choice" || tag == "multichoice" || tag == "matching" || tag == "ordering" {
				hint := task21Entry(number, model.LineTypeBlockStart, "hint", decl)
				entries = append(entries, hint, task21Entry(number+1, model.LineTypeContent, "", hint), task21Entry(number+2, model.LineTypeBlockEnd, "", hint))
				number += 3
			}
			entries = append(entries, task21Entry(number, model.LineTypeBlockEnd, "", decl))
			stop := len(entries)
			entries = append(entries, task21Entry(number+1, model.LineTypeContent, "", nil))
			next, errs := appendContent(state, entries, 0)
			if len(errs) != 0 || next != stop || len(state.bindings) != 1 {
				t.Fatalf("next=%d, bindings=%d, ошибки=%v", next, len(state.bindings), errs)
			}
			task21Entries(t, state.bindings[0].Entries, entries[:stop])
			node := state.pages[0].Nodes[0]
			if len(node.Nodes) != 0 || len(node.Data) != 0 || node.Flags != (lesson.NodeFlags{}) {
				t.Fatalf("лист сформировал данные/детей: %+v", node)
			}
		})
	}
}

func TestTask21AppendContentStandaloneOrdering(t *testing.T) {
	state := task21State()
	entries := []*fragments.Entry{task21Entry(1, model.LineTypeContent, "", nil), task21Entry(2, model.LineTypeContent, "", nil), task21Entry(3, model.LineTypeTag, "media", nil), task21Entry(4, model.LineTypeTag, "script", nil), task21Entry(5, model.LineTypeTag, "hint", nil), task21Entry(6, model.LineTypeTag, "hint", nil)}
	want := []lesson.NodeType{lesson.NodeText, lesson.NodeText, lesson.NodeMedia, lesson.NodeScript, lesson.NodeHint, lesson.NodeHint}
	for at := range entries {
		next, errs := appendContent(state, entries, at)
		if len(errs) != 0 || next != at+1 {
			t.Fatalf("at=%d next=%d ошибки=%v", at, next, errs)
		}
	}
	if len(state.bindings) != len(want) {
		t.Fatalf("bindings=%d", len(state.bindings))
	}
	for i, kind := range want {
		if state.pages[0].Nodes[i].Type != kind {
			t.Fatalf("нода %d", i)
		}
		task21Entries(t, state.bindings[i].Entries, entries[i:i+1])
	}
}

func TestTask21AppendContentInstructionPairSameNearestParent(t *testing.T) {
	for _, area := range []string{"step", "variant"} {
		for _, tag := range []string{"multifill", "ordering"} {
			t.Run(fmt.Sprint(area, "/", tag), func(t *testing.T) {
				state := task21State()
				outerTag := "task"
				outerKind := lesson.NodeTask
				innerKind := lesson.NodeStep
				if area == "variant" {
					outerTag = "variants"
					outerKind = lesson.NodeVariants
					innerKind = lesson.NodeVariant
				}
				outer := task21Entry(1, model.LineTypeBlockStart, outerTag, nil)
				inner := task21Entry(2, model.LineTypeTag, area, outer)
				a := appendNode(state, nil, outerKind, []*fragments.Entry{outer})
				b := appendNode(state, []int{0}, innerKind, []*fragments.Entry{inner})
				state.frames = []frame{{binding: a}, {binding: b}}
				instr := task21Entry(3, model.LineTypeTag, "instr", outer)
				task21Content(instr, "совпадает")
				decl := task21Entry(4, model.LineTypeBlockStart, tag, outer)
				task21Content(decl, "совпадает")
				entries := []*fragments.Entry{instr, decl, task21Entry(5, model.LineTypeContent, "", decl), task21Entry(6, model.LineTypeBlockEnd, "", decl)}
				if next, errs := appendContent(state, entries, 0); next != 1 || len(errs) != 0 {
					t.Fatalf("instr: %d %v", next, errs)
				}
				if next, errs := appendContent(state, entries, 1); next != len(entries) || len(errs) != 0 {
					t.Fatalf("pair: %d %v", next, errs)
				}
				want := []lesson.NodeType{lesson.NodeInstruction, lesson.NodeInstruction, lesson.NodeType(tag)}
				children := state.pages[0].Nodes[0].Nodes[0].Nodes
				if len(children) != 3 || len(state.bindings) != 5 {
					t.Fatalf("children=%d bindings=%d", len(children), len(state.bindings))
				}
				for i, kind := range want {
					if children[i].Type != kind || !slices.Equal(state.bindings[i+2].NodePath, []int{0, 0, i}) {
						t.Fatalf("неверный родитель/тип пары %d", i)
					}
				}
				task21Entries(t, state.bindings[3].Entries, entries[1:2])
				task21Entries(t, state.bindings[4].Entries, entries[1:])
			})
		}
	}
}
