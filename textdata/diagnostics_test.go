package textdata

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestDiagnosticsQAStableCodes(t *testing.T) {
	// Локальные const дополнительно проверяют доступность кодов как констант.
	const (
		unsupported = CodeUnsupportedType
		input       = CodeInput
		missing     = CodeMissingValue
		separator   = CodeExampleSeparator
		hint        = CodeInstructionHint
	)
	for _, tc := range []struct{ name, got, want string }{
		{"unsupported", unsupported, "textdata.unsupported_type"},
		{"input", input, "textdata.input"},
		{"missing", missing, "textdata.missing_value"},
		{"separator", separator, "textdata.example_separator"},
		{"hint", hint, "textdata.instruction_hint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("код = %q, нужен %q", tc.got, tc.want)
			}
		})
	}
}

func TestDiagnosticsQACoordinatesPreserveCallerContext(t *testing.T) {
	context := diagnostic.Context{
		Path:     `C:\Курс с пробелами\..\Вход\урок.parsed.json`,
		FileName: "001.json", LessonID: "Ab82", PageID: "Ab82-p003",
		Line: 901, Column: 77,
	}
	// До элемента находятся кириллица, emoji и combining mark: Start —
	// готовая колонка Unicode, а не байтовая позиция или UTF-16 offset.
	line := model.Line{Number: 42, Raw: "Я🙂е\u0301 x"}
	element := model.Element{Start: 6, End: 7, Raw: "x"}
	for _, tc := range []struct {
		name    string
		line    *model.Line
		element *model.Element
		context diagnostic.Context
		want    diagnostic.Context
	}{
		{"оба nil", nil, nil, context, context},
		{"только строка", &line, nil, context, diagnosticsQAAt(context, 42, 77)},
		{"только элемент", nil, &element, context, diagnosticsQAAt(context, 901, 6)},
		{"строка и элемент", &line, &element, context, diagnosticsQAAt(context, 42, 6)},
		{"неизвестное место", nil, nil, diagnostic.Context{}, diagnostic.Context{}},
		{"строка без внешней колонки", &line, nil, diagnostic.Context{}, diagnostic.Context{Line: 42}},
		{"элемент без внешней строки", nil, &element, diagnostic.Context{}, diagnostic.Context{Column: 6}},
		{"первая позиция", &model.Line{Number: 1}, &model.Element{Start: 1}, context, diagnosticsQAAt(context, 1, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const reason = "  Неизвестно семантическое значение «Ё🙂».\nУточните исходные данные.  "
			got := dataError(CodeMissingValue, reason, tc.line, tc.element, tc.context)
			diagnosticsQAAssertError(t, got, CodeMissingValue, reason, tc.want)
		})
	}
}

func TestDiagnosticsQAPassesEveryCodeAndCallerReason(t *testing.T) {
	for _, code := range []string{
		CodeUnsupportedType, CodeInput, CodeMissingValue,
		CodeExampleSeparator, CodeInstructionHint, "textdata.локальный_код",
	} {
		t.Run(code, func(t *testing.T) {
			const reason = "Невозможно однозначно выделить подсказку инструкции"
			context := diagnostic.Context{Path: "вход/урок.parsed.json"}
			got := dataError(code, reason, nil, nil, context)
			diagnosticsQAAssertError(t, got, code, reason, context)
		})
	}
}

