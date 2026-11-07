package console

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/preview"
)

func TestConsoleIntegrationUpdatePublicRevisionAndSeparateOverwrite(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject", true: "accept"}[accept], func(t *testing.T) {
			base := t.TempDir()
			oldDir := filepath.Join(base, "old")
			original := integrationTestSource(t, oldDir, "UP41", "Старое название", "OldBody41", nil, nil)
			archive := integrationTestSeed(t, base, oldDir, "update.zip")
			beforeArchive := integrationTestBytes(t, archive)
			beforeSource := integrationTestBytes(t, original)
			source := integrationTestSource(t, filepath.Join(base, "new"), "UP41", "Новое название", "NewBody41", nil, nil)
			op, issues := app.NewUpdate(base, archive, source)
			integrationTestIssues(t, issues)
			integrationTestIssues(t, op.Analyze(time.Date(2024, 2, 4, 0, 0, 0, 0, time.UTC)))
			initial := op.Snapshot()
			if initial.Plan == nil || len(initial.Plan.Fields) != 1 {
				t.Fatalf("fixture fields: %+v", initial.Plan)
			}
			field, err := preview.FieldProposal(initial.Before, initial.Plan.Fields[0])
			if err != nil {
				t.Fatal(err)
			}
			integrationTestIssues(t, op.DecideField(initial.Token, 0, accept))
			integrationTestIssues(t, op.PrepareRevision(op.Snapshot().Token))
			prepared := op.Snapshot()
			replacements, err := preview.Replacements(prepared.Before, prepared.After, prepared.Plan)
			if err != nil {
				t.Fatal(err)
			}
			steps := []integrationTestStep{integrationTestNumber(2, "Обнов")}
			steps = integrationTestAppendValue(steps, archive, source)
			steps = append(steps, integrationTestMenu("(изменени.*пол|пол.*изменени|следующ.*пол|решить.*пол)"), integrationTestStep{literal: map[bool]string{true: "1", false: "2"}[accept], check: func(s string) { integrationTestContains(t, s, field) }})
			revision := integrationTestMenu("Перейти к итоговому просмотру изменений")
			revision.check = func(s string) {
				if strings.Contains(s, "NewBody41") {
					t.Fatal("node data leaked before revision")
				}
			}
			steps = append(steps, revision, integrationTestStep{menu: "(запис|сохран).*", check: func(s string) {
				integrationTestContains(t, s, replacements)
				if strings.Contains(s, "NewBody41") {
					t.Fatal("Node.Data leaked")
				}
			}})
			steps = append(steps, integrationTestStep{literal: "1", check: func(s string) { integrationTestContains(t, s, archive, replacements) }}, integrationTestStep{literal: "1", check: func(s string) { integrationTestContains(t, s, archive, "Перезапис") }}, integrationTestMenu("Выход|Выйти"))
			integrationTestRun(t, base, steps)
			saved := integrationTestRead(t, archive)
			title := "Старое название"
			if accept {
				title = "Новое название"
			}
			if saved.Files[0].Lesson().Document.Title != title {
				t.Fatalf("field decision lost: %s", saved.Files[0].Lesson().Document.Title)
			}
			if bytes.Equal(beforeArchive, integrationTestBytes(t, archive)) {
				t.Fatal("update did not install")
			}
			if !bytes.Equal(beforeSource, integrationTestBytes(t, original)) {
				t.Fatal("old source modified")
			}
		})
	}
}

func TestConsoleIntegrationUpdateNoChangesOnlyAfterExplicitRevision(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "old")
	source := integrationTestSource(t, dir, "SAME41", "Same", "Text", nil, nil)
	archive := integrationTestSeed(t, base, dir, "same.zip")
	before := integrationTestBytes(t, archive)
	steps := []integrationTestStep{integrationTestNumber(2, "Обнов")}
	steps = integrationTestAppendValue(steps, archive, source)
	steps = append(steps, integrationTestStep{menu: "Перейти к итоговому просмотру изменений", check: func(s string) {
		if strings.Contains(strings.ToLower(s), "изменений нет") {
			t.Fatal("premature NoChanges terminal")
		}
	}}, integrationTestMenu("Выход|Выйти"))
	integrationTestRun(t, base, steps)
	if !bytes.Equal(before, integrationTestBytes(t, archive)) {
		t.Fatal("no-change update wrote archive")
	}
}

func TestConsoleIntegrationPlacementIsSeparateAndPreviewOwned(t *testing.T) {
	base := t.TempDir()
	old := filepath.Join(base, "old")
	integrationTestSource(t, old, "PLACE41", "Title", "Text", nil, integrationTestPointer("1"))
	archive := integrationTestSeed(t, base, old, "placement.zip")
	source := integrationTestSource(t, filepath.Join(base, "new"), "PLACE41", "Title", "Text", integrationTestPointer("Секция"), integrationTestPointer("2"))
	op, issues := app.NewUpdate(base, archive, source)
	integrationTestIssues(t, issues)
	integrationTestIssues(t, op.Analyze(time.Now()))
	snapshot := op.Snapshot()
	if len(snapshot.PlacementProposals) != 2 {
		t.Fatalf("expected independent section/order proposals: %+v", snapshot.PlacementProposals)
	}
	for i := 0; i < 2; i++ {
		snapshot = op.Snapshot()
		expected, err := preview.ManifestChanges(snapshot.PlacementProposals[i : i+1])
		if err != nil {
			t.Fatal(err)
		}
		driver := &integrationTestDriver{t: t, steps: []integrationTestStep{{literal: map[int]string{0: "1", 1: "2"}[i], check: func(s string) { integrationTestContains(t, s, expected) }}}}
		d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
		if err := d.decidePlacement(snapshot); err != nil {
			t.Fatal(err)
		}
		next := op.Snapshot()
		if !next.PlacementAnswered[i] {
			t.Fatalf("placement %d unanswered", i)
		}
		if i == 0 && next.PlacementAnswered[1] {
			t.Fatal("section answer also answered order")
		}
	}
}
