package console

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRunPublicInitialGroupsCancelAtomicallyAtEveryEOF(t *testing.T) {
	base := t.TempDir()
	source := runTestSource(t, filepath.Join(base, "source"), "BA82")
	archive := runTestArchive(t, base, filepath.Dir(source))
	groups := []string{
		"1\n1\n" + filepath.Dir(source) + "\n1\nКурс\n1\n\n",
		"2\n1\n" + archive + "\n1\n" + source + "\n",
		"3\n1\n" + archive + "\n1\n" + source + "\n",
	}
	for _, group := range groups {
		// Every answer boundary and a partial value exercise the same public Run.
		for end := 1; end <= len(group); end++ {
			if group[end-1] != '\n' && end != len(group)-1 {
				continue
			}
			prefix := group[:end]
			t.Run(strings.ReplaceAll(prefix, "\n", "|")[:1]+"/"+strconv.Itoa(end), func(t *testing.T) {
				beforeArchive, err := os.ReadFile(archive)
				if err != nil {
					t.Fatal(err)
				}
				beforeSource, err := os.ReadFile(source)
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				if err := Run(strings.NewReader(prefix), &out, base, "v-contract", nil); err != nil {
					t.Fatal(err)
				}
				for path, before := range map[string][]byte{archive: beforeArchive, source: beforeSource} {
					after, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(before, after) {
						t.Errorf("EOF changed input %s", path)
					}
				}
				entries, err := os.ReadDir(base)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 2 {
					t.Errorf("EOF created files: %v", entries)
				}
			})
		}
	}
}

func TestRunPublicMainNumbersEnterTheRequestedInitialDialog(t *testing.T) {
	for _, choice := range []string{"1", "2", "3"} {
		t.Run(choice, func(t *testing.T) {
			var out bytes.Buffer
			if err := Run(strings.NewReader(choice+"\n"), &out, t.TempDir(), "v-contract", nil); err != nil {
				t.Fatal(err)
			}
			text := out.String()
			if !strings.Contains(text, "1. Ввести значение") || !strings.Contains(text, "2. Отменить текущий ввод и вернуться") {
				t.Fatalf("mode has no numeric initial input: %q", text)
			}
			if choice == "1" && !strings.Contains(strings.ToLower(text), "каталог") {
				t.Fatal("create did not ask for source directory")
			}
			if choice != "1" && !strings.Contains(strings.ToLower(text), "архив") && !strings.Contains(strings.ToLower(text), "zip") {
				t.Fatal("update/add did not ask for archive")
			}
		})
	}
}

func TestRunStartupDoesNotInspectBaseFilesystem(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "regular-file")
	if err := os.WriteFile(file, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{file, filepath.Join(root, "missing", "child")} {
		if err := Run(strings.NewReader("4\n"), io.Discard, base, "v9", []string{"--config=missing.json"}); err != nil {
			t.Fatalf("startup inspected base %q: %v", base, err)
		}
	}
}

type runTestFailAfterReader struct {
	prefix      *strings.Reader
	cause       error
	failedReads int
}

func (r *runTestFailAfterReader) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		return r.prefix.Read(p)
	}
	r.failedReads++
	return 0, r.cause
}

func TestRunReadFailureAfterInitialAnswersNeverContinues(t *testing.T) {
	base := t.TempDir()
	source := runTestSource(t, filepath.Join(base, "source"), "BA82")
	// Ни один ответ не выбирает действие: порядок меню операции не фиксирован.
	// Нечисловая полная строка повторяет то же меню до ошибки чтения.
	for _, suffix := range []string{"", "partial", "not-a-number\npartial"} {
		fault := errors.New("late input failure")
		in := &runTestFailAfterReader{prefix: strings.NewReader("1\n1\n" + filepath.Dir(source) + "\n1\nКурс\n1\n\n" + suffix), cause: fault}
		var out bytes.Buffer
		err := Run(in, &out, base, "v9", nil)
		runTestCode(t, err, CodeInput)
		if !errors.Is(err, fault) || in.failedReads != 1 {
			t.Errorf("failure lost/retried: %v; reads=%d", err, in.failedReads)
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Errorf("stream failure wrote output: %v", entries)
		}
	}
}
