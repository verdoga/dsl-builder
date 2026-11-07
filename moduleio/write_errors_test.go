package moduleio

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

type WriteTestSink struct {
	bytes.Buffer
	calls          int
	closed, synced bool
	failure        error
	short          bool
}

func (w *WriteTestSink) Write(raw []byte) (int, error) {
	w.calls++
	if w.failure != nil {
		return 0, w.failure
	}
	if w.short {
		return len(raw) - 1, nil
	}
	return w.Buffer.Write(raw)
}

func (w *WriteTestSink) Close() error { w.closed = true; return nil }
func (w *WriteTestSink) Sync() error  { w.synced = true; return nil }

type WriteTestCompressor struct {
	writeErr, closeErr error
	short              bool
	closed             int
}

func (c *WriteTestCompressor) Write(raw []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	if c.short {
		return len(raw) - 1, nil
	}
	return len(raw), nil
}

func (c *WriteTestCompressor) Close() error { c.closed++; return c.closeErr }

func WriteTestProblem(t *testing.T, problems []*diagnostic.Error, code string, cause error) int {
	t.Helper()
	for i, problem := range problems {
		if problem != nil && problem.Code == code && errors.Is(problem, cause) {
			if problem.Reason == "" || problem.Cause == nil {
				t.Fatalf("incomplete %s diagnostic: %#v", code, problem)
			}
			return i
		}
	}
	t.Fatalf("missing %s with cause %v: %#v", code, cause, problems)
	return -1
}

func TestWriteReportsFinalCloseFailureAndRetainsCallerStream(t *testing.T) {
	sentinel := errors.New("destination failed at final flush")
	dst := &WriteTestSink{failure: sentinel}
	context := diagnostic.Context{Path: "кандидат.zip"}
	problems := Write(dst, WriteTestModel(t), context)
	index := WriteTestProblem(t, problems, CodeClose, sentinel)
	if problems[index].Context.Path != context.Path || dst.closed || dst.synced {
		t.Fatal("close failure lost path or closed caller stream")
	}
}

func TestWritePreservesMainThenCloseFailureAndShortWrites(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint("short=", short), func(t *testing.T) {
			sentinel := errors.New("destination failed while writing lesson")
			cause := error(sentinel)
			dst := &WriteTestSink{failure: sentinel}
			if short {
				dst.failure, dst.short, cause = nil, true, io.ErrShortWrite
			}
			value := WriteTestLargeModel(t)
			before := WriteTestSnapshot(t, value)
			context := diagnostic.Context{Path: "кандидат.zip"}
			problems := Write(dst, value, context)
			main := WriteTestProblem(t, problems, CodeWrite, cause)
			close := WriteTestProblem(t, problems, CodeClose, cause)
			if main >= close {
				t.Fatal("close error must follow main error")
			}
			problem := problems[main]
			file := value.Files[1]
			if problem.Context.Path != context.Path || problem.Context.FileName != file.FileName() || problem.Context.LessonID != file.Context().LessonID {
				t.Fatalf("write context: %+v", problem.Context)
			}
			found := false
			for _, related := range problem.Related {
				found = found || related == file.Context()
			}
			if !found {
				t.Fatalf("source missing from Related: %#v", problem.Related)
			}
			if dst.closed || dst.synced || before != WriteTestSnapshot(t, value) {
				t.Fatal("failed Write mutated source or took ownership of destination")
			}
			WriteTestNoBody(t, problems)
		})
	}
}

func WriteTestNoBody(t *testing.T, problems []*diagnostic.Error) {
	t.Helper()
	text := string(WriteTestJSON(t, problems, false))
	for _, secret := range []string{"SECRET_BODY_WRITE", "AAECA//+", "_____{am}", `"formatVersion"`} {
		if strings.Contains(text, secret) {
			t.Errorf("diagnostics contain lesson body %q", secret)
		}
	}
}

func TestWriteEntryDetectsCompressorCreationWriteAndShortWriteErrors(t *testing.T) {
	sentinel := errors.New("compressor failed")
	for _, mode := range []string{"create", "write", "short"} {
		t.Run(mode, func(t *testing.T) {
			zw := zip.NewWriter(io.Discard)
			compressor := &WriteTestCompressor{}
			cause := error(sentinel)
			zw.RegisterCompressor(zip.Deflate, func(io.Writer) (io.WriteCloser, error) {
				if mode == "create" {
					return nil, sentinel
				}
				return compressor, nil
			})
			if mode == "write" {
				compressor.writeErr = sentinel
			} else if mode == "short" {
				compressor.short, cause = true, io.ErrShortWrite
			}
			context := diagnostic.Context{Path: "кандидат.zip", FileName: "l010.json", LessonID: "WRAA"}
			problem := writeEntry(zw, "l010.json", []byte(`{"value":"SECRET_BODY_WRITE"}`), context)
			WriteTestProblem(t, []*diagnostic.Error{problem}, CodeWrite, cause)
			if problem.Context != context {
				t.Fatalf("lost entry context: %+v", problem.Context)
			}
			WriteTestNoBody(t, []*diagnostic.Error{problem})
			_ = zw.Close()
		})
	}
}

func TestWriteContentsStopsAfterManifestWriteOrFinalizationFailure(t *testing.T) {
	for _, failAtClose := range []bool{false, true} {
		t.Run(fmt.Sprint("close=", failAtClose), func(t *testing.T) {
			sentinel := errors.New("manifest compressor failed")
			compressor := &WriteTestCompressor{}
			if failAtClose {
				compressor.closeErr = sentinel
			} else {
				compressor.writeErr = sentinel
			}
			created := 0
			zw := zip.NewWriter(io.Discard)
			zw.RegisterCompressor(zip.Deflate, func(io.Writer) (io.WriteCloser, error) {
				created++
				return compressor, nil
			})
			problems := writeContents(zw, WriteTestModel(t), diagnostic.Context{Path: "кандидат.zip"})
			WriteTestProblem(t, problems, CodeWrite, sentinel)
			if created != 1 {
				t.Fatalf("continued after failure: created %d compressors", created)
			}
			if !failAtClose && compressor.closed != 0 {
				t.Fatal("writeContents closed writer owned by Write")
			}
			_ = zw.Close()
		})
	}
}

func TestWriteLessonFailurePreservesCandidateAndOriginalSource(t *testing.T) {
	file := WriteTestModel(t).Files[0]
	source := file.Context()
	original := file.OriginalBytes()
	sentinel := errors.New("lesson compression failed")
	zw := zip.NewWriter(io.Discard)
	zw.RegisterCompressor(zip.Deflate, func(io.Writer) (io.WriteCloser, error) {
		return &WriteTestCompressor{writeErr: sentinel}, nil
	})
	context := diagnostic.Context{Path: "временный кандидат.zip"}
	problems := writeLesson(zw, file, context)
	index := WriteTestProblem(t, problems, CodeWrite, sentinel)
	expected := context
	expected.FileName, expected.LessonID = file.FileName(), source.LessonID
	if problems[index].Context != expected {
		t.Fatalf("candidate context: %+v, want %+v", problems[index].Context, expected)
	}
	found := false
	for _, related := range problems[index].Related {
		found = found || related == source
	}
	if !found || file.Context() != source || !bytes.Equal(file.OriginalBytes(), original) {
		t.Fatal("source context or immutable original bytes were lost")
	}
	WriteTestNoBody(t, problems)
	_ = zw.Close()
}
