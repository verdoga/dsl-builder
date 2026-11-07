package console

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/safewrite"
)

func TestResultWriteOutcomeDistinguishesInstalledFromFullSuccess(t *testing.T) {
	cleanup := &diagnostic.Error{Code: "safewrite.cleanup", Reason: "Не удалось удалить резерву"}
	var typedNil *diagnostic.Error
	for _, test := range []struct {
		name      string
		installed bool
		issues    []*diagnostic.Error
		success   bool
	}{
		{"installed_without_issues", true, nil, true},
		{"nil_issues_are_absent", true, []*diagnostic.Error{nil, typedNil}, true},
		{"installed_with_cleanup_error", true, []*diagnostic.Error{cleanup}, false},
		{"not_installed_with_error", false, []*diagnostic.Error{cleanup}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			result := &safewrite.Result{Installed: test.installed, TargetPath: "/Каталог результата/архив.zip"}
			snapshot := app.Snapshot{WriteResult: result}
			d := resultTestDialog(t, strings.NewReader(""), &output, nil)
			if err := d.showWriteResult(snapshot, test.issues); err != nil {
				t.Fatal(err)
			}
			text := strings.ToLower(output.String())
			if fullSuccess := strings.Contains(text, "успешно сохран"); fullSuccess != test.success {
				t.Errorf("full success=%t, want %t: %s", fullSuccess, test.success, output.String())
			}
			if !strings.Contains(output.String(), result.TargetPath) {
				t.Error("full target path absent")
			}
			if len(test.issues) != 0 && test.issues[0] != nil {
				if !strings.Contains(output.String(), cleanup.Code) || !strings.Contains(output.String(), cleanup.Reason) {
					t.Error("cleanup issue omitted")
				}
				if strings.Contains(text, "не изменён") || strings.Contains(text, "без изменений") {
					t.Error("write failure reported as unchanged")
				}
				if test.installed && !strings.Contains(text, "установ") && !strings.Contains(text, "запис") {
					t.Error("installed result with issues was not identified as installed")
				}
			}
			if result.Installed != test.installed || snapshot.WriteResult != result {
				t.Error("projection changed source result")
			}
		})
	}
}

func TestResultWriteErrorsShowEveryKnownPathSeparately(t *testing.T) {
	for _, test := range []struct {
		name   string
		result safewrite.Result
	}{
		{"target_only", safewrite.Result{TargetPath: "/Цель/урок.zip"}},
		{"original_only", safewrite.Result{OriginalPath: "/Резерва/исходник.zip"}},
		{"temporary_only", safewrite.Result{TemporaryPath: "/Временный/кандидат.zip"}},
		{"all_paths", safewrite.Result{TargetPath: "/Цель/目标.zip", OriginalPath: "/Резерва/😀.zip", TemporaryPath: "/Временный/тест.zip"}},
		{"long_unicode", safewrite.Result{TargetPath: "/Длинный путь/" + strings.Repeat("Юникод😀 ", 12000) + "конец.zip"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			d := resultTestDialog(t, strings.NewReader(""), &output, nil)
			issue := &diagnostic.Error{Code: "safewrite.replace", Reason: "Замена не завершена"}
			before := test.result
			if err := d.showWriteResult(app.Snapshot{WriteResult: &test.result}, []*diagnostic.Error{issue}); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{test.result.TargetPath, test.result.OriginalPath, test.result.TemporaryPath} {
				if path == "" {
					continue
				}
				if !strings.Contains(output.String(), path) {
					t.Errorf("full known path absent, length=%d", len(path))
				}
				for _, line := range strings.Split(output.String(), "\n") {
					if strings.Contains(line, path) {
						for _, other := range []string{test.result.TargetPath, test.result.OriginalPath, test.result.TemporaryPath} {
							if other != "" && other != path && strings.Contains(line, other) {
								t.Error("distinct result paths share one unlabeled line")
							}
						}
					}
				}
			}
			if before != test.result {
				t.Error("result projection mutated DTO")
			}
		})
	}
}

