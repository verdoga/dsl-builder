package console_test

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/console"
)

// docTestStep задаёт один ответ стандартного ввода, а не состояние приложения.
type docTestStep struct {
	label   string
	answer  string
	before  func(string)
	failure error
}

// docTestInput выдаёт только текущую строку: следующий ответ зависит от вывода.
type docTestInput struct {
	t                 *testing.T
	output            *bytes.Buffer
	steps             []docTestStep
	next              int
	offset            int
	pending           string
	closed            bool
	readsAfterFailure int
	failed            bool
}

func (r *docTestInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.pending != "" {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	if r.failed {
		r.readsAfterFailure++
		return 0, io.EOF
	}
	if r.next == len(r.steps) {
		return 0, io.EOF
	}
	step := r.steps[r.next]
	r.next++
	screen := r.output.String()[r.offset:]
	r.offset = r.output.Len()
	if step.before != nil {
		step.before(screen)
	}
	if step.failure != nil {
		r.failed = true
		return 0, step.failure
	}
	response := step.answer
	if step.label != "" {
		selector := regexp.MustCompile(step.label)
		matches := regexp.MustCompile(`(?m)^([0-9]+)\. ([^\r\n]*)`).FindAllStringSubmatch(screen, -1)
		var choices []string
		for _, m := range matches {
			if selector.MatchString(m[2]) {
				choices = append(choices, m[1])
			}
		}
		if len(choices) != 1 {
			r.t.Fatalf("step %d: menu %q matched %v\n%s", r.next, step.label, choices, screen)
		}
		response = choices[0] + "\n"
	}
	r.pending = response
	if response == "" {
		r.failed = true
		return 0, io.EOF
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *docTestInput) Close() error { r.closed = true; return nil }

type docTestOutput struct {
	bytes.Buffer
	closed bool
}

func (w *docTestOutput) Close() error { w.closed = true; return nil }

func docTestAnswer(answer string) docTestStep { return docTestStep{answer: answer + "\n"} }
func docTestMenu(label string) docTestStep    { return docTestStep{label: label} }

func docTestValue(value string) []docTestStep {
	return []docTestStep{docTestMenu(`^Ввести значение$`), docTestAnswer(value)}
}

func docTestCreateInput(directory, course string) []docTestStep {
	result := []docTestStep{docTestAnswer("1")}
	result = append(result, docTestValue(directory)...)
	result = append(result, docTestValue(course)...)
	return append(result, docTestValue("")...)
}

func docTestExistingInput(mode int, archive, source string) []docTestStep {
	result := []docTestStep{docTestAnswer(strconv.Itoa(mode))}
	result = append(result, docTestValue(archive)...)
	return append(result, docTestValue(source)...)
}

func docTestDestination(directory, name string) []docTestStep {
	result := []docTestStep{docTestMenu(`(?i)(назнач|каталог.*(zip|архив)|цел[ьи])`)}
	result = append(result, docTestValue(directory)...)
	return append(result, docTestValue(name)...)
}

func docTestRun(t *testing.T, base string, steps []docTestStep) string {
	t.Helper()
	output := &docTestOutput{}
	input := &docTestInput{t: t, output: &output.Buffer, steps: steps}
	err := console.Run(input, output, base, "v39.λ", []string{"--source=ignored", "--overwrite"})
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, output.String())
	}
	if input.next != len(steps) {
		t.Fatalf("consumed %d/%d answers\n%s", input.next, len(steps), output.String())
	}
	if input.closed || output.closed {
		t.Fatal("Run closed caller streams")
	}
	if input.readsAfterFailure != 0 {
		t.Fatal("Run continued reading after stream failure")
	}
	if !strings.Contains(output.String(), "v39.λ") || !strings.Contains(output.String(), "аргументы не используются") {
		t.Fatalf("missing explicit version/argument notice\n%s", output.String())
	}
	if strings.Contains(output.String(), "DOC39_BODY_PRIVATE") {
		t.Fatal("Run printed Node.Data")
	}
	return output.String()
}

func docTestRequire(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in:\n%s", part, text)
		}
	}
}

func docTestBefore(t *testing.T, parts ...string) func(string) {
	t.Helper()
	return func(screen string) { docTestRequire(t, screen, parts...) }
}

func docTestReviewStep(before func(string)) docTestStep {
	return docTestStep{label: `(?i)(запис|сохран|создать модуль|создать архив)`, before: before}
}

func docTestMissingMenu(t *testing.T, screen, pattern string) {
	t.Helper()
	re := regexp.MustCompile(`(?mi)^\d+\. .*` + pattern)
	if re.MatchString(screen) {
		t.Fatalf("unexpected action %s:\n%s", pattern, screen)
	}
}

func docTestQuote(value string) string { return fmt.Sprintf("\"%s\"", value) }
