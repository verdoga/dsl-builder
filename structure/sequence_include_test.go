package structure

import (
	"testing"

	"github.com/verdoga/dsl-builder/fragments"
)

func TestTask24IncludePreservesOuterTaskStepVariantAndClosesInnerTask(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("tag", 0, "task", "identifier", "outer"),
		task24Line("tag", 1, "step", "title", "outer step"),
		task24Line("block-start", 1, "variants"),
		task24Line("tag", 3, "variant", "name", "outer branch"),
		task24Line("tag", 3, "include", "identifier", "F"),
		task24Line("tag", 3, "note", "content", "after include"),
		task24Line("block-end", 3, ""),
		task24Line("tag", 1, "note", "content", "after variants"),
		task24Line("block-start", 0, "fragment", "identifier", "F"),
		task24Line("tag", 9, "note", "content", "inside outer branch"),
		task24Line("tag", 9, "task", "identifier", "inner"),
		task24Line("tag", 11, "step", "title", "inner step"),
		task24Line("tag", 11, "note", "content", "inside inner"),
		task24Line("block-end", 9, ""))
	state := task24State(input)
	task24VisitPrefix(t, state, entries, 4)
	if len(state.frames) != 4 {
		t.Fatalf("outer frames=%d", len(state.frames))
	}
	task24NoErrors(t, walkInclude(state, entries[4]))
	if len(state.frames) != 4 {
		t.Fatalf("include leaked or closed frames: %+v", state.frames)
	}
	task24VisitPrefix(t, state, entries[5:], len(entries)-5)
	task24Paths(t, state,
		"0:[0]:task", "0:[0 0]:step", "0:[0 0 0]:variants", "0:[0 0 0 0]:variant",
		"0:[0 0 0 0 0]:note", "0:[0 0 0 0 1]:task", "0:[0 0 0 0 1 0]:step",
		"0:[0 0 0 0 1 0 0]:note", "0:[0 0 0 0 2]:note", "0:[0 0 1]:note")
	if state.bindings[4].Entries[0] != entries[4].Included[0] {
		t.Fatal("included source identity changed")
	}
	if len(state.frames) != 2 {
		t.Fatalf("variants close affected task/step: %+v", state.frames)
	}
	task24NoErrors(t, walkSequence(state, nil, nil))
	if len(state.frames) != 0 {
		t.Fatal("end document did not close all frames")
	}
}

func TestTask24IncludeDoesNotResurrectOuterContextAfterRealBoundary(t *testing.T) {
	for _, boundary := range []string{"endtask", "newpage"} {
		t.Run(boundary, func(t *testing.T) {
			input, entries := task24Fixture(t,
				task24Line("tag", 0, "task", "identifier", "outer"),
				task24Line("tag", 1, "step", "title", "s"),
				task24Line("tag", 1, "include", "identifier", "F"),
				task24Line("tag", 1, "note", "content", "after include"),
				task24Line("block-start", 0, "fragment", "identifier", "F"),
				task24Line("tag", 5, boundary),
				task24Line("tag", 5, "note", "content", "after boundary"),
				task24Line("block-end", 5, ""))
			state := task24State(input)
			task24VisitPrefix(t, state, entries, 2)
			task24NoErrors(t, walkInclude(state, entries[2]))
			if len(state.frames) != 0 {
				t.Fatalf("closed outer frames resurrected: %+v", state.frames)
			}
			task24VisitPrefix(t, state, entries[3:], len(entries)-3)
			if boundary == "newpage" {
				task24Paths(t, state, "0:[0]:task", "0:[0 0]:step", "1:[0]:note", "1:[1]:note")
			} else {
				task24Paths(t, state, "0:[0]:task", "0:[0 0]:step", "0:[1]:note", "0:[2]:note")
			}
		})
	}
}

func TestTask24RepeatedIncludesRetainOwnEntryIdentities(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("tag", 0, "include", "identifier", "F"),
		task24Line("tag", 0, "include", "identifier", "F"),
		task24Line("block-start", 0, "fragment", "identifier", "F"),
		task24Line("tag", 3, "task", "identifier", "same"),
		task24Line("tag", 4, "note", "content", "body"),
		task24Line("block-end", 3, ""))
	state := task24State(input)
	task24NoErrors(t, walkSequence(state, entries, nil))
	task24Paths(t, state, "0:[0]:task", "0:[0 0]:note", "0:[1]:task", "0:[1 0]:note")
	first, second := state.bindings[0].Entries[0], state.bindings[2].Entries[0]
	if first == second || first.Source.Number != second.Source.Number || first.Parent != entries[0] || second.Parent != entries[1] {
		t.Fatal("repeat occurrences merged or renumbered")
	}
	if state.bindings[1].Entries[0].Parent != first || state.bindings[3].Entries[0].Parent != second {
		t.Fatal("body linked to wrong occurrence")
	}
	if len(state.frames) != 0 {
		t.Fatal("include task leaked")
	}
}

