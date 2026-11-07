package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/preview"
)

type consoleUpdateTestReader struct {
	next    func() (string, error)
	pending string
	reads   int
}

func (r *consoleUpdateTestReader) Read(p []byte) (int, error) {
	if r.pending == "" {
		r.reads++
		value, err := r.next()
		if err != nil {
			return 0, err
		}
		r.pending = value
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

type consoleUpdateTestFailWriter struct {
	short bool
	cause error
	calls int
}

func (w *consoleUpdateTestFailWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.short {
		return len(p) / 2, nil
	}
	return 0, w.cause
}

func TestConsoleUpdateDecisionsStopBeforeAnswerOnPresentationFailure(t *testing.T) {
	for _, kind := range []string{"field", "placement"} {
		t.Run(kind, func(t *testing.T) {
			before := consoleUpdateTestPlain()
			after := consoleUpdateTestResult("Новое", nil, []*string{nil}, [][]string{{"Материал", "Второй"}})
			after.Document.Metadata.Order = consoleUpdateTestText("030")
			op := consoleUpdateTestOperation(t, before, after, false)
			snapshot := op.Snapshot()
			reader := &consoleUpdateTestReader{next: func() (string, error) { return "1\n", nil }}
			var out bytes.Buffer
			d := &dialog{operation: op, input: bufio.NewReader(reader), output: &out}
			var err error
			if kind == "field" {
				snapshot.Before = nil // Независимый повреждённый набор для preview.
				err = d.decideField(snapshot)
			} else {
				snapshot.PlacementProposals[0].After = []byte("{")
				err = d.decidePlacement(snapshot)
			}
			got := op.Snapshot()
			if err == nil || isStreamFailure(err) || reader.reads != 0 || got.FieldAnswered[0] ||
				got.PlacementAnswered[0] || got.WriteResult != nil {
				t.Fatalf("incomplete presentation allowed a decision: %v reads=%d", err, reader.reads)
			}
		})
	}
}

func TestConsoleUpdateDecisionsDoNotUseEOFOrFailedStreamsAsAnswer(t *testing.T) {
	cause := errors.New("task30 input lost")
	for _, kind := range []string{"field", "placement"} {
		for _, fault := range []string{"eof", "partial-eof", "read", "write", "short-write"} {
			t.Run(kind+"/"+fault, func(t *testing.T) {
				before := consoleUpdateTestPlain()
				after := consoleUpdateTestResult("Новое", nil, []*string{nil}, [][]string{{"Материал", "Второй"}})
				after.Document.Metadata.Order = consoleUpdateTestText("030")
				op := consoleUpdateTestOperation(t, before, after, false)
				snapshot := op.Snapshot()
				var out bytes.Buffer
				d := consoleUpdateTestDialog(op, "", &out)
				var writer *consoleUpdateTestFailWriter
				want := error(io.EOF)
				switch fault {
				case "partial-eof":
					d = consoleUpdateTestDialog(op, "1", &out)
				case "read":
					d.input = bufio.NewReader(&consoleUpdateTestReader{next: func() (string, error) { return "", cause }})
					want = cause
				case "write", "short-write":
					writer = &consoleUpdateTestFailWriter{cause: cause, short: fault == "short-write"}
					d = consoleUpdateTestDialog(op, "1\n", writer)
					want = cause
					if writer.short {
						want = io.ErrShortWrite
					}
				}
				var err error
				if kind == "field" {
					err = d.decideField(snapshot)
				} else {
					err = d.decidePlacement(snapshot)
				}
				got := op.Snapshot()
				if !errors.Is(err, want) || !isStreamFailure(err) || got.FieldAnswered[0] || got.PlacementAnswered[0] || got.WriteResult != nil {
					t.Fatalf("stream failure became consent: %v", err)
				}
				if writer != nil && writer.calls != 1 {
					t.Fatalf("failed output was retried %d times", writer.calls)
				}
			})
		}
	}
}

func TestConsoleUpdatePrepareDiagnosticStreamFailureIsFatal(t *testing.T) {
	before := consoleUpdateTestPlain()
	after := consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Новый материал", "Второй"}})
	op := consoleUpdateTestOperation(t, before, after, false, true)
	snapshot := op.Snapshot()
	writer := &consoleUpdateTestFailWriter{short: true}
	err := consoleUpdateTestDialog(op, "1\n", writer).prepareRevision(snapshot)
	if !errors.Is(err, io.ErrShortWrite) || !isStreamFailure(err) || writer.calls != 1 ||
		op.Snapshot().RevisionReady || op.Snapshot().Token != snapshot.Token || op.Snapshot().WriteResult != nil {
		t.Fatalf("diagnostic stream failure was swallowed or retried: %v", err)
	}
}

func TestConsoleUpdatePlacementDoesNotRefreshStaleToken(t *testing.T) {
	before, after := consoleUpdateTestPlain(), consoleUpdateTestPlain()
	after.Document.Metadata.Section, after.Document.Metadata.Order = consoleUpdateTestText("Beta"), consoleUpdateTestText("030")
	op := consoleUpdateTestOperation(t, before, after, true)
	snapshot := op.Snapshot()
	consoleUpdateTestOK(t, op.DecidePlacement(snapshot.Token, 1, true))
	var out bytes.Buffer
	if err := consoleUpdateTestDialog(op, "1\n", &out).decidePlacement(snapshot); err != nil {
		t.Fatal(err)
	}
	if op.Snapshot().PlacementAnswered[0] || !bytes.Contains(out.Bytes(), []byte(app.CodeStaleReview)) {
		t.Fatal("stale placement token bypassed owner")
	}
}

// consoleUpdateTestLimitedWriter допускает полный proposal и обрывает вопрос.
type consoleUpdateTestLimitedWriter struct{ remaining int }

func (w *consoleUpdateTestLimitedWriter) Write(p []byte) (int, error) {
	if len(p) <= w.remaining {
		w.remaining -= len(p)
		return len(p), nil
	}
	n := w.remaining
	w.remaining = 0
	return n, nil
}

func TestConsoleUpdateQuestionOutputFailurePreventsDecision(t *testing.T) {
	for _, kind := range []string{"field", "placement"} {
		t.Run(kind, func(t *testing.T) {
			before := consoleUpdateTestPlain()
			after := consoleUpdateTestResult("Новое", nil, []*string{nil}, [][]string{{"Материал", "Второй"}})
			after.Document.Metadata.Order = consoleUpdateTestText("030")
			op := consoleUpdateTestOperation(t, before, after, false)
			snapshot := op.Snapshot()
			var text string
			var err error
			if kind == "field" {
				text, err = preview.FieldProposal(snapshot.Before, snapshot.Plan.Fields[0])
			} else {
				text, err = preview.ManifestChanges(snapshot.PlacementProposals[:1])
			}
			if err != nil {
				t.Fatal(err)
			}
			d := consoleUpdateTestDialog(op, "1\n", &consoleUpdateTestLimitedWriter{remaining: len(text)})
			if kind == "field" {
				err = d.decideField(snapshot)
			} else {
				err = d.decidePlacement(snapshot)
			}
			if !errors.Is(err, io.ErrShortWrite) || op.Snapshot().FieldAnswered[0] || op.Snapshot().PlacementAnswered[0] || op.Snapshot().WriteResult != nil {
				t.Fatalf("partial confirmation question allowed decision: %v", err)
			}
		})
	}
}
