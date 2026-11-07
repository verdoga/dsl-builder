package structure

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestTask21QuestionTailFullAnswerAndOneHint(t *testing.T) {
	state := task21State()
	question := task21Entry(1, model.LineTypeTag, "question", nil)
	answer := task21Entry(2, model.LineTypeBlockStart, "answer", nil)
	hint := task21Entry(6, model.LineTypeBlockStart, "hint", nil)
	entries := []*fragments.Entry{question, answer, task21Entry(3, model.LineTypeContent, "", answer), task21Entry(4, model.LineTypeBlank, "", answer), task21Entry(5, model.LineTypeBlockEnd, "", answer), hint, task21Entry(7, model.LineTypeContent, "", hint), task21Entry(8, model.LineTypeBlockEnd, "", hint), task21Entry(9, model.LineTypeTag, "hint", nil)}
	before, _ := json.Marshal(entries)
	tail, after, errs := questionTail(state, entries, 0, 1)
	if len(errs) != 0 || after != 8 {
		t.Fatalf("after=%d ошибки=%v", after, errs)
	}
	task21Entries(t, tail, entries[1:8])
	afterSource, _ := json.Marshal(entries)
	if !slices.Equal(before, afterSource) {
		t.Fatal("исходные Entries изменены")
	}
	next, errs := appendContent(state, entries, 0)
	if len(errs) != 0 || next != 8 || len(state.bindings) != 1 {
		t.Fatalf("append question: %d %v", next, errs)
	}
	task21Entries(t, state.bindings[0].Entries, entries[:8])
	if next, errs = appendContent(state, entries, next); len(errs) != 0 || next != 9 || len(state.bindings) != 2 || state.pages[0].Nodes[1].Type != lesson.NodeHint {
		t.Fatalf("вторая подсказка потеряна: %d %v", next, errs)
	}
}

func TestTask21QuestionTailSkipsBlankAndCompleteEditor(t *testing.T) {
	state := task21State()
	question := task21Entry(1, model.LineTypeTag, "question", nil)
	editor := task21Entry(3, model.LineTypeBlockStart, "editor", nil)
	answer := task21Entry(6, model.LineTypeTag, "answer", nil)
	hint := task21Entry(9, model.LineTypeTag, "hint", nil)
	entries := []*fragments.Entry{question, task21Entry(2, model.LineTypeBlank, "", nil), editor, task21Entry(4, model.LineTypeTag, "newpage", editor), task21Entry(5, model.LineTypeBlockEnd, "", editor), answer, task21Entry(7, model.LineTypeBlank, "", nil), task21Entry(8, model.LineTypeTag, "editor", nil), hint}
	tail, after, errs := questionTail(state, entries, 0, 1)
	if len(errs) != 0 || after != len(entries) {
		t.Fatalf("after=%d ошибки=%v", after, errs)
	}
	var selected []*fragments.Entry
	for _, entry := range tail {
		if entry == answer || entry == hint {
			selected = append(selected, entry)
		}
	}
	task21Entries(t, selected, []*fragments.Entry{answer, hint})
}

func TestTask21QuestionTailNoAnswerLeavesHintStandalone(t *testing.T) {
	for _, gap := range []bool{false, true} {
		t.Run(map[bool]string{false: "рядом", true: "после_blank"}[gap], func(t *testing.T) {
			state := task21State()
			question := task21Entry(1, model.LineTypeTag, "question", nil)
			hint := task21Entry(3, model.LineTypeTag, "hint", nil)
			entries := []*fragments.Entry{question}
			if gap {
				entries = append(entries, task21Entry(2, model.LineTypeBlank, "", nil))
			}
			entries = append(entries, hint)
			tail, after, errs := questionTail(state, entries, 0, 1)
			if len(errs) != 0 || len(tail) != 0 || after != 1 {
				t.Fatalf("hint без answer поглощён: %v %d %v", tail, after, errs)
			}
			next, errs := appendContent(state, entries, 0)
			if len(errs) != 0 || next != 1 {
				t.Fatalf("question: %d %v", next, errs)
			}
			if next, errs := appendContent(state, entries, len(entries)-1); next != len(entries) || len(errs) != 0 {
				t.Fatalf("hint: %d %v", next, errs)
			}
			if len(state.bindings) != 2 || state.pages[0].Nodes[1].Type != lesson.NodeHint {
				t.Fatal("самостоятельный hint потерян")
			}
		})
	}
}

