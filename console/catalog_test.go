package console

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestConsoleCatalogCourseAndUnitSeparateConsent(t *testing.T) {
	for _, test := range []struct {
		name, input, want string
		unit, accept      bool
	}{
		{"course accepted", "1\n  Курс Ω  \n1\n", "  Курс Ω  ", false, true},
		{"course refused", "1\nНепринятое название\n2\n", "Курс", false, false},
		{"unit accepted", "1\n  Юнит Ω  \n1\n", "  Юнит Ω  ", true, true},
		{"unit cleared", "1\n\n1\n", "", true, true},
		{"unit clear refused", "1\n\n2\n", "Юнит", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			catalogTestSource(t, root, "sources/one.json", "CT03", "Новый урок", nil, nil)
			op, issues := app.NewCreate(root, "sources", "Курс", "Юнит")
			catalogTestOK(t, issues)
			catalogTestOK(t, op.Analyze(catalogTestTime()))
			defer op.Cancel()
			before := op.Snapshot()
			disk := catalogTestDisk(t, root)
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader(test.input)), output: &output, operation: op}
			var err error
			if test.unit {
				err = d.editUnit(before)
			} else {
				err = d.editCourse(before)
			}
			if err != nil {
				t.Fatal(err)
			}
			after := op.Snapshot()
			if test.unit {
				if !strings.Contains(output.String(), "Enter — убрать юнит") {
					t.Fatalf("missing explicit empty-unit explanation: %s", output.String())
				}
				if test.want == "" {
					if after.Catalog.UnitTitle != nil {
						t.Fatal("Enter did not remove unit")
					}
				} else if after.Catalog.UnitTitle == nil || *after.Catalog.UnitTitle != test.want {
					t.Fatalf("unit not preserved literally: %+v", after.Catalog)
				}
			} else if after.Catalog.CourseTitle != test.want {
				t.Fatalf("course = %q", after.Catalog.CourseTitle)
			}
			if !strings.Contains(output.String(), "1. Да") || !strings.Contains(output.String(), "2. Нет") {
				t.Fatal("no separate numeric edit consent")
			}
			if test.accept && after.Token == before.Token {
				t.Fatal("accepted edit did not invalidate old snapshot")
			}
			if !test.accept {
				catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
				if after.Token != before.Token {
					t.Fatal("refusal invalidated accepted state")
				}
			}
			catalogTestFilesEqual(t, before, after)
			catalogTestUnwritten(t, root, disk, op)
			// A closed candidate must not block the next independent proposal.
			probe, issues := op.ProposeCourseTitle(after.Token, "Следующая правка")
			catalogTestOK(t, issues)
			if probe == nil {
				t.Fatal("candidate remained open")
			}
			catalogTestOK(t, op.RejectEdit(probe.Token))
			if test.accept {
				stale, issues := op.ProposeUnitTitle(before.Token, "Старый токен")
				if stale != nil {
					t.Fatal("old snapshot still usable")
				}
				catalogTestCode(t, issues, app.CodeStaleReview)
			}
		})
	}
}

func TestConsoleCatalogLocalCancelDoesNotCallApp(t *testing.T) {
	for _, method := range []string{"course", "unit", "section"} {
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
			// nil operation makes an accidental app call observable as its state diagnostic.
			d := dialog{input: bufio.NewReader(strings.NewReader("2\n")), output: &output}
			var err error
			switch method {
			case "course":
				err = d.editCourse(before)
			case "unit":
				err = d.editUnit(before)
			case "section":
				err = d.createSection(before)
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), app.CodeState) || strings.Contains(output.String(), "1. Да") {
				t.Fatalf("cancel reached proposal/review: %s", output.String())
			}
			catalogTestCatalogEqual(t, before.Catalog, op.Snapshot().Catalog)
			catalogTestFilesEqual(t, before, op.Snapshot())
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}

func TestConsoleCatalogCreateSectionUsesOwnerNormalizedCandidate(t *testing.T) {
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
			disk := catalogTestDisk(t, root)
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader("1\n  Новая\t секция  \n" + answer + "\n")), output: &output, operation: op}
			if err := d.createSection(before); err != nil {
				t.Fatal(err)
			}
			after := op.Snapshot()
			if answer == "1" {
				if !after.Catalog.Sectioned || len(after.Catalog.Sections) != 1 || after.Catalog.Sections[0].Title != "Новая секция" {
					t.Fatalf("wrong normalized section: %+v", after.Catalog)
				}
				if len(after.Blockers) == 0 || after.Actions.ReviewSave {
					t.Fatal("incomplete distribution did not remain blocked")
				}
				for _, item := range after.Catalog.Items {
					if item.Section != nil {
						t.Fatal("section creation moved a lesson automatically")
					}
				}
				if !strings.Contains(output.String(), "Новая секция") {
					t.Fatal("normalized candidate not shown")
				}
			} else {
				catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
			}
			catalogTestFilesEqual(t, before, after)
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}
