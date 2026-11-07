package structure_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/structure"
)

func task3Value(value string) *string { return &value }

func task3Element(kind model.ElementType, raw string, value *string) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: value, ErrorIDs: []string{}}
}

func task3Text(value string) model.Element {
	return task3Element(model.ElementTypeContent, value, task3Value(value))
}

func task3Tag(tag string) model.Element {
	return task3Element(model.ElementTypeTag, "@"+tag, task3Value(tag))
}

func task3Line(kind model.LineType, parent int, elements ...model.Element) model.Line {
	line := model.Line{LineType: kind, Elements: []model.Element{}}
	if parent != 0 {
		line.ParentLine = &parent
	}
	for _, element := range elements {
		if len(line.Elements) > 0 {
			line.Raw += " "
		}
		element.Start = utf8.RuneCountInString(line.Raw) + 1
		line.Raw += element.Raw
		element.End = utf8.RuneCountInString(line.Raw) + 1
		line.Elements = append(line.Elements, element)
	}
	return line
}

func task3Open(tag string, parent int, elements ...model.Element) model.Line {
	parts := append([]model.Element{task3Tag(tag)}, elements...)
	parts = append(parts, task3Element(model.ElementTypeBlockOpen, "{", task3Value("{")))
	return task3Line(model.LineTypeBlockStart, parent, parts...)
}

func task3Close(parent int) model.Line {
	return task3Line(model.LineTypeBlockEnd, parent, task3Element(model.ElementTypeBlockClose, "}", task3Value("}")))
}

func task3JSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func task3Load(t *testing.T, lines ...model.Line) (*parsed.Input, []*fragments.Entry) {
	t.Helper()
	if lines == nil {
		lines = []model.Line{}
	}
	for i := range lines {
		lines[i].Number, lines[i].LineEnding = i+1, model.LineEndingLF
		if i%3 == 0 {
			lines[i].LineEnding = model.LineEndingCRLF
		}
		if lines[i].ParentLine != nil {
			lines[i].NestingLevel = lines[*lines[i].ParentLine-1].NestingLevel + 1
		}
	}
	if len(lines) > 0 {
		lines[len(lines)-1].LineEnding = model.LineEndingNone
	}
	count := len(lines)
	result := model.Result{Format: model.FormatVersion1, Lines: lines,
		Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Document: model.Document{LineCount: &count, Metadata: model.DocumentMetadata{
			DocumentID: task3Value("QA3"), Title: task3Value("Document"), ResourceDirs: []string{}}}}
	path := filepath.Join(t.TempDir(), "исходный.json")
	if err := os.WriteFile(path, task3JSON(t, result), 0600); err != nil {
		t.Fatal(err)
	}
	input, problems := parsed.Load(path)
	if problems != nil {
		t.Fatalf("Load: %v", problems)
	}
	entries, problems := fragments.Expand(input)
	if problems != nil {
		t.Fatalf("Expand: %v", problems)
	}
	return input, entries
}

func task3Build(t *testing.T, input *parsed.Input, entries []*fragments.Entry) ([]lesson.Page, []structure.Binding) {
	t.Helper()
	before := task3JSON(t, input)
	pages, bindings, problems := structure.Build(input, entries)
	if problems != nil || pages == nil || bindings == nil {
		t.Fatalf("Build: pages=%v bindings=%v errors=%v", pages, bindings, problems)
	}
	if !bytes.Equal(before, task3JSON(t, input)) {
		t.Fatal("Build изменил Input")
	}
	return pages, bindings
}

