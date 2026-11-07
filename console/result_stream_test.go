package console

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestResultNavigationStopsAtFirstOutputFailure(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "presentation", true: "terminal"}[terminal], func(t *testing.T) {
			op, source, directory := resultTestCreate(t)
			before := resultTestBytes(t, source)
			path := resultTestConsent(t, op, directory)
			reads, writes := 0, 0
			fault := errors.New("writer failed")
			input := resultTestReaderFunc(func(p []byte) (int, error) {
				reads++
				return copy(p, "1\n"), nil
			})
			output := resultTestWriterFunc(func(p []byte) (int, error) {
				writes++
				return 0, fault
			})
			d := resultTestDialog(t, input, output, op)
			var err error
			if terminal {
				_, err = d.terminalChoice(op.Snapshot())
			} else {
				_, err = d.presentationFailureChoice(op.Snapshot())
			}
			if !errors.Is(err, fault) || reads != 0 || writes != 1 {
				t.Fatalf("fault did not stop immediately: err=%v reads=%d writes=%d", err, reads, writes)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || op.Snapshot().WriteResult != nil {
				t.Error("output failure led to Save")
			}
			resultTestUnchanged(t, source, before)
		})
	}
}

func TestResultNavigationReadFailureIsNotAnAnswer(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "presentation", true: "terminal"}[terminal], func(t *testing.T) {
			op, _, directory := resultTestCreate(t)
			path := resultTestConsent(t, op, directory)
			fault := errors.New("reader failed")
			var output bytes.Buffer
			reads, lengthAtRead := 0, 0
			input := resultTestReaderFunc(func(p []byte) (int, error) {
				reads++
				lengthAtRead = output.Len()
				return 0, fault
			})
			d := resultTestDialog(t, input, &output, op)
			var err error
			if terminal {
				_, err = d.terminalChoice(op.Snapshot())
			} else {
				_, err = d.presentationFailureChoice(op.Snapshot())
			}
			if !errors.Is(err, fault) || reads != 1 || output.Len() != lengthAtRead {
				t.Fatalf("read failure was retried or printed again: err=%v reads=%d", err, reads)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || op.Snapshot().WriteResult != nil {
				t.Error("read failure led to Save")
			}
		})
	}
}

func TestResultNoChangesOutputFailurePreventsReadingTerminalAnswer(t *testing.T) {
	op, _, _ := resultTestUpdate(t)
	resultTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
	fault := errors.New("no-change output failed")
	reads, writes := 0, 0
	d := resultTestDialog(t, resultTestReaderFunc(func(p []byte) (int, error) {
		reads++
		return 0, io.EOF
	}), resultTestWriterFunc(func(p []byte) (int, error) {
		writes++
		return 0, fault
	}), op)
	_, err := d.noChangesChoice(op.Snapshot())
	if !errors.Is(err, fault) || reads != 0 || writes != 1 || op.Snapshot().WriteResult != nil {
		t.Fatalf("no-change output failure: err=%v reads=%d writes=%d", err, reads, writes)
	}
}

func TestResultPresentationInvalidNumericInputKeepsSafeMenu(t *testing.T) {
	op, _, _ := resultTestCreate(t)
	snapshot := op.Snapshot()
	var output bytes.Buffer
	d := resultTestDialog(t, strings.NewReader("0\n999\n"), &output, op)
	_, err := d.presentationFailureChoice(snapshot)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("invalid numeric choices did not repeat until EOF: %v", err)
	}
	rows := strings.Split(output.String(), "\n")
	for _, row := range rows {
		if strings.Contains(row, ". ") && (strings.Contains(strings.ToLower(row), "запис") || strings.HasSuffix(row, ". Да")) {
			t.Errorf("unsafe action after invalid input: %s", row)
		}
	}
}
