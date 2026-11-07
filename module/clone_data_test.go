package module

import (
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
)

func TestCloneDataIndependentValuesRolesAndAnswers(t *testing.T) {
	cases := []struct {
		name       string
		selectData func(*lesson.Lesson) lesson.Data
	}{
		{"text", func(v *lesson.Lesson) lesson.Data { return v.Document.Pages[0].Nodes[0].Nodes[0].Data[0] }},
		{"multifill", func(v *lesson.Lesson) lesson.Data { return v.Document.Pages[0].Nodes[0].Nodes[1].Data[0] }},
		{"media", func(v *lesson.Lesson) lesson.Data { return v.Document.Pages[0].Nodes[0].Nodes[2].Data[0] }},
		{"script", func(v *lesson.Lesson) lesson.Data { return v.Document.Pages[0].Nodes[0].Nodes[2].Data[1] }},
		{"subtitles", func(v *lesson.Lesson) lesson.Data { return v.Document.Pages[0].Nodes[0].Nodes[2].Data[2] }},
		{"nested collection", func(v *lesson.Lesson) lesson.Data { return v.Document.Pages[0].Nodes[0].Nodes[3].Data[0] }},
	}
	for _, test := range cases {
		for _, direction := range []string{"copy", "input"} {
			t.Run(test.name+"/"+direction, func(t *testing.T) {
				input := test.selectData(cloneTestFixture(t))
				want := test.selectData(cloneTestFixture(t))
				first, second := cloneData(input), cloneData(input)
				third := cloneData(first)
				cloneTestEqual(t, input, want)
				cloneTestEqual(t, first, want)
				cloneTestEqual(t, second, want)
				cloneTestEqual(t, third, want)
				if direction == "copy" {
					cloneTestMutateData(t, &first)
					cloneTestEqual(t, input, want)
				} else {
					cloneTestMutateData(t, &input)
					cloneTestEqual(t, first, want)
				}
				cloneTestEqual(t, second, want)
				cloneTestEqual(t, third, want)
			})
		}
	}
}

func TestCloneDataEmptyCollectionAndAnswersPreserveStorage(t *testing.T) {
	value := cloneTestFixture(t)
	input := lesson.Data{Type: lesson.DataCollection, Role: cloneTestRole(t, lesson.RoleBody),
		Value: make([]lesson.Data, 0, 1), Answers: make([]string, 0, 1), Hint: ""}
	value.Document.Pages[0].Nodes[0].Data = []lesson.Data{input}
	cloneTestValid(t, value)
	got := cloneData(input)
	cloneTestEqual(t, got, input)
	children, ok := got.Value.([]lesson.Data)
	if !ok || children == nil || len(children) != 0 || got.Answers == nil {
		t.Fatal("empty typed collection or answers lost its representation")
	}
	input.Value = append(input.Value.([]lesson.Data), lesson.Data{Hint: "source"})
	got.Value = append(children, lesson.Data{Hint: "copy"})
	input.Answers = append(input.Answers, "source")
	got.Answers = append(got.Answers, "copy")
	if input.Value.([]lesson.Data)[0].Hint != "source" || input.Answers[0] != "source" {
		t.Fatal("empty collection or answers shares backing storage")
	}
}

func TestCloneDataNoArtificialCollectionDepthLimit(t *testing.T) {
	const depth = 512
	makeCollection := func() lesson.Data {
		branch := cloneTestText(t, nil, "  <i>_____ {ANSWER}</i>\n\n\\\t ")
		branch.Answers = []string{"первый", "второй"}
		for index := 0; index < depth; index++ {
			branch = lesson.Data{Type: lesson.DataCollection, Value: []lesson.Data{branch}, Answers: []string{}, Hint: "уровень"}
		}
		branch.Role = cloneTestRole(t, lesson.RoleBody)
		return branch
	}
	input, want := makeCollection(), makeCollection()
	value := cloneTestFixture(t)
	value.Document.Pages[0].Nodes[0].Data = []lesson.Data{input}
	cloneTestValid(t, value)
	got := cloneData(input)
	cloneTestEqual(t, input, want)
	cloneTestEqual(t, got, want)
	leaf := got
	for index := 0; index < depth; index++ {
		children, ok := leaf.Value.([]lesson.Data)
		if !ok || len(children) != 1 {
			t.Fatalf("collection cut off at depth %d", index)
		}
		leaf = children[0]
	}
	leaf.Answers[0] = "changed"
	cloneTestEqual(t, input, want)
}

func cloneTestMutateData(t *testing.T, value *lesson.Data) {
	t.Helper()
	if value.Role != nil {
		*value.Role = lesson.RoleAfter
	}
	for index := range value.Answers {
		value.Answers[index] += "changed"
	}
	value.Hint += "changed"
	switch content := value.Value.(type) {
	case string:
		value.Value = content + "changed"
	case lesson.EmbeddedFile:
		content.FileName += "changed"
		content.MIMEType += "changed"
		content.Content += "changed"
		value.Value = content
	case []lesson.Data:
		for index := range content {
			cloneTestMutateData(t, &content[index])
		}
		if len(content) > 1 {
			content[0], content[1] = content[1], content[0]
		}
	default:
		t.Fatalf("fixture has unsupported union member %T", content)
	}
	value.Type = lesson.DataText
}
