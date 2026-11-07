package preview_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/preview"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func docTestJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func docTestFile(t *testing.T, loaded bool) *module.File {
	t.Helper()
	body := lesson.RoleBody
	title := "  Кириллица Ω\r\n\t\x1b[31m\u0085 конец  "
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EFX7", Version: "900719925474099312345678901", Title: title,
		Subtitle: &title, LineCount: "123456789012345678901234567890", CreatedAt: "2026-11-04T12:00:00Z",
		Pages: []lesson.Page{{ID: "efx7-p001", Title: &title, Nodes: []lesson.Node{
			{ID: "efx7-n001", Type: lesson.NodeQuestion, Nodes: []lesson.Node{}, Data: []lesson.Data{
				{Type: lesson.DataText, Role: &body, Value: "DOC_PRIVATE_BODY", Answers: []string{"DOC_PRIVATE_ANSWER"}, Hint: "DOC_PRIVATE_HINT"},
			}},
			{ID: "efx7-n002", Type: lesson.NodeMedia, Nodes: []lesson.Node{}, Data: []lesson.Data{
				{Type: lesson.DataMedia, Role: &body, Value: lesson.EmbeddedFile{FileName: "DOC_PRIVATE_MEDIA.mp3", MIMEType: "audio/mpeg", Content: base64.StdEncoding.EncodeToString([]byte("DOC_PRIVATE_BYTES"))}, Answers: []string{}, Hint: ""},
			}},
		}}},
	}}
	if !loaded {
		value.Document.Title = "Итог " + title
		value.Document.Version = "900719925474099312345678902"
		value.Document.Pages[0].ID = "efx7-p002"
		value.Document.Pages[0].Nodes[0].ID = "efx7-n003-v900719925474099312345678902"
		value.Document.Pages[0].Nodes[1].ID = "efx7-n004-v900719925474099312345678902"
		value.Document.Pages = append(value.Document.Pages, lesson.Page{ID: "efx7-p003", Nodes: []lesson.Node{}})
	}
	context := diagnostic.Context{Path: "C:\\Несуществующий источник\\урок.json"}
	file, problems := module.NewFile("l001.json", value, context)
	if loaded {
		raw := []byte(docTestJSON(t, value))
		raw = append([]byte(`{"docOriginalOnly":"DOC_PRIVATE_ORIGINAL",`), raw[1:]...)
		if issues := lesson.ValidateJSON(raw, context); len(issues) != 0 {
			t.Fatalf("невалидный raw fixture: %v", issues)
		}
		var decoded lesson.Lesson
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		file, problems = module.LoadedFile("l001.json", &decoded, raw, context)
	}
	if file == nil || len(problems) != 0 {
		t.Fatalf("невалидный настоящий File: %v", problems)
	}
	return file
}

func docTestText(t *testing.T, text string) {
	t.Helper()
	if text == "" || !utf8.ValidString(text) {
		t.Fatalf("ожидался непустой UTF-8 текст: %q", text)
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("сырой управляющий символ U+%04X", r)
		}
	}
	for _, secret := range []string{"DOC_PRIVATE_BODY", "DOC_PRIVATE_ANSWER", "DOC_PRIVATE_HINT", "DOC_PRIVATE_MEDIA", "DOC_PRIVATE_ORIGINAL", base64.StdEncoding.EncodeToString([]byte("DOC_PRIVATE_BYTES"))} {
		if strings.Contains(text, secret) {
			t.Errorf("содержимое закрытой части снимка попало в текст: %s", secret)
		}
	}
	for _, phrase := range []string{"ваш выбор", "1. да", "2. нет", "успешно сохран", "запись разрешена", "изменения применены", "записать изменения?", "применить изменение?", "содержательных изменений не обнаружено", "модуль готов к записи"} {
		if strings.Contains(strings.ToLower(text), phrase) {
			t.Errorf("представление содержит меню, решение или успех: %q", phrase)
		}
	}
}

