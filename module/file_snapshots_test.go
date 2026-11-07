package module

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func modFileRich(t *testing.T) *lesson.Lesson {
	t.Helper()
	input := modFileFixture(t)
	subtitle, title := "Подзаголовок\r\n", "Страница\n"
	speaking, stepped, checkable := false, true, false
	body, subtitles, nestedRole := lesson.RoleBody, lesson.RoleSubtitles, lesson.RoleTitle
	input.Document.Subtitle = &subtitle
	inner := []lesson.Data{
		{Type: lesson.DataText, Role: &nestedRole, Value: "I _____{am}\n\n<b>Кириллица</b>", Answers: []string{"ответ", "<&>"}, Hint: "точная\r\nподсказка"},
		{Type: lesson.DataCollection, Value: []lesson.Data{}, Answers: []string{}, Hint: ""},
	}
	collection := []lesson.Data{{Type: lesson.DataCollection, Value: inner, Answers: []string{}, Hint: "вложенная подсказка"}}
	child := lesson.Node{
		ID: "efx7-n002-v03", Type: lesson.NodeQuestion, Flags: lesson.NodeFlags{Checkable: &checkable},
		Data: []lesson.Data{{Type: lesson.DataMedia, Role: &subtitles,
			Value:   lesson.EmbeddedFile{FileName: "Субтитры.vtt", MIMEType: "text/vtt", Content: "AAEC/w=="},
			Answers: []string{}, Hint: ""}}, Nodes: []lesson.Node{},
	}
	node := lesson.Node{
		ID: "efx7-n001", Type: lesson.NodeTask, Flags: lesson.NodeFlags{Speaking: &speaking, Stepped: &stepped},
		Data:  []lesson.Data{{Type: lesson.DataCollection, Role: &body, Answers: []string{}, Hint: "внешняя подсказка", Value: collection}},
		Nodes: []lesson.Node{child},
	}
	input.Document.Pages = []lesson.Page{
		{ID: "efx7-p001", Title: &title, Nodes: []lesson.Node{node}},
		{ID: "efx7-p002", Nodes: []lesson.Node{}},
	}
	if problems := lesson.Validate(input, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("fixture не прошёл реальный lesson.Validate: %v", problems)
	}
	return input
}

func modFileDecode(t *testing.T, raw []byte, context diagnostic.Context) *lesson.Lesson {
	t.Helper()
	if !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("fixture должен быть UTF-8 без BOM")
	}
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		t.Fatalf("fixture не прошёл реальный lesson.ValidateJSON: %v", problems)
	}
	var decoded lesson.Lesson
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return &decoded
}

func modFileMutateRich(t *testing.T, value *lesson.Lesson) {
	t.Helper()
	value.FormatVersion = "изменено"
	value.Document.ID, value.Document.Version = "other", "4"
	value.Document.Title, value.Document.LineCount = "другое", "9"
	value.Document.CreatedAt = "2000-01-01T00:00:00Z"
	*value.Document.Subtitle = "другой подзаголовок"
	page := &value.Document.Pages[0]
	page.ID = "changed-page"
	*page.Title = "другая страница"
	node := &page.Nodes[0]
	node.ID, node.Type = "changed-node", lesson.NodeText
	*node.Flags.Speaking = true
	*node.Flags.Stepped = false
	*node.Data[0].Role = lesson.RoleHint
	node.Data[0].Hint = "новая подсказка"
	collection := node.Data[0].Value.([]lesson.Data)
	collection[0].Hint = "новая вложенная подсказка"
	inner := collection[0].Value.([]lesson.Data)
	*inner[0].Role = lesson.RoleBody
	inner[0].Value, inner[0].Answers[0], inner[0].Hint = "другое тело", "другой ответ", "другое"
	inner[1].Value = append(inner[1].Value.([]lesson.Data), lesson.Data{})
	*node.Nodes[0].Flags.Checkable = true
	node.Nodes[0].Data[0].Value = lesson.EmbeddedFile{FileName: "замена.bin", Content: "QQ=="}
	*node.Nodes[0].Data[0].Role = lesson.RoleBody
	node.Nodes[0].Nodes = append(node.Nodes[0].Nodes, lesson.Node{})
	node.Data = append(node.Data, lesson.Data{})
	node.Nodes = append(node.Nodes, lesson.Node{})
	page.Nodes = append(page.Nodes, lesson.Node{})
	value.Document.Pages[1].Nodes = append(value.Document.Pages[1].Nodes, lesson.Node{})
	value.Document.Pages = append(value.Document.Pages, lesson.Page{})
}

