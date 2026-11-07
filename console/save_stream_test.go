package console

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsoleSaveDestinationEveryOutputFailureCancels(t *testing.T) {
	baseline := consoleSaveTestCreate(t, false)
	var transcript consoleSaveTestWriter
	input := "1\n" + filepath.Dir(baseline.target) + "\n1\nResult\n"
	if err := consoleSaveTestDialog(baseline.op, strings.NewReader(input), &transcript).chooseDestination(baseline.op.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if transcript.calls == 0 {
		t.Fatal("no output observed")
	}
	for _, short := range []bool{false, true} {
		for call := 1; call <= transcript.calls; call++ {
			t.Run(fmt.Sprintf("short=%v/write=%d", short, call), func(t *testing.T) {
				f := consoleSaveTestCreate(t, false)
				unchanged := consoleSaveTestUnchanged(t, f.target)
				fault := errors.New("destination output fault")
				w := &consoleSaveTestWriter{failAt: call, cause: fault}
				if short {
					w.cause = nil
				}
				r := &consoleSaveTestReader{lines: []string{"1\n", filepath.Dir(f.target) + "\n", "1\n", "Result\n"}}
				err := consoleSaveTestDialog(f.op, r, w).chooseDestination(f.op.Snapshot())
				want := error(fault)
				if short {
					want = io.ErrShortWrite
				}
				if !errors.Is(err, want) || !consoleSaveTestHasCode(err, CodeOutput) {
					t.Fatalf("output cause lost: %v", err)
				}
				if !w.failed || w.afterFailure != 0 || f.op.Snapshot().Actions.Cancel || f.op.Snapshot().Actions.Save || f.op.Snapshot().Target != nil {
					t.Fatalf("continued after output failure: writes=%d actions=%+v", w.afterFailure, f.op.Snapshot().Actions)
				}
				unchanged()
			})
		}
	}
}

func TestConsoleSaveReviewEveryOutputFailureStopsImmediately(t *testing.T) {
	baseline := consoleSaveTestCreate(t, true)
	snapshot := consoleSaveTestTarget(t, baseline)
	var transcript consoleSaveTestWriter
	if got, err := consoleSaveTestDialog(baseline.op, strings.NewReader("1\n2\n"), &transcript).reviewSaving(snapshot); err != nil || got != routePreview {
		t.Fatalf("baseline route=%v err=%v", got, err)
	}
	if transcript.calls == 0 {
		t.Fatal("no output observed")
	}
	for _, short := range []bool{false, true} {
		for call := 1; call <= transcript.calls; call++ {
			t.Run(fmt.Sprintf("short=%v/write=%d", short, call), func(t *testing.T) {
				f := consoleSaveTestCreate(t, true)
				unchanged := consoleSaveTestUnchanged(t, f.target)
				s := consoleSaveTestTarget(t, f)
				fault := errors.New("save review output fault")
				w := &consoleSaveTestWriter{failAt: call, cause: fault}
				if short {
					w.cause = nil
				}
				r := &consoleSaveTestReader{lines: []string{"1\n", "1\n", "2\n"}}
				_, err := consoleSaveTestDialog(f.op, r, w).reviewSaving(s)
				want := error(fault)
				if short {
					want = io.ErrShortWrite
				}
				if !errors.Is(err, want) || !consoleSaveTestHasCode(err, CodeOutput) {
					t.Fatalf("output cause/code lost: %v", err)
				}
				if !w.failed || w.afterFailure != 0 {
					t.Errorf("writes after failed output: %d", w.afterFailure)
				}
				after := f.op.Snapshot()
				if after.Actions.Cancel || after.Actions.Save || after.WriteResult != nil {
					t.Fatalf("stream failure continued/retained consent: %+v", after.Actions)
				}
				unchanged()
			})
		}
	}
}

func TestConsoleSaveReviewInputLossAfterFullPreviewCancels(t *testing.T) {
	for _, fragment := range []string{"", "1"} {
		t.Run("fragment="+fragment, func(t *testing.T) {
			f := consoleSaveTestCreate(t, false)
			s := consoleSaveTestTarget(t, f)
			unchanged := consoleSaveTestUnchanged(t, f.target)
			var out bytes.Buffer
			_, err := consoleSaveTestDialog(f.op, strings.NewReader(fragment), &out).reviewSaving(s)
			if !errors.Is(err, io.EOF) {
				t.Fatalf("partial answer accepted: %v", err)
			}
			if !strings.Contains(out.String(), f.target) || f.op.Snapshot().Actions.Cancel || f.op.Snapshot().WriteResult != nil {
				t.Fatalf("preview/Cancel missing: %s", out.String())
			}
			unchanged()
		})
	}
}
