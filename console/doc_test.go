package console_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/console"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestConsoleDocMainCallerOwnsStreamsAndVersion(t *testing.T) {
	beforeRun := errors.New("не удалось определить версию сборки")
	stderr := &docTestOutput{}
	if err := console.PrintError(stderr, beforeRun); err != nil {
		t.Fatal(err)
	}
	docTestRequire(t, stderr.String(), "Не удалось выполнить действие", beforeRun.Error())
	if stderr.closed {
		t.Fatal("PrintError closed stderr")
	}
	output := docTestRun(t, t.TempDir(), []docTestStep{docTestAnswer("да"), docTestAnswer("0"), docTestAnswer("4")})
	docTestRequire(t, output, "1. Создать", "2. Обновить", "3. Добавить", "4. Выход")
	if strings.Contains(output, "\x1b") {
		t.Fatal("terminal control sequences in sequential output")
	}
	if _, err := stderr.Write([]byte("caller still owns stderr")); err != nil {
		t.Fatal(err)
	}
}

func TestConsoleDocConfigurationIsExplicit(t *testing.T) {
	for _, tc := range []struct {
		name, base, version string
		input               io.Reader
		output              io.Writer
	}{
		{name: "nil input", base: t.TempDir(), version: "v39", output: &bytes.Buffer{}},
		{name: "nil output", base: t.TempDir(), version: "v39", input: strings.NewReader("4\n")},
		{name: "no base", version: "v39", input: strings.NewReader("4\n"), output: &bytes.Buffer{}},
		{name: "no version", base: t.TempDir(), input: strings.NewReader("4\n"), output: &bytes.Buffer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := console.Run(tc.input, tc.output, tc.base, tc.version, nil)
			var issue *diagnostic.Error
			if !errors.As(err, &issue) || issue.Code != console.CodeConfiguration {
				t.Fatalf("configuration error: %v", err)
			}
		})
	}
}

func TestConsoleDocPartialMenuAnswerIsCancelled(t *testing.T) {
	output := &docTestOutput{}
	input := &docTestInput{t: t, output: &output.Buffer, steps: []docTestStep{{answer: "1"}}}
	if err := console.Run(input, output, t.TempDir(), "v39", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "Ввести значение") {
		t.Fatal("unterminated main answer started operation")
	}
	if input.closed || output.closed {
		t.Fatal("caller streams closed")
	}
}

func TestConsoleDocPrintErrorPreservesCallerCauseOnBrokenOutput(t *testing.T) {
	original := errors.New("ошибка main до Run")
	failure := errors.New("stderr недоступен")
	writer := &docTestFaultWriter{cause: failure}
	err := console.PrintError(writer, original)
	if !errors.Is(err, original) || !errors.Is(err, failure) {
		t.Fatalf("lost causes: %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("retried broken stderr %d times", writer.calls)
	}
	if err := console.PrintError(nil, original); err == nil {
		t.Fatal("nil stderr accepted")
	}
}

type docTestFaultWriter struct {
	calls int
	cause error
}

func (w *docTestFaultWriter) Write(p []byte) (int, error) { w.calls++; return 0, w.cause }
