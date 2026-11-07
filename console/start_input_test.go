package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestStartEveryQuestionCancelsOrDiscardsEOFFragment(t *testing.T) {
	base := t.TempDir()
	archive := startTestArchive(t, base)
	source := startTestSource(t, base, "входы/урок.json", "BA82", nil)
	startTestUnchanged(t, source, archive)
	for _, mode := range []string{"create", "update", "add"} {
		values := []string{archive, source}
		if mode == "create" {
			values = []string{filepath.Dir(source), "Курс", "Юнит"}
		}
		for question := range values {
			for _, ending := range []string{"cancel", "eof-menu", "eof-value", "read-error"} {
				t.Run(mode+"/"+values[question]+"/"+ending, func(t *testing.T) {
					prefix := startTestAnswers(values[:question]...)
					var input io.Reader
					fault := errors.New("потерян поток начального ввода")
					switch ending {
					case "cancel":
						input = strings.NewReader(prefix + "2\n")
					case "eof-menu":
						input = strings.NewReader(prefix + "1")
					case "eof-value":
						input = strings.NewReader(prefix + "1\n" + values[question])
					case "read-error":
						input = io.MultiReader(strings.NewReader(prefix+"1\n"), startTestReadFunc(func([]byte) (int, error) { return 0, fault }))
					}
					var output bytes.Buffer
					d := dialog{input: bufio.NewReader(input), output: &output, baseDir: base}
					start := d.startCreate
					if mode == "update" {
						start = d.startUpdate
					} else if mode == "add" {
						start = d.startAdd
					}
					op, err := start()
					if op != nil || d.operation != nil {
						t.Fatal("неполная группа создала Operation")
					}
					if ending == "cancel" && err != nil || strings.HasPrefix(ending, "eof") && !errors.Is(err, io.EOF) || ending == "read-error" && !errors.Is(err, fault) {
						t.Fatalf("ending=%s err=%v", ending, err)
					}
				})
			}
		}
	}
}

func TestStartCancellationDefersConstructorUntilWholeGroup(t *testing.T) {
	for _, mode := range []string{"create", "update", "add"} {
		t.Run(mode, func(t *testing.T) {
			var output bytes.Buffer
			// NUL даёт ошибку настоящего конструктора, если вызвать его преждевременно.
			d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers("bad\x00path") + "2\n")), output: &output, baseDir: t.TempDir()}
			start := d.startCreate
			if mode == "update" {
				start = d.startUpdate
			} else if mode == "add" {
				start = d.startAdd
			}
			op, err := start()
			if op != nil || err != nil || strings.Contains(output.String(), "filetree.") {
				t.Fatalf("отмена дошла до конструктора: op=%v err=%v output=%s", op, err, &output)
			}
		})
	}
}

func TestStartCreateOnlyUnitAllowsEmptyAndNoAnswersLeak(t *testing.T) {
	base := t.TempDir()
	source := startTestSource(t, base, "json/урок.json", "BA82", nil)
	startTestUnchanged(t, source)
	var output bytes.Buffer
	input := startTestAnswers("отброшенный путь", "отброшенный курс") + "2\n" +
		startTestAnswers("json", "Курс Ё", "")
	d := dialog{input: bufio.NewReader(strings.NewReader(input)), output: &output, baseDir: base}
	if op, err := d.startCreate(); op != nil || err != nil {
		t.Fatalf("отмена юнита: %v %v", op, err)
	}
	op, err := d.startCreate()
	if op == nil || err != nil {
		t.Fatalf("полный повтор: %v %v", op, err)
	}
	d.operation = op
	if err := d.analyze(); err != nil {
		t.Fatal(err)
	}
	snapshot := op.Snapshot()
	if snapshot.Catalog == nil || snapshot.Catalog.CourseTitle != "Курс Ё" || snapshot.Catalog.UnitTitle != nil || len(snapshot.Blockers) != 0 {
		t.Fatalf("пустой обязательный ответ/отмена потеряли данные: %+v", snapshot)
	}
	for _, input := range []string{"1\n\n", startTestAnswers("json") + "1\n\n"} {
		d := dialog{input: bufio.NewReader(strings.NewReader(input)), output: io.Discard, baseDir: base}
		if op, err := d.startCreate(); op != nil || !errors.Is(err, io.EOF) {
			t.Fatalf("пустое обязательное значение принято: %v %v", op, err)
		}
	}
}

