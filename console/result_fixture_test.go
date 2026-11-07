package console

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func resultTestDialog(t *testing.T, input io.Reader, output io.Writer, op *app.Operation) *dialog {
	t.Helper()
	return &dialog{input: bufio.NewReader(input), output: output, operation: op}
}

func resultTestIssues(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("owner fixture issues: %v", issues)
	}
}

func resultTestCreate(t *testing.T) (*app.Operation, string, string) {
	t.Helper()
	base := t.TempDir()
	source := filepath.Join(base, "Исходники с пробелами")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/result/source.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(source, "урок.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	op, issues := app.NewCreate(base, source, "Курс результата", "")
	resultTestIssues(t, issues)
	resultTestIssues(t, op.Analyze(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)))
	return op, path, base
}

func resultTestConsent(t *testing.T, op *app.Operation, directory string) string {
	t.Helper()
	resultTestIssues(t, op.SetDestination(op.Snapshot().Token, directory, "Результат.zip"))
	review, issues := op.ReviewSave(op.Snapshot().Token)
	resultTestIssues(t, issues)
	if review == nil {
		t.Fatal("owner did not return save preview")
	}
	resultTestIssues(t, op.ConfirmSave(review.Token, true, false))
	if !op.Snapshot().Actions.Save {
		t.Fatal("owner fixture lacks current save consent")
	}
	return review.Destination.Path
}

func resultTestSaved(t *testing.T) (*app.Operation, string, string) {
	t.Helper()
	op, source, directory := resultTestCreate(t)
	path := resultTestConsent(t, op, directory)
	result, issues := op.Save(time.Date(2026, 11, 7, 12, 1, 0, 0, time.UTC))
	resultTestIssues(t, issues)
	if !result.Installed {
		t.Fatal("owner fixture did not install ZIP")
	}
	return op, source, path
}

func resultTestUpdate(t *testing.T) (*app.Operation, string, string) {
	t.Helper()
	_, source, archive := resultTestSaved(t)
	op, issues := app.NewUpdate(filepath.Dir(archive), archive, source)
	resultTestIssues(t, issues)
	resultTestIssues(t, op.Analyze(time.Date(2026, 11, 7, 13, 0, 0, 0, time.UTC)))
	return op, source, archive
}

func resultTestCanceled(t *testing.T, op *app.Operation, previous app.ReviewToken) {
	t.Helper()
	if op.Snapshot().Token == previous {
		t.Fatal("navigation retained current operation token; Cancel was not observed")
	}
	if edit, issues := op.ProposeCourseTitle(previous, "Не применять"); edit != nil || len(issues) == 0 {
		t.Fatal("navigation left the former token usable")
	}
}

func resultTestUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("file changed: %s; error: %v", path, err)
	}
}

func resultTestBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type resultTestReaderFunc func([]byte) (int, error)

func (read resultTestReaderFunc) Read(p []byte) (int, error) { return read(p) }

type resultTestWriterFunc func([]byte) (int, error)

func (write resultTestWriterFunc) Write(p []byte) (int, error) { return write(p) }

func resultTestMenuKind(t *testing.T, label string) string {
	t.Helper()
	label = strings.ToLower(label)
	switch {
	case strings.Contains(label, "главное меню"):
		return "main"
	case strings.Contains(label, "выход"):
		return "exit"
	case strings.Contains(label, "заново"):
		return "restart"
	case strings.Contains(label, "повтор") && strings.Contains(label, "анализ"):
		return "retry"
	default:
		return "unsafe"
	}
}

// resultTestMenuInput выбирает номер только из уже напечатанного меню.
func resultTestMenuInput(t *testing.T, output *bytes.Buffer, wanted string) io.Reader {
	t.Helper()
	used := false
	return resultTestReaderFunc(func(p []byte) (int, error) {
		if used {
			return 0, io.EOF
		}
		used = true
		rows := regexp.MustCompile(`(?m)^\s*([0-9]+)\.\s*(.+)$`).FindAllStringSubmatch(output.String(), -1)
		for _, row := range rows {
			if resultTestMenuKind(t, row[2]) == wanted {
				return copy(p, row[1]+"\n"), nil
			}
		}
		t.Errorf("requested %s action absent from menu: %s", wanted, output.String())
		return 0, io.EOF
	})
}

func resultTestMenu(t *testing.T, output string, expected ...string) {
	t.Helper()
	rows := regexp.MustCompile(`(?m)^\s*([0-9]+)\.\s*(.+)$`).FindAllStringSubmatch(output, -1)
	want := make(map[string]bool, len(expected))
	for _, kind := range expected {
		want[kind] = true
	}
	if len(rows) != len(want) {
		t.Errorf("menu has %d rows; want %d: %s", len(rows), len(want), output)
	}
	for index, row := range rows {
		if row[1] != strconv.Itoa(index+1) {
			t.Errorf("nonconsecutive menu numbering: %s", output)
		}
		kind := resultTestMenuKind(t, row[2])
		if !want[kind] {
			t.Errorf("unexpected or duplicate menu action %q: %s", kind, output)
		}
		delete(want, kind)
	}
	if len(want) != 0 {
		t.Errorf("missing actions: %v", want)
	}
}

func resultTestTime(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 11, 7, 14, 0, 0, 0, time.UTC)
}

func resultTestWriteSource(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func resultTestNoTerminal(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, message := range []string{"содержательных изменений не обнаружено", "урок, его версия и архив оставлены без изменений"} {
		if strings.Contains(lower, message) {
			t.Errorf("invalid state announced no changes: %s", text)
		}
	}
}