func TestDiagnosticsQADoesNotCopyRawOrMutateInput(t *testing.T) {
	parent := 11
	value := "Семантическое значение"
	raw := "СЕКРЕТНЫЙ_RAW_БЛОК🙂\t_____{ОТВЕТ}"
	line := model.Line{
		Number: 42, LineType: model.LineTypeContent,
		NestingLevel: 2, ParentLine: &parent, LineEnding: model.LineEndingCRLF,
		Raw: raw,
		Elements: []model.Element{{
			ElementType: model.ElementTypeContent, Raw: raw,
			Value: &value, Start: 1, End: utf8.RuneCountInString(raw) + 1, ErrorIDs: []string{},
		}},
	}
	before, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	context := diagnostic.Context{Path: "технический.json", Line: 700, Column: 800}
	const reason = "Недопустимая локальная форма данных"
	got := dataError(CodeInput, reason, &line, &line.Elements[0], context)
	want := diagnosticsQAAt(context, 42, 1)
	diagnosticsQAAssertError(t, got, CodeInput, reason, want)
	after, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || parent != 11 || value != "Семантическое значение" {
		t.Fatal("dataError изменил переданную строку, элемент или их вложенные данные")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, rawJSON[1:len(rawJSON)-1]) || strings.Contains(got.Error(), raw) {
		t.Fatal("диагностика скопировала полный исходный Raw")
	}
	// Диагностика хранит сведения о месте на момент вызова.
	line.Number = 999
	line.Elements[0].Start = 999
	context.Path = "другой.json"
	diagnosticsQAAssertError(t, got, CodeInput, reason, want)
}

func TestDiagnosticsQAIndependentInclusionsAndRelated(t *testing.T) {
	line := model.Line{Number: 17}
	element := model.Element{Start: 5}
	firstContext := diagnostic.Context{Path: "первый.json", LessonID: "A", PageID: "A-p001"}
	secondContext := diagnostic.Context{Path: "второй.json", LessonID: "B", PageID: "B-p002"}
	const reason = "Неоднозначное разделение примера"
	first := dataError(CodeExampleSeparator, reason, &line, &element, firstContext)
	second := dataError(CodeExampleSeparator, reason, &line, &element, secondContext)
	diagnosticsQAAssertError(t, first, CodeExampleSeparator, reason, diagnosticsQAAt(firstContext, 17, 5))
	diagnosticsQAAssertError(t, second, CodeExampleSeparator, reason, diagnosticsQAAt(secondContext, 17, 5))
	if first == second {
		t.Fatal("разные вызовы вернули один изменяемый объект ошибки")
	}
	// Все неоднозначные места добавляет caller; dataError не выдумывает их.
	if len(first.Related) != 0 || len(second.Related) != 0 {
		t.Fatal("dataError добавил не переданные ему связанные места")
	}
	first.Related = append(first.Related, diagnosticsQAAt(firstContext, 17, 5), diagnosticsQAAt(firstContext, 19, 5))
	first.Related[0].Path = "место, изменённое вызывающим кодом"
	first.Context = diagnostic.Context{Path: "изменённый.json"}
	first.Code, first.Reason = "изменённый код", "Изменённая причина"
	diagnosticsQAAssertError(t, second, CodeExampleSeparator, reason, diagnosticsQAAt(secondContext, 17, 5))
	third := dataError(CodeExampleSeparator, reason, &line, &element, secondContext)
	diagnosticsQAAssertError(t, third, CodeExampleSeparator, reason, diagnosticsQAAt(secondContext, 17, 5))
	if len(second.Related) != 0 || len(third.Related) != 0 {
		t.Fatal("связанные места одного включения попали в другое")
	}
	if len(first.Related) != 2 || first.Related[1].Line != 19 {
		t.Fatal("новый вызов изменил связанные места предыдущей ошибки")
	}
}

func diagnosticsQAAt(context diagnostic.Context, line, column int) diagnostic.Context {
	context.Line, context.Column = line, column
	return context
}

func diagnosticsQAAssertError(t *testing.T, got *diagnostic.Error, code, reason string, context diagnostic.Context) {
	t.Helper()
	if got == nil {
		t.Fatal("dataError вернул nil")
	}
	if got.Code != code || got.Reason != reason || got.Context != context {
		t.Fatalf("получено code=%q reason=%q context=%+v; нужно code=%q reason=%q context=%+v",
			got.Code, got.Reason, got.Context, code, reason, context)
	}
}
