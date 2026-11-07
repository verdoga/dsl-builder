package console

import (
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/revision"
)

// TestConsoleUpdateFixtureOwners проверяет предусловия отдельно от console runtime.
func TestConsoleUpdateFixtureOwners(t *testing.T) {
	for _, kind := range []string{"fields", "unmatched-null", "node", "page", "lesson", "history-error"} {
		t.Run(kind, func(t *testing.T) {
			before := consoleUpdateTestPlain()
			after := consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Новый материал", "Второй"}})
			switch kind {
			case "fields":
				before = consoleUpdateTestResult("Старое", consoleUpdateTestText("Подзаголовок"), []*string{nil}, [][]string{{"Материал"}})
				after = consoleUpdateTestResult("Новое", nil, []*string{consoleUpdateTestText("Страница")}, [][]string{{"Материал"}})
			case "page":
				after = consoleUpdateTestResult("Прежнее", nil, []*string{nil}, [][]string{{"Материал", "Второй", "Третий"}})
			case "lesson", "unmatched-null":
				after = consoleUpdateTestResult("Прежнее", nil, []*string{nil, nil}, [][]string{{"Материал", "Второй"}, {"Новая страница"}})
			}
			op := consoleUpdateTestOperation(t, before, after, false, kind == "history-error")
			snapshot := op.Snapshot()
			if snapshot.Plan == nil {
				t.Fatal("missing owner plan")
			}
			if kind == "fields" {
				consoleUpdateTestFieldCount(t, snapshot, 3)
			}
			if kind == "unmatched-null" {
				consoleUpdateTestFieldCount(t, snapshot, 1)
				field := snapshot.Plan.Fields[0]
				if field.Field != compare.PageTitle || field.OldPage != -1 || field.After != nil {
					t.Fatal("missing unmatched null proposal")
				}
			}
			for index := range snapshot.Plan.Fields {
				consoleUpdateTestOK(t, op.DecideField(op.Snapshot().Token, index, false))
			}
			issues := op.PrepareRevision(op.Snapshot().Token)
			if kind == "history-error" {
				if len(issues) != 1 || issues[0].Code != revision.CodeHistory || op.Snapshot().RevisionReady {
					t.Fatalf("missing recoverable owner revision failure: %+v", issues)
				}
			} else {
				consoleUpdateTestOK(t, issues)
				if !op.Snapshot().RevisionReady {
					t.Fatal("owner fixture did not prepare revision")
				}
			}
		})
	}
}

func TestConsoleUpdateFixtureMultipleBlockers(t *testing.T) {
	op := consoleUpdateTestOperation(t, consoleUpdateTestPlain(), consoleUpdateTestPlain(), false)
	edit, _ := op.ProposeSection(op.Snapshot().Token, "Новая секция")
	if edit == nil {
		t.Fatal("missing editable section candidate")
	}
	op.AcceptEdit(edit.Token)
	edit, _ = op.ProposeMove(op.Snapshot().Token, "l001.json", consoleUpdateTestText("Новая секция"))
	if edit == nil {
		t.Fatal("missing editable move candidate")
	}
	op.AcceptEdit(edit.Token)
	issues := op.PrepareRevision(op.Snapshot().Token)
	if len(issues) < 2 || !op.Snapshot().RevisionReady || op.Snapshot().Actions.Save {
		t.Fatalf("fixture lacks multiple ready-but-blocked owner diagnostics: %+v", issues)
	}
}
