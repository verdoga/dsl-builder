package structure

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// task6Entry создаёт исходные связи; семантическая вложенность задаётся отдельно.
func task6Entry(number int, tag string, parent *fragments.Entry) *fragments.Entry {
	raw := "@" + tag
	entry := &fragments.Entry{Parent: parent, Source: model.Line{
		Number: number, LineType: model.LineTypeTag, Raw: raw, LineEnding: model.LineEndingLF,
		Elements: []model.Element{{ElementType: model.ElementTypeTag, Raw: raw,
			Value: &tag, Start: 1, End: len(raw) + 1, ErrorIDs: []string{}}},
	}}
	if parent != nil {
		n := parent.Source.Number
		entry.Source.ParentLine, entry.Source.NestingLevel = &n, parent.Source.NestingLevel+1
	}
	parameterType := map[string]model.ElementType{"task": model.ElementTypeIdentifier,
		"step": model.ElementTypeTitle, "variant": model.ElementTypeName}[tag]
	if parameterType != "" {
		value := "same"
		entry.Source.Raw += " " + value
		entry.Source.Elements = append(entry.Source.Elements, model.Element{
			ElementType: parameterType, Raw: value, Value: &value,
			Start: len(raw) + 2, End: len(raw) + 2 + len(value), ErrorIDs: []string{},
		})
	}
	if tag == "variants" || tag == "text" {
		open := "{"
		entry.Source.LineType = model.LineTypeBlockStart
		entry.Source.Raw += " {"
		entry.Source.Elements = append(entry.Source.Elements, model.Element{
			ElementType: model.ElementTypeBlockOpen, Raw: open, Value: &open,
			Start: len(raw) + 2, End: len(raw) + 3, ErrorIDs: []string{},
		})
	}
	return entry
}

// task6State собирает допустимую активную цепь без вызовов проверяемых функций.
// Source.Parent задаёт task A→variants→task B, а дерево включает их step/variant.
func task6State(t *testing.T, count int) (*builder, []*fragments.Entry) {
	t.Helper()
	entries := make([]*fragments.Entry, 8)
	entries[0] = task6Entry(1, "task", nil)
	entries[1] = task6Entry(2, "step", entries[0])
	entries[2] = task6Entry(3, "variants", entries[0])
	entries[3] = task6Entry(4, "variant", entries[2])
	entries[4] = task6Entry(5, "task", entries[2])
	entries[5] = task6Entry(6, "step", entries[4])
	entries[6] = task6Entry(7, "variants", entries[4])
	entries[7] = task6Entry(8, "variant", entries[6])
	id := "STATE6"
	state := &builder{input: &parsed.Input{Path: "/state/technical.json",
		Result: model.Result{Format: model.FormatVersion1,
			Document: model.Document{Metadata: model.DocumentMetadata{DocumentID: &id}}}},
		page: 1, pages: []lesson.Page{{Nodes: []lesson.Node{}}, {Nodes: []lesson.Node{}}}}
	kinds := []lesson.NodeType{lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants,
		lesson.NodeVariant, lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants, lesson.NodeVariant}
	nodes := &state.pages[1].Nodes
	path := []int{}
	for i := range count {
		*nodes = append(*nodes, lesson.Node{Type: kinds[i], Data: []lesson.Data{}, Nodes: []lesson.Node{}})
		path = append(slices.Clone(path), 0)
		state.bindings = append(state.bindings, Binding{Page: 1, NodePath: path, Entries: []*fragments.Entry{entries[i]}})
		state.frames = append(state.frames, frame{binding: i})
		nodes = &(*nodes)[0].Nodes
	}
	return state, entries
}

func task6WantPath(t *testing.T, state *builder, entry *fragments.Entry, depth int) {
	t.Helper()
	got, problem := receivingPath(state, entry)
	if problem != nil {
		t.Fatalf("Неожиданная ошибка получателя: %v", problem)
	}
	if depth == 0 {
		if got != nil {
			t.Fatalf("Корень страницы: получен путь %v", got)
		}
		return
	}
	if want := make([]int, depth); !slices.Equal(got, want) {
		t.Fatalf("Путь получателя = %v, нужен %v", got, want)
	}
}

func TestTask6ReceivingPathChoosesClosestSemanticFrame(t *testing.T) {
	for _, tc := range []struct {
		name          string
		frames, owner int
	}{
		{"корень", 0, -1}, {"задание", 1, 0}, {"этап_при_parent_task", 2, 0},
		{"ветвь_при_parent_variants", 4, 2}, {"внутреннее_задание_ближе_внешнего_этапа", 5, 4},
		{"собственный_этап_внутреннего_задания", 6, 4}, {"внутренняя_ветвь", 8, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, entries := task6State(t, tc.frames)
			var parent *fragments.Entry
			if tc.owner >= 0 {
				parent = entries[tc.owner]
			}
			entry := task6Entry(20, "instr", parent)
			trimContexts(state, entry)
			task6WantPath(t, state, entry, tc.frames)
		})
	}
}

