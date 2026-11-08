package preview_test

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/preview"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestIntegrationCreateGoldenCompositionAndResources(t *testing.T) {
	wantPages := []int{1, 1, 2, 3, 3, 1, 1}
	wantNodes := []int{7, 7, 18, 3, 8, 15, 15}
	var files []*module.File
	var items []catalog.Item
	for index := range 7 {
		path := fmt.Sprintf("../golden/%d. lesson.json", index+1)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var value lesson.Lesson
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("l%03d.json", index+1)
		file := integrationTestFile(t, name, &value, false)
		files = append(files, file)
		items = append(items, catalog.Item{Entry: catalog.EntryFromLesson(value.Document, name, nil), RelativePath: path})
	}
	state, problems := catalog.Create("Курс золотых примеров", "", items)
	if len(problems) != 0 || state == nil {
		t.Fatalf("catalog.Create: %v", problems)
	}
	t.Cleanup(integrationTestPreserve(t, func() any { return state }))
	// Порядок Files и ложный Manifest не задают порядок отображения.
	value := &module.Module{Manifest: manifest.Manifest{Course: manifest.Course{Title: "НЕ ИСТОЧНИК"}}}
	for index := len(files) - 1; index >= 0; index-- {
		value.Files = append(value.Files, files[index])
	}
	t.Cleanup(integrationTestPreserve(t, func() any { return value }))
	text, err := preview.Composition(state, value, nil)
	text = integrationTestBlock(t, text, err)
	integrationTestContains(t, text, "Курс золотых примеров", "Без секций")
	integrationTestAbsent(t, text, "НЕ ИСТОЧНИК", "НОВЫЙ", "Юнит", "Версия", "версия")
	for index, item := range state.Items {
		row := integrationTestRow(t, text, item.Entry.FileName)
		integrationTestContains(t, row, item.Entry.Title, string(item.Entry.LineCount), *item.Entry.Order)
		for _, count := range []struct {
			label string
			want  int
		}{{"[Сс]траниц", wantPages[index]}, {"[Нн]од", wantNodes[index]}} {
			if !regexp.MustCompile(fmt.Sprintf(`%s[^0-9\n]*%d\b`, count.label, count.want)).MatchString(row) {
				t.Errorf("missing %s count %d in %s", count.label, count.want, row)
			}
		}
		if item.Entry.Subtitle != nil {
			integrationTestContains(t, row, *item.Entry.Subtitle)
		}
	}
	integrationTestOrder(t, text, "l001.json", "l002.json", "l003.json", "l004.json", "l005.json", "l006.json", "l007.json")
	for _, total := range []string{`уроков[^0-9\n]*7\b`, `страниц[^0-9\n]*12\b`, `нод[^0-9\n]*73\b`} {
		if !regexp.MustCompile("(?i)" + total).MatchString(text) {
			t.Errorf("missing total %s in:\n%s", total, text)
		}
	}
	integrationTestAbsent(t, text, "Conversation 1", "INSTRUCTION TEXT", "HINT TEXT", "ANSWER B", "ITEM A")
	// Готовый отчёт намеренно имеет порядок путей, отличный от сортировки.
	report := resourceusage.Report{Total: 4, Used: 1, Unused: 3,
		UsedPaths:   []string{`C:\Текущие\used.mp3`},
		UnusedPaths: []string{`C:\Текущие\Я.mp3`, "C:\\Текущие\\А\nимя.mp4", `C:\Внешние\字幕.vtt`}}
	t.Cleanup(integrationTestPreserve(t, func() any { return report }))
	resources := integrationTestBlock(t, preview.Resources(report), nil)
	unused := integrationTestBlock(t, preview.UnusedResources(report), nil)
	for _, block := range []string{resources, unused} {
		integrationTestContains(t, block, "4", "1", "3", "текущ")
		integrationTestAbsent(t, block, "ОШИБКА СБОРКИ")
	}
	integrationTestOrder(t, unused, "1.", "Я.mp3", "2.", `А\nимя.mp4`, "3.", "字幕.vtt")
	integrationTestContains(t, preview.UnusedResources(resourceusage.Report{}), "Неиспользованных файлов нет")
}

