package moduleio_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/safewrite"
)

type IntegrationWriter struct {
	bytes.Buffer
	Closed, Synced bool
	Failure        error
	Short          bool
}

func (w *IntegrationWriter) Write(raw []byte) (int, error) {
	if w.Failure != nil {
		return 0, w.Failure
	}
	if w.Short {
		return 0, nil
	}
	return w.Buffer.Write(raw)
}
func (w *IntegrationWriter) Close() error { w.Closed = true; return nil }
func (w *IntegrationWriter) Sync() error  { w.Synced = true; return nil }

func TestIntegrationWriteValidationAndOwnership(t *testing.T) {
	valid := IntegrationModel(t, IntegrationNew(t, "l001.json", "AB88"))
	for _, tc := range []struct {
		name  string
		value *module.Module
	}{
		{"nil", nil}, {"missing-files", &module.Module{Manifest: valid.Manifest}},
		{"metadata", IntegrationModel(t, valid.Files...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "metadata" {
				(*tc.value.Manifest.Course.Lessons)[0].Title = "Mismatch"
			}
			var dst IntegrationWriter
			problems := moduleio.Write(&dst, tc.value, diagnostic.Context{Path: "candidate.zip"})
			if len(problems) == 0 || dst.Len() != 0 || dst.Closed || dst.Synced {
				t.Fatalf("invalid candidate wrote/owned destination: %v %+v", problems, dst)
			}
		})
	}
	IntegrationProblem(t, moduleio.Write(nil, valid, diagnostic.Context{}), moduleio.CodeWriter)
	var dst IntegrationWriter
	if problems := moduleio.Write(&dst, valid, diagnostic.Context{}); len(problems) != 0 {
		t.Fatal(problems)
	}
	if dst.Closed || dst.Synced {
		t.Fatal("Write took destination ownership")
	}
	if _, err := dst.Write([]byte("caller still owns stream")); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationWriteAndFinalizationErrors(t *testing.T) {
	file := IntegrationNew(t, "l001.json", "AB88")
	value := file.Lesson()
	payload := make([]byte, 0, 128*1024)
	block := sha256.Sum256([]byte("deterministic non-compressible fixture"))
	for range 4096 {
		payload = append(payload, block[:]...)
		block = sha256.Sum256(block[:])
	}
	value.Document.Title = base64.StdEncoding.EncodeToString(payload)
	large, problems := module.NewFile(file.FileName(), value, diagnostic.Context{Path: "source.json"})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	model := IntegrationModel(t, large)
	sentinel := errors.New("stream failure")
	dst := &IntegrationWriter{Failure: sentinel}
	problems = moduleio.Write(dst, model, diagnostic.Context{Path: "candidate.zip"})
	main := IntegrationProblem(t, problems, moduleio.CodeWrite)
	closed := IntegrationProblem(t, problems, moduleio.CodeClose)
	if !errors.Is(main, sentinel) || !errors.Is(closed, sentinel) || problems[len(problems)-1] != closed {
		t.Fatalf("write/close causes or order lost: %v", problems)
	}
	if dst.Closed || dst.Synced {
		t.Fatal("failed write owned destination")
	}
	short := &IntegrationWriter{Short: true}
	problems = moduleio.Write(short, model, diagnostic.Context{Path: "candidate.zip"})
	found := false
	for _, problem := range problems {
		if errors.Is(problem, io.ErrShortWrite) {
			found = true
		}
	}
	if !found {
		t.Fatalf("short write cause lost: %v", problems)
	}
}

func IntegrationJoin(t *testing.T, problems []*diagnostic.Error) error {
	t.Helper()
	causes := make([]error, len(problems))
	for i, problem := range problems {
		causes[i] = problem
	}
	return errors.Join(causes...)
}

func TestIntegrationSafewriteCallbacksPreserveTargetAndErrors(t *testing.T) {
	model := IntegrationModel(t, IntegrationNew(t, "l001.json", "AB88"))
	original := IntegrationWrite(t, model)
	for _, mode := range []string{"success", "invalid-model", "verify-reject"} {
		t.Run(mode, func(t *testing.T) {
			target := IntegrationPath(t, original)
			writes, verifies := 0, 0
			var callbackProblems []*diagnostic.Error
			result, problems := safewrite.Write(target, true, func(dst io.Writer) error {
				writes++
				candidate := model
				if mode == "invalid-model" {
					candidate = nil
				}
				if mode == "verify-reject" {
					_, err := dst.Write([]byte("broken ZIP"))
					return err
				}
				callbackProblems = moduleio.Write(dst, candidate, diagnostic.Context{Path: target})
				return IntegrationJoin(t, callbackProblems)
			}, func(path string) error {
				verifies++
				if path == target {
					t.Fatal("verification did not receive temporary candidate")
				}
				callbackProblems = moduleio.Verify(path)
				return IntegrationJoin(t, callbackProblems)
			})
			if writes != 1 {
				t.Fatal("write callback count", writes)
			}
			if mode == "success" {
				if len(problems) != 0 || !result.Installed || verifies != 1 {
					t.Fatalf("transaction failed: %+v %v", result, problems)
				}
				IntegrationRead(t, target)
			} else {
				if len(problems) == 0 || result.Installed {
					t.Fatalf("failed candidate installed: %+v %v", result, problems)
				}
				wantVerifies := 1
				if mode == "invalid-model" {
					wantVerifies = 0
				}
				if verifies != wantVerifies {
					t.Fatal("verify callback count", verifies)
				}
				raw, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(raw, original) {
					t.Fatal("source target changed", err)
				}
				if len(callbackProblems) == 0 {
					t.Fatal("missing callback diagnostics")
				}
				preserved := false
				for _, problem := range problems {
					if errors.Is(problem, callbackProblems[0]) {
						preserved = true
					}
				}
				if !preserved {
					t.Fatalf("structured callback error chain lost: %v", problems)
				}
			}
			if result.TemporaryPath != "" {
				t.Fatal("temporary candidate remains", result.TemporaryPath)
			}
		})
	}
}
