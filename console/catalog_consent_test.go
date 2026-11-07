package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestConsoleCatalogRequiredCourseAndSectionDoNotSubmitEmptyInput(t *testing.T) {
	for _, method := range []string{"course", "section"} {
		t.Run(method, func(t *testing.T) {
			root := t.TempDir()
			archive := catalogTestArchive(t, root, false)
			source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
			op, issues := app.NewAdd(root, archive, source)
			catalogTestOK(t, issues)
			catalogTestOK(t, op.Analyze(catalogTestTime()))
			defer op.Cancel()
			before := op.Snapshot()
			disk := catalogTestDisk(t, root)
			var output bytes.Buffer
			// EOF follows an empty completed line. Required input must stay local;
			// nil receiver would expose an accidental proposal as app.CodeState.
			d := dialog{input: bufio.NewReader(strings.NewReader("1\n\n")), output: &output}
			var err error
			if method == "course" {
				err = d.editCourse(before)
			} else {
				err = d.createSection(before)
			}
			if !errors.Is(err, io.EOF) {
				t.Fatalf("required empty value was submitted: %v\n%s", err, output.String())
			}
			if strings.Contains(output.String(), app.CodeState) || strings.Contains(output.String(), "1. Да") {
				t.Fatal("empty required value reached app/review")
			}
			catalogTestCatalogEqual(t, before.Catalog, op.Snapshot().Catalog)
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}

func TestConsoleCatalogEditConsentRevokesPreviousSaveConsent(t *testing.T) {
	for _, answer := range []string{"1", "2"} {
		t.Run(answer, func(t *testing.T) {
			root := t.TempDir()
			archive := catalogTestArchive(t, root, false)
			source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
			op, issues := app.NewAdd(root, archive, source)
			catalogTestOK(t, issues)
			catalogTestOK(t, op.Analyze(catalogTestTime()))
			defer op.Cancel()
			saving, issues := op.ReviewSave(op.Snapshot().Token)
			catalogTestOK(t, issues)
			if saving == nil {
				t.Fatal("missing real SavePreview")
			}
			catalogTestOK(t, op.ConfirmSave(saving.Token, true, true))
			before := op.Snapshot()
			if !before.Actions.Save {
				t.Fatal("fixture did not have actual write consent")
			}
			disk := catalogTestDisk(t, root)
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader("1\nНовый курс\n" + answer + "\n")), output: &output, operation: op}
			if err := d.editCourse(before); err != nil {
				t.Fatal(err)
			}
			after := op.Snapshot()
			if after.Actions.Save || after.WriteResult != nil {
				t.Fatal("edit consent retained/granted write consent")
			}
			if len(op.ConfirmSave(saving.Token, true, true)) == 0 {
				t.Fatal("previous SavePreview token survived separate edit")
			}
			if answer == "2" {
				catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
				if before.Token != after.Token {
					t.Fatal("rejected edit changed accepted token")
				}
			} else {
				if after.Catalog.CourseTitle != "Новый курс" || before.Catalog.CourseTitle != "Курс" {
					t.Fatal("accepted/old snapshot storage not independent")
				}
				proposal, issues := op.ProposeCourseTitle(before.Token, "Устаревший")
				if proposal != nil {
					t.Fatal("old snapshot accepted another change")
				}
				catalogTestCode(t, issues, app.CodeStaleReview)
			}
			catalogTestFilesEqual(t, before, after)
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}

func TestConsoleCatalogUpdateEditPreservesPendingContentDecisions(t *testing.T) {
	root := t.TempDir()
	archive := catalogTestArchive(t, root, false)
	source := catalogTestSource(t, root, "source.json", "CT01", "Старый урок один", nil, nil)
	op, issues := app.NewUpdate(root, archive, source)
	catalogTestOK(t, issues)
	catalogTestOK(t, op.Analyze(catalogTestTime()))
	defer op.Cancel()
	before := op.Snapshot()
	if !before.Actions.EditSections || !before.Actions.DecideFields || before.RevisionReady || len(before.PlacementProposals) != 0 {
		t.Fatal("Update fixture did not preserve separate content decisions")
	}
	planBefore, err := json.Marshal(before.Plan)
	if err != nil {
		t.Fatal(err)
	}
	disk := catalogTestDisk(t, root)
	var output bytes.Buffer
	d := dialog{input: bufio.NewReader(strings.NewReader("1\nРабочая секция\n1\n")), output: &output, operation: op}
	if err := d.createSection(before); err != nil {
		t.Fatal(err)
	}
	after := op.Snapshot()
	if !after.Catalog.Sectioned || !after.Actions.DecideFields || after.RevisionReady || after.Actions.ReviewSave {
		t.Fatal("catalog edit answered content decisions or prepared revision")
	}
	planAfter, err := json.Marshal(after.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(planBefore, planAfter) {
		t.Fatal("catalog edit changed comparison plan")
	}
	for _, answered := range after.FieldAnswered {
		if answered {
			t.Fatal("catalog consent answered a field")
		}
	}
	catalogTestFilesEqual(t, before, after)
	catalogTestUnwritten(t, root, disk, op)
}