func TestTask6ReceivingPathRequiresOwnVariantOfNearestSet(t *testing.T) {
	for _, owner := range []int{6, 4} {
		state, entries := task6State(t, 7)
		entry := task6Entry(20, "instr", entries[owner])
		// Parent внутреннего task покидает внутренний набор, а Parent набора — нет.
		trimContexts(state, entry)
		if owner == 4 {
			task6WantPath(t, state, entry, 6)
			continue
		}
		path, problem := receivingPath(state, entry)
		if problem == nil || problem.Code != CodeContext || path != nil {
			t.Fatalf("Внутренний набор без своей ветви: path=%v, error=%v", path, problem)
		}
		if problem.Context.Path != state.input.Path || problem.Context.Line != 20 || problem.Context.PageID != "" || problem.Reason == "" {
			t.Fatalf("Потеряно место ошибки: %+v", problem)
		}
	}
}

func TestTask6TrimContextsKeepsOnlyBelongingSemanticAncestors(t *testing.T) {
	for _, tc := range []struct {
		name        string
		owner, keep int
	}{
		{"общий_task_сохраняет_только_его_step", 0, 2},
		{"общий_variants_сохраняет_только_его_variant", 2, 4},
		{"внутренний_task_сохраняет_свой_step", 4, 6},
		{"внутренний_variants_сохраняет_свою_variant", 6, 8},
		{"выход_в_документ", -1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, entries := task6State(t, 8)
			var parent *fragments.Entry
			if tc.owner >= 0 {
				parent = entries[tc.owner]
			}
			entry := task6Entry(20, "instr", parent)
			trimContexts(state, entry)
			if len(state.frames) != tc.keep {
				t.Fatalf("Осталось %d областей, нужно %d", len(state.frames), tc.keep)
			}
			task6WantPath(t, state, entry, tc.keep)
			// Старый Entry из закрытого хвоста не открывает его повторно.
			if tc.keep < 8 {
				later := task6Entry(21, "instr", entries[7])
				trimContexts(state, later)
				if len(state.frames) != tc.keep {
					t.Fatal("Изменена цепочка оставшихся предков")
				}
				task6WantPath(t, state, later, tc.keep)
			}
		})
	}
}

func TestTask6TrimContextsPreservesNestedTaskAndOrdinaryBlock(t *testing.T) {
	for _, tag := range []string{"task", "text"} {
		state, entries := task6State(t, 6)
		entry := task6Entry(20, tag, entries[4])
		if tag == "text" {
			entry.Source.LineType = model.LineTypeBlockStart
			close := "}"
			parentNumber := entry.Source.Number
			entry = &fragments.Entry{Parent: entry, Source: model.Line{
				Number: 21, LineType: model.LineTypeBlockEnd, ParentLine: &parentNumber,
				NestingLevel: entry.Source.NestingLevel + 1, Raw: close, LineEnding: model.LineEndingLF,
				Elements: []model.Element{{ElementType: model.ElementTypeBlockClose, Raw: close,
					Value: &close, Start: 1, End: 2, ErrorIDs: []string{}}},
			}}
		}
		trimContexts(state, entry)
		if len(state.frames) != 6 {
			t.Fatalf("%s закрыл внешние семантические области", tag)
		}
		task6WantPath(t, state, entry, 6)
	}
}

func TestTask6TrimContextsUsesIncludeEntryIdentity(t *testing.T) {
	state, entries := task6State(t, 6)
	site := task6Entry(30, "include", entries[2])
	site.Included = []*fragments.Entry{entries[4], entries[5]}
	entries[4].Parent = site
	state.frames[4].boundary, state.frames[5].boundary = site, site
	inside := task6Entry(40, "include", entries[4])
	inside.Included = []*fragments.Entry{}
	// Копия корневой строки фрагмента сохраняет исходную глубину 0.
	content := task6Entry(100, "instr", nil)
	content.Parent = inside
	trimContexts(state, content)
	task6WantPath(t, state, content, 6)
	otherSite := task6Entry(30, "include", entries[2])
	otherSite.Included = []*fragments.Entry{}
	otherTask := task6Entry(5, "task", nil)
	otherTask.Parent = otherSite
	outside := task6Entry(101, "instr", otherTask)
	trimContexts(state, outside)
	if len(state.frames) != 4 {
		t.Fatalf("Другое вхождение с теми же номерами сохранило чужое задание: %d", len(state.frames))
	}
	task6WantPath(t, state, outside, 4)
}

func TestTask6CloseFromRetainsTreeBindingsAndOuterFrames(t *testing.T) {
	for _, cut := range []int{0, 2, 4, 7} {
		state, entries := task6State(t, 8)
		state.pages[1].Nodes[0].Data = []lesson.Data{{Type: lesson.DataText,
			Value: "Собственные данные сохраняются", Answers: []string{}, Hint: "подсказка"}}
		before, err := json.Marshal(struct {
			Pages    []lesson.Page
			Bindings []Binding
		}{state.pages, state.bindings})
		if err != nil {
			t.Fatal(err)
		}
		closeFrom(state, cut)
		if len(state.frames) != cut {
			t.Fatalf("closeFrom(%d): областей %d", cut, len(state.frames))
		}
		after, err := json.Marshal(struct {
			Pages    []lesson.Page
			Bindings []Binding
		}{state.pages, state.bindings})
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatal("Закрытие изменило построенное дерево или привязки")
		}
		for i, active := range state.frames {
			if state.bindings[active.binding].Entries[0] != entries[i] {
				t.Fatal("Изменён внешний предок")
			}
		}
	}
}
