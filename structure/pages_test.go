package structure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task12Element(kind model.ElementType, raw, value string) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: &value, ErrorIDs: []string{}}
}

func task12Line(kind model.LineType, elements ...model.Element) model.Line {
	line := model.Line{LineType: kind, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
	for _, element := range elements {
		if len(line.Elements) > 0 {
			line.Raw += " "
		}
		element.Start = len([]rune(line.Raw)) + 1
		line.Raw += element.Raw
		element.End = len([]rune(line.Raw)) + 1
		line.Elements = append(line.Elements, element)
	}
	return line
}

func task12Heading(level, title string) model.Line {
	return task12Line(model.LineTypeHeading,
		task12Element(model.ElementTypeHeadingLevel, strings.Repeat("#", int(level[0]-'0')), level),
		task12Element(model.ElementTypeTitle, title, title))
}

func task12Tag(tag string) model.Line {
	return task12Line(model.LineTypeTag, task12Element(model.ElementTypeTag, "@"+strings.ToUpper(tag), tag))
}

func task12Input(t *testing.T, lines ...model.Line) (*parsed.Input, []*fragments.Entry) {
	t.Helper()
	if lines == nil {
		lines = []model.Line{}
	}
	for i := range lines {
		lines[i].Number = i + 1
	}
	id, title := "TEST12", "Название документа"
	result := model.Result{Format: model.FormatVersion1, Lines: lines, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{}}
	result.Document.Metadata = model.DocumentMetadata{DocumentID: &id, Title: &title, ResourceDirs: []string{}}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "source.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
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

func TestTask12PageBoundaryUsesSemanticValues(t *testing.T) {
	exact := "  Я\\n <b>А</b>\t "
	heading := task12Heading("3", exact)
	heading = task12Line(model.LineTypeHeading, heading.Elements[0], task12Element(model.ElementTypeTitle, "другой raw", exact))
	for _, tc := range []struct {
		name     string
		line     model.Line
		boundary bool
		title    *string
	}{
		{"heading3", heading, true, &exact},
		{"empty title", task12Heading("3", ""), true, new(string)},
		{"heading1", task12Heading("1", "Документ"), false, nil},
		{"heading2", task12Heading("2", "Подзаголовок"), false, nil},
		{"header", task12Tag("header"), false, nil},
		{"newpage", task12Tag("newpage"), true, nil},
		{"raw is not heading", task12Line(model.LineTypeContent, task12Element(model.ElementTypeContent, "### ложная граница", "текст")), false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, entries := task12Input(t, tc.line)
			before, _ := json.Marshal(entries[0])
			boundary, title, problems := pageBoundary(entries[0], &builder{input: input, page: -1})
			if problems != nil || boundary != tc.boundary || !reflect.DeepEqual(title, tc.title) {
				t.Fatalf("pageBoundary = %v, %v, %v; want boundary=%v title=%v", boundary, title, problems, tc.boundary, tc.title)
			}
			after, _ := json.Marshal(entries[0])
			if string(before) != string(after) {
				t.Fatal("pageBoundary изменил исходное вхождение")
			}
		})
	}
}

func TestTask12PageBoundaryRejectsMissingNullAndAmbiguousValues(t *testing.T) {
	valid := task12Heading("3", "Страница")
	nullLevel, nullTitle := valid.Elements[0], valid.Elements[1]
	nullLevel.Value, nullTitle.Value = nil, nil
	nullTag := task12Tag("newpage").Elements[0]
	nullTag.Value = nil
	for _, tc := range []struct {
		name string
		line model.Line
	}{
		{"missing level", task12Line(model.LineTypeHeading, valid.Elements[1])},
		{"missing title", task12Line(model.LineTypeHeading, valid.Elements[0])},
		{"null level", task12Line(model.LineTypeHeading, nullLevel, valid.Elements[1])},
		{"null title", task12Line(model.LineTypeHeading, valid.Elements[0], nullTitle)},
		{"ambiguous level", task12Line(model.LineTypeHeading, valid.Elements[0], valid.Elements[0], valid.Elements[1])},
		{"ambiguous title", task12Line(model.LineTypeHeading, valid.Elements[0], valid.Elements[1], valid.Elements[1])},
		{"null tag", task12Line(model.LineTypeTag, nullTag)},
		{"ambiguous tag", task12Line(model.LineTypeTag, task12Tag("newpage").Elements[0], task12Tag("header").Elements[0])},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := task12Input(t, valid)
			// Null проверяет локальный контракт pageBoundary; такой Entry не передаётся Build.
			entry := &fragments.Entry{Source: tc.line}
			entry.Source.Number = 1
			_, _, problems := pageBoundary(entry, &builder{input: input, page: -1})
			if len(problems) == 0 {
				t.Fatal("нет CodeSource для неоднозначной границы")
			}
			for _, problem := range problems {
				if problem.Code != CodeSource || problem.Reason == "" || problem.Context.Path != input.Path || problem.Context.Line != 1 || problem.Context.PageID != "" {
					t.Fatalf("неверная диагностика: %+v", problem)
				}
			}
		})
	}
}

