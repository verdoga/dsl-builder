package preview

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/module"
)

func TestFieldsAcceptedOnlySelectedInCallerOrderAndFinalAddresses(t *testing.T) {
	before := fieldsTestFile(t, true, "efx7-p009", "efx7-p1044")
	after := fieldsTestFile(t, false, "efx7-p2001", "efx7-p6007", "efx7-p9009")
	accepted := []compare.FieldChange{
		{Field: compare.PageTitle, OldPage: 1, NewPage: 0, Before: fieldsTestString("Прежнее выбранное"), After: fieldsTestString("Итоговое выбранное")},
		{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1, Before: fieldsTestString(""), After: nil},
		{Field: compare.PageTitle, OldPage: -1, NewPage: 2, After: fieldsTestString("Новое без пары")},
	}
	original := fieldsTestJSON(t, accepted)
	fieldsTestUnchanged(t, before, func() {
		fieldsTestUnchanged(t, after, func() {
			text, err := AcceptedFields(before, after, accepted)
			if err != nil {
				t.Fatal(err)
			}
			fieldsTestContains(t, text, "page.title", "document.subtitle", "efx7-p1044", "efx7-p2001", "efx7-p9009",
				"Прежнее выбранное", "Итоговое выбранное", "Новое без пары", `""`, "null", "прежнее поле не установлено")
			first, second, third := strings.Index(text, "Итоговое выбранное"), strings.Index(text, "document.subtitle"), strings.Index(text, "Новое без пары")
			if !(first < second && second < third) {
				t.Errorf("порядок accepted потерян: %q", text)
			}
			for _, unwanted := range []string{"document.title", "Название снимка", "Подзаголовок снимка", "efx7-p009", "efx7-p6007", "ID страницы будет сохранён"} {
				if strings.Contains(text, unwanted) {
					t.Errorf("лишнее предложение/ложный перенос ID %q: %q", unwanted, text)
				}
			}
			fieldsTestSafe(t, text)
		})
	})
	if !bytes.Equal(original, fieldsTestJSON(t, accepted)) {
		t.Error("accepted или указатели значений изменены")
	}
}

func TestFieldsAcceptedEmptyIsLocalToFields(t *testing.T) {
	before := fieldsTestFile(t, false, "efx7-p009")
	after := fieldsTestFile(t, false, "efx7-p2001", "efx7-p6007")
	for _, accepted := range [][]compare.FieldChange{nil, {}} {
		text, err := AcceptedFields(before, after, accepted)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(text)
		if !strings.Contains(lower, "пол") || !(strings.Contains(lower, "нет") || strings.Contains(lower, "отсутств")) {
			t.Errorf("не обозначен пустой список полей: %q", text)
		}
		for _, forbidden := range []string{"изменений нет", "изменения отсутствуют", "архив не будет", "содержимое не измен", "манифест не измен"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("ложный вывод обо всей операции: %q", text)
			}
		}
		fieldsTestSafe(t, text)
	}
}

func TestFieldsPublicErrorsReturnNoPartialTextOrDump(t *testing.T) {
	file := fieldsTestFile(t, false, "efx7-p009", "efx7-p1044")
	valid := compare.FieldChange{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1,
		Before: fieldsTestString("SECRET-BEFORE"), After: fieldsTestString("SECRET-AFTER")}
	for _, missing := range []*module.File{nil, {}} {
		text, err := FieldProposal(missing, valid)
		fieldsTestError(t, text, err)
		text, err = AcceptedFields(missing, file, nil)
		fieldsTestError(t, text, err)
		text, err = AcceptedFields(file, missing, nil)
		fieldsTestError(t, text, err)
	}
	for _, tc := range []struct {
		name     string
		change   compare.FieldChange
		proposal bool
	}{
		{"unknown-field", compare.FieldChange{Field: "SECRET-SELECTOR", OldPage: -1, NewPage: -1}, true},
		{"document-old-index", compare.FieldChange{Field: compare.DocumentSubtitle, OldPage: 0, NewPage: -1}, true},
		{"document-new-index", compare.FieldChange{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: 0}, true},
		{"title-null-before", compare.FieldChange{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, After: valid.After}, true},
		{"title-null-after", compare.FieldChange{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: valid.Before}, true},
		{"page-old-negative", compare.FieldChange{Field: compare.PageTitle, OldPage: -2, NewPage: 0}, true},
		{"page-old-unavailable", compare.FieldChange{Field: compare.PageTitle, OldPage: 2, NewPage: 0}, true},
		{"page-new-negative", compare.FieldChange{Field: compare.PageTitle, OldPage: 0, NewPage: -1}, true},
		{"unknown-with-before", compare.FieldChange{Field: compare.PageTitle, OldPage: -1, NewPage: 0, Before: valid.Before}, true},
		{"accepted-new-unavailable", compare.FieldChange{Field: compare.PageTitle, OldPage: 0, NewPage: 2}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.proposal {
				text, err := FieldProposal(file, tc.change)
				fieldsTestError(t, text, err)
			}
			text, err := AcceptedFields(file, file, []compare.FieldChange{valid, tc.change})
			fieldsTestError(t, text, err)
		})
	}
}

