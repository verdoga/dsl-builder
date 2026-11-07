package console_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/preview"
)

func TestConsoleDocUpdateDecidesEachFieldThenExplicitRevision(t *testing.T) {
	base := t.TempDir()
	archive, before := docTestBaseline(t, base)
	rawArchive := docTestRead(t, archive)
	source := docTestSource(t, base, "new/исправление.json", [][2]string{
		{"Урок исходный", "Урок предложенный"}, {"Подзаголовок исходный", "Подзаголовок предложенный"},
		{"Страница исходная", "Страница предложенная"}, {"DOC39_BODY_PRIVATE", "DOC39_BODY_CHANGED"},
	}, nil)
	expected := docTestUpdate(t, base, archive, source, []bool{false, true, true})
	snapshot := expected.Snapshot()
	fullReview := docTestFullReview(t, expected)
	steps := docTestExistingInput(2, docTestQuote(archive), docTestQuote(source))
	for i, answer := range []string{"2", "1", "1"} {
		proposal, err := preview.FieldProposal(snapshot.Before, snapshot.Plan.Fields[i])
		if err != nil {
			t.Fatal(err)
		}
		steps = append(steps, docTestMenu(`(?i)(пол[еяю]|метаданн)`), docTestStep{answer: answer + "\n", before: func(screen string) {
			docTestRequire(t, screen, proposal)
			docTestAssertUnchanged(t, archive, rawArchive)
			if strings.Contains(screen, "doc39-n001-v02") {
				t.Fatal("final ID shown before explicit revision")
			}
		}})
	}
	revision := docTestMenu(`^Перейти к итоговому просмотру изменений$`)
	revision.before = func(screen string) {
		docTestAssertUnchanged(t, archive, rawArchive)
		if strings.Contains(screen, "doc39-n001-v02") {
			t.Fatal("revision prepared automatically")
		}
	}
	steps = append(steps, revision, docTestReviewStep(docTestBefore(t, "doc39-n001-v02")), docTestStep{answer: "1\n", before: fullReview}, docTestStep{answer: "1\n", before: func(screen string) {
		docTestRequire(t, screen, archive, "Перезаписать")
		docTestAssertUnchanged(t, archive, rawArchive)
	}}, docTestMenu(`^Выход$`))
	docTestRun(t, base, steps)
	after := docTestModule(t, archive)
	docTestSameOldFiles(t, before, after, "l002.json")
	document := docTestFile(t, after, "l001.json").Lesson().Document
	if document.Title != "Урок исходный" || document.Subtitle == nil || *document.Subtitle != "Подзаголовок предложенный" {
		t.Fatalf("separate decisions lost: %+v", document)
	}
	if document.Version.String() != "2" || document.Pages[0].Title == nil || *document.Pages[0].Title != "Страница предложенная" || document.Pages[0].Nodes[0].ID != "doc39-n001-v02" {
		t.Fatalf("wrong revision: %+v", document)
	}
}

func TestConsoleDocRejectedFieldsNeedRevisionBeforeNoChanges(t *testing.T) {
	base := t.TempDir()
	archive, before := docTestBaseline(t, base)
	raw := docTestRead(t, archive)
	source := docTestSource(t, base, "new/только поля.json", [][2]string{
		{"Урок исходный", "Новое название"}, {"Подзаголовок исходный", "Новый подзаголовок"}, {"Страница исходная", "Новая страница"},
	}, nil)
	expected := docTestUpdate(t, base, archive, source, []bool{false, false, false})
	if !expected.Snapshot().NoChanges {
		t.Fatal("fixture should have no changes after revision")
	}
	steps := docTestExistingInput(2, archive, source)
	for range 3 {
		steps = append(steps, docTestMenu(`(?i)(пол[еяю]|метаданн)`), docTestAnswer("2"))
	}
	revision := docTestMenu(`^Перейти к итоговому просмотру изменений$`)
	revision.before = func(screen string) {
		if strings.Contains(screen, "Содержательных изменений не обнаружено") {
			t.Fatal("NoChanges announced before revision")
		}
		docTestMissingMenu(t, screen, `(?i:запис|сохран)`)
		docTestAssertUnchanged(t, archive, raw)
	}
	steps = append(steps, revision, docTestStep{label: `^Выход$`, before: docTestBefore(t, "Содержательных изменений не обнаружено")})
	docTestRun(t, base, steps)
	docTestAssertUnchanged(t, archive, raw)
	docTestSameOldFiles(t, before, docTestModule(t, archive), "l001.json", "l002.json")
}

func TestConsoleDocUpdateCancelBetweenFieldDecisionsLeavesArchive(t *testing.T) {
	base := t.TempDir()
	archive, _ := docTestBaseline(t, base)
	raw := docTestRead(t, archive)
	source := docTestSource(t, base, "new/поля.json", [][2]string{{"Урок исходный", "Новое название"}, {"Подзаголовок исходный", "Новый подзаголовок"}}, nil)
	steps := docTestExistingInput(2, archive, source)
	steps = append(steps, docTestMenu(`(?i)(пол[еяю]|метаданн)`), docTestAnswer("1"), docTestMenu(`(?i)главное меню`), docTestAnswer("4"))
	docTestRun(t, base, steps)
	docTestAssertUnchanged(t, archive, raw)
}

func TestConsoleDocRetryUsesFreshlyReadSource(t *testing.T) {
	base := t.TempDir()
	directory := filepath.Join(base, "repair")
	source := docTestSource(t, base, "repair/урок.json", nil, nil)
	valid := docTestRead(t, source)
	if err := docTestWrite(source, []byte("{")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "после исправления.zip")
	steps := docTestCreateInput(directory, "Курс после исправления")
	retry := docTestMenu(`(?i)(повтор.*анализ|анализ.*повтор)`)
	retry.before = func(screen string) {
		docTestRequire(t, screen, source)
		docTestMissingMenu(t, screen, `(?i:запис|сохран)`)
		if err := docTestWrite(source, valid); err != nil {
			t.Fatal(err)
		}
	}
	steps = append(steps, retry)
	steps = append(steps, docTestDestination(base, "после исправления")...)
	steps = append(steps, docTestReviewStep(docTestBefore(t, "Урок исходный", "l001.json")), docTestStep{answer: "1\n", before: docTestBefore(t, target, "Урок исходный")}, docTestMenu(`^Выход$`))
	docTestRun(t, base, steps)
	if len(docTestModule(t, target).Files) != 1 {
		t.Fatal("retry did not publish repaired source")
	}
}
