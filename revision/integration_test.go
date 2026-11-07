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

func integrationTestText(text string) lesson.Node {
	role := lesson.RoleBody
	return lesson.Node{Type: lesson.NodeText, Data: []lesson.Data{{Type: lesson.DataText,
		Role: &role, Value: text, Answers: []string{"ответ"}, Hint: "подсказка"}}, Nodes: []lesson.Node{}}
}

func integrationTestFixture() *lesson.Lesson {
	title, subtitle, speaking := "Страница", "Подзаголовок", false
	parent := lesson.Node{Type: lesson.NodeTask, Flags: lesson.NodeFlags{Speaking: &speaking},
		Data: []lesson.Data{}, Nodes: []lesson.Node{integrationTestText("Вложенный текст")}}
	return &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EfX7", Title: "Урок", Subtitle: &subtitle, LineCount: "123456789012345678901234567890e2",
		Pages: []lesson.Page{{Title: &title, Nodes: []lesson.Node{parent}},
			{Nodes: []lesson.Node{integrationTestText("Вторая страница")}}}}}
}

func integrationTestTime() time.Time {
	return time.Date(2026, 11, 7, 12, 34, 56, 987654321, time.FixedZone("UTC+3", 10800))
}

func integrationTestJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func integrationTestRoundTrip(t *testing.T, value *lesson.Lesson) *lesson.Lesson {
	t.Helper()
	if problems := lesson.Validate(value, diagnostic.Context{Path: "результат.json"}); len(problems) != 0 {
		t.Fatalf("lesson.Validate: %v", problems)
	}
	raw := integrationTestJSON(t, value)
	var decoded lesson.Lesson
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	if problems := lesson.Validate(&decoded, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("lesson.Validate после JSON: %v", problems)
	}
	if integrationTestJSON(t, &decoded) != raw {
		t.Fatal("JSON roundtrip изменил точные значения модели")
	}
	return &decoded
}

func integrationTestInitial(t *testing.T, raw *lesson.Lesson) *lesson.Lesson {
	t.Helper()
	before := integrationTestJSON(t, raw)
	result, problems := revision.Initial(raw, integrationTestTime(), diagnostic.Context{Path: "новый.json"})
	if len(problems) != 0 || result == nil {
		t.Fatalf("Initial: %v", problems)
	}
	if integrationTestJSON(t, raw) != before {
		t.Fatal("Initial изменил вход")
	}
	integrationTestRoundTrip(t, result)
	return result
}

func integrationTestPlan(t *testing.T, old, candidate *lesson.Lesson) *compare.Plan {
	t.Helper()
	plan, problems := compare.Build(old, candidate, diagnostic.Context{Path: "старый.json"}, diagnostic.Context{Path: "новый.json"})
	if len(problems) != 0 || plan == nil {
		t.Fatalf("compare.Build: %v", problems)
	}
	return plan
}

func integrationTestInputs(t *testing.T, old, candidate *lesson.Lesson, plan *compare.Plan, decisions []revision.Decision) string {
	t.Helper()
	return integrationTestJSON(t, struct {
		Old, Candidate *lesson.Lesson
		Plan           *compare.Plan
		Decisions      []revision.Decision
	}{old, candidate, plan, decisions})
}

func integrationTestApply(t *testing.T, old, candidate *lesson.Lesson, plan *compare.Plan, decisions []revision.Decision, changedWant bool, at time.Time) *lesson.Lesson {
	t.Helper()
	before := integrationTestInputs(t, old, candidate, plan, decisions)
	result, changed, problems := revision.Apply(old, candidate, plan, decisions, at,
		diagnostic.Context{Path: "старый.json"}, diagnostic.Context{Path: "новый.json"})
	if len(problems) != 0 || result == nil || changed != changedWant {
		t.Fatalf("Apply: changed=%v, want=%v, errors=%v", changed, changedWant, problems)
	}
	if integrationTestInputs(t, old, candidate, plan, decisions) != before {
		t.Fatal("Apply изменил снимки, план либо решения")
	}
	integrationTestRoundTrip(t, result)
	return result
}

func TestIntegrationInitialModesAndPreorder(t *testing.T) {
	for _, mode := range []string{"создание", "добавление", "кандидат обновления"} {
		t.Run(mode, func(t *testing.T) {
			raw := integrationTestFixture()
			for len(raw.Document.Pages) < 1001 {
				raw.Document.Pages = append(raw.Document.Pages, lesson.Page{Nodes: []lesson.Node{integrationTestText("текст")}})
			}
			before := integrationTestJSON(t, raw)
			result := integrationTestInitial(t, raw)
			doc := &result.Document
			if doc.ID != "EfX7" || doc.Version != "1" || doc.CreatedAt != "2026-11-07T09:34:56Z" || doc.LineCount != raw.Document.LineCount {
				t.Fatalf("первичные метаданные: %+v", doc)
			}
			if doc.Pages[0].Nodes[0].ID != "efx7-n001" || doc.Pages[0].Nodes[0].Nodes[0].ID != "efx7-n002" || doc.Pages[1].Nodes[0].ID != "efx7-n003" {
				t.Fatal("нет сквозного preorder")
			}
			if doc.Pages[999].ID != "efx7-p1000" || doc.Pages[999].Nodes[0].ID != "efx7-n1001" {
				t.Fatal("обрезана нумерация после 999")
			}
			*doc.Subtitle, *doc.Pages[0].Title, *doc.Pages[0].Nodes[0].Flags.Speaking = "иной", "иная", true
			doc.Pages[0].Nodes[0].Nodes[0].Data[0].Answers[0] = "иной"
			if integrationTestJSON(t, raw) != before {
				t.Fatal("результат Initial разделяет память с входом")
			}
		})
	}
}