func TestTask24IncludeSeparatesQuestionAnswerHintAdjacency(t *testing.T) {
	t.Run("answer inside cannot attach outside", func(t *testing.T) {
		input, entries := task24Fixture(t,
			task24Line("tag", 0, "question", "content", "outside"),
			task24Line("tag", 0, "include", "identifier", "F"),
			task24Line("block-start", 0, "fragment", "identifier", "F"),
			task24Line("tag", 3, "answer", "content", "inside"),
			task24Line("block-end", 3, ""))
		state := task24State(input)
		_, problems := visitEntry(state, entries, 0, nil)
		task24NoErrors(t, problems)
		if len(state.bindings[0].Entries) != 1 {
			t.Fatal("question consumed include answer")
		}
		problems = walkInclude(state, entries[1])
		task24Code(t, problems, CodeContext, input, 4)
	})
	t.Run("answer outside cannot attach inside", func(t *testing.T) {
		input, entries := task24Fixture(t,
			task24Line("tag", 0, "include", "identifier", "F"),
			task24Line("tag", 0, "answer", "content", "outside"),
			task24Line("block-start", 0, "fragment", "identifier", "F"),
			task24Line("tag", 3, "question", "content", "inside"),
			task24Line("block-end", 3, ""))
		state := task24State(input)
		task24NoErrors(t, walkInclude(state, entries[0]))
		if len(state.bindings[0].Entries) != 1 {
			t.Fatal("included question crossed include end")
		}
		_, problems := visitEntry(state, entries, 1, nil)
		task24Code(t, problems, CodeContext, input, 2)
	})
	t.Run("hint after empty include stays standalone", func(t *testing.T) {
		input, entries := task24Fixture(t,
			task24Line("tag", 0, "question", "content", "q"),
			task24Line("tag", 0, "answer", "content", "a"),
			task24Line("tag", 0, "include", "identifier", "F"),
			task24Line("tag", 0, "hint", "content", "h"),
			task24Line("block-start", 0, "fragment", "identifier", "F"),
			task24Line("block-end", 5, ""))
		state := task24State(input)
		task24NoErrors(t, walkSequence(state, entries, nil))
		task24Paths(t, state, "0:[0]:question", "0:[1]:hint")
		if len(state.bindings[0].Entries) != 2 || state.bindings[1].Entries[0] != entries[3] {
			t.Fatal("hint crossed include boundary")
		}
	})
}

func TestTask24CloseBlockClosesExactInnerContainerOnly(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("tag", 0, "task", "identifier", "outer"),
		task24Line("tag", 1, "step", "title", "s"),
		task24Line("block-start", 1, "variants"),
		task24Line("tag", 3, "variant", "name", "outer"),
		task24Line("block-start", 3, "variants"),
		task24Line("tag", 5, "variant", "name", "inner"),
		task24Line("tag", 5, "task", "identifier", "inner"),
		task24Line("block-end", 5, ""),
		task24Line("tag", 3, "note", "content", "outer branch"),
		task24Line("block-end", 3, ""),
		task24Line("tag", 1, "note", "content", "outer step"))
	state := task24State(input)
	task24VisitPrefix(t, state, entries, 7)
	if len(state.frames) != 7 {
		t.Fatalf("frames before close=%d", len(state.frames))
	}
	// Одинаковый физический номер чужого вхождения не закрывает наш контейнер.
	foreign := *entries[4]
	foreignEnd := *entries[7]
	foreignEnd.Parent = &foreign
	_ = closeBlock(state, &foreignEnd)
	if len(state.frames) != 7 {
		t.Fatal("foreign occurrence closed active container")
	}
	task24NoErrors(t, closeBlock(state, entries[7]))
	if len(state.frames) != 4 {
		t.Fatalf("inner close left %d frames", len(state.frames))
	}
	task24VisitPrefix(t, state, entries[8:], len(entries)-8)
	task24Paths(t, state, "0:[0]:task", "0:[0 0]:step", "0:[0 0 0]:variants", "0:[0 0 0 0]:variant",
		"0:[0 0 0 0 0]:variants", "0:[0 0 0 0 0 0]:variant", "0:[0 0 0 0 0 0 0]:task",
		"0:[0 0 0 0 1]:note", "0:[0 0 1]:note")
	if len(state.frames) != 2 {
		t.Fatalf("outer close damaged task/step: %+v", state.frames)
	}
}

func TestTask24BlockEntriesCannotCloseWithAnotherIncludeOccurrence(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("tag", 0, "include", "identifier", "F"),
		task24Line("tag", 0, "include", "identifier", "F"),
		task24Line("block-start", 0, "fragment", "identifier", "F"),
		task24Line("block-start", 3, "note"),
		task24Line("content", 4, "", "content", "{ }"),
		task24Line("block-end", 4, ""),
		task24Line("block-end", 3, ""))
	first, second := entries[0].Included, entries[1].Included
	// Локальный guard: та же физическая граница другого включения не подходит.
	local := []*fragments.Entry{first[0], first[1], second[2]}
	_, _, problems := blockEntries(local, 0, task24State(input))
	if len(problems) == 0 || problems[0].Code != CodeBoundary {
		t.Fatalf("foreign close: %v", problems)
	}
}