func TestDocPublicBlocksPreserveInputsAndHidePayloads(t *testing.T) {
	before, after := docTestFile(t, true), docTestFile(t, false)
	document := after.Lesson().Document
	order, section := "000999999999999999999999999999999", "Секция\r\n\x1b[2J конец"
	state := &catalog.State{CourseTitle: document.Title, UnitTitle: document.Subtitle, Sectioned: true,
		Sections: []catalog.SectionOrder{{Title: section, Weight: &order}},
		Items: []catalog.Item{{Entry: manifest.Entry{ID: document.ID, FileName: after.FileName(), Order: &order, Title: document.Title,
			Subtitle: document.Subtitle, LineCount: document.LineCount, Version: document.Version, CreatedAt: document.CreatedAt}, Section: &section, Context: before.Context()}},
	}
	value := &module.Module{Files: []*module.File{after}}
	newNames := []string{after.FileName()}
	oldTitle := before.Lesson().Document.Title
	change := compare.FieldChange{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: &oldTitle, After: &document.Title}
	accepted := []compare.FieldChange{change}
	plan := &compare.Plan{ReplaceLesson: true, Fields: accepted, Reasons: []compare.Reason{{Code: compare.PageCountChanged, Description: "Причина\r\n\x1b[2J конец", OldPointer: "/document/pages", NewPointer: "/document/pages"}}}
	report := resourceusage.Report{Total: 1, Unused: 1, UnusedPaths: []string{"C:\\Несуществующие ресурсы\\Файл\r\n\x1b[2J конец.mp3"}}
	changes := []catalog.Change{{FileName: after.FileName(), Field: "title", Before: json.RawMessage(docTestJSON(t, document.Title)), After: json.RawMessage(docTestJSON(t, section))}}
	peer := state.Items[0]
	peer.Entry.FileName = "l002.json"
	peer.Context.Path = "C:\\Другой источник\\Файл\r\n\x1b[2J конец.json"
	conflicts := catalog.Conflicts(&catalog.State{Sections: state.Sections, Items: []catalog.Item{state.Items[0], peer}})
	snapshot := func() string {
		return docTestJSON(t, struct {
			State         *catalog.State
			Module        *module.Module
			Before, After *lesson.Lesson
			Raw           []byte
			Plan          *compare.Plan
			Accepted      []compare.FieldChange
			Report        resourceusage.Report
			Changes       []catalog.Change
			Conflicts     []catalog.Conflict
			NewNames      []string
		}{state, value, before.Lesson(), after.Lesson(), before.OriginalBytes(), plan, accepted, report, changes, conflicts, newNames})
	}
	want := snapshot()
	blocks := []struct {
		name     string
		call     func() (string, error)
		contains []string
	}{
		{"composition", func() (string, error) { return preview.Composition(state, value, newNames) }, []string{"Кириллица Ω", "конец", order, string(document.LineCount)}},
		{"heading", func() (string, error) {
			return preview.UpdateHeading("C:\\Не открыт\\Архив\r\n\x1b[2J.zip", before, after)
		}, []string{"Не открыт", "Архив", "Кириллица Ω", "конец", "900719925474099312345678901", string(document.Version)}},
		{"proposal", func() (string, error) { return preview.FieldProposal(before, change) }, []string{"Кириллица Ω", "Итог", "конец"}},
		{"accepted", func() (string, error) { return preview.AcceptedFields(before, after, accepted) }, []string{"Кириллица Ω", "Итог", "конец"}},
		{"replacements", func() (string, error) { return preview.Replacements(before, after, plan) }, []string{"Причина", "конец", "efx7-n003-v900719925474099312345678902"}},
		{"manifest", func() (string, error) { return preview.ManifestChanges(changes) }, []string{"Кириллица Ω", "Секция", "конец"}},
		{"conflicts", func() (string, error) { return preview.OrderConflicts(conflicts), nil }, []string{"Секция", "конец", order}},
		{"resources", func() (string, error) { return preview.Resources(report), nil }, nil},
		{"unused", func() (string, error) { return preview.UnusedResources(report), nil }, []string{"Несуществующие ресурсы", "конец.mp3"}},
	}
	docTestNoConsoleIO(t, func() {
		for _, block := range blocks {
			t.Run(block.name, func(t *testing.T) {
				text, err := block.call()
				if err != nil {
					t.Fatal(err)
				}
				docTestText(t, text)
				for _, fragment := range block.contains {
					if !strings.Contains(text, fragment) {
						t.Errorf("внешнее значение потеряно: %q", fragment)
					}
				}
				if snapshot() != want || value.Files[0] != after {
					t.Fatal("переданные данные изменились")
				}
			})
		}
	})
}

