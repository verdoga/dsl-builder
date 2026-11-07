//go:build linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/console"
)

func TestMainProcessRealOSFailures(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"getwd", "input", "output", "diagnostic", "output-and-diagnostic"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			output, diagnostic, code := mainTestExecute(t, binary, directory, "4\n", []string{"-test.run=^TestMainProcessChild$"}, []string{
				"DSL_MAIN_TEST_CHILD=" + scenario,
				"DSL_MAIN_TEST_DIRECTORY=" + directory,
			})
			wantOutput, err := os.ReadFile(filepath.Join(directory, "expected-stdout.txt"))
			if err != nil {
				t.Fatalf("child setup/reference did not complete: %v; code=%d stdout=%q stderr=%q", err, code, output, diagnostic)
			}
			wantDiagnostic, err := os.ReadFile(filepath.Join(directory, "expected-stderr.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if code != 1 || output != string(wantOutput) || diagnostic != string(wantDiagnostic) {
				t.Fatalf("actual main code=%d stdout=%q stderr=%q; want code=1 stdout=%q stderr=%q", code, output, diagnostic, wantOutput, wantDiagnostic)
			}
		})
	}
}

// TestMainProcessChild вызывает настоящий main после детерминированного сбоя ОС.
// Переменные окружения управляют только дочерним тестовым процессом.
func TestMainProcessChild(t *testing.T) {
	scenario := os.Getenv("DSL_MAIN_TEST_CHILD")
	if scenario == "" {
		return
	}
	directory := os.Getenv("DSL_MAIN_TEST_DIRECTORY")
	buildVersion = "v-main-os-fault-qa"
	var output, diagnostic bytes.Buffer
	var runError error
	if scenario == "getwd" {
		removed := filepath.Join(directory, "removed-current-directory")
		if err := os.Mkdir(removed, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(removed); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(removed); err != nil {
			t.Fatal(err)
		}
		if err := os.Unsetenv("PWD"); err != nil {
			t.Fatal(err)
		}
		_, runError = os.Getwd()
		if runError == nil {
			t.Fatal("removed CWD did not cause a real Getwd failure")
		}
	} else {
		if scenario == "input" || scenario == "diagnostic" {
			input, err := os.Open(directory)
			if err != nil {
				t.Fatal(err)
			}
			os.Stdin = input
		}
		if scenario == "output" || scenario == "output-and-diagnostic" {
			os.Stdout = mainTestReadOnlyFile(t, directory, "unwritable-stdout")
			runError = console.Run(os.Stdin, os.Stdout, directory, buildVersion, os.Args[1:])
		} else {
			runError = console.Run(os.Stdin, &output, directory, buildVersion, os.Args[1:])
		}
		if runError == nil {
			t.Fatal("real descriptor fault did not reach the Run error boundary")
		}
	}
	if err := console.PrintError(&diagnostic, runError); err != nil {
		t.Fatal(err)
	}
	if scenario == "diagnostic" || scenario == "output-and-diagnostic" {
		os.Stderr = mainTestReadOnlyFile(t, directory, "unwritable-stderr")
		diagnostic.Reset()
	}
	for name, contents := range map[string][]byte{
		"expected-stdout.txt": output.Bytes(),
		"expected-stderr.txt": diagnostic.Bytes(),
	} {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	main()
	t.Fatal("main returned without terminating the process")
}

func mainTestReadOnlyFile(t *testing.T, directory, name string) *os.File {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return file
}
