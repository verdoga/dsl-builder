package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func runTestCode(t *testing.T, err error, code string) {
	t.Helper()
	var issue *diagnostic.Error
	if !errors.As(err, &issue) || issue.Code != code {
		t.Fatalf("error = %v; want diagnostic code %s", err, code)
	}
}

func runTestMainMenu(t *testing.T, text string) {
	t.Helper()
	for _, line := range []string{"1. Создать новый модуль", "2. Обновить существующий модуль", "3. Добавить уроки в существующий модуль", "4. Выход"} {
		if !strings.Contains(text, line+"\n") && !strings.Contains(text, line+"\r\n") {
			t.Errorf("missing separate main-menu line %q in %q", line, text)
		}
	}
	if strings.ContainsAny(text, "\x1b\x0c") {
		t.Errorf("terminal control in output %q", text)
	}
}

func TestRunRejectsMissingConfiguration(t *testing.T) {
	base := t.TempDir()
	cases := []struct {
		name          string
		input         io.Reader
		output        io.Writer
		base, version string
	}{
		{"input", nil, io.Discard, base, "v7"},
		{"output", strings.NewReader("4\n"), nil, base, "v7"},
		{"base", strings.NewReader("4\n"), io.Discard, "", "v7"},
		{"version", strings.NewReader("4\n"), io.Discard, base, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runTestCode(t, Run(tc.input, tc.output, tc.base, tc.version, nil), CodeConfiguration)
		})
	}
}

func TestRunWelcomeUsesBuildVersionAndIgnoresArguments(t *testing.T) {
	for _, version := range []string{"v8.12.34-тест Ω", "build 2031.9"} {
		for _, args := range [][]string{nil, {}, {"--create", "--version", "4", "argument-secret.zip"}} {
			var out bytes.Buffer
			if err := Run(strings.NewReader("4\n"), &out, filepath.Join(t.TempDir(), "absent"), version, args); err != nil {
				t.Fatal(err)
			}
			text := out.String()
			if !strings.Contains(text, "DSL-Builder") || !strings.Contains(text, version) {
				t.Fatalf("welcome = %q", text)
			}
			if strings.Contains(text, "v1.0.0") {
				t.Errorf("hardcoded version: %q", text)
			}
			notice := "Работа выполняется через интерактивное меню; аргументы не используются"
			if strings.Contains(text, notice) != (len(args) > 0) {
				t.Errorf("arguments notice = %q", text)
			}
			if strings.Contains(text, "argument-secret.zip") {
				t.Error("argument interpreted or echoed")
			}
			runTestMainMenu(t, text)
		}
	}
}

func TestRunMainChoiceOnlyReturnsShownNumbers(t *testing.T) {
	invalid := []string{"", "0", "5", "-1", "one", "да", "1x", "1 2", "1.0", strings.Repeat("9", 100000)}
	for choice := 1; choice <= 4; choice++ {
		var out bytes.Buffer
		input := strings.Join(invalid, "\n") + "\n" + string(rune('0'+choice)) + "\r\n"
		d := dialog{input: bufio.NewReader(strings.NewReader(input)), output: &out}
		got, err := d.mainChoice()
		if err != nil || got != choice {
			t.Fatalf("choice %d = %d, %v", choice, got, err)
		}
		runTestMainMenu(t, out.String())
		if got := strings.Count(out.String(), "1. Создать новый модуль"); got != len(invalid)+1 {
			t.Errorf("menu shown %d times; want %d", got, len(invalid)+1)
		}
	}
}

func TestRunEOFDiscardsPartialMainChoice(t *testing.T) {
	for _, input := range []string{"", "1", "2", "3", "4", "1\r", "\n1"} {
		var out bytes.Buffer
		if err := Run(strings.NewReader(input), &out, t.TempDir(), "v9", nil); err != nil {
			t.Fatalf("input %q: %v", input, err)
		}
		if strings.Contains(out.String(), "1. Ввести значение") {
			t.Errorf("unterminated choice started mode: %q", input)
		}
	}
}

func TestRunShowWelcomeEscapesExternalControlsWithoutTruncation(t *testing.T) {
	version := "  версия " + strings.Repeat("Ю🙂", 40000) + "\n\t\x1b[2J\r\x00  "
	var out bytes.Buffer
	d := dialog{output: &out}
	if err := d.showWelcome(version, nil); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "  версия "+strings.Repeat("Ю🙂", 40000)) || !strings.Contains(text, "  ") {
		t.Fatal("version Unicode/spaces truncated")
	}
	for _, control := range []string{"\t", "\x1b", "\r", "\x00"} {
		if strings.Contains(text, control) {
			t.Errorf("unescaped control %q", control)
		}
	}
	if strings.Contains(text, "🙂\n") {
		t.Error("external newline changed output structure")
	}
}
