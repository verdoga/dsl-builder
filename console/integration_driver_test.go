package console

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// integrationTestStep answers only after observing the preceding screen.
type integrationTestStep struct {
	literal  string
	rawValue bool
	menu     string
	check    func(string)
	before   func()
}

type integrationTestDriver struct {
	t       *testing.T
	output  bytes.Buffer
	steps   []integrationTestStep
	pending []byte
	offset  int
	reads   int
	final   []byte
	failure error
}

func (d *integrationTestDriver) Read(p []byte) (int, error) {
	if len(d.pending) == 0 {
		if len(d.steps) == 0 {
			if len(d.final) != 0 {
				n := copy(p, d.final)
				d.final = d.final[n:]
				return n, nil
			}
			if d.failure != nil {
				return 0, d.failure
			}
			return 0, io.EOF
		}
		step := d.steps[0]
		d.steps = d.steps[1:]
		d.reads++
		screen := d.output.String()[d.offset:]
		d.offset = d.output.Len()
		if step.check != nil {
			step.check(screen)
		}
		answer := step.literal
		if step.menu != "" {
			pattern := regexp.MustCompile("(?i)" + step.menu)
			matches := regexp.MustCompile(`(?m)^\s*([0-9]+)\.\s*(.+)$`).FindAllStringSubmatch(screen, -1)
			answer = ""
			for _, m := range matches {
				if pattern.MatchString(m[2]) {
					if answer != "" {
						d.t.Fatalf("ambiguous menu %q at answer %d:\n%s", step.menu, d.reads, screen)
					}
					answer = m[1]
				}
			}
			if answer == "" {
				d.t.Fatalf("menu %q absent before answer %d:\n%s", step.menu, d.reads, screen)
			}
		}
		if step.menu == "" && !step.rawValue && regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(answer) {
			shown := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(answer) + `\.\s+\S`)
			if !shown.MatchString(screen) {
				d.t.Fatalf("numeric answer %s was not displayed before reply %d:\n%s", answer, d.reads, screen)
			}
		}
		if step.before != nil {
			step.before()
		}
		d.pending = []byte(answer + "\n")
	}
	n := copy(p, d.pending)
	d.pending = d.pending[n:]
	return n, nil
}

func integrationTestMenu(pattern string) integrationTestStep {
	return integrationTestStep{menu: pattern}
}
func integrationTestNumber(n int, label string) integrationTestStep {
	return integrationTestStep{literal: strconv.Itoa(n), check: func(s string) {
		expression := fmt.Sprintf(`(?mi)^\s*%d\.\s*.*%s`, n, label)
		if !regexp.MustCompile(expression).MatchString(s) {
			panic(fmt.Sprintf("numbered choice %d/%q not displayed:\n%s", n, label, s))
		}
	}}
}
func integrationTestValue(raw string) []integrationTestStep {
	return []integrationTestStep{integrationTestNumber(1, "Ввести значение"), {literal: raw, rawValue: true}}
}
func integrationTestAppendValue(steps []integrationTestStep, raw ...string) []integrationTestStep {
	for _, s := range raw {
		steps = append(steps, integrationTestValue(s)...)
	}
	return steps
}
func integrationTestContains(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, s := range values {
		if !strings.Contains(text, s) {
			t.Fatalf("output lacks %q:\n%s", s, text)
		}
	}
}
func integrationTestRun(t *testing.T, base string, steps []integrationTestStep) *integrationTestDriver {
	t.Helper()
	d := &integrationTestDriver{t: t, steps: steps}
	if err := Run(d, &d.output, base, "task41-версия", []string{"--unknown", "ignored.zip"}); err != nil {
		t.Fatalf("Run: %v\n%s", err, d.output.String())
	}
	if len(d.steps) != 0 || len(d.pending) != 0 {
		t.Fatalf("Run ended before %d scripted steps", len(d.steps))
	}
	return d
}