func fieldsTestError(t *testing.T, text string, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("ожидалась ошибка представления")
	}
	if text != "" {
		t.Errorf("частичный текст при ошибке: %q", text)
	}
	russian := false
	for _, r := range err.Error() {
		russian = russian || unicode.In(r, unicode.Cyrillic)
	}
	if !russian {
		t.Errorf("нет русской причины: %v", err)
	}
	for _, secret := range []string{"SECRET-", "Название снимка", "Подзаголовок снимка", `{"`, "FieldChange{"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("дамп внешних данных в ошибке: %v", err)
		}
	}
	fieldsTestSafe(t, err.Error())
}

func TestFieldsValuesAndPublicBlocksPreserveEscapedFullValues(t *testing.T) {
	file := fieldsTestFile(t, true, "efx7-p009")
	before := "  Ёж Ω 新 \"обратная\\\"\n\r\t\x00\x1b[31m\x7f\u0085 " + strings.Repeat("Длинное значение", 40)
	after := before + " КОНЕЦ"
	change := compare.FieldChange{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: &before, After: &after}
	original := fieldsTestJSON(t, change)
	proposal, err := FieldProposal(file, change)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := AcceptedFields(file, file, []compare.FieldChange{change})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{fieldValues(change), proposal, accepted} {
		fieldsTestSafe(t, text)
		fieldsTestContains(t, text, "Ёж Ω 新", "Длинное значение", "КОНЕЦ")
		quoted := regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`).FindAllString(text, -1)
		for _, want := range []string{before, after} {
			found := false
			for _, token := range quoted {
				decoded, err := strconv.Unquote(token)
				found = found || err == nil && decoded == want
			}
			if !found {
				t.Errorf("нет полного обратимо экранированного значения %q в %q", want, text)
			}
		}
	}
	if !bytes.Equal(original, fieldsTestJSON(t, change)) {
		t.Error("значения изменены")
	}
}

func TestFieldsValuesDistinguishUnknownNullEmptyAndText(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  compare.FieldChange
		want    []string
		unknown bool
	}{
		{"unknown", compare.FieldChange{Field: compare.PageTitle, OldPage: -1, NewPage: 0}, []string{"прежнее поле не установлено", "null"}, true},
		{"known-null", compare.FieldChange{Field: compare.PageTitle, OldPage: 0, NewPage: 0, After: fieldsTestString("")}, []string{"null", `""`}, false},
		{"empty-to-text", compare.FieldChange{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1,
			Before: fieldsTestString(""), After: fieldsTestString("  Ёж Ω 新  ")}, []string{`""`, "  Ёж Ω 新  "}, false},
		{"text-to-null", compare.FieldChange{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1,
			Before: fieldsTestString("Регистр И пробелы ")}, []string{"Регистр И пробелы ", "null"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := fieldValues(tc.change)
			fieldsTestContains(t, text, "Было", "Станет")
			fieldsTestContains(t, text, tc.want...)
			separator := strings.Index(text, "Станет")
			if separator < 0 {
				t.Fatal("не выделено новое значение")
			}
			fieldsTestContains(t, text[:separator], tc.want[0])
			fieldsTestContains(t, text[separator:], tc.want[1])
			if strings.Contains(text, "прежнее поле не установлено") != tc.unknown {
				t.Errorf("перепутана известность поля: %q", text)
			}
			fieldsTestSafe(t, text)
		})
	}
}
