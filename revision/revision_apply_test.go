package revision_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/revision"
)

func revisionTestCandidate(t *testing.T, value *lesson.Lesson) *lesson.Lesson {
	t.Helper()
	result, errors := revision.Initial(value, revisionTestAt(), diagnostic.Context{})
	if result == nil || len(errors) != 0 {
		t.Fatalf("кандидат Initial: %v", errors)
	}
	return result
}

func TestRevisionApplyValidatesInputsPlanAndDecisionsAtomically(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		damage     func(**lesson.Lesson, **lesson.Lesson, **compare.Plan, *[]revision.Decision)
	}{
		{"old nil", lesson.CodeContract, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { *o = nil }},
		{"candidate nil", lesson.CodeContract, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { *c = nil }},
		{"old schema", lesson.CodeContract, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { (*o).Document.Version = "0" }},
		{"candidate schema", lesson.CodeContract, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) {
			(*c).Document.Pages[0].Nodes = nil
		}},
		{"different lesson", revision.CodeLessonMismatch, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) {
			*c = revisionTestCandidate(t, &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{ID: "Other", LineCount: "0", Pages: []lesson.Page{}}})
		}},
		{"history", revision.CodeHistory, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) {
			(*o).Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n081-v06"
		}},
		{"plan nil", revision.CodePlan, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { *p = nil }},
		{"missing decision", revision.CodeDecision, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { *d = nil }},
		{"extra decision", revision.CodeDecision, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { *d = append(*d, revision.Accept) }},
		{"zero decision", revision.CodeDecision, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { (*d)[0] = "" }},
		{"unknown decision", revision.CodeDecision, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { (*d)[0] = "yes" }},
		{"field address", revision.CodePlan, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { (*p).Fields[0].NewPage = 0 }},
		{"field name", revision.CodePlan, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { (*p).Fields[0].Field = "unknown" }},
		{"page address", revision.CodePlan, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { (*p).Pages[0].Index = 99 }},
		{"node path", revision.CodePlan, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) {
			(*p).Pages[0].Nodes[0].Path = []int{99}
		}},
		{"node field", revision.CodePlan, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) {
			(*p).Pages[0].Nodes[0].Fields = []compare.NodeField{"unknown"}
		}},
		{"replacement form", revision.CodePlan, func(o, c **lesson.Lesson, p **compare.Plan, d *[]revision.Decision) { (*p).ReplaceLesson = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := revisionTestLesson()
			candidate := revisionTestCandidate(t, old)
			candidate.Document.Title = "новое"
			candidate.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Hint = "новое"
			plan := revisionTestPlan(t, old, candidate)
			decisions := revisionTestDecisions(plan, revision.Accept)
			tc.damage(&old, &candidate, &plan, &decisions)
			oldBefore, candidateBefore, planBefore, decisionsBefore := revisionTestJSON(t, old), revisionTestJSON(t, candidate), revisionTestJSON(t, plan), revisionTestJSON(t, decisions)
			result, changed, errors := revision.Apply(old, candidate, plan, decisions, revisionTestAt(), diagnostic.Context{Path: "old.json"}, diagnostic.Context{Path: "new.json"})
			if result != nil || changed {
				t.Fatalf("частичный результат: %v changed=%v", result, changed)
			}
			revisionTestCode(t, errors, tc.code)
			revisionTestEqual(t, old, oldBefore)
			revisionTestEqual(t, candidate, candidateBefore)
			revisionTestEqual(t, plan, planBefore)
			revisionTestEqual(t, decisions, decisionsBefore)
		})
	}
}

