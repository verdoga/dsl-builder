package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func compositionTestString(value string) *string { return &value }

func compositionTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func compositionTestFile(t *testing.T, name, id string, loaded bool) *module.File {
	t.Helper()
	prefix := strings.ToLower(id)
	body := lesson.RoleBody
	leaf := func(number int) lesson.Node {
		return lesson.Node{ID: fmt.Sprintf("%s-n%03d", prefix, number), Type: lesson.NodeText,
			Data: []lesson.Data{}, Nodes: []lesson.Node{}}
	}
	textNode, mediaNode := leaf(3), leaf(6)
	textNode.Data = []lesson.Data{{Type: lesson.DataText, Role: &body,
		Value: "PRIVATE-BODY", Answers: []string{"PRIVATE-ANSWER"}, Hint: "PRIVATE-HINT"}}
	mediaNode.Type = lesson.NodeMedia
	mediaNode.Data = []lesson.Data{{Type: lesson.DataMedia, Role: &body,
		Value:   lesson.EmbeddedFile{FileName: "private.mp3", MIMEType: "audio/mpeg", Content: "cHJpdmF0ZS1tZWRpYQ=="},
		Answers: []string{}, Hint: ""}}
	step, variant, branches, task := leaf(2), leaf(5), leaf(4), leaf(1)
	step.Type, step.Nodes = lesson.NodeStep, []lesson.Node{textNode}
	variant.Type, variant.Nodes = lesson.NodeVariant, []lesson.Node{mediaNode}
	branches.Type, branches.Nodes = lesson.NodeVariants, []lesson.Node{variant}
	task.Type, task.Nodes = lesson.NodeTask, []lesson.Node{step, branches}
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: id, Title: "FILE-TITLE", Subtitle: compositionTestString("FILE-SUBTITLE"),
		Version: "987654321987654321987654321", LineCount: "17", CreatedAt: "2026-11-04T09:00:00Z",
		Pages: []lesson.Page{{ID: prefix + "-p001", Nodes: []lesson.Node{task}},
			{ID: prefix + "-p002", Nodes: []lesson.Node{leaf(7)}}},
	}}
	var file *module.File
	var problems []*diagnostic.Error
	if loaded {
		raw := compositionTestJSON(t, value)
		file, problems = module.LoadedFile(name, value, raw, diagnostic.Context{Path: "архив.zip"})
	} else {
		file, problems = module.NewFile(name, value, diagnostic.Context{Path: "исходник.json"})
	}
	if len(problems) != 0 || file == nil {
		t.Fatalf("настоящий конструктор отклонил фикстуру: %v", problems)
	}
	return file
}

func compositionTestItem(file *module.File) catalog.Item {
	doc := file.Lesson().Document
	return catalog.Item{Entry: manifest.Entry{FileName: file.FileName(), ID: doc.ID,
		Title:     "ENTRY-" + strings.TrimSuffix(strings.TrimPrefix(file.FileName(), "l"), ".json"),
		Order:     compositionTestString("0010"),
		LineCount: "900719925474099312345678901234567890", Version: "123456789123456789",
		CreatedAt: doc.CreatedAt}}
}

func compositionTestContains(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(text, value) {
			t.Errorf("нет полного значения %q в %q", value, text)
		}
	}
}

func compositionTestCount(t *testing.T, text, label string, count int) {
	t.Helper()
	pattern := fmt.Sprintf(`(?i)%s[^\d\n]*%d([^\d]|$)`, label, count)
	if !regexp.MustCompile(pattern).MatchString(text) {
		t.Errorf("нет счётчика %s=%d в %q", label, count, text)
	}
}

func compositionTestError(t *testing.T, text string, err error) {
	t.Helper()
	if err == nil || text != "" {
		t.Fatalf("нужны пустой текст и error: text=%q error=%v", text, err)
	}
	if !strings.ContainsFunc(err.Error(), func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("нет русской причины: %v", err)
	}
}

func compositionTestSafe(t *testing.T, text string) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Fatal("текст не UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("сырой управляющий символ U+%04X", r)
		}
	}
}

