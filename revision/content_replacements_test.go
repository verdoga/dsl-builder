package revision

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

// contentTestPrimary задаёт независимые первичные ID без функций revision.
func contentTestPrimary(value *lesson.Lesson) {
	next := 1
	var walk func([]lesson.Node)
	walk = func(nodes []lesson.Node) {
		for i := range nodes {
			nodes[i].ID = fmt.Sprintf("a:b-n%03d", next)
			next++
			walk(nodes[i].Nodes)
		}
	}
	value.Document.Version = "1"
	for i := range value.Document.Pages {
		value.Document.Pages[i].ID = fmt.Sprintf("a:b-p%03d", i+1)
		walk(value.Document.Pages[i].Nodes)
	}
}

func TestContentReplacePageNodesPreservesPageAndSharesCounter(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint(empty), func(t *testing.T) {
			old := contentTestLesson(contentTestPage("a:b-p999", "прежнее название", contentTestNode("a:b-n008", "старое")))
			candidate := contentTestLesson(contentTestPage("a:b-p001", "новое название"))
			if !empty {
				candidate.Document.Pages[0].Nodes = []lesson.Node{
					contentTestNode("a:b-n001", " \tЁ\r\n\n{a|b} ", contentTestNode("a:b-n002", "вложено")), contentTestNode("a:b-n003", "конец")}
			}
			contentTestValid(t, old, candidate)
			oldBefore, candidateBefore := contentTestSnapshot(t, old), contentTestSnapshot(t, candidate)
			result, want := contentTestCopy(t, old), contentTestCopy(t, candidate)
			want.Document.Pages[0].ID, want.Document.Pages[0].Title = old.Document.Pages[0].ID, old.Document.Pages[0].Title
			next, _ := new(big.Int).SetString("184467440737095516160000", 10)
			wantNext := "184467440737095516160000"
			if !empty {
				want.Document.Pages[0].Nodes[0].ID = "a:b-n184467440737095516160000-v100"
				want.Document.Pages[0].Nodes[0].Nodes[0].ID = "a:b-n184467440737095516160001-v100"
				want.Document.Pages[0].Nodes[1].ID = "a:b-n184467440737095516160002-v100"
				wantNext = "184467440737095516160003"
			}
			if problems := replacePageNodes(&result.Document.Pages[0], candidate.Document.Pages[0], old.Document.ID, next, "100", diagnostic.Context{}); len(problems) != 0 {
				t.Fatal(problems)
			}
			contentTestEqual(t, result.Document.Pages[0], want.Document.Pages[0])
			if next.String() != wantNext || result.Document.Pages[0].Nodes == nil {
				t.Error("Неверный общий счётчик или потерян пустой ненулевой лес")
			}
			contentTestDamage(result.Document.Pages[0].Nodes)
			if contentTestSnapshot(t, old) != oldBefore || contentTestSnapshot(t, candidate) != candidateBefore {
				t.Error("Полная замена страницы разделяет память со снимками")
			}
		})
	}
}

