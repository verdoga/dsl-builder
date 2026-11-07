package revision

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

// contentTestNode содержит все изменяемые разновидности данных для проверки владения.
func contentTestNode(id, text string, children ...lesson.Node) lesson.Node {
	body, hint, script := lesson.RoleBody, lesson.RoleHint, lesson.RoleScript
	speaking, stepped := true, false
	if children == nil {
		children = []lesson.Node{}
	}
	return lesson.Node{ID: id, Type: lesson.NodeTask,
		Flags: lesson.NodeFlags{Speaking: &speaking, Stepped: &stepped}, Nodes: children,
		Data: []lesson.Data{
			{Type: lesson.DataText, Role: &body, Value: text, Answers: []string{" ответ\r\n", "Ё"}, Hint: " \tподсказка "},
			{Type: lesson.DataCollection, Role: &hint, Answers: []string{}, Value: []lesson.Data{
				{Type: lesson.DataText, Value: "\n\n{a|b} е\u0308", Answers: []string{" 1 ", "2"}},
				{Type: lesson.DataCollection, Role: &body, Answers: []string{"вложенный"}, Value: []lesson.Data{
					{Type: lesson.DataMedia, Role: &script, Answers: []string{}, Value: lesson.EmbeddedFile{
						FileName: "Ёж.bin", MIMEType: "application/octet-stream", Content: "AP8QIA=="}},
				}},
			}},
		}}
}

func contentTestLesson(pages ...lesson.Page) *lesson.Lesson {
	subtitle := " старый подзаголовок\r\n"
	if pages == nil {
		pages = []lesson.Page{}
	}
	return &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "A:B", Version: json.Number("99.0"), Title: " Старое название ", Subtitle: &subtitle,
		LineCount: json.Number("123456789012345678901234567890e2"), CreatedAt: "2024-02-29T01:02:03Z", Pages: pages}}
}

func contentTestPage(id, title string, nodes ...lesson.Node) lesson.Page {
	if nodes == nil {
		nodes = []lesson.Node{}
	}
	return lesson.Page{ID: id, Title: &title, Nodes: nodes}
}

func contentTestSnapshot(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// contentTestCopy не использует проверяемое копирование revision.
func contentTestCopy(t *testing.T, value *lesson.Lesson) *lesson.Lesson {
	t.Helper()
	var result lesson.Lesson
	if err := json.Unmarshal([]byte(contentTestSnapshot(t, value)), &result); err != nil {
		t.Fatal(err)
	}
	return &result
}

func contentTestEqual(t *testing.T, got, want any) {
	t.Helper()
	if contentTestSnapshot(t, got) != contentTestSnapshot(t, want) {
		t.Error("Результат не совпал с точным ожидаемым снимком")
	}
}

func contentTestValid(t *testing.T, values ...*lesson.Lesson) {
	t.Helper()
	for i, value := range values {
		if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
			t.Fatalf("Невалидный снимок %d: %v", i, problems)
		}
	}
}

func contentTestPlan(t *testing.T, old, candidate *lesson.Lesson) *compare.Plan {
	t.Helper()
	contentTestValid(t, old, candidate)
	plan, problems := compare.Build(old, candidate, diagnostic.Context{}, diagnostic.Context{})
	if len(problems) != 0 || plan == nil {
		t.Fatalf("Не построен реальный план: %v", problems)
	}
	return plan
}

// contentTestDamage изменяет каждый срез и указатель, включая рекурсивные Value.
func contentTestDamage(nodes []lesson.Node) {
	var damageData func([]lesson.Data)
	damageData = func(values []lesson.Data) {
		for i := range values {
			values[i].Hint = "изменено"
			if values[i].Role != nil {
				*values[i].Role = lesson.RoleAfter
			}
			for j := range values[i].Answers {
				values[i].Answers[j] = "изменено"
			}
			if children, ok := values[i].Value.([]lesson.Data); ok {
				damageData(children)
			}
		}
	}
	for i := range nodes {
		nodes[i].ID = "изменено"
		for _, flag := range []*bool{nodes[i].Flags.Speaking, nodes[i].Flags.Stepped, nodes[i].Flags.Checkable} {
			if flag != nil {
				*flag = !*flag
			}
		}
		damageData(nodes[i].Data)
		contentTestDamage(nodes[i].Nodes)
	}
}

