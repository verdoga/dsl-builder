package console

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestConsoleCatalogMoveEveryOldNewAndUnplacedLesson(t *testing.T) {
	for selected := 0; selected < 3; selected++ {
		for _, answer := range []string{"1", "2"} {
			t.Run(fmt.Sprintf("item%d-answer%s", selected, answer), func(t *testing.T) {
				root := t.TempDir()
				archive := catalogTestArchive(t, root, true)
				source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
				op, issues := app.NewAdd(root, archive, source)
				catalogTestOK(t, issues)
				op.Analyze(catalogTestTime()) // The new unplaced lesson is an editable blocker.
				defer op.Cancel()
				before := op.Snapshot()
				if !before.Actions.EditSections || len(before.Catalog.Items) != 3 {
					t.Fatal("missing editable full composition")
				}
				item := before.Catalog.Items[selected]
				destination := len(before.Catalog.Sections) + 1 // Explicit «Без секции».
				var want *string
				if item.Section == nil {
					destination = 2
					want = catalogTestString(before.Catalog.Sections[1].Title)
				}
				disk := catalogTestDisk(t, root)
				var output bytes.Buffer
				d := dialog{input: bufio.NewReader(strings.NewReader(fmt.Sprintf("%d\n%d\n%s\n", selected+1, destination, answer))), output: &output, operation: op}
				if err := d.moveLesson(before); err != nil {
					t.Fatal(err)
				}
				after := op.Snapshot()
				if answer == "2" {
					catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
				} else {
					actual := catalogTestItemSection(t, after, item.Entry.FileName)
					if (actual == nil) != (want == nil) || actual != nil && *actual != *want {
						t.Fatalf("wrong exact file/destination %s", item.Entry.FileName)
					}
					if after.Token == before.Token {
						t.Fatal("explicit nil section treated as cancel")
					}
					for _, old := range before.Catalog.Items {
						if old.Entry.FileName == item.Entry.FileName {
							continue
						}
						current := catalogTestItemSection(t, after, old.Entry.FileName)
						if (old.Section == nil) != (current == nil) || current != nil && *old.Section != *current {
							t.Fatalf("other lesson moved: %s", old.Entry.FileName)
						}
					}
				}
				for _, old := range before.Catalog.Items {
					if !strings.Contains(output.String(), old.Entry.FileName) || !strings.Contains(output.String(), old.Entry.ID) {
						t.Fatalf("lesson omitted from choice: %s", old.Entry.FileName)
					}
				}
				for _, section := range before.Catalog.Sections {
					if !strings.Contains(output.String(), section.Title) {
						t.Fatalf("section omitted: %q", section.Title)
					}
				}
				catalogTestFilesEqual(t, before, after)
				catalogTestUnwritten(t, root, disk, op)
				probe, issues := op.ProposeUnitTitle(after.Token, "Следующий юнит")
				if probe == nil {
					t.Fatal("move candidate remained active")
				}
				wantProbe := catalog.Clone(after.Catalog)
				wantProbe.UnitTitle = catalogTestString("Следующий юнит")
				unassigned := 1
				if answer == "1" {
					unassigned = 2
					if item.Section == nil {
						unassigned = 0
					}
				}
				catalogTestMoveProbe(t, after, probe, issues, wantProbe, unassigned)
				catalogTestOK(t, op.RejectEdit(probe.Token))
				if op.Snapshot().Token != after.Token {
					t.Fatal("rejecting follow-up changed accepted token")
				}
				catalogTestCatalogEqual(t, after.Catalog, op.Snapshot().Catalog)
				catalogTestFilesEqual(t, after, op.Snapshot())
				catalogTestUnwritten(t, root, disk, op)
			})
		}
	}
}

