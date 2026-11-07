package console

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/preview"
)

func TestConsoleIntegrationCreateNumericLayouts(t *testing.T) {
	for _, layout := range []string{"flat", "unit", "sections"} {
		t.Run(layout, func(t *testing.T) {
			base := t.TempDir()
			sources := filepath.Join(base, "исходники с пробелом")
			unit := ""
			var section *string
			if layout != "flat" {
				unit = "Юнит 四"
			}
			if layout == "sections" {
				section = integrationTestPointer("Раздел 一")
			}
			source := integrationTestSource(t, sources, "L41", "Урок λ", "Содержимое", section, integrationTestPointer("0007"))
			original := integrationTestBytes(t, source)
			owner := integrationTestCreateOperation(t, base, sources, "Курс Ω", unit)
			if layout == "sections" {
				s := owner.Snapshot()
				sections := s.Catalog.Sections
				sections[0].Weight = integrationTestPointer("1")
				edit, issues := owner.ProposeSectionWeights(s.Token, sections)
				integrationTestIssues(t, issues)
				if edit == nil {
					t.Fatal("missing reference section-weight candidate")
				}
				integrationTestIssues(t, owner.AcceptEdit(edit.Token))
			}
			expected := integrationTestCompositionBlocks(t, owner.Snapshot())
			steps := []integrationTestStep{integrationTestNumber(1, "Созд")}
			steps = integrationTestAppendValue(steps, `"`+sources+`"`, "Курс Ω", unit)
			if layout == "sections" {
				steps = append(steps, integrationTestMenu("(вес.*секци|поряд.*секци)"))
				steps = integrationTestAppendValue(steps, "1")
				steps = append(steps, integrationTestStep{literal: "1"})
			}
			steps = append(steps, integrationTestMenu("(цел|назначени|место.*сохран|каталог.*архив)"))
			steps = integrationTestAppendValue(steps, `"`+base+`"`, "результат.zip")
			steps = append(steps, integrationTestMenu("(запис|сохран).*"), integrationTestStep{literal: "1", check: func(screen string) {
				integrationTestContains(t, screen, expected...)
				integrationTestContains(t, screen, filepath.Join(base, "результат.zip"), "Курс Ω", `"l001.json" | "Урок λ" | order: "0007"`)
				if strings.Contains(screen, "НОВЫЙ") {
					t.Fatal("Create rows were marked as additions")
				}
				integrationTestNoFile(t, filepath.Join(base, "результат.zip"))
			}}, integrationTestMenu("Выход|Выйти"))
			d := integrationTestRun(t, base, steps)
			integrationTestContains(t, d.output.String(), "task41-версия")
			saved := integrationTestRead(t, filepath.Join(base, "результат.zip"))
			if len(saved.Files) != 1 {
				t.Fatalf("saved %d lessons", len(saved.Files))
			}
			if saved.Files[0].FileName() != "l001.json" || saved.Files[0].Lesson().Document.ID != "L41" {
				t.Fatal("saved technical name or stable lesson ID changed")
			}
			if !bytes.Equal(original, integrationTestBytes(t, source)) {
				t.Fatal("console changed technical JSON")
			}
		})
	}
}

func TestConsoleIntegrationCreateValueBeyondScannerLimit(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "json")
	integrationTestSource(t, dir, "LONG41", "Урок", "Text", nil, nil)
	title := "Курс" + strings.Repeat("x", 65540) + "終"
	steps := []integrationTestStep{integrationTestNumber(1, "Созд")}
	steps = integrationTestAppendValue(steps, dir, title, "")
	steps = append(steps, integrationTestMenu("Выход|Выйти"))
	d := integrationTestRun(t, base, steps)
	integrationTestContains(t, d.output.String(), title)
	integrationTestNoFile(t, filepath.Join(base, "ignored.zip"))
}

func TestConsoleIntegrationCreateRetryAfterSourceRepair(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "не существует")
	steps := []integrationTestStep{integrationTestNumber(1, "Созд")}
	steps = integrationTestAppendValue(steps, source, "Курс", "")
	retry := integrationTestMenu("Повтор.*анализ|анализ.*повтор|Повторить")
	var expected []string
	retry.before = func() {
		integrationTestSource(t, source, "RETRY41", "Исправлено", "Text", nil, nil)
		expected = integrationTestCompositionBlocks(t, integrationTestCreateOperation(t, base, source, "Курс", "").Snapshot())
	}
	steps = append(steps, retry, integrationTestStep{menu: "Выход|Выйти", check: func(screen string) {
		integrationTestContains(t, screen, expected...)
		integrationTestContains(t, screen, `"l001.json" | "Исправлено" | order: "1"`)
		if strings.Contains(screen, "НОВЫЙ") {
			t.Fatal("repaired Create row was marked as an addition")
		}
	}})
	integrationTestRun(t, base, steps)
}

// integrationTestCompositionBlocks получает полные блоки у владельца представления.
func integrationTestCompositionBlocks(t *testing.T, snapshot app.Snapshot) []string {
	t.Helper()
	composition, err := preview.Composition(snapshot.Catalog, snapshot.Module, snapshot.NewFileNames)
	if err != nil {
		t.Fatal(err)
	}
	return []string{composition, preview.OrderConflicts(snapshot.Conflicts), preview.Resources(snapshot.Resources)}
}
