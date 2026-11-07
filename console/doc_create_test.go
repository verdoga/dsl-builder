package console_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/console"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestConsoleDocCreateShowsFullReviewBeforeSingleConsent(t *testing.T) {
	base := t.TempDir()
	source := docTestSource(t, base, "Исходники λ/вложено/урок.json", nil, nil)
	directory := filepath.Dir(filepath.Dir(source))
	target := filepath.Join(base, "Модуль λ.zip")
	expected := docTestCreate(t, base, directory, "Курс Язык λ")
	docTestIssues(t, expected.SetDestination(expected.Snapshot().Token, base, "Модуль λ"))
	fullReview := docTestFullReview(t, expected)
	steps := docTestCreateInput(docTestQuote(directory), "Курс Язык λ")
	steps = append(steps, docTestDestination(docTestQuote(base), "Модуль λ")...)
	steps = append(steps, docTestReviewStep(nil), docTestStep{answer: "1\r\n", before: func(screen string) {
		fullReview(screen)
		docTestAssertAbsent(t, target)
	}}, docTestStep{label: `^Выход$`, before: docTestBefore(t, target)})
	output := docTestRun(t, base, steps)
	value := docTestModule(t, target)
	if len(value.Files) != 1 || value.Manifest.Course.Title != "Курс Язык λ" || value.Manifest.Course.Unit != nil {
		t.Fatalf("wrong module: %+v", value.Manifest)
	}
	lesson := value.Files[0].Lesson()
	if lesson.Document.ID != "DOC39" || lesson.Document.Version.String() != "1" || lesson.Document.LineCount.String() != "5" {
		t.Fatalf("wrong converted document: %+v", lesson.Document)
	}
	if value.Manifest.Course.Lessons == nil || (*value.Manifest.Course.Lessons)[0].Order == nil || *(*value.Manifest.Course.Lessons)[0].Order != "1" {
		t.Fatal("missing synthesized weight")
	}
	if !strings.Contains(output, target) {
		t.Fatal("missing result path")
	}
}

func TestConsoleDocRefusalAndIncompleteConsentNeverWrite(t *testing.T) {
	for _, tc := range []struct{ name, answer string }{{"refuse", "2\n"}, {"EOF", ""}, {"partial yes", "1"}} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			source := docTestSource(t, base, "input/a.json", nil, nil)
			target := filepath.Join(base, "запрет.zip")
			steps := docTestCreateInput(filepath.Dir(source), "Курс")
			steps = append(steps, docTestDestination(base, "запрет")...)
			steps = append(steps, docTestReviewStep(nil), docTestStep{answer: tc.answer, before: docTestBefore(t, target, "l001.json", "Урок исходный")})
			if tc.answer == "2\n" {
				steps = append(steps, docTestMenu(`^Выход$`))
			}
			docTestRun(t, base, steps)
			docTestAssertAbsent(t, target)
			entries, err := os.ReadDir(base)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "input" {
				t.Fatalf("write artifacts without consent: %v", entries)
			}
		})
	}
}

func TestConsoleDocPlanAndDestinationChangesNeedNewReview(t *testing.T) {
	base := t.TempDir()
	source := docTestSource(t, base, "input/a.json", nil, nil)
	first := filepath.Join(base, "первый.zip")
	last := filepath.Join(base, "второй.zip")
	expected := docTestCreate(t, base, filepath.Dir(source), "Курс новый")
	docTestIssues(t, expected.SetDestination(expected.Snapshot().Token, base, "второй"))
	fullReview := docTestFullReview(t, expected)
	steps := docTestCreateInput(filepath.Dir(source), "Курс старый")
	steps = append(steps, docTestDestination(base, "первый")...)
	steps = append(steps, docTestReviewStep(nil), docTestStep{answer: "2\n", before: docTestBefore(t, first, "Курс старый")})
	steps = append(steps, docTestMenu(`(?i)(измен|назван).*курс`))
	steps = append(steps, docTestValue("Курс новый")...)
	steps = append(steps, docTestStep{answer: "1\n", before: docTestBefore(t, "Курс старый", "Курс новый")})
	steps = append(steps, docTestDestination(base, "второй")...)
	steps = append(steps, docTestReviewStep(docTestBefore(t, "Курс новый")), docTestStep{answer: "1\n", before: func(screen string) {
		fullReview(screen)
		docTestAssertAbsent(t, first)
		docTestAssertAbsent(t, last)
	}}, docTestMenu(`^Выход$`))
	docTestRun(t, base, steps)
	docTestAssertAbsent(t, first)
	if got := docTestModule(t, last).Manifest.Course.Title; got != "Курс новый" {
		t.Fatalf("stale plan written: %q", got)
	}
}

func TestConsoleDocInputFailureCancelsAnalysedOperation(t *testing.T) {
	base := t.TempDir()
	source := docTestSource(t, base, "input/a.json", nil, nil)
	failure := errors.New("input failure task39")
	output := &docTestOutput{}
	steps := docTestCreateInput(filepath.Dir(source), "Курс")
	steps = append(steps, docTestStep{failure: failure, before: docTestBefore(t, "l001.json")})
	input := &docTestInput{t: t, output: &output.Buffer, steps: steps}
	err := console.Run(input, output, base, "v39", nil)
	var issue *diagnostic.Error
	if !errors.Is(err, failure) || !errors.As(err, &issue) || issue.Code != console.CodeInput {
		t.Fatalf("lost input failure: %v", err)
	}
	if input.readsAfterFailure != 0 || input.closed || output.closed {
		t.Fatal("continued or closed input after failure")
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected write artifacts: %v", entries)
	}
}

// docTestGateWriter воспроизводит отказ стандартного вывода перед согласием.
type docTestGateWriter struct {
	buffer            bytes.Buffer
	trigger           string
	failure           error
	callsAfterFailure int
	failed            bool
}

func (w *docTestGateWriter) Write(p []byte) (int, error) {
	if w.failed {
		w.callsAfterFailure++
		return 0, w.failure
	}
	if strings.Contains(w.buffer.String()+string(p), w.trigger) {
		w.failed = true
		return 0, w.failure
	}
	return w.buffer.Write(p)
}

func TestConsoleDocOutputFailureAtConsentPreventsWrite(t *testing.T) {
	base := t.TempDir()
	source := docTestSource(t, base, "input/a.json", nil, nil)
	target := filepath.Join(base, "отказ вывода.zip")
	failure := errors.New("output failure task39")
	writer := &docTestGateWriter{trigger: "Записать показанный модуль", failure: failure}
	steps := docTestCreateInput(filepath.Dir(source), "Курс")
	steps = append(steps, docTestDestination(base, "отказ вывода")...)
	steps = append(steps, docTestReviewStep(nil), docTestAnswer("1"))
	input := &docTestInput{t: t, output: &writer.buffer, steps: steps}
	err := console.Run(input, writer, base, "v39", nil)
	if !errors.Is(err, failure) {
		t.Fatalf("lost output failure: %v", err)
	}
	if !writer.failed || writer.callsAfterFailure != 0 {
		t.Fatal("continued broken output")
	}
	if input.next != len(steps)-1 {
		t.Fatal("read consent after failed output")
	}
	docTestAssertAbsent(t, target)
}

func TestConsoleDocShortWelcomeWriteIsReturnedToCaller(t *testing.T) {
	writer := docTestShortWriter{}
	err := console.Run(strings.NewReader("4\n"), writer, t.TempDir(), "v39", nil)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short output: %v", err)
	}
}

type docTestShortWriter struct{}

func (docTestShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