func TestConsoleCatalogMoveCancelDiscardsWholeSelection(t *testing.T) {
	for _, second := range []bool{false, true} {
		t.Run(fmt.Sprint(second), func(t *testing.T) {
			root := t.TempDir()
			archive := catalogTestArchive(t, root, true)
			source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
			op, issues := app.NewAdd(root, archive, source)
			catalogTestOK(t, issues)
			op.Analyze(catalogTestTime())
			defer op.Cancel()
			before := op.Snapshot()
			input := fmt.Sprintf("%d\n", len(before.Catalog.Items)+1)
			if second {
				input = fmt.Sprintf("1\n%d\n", len(before.Catalog.Sections)+2)
			}
			disk := catalogTestDisk(t, root)
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader(input)), output: &output, operation: op}
			if err := d.moveLesson(before); err != nil {
				t.Fatal(err)
			}
			after := op.Snapshot()
			catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
			if before.Token != after.Token || strings.Contains(output.String(), "1. Да") {
				t.Fatal("cancel applied/reviewed a partial group")
			}
			probe, issues := op.ProposeCourseTitle(before.Token, "Проверка закрытой группы")
			if probe == nil {
				t.Fatal("local cancel left an active proposal")
			}
			wantProbe := catalog.Clone(after.Catalog)
			wantProbe.CourseTitle = "Проверка закрытой группы"
			catalogTestMoveProbe(t, after, probe, issues, wantProbe, 1)
			catalogTestOK(t, op.RejectEdit(probe.Token))
			if op.Snapshot().Token != after.Token {
				t.Fatal("rejecting follow-up changed accepted token")
			}
			catalogTestCatalogEqual(t, after.Catalog, op.Snapshot().Catalog)
			catalogTestFilesEqual(t, before, after)
			catalogTestFilesEqual(t, after, op.Snapshot())
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}

// catalogTestMoveProbe проверяет новую правку названия на текущем размещении.
// В этих fixtures блокируют только смешанный состав и нераспределённые уроки;
// перенос последнего нераспределённого урока устраняет обе причины.
func catalogTestMoveProbe(t *testing.T, accepted app.Snapshot, probe *app.EditPreview, issues []*diagnostic.Error, want *catalog.State, unassignedCount int) {
	t.Helper()
	if probe.Catalog == nil || probe.Token == (app.ReviewToken{}) || probe.Token == accepted.Token {
		t.Fatal("follow-up proposal has no independent candidate/token")
	}
	catalogTestCatalogEqual(t, want, probe.Catalog)
	var all, unassigned []diagnostic.Context
	var firstAssigned diagnostic.Context
	for _, item := range accepted.Catalog.Items {
		all = append(all, item.Context)
		if item.Section == nil {
			unassigned = append(unassigned, item.Context)
		} else if firstAssigned == (diagnostic.Context{}) {
			firstAssigned = item.Context
		}
	}
	if !accepted.Catalog.Sectioned || len(unassigned) != unassignedCount {
		t.Fatalf("wrong distribution after move/cancel: unassigned=%d, want %d", len(unassigned), unassignedCount)
	}
	wantReasons := map[string][]diagnostic.Context{}
	wantContext := map[string]diagnostic.Context{}
	if unassignedCount != 0 {
		wantReasons[catalog.CodeMixedSections] = all
		wantContext[catalog.CodeMixedSections] = firstAssigned
		wantReasons[catalog.CodeUnassigned] = unassigned
		wantContext[catalog.CodeUnassigned] = unassigned[0]
	}
	for _, actual := range [][]*diagnostic.Error{issues, probe.Blockers} {
		if len(actual) != len(wantReasons) {
			t.Fatalf("follow-up reason count = %d, want %d: %v", len(actual), len(wantReasons), actual)
		}
		seen := map[string]bool{}
		for _, issue := range actual {
			if issue == nil {
				t.Fatal("nil follow-up diagnostic")
			}
			related, exists := wantReasons[issue.Code]
			if !exists || seen[issue.Code] || issue.Reason == "" || issue.Context != wantContext[issue.Code] || !slices.Equal(issue.Related, related) || len(issue.Details) != 0 || issue.Cause != nil {
				t.Fatalf("unexpected follow-up diagnostic: %+v", issue)
			}
			seen[issue.Code] = true
		}
	}
}

func TestConsoleCatalogFlatPlacementRequiresSeparateExplicitAcceptance(t *testing.T) {
	for _, answer := range []string{"1", "2"} {
		t.Run(answer, func(t *testing.T) {
			root := t.TempDir()
			archive := catalogTestArchive(t, root, true)
			source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
			op, issues := app.NewAdd(root, archive, source)
			catalogTestOK(t, issues)
			op.Analyze(catalogTestTime())
			defer op.Cancel()
			for _, item := range op.Snapshot().Catalog.Items {
				if item.Section == nil {
					continue
				}
				edit, _ := op.ProposeMove(op.Snapshot().Token, item.Entry.FileName, nil)
				if edit == nil {
					t.Fatal("owner refused staged move to unplaced")
				}
				op.AcceptEdit(edit.Token)
			}
			before := op.Snapshot()
			if !before.Catalog.Sectioned {
				t.Fatal("individual moves automatically completed flat placement")
			}
			disk := catalogTestDisk(t, root)
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader(answer + "\n")), output: &output, operation: op}
			if err := d.useFlatPlacement(before); err != nil {
				t.Fatal(err)
			}
			after := op.Snapshot()
			if answer == "1" {
				if after.Catalog.Sectioned || len(after.Catalog.Sections) != 0 || !after.ManifestReady {
					t.Fatal("explicit accepted flat placement not completed")
				}
			} else {
				catalogTestCatalogEqual(t, before.Catalog, after.Catalog)
			}
			for _, item := range after.Catalog.Items {
				if item.Section != nil {
					t.Fatal("flat completion changed individual placement")
				}
			}
			catalogTestFilesEqual(t, before, after)
			catalogTestUnwritten(t, root, disk, op)
		})
	}
}

func TestConsoleCatalogFlatDoesNotTransferOccupiedSections(t *testing.T) {
	root := t.TempDir()
	archive := catalogTestArchive(t, root, true)
	source := catalogTestSource(t, root, "source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
	op, issues := app.NewAdd(root, archive, source)
	catalogTestOK(t, issues)
	op.Analyze(catalogTestTime())
	defer op.Cancel()
	before := op.Snapshot()
	disk := catalogTestDisk(t, root)
	var output bytes.Buffer
	input := strings.NewReader("1\n")
	d := dialog{input: bufio.NewReader(input), output: &output, operation: op}
	if err := d.useFlatPlacement(before); err != nil {
		t.Fatal(err)
	}
	catalogTestCatalogEqual(t, before.Catalog, op.Snapshot().Catalog)
	if strings.Contains(output.String(), "1. Да") || input.Len() != 2 {
		t.Fatal("invalid flat placement requested consent")
	}
	catalogTestFilesEqual(t, before, op.Snapshot())
	catalogTestUnwritten(t, root, disk, op)
}