func TestContentNodeAtArbitraryValidPaths(t *testing.T) {
	for _, depth := range []int{1, 4, 128} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			path := make([]int, depth)
			for i := range path {
				path[i] = (i + 2) % 3
			}
			selected := contentTestNode("a:b-n999", "цель")
			next := 1
			makeNode := func(text string) lesson.Node {
				node := contentTestNode(fmt.Sprintf("a:b-n%03d", next), text)
				next++
				return node
			}
			var forest []lesson.Node
			for i := depth - 1; i >= 0; i-- {
				forest = []lesson.Node{makeNode("лево"), makeNode("середина"), makeNode("право")}
				forest[path[i]] = selected
				selected = makeNode("предок")
				selected.Nodes = forest
			}
			contentTestValid(t, contentTestLesson(contentTestPage("a:b-p001", "глубина", forest...)))
			got := nodeAt(forest, path)
			if got == nil || got.ID != "a:b-n999" || got.Data[0].Value != "цель" {
				t.Fatal("Путь не привёл к выбранной ноде")
			}
			got.ID = "a:b-n888"
			if nodeAt(forest, path).ID != "a:b-n888" {
				t.Error("Возвращена копия вместо адреса ноды")
			}
		})
	}
}

func TestContentDeepOwnChangeUsesEntirePath(t *testing.T) {
	oldLeaf, newLeaf := contentTestNode("a:b-n999-v03", "старое"), contentTestNode("a:b-n001", "новое\r\n\n{a|b}")
	checkable := true
	newLeaf.Type, newLeaf.Flags = lesson.NodeQuestion, lesson.NodeFlags{Checkable: &checkable}
	path := make([]int, 64)
	for i := 63; i >= 0; i-- {
		path[i] = 1
		leftID, parentID := fmt.Sprintf("a:b-n%03d", i*2+2), fmt.Sprintf("a:b-n%03d", i*2+3)
		oldLeaf = contentTestNode(parentID, "родитель", contentTestNode(leftID, "сосед"), oldLeaf)
		newLeaf = contentTestNode(parentID, "родитель", contentTestNode(leftID, "сосед"), newLeaf)
	}
	path = append([]int{0}, path...)
	old := contentTestLesson(contentTestPage("a:b-p017", "глубокая", oldLeaf))
	candidate := contentTestLesson(contentTestPage("a:b-p001", "глубокая", newLeaf))
	contentTestPrimary(candidate)
	plan := contentTestPlan(t, old, candidate)
	if plan.ReplaceLesson || len(plan.Pages) != 1 || plan.Pages[0].ReplaceNodes || len(plan.Pages[0].Nodes) != 1 {
		t.Fatal("Ожидалась одна глубокая точечная замена")
	}
	contentTestEqual(t, plan.Pages[0].Nodes[0].Path, path)
	result := contentTestCopy(t, old)
	if problems := applyPageChanges(result, old, candidate, plan.Pages, "100", diagnostic.Context{}); len(problems) != 0 {
		t.Fatal(problems)
	}
	want := contentTestCopy(t, old)
	want.Document.Pages = contentTestCopy(t, candidate).Document.Pages
	// Все предки и их соседи сохраняют старые ID; меняются только собственные поля листа.
	var restore func([]lesson.Node, []lesson.Node)
	restore = func(target, source []lesson.Node) {
		for i := range target {
			target[i].ID = source[i].ID
			restore(target[i].Nodes, source[i].Nodes)
		}
	}
	restore(want.Document.Pages[0].Nodes, old.Document.Pages[0].Nodes)
	leaf := &want.Document.Pages[0].Nodes[0]
	for range 64 {
		leaf = &leaf.Nodes[1]
	}
	leaf.ID, want.Document.Pages[0].ID = "a:b-n999-v100", "a:b-p017"
	contentTestEqual(t, result, want)
}

