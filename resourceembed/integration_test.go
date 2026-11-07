package resourceembed_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourceembed"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
)

type task11Expected struct {
	name, mime string
	role       lesson.DataRole
	content    []byte
}

func task11Write(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
}

func task11Find(t *testing.T, dir, kind, name string, dirs []string) (resourcefind.Selection, diagnostic.Context, []string) {
	t.Helper()
	context := diagnostic.Context{Path: filepath.Join(dir, "исходный урок.json"), FileName: "l001.json", LessonID: "EFX7", PageID: "efx7-p001", Line: 35, Column: 14}
	task11Write(t, context.Path, []byte("{\"source\":\"не изменять\"}\r\n"))
	finder, inventory, problem := resourcefind.Prepare(&filetree.Resolver{}, context.Path, dirs, context)
	if problem != nil {
		t.Fatalf("Prepare: %+v", problem)
	}
	selection, problem := finder.Find(kind, `"`+name+`"`, name, context)
	if problem != nil {
		t.Fatalf("Find: %+v", problem)
	}
	return selection, context, inventory
}

func task11Check(t *testing.T, actual []lesson.Data, expected []task11Expected) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("получено %d элементов, требуется %d", len(actual), len(expected))
	}
	for i, want := range expected {
		got := actual[i]
		if got.Type != lesson.DataMedia || got.Role == nil || *got.Role != want.role || got.Answers == nil || len(got.Answers) != 0 || got.Hint != "" {
			t.Fatalf("элемент %d: неверные type/role/answers/hint: %+v", i, got)
		}
		file, ok := got.Value.(lesson.EmbeddedFile)
		if !ok {
			t.Fatalf("элемент %d: Value имеет тип %T", i, got.Value)
		}
		if file.FileName != want.name || file.MIMEType != want.mime || file.Content != base64.StdEncoding.EncodeToString(want.content) {
			t.Fatalf("элемент %d: неверные имя, MIME или полное Base64", i)
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(file.Content)
		if err != nil || !bytes.Equal(decoded, want.content) {
			t.Fatalf("элемент %d: не сохранены исходные байты: %v", i, err)
		}
		for j := 0; j < i; j++ {
			if got.Role == actual[j].Role {
				t.Fatalf("элементы %d и %d разделяют указатель Role", i, j)
			}
		}
	}
}

func task11Lesson(data []lesson.Data) lesson.Lesson {
	role := lesson.RoleBody
	return lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EFX7", Version: json.Number("1"), Title: "Медиа", LineCount: json.Number("36"), CreatedAt: "2026-11-07T00:00:00Z",
		Pages: []lesson.Page{{ID: "efx7-p001", Nodes: []lesson.Node{
			{ID: "efx7-n001", Type: lesson.NodeMedia, Data: data, Nodes: []lesson.Node{}},
			{ID: "efx7-n002", Type: lesson.NodeScript, Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: "Исходная расшифровка", Answers: []string{}}}, Nodes: []lesson.Node{}},
		}}},
	}}
}

func task11CopySelection(selection resourcefind.Selection) resourcefind.Selection {
	selection.Subtitles = slices.Clone(selection.Subtitles)
	selection.AccountedPaths = slices.Clone(selection.AccountedPaths)
	if selection.Script != nil {
		script := *selection.Script
		selection.Script = &script
	}
	return selection
}

func task11Unchanged(t *testing.T, actual, before resourcefind.Selection) {
	t.Helper()
	if actual.Body != before.Body || !slices.Equal(actual.Subtitles, before.Subtitles) || !slices.Equal(actual.AccountedPaths, before.AccountedPaths) {
		t.Fatal("Embed изменил Selection")
	}
	if (actual.Script == nil) != (before.Script == nil) || (actual.Script != nil && *actual.Script != *before.Script) {
		t.Fatal("Embed изменил Selection.Script")
	}
}

