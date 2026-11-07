package console

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/moduleio"
)

func TestConsoleSaveRefusalClosesBothConsents(t *testing.T) {
	for _, tc := range []struct {
		name    string
		exists  bool
		answers []string
	}{
		{"new-no", false, []string{"2\n"}}, {"existing-no", true, []string{"2\n"}},
		{"overwrite-no", true, []string{"1\n", "2\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := consoleSaveTestCreate(t, tc.exists)
			unchanged := consoleSaveTestUnchanged(t, f.target)
			value := consoleSaveTestPreview(t, f)
			var out bytes.Buffer
			r := &consoleSaveTestReader{lines: tc.answers, before: func(int) {
				if f.op.Snapshot().Actions.Save {
					t.Error("positive consent published before complete answers")
				}
			}}
			d := consoleSaveTestDialog(f.op, r, &out)
			got, err := d.confirmSaving(value)
			if err != nil || got != routePreview {
				t.Fatalf("route=%v err=%v", got, err)
			}
			if f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil {
				t.Error("refusal produced consent/write")
			}
			issues := f.op.ConfirmSave(value.Token, true, true)
			if len(issues) == 0 || issues[0].Code != app.CodeStaleReview {
				t.Fatalf("refused proposal was not closed: %v", issues)
			}
			if tc.answers[0] == "2\n" && strings.Contains(out.String(), "Перезаписать существующий файл?") {
				t.Error("overwrite asked after refusal")
			}
			unchanged()
		})
	}
}

func TestConsoleSaveConsentPromptFailuresCancel(t *testing.T) {
	fault := errors.New("consent input fault")
	for _, tc := range []struct {
		name    string
		answers []string
		cause   error
	}{
		{"first-EOF", nil, nil}, {"first-fragment", []string{"1"}, nil}, {"first-fault", nil, fault},
		{"second-EOF", []string{"1\n"}, nil}, {"second-fragment", []string{"1\n", "1"}, nil}, {"second-fault", []string{"1\n"}, fault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := consoleSaveTestCreate(t, true)
			unchanged := consoleSaveTestUnchanged(t, f.target)
			value := consoleSaveTestPreview(t, f)
			var out bytes.Buffer
			r := &consoleSaveTestReader{lines: tc.answers, err: tc.cause, before: func(int) {
				if f.op.Snapshot().Actions.Save {
					t.Error("partial positive consent")
				}
			}}
			d := consoleSaveTestDialog(f.op, r, &out)
			_, err := d.confirmSaving(value)
			if tc.cause == nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("want EOF, got %v", err)
				}
			} else if !errors.Is(err, fault) || !consoleSaveTestHasCode(err, CodeInput) {
				t.Fatalf("cause/code lost: %v", err)
			}
			s := f.op.Snapshot()
			if s.Actions.Cancel || s.Actions.Save || s.WriteResult != nil {
				t.Fatalf("not cancelled/no-write: %+v", s.Actions)
			}
			unchanged()
		})
	}
}

func TestConsoleSaveSeparateConsentsWriteRealZIP(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "overwrite"}[existing], func(t *testing.T) {
			f := consoleSaveTestCreate(t, existing)
			value := consoleSaveTestPreview(t, f)
			var out bytes.Buffer
			answers := []string{"1\n", "2\n"}
			if existing {
				answers = []string{"1\n", "1\n", "2\n"}
			}
			r := &consoleSaveTestReader{lines: answers}
			r.before = func(index int) {
				if index == 0 || existing && index == 1 {
					if f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil {
						t.Error("write/consent before both answers")
					}
				}
				if existing && index == 1 {
					first := strings.Index(out.String(), "Записать показанный модуль?")
					second := strings.Index(out.String(), "Перезаписать существующий файл?")
					if first < 0 || second <= first || !strings.Contains(out.String()[first:], f.target) {
						t.Fatalf("overwrite path/question missing after first consent: %s", out.String())
					}
				}
			}
			d := consoleSaveTestDialog(f.op, r, &out)
			if err := d.showSavePreview(value); err != nil {
				t.Fatal(err)
			}
			got, err := d.confirmSaving(value)
			if err != nil || got != routeExit {
				t.Fatalf("route=%v err=%v; output=%s", got, err, out.String())
			}
			if strings.Contains(out.String(), "Перезаписать существующий файл?") != existing {
				t.Error("overwrite consent does not match Exists")
			}
			consoleSaveTestIssues(t, moduleio.Verify(f.target))
			loaded, issues := moduleio.Read(f.target)
			consoleSaveTestIssues(t, issues)
			if loaded == nil || len(loaded.Files) != 1 || loaded.Files[0].Lesson().Document.ID != "BA82" {
				t.Fatal("not the real planned module")
			}
			if s := f.op.Snapshot(); s.WriteResult == nil || !s.WriteResult.Installed || s.Actions.Save {
				t.Fatalf("result=%+v", s.WriteResult)
			}
			unchanged := consoleSaveTestUnchanged(t, f.target)
			if _, issues := f.op.Save(time.Now()); len(issues) == 0 {
				t.Error("second Save accepted without new review")
			}
			unchanged()
		})
	}
}

