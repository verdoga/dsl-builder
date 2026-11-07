package preview

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func TestCompositionPreservesUnfinishedStateAndExplicitNewSelection(t *testing.T) {
	files := []*module.File{
		compositionTestFile(t, "l001.json", "OLD1", false),
		compositionTestFile(t, "l002.json", "NeW2", true),
		compositionTestFile(t, "l003.json", "OLD3", true),
		compositionTestFile(t, "l004.json", "OLD4", false),
	}
	items := make([]catalog.Item, len(files))
	for i, file := range files {
		items[i] = compositionTestItem(file)
	}
	items[0].Section, items[1].Section = compositionTestString("Я первая"), compositionTestString("Я первая")
	items[0].Entry.Order, items[1].Entry.Order = compositionTestString("999"), compositionTestString("01")
	items[2].Section = compositionTestString("Б последняя")
	items[2].Entry.Order, items[3].Entry.Order = compositionTestString("неверный вес"), nil
	state := &catalog.State{CourseTitle: "Курс 日本語", UnitTitle: compositionTestString("Юнит Ω"),
		Sectioned: true, Items: []catalog.Item{items[2], items[0], items[3], items[1]},
		Sections: []catalog.SectionOrder{{Title: "Я первая", Weight: compositionTestString("00030")},
			{Title: "А резерв"}, {Title: "Б последняя", Weight: compositionTestString("2")}}}
	// Манифест намеренно невалиден и расходится со State: preview поддерживает редактируемый состав.
	value := &module.Module{Manifest: manifest.Manifest{Course: manifest.Course{Title: "STALE-MANIFEST"}},
		Files: []*module.File{files[3], files[2], files[1], files[0]}}
	newNames := []string{"l002.json"}
	beforeState, beforeManifest := compositionTestJSON(t, state), compositionTestJSON(t, value.Manifest)
	beforeLessons := make([][]byte, len(files))
	beforeRaw := make([][]byte, len(files))
	for i, file := range files {
		beforeLessons[i], beforeRaw[i] = compositionTestJSON(t, file.Lesson()), file.OriginalBytes()
	}
	text, err := Composition(state, value, newNames)
	if err != nil {
		t.Fatalf("редактируемый состав отвергнут: %v", err)
	}
	compositionTestContains(t, text, state.CourseTitle, *state.UnitTitle, "00030", "неверный вес", "не задан", "Не распределены")
	if strings.Contains(text, "STALE-MANIFEST") || strings.Contains(text, "Без секций") {
		t.Errorf("источник размещения/группа подменены: %q", text)
	}
	last := -1
	for i, section := range state.Sections {
		pattern := fmt.Sprintf(`(?m)^\s*%d[.)]\s*[^\n]*%s`, i+1, regexp.QuoteMeta(section.Title))
		location := regexp.MustCompile(pattern).FindStringIndex(text)
		if location == nil || location[0] <= last {
			t.Fatalf("нарушены нумерация/порядок секции %q: %q", section.Title, text)
		}
		last = location[0]
	}
	start, end := strings.Index(text, "А резерв"), strings.Index(text, "Б последняя")
	if !strings.Contains(strings.ToLower(text[start:end]), "пуст") {
		t.Errorf("пустая рабочая секция не обозначена: %q", text[start:end])
	}
	if regexp.MustCompile(`(?i)(вес|weight|order|не задан|null)`).MatchString(text[start:end]) {
		t.Errorf("отсутствующий временный вес секции показан: %q", text[start:end])
	}
	if strings.Index(text, "l001.json") > strings.Index(text, "l002.json") {
		t.Error("веса пересортировали Items внутри секции")
	}
	for _, file := range files {
		if strings.Count(text, file.FileName()) != 1 {
			t.Errorf("файл %s показан не один раз: %q", file.FileName(), text)
		}
		for _, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, file.FileName()) {
				continue
			}
			if file.FileName() == newNames[0] {
				compositionTestContains(t, line, "НОВЫЙ", "NeW2")
			} else if strings.Contains(line, "НОВЫЙ") || strings.Contains(line, file.Lesson().Document.ID) {
				t.Errorf("самовольно отмечен старый урок: %q", line)
			}
		}
	}
	compositionTestCount(t, text, "уроков", 4)
	compositionTestCount(t, text, "страниц", 8)
	compositionTestCount(t, text, "нод", 28)
	if !bytes.Equal(beforeState, compositionTestJSON(t, state)) ||
		!bytes.Equal(beforeManifest, compositionTestJSON(t, value.Manifest)) || newNames[0] != "l002.json" {
		t.Fatal("State, Manifest или newFileNames изменены")
	}
	for i, file := range files {
		if !bytes.Equal(beforeLessons[i], compositionTestJSON(t, file.Lesson())) || !bytes.Equal(beforeRaw[i], file.OriginalBytes()) {
			t.Errorf("снимок/raw файла %d изменён", i)
		}
		if value.Files[len(files)-1-i] != file {
			t.Error("срез Files изменён")
		}
	}
	compositionTestSafe(t, text)
}