func TestTask21QuestionTailStopsAtEveryBoundary(t *testing.T) {
	for _, tag := range []string{"newpage", "task", "step", "variant", "variants", "endtask", "include", "text", "question"} {
		t.Run(tag, func(t *testing.T) {
			state := task21State()
			q := task21Entry(1, model.LineTypeTag, "question", nil)
			boundary := task21Entry(2, model.LineTypeTag, tag, nil)
			if tag == "include" {
				boundary.Included = []*fragments.Entry{task21Entry(10, model.LineTypeTag, "answer", boundary)}
			}
			entries := []*fragments.Entry{q, boundary, task21Entry(3, model.LineTypeTag, "answer", nil), task21Entry(4, model.LineTypeTag, "hint", nil)}
			tail, after, errs := questionTail(state, entries, 0, 1)
			if len(errs) != 0 || len(tail) != 0 || after != 1 {
				t.Fatalf("граница %s пересечена: %d %v", tag, after, errs)
			}
		})
	}
	t.Run("heading3", func(t *testing.T) {
		q := task21Entry(1, model.LineTypeTag, "question", nil)
		heading := task21Entry(2, model.LineTypeHeading, "", nil)
		level, title := "3", "Страница"
		heading.Source.Raw = "### Страница"
		heading.Source.Elements = []model.Element{{ElementType: model.ElementTypeHeadingLevel, Raw: "###", Value: &level, Start: 1, End: 4}, {ElementType: model.ElementTypeTitle, Raw: title, Value: &title, Start: 5, End: 13}}
		tail, after, errs := questionTail(task21State(), []*fragments.Entry{q, heading, task21Entry(3, model.LineTypeTag, "answer", nil)}, 0, 1)
		if len(errs) != 0 || len(tail) != 0 || after != 1 {
			t.Fatalf("страница пересечена: %d %v", after, errs)
		}
	})
	t.Run("block-end", func(t *testing.T) {
		parent := task21Entry(1, model.LineTypeBlockStart, "task", nil)
		q := task21Entry(2, model.LineTypeTag, "question", parent)
		entries := []*fragments.Entry{q, task21Entry(3, model.LineTypeBlockEnd, "", parent), task21Entry(4, model.LineTypeTag, "answer", nil)}
		tail, after, errs := questionTail(task21State(), entries, 0, 1)
		if len(errs) != 0 || len(tail) != 0 || after != 1 {
			t.Fatalf("block-end пересечён: %d %v", after, errs)
		}
	})
}

func TestTask21QuestionTailRequiresSameArea(t *testing.T) {
	for _, area := range []string{"task", "step", "variant"} {
		t.Run(area, func(t *testing.T) {
			state := task21State()
			outerTag := "task"
			outerKind := lesson.NodeTask
			if area == "variant" {
				outerTag = "variants"
				outerKind = lesson.NodeVariants
			}
			outer := task21Entry(1, model.LineTypeBlockStart, outerTag, nil)
			a := appendNode(state, nil, outerKind, []*fragments.Entry{outer})
			state.frames = []frame{{binding: a}}
			parent := outer
			if area != "task" {
				parent = task21Entry(2, model.LineTypeTag, area, outer)
				kind := lesson.NodeStep
				if area == "variant" {
					kind = lesson.NodeVariant
				}
				b := appendNode(state, []int{0}, kind, []*fragments.Entry{parent})
				state.frames = append(state.frames, frame{binding: b})
			}
			q := task21Entry(3, model.LineTypeTag, "question", parent)
			answer := task21Entry(4, model.LineTypeTag, "answer", nil)
			entries := []*fragments.Entry{q, answer, task21Entry(5, model.LineTypeTag, "hint", nil)}
			tail, after, errs := questionTail(state, entries, 0, 1)
			if len(errs) != 0 || len(tail) != 0 || after != 1 {
				t.Fatalf("чужая область %s: %d %v", area, after, errs)
			}
			answer.Parent = parent
			answer.Source.ParentLine = q.Source.ParentLine
			answer.Source.NestingLevel = q.Source.NestingLevel
			tail, after, errs = questionTail(state, entries, 0, 1)
			if len(errs) != 0 || after != 2 {
				t.Fatalf("answer своей области: %d %v", after, errs)
			}
			task21Entries(t, tail, entries[1:2])
		})
	}
}

