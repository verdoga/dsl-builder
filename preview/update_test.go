package preview

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
)

func TestUpdateHeadingUsesFinalMetadataAndLiteralVersions(t *testing.T) {
	for _, versions := range [][2]string{
		{"5", "6"},
		{"900719925474099312345678901234567890", "900719925474099312345678901234567891"},
		{"12e+300", "12e+300"},
		{"5.00", "6.0"},
	} {
		t.Run(versions[0], func(t *testing.T) {
			old := updateTestLesson()
			old.Document.Version = json.Number(versions[0])
			old.Document.Title = "ПРЕЖНЕЕ НАЗВАНИЕ"
			old.Document.Subtitle = updateTestString("ПРЕЖНИЙ ПОДЗАГОЛОВОК")
			final := updateTestLesson()
			final.Document.Version = json.Number(versions[1])
			final.Document.Title = "  Итоговый Урок 🌍  "
			final.Document.Subtitle = updateTestString("  Финальный подзаголовок β  ")
			before := updateTestFile(t, old, true)
			after := updateTestFile(t, final, false)
			snapshot := updateTestSnapshot(t, before, after, nil)
			path := `C:\Курсы и уроки\Модуль 🌍.zip`
			text, err := UpdateHeading(path, before, after)
			if err != nil {
				t.Fatal(err)
			}
			updateTestContains(t, text, "Курсы и уроки", "Модуль 🌍.zip", "l042.json", "EFX7", final.Document.Title, *final.Document.Subtitle)
			updateTestQuoted(t, text, path)
			updateTestOrder(t, text, versions[0], versions[1])
			updateTestExcludes(t, text, old.Document.Title, *old.Document.Subtitle)
			updateTestClean(t, text)
			if got := updateTestSnapshot(t, before, after, nil); got != snapshot {
				t.Fatal("UpdateHeading изменил входные снимки")
			}
		})
	}
}

func TestUpdateHeadingPreservesSafeExternalStrings(t *testing.T) {
	value := updateTestLesson()
	value.Document.Title = "Заголовок\r\nПродолжение\x1b[31m Конец"
	value.Document.Subtitle = updateTestString("Подзаголовок\tХвост")
	file := updateTestFile(t, value, false)
	text, err := UpdateHeading("Архив\r\nВторая строка\x1b[2J.zip", file, file)
	if err != nil {
		t.Fatal(err)
	}
	updateTestContains(t, text, "Заголовок", "Продолжение", "Конец", "Подзаголовок", "Хвост", "Архив", "Вторая строка")
	updateTestExcludes(t, text, "Заголовок\r\n", "Архив\r\n", "Подзаголовок\t")
	updateTestClean(t, text)
}

func TestUpdateHeadingAcceptsAbsentAndEmptySubtitle(t *testing.T) {
	for _, subtitle := range []*string{nil, updateTestString("")} {
		value := updateTestLesson()
		value.Document.Subtitle = subtitle
		file := updateTestFile(t, value, false)
		text, err := UpdateHeading("Модуль.zip", file, file)
		if err != nil {
			t.Fatal(err)
		}
		updateTestContains(t, text, "EFX7", "l042.json", value.Document.Title)
		if subtitle != nil {
			updateTestContains(t, text, `""`)
		}
		updateTestClean(t, text)
	}
}

func TestUpdateHeadingRejectsMissingSnapshots(t *testing.T) {
	file := updateTestFile(t, updateTestLesson(), true)
	for _, pair := range [][2]*module.File{{nil, file}, {file, nil}, {new(module.File), file}, {file, new(module.File)}} {
		text, err := UpdateHeading("Секретный путь.zip", pair[0], pair[1])
		updateTestError(t, text, err)
	}
}

func updateTestString(value string) *string { return &value }

func updateTestNode(id string, children ...lesson.Node) lesson.Node {
	if children == nil {
		children = []lesson.Node{}
	}
	role := lesson.RoleBody
	return lesson.Node{
		ID: id, Type: lesson.NodeText, Nodes: children,
		Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: "DATA-SECRET", Answers: []string{"ANSWER-SECRET"}, Hint: "HINT-SECRET"}},
	}
}