func TestCompositionLessonUsesEntryAndLiteralMetadata(t *testing.T) {
	file := compositionTestFile(t, "l001.json", "EFX7", true)
	for _, tc := range []struct {
		name  string
		order *string
	}{
		{"missing", nil}, {"empty", compositionTestString("")},
		{"invalid", compositionTestString("не вес")},
		{"huge", compositionTestString("0001234567890123456789012345678901234567890")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := compositionTestItem(file)
			item.Entry.Order = tc.order
			item.Entry.Subtitle = compositionTestString("Подзаголовок 日本語")
			before := compositionTestJSON(t, item)
			text, err := compositionLesson(item, file, false)
			if err != nil {
				t.Fatal(err)
			}
			compositionTestContains(t, text, item.Entry.FileName, item.Entry.Title,
				*item.Entry.Subtitle, item.Entry.LineCount.String())
			if tc.order == nil {
				compositionTestContains(t, text, "не задан")
			} else if *tc.order != "" {
				compositionTestContains(t, text, *tc.order)
			}
			compositionTestCount(t, text, "страниц", 2)
			compositionTestCount(t, text, "нод", 7)
			if len(strings.Split(strings.TrimSpace(text), "\n")) != 1 {
				t.Errorf("строка урока не компактна: %q", text)
			}
			for _, hidden := range []string{"FILE-TITLE", "FILE-SUBTITLE", "PRIVATE-BODY", "PRIVATE-ANSWER",
				"PRIVATE-HINT", "cHJpdmF0ZS1tZWRpYQ==", "formatVersion", item.Entry.Version.String(),
				file.Lesson().Document.Version.String()} {
				if strings.Contains(text, hidden) {
					t.Errorf("утечка %q: %q", hidden, text)
				}
			}
			if !bytes.Equal(before, compositionTestJSON(t, item)) {
				t.Fatal("Entry изменён")
			}
		})
	}
}

func TestCompositionLessonNewMarkerAndOptionalSubtitle(t *testing.T) {
	file := compositionTestFile(t, "l001.json", "EfX7", false)
	item := compositionTestItem(file)
	for _, subtitle := range []*string{nil, compositionTestString(""), compositionTestString("Есть подзаголовок")} {
		item.Entry.Subtitle = subtitle
		text, err := compositionLesson(item, file, true)
		if err != nil {
			t.Fatal(err)
		}
		compositionTestContains(t, text, "НОВЫЙ", item.Entry.ID)
		if subtitle != nil && *subtitle != "" {
			compositionTestContains(t, text, *subtitle)
		}
	}
}

func TestCompositionCountsUseEveryPageAndNestedNode(t *testing.T) {
	first := compositionTestFile(t, "l001.json", "EFX7", false)
	second := compositionTestFile(t, "l002.json", "BA88", true)
	pages, nodes, err := lessonCounts(first)
	if err != nil || pages != 2 || nodes != 7 {
		t.Fatalf("lessonCounts=(%d,%d,%v), нужны 2 страницы и 7 нод", pages, nodes, err)
	}
	for _, files := range [][]*module.File{nil, {}, {first, second}} {
		text, err := totals(files)
		if err != nil {
			t.Fatal(err)
		}
		compositionTestCount(t, text, "уроков", len(files))
		compositionTestCount(t, text, "страниц", len(files)*2)
		compositionTestCount(t, text, "нод", len(files)*7)
	}
	empty := first.Lesson()
	empty.Document.Pages = []lesson.Page{}
	file, problems := module.NewFile("l003.json", empty, diagnostic.Context{})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	pages, nodes, err = lessonCounts(file)
	if err != nil || pages != 0 || nodes != 0 {
		t.Fatalf("пустой готовый урок: (%d,%d,%v)", pages, nodes, err)
	}
	text, err := totals([]*module.File{first, file, second})
	if err != nil {
		t.Fatal(err)
	}
	compositionTestCount(t, text, "уроков", 3)
	compositionTestCount(t, text, "страниц", 4)
	compositionTestCount(t, text, "нод", 14)
}

func TestCompositionCountsRejectMissingSnapshots(t *testing.T) {
	for _, file := range []*module.File{nil, {}} {
		if _, _, err := lessonCounts(file); err == nil {
			t.Error("lessonCounts принял отсутствующий снимок")
		}
		text, err := totals([]*module.File{file})
		compositionTestError(t, text, err)
		text, err = compositionLesson(catalog.Item{}, file, false)
		compositionTestError(t, text, err)
	}
}