func TestCompositionFlatCreationAndEmptyComposition(t *testing.T) {
	file := compositionTestFile(t, "l001.json", "EFX7", false)
	item := compositionTestItem(file)
	state := &catalog.State{CourseTitle: "Без юнита", Items: []catalog.Item{item}}
	text, err := Composition(state, &module.Module{Files: []*module.File{file}}, []string{})
	if err != nil {
		t.Fatal(err)
	}
	compositionTestContains(t, text, "Без секций", file.FileName())
	for _, hidden := range []string{"Не распределены", "Юнит", "НОВЫЙ", item.Entry.ID, item.Entry.Version.String()} {
		if strings.Contains(text, hidden) {
			t.Errorf("лишнее поле/состояние %q: %q", hidden, text)
		}
	}
	text, err = Composition(&catalog.State{CourseTitle: "Пустой"}, &module.Module{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	compositionTestCount(t, text, "уроков", 0)
	compositionTestCount(t, text, "страниц", 0)
	compositionTestCount(t, text, "нод", 0)
}

func TestCompositionRejectsInconsistentFileSetsAndSelectors(t *testing.T) {
	first := compositionTestFile(t, "l001.json", "EFX7", false)
	second := compositionTestFile(t, "l002.json", "BA88", true)
	item := compositionTestItem(first)
	wrongCase := item
	wrongCase.Entry.FileName = "L001.json"
	cases := []struct {
		name  string
		state *catalog.State
		value *module.Module
		new   []string
	}{
		{"nil State", nil, &module.Module{}, nil},
		{"nil Module", &catalog.State{}, nil, nil},
		{"missing file", &catalog.State{Items: []catalog.Item{item}}, &module.Module{}, nil},
		{"extra file", &catalog.State{}, &module.Module{Files: []*module.File{first}}, nil},
		{"duplicate file", &catalog.State{Items: []catalog.Item{item}}, &module.Module{Files: []*module.File{first, first}}, nil},
		{"duplicate entry", &catalog.State{Items: []catalog.Item{item, item}}, &module.Module{Files: []*module.File{first}}, nil},
		{"exact name", &catalog.State{Items: []catalog.Item{wrongCase}}, &module.Module{Files: []*module.File{first}}, nil},
		{"different name", &catalog.State{Items: []catalog.Item{item}}, &module.Module{Files: []*module.File{second}}, nil},
		{"nil file", &catalog.State{Items: []catalog.Item{item}}, &module.Module{Files: []*module.File{nil}}, nil},
		{"zero file", &catalog.State{Items: []catalog.Item{item}}, &module.Module{Files: []*module.File{{}}}, nil},
		{"missing new selector", &catalog.State{Items: []catalog.Item{item}}, &module.Module{Files: []*module.File{first}}, []string{"l002.json"}},
		{"inexact new selector", &catalog.State{Items: []catalog.Item{item}}, &module.Module{Files: []*module.File{first}}, []string{"L001.json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := Composition(tc.state, tc.value, tc.new)
			compositionTestError(t, text, err)
		})
	}
}

func TestCompositionGroupsRequireUnambiguousFiles(t *testing.T) {
	file := compositionTestFile(t, "l001.json", "EFX7", false)
	state := &catalog.State{Items: []catalog.Item{compositionTestItem(file)}}
	for _, files := range [][]*module.File{nil, {file, file}} {
		text, err := compositionGroups(state, files, nil)
		compositionTestError(t, text, err)
	}
}

func TestCompositionExternalStringsRemainVisibleAndSafe(t *testing.T) {
	file := compositionTestFile(t, "l001.json", "EFX7", true)
	item := compositionTestItem(file)
	item.Entry.Title = " Заголовок 日本語\nстрока\r\t\x1b[2J "
	item.Entry.Subtitle = compositionTestString("Подзаголовок\x00конец")
	item.Entry.Order = compositionTestString("000\n\x1b[31m")
	state := &catalog.State{CourseTitle: "Курс Ω\r\n\x1b[2J", UnitTitle: compositionTestString("Юнит\t日本語"),
		Sectioned: true, Items: []catalog.Item{item},
		Sections: []catalog.SectionOrder{{Title: "Секция\nконец", Weight: compositionTestString("0\r\x1b[2J")}}}
	state.Items[0].Section = compositionTestString(state.Sections[0].Title)
	text, err := Composition(state, &module.Module{Files: []*module.File{file}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	compositionTestContains(t, text, "Заголовок 日本語", "строка", "Подзаголовок", "конец", "Курс Ω", "Юнит", "Секция")
	compositionTestSafe(t, text)
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, file.FileName()) {
			compositionTestContains(t, line, "строка", "Подзаголовок", "конец")
		}
	}
}
