package console

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestDiagnosticsStreamAndPresentationWrappersPreserveCause(t *testing.T) {
	first, last := errors.New("первая причина"), errors.New("последняя причина")
	joined := errors.Join(first, last)
	for _, code := range []string{CodeInput, CodeOutput} {
		t.Run(code, func(t *testing.T) {
			issue := streamError(code, joined)
			if issue == nil || issue.Code != code || !errors.Is(issue, first) || !errors.Is(issue, last) {
				t.Fatalf("код или исходные причины потеряны: %#v", issue)
			}
			diagnosticsTestRussian(t, issue.Reason)
			if issue.Context != (diagnostic.Context{}) {
				t.Errorf("для стандартного потока выдумано место: %#v", issue.Context)
			}
		})
	}
	issue := presentationError(joined)
	if issue == nil || issue.Code != CodePreview || !errors.Is(issue, first) || !errors.Is(issue, last) {
		t.Fatalf("ошибка представления потеряла причины: %#v", issue)
	}
	diagnosticsTestRussian(t, issue.Reason)
	if isStreamFailure(issue) {
		t.Error("ошибка только представления признана сбоем потока")
	}
}

func TestDiagnosticsStreamClassificationFindsLateAndDeepBranches(t *testing.T) {
	ordinary := &diagnostic.Error{Code: "owner.invalid", Reason: "Ошибка данных"}
	input := streamError(CodeInput, errors.New("вход закрыт"))
	output := streamError(CodeOutput, errors.New("выход закрыт"))
	var deep error = output
	for i := 0; i < 64; i++ {
		deep = fmt.Errorf("обёртка %d: %w", i, errors.Join(ordinary, deep))
	}
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"ordinary", errors.New("ошибка данных"), false},
		{"owner diagnostic", ordinary, false},
		{"configuration", &diagnostic.Error{Code: CodeConfiguration, Reason: "Нет базы"}, false},
		{"presentation only", presentationError(errors.Join(ordinary, errors.New("неполный снимок"))), false},
		{"direct EOF", io.EOF, true},
		{"input", input, true},
		{"output", output, true},
		{"late input", errors.Join(ordinary, presentationError(errors.New("ошибка показа")), input), true},
		{"late output", errors.Join(ordinary, errors.Join(ordinary, output)), true},
		{"deep output", deep, true},
		{"preview wrapping stream", presentationError(errors.Join(ordinary, output)), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isStreamFailure(test.err); got != test.want {
				t.Errorf("isStreamFailure=%v, требуется %v", got, test.want)
			}
		})
	}
}

func TestDiagnosticsTreeStopsImmediatelyOnOutputFailure(t *testing.T) {
	fault := errors.New("неисправный выход")
	issue := &diagnostic.Error{
		Code: "owner.parent", Reason: "Родительская причина",
		Related: []diagnostic.Context{{Path: "связанный.json"}},
		Details: []diagnostic.Detail{{Label: "Подробность", Value: "значение"}},
		Cause:   errors.Join(errors.New("одна причина"), errors.New("другая причина")),
	}
	for _, test := range []struct {
		name string
		call func(io.Writer) error
	}{
		{"tree", func(w io.Writer) error { return writeErrorTree(w, issue) }},
		{"diagnostic", func(w io.Writer) error { return writeDiagnostic(w, issue) }},
		{"context", func(w io.Writer) error {
			return writeContext(w, diagnostic.Context{Path: "source.json", FileName: "l001.json"})
		}},
		{"details", func(w io.Writer) error {
			return writeDetails(w, []diagnostic.Detail{{Label: "Первая", Value: "1"}, {Label: "Вторая", Value: "2"}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := &diagnosticsTestWriter{cause: fault}
			err := test.call(writer)
			if !errors.Is(err, fault) || writer.calls != 1 {
				t.Errorf("ошибка потеряна либо вывод продолжен: error=%v, calls=%d", err, writer.calls)
			}
		})
	}
}
