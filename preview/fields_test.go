package preview

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
)

func fieldsTestString(value string) *string { return &value }

func fieldsTestPosition(t *testing.T, text string, position string) {
	t.Helper()
	if !regexp.MustCompile(`\b` + position + `\b`).MatchString(text) {
		t.Errorf("не показана позиция страницы с 1 (%s): %q", position, text)
	}
}

func fieldsTestFile(t *testing.T, loaded bool, ids ...string) *module.File {
	t.Helper()
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EFX7", Version: "7", Title: "Название снимка", Subtitle: fieldsTestString("Подзаголовок снимка"),
		LineCount: "0", CreatedAt: "2026-11-04T12:15:00Z", Pages: []lesson.Page{},
	}}
	for _, id := range ids {
		value.Document.Pages = append(value.Document.Pages, lesson.Page{
			ID: id, Title: fieldsTestString("Чужое название " + id), Nodes: []lesson.Node{},
		})
	}
	context := diagnostic.Context{Path: `C:\Модули\Исходный архив.zip`}
	if !loaded {
		file, problems := module.NewFile("l1044.json", value, context)
		if len(problems) != 0 {
			t.Fatalf("NewFile: %v", problems)
		}
		return file
	}
	raw := fieldsTestJSON(t, value)
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		t.Fatalf("ValidateJSON: %v", problems)
	}
	var decoded lesson.Lesson
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	file, problems := module.LoadedFile("l1044.json", &decoded, raw, context)
	if len(problems) != 0 {
		t.Fatalf("LoadedFile: %v", problems)
	}
	return file
}

func fieldsTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func fieldsTestContains(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			t.Errorf("нет %q в %q", fragment, text)
		}
	}
}

func fieldsTestSafe(t *testing.T, text string) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Error("результат не UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("сырой управляющий символ U+%04X", r)
		}
	}
	for _, forbidden := range []string{"Ваш выбор", "1. Да", "2. Нет", "Применить изменение?", "успешно записан"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("диалог/объявление записи: %q", forbidden)
		}
	}
}

func fieldsTestUnchanged(t *testing.T, file *module.File, check func()) {
	t.Helper()
	model, raw, context, name := fieldsTestJSON(t, file.Lesson()), file.OriginalBytes(), file.Context(), file.FileName()
	check()
	if !bytes.Equal(model, fieldsTestJSON(t, file.Lesson())) || !bytes.Equal(raw, file.OriginalBytes()) ||
		context != file.Context() || name != file.FileName() {
		t.Error("снимок File изменён")
	}
}

func TestFieldsProposalDocumentAndKnownPage(t *testing.T) {
	file := fieldsTestFile(t, true, "efx7-p009", "efx7-p1044")
	for _, tc := range []struct {
		name   string
		change compare.FieldChange
		want   []string
	}{
		{"title", compare.FieldChange{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1,
			Before: fieldsTestString(" Старое название Ё "), After: fieldsTestString(" 新ое название Ω ")},
			[]string{"document.title", " Старое название Ё ", " 新ое название Ω "}},
		{"subtitle-null", compare.FieldChange{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1,
			After: fieldsTestString("")}, []string{"document.subtitle", "null", `""`}},
		{"page", compare.FieldChange{Field: compare.PageTitle, OldPage: 1, NewPage: 4,
			Before: fieldsTestString("Старое поле"), After: fieldsTestString("Новое поле")},
			[]string{"page.title", "efx7-p1044", "5", "Старое поле", "Новое поле"}},
		{"page-known-null", compare.FieldChange{Field: compare.PageTitle, OldPage: 0, NewPage: 4,
			After: fieldsTestString("")}, []string{"page.title", "efx7-p009", "null", `""`}},
		{"page-known-empty", compare.FieldChange{Field: compare.PageTitle, OldPage: 0, NewPage: 4,
			Before: fieldsTestString(""), After: fieldsTestString("Непустое название")},
			[]string{"page.title", "efx7-p009", `""`, "Непустое название"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := fieldsTestJSON(t, tc.change)
			fieldsTestUnchanged(t, file, func() {
				text, err := FieldProposal(file, tc.change)
				if err != nil {
					t.Fatal(err)
				}
				fieldsTestContains(t, text, "l1044.json", "EFX7", "Было", "Станет")
				fieldsTestContains(t, text, tc.want...)
				refusal := strings.Index(strings.ToLower(text), "отказ")
				if refusal < 0 {
					t.Error("не показан результат отказа")
				} else {
					want := "null"
					if tc.change.Before != nil {
						want = *tc.change.Before
						if want == "" {
							want = `""`
						}
					}
					fieldsTestContains(t, text[refusal:], want)
				}
				if tc.change.Field == compare.PageTitle {
					fieldsTestPosition(t, text, "5")
					if strings.Contains(text, "прежнее поле не установлено") {
						t.Error("известное поле названо неизвестным")
					}
				}
				fieldsTestSafe(t, text)
			})
			if !bytes.Equal(original, fieldsTestJSON(t, tc.change)) {
				t.Error("указатели значений изменены")
			}
		})
	}
}

func TestFieldsProposalUnmatchedPageHasNoPositionalFallback(t *testing.T) {
	file := fieldsTestFile(t, false, "efx7-p009", "efx7-p1044")
	for _, after := range []*string{nil, fieldsTestString(""), fieldsTestString("Чужое название efx7-p009")} {
		change := compare.FieldChange{Field: compare.PageTitle, OldPage: -1, NewPage: 0, After: after}
		text, err := FieldProposal(file, change)
		if err != nil {
			t.Fatal(err)
		}
		fieldsTestContains(t, text, "прежнее поле не установлено", "title=null", "1", "Станет")
		fieldsTestPosition(t, text, "1")
		if after == nil {
			fieldsTestContains(t, text, "null")
		}
		if after != nil {
			fieldsTestContains(t, text, *after)
		}
		if strings.Contains(text, "efx7-p1044") || strings.Contains(text, "ID страницы будет сохранён") {
			t.Errorf("подобрана чужая страница: %q", text)
		}
		if after == nil && strings.Contains(text, "efx7-p009") {
			t.Errorf("позиционный fallback: %q", text)
		}
		fieldsTestSafe(t, text)
	}
}
