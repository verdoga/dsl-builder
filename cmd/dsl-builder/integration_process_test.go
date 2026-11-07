package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func integrationTestBuild(t *testing.T) string {
	t.Helper()
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go")
	name := "dsl-builder"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goTool, "build", "-ldflags=-X=main.buildVersion=v8.0.0-Зима", "-o", binary, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build actual command: %v\n%s", err, output)
	}
	return binary
}

type integrationTestProcess struct {
	t          *testing.T
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	input      io.WriteCloser
	lines      chan string
	stderr     bytes.Buffer
	transcript strings.Builder
	finished   bool
}

func integrationTestStart(t *testing.T, binary, directory string, arguments ...string) *integrationTestProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	p := &integrationTestProcess{t: t, cancel: cancel, lines: make(chan string, 64)}
	p.cmd = exec.CommandContext(ctx, binary, arguments...)
	p.cmd.Dir, p.cmd.Stderr = directory, &p.stderr
	var err error
	p.input, err = p.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	go func() {
		defer close(p.lines)
		reader := bufio.NewReader(output)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				select {
				case p.lines <- line:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		if !p.finished {
			_ = p.input.Close()
			_ = p.cmd.Wait()
		}
	})
	return p
}

func (p *integrationTestProcess) until(marker string) string {
	p.t.Helper()
	var screen strings.Builder
	for line := range p.lines {
		p.transcript.WriteString(line)
		screen.WriteString(line)
		if strings.TrimSpace(line) == marker {
			return screen.String()
		}
	}
	p.t.Fatalf("process ended before %q\n%s", marker, p.transcript.String())
	return ""
}

func (p *integrationTestProcess) send(value string) {
	p.t.Helper()
	if _, err := fmt.Fprintln(p.input, value); err != nil {
		p.t.Fatal(err)
	}
}

func (p *integrationTestProcess) selectLabel(screen, label string) {
	p.t.Helper()
	for _, line := range strings.Split(screen, "\n") {
		number, text, found := strings.Cut(line, ". ")
		if found && text == label {
			p.send(number)
			return
		}
	}
	p.t.Fatalf("missing menu label %q\n%s", label, screen)
}

func (p *integrationTestProcess) choose(label string) string {
	p.t.Helper()
	screen := p.until("Ваш выбор:")
	p.selectLabel(screen, label)
	return screen
}

func (p *integrationTestProcess) value(value string, optional bool) {
	p.t.Helper()
	p.choose("Ввести значение")
	marker := "Введите значение:"
	if optional {
		marker = "Введите название юнита (Enter — без юнита):"
	}
	p.until(marker)
	p.send(value)
}

func (p *integrationTestProcess) finish() string {
	p.t.Helper()
	_ = p.input.Close()
	for line := range p.lines {
		p.transcript.WriteString(line)
	}
	err := p.cmd.Wait()
	p.finished = true
	p.cancel()
	p.t.Logf("process cwd=%q argv=%q exit=%d\nstdout:\n%s\nstderr:\n%s", p.cmd.Dir, p.cmd.Args, p.cmd.ProcessState.ExitCode(), p.transcript.String(), p.stderr.String())
	if err != nil || p.stderr.Len() != 0 {
		p.t.Fatalf("expected exit 0 and empty stderr: %v", err)
	}
	return p.transcript.String()
}

func integrationTestRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationTestCopy(t *testing.T, fixture, target string) {
	t.Helper()
	raw := integrationTestRead(t, filepath.Join("testdata", "integration", fixture+".json"))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, raw, 0644); err != nil {
		t.Fatal(err)
	}
}
