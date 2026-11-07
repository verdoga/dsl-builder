package console_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestConsoleDocAddPreservesOldBytesAndRequiresSeparateOverwrite(t *testing.T) {
	base := t.TempDir()
	archive, before := docTestBaseline(t, base)
	raw := docTestRead(t, archive)
	source := docTestSource(t, base, "new/новый урок.json", [][2]string{{"DOC39", "NEW39"}, {"Урок исходный", "Новый урок"}}, func(value *model.Result) { weight := "0003"; value.Document.Metadata.Order = &weight })
	expected, issues := app.NewAdd(base, archive, source)
	docTestIssues(t, issues)
	if expected == nil {
		t.Fatal("nil add fixture")
	}
	t.Cleanup(expected.Cancel)
	docTestIssues(t, expected.Analyze(time.Date(2026, 11, 4, 12, 1, 0, 0, time.UTC)))
	fullReview := docTestFullReview(t, expected)
	steps := docTestExistingInput(3, docTestQuote(archive), docTestQuote(source))
	steps = append(steps, docTestReviewStep(nil), docTestStep{answer: "1\n", before: func(screen string) { fullReview(screen); docTestAssertUnchanged(t, archive, raw) }})
	steps = append(steps, docTestStep{answer: "1\n", before: func(screen string) {
		docTestRequire(t, screen, archive, "Перезаписать")
		docTestAssertUnchanged(t, archive, raw)
	}}, docTestMenu(`^Выход$`))
	docTestRun(t, base, steps)
	after := docTestModule(t, archive)
	if len(after.Files) != 3 {
		t.Fatalf("files=%d", len(after.Files))
	}
	docTestSameOldFiles(t, before, after, "l001.json", "l002.json")
	if docTestFile(t, after, "l003.json").Lesson().Document.ID != "NEW39" {
		t.Fatal("new file absent")
	}
	entries := *after.Manifest.Course.Lessons
	if entries[2].Order == nil || *entries[2].Order != "0003" {
		t.Fatal("raw leading zeros lost")
	}
}

func TestConsoleDocAddOverwriteRefusalOrEOFLeavesWholeArchive(t *testing.T) {
	for _, tc := range []struct{ name, answer string }{{"refusal", "2\n"}, {"EOF", ""}, {"partial yes", "1"}} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			archive, before := docTestBaseline(t, base)
			raw := docTestRead(t, archive)
			source := docTestSource(t, base, "new/новый.json", [][2]string{{"DOC39", "NEW39"}}, func(value *model.Result) { weight := "3"; value.Document.Metadata.Order = &weight })
			steps := docTestExistingInput(3, archive, source)
			steps = append(steps, docTestReviewStep(nil), docTestAnswer("1"), docTestStep{answer: tc.answer, before: func(screen string) {
				docTestRequire(t, screen, archive, "Перезаписать")
				docTestAssertUnchanged(t, archive, raw)
			}})
			if tc.answer == "2\n" {
				steps = append(steps, docTestMenu(`^Выход$`))
			}
			docTestRun(t, base, steps)
			docTestAssertUnchanged(t, archive, raw)
			docTestSameOldFiles(t, before, docTestModule(t, archive), "l001.json", "l002.json")
		})
	}
}

func TestConsoleDocInitialGroupCancelAndEOFDoNotAnalyzeEarly(t *testing.T) {
	for _, mode := range []int{2, 3} {
		t.Run(string(rune('0'+mode)), func(t *testing.T) {
			base := t.TempDir()
			archive, _ := docTestBaseline(t, base)
			raw := docTestRead(t, archive)
			steps := []docTestStep{docTestAnswer(string(rune('0' + mode)))}
			steps = append(steps, docTestValue(archive)...)
			// JSON question has not been answered: C-01 prohibits partial operation.
			steps = append(steps, docTestStep{answer: "", before: func(screen string) {
				docTestRequire(t, screen, "JSON")
				if bytes.Contains([]byte(screen), []byte("l001.json")) {
					t.Fatal("baseline analyzed before complete initial group")
				}
			}})
			docTestRun(t, base, steps)
			docTestAssertUnchanged(t, archive, raw)
		})
	}
}

func docTestWrite(path string, raw []byte) error { return os.WriteFile(path, raw, 0600) }
