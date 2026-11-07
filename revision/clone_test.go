package revision

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
)

func cloneTestText(value string) *string                   { return &value }
func cloneTestBool(value bool) *bool                       { return &value }
func cloneTestRole(value lesson.DataRole) *lesson.DataRole { return &value }

func cloneTestData() []lesson.Data {
	return []lesson.Data{
		{Type: lesson.DataText, Role: cloneTestRole(lesson.RoleBody), Value: "  ё e\u0308\r\n\n<b>{a|b}</b>  ", Answers: []string{"", " Да ", "Да"}, Hint: "\tподсказка\n"},
		{Type: lesson.DataMedia, Role: cloneTestRole(lesson.RoleBody), Value: lesson.EmbeddedFile{FileName: "ёж.bin", MIMEType: "application/octet-stream", Content: "AAEC/v8="}, Answers: []string{}, Hint: ""},
		{Type: lesson.DataCollection, Role: cloneTestRole(lesson.RoleBody), Answers: []string{}, Value: []lesson.Data{
			{Type: lesson.DataCollection, Answers: []string{"outer"}, Value: []lesson.Data{
				{Type: lesson.DataText, Role: cloneTestRole(lesson.RoleBody), Value: "nested\n\ntext", Answers: []string{"1", "2"}, Hint: " inner "},
			}},
		}},
	}
}

func cloneTestLesson() *lesson.Lesson {
	return &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "A:B", Version: json.Number("900719925474099312345678901.0"), Title: "  Урок\nЁ ", Subtitle: cloneTestText(""),
		LineCount: json.Number("123456789012345678900e+2"), CreatedAt: "2024-02-29T01:02:03Z",
		Pages: []lesson.Page{{ID: "a:b-p1000", Title: cloneTestText(" Страница "), Nodes: []lesson.Node{
			{ID: "a:b-n1000", Type: lesson.NodeTask, Flags: lesson.NodeFlags{Speaking: cloneTestBool(false), Stepped: cloneTestBool(true)}, Data: cloneTestData(), Nodes: []lesson.Node{
				{ID: "a:b-n1001", Type: lesson.NodeStep, Data: []lesson.Data{}, Nodes: []lesson.Node{
					{ID: "a:b-n1002-v100", Type: lesson.NodeQuestion, Flags: lesson.NodeFlags{Checkable: cloneTestBool(false)}, Data: cloneTestData(), Nodes: []lesson.Node{}},
				}},
			}},
		}}, {ID: "a:b-p1001", Title: nil, Nodes: []lesson.Node{}}},
	}}
}

// cloneTestEqual проверяет все поля без сериализации, сохраняющей не все формы nil.
func cloneTestEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("снимок отличается: got=%#v, want=%#v", got, want)
	}
}

func cloneTestCheckData(t *testing.T, got, source []lesson.Data) {
	t.Helper()
	cloneTestEqual(t, got, source)
	if len(source) > 0 && &got[0] == &source[0] {
		t.Fatal("Data разделяет массив с исходником")
	}
	for i := range source {
		if source[i].Role != nil && got[i].Role == source[i].Role {
			t.Fatalf("Data[%d].Role разделяет указатель", i)
		}
		if len(source[i].Answers) > 0 && &got[i].Answers[0] == &source[i].Answers[0] {
			t.Fatalf("Data[%d].Answers разделяет массив", i)
		}
		if nested, ok := source[i].Value.([]lesson.Data); ok {
			copied, ok := got[i].Value.([]lesson.Data)
			if !ok {
				t.Fatalf("Data[%d].Value потерял тип []lesson.Data", i)
			}
			cloneTestCheckData(t, copied, nested)
		}
	}
}

func cloneTestCheckNodes(t *testing.T, got, source []lesson.Node) {
	t.Helper()
	cloneTestEqual(t, got, source)
	if len(source) > 0 && &got[0] == &source[0] {
		t.Fatal("Nodes разделяет массив с исходником")
	}
	for i := range source {
		pairs := [][2]*bool{{got[i].Flags.Speaking, source[i].Flags.Speaking}, {got[i].Flags.Stepped, source[i].Flags.Stepped}, {got[i].Flags.Checkable, source[i].Flags.Checkable}}
		for _, pair := range pairs {
			if pair[1] != nil && pair[0] == pair[1] {
				t.Fatalf("Nodes[%d].Flags разделяет указатель", i)
			}
		}
		cloneTestCheckData(t, got[i].Data, source[i].Data)
		cloneTestCheckNodes(t, got[i].Nodes, source[i].Nodes)
	}
}