func TestRevisionApplyFieldOnlyHasOneExactRevision(t *testing.T) {
	for _, tc := range []struct {
		version, want string
		field         compare.Field
	}{
		{"5.0", "6", compare.DocumentTitle}, {"5e0", "6", compare.DocumentSubtitle},
		{"9.9e1", "100", compare.PageTitle}, {"9007199254740993", "9007199254740994", compare.DocumentTitle},
		{"18446744073709551616", "18446744073709551617", compare.PageTitle},
	} {
		t.Run(fmt.Sprint(tc.field, tc.version), func(t *testing.T) {
			old := revisionTestLesson()
			old.Document.Version = json.Number(tc.version)
			candidate := revisionTestCandidate(t, old)
			candidate.Document.ID = "eFX7"
			candidate.Document.LineCount = "123456789012345678901234567890.0"
			candidate.Document.Title = "новое"
			candidate.Document.Subtitle = nil
			candidate.Document.Pages[0].Title = revisionTestText("")
			plan := revisionTestPlan(t, old, candidate)
			decisions := revisionTestDecisions(plan, revision.Reject)
			expected := revisionTestCopy(t, old)
			for i, field := range plan.Fields {
				if field.Field == tc.field {
					decisions[i] = revision.Accept
				}
			}
			switch tc.field {
			case compare.DocumentTitle:
				expected.Document.Title = candidate.Document.Title
			case compare.DocumentSubtitle:
				expected.Document.Subtitle = nil
			case compare.PageTitle:
				expected.Document.Pages[0].Title = revisionTestText("")
			}
			expected.Document.Version = json.Number(tc.want)
			expected.Document.CreatedAt = "2026-01-01T22:04:05Z"
			expected.Document.LineCount = candidate.Document.LineCount
			oldBefore, candidateBefore, planBefore, decisionsBefore := revisionTestJSON(t, old), revisionTestJSON(t, candidate), revisionTestJSON(t, plan), revisionTestJSON(t, decisions)
			for range 2 {
				result, changed, errors := revision.Apply(old, candidate, plan, decisions, revisionTestAt(), diagnostic.Context{}, diagnostic.Context{})
				if !changed || result == nil || len(errors) != 0 {
					t.Fatalf("Apply: changed=%v errors=%v", changed, errors)
				}
				revisionTestValid(t, result)
				revisionTestEqual(t, result, revisionTestJSON(t, expected))
				revisionTestEqual(t, old, oldBefore)
				revisionTestEqual(t, candidate, candidateBefore)
				revisionTestEqual(t, plan, planBefore)
				revisionTestEqual(t, decisions, decisionsBefore)
			}
		})
	}
}

func TestRevisionApplyExecutesThreeReplacementLevels(t *testing.T) {
	for _, level := range []string{"node", "page", "lesson"} {
		t.Run(level, func(t *testing.T) {
			old := revisionTestLesson()
			source := revisionTestCopy(t, old)
			switch level {
			case "node":
				source.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value = "ребёнок изменён"
				source.Document.Pages[0].Nodes[1].Data[0].Hint = "медиа изменено"
			case "page":
				source.Document.Pages[0].Nodes = append(source.Document.Pages[0].Nodes, revisionTestLeaf("", "новая нода"))
			case "lesson":
				source.Document.Pages = append(source.Document.Pages, lesson.Page{Title: revisionTestText("новая"), Nodes: []lesson.Node{revisionTestLeaf("", "новая страница")}})
			}
			source.Document.Title = "отклонённое название"
			source.Document.Subtitle = nil
			candidate := revisionTestCandidate(t, source)
			candidate.Document.ID = "eFx7"
			candidate.Document.LineCount = "8e2"
			plan := revisionTestPlan(t, old, candidate)
			decisions := revisionTestDecisions(plan, revision.Reject)
			if (level == "lesson") != plan.ReplaceLesson {
				t.Fatalf("неожиданный уровень compare: %+v", plan)
			}
			if level != "lesson" && (len(plan.Pages) != 1 || plan.Pages[0].ReplaceNodes != (level == "page")) {
				t.Fatalf("неожиданный план: %+v", plan)
			}
			expected := revisionTestCopy(t, old)
			switch level {
			case "node":
				expected.Document.Pages[0].Nodes[0].Nodes[0].Data = candidate.Document.Pages[0].Nodes[0].Nodes[0].Data
				expected.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n081-v06"
				expected.Document.Pages[0].Nodes[1].Data = candidate.Document.Pages[0].Nodes[1].Data
				expected.Document.Pages[0].Nodes[1].ID = "efx7-n1000-v06"
			case "page":
				expected.Document.Pages[0].Nodes = revisionTestCopy(t, candidate).Document.Pages[0].Nodes
				expected.Document.Pages[0].Nodes[0].ID = "efx7-n1001-v06"
				expected.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n1002-v06"
				expected.Document.Pages[0].Nodes[1].ID = "efx7-n1003-v06"
				expected.Document.Pages[0].Nodes[2].ID = "efx7-n1004-v06"
			case "lesson":
				expected.Document.Pages = revisionTestCopy(t, candidate).Document.Pages
				for i := range expected.Document.Pages {
					expected.Document.Pages[i].ID = fmt.Sprintf("efx7-p%d", 1001+i)
				}
				expected.Document.Pages[0].Nodes[0].ID = "efx7-n1001-v06"
				expected.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n1002-v06"
				expected.Document.Pages[0].Nodes[1].ID = "efx7-n1003-v06"
				expected.Document.Pages[1].Nodes[0].ID = "efx7-n1004-v06"
				expected.Document.Pages[2].Nodes[0].ID = "efx7-n1005-v06"
				expected.Document.Pages[2].Title = nil
			}
			expected.Document.Version, expected.Document.CreatedAt, expected.Document.LineCount = "6", "2026-01-01T22:04:05Z", candidate.Document.LineCount
			oldBefore, candidateBefore, planBefore, decisionsBefore := revisionTestJSON(t, old), revisionTestJSON(t, candidate), revisionTestJSON(t, plan), revisionTestJSON(t, decisions)
			result, changed, errors := revision.Apply(old, candidate, plan, decisions, revisionTestAt(), diagnostic.Context{}, diagnostic.Context{})
			if !changed || result == nil || len(errors) != 0 {
				t.Fatalf("Apply: changed=%v errors=%v", changed, errors)
			}
			revisionTestValid(t, result)
			revisionTestEqual(t, result, revisionTestJSON(t, expected))
			revisionTestEqual(t, old, oldBefore)
			revisionTestEqual(t, candidate, candidateBefore)
			revisionTestEqual(t, plan, planBefore)
			revisionTestEqual(t, decisions, decisionsBefore)
			revisionTestMutate(result)
			revisionTestEqual(t, old, oldBefore)
			revisionTestEqual(t, candidate, candidateBefore)
			revisionTestEqual(t, plan, planBefore)
		})
	}
}