func TestIntegrationUnitElevenMetadataSections(t *testing.T) {
	archive, err := zip.OpenReader("../Unit example.ZIP")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var files []*module.File
	var items []catalog.Item
	for _, entry := range archive.File {
		if !strings.HasSuffix(entry.Name, ".json") {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		var source struct {
			Document struct {
				LineCount json.Number `json:"lineCount"`
				Metadata  struct {
					ID       string  `json:"documentId"`
					Title    string  `json:"title"`
					Subtitle *string `json:"subtitle"`
					Section  *string `json:"section"`
					Order    *string `json:"order"`
				} `json:"metadata"`
			} `json:"document"`
		}
		if err := json.Unmarshal(raw, &source); err != nil {
			t.Fatal(err)
		}
		// Снимок только метаданных: конвертация строк Unit не входит в preview.
		value := integrationTestLesson()
		metadata := source.Document.Metadata
		value.Document.ID, value.Document.Title = metadata.ID, metadata.Title
		value.Document.Subtitle, value.Document.LineCount = metadata.Subtitle, source.Document.LineCount
		value.Document.Version = "1"
		name := fmt.Sprintf("l%03d.json", len(files)+1)
		files = append(files, integrationTestFile(t, name, value, false))
		items = append(items, catalog.Item{Entry: catalog.EntryFromLesson(value.Document, name, metadata.Order),
			Section: metadata.Section, RelativePath: entry.Name})
	}
	if len(files) != 11 {
		t.Fatalf("expected all 11 Unit JSONs, got %d", len(files))
	}
	state, problems := catalog.Create("Empower A1", "Unit 1", items)
	if len(problems) != 0 {
		t.Fatalf("catalog.Create: %v", problems)
	}
	t.Cleanup(integrationTestPreserve(t, func() any { return state }))
	text, err := preview.Composition(state, &module.Module{Files: files}, nil)
	text = integrationTestBlock(t, text, err)
	integrationTestOrder(t, text, "Empower A1", "Unit 1", "Additional resources", "Studentbook", "Workbook")
	for _, item := range state.Items {
		row := integrationTestRow(t, text, item.Entry.FileName)
		integrationTestContains(t, row, item.Entry.Title, string(item.Entry.LineCount), *item.Entry.Order)
	}
	integrationTestAbsent(t, text, "НОВЫЙ", "Версия", "версия")
}

func TestIntegrationAddShowsUnfinishedPlacementAndReadyConflicts(t *testing.T) {
	ids := []string{"EFX7", "BA82", "BA83", "BA88", "BA89"}
	names := []string{"l001.json", "l002.json", "l007.json", "l012.json", "l013.json"}
	weights := []*string{integrationTestString("0001"), integrationTestString("01"), integrationTestString("1"), nil, integrationTestString("не-вес")}
	var files []*module.File
	var items []catalog.Item
	for index, id := range ids {
		value := integrationTestLesson()
		value.Document.ID, value.Document.Title = id, "Название "+id
		if index >= 3 {
			value.Document.Version = "1"
		}
		file := integrationTestFile(t, names[index], value, index < 3)
		files = append(files, file)
		items = append(items, catalog.Item{Entry: catalog.EntryFromLesson(value.Document, names[index], weights[index]),
			Context: file.Context()})
	}
	oldItems := catalog.Clone(&catalog.State{Items: items[:3]}).Items
	for index := range oldItems {
		oldItems[index].Entry.Order = integrationTestString(fmt.Sprint((index + 1) * 10))
	}
	before, problems := catalog.Create("Курс", "", oldItems)
	if before == nil || len(problems) != 0 {
		t.Fatalf("ready flat catalog: %v", problems)
	}
	t.Cleanup(integrationTestPreserve(t, func() any { return before }))
	oldText, err := preview.Composition(before, &module.Module{Files: files[:3]}, nil)
	oldText = integrationTestBlock(t, oldText, err)
	integrationTestContains(t, oldText, "Без секций")
	integrationTestOrder(t, oldText, names[:3]...)
	// Независимый кандидат намеренно ещё не готов к экспорту.
	state := catalog.Clone(before)
	state.Items = append(state.Items, catalog.Clone(&catalog.State{Items: items[3:]}).Items...)
	state.UnitTitle, state.Sectioned = integrationTestString("Юнит"), true
	state.Sections = []catalog.SectionOrder{{Title: "Пустая", Weight: integrationTestString("009")}, {Title: "Рабочая"}}
	for index := range state.Items {
		state.Items[index].Section = integrationTestString("Рабочая")
		state.Items[index].Entry.Order = weights[index]
	}
	state.Items[3].Section = nil
	state.Items[4].Entry.Title = "Название только из Entry"
	state.Items[4].Entry.Subtitle = integrationTestString("Подзаголовок Entry")
	state.Items[4].Entry.LineCount = "123456789012345678901234567890"
	newNames := []string{"l012.json", "l013.json"}
	conflicts := catalog.Conflicts(state)
	if len(conflicts) != 1 || len(conflicts[0].Items) != 3 {
		t.Fatalf("ready owner conflicts: %v", conflicts)
	}
	t.Cleanup(integrationTestPreserve(t, func() any { return state }))
	t.Cleanup(integrationTestPreserve(t, func() any { return conflicts }))
	t.Cleanup(integrationTestPreserve(t, func() any { return newNames }))
	text, err := preview.Composition(state, &module.Module{Files: files}, newNames)
	text = integrationTestBlock(t, text, err)
	integrationTestOrder(t, text, "Пустая", "Рабочая", "Не распределены")
	integrationTestContains(t, text, "009", "не задан", "не-вес")
	for index, name := range names {
		row := integrationTestRow(t, text, name)
		entry := state.Items[index].Entry
		integrationTestContains(t, row, entry.Title)
		// ID внутри обязательного title не является отдельным ID строки.
		withoutMetadata := strings.Replace(row, entry.Title, "", 1)
		if entry.Subtitle != nil {
			integrationTestContains(t, row, *entry.Subtitle)
			withoutMetadata = strings.Replace(withoutMetadata, *entry.Subtitle, "", 1)
		}
		if index >= 3 {
			integrationTestContains(t, withoutMetadata, "НОВЫЙ")
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(ids[index]) + `\b`).MatchString(withoutMetadata) {
				t.Errorf("missing separate new lesson ID %s in %s", ids[index], row)
			}
		} else {
			integrationTestAbsent(t, row, "НОВЫЙ")
			integrationTestAbsent(t, withoutMetadata, ids[index])
		}
	}
	entryRow := integrationTestRow(t, text, "l013.json")
	integrationTestContains(t, entryRow, "Название только из Entry", "Подзаголовок Entry", "123456789012345678901234567890")
	integrationTestAbsent(t, entryRow, "Название BA89")
	integrationTestAbsent(t, text, "Версия", "версия")
	conflictText := integrationTestBlock(t, preview.OrderConflicts(conflicts), nil)
	integrationTestContains(t, conflictText, "ОШИБКА СБОРКИ", "0001", "01", "Рабочая", "запрещена")
	integrationTestOrder(t, conflictText, "l001.json", "l002.json", "l007.json")
	for _, item := range conflicts[0].Items {
		integrationTestContains(t, conflictText, item.Entry.Title, "Прежний модуль.zip")
	}
	// Независимый блок конфликтов остаётся доступен при ошибке сопоставления.
	broken, err := preview.Composition(state, &module.Module{Files: files[:1]}, newNames)
	if broken != "" || err == nil || preview.OrderConflicts(conflicts) != conflictText {
		t.Fatal("composition error hid or changed ready conflict block")
	}
	changes := catalog.Changes(before, state)
	var added, moved []string
	for _, change := range changes {
		if change.Field == "entry" {
			if change.Before != nil || change.After == nil {
				t.Fatalf("expected added entry, got %+v", change)
			}
			added = append(added, change.FileName)
		}
		if change.Field == "section" && string(change.Before) == "null" && string(change.After) == `"Рабочая"` {
			moved = append(moved, change.FileName)
		}
	}
	if strings.Join(added, ",") != strings.Join(newNames, ",") || strings.Join(moved, ",") != strings.Join(names[:3], ",") {
		t.Fatalf("owner changes: added=%v, moved=%v; want only two new entries and three old moves", added, moved)
	}
	t.Cleanup(integrationTestPreserve(t, func() any { return changes }))
	manifestText, err := preview.ManifestChanges(changes)
	manifestText = integrationTestBlock(t, manifestText, err)
	integrationTestContains(t, manifestText, "course.unit", "sections", "entry", "l012.json", "l013.json", "BA88", "BA89", "lineCount", "createdAt", "version")
	var registry resourceusage.Registry
	registry.Register([]string{`C:\Новые исходники\не используется.mp3`, `C:\Новые исходники\встроен.mp4`})
	registry.MarkUsed([]string{`C:\Новые исходники\встроен.mp4`})
	report := registry.Snapshot()
	resources := integrationTestBlock(t, preview.Resources(report), nil)
	unused := integrationTestBlock(t, preview.UnusedResources(report), nil)
	integrationTestContains(t, resources, "текущ", "2", "1")
	integrationTestContains(t, unused, "не используется.mp3")
	integrationTestAbsent(t, resources+unused, "Прежний модуль.zip", "ОШИБКА СБОРКИ")
}
