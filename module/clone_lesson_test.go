package module

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func TestCloneLessonIndependentSnapshots(t *testing.T) {
	for _, direction := range []string{"copy", "input"} {
		t.Run(direction, func(t *testing.T) {
			input, want := cloneTestFixture(t), cloneTestFixture(t)
			first, second := cloneLesson(input), cloneLesson(input)
			cloneTestEqual(t, input, want)
			cloneTestEqual(t, first, want)
			cloneTestEqual(t, second, want)
			third := cloneLesson(first)
			if first == input || second == first || third == first {
				t.Fatal("lesson snapshots share their root")
			}
			if direction == "copy" {
				cloneTestMutateLesson(t, first)
				cloneTestEqual(t, input, want)
			} else {
				cloneTestMutateLesson(t, input)
				cloneTestEqual(t, first, want)
			}
			cloneTestEqual(t, second, want)
			cloneTestEqual(t, third, want)
			cloneTestMutateLesson(t, third)
			cloneTestEqual(t, second, want)
		})
	}
}

func TestCloneLessonOptionalTitlesAndExactNumbers(t *testing.T) {
	for subtitleIndex, subtitle := range []*string{nil, new(string), cloneTestString(t, " \tПодзаголовок\n ")} {
		for _, number := range []json.Number{"1", "1.000", "1e+0", "9007199254740993", "123456789012345678901234567890123456789", "1E+9999999999999999999999999"} {
			t.Run(string(number)+"/subtitle"+strconv.Itoa(subtitleIndex), func(t *testing.T) {
				input := cloneTestFixture(t)
				input.Document.Subtitle = subtitle
				input.Document.Version = number
				input.Document.LineCount = number
				cloneTestValid(t, input)
				got := cloneLesson(input)
				cloneTestEqual(t, got, input)
				if subtitle != nil {
					*got.Document.Subtitle += "изменено"
					if *subtitle == *got.Document.Subtitle {
						t.Fatal("subtitle pointer is shared")
					}
				}
			})
		}
	}
	if got := cloneLesson(nil); got != nil {
		t.Fatalf("cloneLesson(nil) = %#v", got)
	}
}

func TestCloneLessonEmptyPagesKeepIndependentStorage(t *testing.T) {
	input := cloneTestFixture(t)
	input.Document.Pages = make([]lesson.Page, 0, 1)
	cloneTestValid(t, input)
	got := cloneLesson(input)
	if got.Document.Pages == nil || len(got.Document.Pages) != 0 {
		t.Fatal("empty pages became nil or nonempty")
	}
	input.Document.Pages = append(input.Document.Pages, lesson.Page{ID: "source"})
	got.Document.Pages = append(got.Document.Pages, lesson.Page{ID: "copy"})
	if input.Document.Pages[0].ID != "source" {
		t.Fatal("empty pages retain shared backing storage")
	}
}

// cloneTestEqual сравнивает всю типизированную модель, включая union и nil/empty.
func cloneTestEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot differs from independently constructed expected model:\ngot: %#v\nwant: %#v", got, want)
	}
}

func cloneTestValid(t *testing.T, value *lesson.Lesson) {
	t.Helper()
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("fixture does not satisfy lesson.Validate: %v", problems)
	}
}

func cloneTestString(t *testing.T, text string) *string {
	t.Helper()
	return &text
}

func cloneTestRole(t *testing.T, role lesson.DataRole) *lesson.DataRole {
	t.Helper()
	return &role
}

func cloneTestBool(t *testing.T, flag bool) *bool {
	t.Helper()
	return &flag
}

func cloneTestText(t *testing.T, role *lesson.DataRole, text string) lesson.Data {
	t.Helper()
	return lesson.Data{Type: lesson.DataText, Role: role, Value: text, Answers: []string{}, Hint: ""}
}