func docTestNoConsoleIO(t *testing.T, call func()) {
	t.Helper()
	directory := t.TempDir()
	input, err := os.Create(filepath.Join(directory, "input"))
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.Create(filepath.Join(directory, "output"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	if _, err := input.WriteString("987654321\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = input, output, output
	defer func() { os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr }()
	call()
	position, err := input.Seek(0, 1)
	if err != nil || position != 0 {
		t.Errorf("preview прочитал консольный ввод: %d, %v", position, err)
	}
	info, err := output.Stat()
	if err != nil || info.Size() != 0 {
		t.Error("preview записал в stdout/stderr")
	}
}

func TestDocMissingInputsAndAddressesReturnOnlyRussianError(t *testing.T) {
	file := docTestFile(t, true)
	field := compare.FieldChange{Field: compare.PageTitle, OldPage: 7, NewPage: 7}
	cases := []struct {
		name string
		call func() (string, error)
	}{
		{"nil state", func() (string, error) { return preview.Composition(nil, &module.Module{}, nil) }},
		{"nil module", func() (string, error) { return preview.Composition(&catalog.State{}, nil, nil) }},
		{"nil old heading", func() (string, error) { return preview.UpdateHeading("архив", nil, file) }},
		{"nil new heading", func() (string, error) { return preview.UpdateHeading("архив", file, nil) }},
		{"nil proposal", func() (string, error) { return preview.FieldProposal(nil, field) }},
		{"nil accepted old", func() (string, error) { return preview.AcceptedFields(nil, file, nil) }},
		{"nil accepted new", func() (string, error) { return preview.AcceptedFields(file, nil, nil) }},
		{"nil plan", func() (string, error) { return preview.Replacements(file, file, nil) }},
		{"nil replacement old", func() (string, error) { return preview.Replacements(nil, file, &compare.Plan{}) }},
		{"nil replacement new", func() (string, error) { return preview.Replacements(file, nil, &compare.Plan{}) }},
		{"missing snapshot", func() (string, error) { return preview.UpdateHeading("архив", new(module.File), file) }},
		{"missing page", func() (string, error) { return preview.AcceptedFields(file, file, []compare.FieldChange{field}) }},
		{"missing node", func() (string, error) {
			return preview.Replacements(file, file, &compare.Plan{Pages: []compare.PageChange{{Index: 0, Nodes: []compare.NodeChange{{Path: []int{99}, Fields: []compare.NodeField{compare.NodeDataChanged}}}}}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := tc.call()
			if text != "" || err == nil {
				t.Fatalf("ожидались пустой текст и error: %q, %v", text, err)
			}
			docTestText(t, err.Error())
			if strings.IndexFunc(err.Error(), func(r rune) bool { return r >= 'А' && r <= 'я' || r == 'ё' || r == 'Ё' }) < 0 {
				t.Errorf("ошибка не содержит русской причины: %v", err)
			}
			if _, ok := err.(*diagnostic.Error); ok {
				t.Errorf("preview вернул предметную диагностику: %T", err)
			}
		})
	}
	if !bytes.Contains(file.OriginalBytes(), []byte("DOC_PRIVATE_ORIGINAL")) {
		t.Fatal("fixture не содержит raw-only контрольную строку")
	}
}
