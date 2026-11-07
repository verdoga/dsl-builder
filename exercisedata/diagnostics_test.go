package exercisedata

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestExDiagStableCodes(t *testing.T) {
	cases := []struct {
		name, got, want string
	}{
		{"kind", CodeKind, "exercisedata.kind"},
		{"source", CodeSource, "exercisedata.source"},
		{"collection", CodeCollection, "exercisedata.collection"},
		{"binding", CodeBinding, "exercisedata.binding"},
		{"answer", CodeAnswer, "exercisedata.answer"},
		{"incomplete answers", CodeIncompleteAnswers, "exercisedata.incomplete_answers"},
		{"reference", CodeReference, "exercisedata.reference"},
		{"contradiction", CodeContradiction, "exercisedata.contradiction"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("код = %q, требуется %q", tc.got, tc.want)
			}
		})
	}
}

func TestExDiagSourceContextUsesPhysicalCoordinates(t *testing.T) {
	base := diagnostic.Context{
		Path:     `C:\Исходники\Путь с пробелами\вход. parsed.json`,
		FileName: "урок.json", LessonID: "Aa001", PageID: "Aa001.002",
		Line: 81, Column: 92,
	}
	cases := []struct {
		name    string
		line    model.Line
		element *model.Element
		row     int
		column  int
	}{
		{"element start", model.Line{Number: 731, Raw: "Кириллица перед элементом"}, &model.Element{Start: 13, End: 19}, 731, 13},
		{"nil element clears stale column", model.Line{Number: 1049}, nil, 1049, 0},
		{"unknown column stays unknown", model.Line{Number: 2}, &model.Element{}, 2, 0},
		{"unknown source stays unknown", model.Line{}, nil, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := base
			got := sourceContext(base, tc.line, tc.element)
			want := base
			want.Line, want.Column = tc.row, tc.column
			if got != want {
				t.Errorf("контекст = %+v, требуется %+v", got, want)
			}
			got.Path, got.Line = "изменено", 1
			if base != original {
				t.Errorf("изменён исходный контекст: %+v", base)
			}
		})
	}
}

func TestExDiagSourceContextDoesNotInventPathOrElement(t *testing.T) {
	value := "содержимое"
	line := model.Line{Number: 406, Elements: []model.Element{{
		ElementType: model.ElementTypeContent, Value: &value, Start: 7, End: 17,
	}}}
	got := sourceContext(diagnostic.Context{FileName: "урок.json"}, line, nil)
	want := diagnostic.Context{FileName: "урок.json", Line: 406}
	if got != want {
		t.Errorf("при nil element контекст = %+v, требуется %+v", got, want)
	}
	if line.Number != 406 || line.Elements[0].Start != 7 || *line.Elements[0].Value != value {
		t.Fatal("sourceContext изменил строку или элемент")
	}
}

func TestExDiagDataErrorPreservesStructuredFieldsAndOrder(t *testing.T) {
	context := diagnostic.Context{
		Path: "исходники/технический JSON.json", FileName: "урок.json",
		LessonID: "Aa001", PageID: "Aa001.003", Line: 91, Column: 17,
	}
	// Порядок экземпляров после fragments может отличаться от числового порядка строк.
	related := []diagnostic.Context{
		{Path: "фрагмент.json", LessonID: "Aa001", Line: 804, Column: 5},
		{Path: "другой.json", FileName: "запись.json", PageID: "Aa001.003", Line: 19, Column: 0},
		{Path: "фрагмент.json", Line: 804, Column: 23},
	}
	details := []diagnostic.Detail{
		{Label: "Ответ", Value: "  Ёж! \\---  "},
		{Label: "Ответ", Value: ""},
		{Label: "Подпись", Value: "<b>ёж</b>"},
	}
	wantRelated := append([]diagnostic.Context(nil), related...)
	wantDetails := append([]diagnostic.Detail(nil), details...)
	const reason = "Ответ ссылается на неоднозначную подпись"
	for attempt := 0; attempt < 5; attempt++ {
		got := dataError(CodeReference, reason, context, related, details)
		if got == nil {
			t.Fatal("dataError вернул nil")
		}
		if got.Code != CodeReference || got.Reason != reason || got.Context != context {
			t.Errorf("потеряны поля диагностики: %+v", got)
		}
		if !slices.Equal(got.Related, wantRelated) || !slices.Equal(got.Details, wantDetails) {
			t.Errorf("нарушены значения/порядок: Related=%+v Details=%+v", got.Related, got.Details)
		}
		if got.Cause != nil {
			t.Errorf("выдумана техническая причина: %v", got.Cause)
		}
		var structured *diagnostic.Error
		if !errors.As(error(got), &structured) || structured != got {
			t.Fatal("результат не доступен как существующий diagnostic.Error")
		}
		if !slices.Equal(related, wantRelated) || !slices.Equal(details, wantDetails) {
			t.Fatal("dataError изменил переданные срезы")
		}
	}
}

func TestExDiagDataErrorDoesNotInventUnknownFields(t *testing.T) {
	got := dataError(CodeSource, "Неизвестно семантическое значение", diagnostic.Context{}, nil, nil)
	if got == nil {
		t.Fatal("dataError вернул nil")
	}
	if got.Context != (diagnostic.Context{}) || len(got.Related) != 0 || len(got.Details) != 0 || got.Cause != nil {
		t.Fatalf("неизвестные сведения подменены выдуманными: %+v", got)
	}
}

func TestExDiagConstructorsDoNotPrint(t *testing.T) {
	capture, err := os.CreateTemp(t.TempDir(), "diagnostic-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	func() {
		stdout, stderr := os.Stdout, os.Stderr
		os.Stdout, os.Stderr = capture, capture
		defer func() { os.Stdout, os.Stderr = stdout, stderr }()
		context := sourceContext(diagnostic.Context{Path: "вход.json"}, model.Line{Number: 109}, nil)
		_ = dataError(CodeBinding, "Подсказка не связана с ответом", context, nil, nil)
	}()
	info, err := capture.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("конструкторы записали %d байт в stdout/stderr", info.Size())
	}
}
