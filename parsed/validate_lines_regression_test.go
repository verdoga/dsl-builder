package parsed

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func validateLinesRegressionTestContext() diagnostic.Context {
	return diagnostic.Context{
		Path: `C:\Технический JSON\Урок.json`, FileName: "l038.json",
		LessonID: "Урок+38", PageID: "урок-p003", Line: 23, Column: 3,
	}
}

func validateLinesRegressionTestErrors(t *testing.T, got []*diagnostic.Error, wantError bool) {
	t.Helper()
	if (len(got) != 0) != wantError {
		t.Fatalf("ошибки = %v; нужна ошибка Contract: %v", got, wantError)
	}
	knownContext := validateLinesRegressionTestContext()
	for _, err := range got {
		if err == nil {
			t.Fatal("возвращена nil вместо диагностики")
		}
		if err.Code != CodeContract {
			t.Errorf("код = %q; нужен %q", err.Code, CodeContract)
		}
		if err.Context != knownContext {
			t.Errorf("контекст = %#v; нужен %#v", err.Context, knownContext)
		}
		if strings.TrimSpace(err.Reason) == "" {
			t.Error("отсутствует причина ошибки")
		}
	}
}

func validateLinesRegressionTestValue(t *testing.T, element model.Element, wantError bool) {
	t.Helper()
	knownContext := validateLinesRegressionTestContext()
	line := model.Line{
		Number: knownContext.Line, LineType: model.LineTypeContent,
		Raw: "  " + element.Raw, LineEnding: model.LineEndingLF,
		Elements: []model.Element{element},
	}
	before, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("value", func(t *testing.T) {
		var got []*diagnostic.Error
		if err := validateElementValue(element, knownContext); err != nil {
			got = append(got, err)
		}
		validateLinesRegressionTestErrors(t, got, wantError)
	})
	t.Run("element", func(t *testing.T) {
		validateLinesRegressionTestErrors(t, validateElement(element, line, knownContext), wantError)
	})
	t.Run("line", func(t *testing.T) {
		validateLinesRegressionTestErrors(t, validateLine(line, false, knownContext), wantError)
	})
	after, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("строка или значение изменены: до %s; после %s", before, after)
	}
}

func TestValidateLinesRegressionNullWithDiagnosticDoesNotReplaceStringValue(t *testing.T) {
	// Contract:82 и словарь :157–191 не расширяют допустимость null по errorIds.
	for _, tc := range []struct {
		kind       model.ElementType
		raw, value string
	}{
		{model.ElementTypeTitle, `\@Заголовок`, "@Заголовок"},
		{model.ElementTypeContent, "<b>_____{ответ}|текст</b>", "<b>_____{ответ}|текст</b>"},
		{model.ElementTypeIdentifier, "A:B+1", "A:B+1"},
		{model.ElementTypeName, " Раздел  А ", " Раздел  А "},
		{model.ElementTypeVersion, "9.future", "9.future"},
		{model.ElementTypeNumber, "0000007", "0000007"},
		{model.ElementTypeSource, "12.01 track A", "12.01 track A"},
		{model.ElementTypeResourcePath, `"..\Медиа,a"`, `..\Медиа,a`},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			for _, scenario := range []struct {
				name        string
				null, empty bool
				ids         []string
			}{
				{name: "string", ids: []string{}},
				{name: "string-with-diagnostic", ids: []string{"e1"}},
				{name: "null-with-diagnostic", null: true, ids: []string{"e1"}},
				{name: "empty-range-null-with-diagnostic", null: true, empty: true, ids: []string{"e1"}},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					value := tc.value
					element := model.Element{
						ElementType: tc.kind, Raw: tc.raw, Value: &value,
						Start: 3, End: 3 + utf8.RuneCountInString(tc.raw), ErrorIDs: scenario.ids,
					}
					if scenario.null {
						element.Value = nil
					}
					if scenario.empty {
						element.Raw = ""
						element.End = element.Start
					}
					validateLinesRegressionTestValue(t, element, scenario.null)
				})
			}
		})
	}
}

func TestValidateLinesRegressionDocumentedNullValuesRemainAccepted(t *testing.T) {
	for _, tc := range []struct {
		kind model.ElementType
		raw  string
		ids  []string
	}{
		{model.ElementTypeMediaType, "unsupported", []string{"e1"}},
		{model.ElementTypePlaceholder, "_____", []string{}},
		{model.ElementTypeUnparsed, "?", []string{"e1"}},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			element := model.Element{
				ElementType: tc.kind, Raw: tc.raw, Value: nil,
				Start: 3, End: 3 + utf8.RuneCountInString(tc.raw), ErrorIDs: tc.ids,
			}
			validateLinesRegressionTestValue(t, element, false)
		})
	}
}
