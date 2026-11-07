package console

import (
	"bufio"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
)

func TestConsoleIntegrationAddEveryOldAndNewLessonCanMove(t *testing.T) {
	base := t.TempDir()
	old := filepath.Join(base, "old")
	integrationTestSource(t, old, "OLD41", "Old", "Text", nil, integrationTestPointer("1"))
	archive := integrationTestSeed(t, base, old, "sections.zip")
	incoming := filepath.Join(base, "new")
	integrationTestSource(t, incoming, "NEW41", "New", "Text", nil, integrationTestPointer("2"))
	integrationTestSource(t, incoming, "NEXT41", "Next", "Text", nil, integrationTestPointer("3"))
	op, issues := app.NewAdd(base, archive, incoming)
	integrationTestIssues(t, issues)
	integrationTestIssues(t, op.Analyze(time.Now()))
	driver := &integrationTestDriver{t: t, steps: append(integrationTestValue("Unit41"), integrationTestStep{literal: "1"})}
	d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
	if err := d.editUnit(op.Snapshot()); err != nil {
		t.Fatal(err)
	}
	driver.steps = append(integrationTestValue("Section41"), integrationTestStep{literal: "1"})
	if err := d.createSection(op.Snapshot()); err != nil {
		t.Fatal(err)
	}
	snapshot := op.Snapshot()
	names := []string{}
	for _, item := range snapshot.Catalog.Items {
		names = append(names, item.Entry.FileName)
	}
	for _, name := range names {
		driver.steps = []integrationTestStep{integrationTestMenu(regexp.QuoteMeta(name)), integrationTestMenu("Section41"), {literal: "1"}}
		if err := d.moveLesson(op.Snapshot()); err != nil {
			t.Fatal(err)
		}
		if len(driver.steps) != 0 {
			t.Fatalf("lesson %s was not individually selectable", name)
		}
	}
	for _, item := range op.Snapshot().Catalog.Items {
		if item.Section == nil || *item.Section != "Section41" {
			t.Fatalf("old/new lesson not distributed: %s", item.Entry.FileName)
		}
	}
	before := op.Snapshot()
	driver.steps = []integrationTestStep{integrationTestNumber(2, "Отмен")}
	if err := d.editSectionWeights(before); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Catalog, op.Snapshot().Catalog) {
		t.Fatal("cancelled section weight group changed catalog")
	}
	driver.steps = append(integrationTestValue("0009"), integrationTestStep{literal: "1"})
	if err := d.editSectionWeights(op.Snapshot()); err != nil {
		t.Fatal(err)
	}
	weight := op.Snapshot().Catalog.Sections[0].Weight
	if weight == nil || *weight != "0009" {
		t.Fatalf("section weight altered: %v", weight)
	}
}

func TestConsoleIntegrationPublicRestartCollectsWholeNewInput(t *testing.T) {
	base := t.TempDir()
	ready := filepath.Join(base, "ready")
	integrationTestSource(t, ready, "RESTART41", "New operation", "Text", nil, nil)
	expected := integrationTestCompositionBlocks(t, integrationTestCreateOperation(t, base, ready, "Current", "").Snapshot())
	steps := []integrationTestStep{integrationTestNumber(1, "Созд")}
	steps = integrationTestAppendValue(steps, filepath.Join(base, "missing"), "Abandoned", "")
	steps = append(steps, integrationTestMenu("заново|новый ввод|начать.*(снова|операци)"))
	steps = integrationTestAppendValue(steps, ready, "Current", "")
	steps = append(steps, integrationTestStep{menu: "Выход|Выйти", check: func(s string) {
		integrationTestContains(t, s, expected...)
		integrationTestContains(t, s, "Current", `"l001.json" | "New operation" | order: "1"`)
		if strings.Contains(s, "НОВЫЙ") || strings.Contains(s, "Abandoned") {
			t.Fatal("restart retained the old input or marked a Create row as an addition")
		}
	}})
	integrationTestRun(t, base, steps)
}