func TestContentMixedPagesUseOldGlobalMaximum(t *testing.T) {
	for _, maximumAt := range []string{"вложенный удаляемый", "другая страница", "удаляемый корень"} {
		for _, viaContent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/content=%t", maximumAt, viaContent), func(t *testing.T) {
				old := contentTestLesson(
					contentTestPage("a:b-p017", "точечно", contentTestNode("a:b-n081-v03", "родитель", contentTestNode("a:b-n035", "ребёнок"))),
					contentTestPage("a:b-p999", "первая замена", contentTestNode("a:b-n900", "удалено", contentTestNode("a:b-n901-v07", "вложенный удалённый"))),
					contentTestPage("a:b-p003", "вторая замена", contentTestNode("a:b-n902", "удаляемый корень")),
					contentTestPage("a:b-p1001", "не трогать", contentTestNode("a:b-n903-v04", "неизменное")))
				switch maximumAt {
				case "вложенный удаляемый":
					old.Document.Pages[1].Nodes[0].Nodes[0].ID = "a:b-n184467440737095516160000-v07"
				case "другая страница":
					old.Document.Pages[3].Nodes[0].ID = "a:b-n184467440737095516160000-v04"
				case "удаляемый корень":
					old.Document.Pages[2].Nodes[0].ID = "a:b-n184467440737095516160000"
				}
				candidate := contentTestCopy(t, old)
				candidate.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value = "замена ребёнка"
				candidate.Document.Pages[1].Nodes = []lesson.Node{contentTestNode("a:b-n001", "новый один", contentTestNode("a:b-n002", "новый ребёнок")), contentTestNode("a:b-n003", "новый два")}
				candidate.Document.Pages[2].Nodes = []lesson.Node{contentTestNode("a:b-n004", "новый три", contentTestNode("a:b-n005", "новый потомок"))}
				*candidate.Document.Pages[1].Title = "ещё не принятое название"
				contentTestPrimary(candidate)
				plan := contentTestPlan(t, old, candidate)
				if plan.ReplaceLesson || len(plan.Pages) != 3 || plan.Pages[0].ReplaceNodes || !plan.Pages[1].ReplaceNodes || !plan.Pages[2].ReplaceNodes {
					t.Fatal("Ожидался смешанный план с двумя заменами страниц")
				}
				oldBefore, candidateBefore, planBefore := contentTestSnapshot(t, old), contentTestSnapshot(t, candidate), contentTestSnapshot(t, plan)
				result, want := contentTestCopy(t, old), contentTestCopy(t, old)
				want.Document.Pages[0].Nodes[0].Nodes[0].ID = "a:b-n035-v100"
				want.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value = "замена ребёнка"
				fresh := contentTestCopy(t, candidate)
				want.Document.Pages[1].Nodes, want.Document.Pages[2].Nodes = fresh.Document.Pages[1].Nodes, fresh.Document.Pages[2].Nodes
				want.Document.Pages[1].Nodes[0].ID = "a:b-n184467440737095516160001-v100"
				want.Document.Pages[1].Nodes[0].Nodes[0].ID = "a:b-n184467440737095516160002-v100"
				want.Document.Pages[1].Nodes[1].ID = "a:b-n184467440737095516160003-v100"
				want.Document.Pages[2].Nodes[0].ID = "a:b-n184467440737095516160004-v100"
				want.Document.Pages[2].Nodes[0].Nodes[0].ID = "a:b-n184467440737095516160005-v100"
				var problems []*diagnostic.Error
				if viaContent {
					problems = applyContent(result, old, candidate, plan, "100", diagnostic.Context{})
				} else {
					problems = applyPageChanges(result, old, candidate, plan.Pages, "100", diagnostic.Context{})
				}
				if len(problems) != 0 {
					t.Fatal(problems)
				}
				contentTestValid(t, result)
				contentTestEqual(t, result, want)
				for i := range result.Document.Pages {
					contentTestDamage(result.Document.Pages[i].Nodes)
					*result.Document.Pages[i].Title = "изменено"
				}
				if contentTestSnapshot(t, old) != oldBefore || contentTestSnapshot(t, candidate) != candidateBefore || contentTestSnapshot(t, plan) != planBefore {
					t.Error("Результат связан со снимками или план изменён")
				}
			})
		}
	}
}