func cloneTestCheckPages(t *testing.T, got, source []lesson.Page) {
	t.Helper()
	cloneTestEqual(t, got, source)
	if len(source) > 0 && &got[0] == &source[0] {
		t.Fatal("Pages разделяет массив с исходником")
	}
	for i := range source {
		if source[i].Title != nil && got[i].Title == source[i].Title {
			t.Fatalf("Pages[%d].Title разделяет указатель", i)
		}
		cloneTestCheckNodes(t, got[i].Nodes, source[i].Nodes)
	}
}

func TestCloneLessonIndependentExactSnapshot(t *testing.T) {
	source := cloneTestLesson()
	got := cloneLesson(source)
	cloneTestEqual(t, got, source)
	if got == source || got.Document.Subtitle == source.Document.Subtitle {
		t.Fatal("урок или подзаголовок разделяет указатель")
	}
	cloneTestCheckPages(t, got.Document.Pages, source.Document.Pages)
	*got.Document.Subtitle = "изменено"
	*got.Document.Pages[0].Title = "изменено"
	root := &got.Document.Pages[0].Nodes[0]
	*root.Flags.Speaking = true
	*root.Flags.Stepped = false
	root.Data[0].Answers[0] = "изменено"
	*root.Data[0].Role = lesson.RoleHint
	nested := root.Data[2].Value.([]lesson.Data)[0].Value.([]lesson.Data)
	nested[0].Answers[0] = "изменено"
	nested[0].Value = "изменено"
	child := &root.Nodes[0].Nodes[0]
	*child.Flags.Checkable = true
	child.ID = "изменено"
	got.Document.Pages[1].ID = "изменено"
	cloneTestEqual(t, source, cloneTestLesson())
}

func TestClonePagesIndependentExactSnapshot(t *testing.T) {
	source := cloneTestLesson().Document.Pages
	got := clonePages(source)
	cloneTestCheckPages(t, got, source)
	cloneTestEqual(t, source, cloneTestLesson().Document.Pages)
}

func TestCloneNodesIndependentExactSnapshot(t *testing.T) {
	source := cloneTestLesson().Document.Pages[0].Nodes
	got := cloneNodes(source)
	cloneTestCheckNodes(t, got, source)
	cloneTestEqual(t, source, cloneTestLesson().Document.Pages[0].Nodes)
}

func TestCloneDataIndependentExactSnapshot(t *testing.T) {
	source := cloneTestData()
	got := cloneData(source)
	cloneTestCheckData(t, got, source)
	cloneTestEqual(t, source, cloneTestData())
	// Изменение источника также не может проникнуть в готовый снимок.
	source[0].Answers[1] = "новый ответ"
	*source[0].Role = lesson.RoleHint
	source[2].Value.([]lesson.Data)[0].Value.([]lesson.Data)[0].Hint = "новая подсказка"
	cloneTestEqual(t, got, cloneTestData())
}

func TestClonePreservesNilAndNonNilEmpty(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "nil", true: "empty"}[empty], func(t *testing.T) {
			var pages []lesson.Page
			var nodes []lesson.Node
			var data []lesson.Data
			var answers []string
			if empty {
				pages, nodes, data, answers = []lesson.Page{}, []lesson.Node{}, []lesson.Data{}, []string{}
			}
			cloneTestEqual(t, clonePages(pages), pages)
			cloneTestEqual(t, cloneNodes(nodes), nodes)
			cloneTestEqual(t, cloneData(data), data)
			input := &lesson.Lesson{Document: lesson.Document{Pages: pages}}
			cloneTestEqual(t, cloneLesson(input), input)
			pages = []lesson.Page{{Nodes: nodes}}
			cloneTestEqual(t, clonePages(pages), pages)
			nodes = []lesson.Node{{Nodes: nodes, Data: data}}
			cloneTestEqual(t, cloneNodes(nodes), nodes)
			data = []lesson.Data{{Type: lesson.DataCollection, Value: data, Answers: answers}}
			cloneTestEqual(t, cloneData(data), data)
		})
	}
}

func TestCloneDataUnknownValueRetainedWithoutMutation(t *testing.T) {
	text := "неподдерживаемый указатель"
	for _, value := range []any{nil, 17, &text, map[string]string{"a": "b"}, &lesson.EmbeddedFile{Content: "AA=="}} {
		source := []lesson.Data{{Type: lesson.DataText, Value: value, Answers: []string{"a"}, Role: cloneTestRole(lesson.RoleBody)}}
		got := cloneData(source)
		cloneTestCheckData(t, got, source)
		cloneTestEqual(t, got[0].Value, value)
		cloneTestEqual(t, source[0].Value, value)
		got[0].Answers[0] = "b"
		if source[0].Answers[0] != "a" {
			t.Fatal("неизвестный Value помешал копированию Answers")
		}
	}
}