func updateTestLesson() *lesson.Lesson {
	media := updateTestNode("efx7-n035-v03")
	media.Type = lesson.NodeMedia
	media.Data[0].Type = lesson.DataMedia
	media.Data[0].Value = lesson.EmbeddedFile{FileName: "скрытый.bin", MIMEType: "application/octet-stream", Content: "U0VDUkVULU1FRElB"}
	return &lesson.Lesson{
		FormatVersion: lesson.FormatVersion,
		Document: lesson.Document{
			ID: "EFX7", Version: "5", Title: "Итоговый урок", LineCount: "999999999999999999999999", CreatedAt: "2026-11-07T12:00:00Z",
			Pages: []lesson.Page{
				{ID: "efx7-p001", Title: updateTestString("Первая страница"), Nodes: []lesson.Node{updateTestNode("efx7-n001", updateTestNode("efx7-n002")), media}},
				{ID: "efx7-p007", Title: updateTestString("Вторая страница"), Nodes: []lesson.Node{updateTestNode("efx7-n099")}},
			},
		},
	}
}

func updateTestFile(t *testing.T, value *lesson.Lesson, loaded bool) *module.File {
	t.Helper()
	context := diagnostic.Context{Path: "Несуществующий архив.zip"}
	if !loaded {
		file, problems := module.NewFile("l042.json", value, context)
		if len(problems) != 0 {
			t.Fatalf("NewFile: %v", problems)
		}
		return file
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(`{"originalSentinel":"ORIGINAL-SECRET",`), encoded[1:]...)
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		t.Fatalf("ValidateJSON: %v", problems)
	}
	var decoded lesson.Lesson
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	file, problems := module.LoadedFile("l042.json", &decoded, raw, context)
	if len(problems) != 0 {
		t.Fatalf("LoadedFile: %v", problems)
	}
	return file
}

func updateTestSnapshot(t *testing.T, before, after *module.File, plan *compare.Plan) string {
	t.Helper()
	value := struct {
		Before, After  *lesson.Lesson
		OldRaw, NewRaw []byte
		Plan           *compare.Plan
	}{before.Lesson(), after.Lesson(), before.OriginalBytes(), after.OriginalBytes(), plan}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func updateTestContains(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			t.Errorf("нет %q в тексте:\n%s", fragment, text)
		}
	}
}

func updateTestExcludes(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			t.Errorf("лишнее %q в тексте:\n%s", fragment, text)
		}
	}
}

func updateTestOrder(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		index := strings.Index(text, fragment)
		if index < 0 {
			t.Fatalf("нарушен порядок: нет %q в оставшемся тексте %q", fragment, text)
		}
		text = text[index+len(fragment):]
	}
}

func updateTestQuoted(t *testing.T, text, value string) {
	t.Helper()
	for _, quoted := range regexp.MustCompile(`"(?:\\.|[^"\\])*"`).FindAllString(text, -1) {
		if decoded, err := strconv.Unquote(quoted); err == nil && decoded == value {
			return
		}
	}
	t.Errorf("нет полного экранированного значения %q в тексте:\n%s", value, text)
}

func updateTestClean(t *testing.T, text string) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Error("вывод не UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("сырой управляющий символ %U", r)
		}
	}
	updateTestExcludes(t, text, "DATA-SECRET", "ANSWER-SECRET", "HINT-SECRET", "U0VDUkVULU1FRElB", "ORIGINAL-SECRET", "Записать изменения?", "Ваш выбор", "1. Да", "успешно сохран", "Содержательных изменений не обнаружено", "архив оставлен без изменений")
}

func updateTestError(t *testing.T, text string, err error) {
	t.Helper()
	if err == nil || text != "" {
		t.Fatalf("ожидалась ошибка с пустым текстом: text=%q, err=%v", text, err)
	}
	hasRussian := false
	for _, r := range err.Error() {
		hasRussian = hasRussian || unicode.In(r, unicode.Cyrillic)
	}
	if !hasRussian {
		t.Errorf("ошибка не на русском: %v", err)
	}
	updateTestClean(t, err.Error())
}