func TestTask3BuildBindingPreorderAndNodeAddress(t *testing.T) {
	input, entries := task3Load(t,
		task3Line(model.LineTypeTag, 0, task3Tag("task"), task3Element("identifier", "a", task3Value("a"))),
		task3Line(model.LineTypeTag, 1, task3Tag("speaking")),
		task3Line(model.LineTypeTag, 1, task3Tag("step"), task3Element("title", "Этап", task3Value("Этап"))),
		task3Line(model.LineTypeContent, 1, task3Text("В этапе")),
		task3Line(model.LineTypeTag, 0, task3Tag("task"), task3Element("identifier", "a", task3Value("a"))),
		task3Line(model.LineTypeContent, 5, task3Text("Без этапов")),
		task3Line(model.LineTypeHeading, 0, task3Element("heading-level", "###", task3Value("3")), task3Element("title", "Страница", task3Value("Страница"))),
		task3Line(model.LineTypeContent, 0, task3Text("На второй")))
	pages, bindings := task3Build(t, input, entries)
	wantPaths := [][]int{{0}, {0, 0}, {0, 0, 0}, {1}, {1, 0}, {0}}
	wantLines := []int{1, 3, 4, 5, 6, 8}
	wantTypes := []lesson.NodeType{lesson.NodeTask, lesson.NodeStep, lesson.NodeText, lesson.NodeTask, lesson.NodeText, lesson.NodeText}
	if len(pages) != 2 || len(bindings) != len(wantPaths) {
		t.Fatalf("pages=%d bindings=%d", len(pages), len(bindings))
	}
	var visited []*lesson.Node
	var walk func([]lesson.Node)
	walk = func(nodes []lesson.Node) {
		for i := range nodes {
			visited = append(visited, &nodes[i])
			walk(nodes[i].Nodes)
		}
	}
	for _, page := range pages {
		if page.ID != "" || page.Nodes == nil {
			t.Fatalf("Некорректная заготовка страницы: %+v", page)
		}
		walk(page.Nodes)
	}
	if len(visited) != len(bindings) {
		t.Fatalf("Нод %d, привязок %d", len(visited), len(bindings))
	}
	for i, binding := range bindings {
		wantPage := 0
		if i == len(bindings)-1 {
			wantPage = 1
		}
		if binding.Page != wantPage || !slices.Equal(binding.NodePath, wantPaths[i]) || len(binding.Entries) != 1 || binding.Entries[0] != entries[wantLines[i]-1] {
			t.Fatalf("Binding[%d]=%+v", i, binding)
		}
		node := binding.Node(pages)
		if node != visited[i] || node.Type != wantTypes[i] || node.ID != "" || node.Data == nil || len(node.Data) != 0 || node.Nodes == nil {
			t.Fatalf("Node[%d]=%+v", i, node)
		}
		if node.Flags.Checkable != nil {
			t.Fatalf("structure установил checkable: %+v", node.Flags)
		}
		if node.Type == lesson.NodeTask {
			want := i == 0
			if node.Flags.Speaking == nil || node.Flags.Stepped == nil || *node.Flags.Speaking != want || *node.Flags.Stepped != want {
				t.Fatalf("task flags[%d]=%+v", i, node.Flags)
			}
		} else if node.Flags.Speaking != nil || node.Flags.Stepped != nil {
			t.Fatalf("Флаги не-task: %+v", node.Flags)
		}
		if node.Type == lesson.NodeTask || node.Type == lesson.NodeStep {
			declaration, lines := binding.TextSource()
			if declaration == nil || declaration.Number != wantLines[i] || len(lines) != 0 {
				t.Fatalf("Структурная нода получила тело потомков: %+v %+v", declaration, lines)
			}
		}
		node.ID = "одинаковый ID"
		node.Data = []lesson.Data{{Type: lesson.DataText, Value: "заполнено"}}
		if binding.Node(pages) != node {
			t.Fatal("Node потерял адрес после заполнения данных/ID")
		}
	}
	again, _ := task3Build(t, input, entries)
	if again[0].Nodes[0].ID != "" || len(again[0].Nodes[0].Data) != 0 {
		t.Fatal("Повторный Build зависит от изменённого результата")
	}
}

func TestTask3BuildNestedTaskFlagsBelongToOwnTask(t *testing.T) {
	input, entries := task3Load(t,
		task3Line(model.LineTypeTag, 0, task3Tag("task"), task3Element("identifier", "A", task3Value("A"))),
		task3Line(model.LineTypeTag, 1, task3Tag("task"), task3Element("identifier", "B", task3Value("B"))),
		task3Line(model.LineTypeTag, 2, task3Tag("step"), task3Element("title", "S", task3Value("S"))),
		task3Line(model.LineTypeContent, 2, task3Text("Внутреннее")))
	pages, bindings := task3Build(t, input, entries)
	if len(bindings) != 4 {
		t.Fatalf("Вложенная структура: %+v", bindings)
	}
	for i, want := range []bool{false, true} {
		node := bindings[i].Node(pages)
		if node.Type != lesson.NodeTask || node.Flags.Stepped == nil || *node.Flags.Stepped != want || node.Flags.Speaking == nil || *node.Flags.Speaking || node.Flags.Checkable != nil {
			t.Fatalf("Флаги вложенного task[%d]: %+v", i, node)
		}
	}
}

func TestTask3BuildEmptyAndAtomicFailure(t *testing.T) {
	input, entries := task3Load(t)
	pages, bindings := task3Build(t, input, entries)
	if len(pages) != 1 || pages[0].Title != nil || pages[0].Nodes == nil || len(pages[0].Nodes) != 0 || len(bindings) != 0 {
		t.Fatalf("Пустой вход: pages=%+v bindings=%+v", pages, bindings)
	}
	input, entries = task3Load(t, task3Line(model.LineTypeContent, 0, task3Text("до ошибки")),
		task3Line(model.LineTypeTag, 0, task3Tag("step"), task3Element("title", "A", task3Value("A"))))
	before := task3JSON(t, input)
	pages, bindings, problems := structure.Build(input, entries)
	if pages != nil || bindings != nil || len(problems) == 0 || problems[0].Code != structure.CodeContext {
		t.Fatalf("Неатомарная ошибка: pages=%v bindings=%v errors=%v", pages, bindings, problems)
	}
	if !bytes.Equal(before, task3JSON(t, input)) {
		t.Fatal("Неуспешный Build изменил Input")
	}
	if problems[0].Context.Path != input.Path || problems[0].Context.LessonID != "QA3" || problems[0].Context.PageID != "" || problems[0].Context.Line != 2 {
		t.Fatalf("Контекст ошибки: %+v", problems[0].Context)
	}
}

func TestTask3BindingPathsSurviveSliceGrowth(t *testing.T) {
	lines := make([]model.Line, 1005)
	for i := range lines {
		lines[i] = task3Line(model.LineTypeContent, 0, task3Text("строка"))
	}
	input, entries := task3Load(t, lines...)
	pages, bindings := task3Build(t, input, entries)
	if len(bindings) != len(lines) || len(pages) != 1 || len(pages[0].Nodes) != len(lines) {
		t.Fatalf("Количество нод/привязок: %d/%d", len(pages[0].Nodes), len(bindings))
	}
	for i, binding := range bindings {
		if binding.Page != 0 || !slices.Equal(binding.NodePath, []int{i}) || binding.Node(pages) != &pages[0].Nodes[i] {
			t.Fatalf("Путь после роста среза [%d]=%+v", i, binding)
		}
	}
}