func TestTask12BuildPreservesExplicitPageSequence(t *testing.T) {
	a, b := "А", "Б"
	text := task12Line(model.LineTypeContent, task12Element(model.ElementTypeContent, "материал", "материал"))
	header := task12Line(model.LineTypeTag, task12Element(model.ElementTypeTag, "@header", "header"), task12Element(model.ElementTypeTitle, "Заголовок", "Заголовок"))
	for _, tc := range []struct {
		name   string
		lines  []model.Line
		titles []*string
		counts []int
	}{
		{"empty", nil, []*string{nil}, []int{0}},
		{"metadata headings", []model.Line{task12Heading("1", "Документ"), task12Heading("2", "Подзаголовок")}, []*string{nil}, []int{0}},
		{"first heading", []model.Line{task12Heading("3", a)}, []*string{&a}, []int{0}},
		{"first newpage", []model.Line{task12Tag("newpage")}, []*string{nil, nil}, []int{0, 0}},
		{"material before heading", []model.Line{text, task12Heading("3", a)}, []*string{nil, &a}, []int{1, 0}},
		{"material before newpage", []model.Line{text, task12Tag("newpage")}, []*string{nil, nil}, []int{1, 0}},
		{"header is material", []model.Line{header, task12Heading("3", a)}, []*string{nil, &a}, []int{1, 0}},
		{"repeated empty pages", []model.Line{task12Tag("newpage"), task12Tag("newpage"), task12Heading("3", a), task12Heading("3", b), task12Tag("newpage")}, []*string{nil, nil, nil, &a, &b, nil}, []int{0, 0, 0, 0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, entries := task12Input(t, tc.lines...)
			pages, bindings, problems := Build(input, entries)
			if problems != nil || pages == nil || bindings == nil || len(pages) != len(tc.titles) {
				t.Fatalf("Build: pages=%+v bindings=%+v errors=%v", pages, bindings, problems)
			}
			nodes := 0
			for i, page := range pages {
				if page.ID != "" || page.Nodes == nil || len(page.Nodes) != tc.counts[i] || !reflect.DeepEqual(page.Title, tc.titles[i]) {
					t.Fatalf("page[%d]=%+v", i, page)
				}
				nodes += len(page.Nodes)
				for _, node := range page.Nodes {
					if node.ID != "" || node.Data == nil || node.Nodes == nil {
						t.Fatalf("неверная заготовка ноды: %+v", node)
					}
				}
			}
			if len(bindings) != nodes {
				t.Fatalf("граница стала нодой: %d bindings для %d нод", len(bindings), nodes)
			}
		})
	}
}

func TestTask12EnsureAndFinishPagesAreIdempotent(t *testing.T) {
	input, _ := task12Input(t)
	state := &builder{input: input, page: -1}
	finishPages(state)
	if len(state.pages) != 1 || state.page != 0 || state.pages[0].Title != nil || state.pages[0].ID != "" || state.pages[0].Nodes == nil {
		t.Fatalf("начальная страница: %+v", state)
	}
	ensurePage(state)
	finishPages(state)
	title := "  Собственное название  "
	state.pages[0].Title = &title
	state.pages[0].Nodes = append(state.pages[0].Nodes, lesson.Node{Type: lesson.NodeText, Data: []lesson.Data{}, Nodes: []lesson.Node{}})
	before, _ := json.Marshal(state.pages)
	ensurePage(state)
	finishPages(state)
	after, _ := json.Marshal(state.pages)
	if string(before) != string(after) || state.page != 0 {
		t.Fatal("ensurePage/finishPages изменили существующую страницу")
	}
	initial := &builder{input: input, page: -1}
	ensurePage(initial)
	if len(initial.pages) != 1 || initial.pages[0].Title != nil || initial.pages[0].Nodes == nil || initial.page != 0 {
		t.Fatalf("ensurePage: %+v", initial)
	}
}

func TestTask12OpenPageClosesEveryActiveFrame(t *testing.T) {
	lines := []model.Line{
		task12Line(model.LineTypeTag, task12Element(model.ElementTypeTag, "@task", "task"), task12Element(model.ElementTypeIdentifier, "a", "a")),
		task12Line(model.LineTypeTag, task12Element(model.ElementTypeTag, "@step", "step"), task12Element(model.ElementTypeTitle, "Этап", "Этап")),
		task12Line(model.LineTypeBlockStart, task12Element(model.ElementTypeTag, "@variants", "variants"), task12Element(model.ElementTypeBlockOpen, "{", "{")),
		task12Line(model.LineTypeTag, task12Element(model.ElementTypeTag, "@variant", "variant"), task12Element(model.ElementTypeName, "Ветка", "Ветка")),
		task12Tag("newpage"),
		task12Line(model.LineTypeBlockEnd, task12Element(model.ElementTypeBlockClose, "}", "}")),
	}
	task, variants := 1, 3
	for _, i := range []int{1, 2} {
		lines[i].ParentLine, lines[i].NestingLevel = &task, 1
	}
	for _, i := range []int{3, 4, 5} {
		lines[i].ParentLine, lines[i].NestingLevel = &variants, 2
	}
	input, entries := task12Input(t, lines...)
	state := &builder{input: input, page: -1}
	ensurePage(state)
	for _, kind := range []lesson.NodeType{lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants, lesson.NodeVariant} {
		path := make([]int, len(state.bindings))
		index := appendNode(state, path, kind, []*fragments.Entry{entries[len(state.bindings)]})
		state.frames = append(state.frames, frame{binding: index})
	}
	before, _ := json.Marshal(state.pages[0])
	openPage(state, entries[4], nil)
	if len(state.frames) != 0 || state.page != 1 || len(state.pages) != 2 || len(state.bindings) != 4 {
		t.Fatalf("openPage: frames=%+v page=%d pages=%d bindings=%d", state.frames, state.page, len(state.pages), len(state.bindings))
	}
	after, _ := json.Marshal(state.pages[0])
	page := state.pages[1]
	if string(before) != string(after) || page.ID != "" || page.Nodes == nil || len(page.Nodes) != 0 || page.Title != nil {
		t.Fatalf("openPage изменил содержимое: %+v", state.pages)
	}
}
