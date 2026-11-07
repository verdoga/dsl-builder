package app

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

// callbacksWriteStream проверяет границу стандартного io.Writer, не заменяя владельцев формата.
type callbacksWriteStream struct {
	bytes.Buffer
	closed bool
	fail   error
}

func (stream *callbacksWriteStream) Write(raw []byte) (int, error) {
	if stream.fail != nil {
		return 0, stream.fail
	}
	return stream.Buffer.Write(raw)
}

func (stream *callbacksWriteStream) Close() error {
	stream.closed = true
	return nil
}

func TestCallbacksWriteCompletesZIPAndPreservesImmutableInput(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		t.Run(fmt.Sprintf("loaded=%t", loaded), func(t *testing.T) {
			value := callbacksFixtureModule(t, loaded, "")
			before := callbacksFixtureJSON(t, value)
			file := value.Files[0]
			lessonBefore := callbacksFixtureJSON(t, file.Lesson())
			original := file.OriginalBytes()
			context := diagnostic.Context{Path: filepath.Join(t.TempDir(), "кандидат.zip")}
			callback := newWriteCallback(value, context)
			first, second := &callbacksWriteStream{}, &callbacksWriteStream{}
			if err := callback(first); err != nil {
				t.Fatal(err)
			}
			if first.closed {
				t.Fatal("callback closed caller-owned stream")
			}
			firstRaw := append([]byte(nil), first.Bytes()...)
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			if err := callback(second); err != nil {
				t.Fatal(err)
			}
			if second.closed || !bytes.Equal(first.Bytes(), firstRaw) {
				t.Fatal("callback retained/reused the prior destination or closed the next")
			}
			if value.Files[0] != file || !bytes.Equal(before, callbacksFixtureJSON(t, value)) ||
				!bytes.Equal(lessonBefore, callbacksFixtureJSON(t, file.Lesson())) || !bytes.Equal(original, file.OriginalBytes()) {
				t.Fatal("callback changed module, file model, timestamps or original bytes")
			}
			for index, raw := range [][]byte{firstRaw, second.Bytes()} {
				reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
				if err != nil {
					t.Fatalf("ZIP %d is unfinished: %v", index, err)
				}
				if len(reader.File) != 2 || reader.File[0].Name != module.ManifestFileName || reader.File[1].Name != file.FileName() {
					t.Fatal("unexpected ZIP root composition/order")
				}
				for _, entry := range reader.File {
					if strings.ContainsAny(entry.Name, `/\`) || !entry.Mode().IsRegular() {
						t.Fatalf("non-flat regular entry: %s", entry.Name)
					}
				}
				path := filepath.Join(t.TempDir(), "завершённый.zip")
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if issues := moduleio.Verify(path); len(issues) != 0 {
					t.Fatalf("Verify completed callback ZIP: %v", issues)
				}
				read, issues := moduleio.Read(path)
				if len(issues) != 0 {
					t.Fatalf("Read completed callback ZIP: %v", issues)
				}
				if !bytes.Equal(lessonBefore, callbacksFixtureJSON(t, read.Files[0].Lesson())) ||
					!bytes.Equal(callbacksFixtureJSON(t, value.Manifest), callbacksFixtureJSON(t, read.Manifest)) {
					t.Fatal("callback transformed model or used current time")
				}
				if loaded && !bytes.Equal(original, read.Files[0].OriginalBytes()) {
					t.Fatal("loaded lesson was reserialized")
				}
			}
		})
	}
}

func TestCallbacksWritePreservesAllOwnerValidationErrors(t *testing.T) {
	context := diagnostic.Context{Path: "не открывать этот путь.zip", Line: 12, Column: 3}
	for _, test := range []struct {
		name  string
		value *module.Module
	}{
		{"nil module", nil},
		{"independent invalid branches", &module.Module{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := module.Validate(test.value, context)
			stream := &callbacksWriteStream{}
			got := newWriteCallback(test.value, context)(stream)
			children := callbacksFixtureChildren(t, got)
			if len(want) == 0 || len(children) != len(want) {
				t.Fatalf("callback errors = %d; owner errors = %d", len(children), len(want))
			}
			for index := range want {
				if !bytes.Equal(callbacksFixtureJSON(t, children[index]), callbacksFixtureJSON(t, want[index])) {
					t.Fatalf("owner diagnostic %d changed code/context/details/order", index)
				}
			}
			if stream.Len() != 0 || stream.closed {
				t.Fatal("invalid module wrote bytes or closed destination")
			}
		})
	}
}

func TestCallbacksWriteNilDestinationPreservesOwnerCode(t *testing.T) {
	value := callbacksFixtureModule(t, false, "")
	context := diagnostic.Context{Path: "кандидат.zip", LessonID: "known-id", Line: 4}
	children := callbacksFixtureChildren(t, newWriteCallback(value, context)(nil))
	if len(children) != 1 || children[0].Code != moduleio.CodeWriter || children[0].Context != context {
		t.Fatalf("nil destination diagnostics = %+v", children)
	}
}

func TestCallbacksWriteKeepsZIPCompletionFailure(t *testing.T) {
	value := callbacksFixtureModule(t, false, "")
	cause := &os.PathError{Op: "write", Path: "кандидат.zip", Err: io.ErrClosedPipe}
	stream := &callbacksWriteStream{fail: cause}
	context := diagnostic.Context{Path: cause.Path}
	err := newWriteCallback(value, context)(stream)
	children := callbacksFixtureChildren(t, err)
	var pathError *os.PathError
	if !errors.Is(err, io.ErrClosedPipe) || !errors.As(err, &pathError) || pathError != cause {
		t.Fatal("ZIP completion failure lost stream cause")
	}
	if len(children) == 0 || children[len(children)-1].Code != moduleio.CodeClose || children[len(children)-1].Context != context || stream.closed {
		t.Fatalf("ZIP completion diagnostic/ownership = %+v, closed %t", children, stream.closed)
	}
}

func TestCallbacksWriteKeepsPrimaryAndCloseFailures(t *testing.T) {
	var title strings.Builder
	for index := 0; index < 4000; index++ {
		sum := sha256.Sum256([]byte(fmt.Sprint(index)))
		fmt.Fprintf(&title, "%x", sum)
	}
	value := callbacksFixtureModule(t, false, title.String())
	cause := errors.New("ошибка стандартного потока")
	context := diagnostic.Context{Path: "кандидат.zip"}
	stream := &callbacksWriteStream{fail: cause}
	err := newWriteCallback(value, context)(stream)
	children := callbacksFixtureChildren(t, err)
	if len(children) < 2 || children[0].Code != moduleio.CodeWrite || children[len(children)-1].Code != moduleio.CodeClose {
		t.Fatalf("primary and completion errors/order lost: %+v", children)
	}
	for index, child := range children {
		if !errors.Is(child, cause) || child.Context.Path != context.Path || child.Reason == "" {
			t.Fatalf("child %d lost cause/path/reason: %+v", index, child)
		}
	}
	if !errors.Is(err, cause) || stream.closed {
		t.Fatal("callback discarded owner cause or closed underlying stream")
	}
}
