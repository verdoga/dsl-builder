package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

type catalogTestFaultReader struct {
	cause error
	calls int
}

func (r *catalogTestFaultReader) Read([]byte) (int, error) { r.calls++; return 0, r.cause }

type catalogTestFaultWriter struct {
	cause error
	short bool
	calls int
}

func (w *catalogTestFaultWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.short {
		return len(p) - 1, nil
	}
	return 0, w.cause
}

func TestConsoleCatalogEveryReviewBoundaryCancelsWholeOperationOnInputLoss(t *testing.T) {
	for _, method := range []string{"course", "unit", "section", "move", "flat", "review"} {
		for _, failure := range []string{"EOF", "partial EOF", "read error"} {
			t.Run(method+"/"+failure, func(t *testing.T) {
				root := t.TempDir()
				archive := catalogTestArchive(t, root, true)
				source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
				op, issues := app.NewAdd(root, archive, source)
				catalogTestOK(t, issues)
				op.Analyze(catalogTestTime())
				defer op.Cancel()
				if method == "flat" {
					for _, item := range op.Snapshot().Catalog.Items {
						if item.Section == nil {
							continue
						}
						proposal, _ := op.ProposeMove(op.Snapshot().Token, item.Entry.FileName, nil)
						if proposal == nil {
							t.Fatal("missing owner move")
						}
						op.AcceptEdit(proposal.Token)
					}
				}
				before := op.Snapshot()
				var edit *app.EditPreview
				var editIssues []*diagnostic.Error
				prefix := ""
				switch method {
				case "course":
					prefix = "1\nНовый курс\n"
				case "unit":
					prefix = "1\n\n"
				case "section":
					prefix = "1\nНовая секция\n"
				case "move":
					prefix = "1\n2\n"
				case "review":
					edit, editIssues = op.ProposeCourseTitle(before.Token, "Новый курс")
				}
				cause := errors.New("catalog reader disconnected")
				var input io.Reader = strings.NewReader(prefix)
				var fault *catalogTestFaultReader
				switch failure {
				case "partial EOF":
					input = strings.NewReader(prefix + "1")
				case "read error":
					fault = &catalogTestFaultReader{cause: cause}
					input = io.MultiReader(strings.NewReader(prefix), fault)
				}
				disk := catalogTestDisk(t, root)
				var output bytes.Buffer
				d := dialog{input: bufio.NewReader(input), output: &output, operation: op}
				var err error
				switch method {
				case "course":
					err = d.editCourse(before)
				case "unit":
					err = d.editUnit(before)
				case "section":
					err = d.createSection(before)
				case "move":
					err = d.moveLesson(before)
				case "flat":
					err = d.useFlatPlacement(before)
				case "review":
					err = d.reviewEdit(before, edit, editIssues)
				}
				if failure == "read error" {
					if !errors.Is(err, cause) || !catalogTestErrorCode(err, CodeInput) || fault.calls != 1 {
						t.Fatalf("input cause/retry lost: %v calls=%d", err, fault.calls)
					}
				} else if !errors.Is(err, io.EOF) {
					t.Fatalf("EOF not propagated: %v", err)
				}
				after := op.Snapshot()
				catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
				if after.Actions.EditCatalog || after.Actions.EditSections || after.Actions.ReviewSave {
					t.Fatal("whole operation remains active after interrupted review")
				}
				proposal, issues := op.ProposeCourseTitle(before.Token, "Недействительный токен")
				if proposal != nil || len(issues) == 0 {
					t.Fatal("current token survived cancelled review")
				}
				catalogTestFilesEqual(t, before, after)
				catalogTestUnwritten(t, root, disk, op)
			})
		}
	}
}

func TestConsoleCatalogReadFailureBeforeProposalPropagates(t *testing.T) {
	for _, method := range []string{"course", "unit", "section", "move"} {
		t.Run(method, func(t *testing.T) {
			root := t.TempDir()
			archive := catalogTestArchive(t, root, false)
			source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
			op, issues := app.NewAdd(root, archive, source)
			catalogTestOK(t, issues)
			catalogTestOK(t, op.Analyze(catalogTestTime()))
			defer op.Cancel() // Run owns cancellation before reviewEdit has been entered.
			before := op.Snapshot()
			disk := catalogTestDisk(t, root)
			cause := errors.New("catalog initial reader failure")
			fault := &catalogTestFaultReader{cause: cause}
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(fault), output: &output, operation: op}
			var err error
			switch method {
			case "course":
				err = d.editCourse(before)
			case "unit":
				err = d.editUnit(before)
			case "section":
				err = d.createSection(before)
			case "move":
				err = d.moveLesson(before)
			}
			if !errors.Is(err, cause) || !catalogTestErrorCode(err, CodeInput) || fault.calls != 1 {
				t.Fatalf("input failure not propagated: %v", err)
			}
			catalogTestCatalogEqual(t, before.Catalog, op.Snapshot().Catalog)
			catalogTestFilesEqual(t, before, op.Snapshot())
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}

func TestConsoleCatalogReviewOutputFailureCancelsAndPreservesIssues(t *testing.T) {
	for _, phase := range []string{"issues", "candidate", "short"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			catalogTestSource(t, root, "sources/one.json", "CT03", "Новый урок", nil, nil)
			op, issues := app.NewCreate(root, "sources", "Курс", "Юнит")
			catalogTestOK(t, issues)
			catalogTestOK(t, op.Analyze(catalogTestTime()))
			defer op.Cancel()
			before := op.Snapshot()
			edit, issues := op.ProposeCourseTitle(before.Token, "Новый курс")
			catalogTestOK(t, issues)
			if edit == nil {
				t.Fatal("missing real candidate")
			}
			cause := errors.New("catalog writer disconnected")
			writer := &catalogTestFaultWriter{cause: cause, short: phase == "short"}
			var original []*diagnostic.Error
			if phase == "issues" {
				// These real owner refusals remain separate causes even when output breaks.
				_, original = op.ProposeCourseTitle(before.Token, "Занятое предложение")
				if len(original) == 0 {
					t.Fatal("owner did not reject second active proposal")
				}
			}
			disk := catalogTestDisk(t, root)
			input := strings.NewReader("1\n")
			d := dialog{input: bufio.NewReader(input), output: writer, operation: op}
			err := d.reviewEdit(before, edit, original)
			expected := cause
			if phase == "short" {
				expected = io.ErrShortWrite
			}
			if !errors.Is(err, expected) || !catalogTestErrorCode(err, CodeOutput) {
				t.Fatalf("output cause not preserved: %v", err)
			}
			for _, issue := range original {
				if !errors.Is(err, issue) {
					t.Fatalf("original issue lost: %v", issue)
				}
			}
			if writer.calls != 1 || input.Len() != 2 {
				t.Fatalf("continued after broken output: writes=%d unread=%d", writer.calls, input.Len())
			}
			after := op.Snapshot()
			catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
			if after.Actions.EditCatalog || after.Actions.ReviewSave {
				t.Fatal("operation not cancelled")
			}
			candidate, issues := op.ProposeUnitTitle(before.Token, "Невозможно")
			if candidate != nil || len(issues) == 0 {
				t.Fatal("cancelled token still valid")
			}
			catalogTestFilesEqual(t, before, after)
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}
