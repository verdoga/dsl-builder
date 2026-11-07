package catalog_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"io"
	"os"
	"sort"
	"testing"
)

const integrationTestDate = "2026-11-07T12:00:00Z"

func integrationTestString(s string) *string { return &s }
func integrationTestRequire(t *testing.T, condition bool, message any) {
	t.Helper()
	if !condition {
		t.Fatal(message)
	}
}
func integrationTestJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	integrationTestRequire(t, err == nil, err)
	return b
}
func integrationTestGuard(t *testing.T, v any) {
	t.Helper()
	before := integrationTestJSON(t, v)
	t.Cleanup(func() {
		if !bytes.Equal(before, integrationTestJSON(t, v)) {
			t.Error("исходный снимок изменён")
		}
	})
}
func integrationTestState(t *testing.T, s *catalog.State, errs []*diagnostic.Error) *catalog.State {
	t.Helper()
	if s == nil || len(errs) != 0 {
		t.Fatalf("снимок=%v, ошибки=%v", s, errs)
	}
	integrationTestGuard(t, s)
	return s
}
func integrationTestExport(t *testing.T, s *catalog.State) *manifest.Manifest {
	t.Helper()
	m, errs := catalog.Export(s, integrationTestDate, diagnostic.Context{Path: "module.zip", FileName: "manifest.json"})
	if m == nil || len(errs) != 0 {
		t.Fatalf("Export: %v", errs)
	}
	errs = manifest.Validate(*m, diagnostic.Context{})
	integrationTestRequire(t, len(errs) == 0, errs)
	var tree any
	dec := json.NewDecoder(bytes.NewReader(integrationTestJSON(t, m)))
	dec.UseNumber()
	integrationTestRequire(t, dec.Decode(&tree) == nil, "декодирование экспорта")
	errs = manifest.ValidateShape(tree, diagnostic.Context{})
	integrationTestRequire(t, len(errs) == 0, errs)
	integrationTestGuard(t, m)
	return m
}
func integrationTestBlocked(t *testing.T, s *catalog.State, errs []*diagnostic.Error, code string) {
	t.Helper()
	integrationTestRequire(t, s != nil, "исправляемый кандидат отсутствует")
	found := false
	for _, e := range errs {
		if e.Code == code {
			found = true
		}
	}
	if !found {
		t.Fatalf("нет %s: %v", code, errs)
	}
	if m, problems := catalog.Export(s, integrationTestDate, diagnostic.Context{}); m != nil || len(problems) == 0 {
		t.Fatal("экспорт заблокированного кандидата")
	}
	integrationTestGuard(t, s)
}

