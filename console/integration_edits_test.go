package console

import (
	"bufio"
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/preview"
)

func TestConsoleIntegrationWholeWeightsGroupCancelAndRawPrecision(t *testing.T) {
	for _, cancel := range []int{0, 1, 2} {
		t.Run(string(rune('A'+cancel)), func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, "json")
			integrationTestSource(t, dir, "A41", "First", "Text", nil, integrationTestPointer("1"))
			integrationTestSource(t, dir, "B41", "Second", "Text", nil, integrationTestPointer("2"))
			op := integrationTestCreateOperation(t, base, dir, "Курс", "")
			before := op.Snapshot()
			huge := "000" + strings.Repeat("9", 200)
			steps := integrationTestValue(huge)
			if cancel == 0 {
				steps = []integrationTestStep{integrationTestNumber(2, "Отмен")}
			}
			if cancel == 1 {
				steps = append(steps, integrationTestNumber(2, "Отмен"))
			}
			if cancel == 2 {
				steps = append(steps, integrationTestValue("0000003")...)
			}
			driver := &integrationTestDriver{t: t, steps: steps}
			d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
			weights, accepted, err := d.collectLessonWeights(before, nil)
			if err != nil {
				t.Fatal(err)
			}
			if cancel < 2 {
				if accepted || weights != nil {
					t.Fatal("partial group escaped cancellation")
				}
			} else {
				if !accepted || weights[before.Catalog.Items[0].Entry.FileName] != huge || weights[before.Catalog.Items[1].Entry.FileName] != "0000003" {
					t.Fatalf("raw group altered: %#v", weights)
				}
			}
			if !reflect.DeepEqual(before.Catalog, op.Snapshot().Catalog) {
				t.Fatal("collect mutated app before full proposal")
			}
		})
	}
}