func TestConsoleSaveUsesOriginalPreviewTokenAfterPlanChanges(t *testing.T) {
	f := consoleSaveTestCreate(t, true)
	unchanged := consoleSaveTestUnchanged(t, f.target)
	value := consoleSaveTestPreview(t, f)
	var out bytes.Buffer
	r := &consoleSaveTestReader{lines: []string{"1\n", "1\n"}}
	r.before = func(index int) {
		if index == 1 {
			edit, issues := f.op.ProposeCourseTitle(f.op.Snapshot().Token, "Изменённый курс")
			consoleSaveTestIssues(t, issues)
			if edit == nil {
				t.Fatal("no real candidate")
			}
			consoleSaveTestIssues(t, f.op.AcceptEdit(edit.Token))
		}
	}
	d := consoleSaveTestDialog(f.op, r, &out)
	got, err := d.confirmSaving(value)
	if err != nil || got != routePreview {
		t.Fatalf("route=%v err=%v", got, err)
	}
	if !strings.Contains(out.String(), app.CodeStaleReview) || f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil {
		t.Fatalf("stale preview bypassed: %s", out.String())
	}
	unchanged()
}

func TestConsoleSaveRejectsChangedTargetBetweenAnswers(t *testing.T) {
	f := consoleSaveTestCreate(t, true)
	value := consoleSaveTestPreview(t, f)
	var out bytes.Buffer
	var unchanged func()
	r := &consoleSaveTestReader{lines: []string{"1\n", "1\n"}, before: func(index int) {
		if index == 1 {
			if err := os.WriteFile(f.target, []byte("externally changed target, longer"), 0640); err != nil {
				t.Fatal(err)
			}
			unchanged = consoleSaveTestUnchanged(t, f.target)
		}
	}}
	got, err := consoleSaveTestDialog(f.op, r, &out).confirmSaving(value)
	if err != nil || got != routePreview {
		t.Fatalf("route=%v err=%v", got, err)
	}
	if unchanged == nil {
		t.Fatal("second independent answer was not requested")
	}
	unchanged()
	if !strings.Contains(out.String(), app.CodeTargetChanged) || f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil {
		t.Fatalf("changed target accepted: %s", out.String())
	}
}

func TestConsoleSaveRejectsOlderProposalAfterNewReview(t *testing.T) {
	f := consoleSaveTestCreate(t, false)
	unchanged := consoleSaveTestUnchanged(t, f.target)
	old := consoleSaveTestPreview(t, f)
	newer, issues := f.op.ReviewSave(f.op.Snapshot().Token)
	consoleSaveTestIssues(t, issues)
	if newer == nil || newer.Token == old.Token {
		t.Fatal("fixture did not create distinct proposal")
	}
	var out bytes.Buffer
	got, err := consoleSaveTestDialog(f.op, strings.NewReader("1\n"), &out).confirmSaving(old)
	if err != nil || got != routePreview {
		t.Fatalf("route=%v err=%v", got, err)
	}
	if !strings.Contains(out.String(), app.CodeStaleReview) || f.op.Snapshot().Actions.Save {
		t.Errorf("old token refreshed/bypassed: %s", out.String())
	}
	unchanged()
}

func TestConsoleSaveConsentOnlyAcceptsNumericMenuAnswers(t *testing.T) {
	f := consoleSaveTestCreate(t, true)
	unchanged := consoleSaveTestUnchanged(t, f.target)
	value := consoleSaveTestPreview(t, f)
	var out bytes.Buffer
	r := &consoleSaveTestReader{lines: []string{"да\n", "yes\n", "0\n", "3\n", "\n", "2\n"}}
	r.before = func(int) {
		if f.op.Snapshot().Actions.Save || f.op.Snapshot().WriteResult != nil {
			t.Error("invalid answer produced consent/write")
		}
	}
	got, err := consoleSaveTestDialog(f.op, r, &out).confirmSaving(value)
	if err != nil || got != routePreview || r.index != 6 {
		t.Fatalf("route=%v err=%v consumed=%d", got, err, r.index)
	}
	if strings.Count(out.String(), "Записать показанный модуль?") != 6 || strings.Contains(out.String(), "Перезаписать существующий файл?") {
		t.Fatalf("invalid input changed confirmation menu: %s", out.String())
	}
	unchanged()
}
