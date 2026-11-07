package revision_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/revision"
)

func TestIntegrationReplacePagesUsesGlobalHistoricalMaximum(t *testing.T) {
	old := integrationTestInitial(t, integrationTestFixture())
	old.Document.Version = "99"
	old.Document.Pages[0].ID = "efx7-p010"
	old.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n18446744073709551616-v03"
	raw := integrationTestFixture()
	added := integrationTestText("Новый ребёнок")
	role := lesson.RoleBody
	added.Data = []lesson.Data{{Type: lesson.DataCollection, Role: &role, Answers: []string{},
		Value: []lesson.Data{{Type: lesson.DataCollection, Answers: []string{}, Value: []lesson.Data{
			{Type: lesson.DataText, Value: "глубоко", Answers: []string{"да"}}}}}}}
	raw.Document.Pages[0].Nodes[0].Nodes = append(raw.Document.Pages[0].Nodes[0].Nodes, added)
	raw.Document.Pages[1].Nodes = append(raw.Document.Pages[1].Nodes, integrationTestText("Новый корень"))
	candidate := integrationTestInitial(t, raw)
	plan := integrationTestPlan(t, old, candidate)
	if plan.ReplaceLesson || len(plan.Pages) != 2 || !plan.Pages[0].ReplaceNodes || !plan.Pages[1].ReplaceNodes {
		t.Fatalf("ожидались две замены страниц: %+v", plan)
	}
	result := integrationTestApply(t, old, candidate, plan, nil, true, integrationTestTime())
	if result.Document.Version != "100" || result.Document.Pages[0].ID != "efx7-p010" || result.Document.Pages[1].ID != old.Document.Pages[1].ID {
		t.Fatal("не сохранены страницы либо нет единого прироста ревизии")
	}
	nodes := []lesson.Node{result.Document.Pages[0].Nodes[0], result.Document.Pages[0].Nodes[0].Nodes[0],
		result.Document.Pages[0].Nodes[0].Nodes[1], result.Document.Pages[1].Nodes[0], result.Document.Pages[1].Nodes[1]}
	for i, node := range nodes {
		want := fmt.Sprintf("efx7-n184467440737095516%d-v100", 17+i)
		if node.ID != want {
			t.Errorf("preorder[%d]=%s, want %s", i, node.ID, want)
		}
	}
	before := integrationTestInputs(t, old, candidate, plan, nil)
	nested := result.Document.Pages[0].Nodes[0].Nodes[1].Data[0].Value.([]lesson.Data)[0].Value.([]lesson.Data)
	nested[0].Answers[0] = "изменено"
	if integrationTestInputs(t, old, candidate, plan, nil) != before {
		t.Fatal("вложенные Data.Value/Answers разделяют память")
	}
}

func TestIntegrationReplaceLessonUnknownRejectAndKnownNull(t *testing.T) {
	oldRaw := integrationTestFixture()
	oldRaw.Document.Pages[0].Title = nil
	old := integrationTestInitial(t, oldRaw)
	old.Document.Version = "999999999999999999999999.0"
	old.Document.Pages[0].ID = "efx7-p010"
	old.Document.Pages[1].ID = "efx7-p18446744073709551616"
	old.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n18446744073709551616-v03"
	raw := integrationTestFixture()
	raw.Document.ID, raw.Document.Title = "eFX7", "Принятое название"
	raw.Document.Subtitle = nil
	first, second := raw.Document.Pages[0], raw.Document.Pages[1]
	known, unknown := "Новое для известного null", "Новое для неизвестного"
	first.Title = &known
	raw.Document.Pages = []lesson.Page{second, first,
		{Title: &unknown, Nodes: []lesson.Node{integrationTestText("Новая страница")}},
		{Nodes: []lesson.Node{integrationTestText("Ещё одна новая")}}}
	candidate := integrationTestInitial(t, raw)
	plan := integrationTestPlan(t, old, candidate)
	if !plan.ReplaceLesson || len(plan.Pages) != 0 {
		t.Fatalf("нет полной замены: %+v", plan)
	}
	decisions := make([]revision.Decision, len(plan.Fields))
	knownNull, unknownText, unknownNull := false, false, false
	for i, field := range plan.Fields {
		decisions[i] = revision.Reject
		if field.Field == "document.title" {
			decisions[i] = revision.Accept
		}
		if field.Field == "page.title" {
			switch field.NewPage {
			case 1:
				knownNull = field.OldPage == 0 && field.Before == nil && field.After != nil
			case 2:
				unknownText = field.OldPage == -1 && field.Before == nil && field.After != nil
			case 3:
				unknownNull = field.OldPage == -1 && field.Before == nil && field.After == nil
				decisions[i] = revision.Accept
			}
		}
	}
	if !knownNull || !unknownText || !unknownNull {
		t.Fatalf("реальный compare не предоставил требуемые независимые поля: %+v", plan.Fields)
	}
	result := integrationTestApply(t, old, candidate, plan, decisions, true, integrationTestTime())
	version := "1000000000000000000000000"
	if result.Document.Version.String() != version || result.Document.ID != "EfX7" || result.Document.Title != "Принятое название" || *result.Document.Subtitle != "Подзаголовок" {
		t.Fatal("полная замена нарушила решения документа либо точное huge+1")
	}
	for i, page := range result.Document.Pages {
		if page.ID != fmt.Sprintf("efx7-p184467440737095516%d", 17+i) || page.Title != nil {
			t.Errorf("страница[%d]: ID=%s, title=%v", i, page.ID, page.Title)
		}
	}
	nodes := []lesson.Node{result.Document.Pages[0].Nodes[0], result.Document.Pages[1].Nodes[0],
		result.Document.Pages[1].Nodes[0].Nodes[0], result.Document.Pages[2].Nodes[0], result.Document.Pages[3].Nodes[0]}
	for i, node := range nodes {
		if want := fmt.Sprintf("efx7-n184467440737095516%d-v%s", 17+i, version); node.ID != want {
			t.Errorf("нода[%d]=%s, want %s", i, node.ID, want)
		}
	}
}