func cloneTestFixture(t *testing.T) *lesson.Lesson {
	t.Helper()
	media := func(role lesson.DataRole, name, mime string, raw []byte) lesson.Data {
		return lesson.Data{Type: lesson.DataMedia, Role: cloneTestRole(t, role),
			Value:   lesson.EmbeddedFile{FileName: name, MIMEType: mime, Content: base64.StdEncoding.EncodeToString(raw)},
			Answers: []string{}, Hint: ""}
	}
	question := cloneTestText(t, cloneTestRole(t, lesson.RoleBody), " Вопрос?\n\t<b>Ёж</b> ")
	question.Answers, question.Hint = []string{"Да", " Нет ", ""}, "Подсказка\r\n\\"
	collection := lesson.Data{Type: lesson.DataCollection, Role: cloneTestRole(t, lesson.RoleBody), Answers: []string{}, Hint: "верх",
		Value: []lesson.Data{
			{Type: lesson.DataText, Value: "первый", Answers: []string{"один", "два"}, Hint: "дочерняя"},
			{Type: lesson.DataCollection, Answers: []string{}, Hint: "вложенная", Value: []lesson.Data{
				{Type: lesson.DataMedia, Value: lesson.EmbeddedFile{FileName: "файл с пробелом.bin", MIMEType: "opaque/type", Content: "AP9BAAoNCg=="}, Answers: []string{}},
				cloneTestText(t, cloneTestRole(t, lesson.RoleHint), "последний\n\n_____ {не разбирать}"),
			}},
		}}
	node := func(id string, kind lesson.NodeType, data ...lesson.Data) lesson.Node {
		if data == nil {
			data = []lesson.Data{}
		}
		return lesson.Node{ID: id, Type: kind, Data: data, Nodes: []lesson.Node{}}
	}
	questionNode := node("clone+урок-n021-v07", lesson.NodeQuestion, question)
	questionNode.Flags.Checkable = cloneTestBool(t, false)
	task := node("clone+урок-n009", lesson.NodeTask)
	task.Flags.Speaking, task.Flags.Stepped = cloneTestBool(t, true), cloneTestBool(t, false)
	task.Nodes = []lesson.Node{
		questionNode,
		node("clone+урок-n003", lesson.NodeMultifill, cloneTestText(t, cloneTestRole(t, lesson.RoleBody), " I _____{am} Camila.\n\nWe _____{are}.\t<b>&</b> ")),
		node("clone+урок-n015", lesson.NodeMedia,
			media(lesson.RoleBody, "Клип 01.mp4", "video/mp4", []byte{0, 255, 65, 0, 10, 13, 10}),
			media(lesson.RoleScript, "Клип 01.txt", "text/plain", []byte("\xef\xbb\xbfТекст\r\n")),
			media(lesson.RoleSubtitles, "Клип 01.vtt", "text/vtt", []byte("WEBVTT\n\n"))),
		node("clone+урок-n002", lesson.NodeText, collection),
	}
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "CLONE+Урок", Version: "9007199254740993.00", Title: " \tУрок <b>Ё</b>\n ", Subtitle: cloneTestString(t, " Подзаголовок "),
		LineCount: "123456789012345678901234567890e+2", CreatedAt: "2026-11-04T12:34:56Z",
		Pages: []lesson.Page{
			{ID: "clone+урок-p003", Title: cloneTestString(t, " \tСтраница\n "), Nodes: []lesson.Node{task,
				node("clone+урок-n001", lesson.NodeTable, cloneTestText(t, cloneTestRole(t, lesson.RoleBody), "<table>\n<tr><td>A & B</td></tr>\n</table>"))}},
			{ID: "clone+урок-p001", Title: nil, Nodes: []lesson.Node{}},
		},
	}}
	cloneTestValid(t, value)
	return value
}

func cloneTestMutateLesson(t *testing.T, value *lesson.Lesson) {
	t.Helper()
	value.FormatVersion += "changed"
	value.Document.ID += "changed"
	value.Document.Version = "2"
	value.Document.Title += "changed"
	if value.Document.Subtitle != nil {
		*value.Document.Subtitle += "changed"
	}
	value.Document.LineCount = "0"
	value.Document.CreatedAt += "changed"
	for index := range value.Document.Pages {
		cloneTestMutatePage(t, &value.Document.Pages[index])
	}
}
