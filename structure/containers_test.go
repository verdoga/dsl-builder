package structure

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task9State(t *testing.T) *builder {
	t.Helper()
	path, err := filepath.Abs("../golden/7. parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	input, problems := parsed.Load(path)
	if len(problems) != 0 {
		t.Fatalf("Load: %v", problems)
	}
	return &builder{input: input, page: -1}
}

// task9Entry создаёт типизированное вхождение, не разбирая DSL или Raw.
func task9Entry(tag string, number int, parent *fragments.Entry) *fragments.Entry {
	entry := &fragments.Entry{Parent: parent, Source: model.Line{
		Number: number, LineType: model.LineTypeTag, LineEnding: model.LineEndingLF,
		Elements: []model.Element{},
	}}
	add := func(kind model.ElementType, raw, value string) {
		if entry.Source.Raw != "" {
			entry.Source.Raw += " "
		}
		start := len([]rune(entry.Source.Raw)) + 1
		entry.Source.Raw += raw
		entry.Source.Elements = append(entry.Source.Elements, model.Element{
			ElementType: kind, Raw: raw, Value: &value, Start: start,
			End: start + len([]rune(raw)), ErrorIDs: []string{},
		})
	}
	add(model.ElementTypeTag, "@"+tag, tag)
	switch tag {
	case "task":
		add(model.ElementTypeIdentifier, "same", "same")
	case "step":
		add(model.ElementTypeTitle, "stage", "stage")
	case "variant":
		add(model.ElementTypeName, "branch", "branch")
	case "variants":
		entry.Source.LineType = model.LineTypeBlockStart
		add(model.ElementTypeBlockOpen, "{", "{")
	}
	if parent != nil {
		number := parent.Source.Number
		entry.Source.ParentLine = &number
		entry.Source.NestingLevel = parent.Source.NestingLevel + 1
	}
	return entry
}

func task9OK(t *testing.T, problems []*diagnostic.Error) {
	t.Helper()
	if len(problems) != 0 {
		t.Fatalf("unexpected diagnostics: %v", problems)
	}
}

func task9Path(t *testing.T, state *builder, entry *fragments.Entry, kind lesson.NodeType, want ...int) {
	t.Helper()
	for _, binding := range state.bindings {
		if len(binding.Entries) == 1 && binding.Entries[0] == entry {
			if !slices.Equal(binding.NodePath, want) || binding.Node(state.pages).Type != kind {
				t.Fatalf("line %d: path %v type %s, want %v %s", entry.Source.Number,
					binding.NodePath, binding.Node(state.pages).Type, want, kind)
			}
			return
		}
	}
	t.Fatalf("no binding for Entry %p (line %d)", entry, entry.Source.Number)
}

func TestTask9TaskPeerAndAncestorIdentity(t *testing.T) {
	s := task9State(t)
	outer := task9Entry("task", 1, nil)
	step := task9Entry("step", 2, outer)
	first := task9Entry("task", 3, outer)
	second := task9Entry("task", 4, outer)
	peer := task9Entry("task", 5, nil)
	// Как visitEntry, перед обработчиком подготавливаем семантические контексты.
	trimContexts(s, outer)
	task9OK(t, openTask(s, outer, nil))
	trimContexts(s, step)
	task9OK(t, openStep(s, step, nil))
	trimContexts(s, first)
	task9OK(t, openTask(s, first, nil))
	trimContexts(s, second)
	task9OK(t, openTask(s, second, nil))
	trimContexts(s, peer)
	task9OK(t, openTask(s, peer, nil))
	task9Path(t, s, outer, lesson.NodeTask, 0)
	task9Path(t, s, step, lesson.NodeStep, 0, 0)
	task9Path(t, s, first, lesson.NodeTask, 0, 0, 0)
	task9Path(t, s, second, lesson.NodeTask, 0, 0, 1)
	task9Path(t, s, peer, lesson.NodeTask, 1)
}

func TestTask9SameBranchTaskClosesOnlyItsPeer(t *testing.T) {
	s := task9State(t)
	outer := task9Entry("task", 1, nil)
	step := task9Entry("step", 2, outer)
	set := task9Entry("variants", 3, outer)
	a := task9Entry("variant", 4, set)
	first := task9Entry("task", 5, set)
	innerStep := task9Entry("step", 6, first)
	second := task9Entry("task", 7, set)
	b := task9Entry("variant", 8, set)
	third := task9Entry("task", 9, set)
	task9OK(t, openTask(s, outer, nil))
	task9OK(t, openStep(s, step, nil))
	task9OK(t, openVariants(s, set, nil))
	task9OK(t, openVariant(s, a, nil))
	task9OK(t, openTask(s, first, nil))
	task9OK(t, openStep(s, innerStep, nil))
	task9OK(t, openTask(s, second, nil))
	task9OK(t, openVariant(s, b, nil))
	task9OK(t, openTask(s, third, nil))
	task9Path(t, s, set, lesson.NodeVariants, 0, 0, 0)
	task9Path(t, s, first, lesson.NodeTask, 0, 0, 0, 0, 0)
	task9Path(t, s, second, lesson.NodeTask, 0, 0, 0, 0, 1)
	task9Path(t, s, b, lesson.NodeVariant, 0, 0, 0, 1)
	task9Path(t, s, third, lesson.NodeTask, 0, 0, 0, 1, 0)
}

func TestTask9TaskOperationsDoNotCrossVariants(t *testing.T) {
	for _, branch := range []bool{false, true} {
		for _, tag := range []string{"step", "speaking", "endtask"} {
			t.Run(tag+map[bool]string{false: "/before-branch", true: "/in-branch"}[branch], func(t *testing.T) {
				s := task9State(t)
				outer := task9Entry("task", 1, nil)
				set := task9Entry("variants", 2, outer)
				task9OK(t, openTask(s, outer, nil))
				task9OK(t, openVariants(s, set, nil))
				if branch {
					task9OK(t, openVariant(s, task9Entry("variant", 3, set), nil))
				}
				entry := task9Entry(tag, 4, set)
				var problems []*diagnostic.Error
				switch tag {
				case "step":
					problems = openStep(s, entry, nil)
				case "speaking":
					problems = markSpeaking(s, entry)
				case "endtask":
					problems = endTask(s, entry)
				}
				if len(problems) == 0 || problems[0].Code != CodeContext || problems[0].Context.Line != 4 {
					t.Fatalf("want context error at operation, got %v", problems)
				}
			})
		}
	}
}

func TestTask9NestedVariantsRequireOwnBranchAndKeepOuter(t *testing.T) {
	s := task9State(t)
	outer := task9Entry("variants", 1, nil)
	branch := task9Entry("variant", 2, outer)
	inner := task9Entry("variants", 3, outer)
	task9OK(t, openVariants(s, outer, nil))
	task9OK(t, openVariant(s, branch, nil))
	task9OK(t, openVariants(s, inner, nil))
	problems := openTask(s, task9Entry("task", 4, inner), nil)
	if len(problems) == 0 || problems[0].Code != CodeContext {
		t.Fatalf("inner set without its own branch: %v", problems)
	}
	a := task9Entry("variant", 5, inner)
	b := task9Entry("variant", 6, inner)
	task9OK(t, openVariant(s, a, nil))
	task9OK(t, openVariant(s, b, nil))
	child := task9Entry("task", 7, inner)
	task9OK(t, openTask(s, child, nil))
	task9Path(t, s, inner, lesson.NodeVariants, 0, 0, 0)
	task9Path(t, s, a, lesson.NodeVariant, 0, 0, 0, 0)
	task9Path(t, s, b, lesson.NodeVariant, 0, 0, 0, 1)
	task9Path(t, s, child, lesson.NodeTask, 0, 0, 0, 1, 0)
}

func TestTask9EndNestedTaskPreservesOuterStep(t *testing.T) {
	s := task9State(t)
	outer := task9Entry("task", 1, nil)
	first := task9Entry("step", 2, outer)
	inner := task9Entry("task", 3, outer)
	task9OK(t, openTask(s, outer, nil))
	task9OK(t, openStep(s, first, nil))
	task9OK(t, openTask(s, inner, nil))
	task9OK(t, endTask(s, task9Entry("endtask", 4, outer)))
	content := task9Entry("instr", 5, outer)
	path, problem := receivingPath(s, content)
	if problem != nil || !slices.Equal(path, []int{0, 0}) {
		t.Fatalf("outer step after inner endtask: path %v, error %v", path, problem)
	}
	second := task9Entry("step", 6, outer)
	task9OK(t, openStep(s, second, nil))
	task9Path(t, s, second, lesson.NodeStep, 0, 1)
	task9OK(t, endTask(s, task9Entry("endtask", 7, nil)))
	path, problem = receivingPath(s, task9Entry("instr", 8, nil))
	if problem != nil || len(path) != 0 {
		t.Fatalf("after outer endtask: path %v, error %v", path, problem)
	}
}

func TestTask9NextStepClosesOnlyOwnPreviousStep(t *testing.T) {
	s := task9State(t)
	outer := task9Entry("task", 1, nil)
	outerStep := task9Entry("step", 2, outer)
	inner := task9Entry("task", 3, outer)
	first := task9Entry("step", 4, inner)
	second := task9Entry("step", 5, inner)
	task9OK(t, openTask(s, outer, nil))
	task9OK(t, openStep(s, outerStep, nil))
	task9OK(t, openTask(s, inner, nil))
	task9OK(t, openStep(s, first, nil))
	task9OK(t, openStep(s, second, nil))
	// speaking допустим и после step: редакторскую грамматику не повторяем.
	task9OK(t, markSpeaking(s, task9Entry("speaking", 6, inner)))
	task9Path(t, s, first, lesson.NodeStep, 0, 0, 0, 0)
	task9Path(t, s, second, lesson.NodeStep, 0, 0, 0, 1)
	path, problem := receivingPath(s, task9Entry("instr", 7, inner))
	if problem != nil || !slices.Equal(path, []int{0, 0, 0, 1}) {
		t.Fatalf("second step receiver: path %v, error %v", path, problem)
	}
}

func TestTask9FlagsBelongToTheirTask(t *testing.T) {
	for _, speaking := range []bool{false, true} {
		for _, stepped := range []bool{false, true} {
			s := task9State(t)
			outer := task9Entry("task", 1, nil)
			inner := task9Entry("task", 2, outer)
			task9OK(t, openTask(s, outer, nil))
			task9OK(t, openTask(s, inner, nil))
			if speaking {
				task9OK(t, markSpeaking(s, task9Entry("speaking", 3, inner)))
			}
			if stepped {
				task9OK(t, openStep(s, task9Entry("step", 4, inner), nil))
			}
			finishTaskFlags(s)
			for _, binding := range s.bindings {
				node := binding.Node(s.pages)
				flags := node.Flags
				if flags.Checkable != nil {
					t.Fatal("structure set checkable")
				}
				if node.Type != lesson.NodeTask {
					if flags.Speaking != nil || flags.Stepped != nil {
						t.Fatal("non-task has task flags")
					}
					continue
				}
				isInner := binding.Entries[0] == inner
				if flags.Speaking == nil || flags.Stepped == nil || *flags.Speaking != (isInner && speaking) || *flags.Stepped != (isInner && stepped) {
					t.Fatalf("inner=%v speaking=%v stepped=%v: flags %+v", isInner, speaking, stepped, flags)
				}
			}
		}
	}
}
