package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func catalogTestErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}
	if issue, ok := err.(*diagnostic.Error); ok && issue != nil && issue.Code == code {
		return true
	}
	if group, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range group.Unwrap() {
			if catalogTestErrorCode(child, code) {
				return true
			}
		}
	} else if single, ok := err.(interface{ Unwrap() error }); ok {
		return catalogTestErrorCode(single.Unwrap(), code)
	}
	return false
}

func TestConsoleCatalogReviewNilCandidateShowsAllIssuesWithoutConsent(t *testing.T) {
	root := t.TempDir()
	catalogTestSource(t, root, "sources/one.json", "CT03", "Новый урок", nil, nil)
	op, issues := app.NewCreate(root, "sources", "Курс", "Юнит")
	catalogTestOK(t, issues)
	catalogTestOK(t, op.Analyze(catalogTestTime()))
	defer op.Cancel()
	before := op.Snapshot()
	edit, issues := op.ProposeCourseTitle(before.Token, " \t ")
	if edit != nil || len(issues) == 0 {
		t.Fatal("expected real owner refusal without candidate")
	}
	// A second independent refused owner proposal contributes a separate cause.
	second, other := op.ProposeLessonWeight(before.Token, before.Catalog.Items[0].Entry.FileName, "0")
	if second != nil || len(other) == 0 {
		t.Fatal("expected invalid-weight refusal")
	}
	issues = append(issues, other...)
	disk := catalogTestDisk(t, root)
	var output bytes.Buffer
	input := strings.NewReader("1\n")
	d := dialog{input: bufio.NewReader(input), output: &output, operation: op}
	if err := d.reviewEdit(before, edit, issues); err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		if !strings.Contains(output.String(), issue.Code) || !strings.Contains(output.String(), issue.Reason) {
			t.Fatalf("issue not shown: %v\n%s", issue, output.String())
		}
	}
	if input.Len() != 2 || strings.Contains(output.String(), "1. Да") {
		t.Fatal("nil candidate was treated as partial success")
	}
	after := op.Snapshot()
	catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
	if before.Token != after.Token {
		t.Fatal("owner refusal changed current token")
	}
	catalogTestFilesEqual(t, before, after)
	catalogTestUnwritten(t, root, disk, op)
}

func TestConsoleCatalogReviewAcceptsOrRejectsEditableBlockedCandidate(t *testing.T) {
	for _, answer := range []string{"1", "2"} {
		t.Run(answer, func(t *testing.T) {
			root := t.TempDir()
			archive := catalogTestArchive(t, root, false)
			source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
			op, issues := app.NewAdd(root, archive, source)
			catalogTestOK(t, issues)
			catalogTestOK(t, op.Analyze(catalogTestTime()))
			defer op.Cancel()
			before := op.Snapshot()
			edit, issues := op.ProposeSection(before.Token, "Рабочая секция")
			if edit == nil || len(edit.Blockers) == 0 || len(issues) == 0 {
				t.Fatal("expected real editable blocked candidate")
			}
			disk := catalogTestDisk(t, root)
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader(answer + "\n")), output: &output, operation: op}
			if err := d.reviewEdit(before, edit, issues); err != nil {
				t.Fatal(err)
			}
			after := op.Snapshot()
			for _, issue := range issues {
				if issue != nil && !strings.Contains(output.String(), issue.Code) {
					t.Fatalf("lost issue %v", issue)
				}
			}
			if !strings.Contains(output.String(), "Рабочая секция") || !strings.Contains(output.String(), "1. Да") || !strings.Contains(output.String(), "2. Нет") {
				t.Fatalf("blocked candidate not separately shown/confirmable: %s", output.String())
			}
			if answer == "1" {
				if !after.Catalog.Sectioned || len(after.Catalog.Sections) != 1 || after.Token == before.Token {
					t.Fatal("blocked candidate could not be accepted")
				}
				if !after.Actions.EditSections || after.Actions.ReviewSave || len(after.Blockers) == 0 {
					t.Fatal("accepted blocker lost editability or allowed save")
				}
			} else {
				catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
				if before.Token != after.Token {
					t.Fatal("RejectEdit changed accepted identity")
				}
			}
			if len(op.RejectEdit(edit.Token)) == 0 || len(op.AcceptEdit(edit.Token)) == 0 {
				t.Fatal("closed candidate token remained usable")
			}
			probe, _ := op.ProposeCourseTitle(after.Token, "Проверка закрытия")
			if probe == nil {
				t.Fatal("candidate was left active")
			}
			catalogTestOK(t, op.RejectEdit(probe.Token))
			// External projection storage cannot change accepted or refused state.
			edit.Catalog.CourseTitle = "Подмена projection"
			edit.Catalog.Items[0].Entry.Title = "Подмена записи"
			if len(edit.Changes) > 0 {
				edit.Changes[0].After = json.RawMessage(`"Подмена"`)
			}
			if len(edit.Blockers) > 0 {
				edit.Blockers[0].Reason = "Подмена причины"
			}
			catalogTestCatalogEqual(t, after.Catalog, op.Snapshot().Catalog)
			catalogTestFilesEqual(t, before, after)
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}

func TestConsoleCatalogReviewShowFailureRejectsAndJoinsRejectionCause(t *testing.T) {
	for _, alreadyClosed := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "stale"}[alreadyClosed], func(t *testing.T) {
			root := t.TempDir()
			catalogTestSource(t, root, "sources/one.json", "CT03", "Новый урок", nil, nil)
			op, issues := app.NewCreate(root, "sources", "Курс", "Юнит")
			catalogTestOK(t, issues)
			catalogTestOK(t, op.Analyze(catalogTestTime()))
			defer op.Cancel()
			before := op.Snapshot()
			edit, issues := op.ProposeCourseTitle(before.Token, "Новый курс")
			catalogTestOK(t, issues)
			if edit == nil || len(edit.Changes) == 0 {
				t.Fatal("no real staged candidate")
			}
			if alreadyClosed {
				catalogTestOK(t, op.RejectEdit(edit.Token))
			}
			// The real candidate and token stay owner-controlled; only its independent
			// display DTO is malformed at the preview projection boundary.
			edit.Changes[0].After = json.RawMessage(`{"invalid":`)
			disk := catalogTestDisk(t, root)
			var output bytes.Buffer
			input := strings.NewReader("1\n")
			d := dialog{input: bufio.NewReader(input), output: &output, operation: op}
			err := d.reviewEdit(before, edit, issues)
			if err == nil || !catalogTestErrorCode(err, CodePreview) {
				t.Fatalf("presentation failure missing: %v", err)
			}
			if alreadyClosed && !catalogTestErrorCode(err, app.CodeStaleReview) {
				t.Fatalf("RejectEdit cause lost from joined error: %v", err)
			}
			if input.Len() != 2 || strings.Contains(output.String(), "1. Да") {
				t.Fatal("failed display asked for consent")
			}
			after := op.Snapshot()
			catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
			if before.Token != after.Token {
				t.Fatal("failed display accepted something")
			}
			probe, issues := op.ProposeUnitTitle(after.Token, "Следующий юнит")
			catalogTestOK(t, issues)
			if probe == nil {
				t.Fatal("failed display did not reject candidate")
			}
			catalogTestOK(t, op.RejectEdit(probe.Token))
			catalogTestFilesEqual(t, before, after)
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}