func TestStartPathsKeepOneQuotePairAndLiteralBackslashes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux fixture использует буквальные обратные слеши и кавычки в имени")
	}
	for _, directory := range []string{`C:\Курс\new\tab`, `"внутренние кавычки"`} {
		t.Run(directory, func(t *testing.T) {
			base := t.TempDir()
			path := startTestSource(t, base, filepath.Join(directory, "урок.json"), "BA82", nil)
			startTestUnchanged(t, path)
			d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers(`"`+directory+`"`, "Курс", ""))), output: io.Discard, baseDir: base}
			op, err := d.startCreate()
			if op == nil || err != nil {
				t.Fatalf("конструктор: %v %v", op, err)
			}
			d.operation = op
			if err := d.analyze(); err != nil || len(op.Snapshot().Blockers) != 0 {
				t.Fatalf("снята лишняя пара или раскрыт escape: %v %v", err, op.Snapshot().Blockers)
			}
		})
	}
}

func TestStartFailureChoiceStrictNumericRoutesWithoutOperation(t *testing.T) {
	for _, test := range []struct {
		answer string
		want   route
	}{{"1", routeRestart}, {"2", routeMain}, {"3", routeExit}} {
		var output bytes.Buffer
		d := dialog{input: bufio.NewReader(strings.NewReader("\nда\n0\n4\n" + test.answer + "\n")), output: &output, baseDir: t.TempDir()}
		got, err := d.startFailureChoice()
		if err != nil || got != test.want || d.operation != nil {
			t.Fatalf("answer=%s got=%v err=%v", test.answer, got, err)
		}
		if strings.Contains(strings.ToLower(output.String()), "повторить анализ") {
			t.Fatalf("Retry предложен без Operation: %s", &output)
		}
	}
	for _, input := range []string{"", "1"} {
		d := dialog{input: bufio.NewReader(strings.NewReader(input)), output: io.Discard}
		if _, err := d.startFailureChoice(); !errors.Is(err, io.EOF) {
			t.Fatalf("EOF стал выбором: %v", err)
		}
	}
}

func TestStartUpdateAndAddPreserveInnerQuotesForOwnerInputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux fixture использует кавычки и буквальные обратные слеши в каталоге")
	}
	for _, mode := range []string{"update", "add-file", "add-directory"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			archive := startTestArchive(t, base)
			directory := `"Курс\new\tab"`
			if err := os.Mkdir(filepath.Join(base, directory), 0700); err != nil {
				t.Fatal(err)
			}
			movedArchive := filepath.Join(base, directory, filepath.Base(archive))
			if err := os.Rename(archive, movedArchive); err != nil {
				t.Fatal(err)
			}
			id := "NEW2"
			if mode == "update" {
				id = "BA82"
			}
			source := startTestSource(t, base, filepath.Join(directory, "json", "урок.json"), id, nil)
			startTestUnchanged(t, movedArchive, source)
			relativeSource := filepath.Join(directory, "json", "урок.json")
			if mode == "add-directory" {
				relativeSource = filepath.Dir(relativeSource)
			}
			d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers(`"`+filepath.Join(directory, filepath.Base(archive))+`"`, `"`+relativeSource+`"`))), output: io.Discard, baseDir: base}
			start := d.startUpdate
			if mode != "update" {
				start = d.startAdd
			}
			op, err := start()
			if op == nil || err != nil {
				t.Fatalf("start: %v %v", op, err)
			}
			d.operation = op
			if err := d.analyze(); err != nil {
				t.Fatal(err)
			}
			snapshot := op.Snapshot()
			// Add без исходного веса имеет настоящий исправляемый catalog blocker.
			if snapshot.Module == nil || snapshot.ArchivePath != movedArchive || snapshot.Catalog == nil {
				t.Fatalf("console снял лишние кавычки/раскрыл escape: %+v", snapshot)
			}
		})
	}
}

func TestStartConstructorErrorsAreShownAndReturnNil(t *testing.T) {
	base := t.TempDir()
	for _, mode := range []app.Mode{app.Create, app.Update, app.Add} {
		t.Run(string(mode), func(t *testing.T) {
			values := []string{"bad\x00path", "second path"}
			_, issues := app.NewUpdate(base, values[0], values[1])
			if mode == app.Create {
				values = []string{values[0], "Курс", ""}
				_, issues = app.NewCreate(base, values[0], values[1], values[2])
			} else if mode == app.Add {
				_, issues = app.NewAdd(base, values[0], values[1])
			}
			if len(issues) == 0 {
				t.Fatal("fixture не вызвал ошибку конструктора")
			}
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers(values...))), output: &output, baseDir: base}
			start := d.startCreate
			if mode == app.Update {
				start = d.startUpdate
			} else if mode == app.Add {
				start = d.startAdd
			}
			op, err := start()
			if op != nil || err != nil {
				t.Fatalf("показанная предметная ошибка: op=%v err=%v", op, err)
			}
			for _, issue := range issues {
				if !strings.Contains(output.String(), issue.Code) || !strings.Contains(output.String(), issue.Reason) {
					t.Fatalf("не показана причина %v: %s", issue, &output)
				}
			}
		})
	}
}