// integrationTestSource читает только метаданные; преобразование нод вне catalog.
func integrationTestSource(t *testing.T, b []byte, name string, n int) (catalog.Item, lesson.Document) {
	t.Helper()
	var src struct {
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
	integrationTestRequire(t, json.Unmarshal(b, &src) == nil, "декодирование fixture")
	md := src.Document.Metadata
	doc := lesson.Document{ID: md.ID, Title: md.Title, Subtitle: md.Subtitle, LineCount: src.Document.LineCount, Version: "1", CreatedAt: integrationTestDate, Pages: []lesson.Page{}}
	item := catalog.Item{Entry: catalog.EntryFromLesson(doc, fmt.Sprintf("l%03d.json", n), md.Order), Section: md.Section, RelativePath: name, Context: diagnostic.Context{Path: name, LessonID: md.ID}}
	return item, doc
}
func TestIntegrationGoldenLifecycle(t *testing.T) {
	var items []catalog.Item
	var documents []lesson.Document
	for n := 1; n <= 7; n++ {
		name := fmt.Sprintf("../golden/%d. parsed.json", n)
		b, err := os.ReadFile(name)
		integrationTestRequire(t, err == nil, err)
		item, doc := integrationTestSource(t, b, name, n)
		b, err = os.ReadFile(fmt.Sprintf("../golden/%d. lesson.json", n))
		integrationTestRequire(t, err == nil, err)
		var root struct {
			Document lesson.Document `json:"document"`
		}
		integrationTestRequire(t, json.Unmarshal(b, &root) == nil, "декодирование fixture")
		gold := root.Document
		integrationTestRequire(t, !(gold.ID != doc.ID || gold.Title != doc.Title || gold.LineCount != doc.LineCount || !bytes.Equal(integrationTestJSON(t, gold.Subtitle), integrationTestJSON(t, doc.Subtitle))), "golden metadata расходятся")
		item.Entry = catalog.EntryFromLesson(gold, item.Entry.FileName, nil)
		items, documents = append(items, item), append(documents, gold)
	}
	integrationTestGuard(t, items)
	integrationTestGuard(t, documents)
	s, errs := catalog.Create("Курс", "", items)
	s = integrationTestState(t, s, errs)
	for i, item := range s.Items {
		want := items[i].Entry
		want.Order = integrationTestString(fmt.Sprint(i + 1))
		if !bytes.Equal(integrationTestJSON(t, want), integrationTestJSON(t, item.Entry)) {
			t.Fatalf("синтез/метаданные %d", i)
		}
	}
	s, errs = catalog.SetCourseTitle(s, "Новый курс")
	s = integrationTestState(t, s, errs)
	s, errs = catalog.SetUnitTitle(s, "Юнит")
	s = integrationTestState(t, s, errs)
	m := integrationTestExport(t, s)
	integrationTestRequire(t, !(m.Course.Unit == nil || m.Course.Lessons != nil || m.Course.Sections != nil), "форма unit")
	loaded, errs := catalog.Import(*m, diagnostic.Context{Path: "module.zip"})
	loaded = integrationTestState(t, loaded, errs)
	if changes := catalog.Changes(s, loaded); len(changes) != 0 {
		t.Fatal(changes)
	}
	newItems := []catalog.Item{{Entry: manifest.Entry{ID: "NEW1", FileName: "l008.json", Title: "Новый 1", LineCount: "0", Version: "1", CreatedAt: integrationTestDate}}, {Entry: manifest.Entry{ID: "NEW2", FileName: "l009.json", Title: "Новый 2", LineCount: "0", Version: "1", CreatedAt: integrationTestDate}}}
	integrationTestGuard(t, newItems)
	s, errs = catalog.Add(loaded, newItems)
	integrationTestBlocked(t, s, errs, catalog.CodeMissingWeight)
	for i := range loaded.Items {
		integrationTestRequire(t, bytes.Equal(integrationTestJSON(t, loaded.Items[i].Entry), integrationTestJSON(t, s.Items[i].Entry)), "Add изменил прежнюю запись")
	}
	for _, item := range s.Items[7:] {
		integrationTestRequire(t, !(item.Entry.Order != nil), "Add синтезировал вес")
	}
	s, errs = catalog.CreateSection(s, "Первая")
	integrationTestBlocked(t, s, errs, catalog.CodeUnassigned)
	s, errs = catalog.CreateSection(s, "Вторая")
	integrationTestBlocked(t, s, errs, catalog.CodeUnassigned)
	for i := 1; i <= 9; i++ {
		section := "Первая"
		if i%2 == 0 {
			section = "Вторая"
		}
		s, errs = catalog.MoveLesson(s, fmt.Sprintf("l%03d.json", i), &section)
		integrationTestState(t, s, nil)
	}
	for i := 1; i <= 9; i++ {
		s, errs = catalog.SetLessonWeight(s, fmt.Sprintf("l%03d.json", i), "001")
		integrationTestState(t, s, nil)
	}
	conflicts := catalog.Conflicts(s)
	if len(conflicts) != 2 || len(conflicts[0].Items)+len(conflicts[1].Items) != 9 {
		t.Fatalf("участники конфликтов: %v", conflicts)
	}
	integrationTestBlocked(t, s, errs, catalog.CodeWeightConflict)
	for i := 1; i <= 9; i++ {
		s, errs = catalog.SetLessonWeight(s, fmt.Sprintf("l%03d.json", i), fmt.Sprintf("00%d", i))
		integrationTestState(t, s, nil)
	}
	s = integrationTestState(t, s, errs)
	weights := []catalog.SectionOrder{{Title: "Первая", Weight: integrationTestString("20")}, {Title: "Вторая", Weight: integrationTestString("010")}}
	integrationTestGuard(t, weights)
	s, errs = catalog.SetSectionWeights(s, weights)
	s = integrationTestState(t, s, errs)
	if got := integrationTestExport(t, s).Course.Unit.Sections; len(got) != 2 || got[0].Title != "Вторая" {
		t.Fatal("ручной порядок секций")
	}
	failed, errs := catalog.UseFlatPlacement(s)
	integrationTestRequire(t, !(failed != nil && len(errs) == 0), "flat снял назначения без решения")
	for i := 1; i <= 9; i++ {
		s, errs = catalog.MoveLesson(s, fmt.Sprintf("l%03d.json", i), nil)
		integrationTestState(t, s, nil)
	}
	s, errs = catalog.UseFlatPlacement(s)
	s = integrationTestState(t, s, errs)
	s, errs = catalog.SetUnitTitle(s, "")
	s = integrationTestState(t, s, errs)
	flat := integrationTestExport(t, s)
	integrationTestRequire(t, !(flat.Course.Unit != nil || flat.Course.Lessons == nil || len(*flat.Course.Lessons) != 9 || flat.Course.Sections == nil || len(*flat.Course.Sections) != 0), "форма flat")
	refused, errs := catalog.SyncLesson(s, "l001.json", documents[0])
	refused = integrationTestState(t, refused, errs)
	integrationTestRequire(t, len(catalog.Changes(s, refused)) == 0, "отказ/no-op")
	proposals, errs := catalog.ProposePlacement(s, "l001.json", integrationTestString("Первая"), integrationTestString("01"))
	if len(errs) != 0 || len(proposals) != 2 || proposals[0].Field != "section" || proposals[1].Field != "order" {
		t.Fatalf("независимые предложения: %v %v", proposals, errs)
	}
	integrationTestRequire(t, len(catalog.Changes(s, refused)) == 0, "предложение принято автоматически")
	accepted := documents[0]
	accepted.Title, accepted.Subtitle, accepted.LineCount, accepted.Version, accepted.CreatedAt = "Принято", integrationTestString(""), "9007199254740993", "2", integrationTestDate
	integrationTestGuard(t, &accepted)
	updated, errs := catalog.SyncLesson(s, "l001.json", accepted)
	updated = integrationTestState(t, updated, errs)
	changes := catalog.Changes(s, updated)
	wantFields := []string{"title", "subtitle", "lineCount", "createdAt", "version"}
	integrationTestRequire(t, len(changes) == len(wantFields), changes)
	for i, change := range changes {
		if change.FileName != "l001.json" || change.Field != wantFields[i] {
			t.Fatalf("порядок Changes: %v", changes)
		}
	}
	out := integrationTestExport(t, updated)
	want := catalog.EntryFromLesson(accepted, "l001.json", integrationTestString("001"))
	integrationTestRequire(t, bytes.Equal(integrationTestJSON(t, (*out.Course.Lessons)[0]), integrationTestJSON(t, want)), "SyncLesson не перенёс окончательный Document")
	for i := 1; i < 9; i++ {
		integrationTestRequire(t, bytes.Equal(integrationTestJSON(t, (*flat.Course.Lessons)[i]), integrationTestJSON(t, (*out.Course.Lessons)[i])), "SyncLesson изменил другой урок")
	}
}
func TestIntegrationUnitMetadataAndImportOrder(t *testing.T) {
	z, err := zip.OpenReader("../Unit example.ZIP")
	integrationTestRequire(t, err == nil, err)
	defer z.Close()
	files := append([]*zip.File(nil), z.File...)
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	var items []catalog.Item
	for _, f := range files {
		if len(f.Name) < 5 || f.Name[len(f.Name)-5:] != ".json" {
			continue
		}
		r, err := f.Open()
		integrationTestRequire(t, err == nil, err)
		b, err := io.ReadAll(r)
		closeErr := r.Close()
		integrationTestRequire(t, err == nil && closeErr == nil, "чтение Unit ZIP")
		item, _ := integrationTestSource(t, b, f.Name, len(items)+1)
		items = append(items, item)
	}
	integrationTestRequire(t, len(items) == 11, "не 11 уроков")
	// Допустимая форма того же исходного веса проверяет сохранение ведущих нулей.
	items[0].Entry.Order = integrationTestString("0010")
	integrationTestGuard(t, items)
	s, errs := catalog.Create("Empower A1", "Unit 1", items)
	s = integrationTestState(t, s, errs)
	m := integrationTestExport(t, s)
	integrationTestRequire(t, !(len(m.Course.Unit.Sections) != 3 || m.Course.Unit.Sections[0].Title != "Additional resources" || *m.Course.Unit.Sections[0].Lessons[0].Order != "0010"), "группы/исходный вес")
	var expected []catalog.Item
	for _, section := range m.Course.Unit.Sections {
		for _, entry := range section.Lessons {
			for _, item := range items {
				if entry.FileName == item.Entry.FileName {
					item.Entry, item.Section = entry, integrationTestString(section.Title)
					expected = append(expected, item)
				}
			}
		}
	}
	sort.Slice(expected, func(i, j int) bool { return expected[i].Entry.FileName < expected[j].Entry.FileName })
	integrationTestRequire(t, bytes.Equal(integrationTestJSON(t, items), integrationTestJSON(t, expected)), "метаданные/имена/группы изменены")
	for _, name := range []string{"Manifest unit example.json", "Manifest flat example.json"} {
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile("../" + name)
			integrationTestRequire(t, err == nil, err)
			var original manifest.Manifest
			integrationTestRequire(t, json.Unmarshal(b, &original) == nil, "декодирование fixture")
			integrationTestGuard(t, &original)
			loaded, errs := catalog.Import(original, diagnostic.Context{Path: "original.zip"})
			if original.Course.Unit == nil {
				integrationTestBlocked(t, loaded, errs, catalog.CodeMissingWeight)
				loaded, errs = catalog.SetLessonWeight(loaded, "l001.json", "0001")
				integrationTestState(t, loaded, errs)
				integrationTestExport(t, loaded)
				return
			}
			loaded = integrationTestState(t, loaded, errs)
			integrationTestRequire(t, !(loaded.Sections[0].Title != "Studentbook" || loaded.Sections[2].Title != "Additional resources"), "Import пересортировал секции")
			exported := integrationTestExport(t, loaded)
			exported.CreatedAt = original.CreatedAt
			integrationTestRequire(t, bytes.Equal(integrationTestJSON(t, &original), integrationTestJSON(t, exported)), "Import/Export изменил массивы")
			exported.CreatedAt = integrationTestDate
			*exported.Course.Unit.Sections[0].Lessons[0].Order = "999"
			exported.Course.Unit.Sections[0].Title = "Изменено снаружи"
			// Guard экспорта ожидает только исходное значение; возвращаем его после проверки независимости.
			integrationTestRequire(t, !(*loaded.Items[0].Entry.Order != "10" || loaded.Sections[0].Title != "Studentbook"), "экспорт разделяет память со State")
			*exported.Course.Unit.Sections[0].Lessons[0].Order = "10"
			exported.Course.Unit.Sections[0].Title = "Studentbook"
		})
	}
}
