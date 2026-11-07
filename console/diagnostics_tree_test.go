package console

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// diagnosticsTestWrapped задаёт обычную стандартную границу Unwrap(error).
type diagnosticsTestWrapped struct {
	text  string
	cause error
}

func (e diagnosticsTestWrapped) Error() string { return e.text }
func (e diagnosticsTestWrapped) Unwrap() error { return e.cause }

func TestDiagnosticsErrorTreeKeepsEachNodeAndJoinedBranch(t *testing.T) {
	left := &diagnostic.Error{Code: "owner.left", Reason: "Левая причина", Context: diagnostic.Context{Path: "левый.json"}}
	right := &diagnostic.Error{Code: "owner.right", Reason: "Правая причина", Context: diagnostic.Context{Path: "правый.json"}}
	parent := &diagnostic.Error{
		Code: "owner.parent", Reason: "Самостоятельная родительская причина",
		Context: diagnostic.Context{Path: "родитель.zip", FileName: "l090.json"},
		Cause:   errors.Join(diagnosticsTestWrapped{"Обычная обёртка", left}, diagnosticsTestWrapped{"Вторая обёртка", errors.Join(errors.New("обычная нижняя причина"), right)}),
	}
	var output bytes.Buffer
	if err := writeErrorTree(&output, parent); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, issue := range []*diagnostic.Error{parent, left, right} {
		diagnosticsTestContains(t, text, issue.Code, issue.Reason, issue.Context.Path)
		if strings.Count(text, issue.Code) != 1 {
			t.Errorf("узел %s потерян либо продублирован текстом Join/обёртки: %q", issue.Code, text)
		}
	}
	diagnosticsTestContains(t, text, "l090.json", "Обычная обёртка", "обычная нижняя причина")
	if strings.Index(text, parent.Reason) > strings.Index(text, left.Reason) {
		t.Errorf("Cause заменил описание родителя: %q", text)
	}
}

func TestDiagnosticsDeepJoinedTreeAndDirectDiagnosticCause(t *testing.T) {
	leaf := &diagnostic.Error{Code: "owner.deep", Reason: "Глубокая причина", Context: diagnostic.Context{PageID: "Тест.P1001"}}
	var tree error = leaf
	for i := 0; i < 64; i++ {
		tree = diagnosticsTestWrapped{fmt.Sprintf("Обёртка %d", i), errors.Join(errors.New(fmt.Sprintf("Ветка %d", i)), tree)}
	}
	root := &diagnostic.Error{Code: "owner.root", Reason: "Корневая причина", Cause: tree}
	var output bytes.Buffer
	if err := writeDiagnostic(&output, root); err != nil {
		t.Fatal(err)
	}
	diagnosticsTestContains(t, output.String(), root.Reason, leaf.Code, leaf.Reason, leaf.Context.PageID, "Ветка 0", "Ветка 63")
}

func TestDiagnosticsSingleWrappedErrorRetainsOuterReasonAndInnerContext(t *testing.T) {
	leaf := &diagnostic.Error{Code: "owner.single", Reason: "Внутренняя причина", Context: diagnostic.Context{Path: "single.json", Line: 91}}
	var output bytes.Buffer
	if err := writeErrorTree(&output, fmt.Errorf("Внешняя причина: %w", leaf)); err != nil {
		t.Fatal(err)
	}
	diagnosticsTestContains(t, output.String(), "Внешняя причина", leaf.Code, leaf.Reason, "single.json", "91")
	output.Reset()
	if err := writeErrorTree(&output, errors.New("обычная причина")); err != nil {
		t.Fatal(err)
	}
	diagnosticsTestContains(t, output.String(), "Не удалось выполнить действие", "обычная причина")
}

func TestDiagnosticsNilTreeAndDiagnosticProduceNoOutput(t *testing.T) {
	var output bytes.Buffer
	var issue *diagnostic.Error
	for _, call := range []func() error{
		func() error { return writeErrorTree(&output, nil) },
		func() error { return writeErrorTree(&output, issue) },
		func() error { return writeDiagnostic(&output, issue) },
	} {
		if err := call(); err != nil || output.Len() != 0 {
			t.Fatalf("nil должен быть безопасен: error=%v, вывод=%q", err, output.String())
		}
	}
}