func TestModFileSnapshotsOwnAllNestedData(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		name := "new"
		if loaded {
			name = "loaded"
		}
		t.Run(name, func(t *testing.T) {
			input := modFileRich(t)
			originalModel := modFileJSON(t, input)
			var raw []byte
			var file *File
			var problems []*diagnostic.Error
			if loaded {
				raw = append([]byte(nil), originalModel...)
				input = modFileDecode(t, raw, diagnostic.Context{})
				file, problems = LoadedFile("l013.json", input, raw, diagnostic.Context{})
			} else {
				file, problems = NewFile("l013.json", input, diagnostic.Context{})
			}
			if file == nil || len(problems) != 0 {
				t.Fatalf("создание снимка: %v", problems)
			}
			copyBefore := file.Lesson()
			modFileMutateRich(t, input)
			modFileRequireModel(t, file.Lesson(), originalModel)
			modFileRequireModel(t, copyBefore, originalModel)
			modFileMutateRich(t, copyBefore)
			modFileRequireModel(t, file.Lesson(), originalModel)
			if loaded && !bytes.Equal(file.OriginalBytes(), raw) {
				t.Fatal("изменение модели затронуло исходные байты")
			}
			if !loaded && file.OriginalBytes() != nil {
				t.Fatal("у нового файла появились исходные байты")
			}
		})
	}
}

func TestModFileLoadedRetainsExactOwnedBytes(t *testing.T) {
	const source = "{\n\t\"document\" : {\"id\":\"EFX7\",\"version\":1.0,\"title\":\"\\u0410\\/\\u003c\",\"lineCount\":0e4,\"createdAt\":\"2026-11-04T12:00:00Z\",\"pages\":[],\"unknown\":{\"x\":true},\"filePath\":\"несуществующий DSL.txt\"},\n\"formatVersion\":\"1.0\",\"extra\":[1,2]}\n"
	for _, ending := range []string{"LF", "CRLF", "no-final-EOL", "trailing-space"} {
		t.Run(ending, func(t *testing.T) {
			sourceText := source
			switch ending {
			case "CRLF":
				sourceText = strings.ReplaceAll(sourceText, "\n", "\r\n")
			case "no-final-EOL":
				sourceText = strings.TrimSuffix(sourceText, "\n")
			case "trailing-space":
				sourceText += " \t\r\n"
			}
			raw := []byte(sourceText)
			context := diagnostic.Context{Path: `C:\Модули\Unit 1.zip`}
			input := modFileDecode(t, raw, context)
			before := modFileJSON(t, input)
			file, problems := LoadedFile("l007.json", input, raw, context)
			if file == nil || len(problems) != 0 {
				t.Fatalf("загрузка: %v", problems)
			}
			raw[0] = '!'
			first := file.OriginalBytes()
			if !bytes.Equal(first, []byte(sourceText)) {
				t.Fatalf("потеряны точные raw: %q", first)
			}
			first[len(first)-1] = '!'
			if !bytes.Equal(file.OriginalBytes(), []byte(sourceText)) {
				t.Fatal("OriginalBytes возвращает внутренний буфер")
			}
			modFileRequireModel(t, file.Lesson(), before)
			expectedContext := context
			expectedContext.FileName, expectedContext.LessonID = "l007.json", "EFX7"
			if file.Context() != expectedContext {
				t.Fatalf("выдуман источник/координаты: %+v", file.Context())
			}
		})
	}
}

func TestModFileLoadedRejectsEmptyOriginalWithoutPartialSuccess(t *testing.T) {
	for _, raw := range [][]byte{nil, {}} {
		input := modFileFixture(t)
		before := modFileJSON(t, input)
		file, problems := LoadedFile("l001.json", input, raw, diagnostic.Context{Path: "модуль.zip"})
		modFileRequireCode(t, file, problems, CodeOriginalBytes)
		modFileRequireModel(t, input, before)
	}
}