func TestIntegrationAcceptedTitleAloneIncrementsOnce(t *testing.T) {
	old := integrationTestInitial(t, integrationTestFixture())
	raw := integrationTestFixture()
	*raw.Document.Pages[0].Title = "Принято"
	raw.Document.LineCount = "0.000e999999999999999999999999999999999999"
	candidate := integrationTestInitial(t, raw)
	plan := integrationTestPlan(t, old, candidate)
	if plan.HasContentChanges() || len(plan.Fields) != 1 {
		t.Fatalf("план одного названия: %+v", plan)
	}
	result := integrationTestApply(t, old, candidate, plan, []revision.Decision{revision.Accept}, true, integrationTestTime().Add(time.Hour))
	if result.Document.Version != "2" || result.Document.CreatedAt != "2026-11-07T10:34:56Z" || result.Document.LineCount != raw.Document.LineCount || result.Document.Pages[0].Nodes[0].ID != "efx7-n001" {
		t.Fatal("принятое поле не обновило только служебные метаданные")
	}
}

func TestIntegrationAtomicErrorsPreserveInputs(t *testing.T) {
	for _, scenario := range []string{"нет плана", "нет решения", "пустое решение", "лишнее решение", "неизвестное решение", "другой урок", "история", "неверная дата"} {
		t.Run(scenario, func(t *testing.T) {
			old := integrationTestInitial(t, integrationTestFixture())
			raw := integrationTestFixture()
			raw.Document.Title = "Новое название"
			wantCode := revision.CodeDecision
			at := integrationTestTime()
			if scenario == "другой урок" {
				raw.Document.ID = "Другой"
				wantCode = revision.CodeLessonMismatch
			}
			if scenario == "история" {
				old.Document.Version = "5"
				old.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n001-v03"
				wantCode = revision.CodeHistory
			}
			candidate := integrationTestInitial(t, raw)
			plan := integrationTestPlan(t, old, candidate)
			decisions := []revision.Decision{revision.Accept}
			switch scenario {
			case "нет плана":
				plan, wantCode = nil, revision.CodePlan
			case "нет решения":
				decisions = nil
			case "пустое решение":
				decisions[0] = ""
			case "лишнее решение":
				decisions = append(decisions, revision.Reject)
			case "неизвестное решение":
				decisions[0] = "неизвестно"
			case "неверная дата":
				at, wantCode = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), lesson.CodeContract
			}
			before := integrationTestInputs(t, old, candidate, plan, decisions)
			result, changed, problems := revision.Apply(old, candidate, plan, decisions, at,
				diagnostic.Context{Path: "старый.json"}, diagnostic.Context{Path: "новый.json"})
			if result != nil || changed || len(problems) == 0 {
				t.Fatalf("неатомарный отказ: result=%v changed=%v errors=%v", result, changed, problems)
			}
			found := false
			for _, problem := range problems {
				if problem.Code == wantCode && problem.Reason != "" && len(problem.Details) != 0 {
					found = true
				}
			}
			if !found {
				t.Fatalf("нет структурированной ошибки %s: %v", wantCode, problems)
			}
			if integrationTestInputs(t, old, candidate, plan, decisions) != before {
				t.Fatal("ошибка изменила входы")
			}
		})
	}
}

func TestIntegrationInitialAtomicValidation(t *testing.T) {
	for _, scenario := range []string{"nil", "неверная версия формата", "null nodes", "неверный ID"} {
		t.Run(scenario, func(t *testing.T) {
			raw := integrationTestFixture()
			wantCode := lesson.CodeContract
			switch scenario {
			case "nil":
				raw, wantCode = nil, revision.CodeInput
			case "неверная версия формата":
				raw.FormatVersion = "2.0"
			case "null nodes":
				raw.Document.Pages[0].Nodes[0].Nodes = nil
			case "неверный ID":
				raw.Document.ID, wantCode = "два слова", "identity.invalid_lesson_id"
			}
			before := integrationTestJSON(t, raw)
			result, problems := revision.Initial(raw, integrationTestTime(), diagnostic.Context{Path: "исходник.json"})
			if result != nil || len(problems) == 0 {
				t.Fatalf("Initial опубликовал ошибочный результат: %v", problems)
			}
			found := false
			for _, problem := range problems {
				found = found || problem.Code == wantCode
			}
			if !found || integrationTestJSON(t, raw) != before {
				t.Fatalf("нет кода %s либо изменён вход: %v", wantCode, problems)
			}
		})
	}
}