func TestDiagnosticsContextEveryKnownFieldAndNoInventedUnknown(t *testing.T) {
	context := diagnostic.Context{
		Path:     "  C:\\Учебные материалы\\" + strings.Repeat("Каталог Ω ", 1000) + "source.json  ",
		FileName: "l123456789012345678901234567890.json", LessonID: "УрокΩ" + strings.Repeat("1234567890", 100),
		PageID: "УрокΩ.P12345678901234567890", Line: 12347, Column: 98761,
	}
	var output bytes.Buffer
	if err := writeContext(&output, context); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	diagnosticsTestContains(t, text, context.Path, context.FileName, context.LessonID, context.PageID, "12347", "98761")
	for _, field := range []diagnostic.Context{
		{Path: "source.json"}, {FileName: "l001.json"}, {LessonID: "LESSON001"},
		{PageID: "LESSON001.P001"}, {Line: 12347}, {Column: 98761},
	} {
		var isolated bytes.Buffer
		if err := writeContext(&isolated, field); err != nil {
			t.Fatal(err)
		}
		// Изолированное ASCII-значение не может само удовлетворить проверку подписи.
		diagnosticsTestRussian(t, isolated.String())
	}
	output.Reset()
	if err := writeContext(&output, diagnostic.Context{}); err != nil || output.Len() != 0 {
		t.Fatalf("неизвестные сведения выдуманы: error=%v, вывод=%q", err, output.String())
	}
	if err := writeContext(&output, diagnostic.Context{Line: 8}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "0") || strings.Contains(output.String(), "document.filePath") {
		t.Errorf("выдуманная колонка/путь: %q", output.String())
	}
}

func TestDiagnosticsRelatedShowsEveryNumberedParticipant(t *testing.T) {
	issue := &diagnostic.Error{Code: "owner.related", Reason: "Все участники конфликта"}
	for i := 1; i <= 12; i++ {
		issue.Related = append(issue.Related, diagnostic.Context{Path: fmt.Sprintf("участник-%02d.json", i), FileName: fmt.Sprintf("l%03d.json", i)})
	}
	var output bytes.Buffer
	if err := writeDiagnostic(&output, issue); err != nil {
		t.Fatal(err)
	}
	text, previous := output.String(), 0
	for i, context := range issue.Related {
		at := strings.Index(text, context.Path)
		if at < previous {
			t.Fatalf("участник отсутствует либо нарушен порядок: %s: %q", context.Path, text)
		}
		diagnosticsTestContains(t, text, context.Path, context.FileName)
		// Номер допускается в заголовке перед контекстом или на строке пути.
		segment := text[previous:at]
		number := regexp.MustCompile(`(^|[^0-9])` + strconv.Itoa(i+1) + `([^0-9]|$)`)
		if !number.MatchString(segment) {
			t.Errorf("нет номера участника %d перед %s: %q", i+1, context.Path, segment)
		}
		previous = at + len(context.Path)
	}
}

func TestDiagnosticsDetailsRetainDuplicatesEmptyAndRawWeights(t *testing.T) {
	longWeight := "000" + strings.Repeat("9876543210", 2000)
	details := []diagnostic.Detail{
		{Label: "Повторная подробность", Value: "первое значение"},
		{Label: "Повторная подробность", Value: ""},
		{Label: "Повторная подробность", Value: longWeight},
		{Label: "Последняя подробность", Value: "  Ω Кириллица  "},
	}
	var output bytes.Buffer
	if err := writeDetails(&output, details); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Count(text, "Повторная подробность") != 3 {
		t.Errorf("пустая или повторная подробность потеряна: %q", text)
	}
	diagnosticsTestContains(t, text, longWeight, "  Ω Кириллица  ")
	first, last := strings.Index(text, "первое значение"), strings.Index(text, longWeight)
	if first < 0 || last < first || strings.Index(text, "Последняя подробность") < last {
		t.Error("нарушен исходный порядок подробностей")
	}
}

func TestDiagnosticsEscapesAllExternalTextControls(t *testing.T) {
	value := "  Кириллица Ω\t\r\n\x00\x1b[31m\x7f\u0085 конец  "
	issue := &diagnostic.Error{
		Code: "owner.control" + value, Reason: "Причина" + value,
		Context: diagnostic.Context{Path: value, FileName: value, LessonID: value, PageID: value},
		Related: []diagnostic.Context{{Path: value}},
		Details: []diagnostic.Detail{{Label: value, Value: value}}, Cause: errors.New("Техническая" + value),
	}
	var output bytes.Buffer
	if err := PrintError(&output, issue); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("в вывод попал управляющий символ U+%04X", r)
		}
	}
	if strings.Contains(text, "Ω\n") || strings.Contains(text, "\n конец") {
		t.Error("внешний перевод строки управляет разметкой вывода")
	}
	if strings.Count(text, "  Кириллица Ω") < 10 || strings.Count(text, " конец  ") < 10 {
		t.Errorf("Unicode/пробелы/одно из внешних полей потеряны: %q", text)
	}
}
