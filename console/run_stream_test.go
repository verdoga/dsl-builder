package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

type runTestReader struct {
	io.Reader
	reads, closes int
	failure       error
}

func (r *runTestReader) Read(p []byte) (int, error) {
	r.reads++
	if r.failure != nil {
		return 0, r.failure
	}
	return r.Reader.Read(p)
}
func (r *runTestReader) Close() error { r.closes++; return nil }

type runTestWriter struct {
	bytes.Buffer
	writes, closes int
	failure        error
	short          bool
}

func (w *runTestWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.failure != nil {
		return 0, w.failure
	}
	if w.short {
		return len(p) - 1, nil
	}
	return w.Buffer.Write(p)
}

// WriteString применяет ту же инъекцию сбоя, что и Write, включая io.WriteString.
func (w *runTestWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func (w *runTestWriter) Close() error { w.closes++; return nil }

func TestRunDoesNotCloseBorrowedStreams(t *testing.T) {
	for _, text := range []string{"4\n", "", "1\n1\npartial path"} {
		in := &runTestReader{Reader: strings.NewReader(text)}
		out := &runTestWriter{}
		if err := Run(in, out, t.TempDir(), "v9", nil); err != nil {
			t.Fatal(err)
		}
		if in.closes != 0 || out.closes != 0 {
			t.Errorf("closes input=%d output=%d", in.closes, out.closes)
		}
	}
}

func TestRunStopsImmediatelyOnReadFailure(t *testing.T) {
	cause := errors.New("reader fault")
	in := &runTestReader{failure: cause}
	var out bytes.Buffer
	err := Run(in, &out, t.TempDir(), "v9", nil)
	runTestCode(t, err, CodeInput)
	if !errors.Is(err, cause) {
		t.Errorf("lost cause: %v", err)
	}
	if in.reads != 1 {
		t.Errorf("read retried %d times", in.reads)
	}
	runTestMainMenu(t, out.String())
	if strings.Contains(out.String(), cause.Error()) {
		t.Error("stream error printed back to console")
	}
}

func TestRunStopsImmediatelyOnOutputFailure(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "short"}[short], func(t *testing.T) {
			cause := errors.New("writer fault")
			out := &runTestWriter{short: short}
			if !short {
				out.failure = cause
			} else {
				cause = io.ErrShortWrite
			}
			in := &runTestReader{Reader: strings.NewReader("1\n1\nsource\n1\ncourse\n1\n\n")}
			err := Run(in, out, t.TempDir(), "v9", nil)
			runTestCode(t, err, CodeOutput)
			if !errors.Is(err, cause) {
				t.Errorf("lost output cause: %v", err)
			}
			if out.writes != 1 || in.reads != 0 {
				t.Errorf("continued after failed welcome: writes=%d reads=%d", out.writes, in.reads)
			}
		})
	}
}

func TestRunMainLoopCancelsExistingOperationOnExit(t *testing.T) {
	base := t.TempDir()
	op, issues := app.NewCreate(base, "unused", "Курс", "")
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	old := op.Snapshot().Token
	d := dialog{input: bufio.NewReader(strings.NewReader("4\n")), output: io.Discard, baseDir: base, operation: op}
	if err := d.mainLoop(); err != nil {
		t.Fatal(err)
	}
	snapshot := op.Snapshot()
	if snapshot.Actions.Cancel || snapshot.Actions.Retry || snapshot.Actions.EditCatalog || snapshot.Token == old {
		t.Error("current operation was not cancelled")
	}
}
