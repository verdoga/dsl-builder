package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegrationFatalOSStreams(t *testing.T) {
	binary := integrationTestBuild(t)
	for _, fault := range []string{"input", "output"} {
		t.Run(fault, func(t *testing.T) {
			cwd := t.TempDir()
			path := filepath.Join(cwd, "Поток только для чтения.txt")
			if err := os.WriteFile(path, []byte("immutable"), 0600); err != nil {
				t.Fatal(err)
			}
			readOnly, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer readOnly.Close()
			before := integrationTestTree(t, cwd, "")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary)
			cmd.Dir = cwd
			var stdout, stderr bytes.Buffer
			cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(""), &stdout, &stderr
			if fault == "input" {
				directory, err := os.Open(cwd)
				if err != nil {
					t.Fatal(err)
				}
				defer directory.Close()
				cmd.Stdin = directory
			} else {
				cmd.Stdout = readOnly
			}
			err = cmd.Run()
			t.Logf("fault=%s exit=%d stdout=%q stderr=%q", fault, cmd.ProcessState.ExitCode(), stdout.String(), stderr.String())
			if err == nil || cmd.ProcessState.ExitCode() != 1 || ctx.Err() != nil {
				t.Fatalf("fatal stream must return promptly with exit 1: %v", err)
			}
			if strings.Count(stderr.String(), "Код: console."+fault+"\n") != 1 || strings.Contains(stderr.String(), "panic:") {
				t.Fatalf("missing one diagnostic root (multiline causes allowed): %s", stderr.String())
			}
			if fault == "output" && stdout.Len() != 0 {
				t.Fatal("failed stdout leaked into another output")
			}
			integrationTestSameTree(t, before, integrationTestTree(t, cwd, ""))
		})
	}
}
