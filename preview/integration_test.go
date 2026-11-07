package preview_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/preview"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func integrationTestString(value string) *string { return &value }

func integrationTestLesson(pages ...lesson.Page) *lesson.Lesson {
	return &lesson.Lesson{FormatVersion: "1.0", Document: lesson.Document{
		ID: "EFX7", Title: "Сохранённое название", Version: "100", LineCount: "900719925474099312345",
		CreatedAt: "2026-11-03T08:34:59Z", Pages: append([]lesson.Page{}, pages...),
	}}
}

func integrationTestNode(id, content string, children ...lesson.Node) lesson.Node {
	role := lesson.RoleBody
	return lesson.Node{ID: id, Type: lesson.NodeText, Nodes: append([]lesson.Node{}, children...),
		Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: content,
			Answers: []string{"ANSWER_SECRET"}, Hint: "HINT_SECRET"}},
	}
}

func integrationTestFile(t *testing.T, name string, value *lesson.Lesson, loaded bool) *module.File {
	t.Helper()
	context := diagnostic.Context{Path: `C:\Исходники\` + name}
	var file *module.File
	var problems []*diagnostic.Error
	if loaded {
		raw := integrationTestJSON(t, value)
		context.Path = `C:\Архивы\Прежний модуль.zip`
		file, problems = module.LoadedFile(name, value, raw, context)
	} else {
		file, problems = module.NewFile(name, value, context)
	}
	if len(problems) != 0 || file == nil {
		t.Fatalf("real module file fixture: %v", problems)
	}
	t.Cleanup(integrationTestPreserve(t, func() any {
		return struct {
			Name     string
			Lesson   *lesson.Lesson
			Original []byte
			Context  diagnostic.Context
		}{file.FileName(), file.Lesson(), file.OriginalBytes(), file.Context()}
	}))
	return file
}

func integrationTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationTestPreserve(t *testing.T, observe func() any) func() {
	t.Helper()
	before := integrationTestJSON(t, observe())
	return func() {
		t.Helper()
		if !bytes.Equal(before, integrationTestJSON(t, observe())) {
			t.Error("preview changed caller-owned input")
		}
	}
}

func integrationTestContains(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(text, value) {
			t.Errorf("missing %q in:\n%s", value, text)
		}
	}
}

func integrationTestAbsent(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(text, value) {
			t.Errorf("unexpected %q in:\n%s", value, text)
		}
	}
}

func integrationTestOrder(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		index := strings.Index(text, value)
		if index < 0 {
			t.Fatalf("missing/out-of-order %q in remaining text:\n%s", value, text)
		}
		text = text[index+len(value):]
	}
}

func integrationTestBlock(t *testing.T, text string, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(text) || text == "" {
		t.Fatal("expected nonempty UTF-8 text")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("raw control character U+%04X", r)
		}
	}
	integrationTestAbsent(t, text, "NODE_DATA_SECRET", "ANSWER_SECRET", "HINT_SECRET", "AA==",
		`"formatVersion"`, "Ваш выбор", "1. Да", "2. Нет", "успешно сохран", "Содержательных изменений не обнаружено")
	return text
}

func integrationTestRow(t *testing.T, text, name string) string {
	t.Helper()
	var rows []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, name) {
			rows = append(rows, line)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("expected one compact row for %s, got %v", name, rows)
	}
	return rows[0]
}

func integrationTestNoPairs(t *testing.T, text string, oldIDs, newIDs []string) {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		for _, oldID := range oldIDs {
			for _, newID := range newIDs {
				if strings.Contains(line, oldID) && strings.Contains(line, newID) {
					t.Errorf("invented old/new pairing: %s", line)
				}
			}
		}
	}
}

func TestIntegrationPresentationBoundaries(t *testing.T) {
	file := integrationTestFile(t, "l001.json", integrationTestLesson(), true)
	item := catalog.Item{Entry: catalog.EntryFromLesson(file.Lesson().Document, file.FileName(), nil)}
	state := &catalog.State{CourseTitle: "Курс \"Ёж\"\nСтрока\r\x1b[31m", Items: []catalog.Item{item}}
	value := &module.Module{Files: []*module.File{file}}
	t.Cleanup(integrationTestPreserve(t, func() any { return state }))
	dir := t.TempDir()
	archive := dir + "/Отсутствующий архив.zip"
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = writer, writer
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	var captured []byte
	done := make(chan struct{})
	go func() { captured, _ = io.ReadAll(reader); close(done) }()
	text, callErr := preview.Composition(state, value, nil)
	composition := integrationTestBlock(t, text, callErr)
	integrationTestContains(t, composition, "Ёж", `\n`, `\r`, "Строка", "[31m")
	text, callErr = preview.UpdateHeading(archive, file, file)
	integrationTestBlock(t, text, callErr)
	text, callErr = preview.AcceptedFields(file, file, nil)
	integrationTestBlock(t, text, callErr)
	text, callErr = preview.Replacements(file, file, &compare.Plan{})
	integrationTestBlock(t, text, callErr)
	text, callErr = preview.ManifestChanges(nil)
	integrationTestBlock(t, text, callErr)
	integrationTestBlock(t, preview.OrderConflicts(nil), nil)
	integrationTestBlock(t, preview.UnusedResources(resourceusage.Report{Total: 1, Unused: 1, UnusedPaths: []string{archive}}), nil)
	writer.Close()
	<-done
	reader.Close()
	if len(captured) != 0 {
		t.Errorf("preview wrote to stdout/stderr: %q", captured)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("preview performed filesystem writes: entries=%v, err=%v", entries, err)
	}
}

func TestIntegrationPresentationErrorsAreAtomic(t *testing.T) {
	file := integrationTestFile(t, "l001.json", integrationTestLesson(), false)
	state := &catalog.State{Items: []catalog.Item{{Entry: catalog.EntryFromLesson(file.Lesson().Document, file.FileName(), nil)}}}
	for _, test := range []struct {
		name string
		call func() (string, error)
	}{
		{"missing state", func() (string, error) { return preview.Composition(nil, &module.Module{}, nil) }},
		{"missing file", func() (string, error) { return preview.Composition(state, &module.Module{}, nil) }},
		{"ambiguous file", func() (string, error) {
			return preview.Composition(state, &module.Module{Files: []*module.File{file, file}}, nil)
		}},
		{"missing snapshot", func() (string, error) { return preview.UpdateHeading("archive", file, &module.File{}) }},
		{"missing plan", func() (string, error) { return preview.Replacements(file, file, nil) }},
		{"bad node address", func() (string, error) {
			return preview.Replacements(file, file, &compare.Plan{Pages: []compare.PageChange{{Index: 0, Nodes: []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}}}})
		}},
		{"bad accepted selector", func() (string, error) {
			return preview.AcceptedFields(file, file, []compare.FieldChange{{Field: compare.PageTitle, OldPage: -1, NewPage: 9}})
		}},
		{"invalid manifest JSON", func() (string, error) {
			return preview.ManifestChanges([]catalog.Change{{Field: "entry", After: json.RawMessage(`NODE_DATA_SECRET{`)}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			text, err := test.call()
			if text != "" || err == nil {
				t.Fatalf("expected empty text and error, got %q, %v", text, err)
			}
			var subjectError *diagnostic.Error
			if errors.As(err, &subjectError) || !strings.ContainsFunc(err.Error(), func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
				t.Errorf("expected Russian presentation error, got %T: %v", err, err)
			}
			integrationTestAbsent(t, err.Error(), "NODE_DATA_SECRET", "ANSWER_SECRET", "HINT_SECRET")
		})
	}
}