func TestConsoleIntegrationCandidateAcceptRejectIncludingTemporaryBlocker(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject", true: "accept"}[accept], func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, "json")
			first := integrationTestSource(t, dir, "A41", "First", "Text", nil, integrationTestPointer("1"))
			second := integrationTestSource(t, dir, "B41", "Second", "Text", nil, integrationTestPointer("2"))
			firstBytes, secondBytes := integrationTestBytes(t, first), integrationTestBytes(t, second)
			op := integrationTestCreateOperation(t, base, dir, "Курс", "")
			integrationTestIssues(t, op.SetDestination(op.Snapshot().Token, base, "blocked.zip"))
			snapshot := op.Snapshot()
			// Duplicate weight is an editable intermediate state, owned by app.
			edit, issues := op.ProposeLessonWeight(snapshot.Token, snapshot.Catalog.Items[1].Entry.FileName, "1")
			if edit == nil || len(edit.Blockers) == 0 {
				t.Fatal("fixture did not produce temporary blocker")
			}
			firstContext := diagnostic.Context{Path: first, FileName: "l001.json", LessonID: "A41"}
			secondContext := diagnostic.Context{Path: second, FileName: "l002.json", LessonID: "B41"}
			expectedIssues := []*diagnostic.Error{{Code: catalog.CodeWeightConflict, Reason: "Уроки одной группы имеют одинаковый числовой вес.", Context: firstContext, Related: []diagnostic.Context{firstContext, secondContext}, Details: []diagnostic.Detail{
				{Label: "Секция", Value: "Без секции"}, {Label: "Общий числовой вес", Value: "1"},
				{Label: "Техническое имя", Value: "l001.json"}, {Label: "Название", Value: "First"}, {Label: "Исходный вес", Value: "1"},
				{Label: "Техническое имя", Value: "l002.json"}, {Label: "Название", Value: "Second"}, {Label: "Исходный вес", Value: "1"},
			}}}
			if !reflect.DeepEqual(issues, expectedIssues) || !reflect.DeepEqual(edit.Blockers, expectedIssues) {
				t.Fatalf("unexpected conflict diagnostics: issues=%#v blockers=%#v", issues, edit.Blockers)
			}
			pending := op.Snapshot()
			if pending.Token != snapshot.Token || !reflect.DeepEqual(pending.Catalog, snapshot.Catalog) || pending.Actions.Save || pending.Actions.ReviewSave {
				t.Fatal("unaccepted candidate changed the accepted state or enabled saving")
			}
			composition, err := preview.Composition(edit.Catalog, snapshot.Module, snapshot.NewFileNames)
			if err != nil {
				t.Fatal(err)
			}
			changes, err := preview.ManifestChanges(edit.Changes)
			if err != nil {
				t.Fatal(err)
			}
			driver := &integrationTestDriver{t: t, steps: []integrationTestStep{{literal: map[bool]string{false: "2", true: "1"}[accept], check: func(s string) {
				integrationTestContains(t, s, "A41", "B41")
				integrationTestContains(t, s, composition, changes, first, second, catalog.CodeWeightConflict)
				if strings.Count(s, catalog.CodeWeightConflict) != 2 {
					t.Fatal("original proposal issues and candidate blockers were not both shown")
				}
				integrationTestNoFile(t, filepath.Join(base, "blocked.zip"))
			}}}}
			d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
			if err := d.reviewEdit(snapshot, edit, issues); err != nil {
				t.Fatal(err)
			}
			next := op.Snapshot()
			if !next.Actions.EditCatalog {
				t.Fatal("temporary blocker closed catalog editing")
			}
			want := "2"
			if accept {
				want = "1"
			}
			if got := next.Catalog.Items[1].Entry.Order; got == nil || *got != want {
				t.Fatalf("candidate decision lost: %v", got)
			}
			if got := next.Catalog.Items[0].Entry.Order; got == nil || *got != "1" {
				t.Fatal("candidate changed the other lesson's weight")
			}
			if accept {
				if next.Token == snapshot.Token || !reflect.DeepEqual(next.Blockers, expectedIssues) || next.Actions.ReviewSave || next.Actions.Save {
					t.Fatal("accepted blocked candidate kept its old token, lost blockers, or enabled saving")
				}
			} else if next.Token != snapshot.Token || !reflect.DeepEqual(next.Catalog, snapshot.Catalog) || len(next.Blockers) != 0 || !next.Actions.ReviewSave || next.Actions.Save {
				t.Fatal("reject changed the accepted catalog/token or save eligibility")
			}
			stale := op.RejectEdit(edit.Token)
			if len(stale) != 1 || stale[0].Code != app.CodeStaleReview {
				t.Fatal("consumed candidate token remained usable")
			}
			integrationTestNoFile(t, filepath.Join(base, "blocked.zip"))
			if !bytes.Equal(firstBytes, integrationTestBytes(t, first)) || !bytes.Equal(secondBytes, integrationTestBytes(t, second)) {
				t.Fatal("catalog review wrote source JSON")
			}
		})
	}
}

func TestConsoleIntegrationFullWeightsOneProposalAndOneAcceptance(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "json")
	integrationTestSource(t, dir, "A41", "First", "Text", nil, integrationTestPointer("1"))
	integrationTestSource(t, dir, "B41", "Second", "Text", nil, integrationTestPointer("2"))
	op := integrationTestCreateOperation(t, base, dir, "Курс", "")
	snapshot := op.Snapshot()
	steps := []integrationTestStep{integrationTestMenu("Без секций")}
	steps = integrationTestAppendValue(steps, "0005", "0006")
	steps = append(steps, integrationTestStep{literal: "1", check: func(s string) { integrationTestContains(t, s, "0005", "0006") }})
	driver := &integrationTestDriver{t: t, steps: steps}
	d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
	if err := d.editGroupWeights(snapshot); err != nil {
		t.Fatal(err)
	}
	if len(driver.steps) != 0 {
		t.Fatal("full group did not request exactly one confirmation")
	}
	next := op.Snapshot()
	for i, want := range []string{"0005", "0006"} {
		if next.Catalog.Items[i].Entry.Order == nil || *next.Catalog.Items[i].Entry.Order != want {
			t.Fatalf("group item %d not accepted together", i)
		}
	}
}