func TestRevisionApplyChangedResultOwnsAcceptedValues(t *testing.T) {
	old := revisionTestLesson()
	candidate := revisionTestCandidate(t, old)
	candidate.Document.Title = "новое"
	candidate.Document.Subtitle = revisionTestText("новый")
	candidate.Document.Pages[0].Title = revisionTestText("новая")
	candidate.Document.Pages[0].Nodes[0].Data[0].Hint = "новые данные"
	candidate.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Hint = "новый ребёнок"
	plan := revisionTestPlan(t, old, candidate)
	decisions := revisionTestDecisions(plan, revision.Accept)
	result, changed, errors := revision.Apply(old, candidate, plan, decisions, revisionTestAt(), diagnostic.Context{}, diagnostic.Context{})
	if result == nil || !changed || len(errors) != 0 {
		t.Fatalf("Apply: %v", errors)
	}
	before := revisionTestJSON(t, result)
	revisionTestMutate(old)
	revisionTestMutate(candidate)
	for i := range plan.Fields {
		if plan.Fields[i].Before != nil {
			*plan.Fields[i].Before = "план"
		}
		if plan.Fields[i].After != nil {
			*plan.Fields[i].After = "план"
		}
		decisions[i] = revision.Reject
	}
	plan.Pages[0].Nodes[0].Path[0] = 99
	plan.Pages[0].Nodes[0].Fields[0] = "план"
	revisionTestEqual(t, result, before)
}

func TestRevisionFinalInvalidDateIsAtomic(t *testing.T) {
	at := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	candidate := revisionTestLesson()
	before := revisionTestJSON(t, candidate)
	result, errors := revision.Initial(candidate, at, diagnostic.Context{Path: "new.json"})
	if result != nil {
		t.Fatal("Initial опубликовал недопустимую дату")
	}
	revisionTestCode(t, errors, lesson.CodeContract)
	revisionTestEqual(t, candidate, before)
	old := revisionTestLesson()
	candidate = revisionTestCandidate(t, old)
	candidate.Document.Title = "новое"
	plan := revisionTestPlan(t, old, candidate)
	decisions := revisionTestDecisions(plan, revision.Accept)
	oldBefore, candidateBefore, planBefore, decisionsBefore := revisionTestJSON(t, old), revisionTestJSON(t, candidate), revisionTestJSON(t, plan), revisionTestJSON(t, decisions)
	result, changed, errors := revision.Apply(old, candidate, plan, decisions, at, diagnostic.Context{}, diagnostic.Context{})
	if result != nil || changed {
		t.Fatal("Apply опубликовал частичный результат")
	}
	revisionTestCode(t, errors, lesson.CodeContract)
	revisionTestEqual(t, old, oldBefore)
	revisionTestEqual(t, candidate, candidateBefore)
	revisionTestEqual(t, plan, planBefore)
	revisionTestEqual(t, decisions, decisionsBefore)
}

func TestRevisionApplyValidatesBothSnapshotsBeforeNoOp(t *testing.T) {
	old := revisionTestLesson()
	candidate := revisionTestCandidate(t, old)
	plan := revisionTestPlan(t, old, candidate)
	old.Document.Version = "0"
	candidate.Document.LineCount = "-1"
	oldBefore, candidateBefore := revisionTestJSON(t, old), revisionTestJSON(t, candidate)
	result, changed, errors := revision.Apply(old, candidate, plan, nil, revisionTestAt(), diagnostic.Context{Path: "old.json"}, diagnostic.Context{Path: "new.json"})
	if result != nil || changed || len(errors) != 2 {
		t.Fatalf("ожидаются две входные ошибки: result=%v changed=%v errors=%v", result, changed, errors)
	}
	for i, path := range []string{"old.json", "new.json"} {
		if errors[i].Code != lesson.CodeContract || errors[i].Context.Path != path || len(errors[i].Details) == 0 {
			t.Fatalf("не сохранены код/порядок/контекст/адрес: %+v", errors[i])
		}
	}
	revisionTestEqual(t, old, oldBefore)
	revisionTestEqual(t, candidate, candidateBefore)
}