func TestTask11FindEmbedPreservesFilesAndValidLesson(t *testing.T) {
	dir := t.TempDir()
	mediaDir := filepath.Join(dir, "ресурсы с пробелами")
	if err := os.Mkdir(mediaDir, 0700); err != nil {
		t.Fatal(err)
	}
	expected := []task11Expected{
		{"Лекция 01.MP4", "video/mp4", lesson.RoleBody, []byte{0, 255, 128, 1, 2}},
		{"Лекция 01.txt", "text/plain", lesson.RoleScript, []byte{239, 187, 191, 'A', '\r', '\n', 255}},
		{"Лекция 01.vtt", "text/vtt", lesson.RoleSubtitles, []byte{}},
		{"Лекция 01.srt", "application/x-subrip", lesson.RoleSubtitles, []byte("\r\n1\r\n")},
		{"Лекция 01.ttml", "application/ttml+xml", lesson.RoleSubtitles, []byte{255, 254, '<', 0, 't', 0}},
		{"Лекция 01.dfxp", "application/ttml+xml", lesson.RoleSubtitles, []byte{239, 187, 191, 'x'}},
		{"Лекция 01.ass", "text/plain", lesson.RoleSubtitles, []byte("same\r\n")},
		{"Лекция 01.ssa", "text/plain", lesson.RoleSubtitles, []byte("same\r\n")},
	}
	for _, file := range expected {
		task11Write(t, filepath.Join(mediaDir, file.name), file.content)
	}
	task11Write(t, filepath.Join(mediaDir, "не использован.bin"), []byte("unused"))
	selection, context, inventory := task11Find(t, dir, "video", filepath.Join(filepath.Base(mediaDir), expected[0].name), []string{filepath.Base(mediaDir)})
	before := task11CopySelection(selection)
	var usage resourceusage.Registry
	usage.Register(inventory)
	usage.Register(selection.AccountedPaths)
	registered := usage.Snapshot()
	if registered.Total != 9 || registered.Used != 0 || registered.Unused != 9 {
		t.Fatalf("регистрация до встраивания: %+v", registered)
	}
	actual, problem := resourceembed.Embed(selection, context)
	if problem != nil {
		t.Fatal(problem)
	}
	task11Check(t, actual, expected)
	task11Unchanged(t, selection, before)
	if after := usage.Snapshot(); after.Used != 0 || !slices.Equal(after.UnusedPaths, registered.UnusedPaths) {
		t.Fatal("получение результата Embed изменило учёт")
	}
	value := task11Lesson([]lesson.Data{})
	if problems := lesson.Validate(&value, context); len(problems) != 0 {
		t.Fatalf("невалидная исходная синтетическая модель: %+v", problems)
	}
	value.Document.Pages[0].Nodes[0].Data = actual
	usage.MarkUsed(selection.AccountedPaths)
	if problems := lesson.Validate(&value, context); len(problems) != 0 {
		t.Fatalf("Embed несовместим с фиксированным контрактом lesson: %+v", problems)
	}
	report := usage.Snapshot()
	paths := slices.Clone(selection.AccountedPaths)
	slices.Sort(paths)
	if report.Total != 9 || report.Used != 8 || report.Unused != 1 || !slices.Equal(report.UsedPaths, paths) || value.Document.Pages[0].Nodes[1].Data[0].Value != "Исходная расшифровка" {
		t.Fatalf("после включения в урок: %+v", report)
	}
	repeated, problem := resourceembed.Embed(selection, context)
	if problem != nil {
		t.Fatal(problem)
	}
	*actual[0].Role = lesson.RoleTitle
	actual[1].Value = lesson.EmbeddedFile{Content: "изменено"}
	actual[2].Answers = append(actual[2].Answers, "изменено")
	task11Check(t, repeated, expected)
	task11Unchanged(t, selection, before)
	for _, file := range expected {
		got, err := os.ReadFile(filepath.Join(mediaDir, file.name))
		if err != nil || !bytes.Equal(got, file.content) {
			t.Fatalf("исходный файл %s изменён: %v", file.name, err)
		}
	}
	if got, err := os.ReadFile(context.Path); err != nil || string(got) != "{\"source\":\"не изменять\"}\r\n" {
		t.Fatalf("исходный JSON изменён: %v", err)
	}
}

func TestTask11OptionalCompanionsAndImageBodyOnly(t *testing.T) {
	for _, test := range []struct {
		name, kind, extension string
		companions            []string
		want                  []task11Expected
	}{
		{"empty audio body", "audio", ".mp3", nil, []task11Expected{{"кадр.mp3", "audio/mpeg", lesson.RoleBody, []byte{}}}},
		{"script only", "audio", ".wav", []string{".txt"}, []task11Expected{{"кадр.wav", "audio/wav", lesson.RoleBody, []byte{}}, {"кадр.txt", "text/plain", lesson.RoleScript, []byte{255}}}},
		{"subtitle only", "video", ".webm", []string{".srt"}, []task11Expected{{"кадр.webm", "video/webm", lesson.RoleBody, []byte{}}, {"кадр.srt", "application/x-subrip", lesson.RoleSubtitles, []byte{255}}}},
		{"image ignores companions", "image", ".png", []string{".txt", ".vtt", ".ssa"}, []task11Expected{{"кадр.png", "image/png", lesson.RoleBody, []byte{}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			name := "кадр" + test.extension
			task11Write(t, filepath.Join(dir, name), nil)
			for _, extension := range test.companions {
				task11Write(t, filepath.Join(dir, "кадр"+extension), []byte{255})
			}
			selection, context, _ := task11Find(t, dir, test.kind, name, nil)
			actual, problem := resourceembed.Embed(selection, context)
			if problem != nil {
				t.Fatal(problem)
			}
			task11Check(t, actual, test.want)
		})
	}
}
