package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func selectTestString(value string) *string { return &value }

func selectTestDialog(input string) (*dialog, *bytes.Buffer) {
	output := new(bytes.Buffer)
	return &dialog{input: bufio.NewReader(strings.NewReader(input)), output: output}, output
}

func selectTestSnapshot() app.Snapshot {
	return app.Snapshot{Catalog: &catalog.State{
		CourseTitle: "Курс", Sectioned: true,
		Sections: []catalog.SectionOrder{
			{Title: "Я — первая", Weight: selectTestString("900")},
			{Title: "Пустая рабочая", Weight: selectTestString("001")},
			{Title: "А — вторая", Weight: selectTestString("10")},
		},
		Items: []catalog.Item{
			{Entry: selectTestEntry("l701.json", "BA82", "Первый урок", "900"), Section: selectTestString("А — вторая")},
			{Entry: selectTestEntry("l003.json", "BA83", "Второй урок", "01"), Section: nil},
			{Entry: selectTestEntry("l2000.json", "BA84", "Третий урок", "10"), Section: selectTestString("Я — первая")},
		},
	}}
}

func selectTestEntry(name, id, title, weight string) manifest.Entry {
	return manifest.Entry{FileName: name, ID: id, Title: title, Order: selectTestString(weight),
		Version: "1", LineCount: "3", CreatedAt: "2026-11-07T12:00:00Z"}
}

func selectTestBytes(t *testing.T, value app.Snapshot) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	parts := []json.RawMessage{raw}
	if value.Module != nil {
		for _, file := range value.Module.Files {
			observed := struct {
				Name     string
				Lesson   *lesson.Lesson
				Original []byte
				Context  diagnostic.Context
			}{file.FileName(), file.Lesson(), file.OriginalBytes(), file.Context()}
			content, err := json.Marshal(observed)
			if err != nil {
				t.Fatal(err)
			}
			parts = append(parts, content)
		}
	}
	result, err := json.Marshal(parts)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func selectTestUnchanged(t *testing.T, before []byte, snapshot app.Snapshot) {
	t.Helper()
	if !bytes.Equal(before, selectTestBytes(t, snapshot)) {
		t.Fatal("выбор изменил переданный снимок")
	}
}

func selectTestOrder(t *testing.T, output string, values ...string) {
	t.Helper()
	previous := -1
	for _, value := range values {
		at := strings.Index(output, value)
		if at <= previous {
			t.Fatalf("подпись %q отсутствует или нарушен порядок (позиция %d после %d)", value, at, previous)
		}
		previous = at
	}
}

func selectTestFile(t *testing.T, entry manifest.Entry, body string) *module.File {
	t.Helper()
	role := lesson.RoleBody
	id := strings.ToLower(entry.ID)
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: entry.ID, Title: entry.Title, Version: entry.Version,
		LineCount: entry.LineCount, CreatedAt: entry.CreatedAt,
		Pages: []lesson.Page{{ID: id + "-p001", Nodes: []lesson.Node{{
			ID: id + "-n001", Type: lesson.NodeText, Nodes: []lesson.Node{},
			Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: body, Answers: []string{}}},
		}}}},
	}}
	file, issues := module.NewFile(entry.FileName, value, diagnostic.Context{FileName: entry.FileName})
	if len(issues) != 0 || file == nil {
		t.Fatalf("module.NewFile fixture: %v", issues)
	}
	return file
}

func TestSelectLessonPreservesItemsOrderAndReturnsFileName(t *testing.T) {
	for index, name := range []string{"l701.json", "l003.json", "l2000.json"} {
		t.Run(name, func(t *testing.T) {
			snapshot := selectTestSnapshot()
			before := selectTestBytes(t, snapshot)
			d, output := selectTestDialog(string(rune('1'+index)) + "\n")
			got, accepted, err := d.chooseLesson(snapshot)
			if err != nil || !accepted || got != name {
				t.Fatalf("chooseLesson = (%q, %v, %v), нужен %q", got, accepted, err, name)
			}
			selectTestOrder(t, output.String(), "l701.json", "l003.json", "l2000.json")
			for _, item := range snapshot.Catalog.Items {
				for _, text := range []string{item.Entry.ID, item.Entry.Title} {
					if !strings.Contains(output.String(), text) {
						t.Fatalf("в списке отсутствует %q", text)
					}
				}
			}
			selectTestUnchanged(t, before, snapshot)
		})
	}
}

func TestSelectLessonDoesNotPrintNodeData(t *testing.T) {
	snapshot := selectTestSnapshot()
	const body = "СЕКРЕТНОЕ_СОДЕРЖИМОЕ_НОДЫ_QWxwaGE="
	snapshot.Module = &module.Module{}
	for _, item := range snapshot.Catalog.Items {
		snapshot.Module.Files = append(snapshot.Module.Files, selectTestFile(t, item.Entry, body))
	}
	before := selectTestBytes(t, snapshot)
	d, output := selectTestDialog("1\n")
	if _, accepted, err := d.chooseLesson(snapshot); err != nil || !accepted {
		t.Fatalf("chooseLesson = (%v, %v)", accepted, err)
	}
	for _, forbidden := range []string{body, "СЕКРЕТНОЕ_СОДЕРЖИМОЕ", "QWxwaGE="} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("в меню попало содержимое урока %q", forbidden)
		}
	}
	selectTestUnchanged(t, before, snapshot)
}

func TestSelectLessonLongUnicodeMetadataIsNotTruncatedOrNormalized(t *testing.T) {
	snapshot := selectTestSnapshot()
	title := "  Кириллица 世界🙂 " + strings.Repeat("длинное", 10000) + " КОНЕЦ  "
	name := "l" + strings.Repeat("8", 90) + ".json"
	snapshot.Catalog.Items[0].Entry.FileName = name
	snapshot.Catalog.Items[0].Entry.Title = title
	d, output := selectTestDialog("1\n")
	got, accepted, err := d.chooseLesson(snapshot)
	if err != nil || !accepted || got != name {
		t.Fatalf("длинное имя не возвращено буквально: accepted=%v err=%v", accepted, err)
	}
	if !strings.Contains(output.String(), title) || !strings.Contains(output.String(), name) {
		t.Fatal("Unicode, точные пробелы либо длинные метаданные сокращены")
	}
}

func TestSelectLessonEscapesExternalControls(t *testing.T) {
	snapshot := selectTestSnapshot()
	snapshot.Catalog.Items[0].Entry.Title = "Начало世界\nПОДМЕНА\tЗавершение\x1b[31m"
	d, output := selectTestDialog("1\n")
	_, accepted, err := d.chooseLesson(snapshot)
	if err != nil || !accepted {
		t.Fatalf("chooseLesson = (%v, %v)", accepted, err)
	}
	text := output.String()
	for _, forbidden := range []string{"\nПОДМЕНА", "\t", "\x1b"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("неэкранированный внешний control %q", forbidden)
		}
	}
	for _, preserved := range []string{"Начало世界", "ПОДМЕНА", "Завершение", "31m", "\\"} {
		if !strings.Contains(text, preserved) {
			t.Fatalf("отображение потеряло %q", preserved)
		}
	}
}
