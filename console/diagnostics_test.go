package console

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

type diagnosticsTestWriter struct {
	calls int
	cause error
	short bool
}

func (w *diagnosticsTestWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.short {
		return len(p) / 2, nil
	}
	return 0, w.cause
}

func diagnosticsTestContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Errorf("вывод не содержит %q: %q", value, output)
		}
	}
}

func diagnosticsTestRussian(t *testing.T, value string) {
	t.Helper()
	if !strings.ContainsFunc(value, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("нет русской причины: %q", value)
	}
}

func diagnosticsTestFindCode(err error, code string) *diagnostic.Error {
	if issue, ok := err.(*diagnostic.Error); ok {
		if issue == nil {
			return nil
		}
		if issue.Code == code {
			return issue
		}
	}
	if branch, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range branch.Unwrap() {
			if found := diagnosticsTestFindCode(child, code); found != nil {
				return found
			}
		}
	} else if branch, ok := err.(interface{ Unwrap() error }); ok {
		return diagnosticsTestFindCode(branch.Unwrap(), code)
	}
	return nil
}

func TestDiagnosticsStableCodes(t *testing.T) {
	for _, test := range []struct{ got, want string }{
		{CodeConfiguration, "console.configuration"}, {CodeInput, "console.input"},
		{CodeOutput, "console.output"}, {CodePreview, "console.preview"},
	} {
		if test.got != test.want {
			t.Errorf("код %q, требуется %q", test.got, test.want)
		}
	}
}

func TestDiagnosticsPrintErrorOrdinaryAndNil(t *testing.T) {
	var output bytes.Buffer
	if err := PrintError(&output, errors.New("внешняя причина Ω")); err != nil {
		t.Fatal(err)
	}
	diagnosticsTestContains(t, output.String(), "Не удалось выполнить действие", "внешняя причина Ω")
	output.Reset()
	if err := PrintError(&output, nil); err != nil || output.Len() != 0 {
		t.Fatalf("nil: error=%v, вывод=%q", err, output.String())
	}
	err := PrintError(nil, errors.New("причина"))
	if diagnosticsTestFindCode(err, CodeConfiguration) == nil {
		t.Fatalf("nil writer должен дать %s: %v", CodeConfiguration, err)
	}
}

func TestDiagnosticsPrintErrorOutputFailurePreservesBothCauses(t *testing.T) {
	originalCause := errors.New("ошибка исходных данных")
	original := &os.PathError{Op: "read", Path: "исходный.json", Err: originalCause}
	outputCause := errors.New("поток закрыт")
	for _, test := range []struct {
		name   string
		writer diagnosticsTestWriter
		cause  error
	}{
		{"write error", diagnosticsTestWriter{cause: outputCause}, outputCause},
		{"short write", diagnosticsTestWriter{short: true}, io.ErrShortWrite},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := test.writer
			err := PrintError(&writer, original)
			if !errors.Is(err, original) || !errors.Is(err, originalCause) || !errors.Is(err, test.cause) {
				t.Fatalf("потеря исходной/потоковой причины: %v", err)
			}
			var pathError *os.PathError
			if !errors.As(err, &pathError) || pathError != original {
				t.Errorf("errors.As не сохранил исходную ошибку: %v", err)
			}
			stream := diagnosticsTestFindCode(err, CodeOutput)
			if stream == nil || !errors.Is(stream, test.cause) {
				t.Errorf("сбой вывода не оформлен %s: %v", CodeOutput, err)
			}
			if writer.calls != 1 {
				t.Errorf("после сбоя выполнен повторный вывод: вызовов %d", writer.calls)
			}
		})
	}
}

func TestDiagnosticsIssuesKeepOrderAndFilterTypedNil(t *testing.T) {
	first := &diagnostic.Error{Code: "owner.first", Reason: "Первая причина"}
	last := &diagnostic.Error{Code: "owner.last", Reason: "Последняя причина"}
	var typedNil *diagnostic.Error
	for _, issues := range [][]*diagnostic.Error{nil, {}, {nil, typedNil}} {
		var output bytes.Buffer
		d := dialog{output: &output}
		if err := d.showIssues(issues); err != nil || output.Len() != 0 || joinedIssues(issues) != nil {
			t.Fatalf("пустые issues: error=%v, вывод=%q", err, output.String())
		}
	}
	issues := []*diagnostic.Error{typedNil, first, nil, last}
	var output bytes.Buffer
	d := dialog{output: &output}
	if err := d.showIssues(issues); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	diagnosticsTestContains(t, text, first.Code, last.Code)
	if strings.Index(text, first.Code) >= strings.Index(text, last.Code) {
		t.Errorf("изменён порядок issues: %q", text)
	}
	joined := joinedIssues(issues)
	if !errors.Is(joined, first) || !errors.Is(joined, last) {
		t.Fatal("joinedIssues потерял исходные указатели")
	}
	children := joined.(interface{ Unwrap() []error }).Unwrap()
	if len(children) != 2 || children[0] != first || children[1] != last {
		t.Fatalf("typed nil либо перестановка в дереве: %#v", children)
	}
}

func TestDiagnosticsIssuesFailureKeepsUnprintedIssues(t *testing.T) {
	first := &diagnostic.Error{Code: "owner.first", Reason: "Первая причина"}
	lastCause := errors.New("непоказанная техническая причина")
	last := &diagnostic.Error{Code: "owner.last", Reason: "Последняя причина", Cause: lastCause}
	fault := errors.New("вывод недоступен")
	writer := &diagnosticsTestWriter{cause: fault}
	d := dialog{output: writer}
	err := d.showIssues([]*diagnostic.Error{nil, first, nil, last})
	for _, cause := range []error{first, last, lastCause, fault} {
		if !errors.Is(err, cause) {
			t.Errorf("потеря причины %v: %v", cause, err)
		}
	}
	if diagnosticsTestFindCode(err, CodeOutput) == nil || writer.calls != 1 {
		t.Fatalf("сбой потока/остановка вывода: error=%v, вызовов %d", err, writer.calls)
	}
}

func TestDiagnosticsIssuesDoNotDeduplicateRepeatedIssue(t *testing.T) {
	issue := &diagnostic.Error{Code: "owner.repeated", Reason: "Повторная причина"}
	issues := []*diagnostic.Error{issue, issue}
	joined := joinedIssues(issues)
	children, ok := joined.(interface{ Unwrap() []error })
	if !ok || len(children.Unwrap()) != 2 || children.Unwrap()[0] != issue || children.Unwrap()[1] != issue {
		t.Fatalf("повторные исходные issues потеряны: %v", joined)
	}
	var output bytes.Buffer
	d := dialog{output: &output}
	if err := d.showIssues(issues); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), issue.Code) != 2 {
		t.Fatalf("повторная причина не выведена: %q", output.String())
	}
}
