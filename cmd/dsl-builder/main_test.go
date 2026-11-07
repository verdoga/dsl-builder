package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/console"
)

func TestMainProcessLaunch(t *testing.T) {
	const version = "v-main-entry-qa"
	binary := mainTestBuild(t, version)
	cwd := t.TempDir()
	for _, scenario := range []struct {
		name  string
		input string
		args  []string
	}{
		{name: "numeric exit", input: "4\n"},
		{name: "empty EOF"},
		{name: "partial numeric answer is cancelled", input: "1"},
		{name: "partial path is cancelled", input: "1\n1\nнезавершённый путь"},
		{name: "partial create group is cancelled", input: "1\n1\nнесуществующий каталог\n1\nКурс\n1\n"},
		{name: "invalid input remains interactive", input: "--help\n0\n4\n"},
		{name: "arguments only explain menu", input: "4\n", args: []string{"--help", "--version", "create", "чужой путь.zip"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			wantOutput, wantDiagnostic := mainTestConsole(t, scenario.input, cwd, version, scenario.args)
			output, diagnostic, code := mainTestExecute(t, binary, cwd, scenario.input, scenario.args, nil)
			if code != 0 || output != wantOutput || diagnostic != wantDiagnostic {
				t.Fatalf("process code=%d\nstdout=%q\nstderr=%q\nwant stdout=%q\nwant stderr=%q", code, output, diagnostic, wantOutput, wantDiagnostic)
			}
		})
	}
}

func TestMainProcessUsesCurrentDirectoryForRelativeInput(t *testing.T) {
	const version = "v-main-cwd-qa"
	binary := mainTestBuild(t, version)
	cwd := filepath.Join(t.TempDir(), "Текущий каталог с пробелами")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	relative := "отсутствующий вход"
	input := "1\n1\n" + relative + "\n1\nКурс\n1\n\n"
	wantOutput, wantDiagnostic := mainTestConsole(t, input, cwd, version, nil)
	if !strings.Contains(wantOutput, filepath.Join(cwd, relative)) {
		t.Fatalf("reference console did not expose the resolved relative input: %q", wantOutput)
	}
	output, diagnostic, code := mainTestExecute(t, binary, cwd, input, nil, []string{"PWD=/unrelated/environment/path"})
	if code != 0 || output != wantOutput || diagnostic != wantDiagnostic {
		t.Fatalf("CWD process code=%d\nstdout=%q\nstderr=%q\nwant stdout=%q\nwant stderr=%q", code, output, diagnostic, wantOutput, wantDiagnostic)
	}
	entries, err := os.ReadDir(cwd)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancelled operation changed CWD: entries=%v err=%v", entries, err)
	}
}

func mainTestBuild(t *testing.T, version string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "dsl-builder")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-buildvcs=false", "-ldflags", "-X main.buildVersion="+version, "-o", binary, ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build explicit-version executable: %v\n%s", err, output)
	}
	return binary
}

func mainTestConsole(t *testing.T, input, cwd, version string, args []string) (string, string) {
	t.Helper()
	var output, diagnostic bytes.Buffer
	if err := console.Run(strings.NewReader(input), &output, cwd, version, args); err != nil {
		if failure := console.PrintError(&diagnostic, err); failure != nil {
			t.Fatal(failure)
		}
		t.Fatalf("reference ordinary console scenario unexpectedly failed: %v\n%s", err, diagnostic.String())
	}
	return output.String(), diagnostic.String()
}

func mainTestExecute(t *testing.T, binary, cwd, input string, args, environment []string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = cwd
	command.Env = mainTestEnvironment(environment)
	command.Stdin = strings.NewReader(input)
	var output, diagnostic bytes.Buffer
	command.Stdout, command.Stderr = &output, &diagnostic
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("process did not finish without an extra read, pause or retry: %v", ctx.Err())
	}
	if err == nil {
		return output.String(), diagnostic.String(), 0
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("process never reached executable main: %v", err)
	}
	return output.String(), diagnostic.String(), exit.ExitCode()
}

func mainTestEnvironment(additions []string) []string {
	environment := os.Environ()
	for _, addition := range additions {
		key, _, _ := strings.Cut(addition, "=")
		for index := len(environment) - 1; index >= 0; index-- {
			if strings.HasPrefix(environment[index], key+"=") {
				environment = append(environment[:index], environment[index+1:]...)
			}
		}
		environment = append(environment, addition)
	}
	return environment
}
