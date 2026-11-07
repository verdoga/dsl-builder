package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

type inputTestReader struct {
	text  string
	err   error
	calls int
}

func (r *inputTestReader) Read(p []byte) (int, error) {
	r.calls++
	if r.calls > 1 {
		return 0, io.EOF
	}
	return copy(p, r.text), r.err
}

type inputTestWriter struct {
	count int
	err   error
	calls int
}

func (w *inputTestWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.count, w.err
}

func inputTestDialog(input io.Reader, output io.Writer) *dialog {
	return &dialog{input: bufio.NewReader(input), output: output}
}

func inputTestStreamError(t *testing.T, err error, code string, cause error) {
	t.Helper()
	var issue *diagnostic.Error
	if !errors.As(err, &issue) || issue == nil {
		t.Fatalf("error = %v; want diagnostic.Error", err)
	}
	if issue.Code != code || !strings.ContainsFunc(issue.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("diagnostic = %#v; want code %q and reason", issue, code)
	}
	if issue.Context != (diagnostic.Context{}) {
		t.Errorf("standard stream acquired file context: %#v", issue.Context)
	}
	if !errors.Is(err, cause) {
		t.Errorf("error = %v; original cause %v lost", err, cause)
	}
}

func TestInputReadLinePreservesDataAndLineEndings(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"LF", "строка\n", "строка"},
		{"CRLF", "строка\r\n", "строка"},
		{"only final CR removed", "а\rб\r\r\n", "а\rб\r"},
		{"empty", "\n", ""},
		{"raw", "  \t\"C:\\Курс 雪\\0\"  \n", "  \t\"C:\\Курс 雪\\0\"  "},
		{"zero", "0\n", "0"},
		{"unicode digits", "٠１２٣\n", "٠１２٣"},
		{"long", strings.Repeat("Ж🙂 ", 20000) + "\n", strings.Repeat("Ж🙂 ", 20000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := inputTestDialog(strings.NewReader(tc.input), io.Discard)
			got, err := d.readLine()
			if err != nil || got != tc.want {
				t.Fatalf("readLine length=%d error=%v; want length=%d exact data", len(got), err, len(tc.want))
			}
		})
	}
}

func TestInputReadLineUsesOneBufferAndDiscardsIncompleteEOF(t *testing.T) {
	d := inputTestDialog(strings.NewReader("первая\r\nвторая\nне ответ\r"), io.Discard)
	for _, want := range []string{"первая", "вторая"} {
		got, err := d.readLine()
		if err != nil || got != want {
			t.Fatalf("readLine = %q, %v; want %q, nil", got, err, want)
		}
	}
	for range 2 {
		got, err := d.readLine()
		if got != "" || err != io.EOF {
			t.Fatalf("incomplete/exhausted line = %q, %v; want empty and direct EOF", got, err)
		}
	}
}

func TestInputReadLineEOFWithDataInSameRead(t *testing.T) {
	for _, text := range []string{"", "не завершено", "\"путь\"\r"} {
		r := &inputTestReader{text: text, err: io.EOF}
		got, err := inputTestDialog(r, io.Discard).readLine()
		if got != "" || err != io.EOF || r.calls != 1 {
			t.Errorf("readLine(%q) = %q, %v, reads=%d; want empty, direct EOF, one read", text, got, err, r.calls)
		}
	}
}

func TestInputReadLineAcceptsCompletedLineBeforeEOF(t *testing.T) {
	r := &inputTestReader{text: "готово\n", err: io.EOF}
	d := inputTestDialog(r, io.Discard)
	got, err := d.readLine()
	if got != "готово" || err != nil {
		t.Fatalf("completed line = %q, %v; want completed answer before EOF", got, err)
	}
	got, err = d.readLine()
	if got != "" || err != io.EOF {
		t.Fatalf("after completed line = %q, %v; want direct EOF", got, err)
	}
}

func TestInputReadLineFailurePreservesOriginalWithoutRetry(t *testing.T) {
	for _, partial := range []string{"", "незавершённая строка"} {
		cause := &os.PathError{Op: "read", Path: "stream", Err: errors.New("broken input")}
		r := &inputTestReader{text: partial, err: cause}
		got, err := inputTestDialog(r, io.Discard).readLine()
		if got != "" || r.calls != 1 {
			t.Fatalf("readLine returned fragment %q or retried %d reads", got, r.calls)
		}
		inputTestStreamError(t, err, CodeInput, cause)
		var original *os.PathError
		if !errors.As(err, &original) || original != cause {
			t.Errorf("original typed cause lost: %v", err)
		}
	}
}

func TestInputWriteTextWritesExactCompleteText(t *testing.T) {
	for _, text := range []string{"", "  Текст 雪\r\n0\t\"путь\"  ", strings.Repeat("длинная строка🙂", 10000)} {
		var out bytes.Buffer
		err := inputTestDialog(strings.NewReader(""), &out).writeText(text)
		if err != nil || out.String() != text {
			t.Fatalf("writeText length=%d: written=%d error=%v", len(text), out.Len(), err)
		}
	}
}

func TestInputWriteTextFailureIsFatalWithoutRetry(t *testing.T) {
	cause := &os.PathError{Op: "write", Path: "stream", Err: errors.New("broken output")}
	for _, tc := range []struct {
		name  string
		count int
		err   error
		want  error
	}{
		{"zero short", 0, nil, io.ErrShortWrite},
		{"partial short", 2, nil, io.ErrShortWrite},
		{"zero error", 0, cause, cause},
		{"partial error", 2, cause, cause},
		{"full count with error", len("готовый текст"), cause, cause},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &inputTestWriter{count: tc.count, err: tc.err}
			err := inputTestDialog(strings.NewReader(""), w).writeText("готовый текст")
			inputTestStreamError(t, err, CodeOutput, tc.want)
			if w.calls != 1 {
				t.Errorf("write calls=%d; want one without retry/diagnostic write", w.calls)
			}
			if tc.err != nil {
				var original *os.PathError
				if !errors.As(err, &original) || original != cause {
					t.Errorf("original typed cause lost: %v", err)
				}
			}
		})
	}
}
