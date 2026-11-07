package main

import (
	"bytes"
	"context"
	"crypto/sha256"
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

func TestVersionDefaultIsEmpty(t *testing.T) {
	if buildVersion != "" {
		t.Fatalf("buildVersion по умолчанию = %q; требуется пустое значение", buildVersion)
	}
}

func TestVersionLinkedBinaries(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		inject  bool
	}{
		{name: "unset"},
		{name: "explicit_empty", inject: true},
		{name: "release", version: "v7.3.19", inject: true},
		{name: "unicode", version: "релиз-Ёж_日本語-Ω+42", inject: true},
		{name: "significant_spaces", version: "  Выпуск 2031.07 β  ", inject: true},
		{name: "only_spaces", version: "   ", inject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			binary := versionTestBuild(t, test.version, test.inject)
			for _, input := range []struct {
				name string
				text string
			}{
				{name: "eof"},
				{name: "numeric_exit", text: "4\n"},
			} {
				t.Run(input.name, func(t *testing.T) {
					base := t.TempDir()
					versionTestAssertRun(t, binary, base, test.version, input.text, nil, nil)
				})
			}

			t.Run("runtime_settings_do_not_replace_version", func(t *testing.T) {
				base := t.TempDir()
				config := "{\"buildVersion\":\"runtime-replacement\",\"version\":\"runtime-replacement\",\"mode\":\"create\"}\n"
				for _, name := range []string{"config.json", "dsl-builder.json", ".dsl-builder.json", ".env"} {
					if err := os.WriteFile(filepath.Join(base, name), []byte(config), 0600); err != nil {
						t.Fatal(err)
					}
				}
				arguments := []string{"--version=runtime-replacement", "--build-version", "runtime-replacement", "--config", "config.json", "create", "missing.json"}
				environment := []string{
					"BUILD_VERSION=runtime-replacement", "buildVersion=runtime-replacement",
					"VERSION=runtime-replacement", "DSL_BUILDER_VERSION=runtime-replacement",
					"DSL_BUILDER_BUILD_VERSION=runtime-replacement", "DSL_BUILDER_MODE=create",
					"DSL_BUILDER_INPUT=missing.json", "DSL_BUILDER_CONFIG=" + filepath.Join(base, "config.json"),
				}
				versionTestAssertRun(t, binary, base, test.version, "4\n", arguments, environment)
				for _, name := range []string{"config.json", "dsl-builder.json", ".dsl-builder.json", ".env"} {
					contents, err := os.ReadFile(filepath.Join(base, name))
					if err != nil || string(contents) != config {
						t.Fatalf("исходный конфиг %s изменён: %q, %v", name, contents, err)
					}
				}
				entries, err := os.ReadDir(base)
				if err != nil || len(entries) != 4 {
					t.Fatalf("неожиданные результаты операции: %v, %v", entries, err)
				}
			})
		})
	}
}

// versionTestBuild собирает настоящий main; -X относится к его символу,
// а не к иному пространству имён исполняемого файла go test.
func versionTestBuild(t *testing.T, version string, inject bool) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "dsl-builder")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	arguments := []string{"build", "-buildvcs=false", "-o", binary}
	if inject {
		arguments = append(arguments, "-ldflags", "-X 'main.buildVersion="+version+"'")
	}
	arguments = append(arguments, ".")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), arguments...)
	command.Env = versionTestEnvironment([]string{
		"GOFLAGS=", "GOENV=off", "GOTOOLCHAIN=local",
		"GOOS=" + runtime.GOOS, "GOARCH=" + runtime.GOARCH,
	})
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go %q: %v (deadline: %v)\n%s", arguments, err, ctx.Err(), output)
	}
	contents, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("go %q: exit0; binary SHA256=%x", arguments, sha256.Sum256(contents))
	return binary
}

// versionTestAssertRun сравнивает процесс с публичным стыком настоящего console.
// Формат приветствия и дерева диагностики остаётся обязанностью console.
func versionTestAssertRun(t *testing.T, binary, base, version, input string, arguments, environment []string) {
	t.Helper()
	var expectedOutput, expectedError bytes.Buffer
	runError := console.Run(strings.NewReader(input), &expectedOutput, base, version, arguments)
	expectedExit := 0
	if runError != nil {
		expectedExit = 1
		if err := console.PrintError(&expectedError, runError); err != nil {
			t.Fatalf("ожидаемая диагностика: %v", err)
		}
	}
	if version == "" && (runError == nil || !strings.Contains(expectedError.String(), console.CodeConfiguration)) {
		t.Fatal("публичный console обязан отклонить пустую версию как configuration error")
	}
	if version != "" && runError != nil {
		t.Fatalf("непустая версия должна дойти до штатного EOF/выхода: %v", runError)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Dir = base
	command.Env = versionTestEnvironment(environment)
	command.Stdin = strings.NewReader(input)
	var output, diagnostics bytes.Buffer
	command.Stdout, command.Stderr = &output, &diagnostics
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("процесс не завершился после EOF/числового выхода: %v", ctx.Err())
	}
	exit := 0
	if err != nil {
		var failure *exec.ExitError
		if !errors.As(err, &failure) {
			t.Fatalf("запуск процесса: %v", err)
		}
		exit = failure.ExitCode()
	}
	if exit != expectedExit || output.String() != expectedOutput.String() || diagnostics.String() != expectedError.String() {
		t.Fatalf("version=%q, args=%q\nexit=%d, want=%d\nstdout=%q\nwant stdout=%q\nstderr=%q\nwant stderr=%q", version, arguments, exit, expectedExit, output.String(), expectedOutput.String(), diagnostics.String(), expectedError.String())
	}
	t.Logf("version=%q input=%q args=%q: exit%d stdout=%q stderr=%q", version, input, arguments, exit, output.String(), diagnostics.String())
}

// versionTestEnvironment заменяет только переданные значения дочернего процесса,
// сохраняя среду тестового процесса и общий toolchain/cache.
func versionTestEnvironment(overrides []string) []string {
	environment := os.Environ()
	for _, override := range overrides {
		name, _, _ := strings.Cut(override, "=")
		for index := len(environment) - 1; index >= 0; index-- {
			key, _, _ := strings.Cut(environment[index], "=")
			if strings.EqualFold(key, name) {
				environment = append(environment[:index], environment[index+1:]...)
			}
		}
		environment = append(environment, override)
	}
	return environment
}
