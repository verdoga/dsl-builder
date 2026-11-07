package console_test

import (
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/preview"
)

func TestConsoleDocWeightGroupCancelThenCompleteProposal(t *testing.T) {
	base := t.TempDir()
	archive, before := docTestBaseline(t, base)
	raw := docTestRead(t, archive)
	source := docTestSource(t, base, "new/новый.json", [][2]string{{"DOC39", "NEW39"}}, func(value *model.Result) { weight := "3"; value.Document.Metadata.Order = &weight })
	expected, issues := app.NewAdd(base, archive, source)
	docTestIssues(t, issues)
	if expected == nil {
		t.Fatal("nil add fixture")
	}
	t.Cleanup(expected.Cancel)
	docTestIssues(t, expected.Analyze(time.Date(2026, 11, 4, 12, 1, 0, 0, time.UTC)))
	unchangedReview := docTestFullReview(t, expected)
	huge := "00012345678901234567890123456789012345678901234567890"
	edit, issues := expected.ProposeLessonWeights(expected.Snapshot().Token, nil, map[string]string{
		"l001.json": huge, "l002.json": "0001", "l003.json": "2",
	})
	docTestIssues(t, issues)
	if edit == nil {
		t.Fatal("nil complete weights candidate")
	}
	composition, err := preview.Composition(edit.Catalog, expected.Snapshot().Module, expected.Snapshot().NewFileNames)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := preview.ManifestChanges(edit.Changes)
	if err != nil {
		t.Fatal(err)
	}
	docTestIssues(t, expected.AcceptEdit(edit.Token))
	changedReview := docTestFullReview(t, expected)
	group := `(?i)(вес[аы].*групп|групп.*вес)`
	steps := docTestExistingInput(3, archive, source)
	steps = append(steps, docTestMenu(group), docTestMenu(`(?i)^Без секций$`))
	steps = append(steps, docTestValue(huge)...)
	// Отмена второго вопроса отбрасывает также уже введённый вес первого урока.
	steps = append(steps, docTestMenu(`^Отменить текущий ввод`), docTestReviewStep(nil), docTestStep{answer: "2\n", before: func(screen string) {
		unchangedReview(screen)
		docTestAssertUnchanged(t, archive, raw)
	}})
	steps = append(steps, docTestMenu(group), docTestMenu(`(?i)^Без секций$`))
	for _, weight := range []string{huge, "0001", "2"} {
		steps = append(steps, docTestValue(weight)...)
	}
	steps = append(steps, docTestStep{answer: "1\n", before: func(screen string) {
		docTestRequire(t, screen, composition, changes, huge, "0001")
		docTestAssertUnchanged(t, archive, raw)
	}})
	steps = append(steps, docTestReviewStep(nil), docTestStep{answer: "1\n", before: changedReview}, docTestAnswer("1"), docTestMenu(`^Выход$`))
	docTestRun(t, base, steps)
	after := docTestModule(t, archive)
	docTestSameOldFiles(t, before, after, "l001.json", "l002.json")
	entries := *after.Manifest.Course.Lessons
	if len(entries) != 3 || entries[0].FileName != "l002.json" || entries[1].FileName != "l003.json" || entries[2].FileName != "l001.json" {
		t.Fatalf("wrong full group order: %+v", entries)
	}
	if entries[2].Order == nil || *entries[2].Order != huge || entries[0].Order == nil || *entries[0].Order != "0001" {
		t.Fatal("raw weights altered")
	}
}