func TestTask21QuestionTailSameTaskParentUsesActiveStep(t *testing.T) {
	state := task21State()
	task := task21Entry(1, model.LineTypeTag, "task", nil)
	step := task21Entry(2, model.LineTypeTag, "step", task)
	a := appendNode(state, nil, lesson.NodeTask, []*fragments.Entry{task})
	b := appendNode(state, []int{0}, lesson.NodeStep, []*fragments.Entry{step})
	state.frames = []frame{{binding: a}, {binding: b}}
	q := task21Entry(3, model.LineTypeTag, "question", task)
	answer := task21Entry(4, model.LineTypeTag, "answer", task)
	hint := task21Entry(5, model.LineTypeTag, "hint", task)
	entries := []*fragments.Entry{q, answer, hint}
	tail, after, errs := questionTail(state, entries, 0, 1)
	if len(errs) != 0 || after != 3 {
		t.Fatalf("общий Parent task: %d %v", after, errs)
	}
	task21Entries(t, tail, entries[1:])
}

func TestTask21NextSignificantOpaqueEditorAndBoundaries(t *testing.T) {
	editor := task21Entry(2, model.LineTypeBlockStart, "editor", nil)
	entries := []*fragments.Entry{task21Entry(1, model.LineTypeBlank, "", nil), editor, task21Entry(3, model.LineTypeTag, "answer", editor), task21Entry(4, model.LineTypeBlockEnd, "", editor), task21Entry(5, model.LineTypeTag, "editor", nil), task21Entry(6, model.LineTypeBlank, "", nil), task21Entry(7, model.LineTypeTag, "hint", nil)}
	before, _ := json.Marshal(entries)
	next, errs := nextSignificant(task21State(), entries, 0)
	if len(errs) != 0 || next != 6 {
		t.Fatalf("next=%d ошибки=%v", next, errs)
	}
	after, _ := json.Marshal(entries)
	if !slices.Equal(before, after) {
		t.Fatal("поиск изменил вход")
	}
	if next, errs := nextSignificant(task21State(), entries, len(entries)); next != len(entries) || len(errs) != 0 {
		t.Fatalf("конец: %d %v", next, errs)
	}
	for _, tag := range []string{"include", "newpage", "task", "step", "variant", "answer"} {
		t.Run(tag, func(t *testing.T) {
			boundary := task21Entry(8, model.LineTypeTag, tag, nil)
			boundary.Included = nil
			if tag == "include" {
				boundary.Included = []*fragments.Entry{}
			}
			got, errs := nextSignificant(task21State(), []*fragments.Entry{entries[0], boundary, entries[6]}, 0)
			if len(errs) != 0 || got != 1 {
				t.Fatalf("граница %s пропущена: %d %v", tag, got, errs)
			}
		})
	}
	end := task21Entry(8, model.LineTypeBlockEnd, "", editor)
	if next, errs := nextSignificant(task21State(), []*fragments.Entry{entries[0], end}, 0); len(errs) != 0 || next != 1 {
		t.Fatalf("block-end: %d %v", next, errs)
	}
	only := entries[:6]
	if next, errs := nextSignificant(task21State(), only, 0); len(errs) != 0 || next != len(only) {
		t.Fatalf("только blank/editor: %d %v", next, errs)
	}
}