func TestIntegrationMixedFieldsAndNestedNode(t *testing.T) {
	old := integrationTestInitial(t, integrationTestFixture())
	old.Document.Version = "5.0"
	old.Document.Pages[0].ID = "efx7-p010"
	old.Document.Pages[0].Nodes[0].ID = "efx7-n035-v03"
	old.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n081"
	old.Document.Pages[1].Nodes[0].ID = "efx7-n999-v04"
	raw := integrationTestFixture()
	raw.Document.ID, raw.Document.Title, raw.Document.Subtitle = "eFX7", "Новое название", nil
	*raw.Document.Pages[0].Title = "Новое название страницы"
	raw.Document.LineCount = "9.007199254740993e30"
	raw.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value = "Исправлено"
	candidate := integrationTestInitial(t, raw)
	plan := integrationTestPlan(t, old, candidate)
	if plan.ReplaceLesson || len(plan.Fields) != 3 || len(plan.Pages) != 1 || len(plan.Pages[0].Nodes) != 1 {
		t.Fatalf("неожиданный уровень реального compare: %+v", plan)
	}
	decisions := []revision.Decision{revision.Reject, revision.Accept, revision.Accept}
	result := integrationTestApply(t, old, candidate, plan, decisions, true, integrationTestTime())
	doc := &result.Document
	if doc.ID != old.Document.ID || doc.Version != "6" || doc.LineCount != candidate.Document.LineCount || doc.CreatedAt != "2026-11-07T09:34:56Z" || doc.Title != "Урок" || doc.Subtitle != nil || *doc.Pages[0].Title != "Новое название страницы" {
		t.Fatal("не исполнены независимые решения/метаданные")
	}
	if doc.Pages[0].ID != "efx7-p010" || doc.Pages[0].Nodes[0].ID != "efx7-n035-v03" || doc.Pages[0].Nodes[0].Nodes[0].ID != "efx7-n081-v06" || doc.Pages[1].Nodes[0].ID != "efx7-n999-v04" {
		t.Fatal("точечная замена затронула чужой ID")
	}
	if doc.Pages[0].Nodes[0].Nodes[0].Data[0].Value != "Исправлено" {
		t.Fatal("данные не заменены")
	}
	again := integrationTestApply(t, old, candidate, plan, decisions, true, integrationTestTime())
	if integrationTestJSON(t, result) != integrationTestJSON(t, again) {
		t.Fatal("повторное превью увеличило ревизию ещё раз")
	}
	before := integrationTestInputs(t, old, candidate, plan, decisions)
	*doc.Pages[0].Title = "чужое"
	*doc.Pages[0].Nodes[0].Flags.Speaking = true
	*doc.Pages[0].Nodes[0].Nodes[0].Data[0].Role = lesson.RoleHint
	doc.Pages[0].Nodes[0].Nodes[0].Data[0].Answers[0] = "чужой"
	if integrationTestInputs(t, old, candidate, plan, decisions) != before {
		t.Fatal("результат разделяет указатели/срезы с входами либо планом")
	}
}

func TestIntegrationNoopAndRejectedFieldsIgnoreAt(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprint(rejected), func(t *testing.T) {
			old := integrationTestInitial(t, integrationTestFixture())
			old.Document.Version = "5e0"
			raw := integrationTestFixture()
			raw.Document.ID, raw.Document.LineCount = "efx7", "0"
			if rejected {
				raw.Document.Title, raw.Document.Subtitle = "Другое", nil
				*raw.Document.Pages[0].Title = "Иная"
			}
			candidate := integrationTestInitial(t, raw)
			plan := integrationTestPlan(t, old, candidate)
			decisions := make([]revision.Decision, len(plan.Fields))
			for i := range decisions {
				decisions[i] = revision.Reject
			}
			result := integrationTestApply(t, old, candidate, plan, decisions, false, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))
			if integrationTestJSON(t, result) != integrationTestJSON(t, old) {
				t.Fatal("no-op изменил старую дату, версию, lineCount либо содержание")
			}
			before := integrationTestJSON(t, old)
			*result.Document.Subtitle = "изменено"
			result.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Answers[0] = "изменено"
			if integrationTestJSON(t, old) != before {
				t.Fatal("no-op вернул общий снимок")
			}
		})
	}
}