func TestContentReplaceNodeOwnPreservesResultChildren(t *testing.T) {
	old := contentTestLesson(contentTestPage("a:b-p017", "старое", contentTestNode("a:b-n081-v03", "родитель", contentTestNode("a:b-n035", "ребёнок"))))
	candidate := contentTestCopy(t, old)
	node := &candidate.Document.Pages[0].Nodes[0]
	checkable := false
	node.Type, node.Flags, node.Data[0].Value = lesson.NodeQuestion, lesson.NodeFlags{Checkable: &checkable}, " \tНовый\r\n\n{текст} "
	node.ID, node.Nodes[0].ID, node.Nodes[0].Data[0].Value = "a:b-n001", "a:b-n002", "другие дети кандидата"
	contentTestValid(t, old, candidate)
	result := contentTestCopy(t, old)
	result.Document.Pages[0].Nodes[0].Nodes[0].ID = "a:b-n035-v06"
	childrenBefore := contentTestSnapshot(t, result.Document.Pages[0].Nodes[0].Nodes)
	oldBefore, candidateBefore := contentTestSnapshot(t, old), contentTestSnapshot(t, candidate)
	change := compare.NodeChange{Path: []int{0}, Fields: []compare.NodeField{compare.NodeTypeChanged, compare.NodeFlagsChanged, compare.NodeDataChanged}}
	problem := replaceNodeOwn(&result.Document.Pages[0], old.Document.Pages[0], candidate.Document.Pages[0], change, old.Document.ID, "100", diagnostic.Context{})
	if problem != nil {
		t.Fatal(problem)
	}
	got := &result.Document.Pages[0].Nodes[0]
	if got.ID != "a:b-n081-v100" || got.Type != node.Type {
		t.Error("Не сохранён базовый номер или не заменён старый суффикс/тип")
	}
	contentTestEqual(t, got.Data, node.Data)
	contentTestEqual(t, got.Flags, node.Flags)
	if contentTestSnapshot(t, got.Nodes) != childrenBefore || result.Document.Pages[0].ID != "a:b-p017" {
		t.Error("Изменены дети результата или ID страницы")
	}
	contentTestDamage(result.Document.Pages[0].Nodes)
	if contentTestSnapshot(t, old) != oldBefore || contentTestSnapshot(t, candidate) != candidateBefore {
		t.Error("Собственные поля результата разделяют память со снимками")
	}
}

func TestContentPointChangesKeepUnchangedIDs(t *testing.T) {
	for _, parentChanged := range []bool{false, true} {
		t.Run(fmt.Sprint(parentChanged), func(t *testing.T) {
			old := contentTestLesson(contentTestPage("a:b-p017", "страница", contentTestNode("a:b-n081-v03", "родитель", contentTestNode("a:b-n035", "ребёнок")), contentTestNode("a:b-n010-v02", "сосед")))
			candidate := contentTestCopy(t, old)
			candidate.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value = "новый ребёнок"
			if parentChanged {
				candidate.Document.Pages[0].Nodes[0].Data[0].Value = "новый родитель"
			}
			plan := contentTestPlan(t, old, candidate)
			if plan.ReplaceLesson || len(plan.Pages) != 1 || plan.Pages[0].ReplaceNodes {
				t.Fatal("Ожидался план точечных замен")
			}
			before := contentTestSnapshot(t, plan)
			result, want := contentTestCopy(t, old), contentTestCopy(t, old)
			want.Document.Pages[0].Nodes[0].Nodes[0].ID = "a:b-n035-v100"
			want.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value = "новый ребёнок"
			if parentChanged {
				want.Document.Pages[0].Nodes[0].ID = "a:b-n081-v100"
				want.Document.Pages[0].Nodes[0].Data[0].Value = "новый родитель"
			}
			if problems := applyContent(result, old, candidate, plan, "100", diagnostic.Context{}); len(problems) != 0 {
				t.Fatal(problems)
			}
			contentTestEqual(t, result, want)
			if contentTestSnapshot(t, plan) != before {
				t.Error("План изменён")
			}
		})
	}
}