func TestContentFullReplacementRenumbersEvenMatchingPage(t *testing.T) {
	for _, viaContent := range []bool{false, true} {
		t.Run(fmt.Sprint(viaContent), func(t *testing.T) {
			old := contentTestLesson(
				contentTestPage("a:b-p003", "точное совпадение", contentTestNode("a:b-n081-v03", "тот же текст", contentTestNode("a:b-n035", "те же дети"))),
				contentTestPage("a:b-p184467440737095516160000", "удалена", contentTestNode("a:b-n007", "удалена", contentTestNode("a:b-n184467440737095516160000-v09", "максимум"))))
			candidate := contentTestCopy(t, old)
			candidate.Document.Pages = append(candidate.Document.Pages[:1], contentTestPage("a:b-p002", "новая", contentTestNode("a:b-n001", "новые данные")), contentTestPage("a:b-p003", "пустая"))
			candidate.Document.ID, candidate.Document.Title, *candidate.Document.Subtitle = "a:b", "новое название", "новый подзаголовок"
			candidate.Document.LineCount = "987654321098765432109876543210.00"
			contentTestPrimary(candidate)
			plan := contentTestPlan(t, old, candidate)
			if !plan.ReplaceLesson || len(plan.Pages) != 0 {
				t.Fatal("Добавление страницы должно дать полную замену")
			}
			oldBefore, candidateBefore, planBefore := contentTestSnapshot(t, old), contentTestSnapshot(t, candidate), contentTestSnapshot(t, plan)
			result, want := contentTestCopy(t, old), contentTestCopy(t, old)
			want.Document.Pages = contentTestCopy(t, candidate).Document.Pages
			want.Document.Pages[0].ID = "a:b-p184467440737095516160001"
			want.Document.Pages[1].ID = "a:b-p184467440737095516160002"
			want.Document.Pages[2].ID = "a:b-p184467440737095516160003"
			want.Document.Pages[0].Nodes[0].ID = "a:b-n184467440737095516160001-v100"
			want.Document.Pages[0].Nodes[0].Nodes[0].ID = "a:b-n184467440737095516160002-v100"
			want.Document.Pages[1].Nodes[0].ID = "a:b-n184467440737095516160003-v100"
			var problems []*diagnostic.Error
			if viaContent {
				problems = applyContent(result, old, candidate, plan, "100", diagnostic.Context{})
			} else {
				problems = replaceLessonPages(result, old, candidate, "100", diagnostic.Context{})
			}
			if len(problems) != 0 {
				t.Fatal(problems)
			}
			contentTestValid(t, result)
			contentTestEqual(t, result, want)
			*result.Document.Subtitle = "повреждено"
			for i := range result.Document.Pages {
				*result.Document.Pages[i].Title = "повреждено"
				contentTestDamage(result.Document.Pages[i].Nodes)
			}
			if contentTestSnapshot(t, old) != oldBefore || contentTestSnapshot(t, candidate) != candidateBefore || contentTestSnapshot(t, plan) != planBefore {
				t.Error("Полная замена разделяет память со снимками или меняет план")
			}
		})
	}
}

func TestContentEmptyFullReplacementAndEmptyPlan(t *testing.T) {
	for _, emptyOld := range []bool{false, true} {
		old, candidate := contentTestLesson(), contentTestLesson()
		if emptyOld {
			candidate.Document.Pages = []lesson.Page{contentTestPage("a:b-p003", "новая", contentTestNode("a:b-n009", "новая"))}
		} else {
			old.Document.Pages = []lesson.Page{contentTestPage("a:b-p009", "старая", contentTestNode("a:b-n999", "старая"))}
		}
		plan := contentTestPlan(t, old, candidate)
		result := contentTestCopy(t, old)
		if problems := applyContent(result, old, candidate, plan, "100", diagnostic.Context{}); len(problems) != 0 {
			t.Fatal(problems)
		}
		if emptyOld {
			if result.Document.Pages[0].ID != "a:b-p001" || result.Document.Pages[0].Nodes[0].ID != "a:b-n001-v100" {
				t.Error("Пустой старый урок должен начинать независимые счётчики с 1")
			}
		} else if result.Document.Pages == nil || len(result.Document.Pages) != 0 {
			t.Error("Пустой кандидат должен дать пустой ненулевой массив страниц")
		}
		contentTestValid(t, result)
		same := contentTestCopy(t, old)
		emptyPlan := contentTestPlan(t, old, same)
		untouched := contentTestCopy(t, old)
		if problems := applyContent(untouched, old, same, emptyPlan, "100", diagnostic.Context{}); len(problems) != 0 {
			t.Fatal(problems)
		}
		contentTestEqual(t, untouched, old)
	}
}