func TestResultWritePathsEscapeControlsAndKeepUnicode(t *testing.T) {
	result := safewrite.Result{
		TargetPath:    "/Цель/до\nпосле\t😀\x1b[31m.zip",
		OriginalPath:  "/Исходник/до\rпосле\x00.zip",
		TemporaryPath: "/Кандидат/до\aпосле\b.zip",
	}
	var output bytes.Buffer
	d := resultTestDialog(t, strings.NewReader(""), &output, nil)
	if err := d.showWriteResult(app.Snapshot{WriteResult: &result}, []*diagnostic.Error{{Code: "safewrite.cleanup", Reason: "Остались файлы"}}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, marker := range []string{"Цель", "Исходник", "Кандидат", "до", "после", "😀", "[31m.zip"} {
		if !strings.Contains(text, marker) {
			t.Errorf("escaped path lost %q", marker)
		}
	}
	if strings.ContainsAny(text, "\t\r\x00\x1b\a\b") || strings.Contains(text, "до\nпосле") {
		t.Error("external path contains active controls")
	}
}

func TestResultWriteIssuesPreserveAllCausesOnOutputFailure(t *testing.T) {
	one := &diagnostic.Error{Code: "safewrite.replace", Reason: "Ошибка замены", Cause: errors.New("replace cause")}
	two := &diagnostic.Error{Code: "safewrite.cleanup", Reason: "Ошибка очистки", Cause: errors.New("cleanup cause")}
	var typedNil *diagnostic.Error
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete_output", true: "failed_output"}[fail], func(t *testing.T) {
			var output bytes.Buffer
			writes := 0
			fault := errors.New("output unavailable")
			writer := resultTestWriterFunc(func(p []byte) (int, error) {
				writes++
				if fail {
					return 0, fault
				}
				return output.Write(p)
			})
			d := resultTestDialog(t, strings.NewReader(""), writer, nil)
			err := d.showWriteResult(app.Snapshot{WriteResult: &safewrite.Result{TargetPath: "/target.zip", Installed: true}}, []*diagnostic.Error{nil, one, typedNil, two})
			if fail {
				if !errors.Is(err, fault) || !errors.Is(err, one) || !errors.Is(err, two) || !errors.Is(err, two.Cause) {
					t.Fatalf("output failure lost original issues: %v", err)
				}
				if writes != 1 {
					t.Errorf("broken writer called %d times", writes)
				}
			} else if err != nil || !strings.Contains(output.String(), one.Reason) || !strings.Contains(output.String(), two.Reason) {
				t.Errorf("issues not all presented: %v; %s", err, output.String())
			}
		})
	}
}

func TestResultNilWriteResultDoesNotConsumeSaveConsent(t *testing.T) {
	op, source, directory := resultTestCreate(t)
	before := resultTestBytes(t, source)
	path := resultTestConsent(t, op, directory)
	var output bytes.Buffer
	d := resultTestDialog(t, strings.NewReader(""), &output, op)
	if err := d.showWriteResult(op.Snapshot(), nil); err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(output.String())
	for _, falseOutcome := range []string{"успешно сохран", "архив не измен", "без изменений"} {
		if strings.Contains(text, falseOutcome) {
			t.Errorf("nil result fabricated file outcome: %s", output.String())
		}
	}
	if !op.Snapshot().Actions.Save || op.Snapshot().WriteResult != nil {
		t.Error("nil-result projection attempted Save or consumed consent")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nil-result projection created target: %v", err)
	}
	resultTestUnchanged(t, source, before)
}

func TestResultNilWriteResultStillShowsEveryIssue(t *testing.T) {
	var output bytes.Buffer
	d := resultTestDialog(t, strings.NewReader(""), &output, nil)
	issues := []*diagnostic.Error{nil,
		{Code: "app.state", Reason: "Попытка записи не начиналась"},
		{Code: "app.destination", Reason: "Цель требует нового просмотра"},
	}
	if err := d.showWriteResult(app.Snapshot{}, issues); err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		if issue != nil && (!strings.Contains(output.String(), issue.Code) || !strings.Contains(output.String(), issue.Reason)) {
			t.Errorf("nil-result projection lost issue: %s", issue.Code)
		}
	}
	if strings.Contains(strings.ToLower(output.String()), "успешно сохран") {
		t.Error("nil result announced successful file outcome")
	}
}

func TestResultWriteShortOutputStopsImmediately(t *testing.T) {
	writes := 0
	d := resultTestDialog(t, strings.NewReader(""), resultTestWriterFunc(func(p []byte) (int, error) {
		writes++
		return 0, nil
	}), nil)
	err := d.showWriteResult(app.Snapshot{WriteResult: &safewrite.Result{Installed: true, TargetPath: "/результат.zip"}}, nil)
	if !errors.Is(err, io.ErrShortWrite) || writes != 1 {
		t.Fatalf("short write: error=%v, writes=%d", err, writes)
	}
}
