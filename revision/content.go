package revision

import (
	"math/big"
	"strconv"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

// applyContent координирует replaceLessonPages либо applyPageChanges.
// result первоначально является независимой копией old. revision — новая
// положительная глобальная ревизия десятичными цифрами без padding.
// Выбор ветви уже задан планом; нет сравнений содержимого или порогов.
func applyContent(result *lesson.Lesson, old *lesson.Lesson, candidate *lesson.Lesson, plan *compare.Plan, revision string, context diagnostic.Context) []*diagnostic.Error {
	if plan.ReplaceLesson {
		return replaceLessonPages(result, old, candidate, revision, context)
	}
	return applyPageChanges(result, old, candidate, plan.Pages, revision, context)
}

// replaceLessonPages копирует все страницы кандидата и вызывает assignPageIDs
// и assignNodeIDs с максимумов прежнего урока плюс 1. Старые названия урока
// и подзаголовок остаются в result; отдельно их меняет applyFields.
// Все страницы и все ноды получают новые ID, включая точные совпадения.
// Порядок страниц/нод кандидата сохраняется, пары старых и новых ID не строятся.
func replaceLessonPages(result *lesson.Lesson, old *lesson.Lesson, candidate *lesson.Lesson, revision string, context diagnostic.Context) []*diagnostic.Error {
	context.LessonID = old.Document.ID
	nextPage, nextNode := maxPageNumber(old.Document.Pages), maxNodeNumber(old.Document.Pages)
	nextPage.Add(nextPage, big.NewInt(1))
	nextNode.Add(nextNode, big.NewInt(1))
	result.Document.Pages = clonePages(candidate.Document.Pages)
	if problems := assignPageIDs(result.Document.Pages, old.Document.ID, nextPage, context); len(problems) != 0 {
		return problems
	}
	for i := range result.Document.Pages {
		page := &result.Document.Pages[i]
		pageContext := context
		pageContext.PageID = page.ID
		if problems := assignNodeIDs(page.Nodes, old.Document.ID, nextNode, revision, pageContext); len(problems) != 0 {
			for _, problem := range problems {
				for j := range problem.Details {
					if problem.Details[j].Label == "Поле JSON" {
						problem.Details[j].Value = "/document/pages/" + strconv.Itoa(i) + problem.Details[j].Value
					}
				}
			}
			return problems
		}
	}
	return nil
}

// applyPageChanges обрабатывает изменения страниц по индексу плана.
// Единственный nextNode для всех ReplaceNodes начинается с максимума
// базовых номеров ВСЕХ нод old плюс 1, включая удаляемые и вложенные ноды.
// Точечные замены не расходуют этот счётчик; номера не уплотняются.
// Вызывает replacePageNodes либо replaceNodeOwn для каждого изменения.
// ID всех страниц сохраняются; страницы вне плана остаются копиями old.
func applyPageChanges(result *lesson.Lesson, old *lesson.Lesson, candidate *lesson.Lesson, changes []compare.PageChange, revision string, context diagnostic.Context) []*diagnostic.Error {
	context.LessonID = old.Document.ID
	nextNode := maxNodeNumber(old.Document.Pages)
	nextNode.Add(nextNode, big.NewInt(1))
	for _, change := range changes {
		page := &result.Document.Pages[change.Index]
		pageContext := context
		pageContext.PageID = page.ID
		var problems []*diagnostic.Error
		if change.ReplaceNodes {
			problems = replacePageNodes(page, candidate.Document.Pages[change.Index], old.Document.ID, nextNode, revision, pageContext)
		} else {
			for _, nodeChange := range change.Nodes {
				if problem := replaceNodeOwn(page, old.Document.Pages[change.Index], candidate.Document.Pages[change.Index], nodeChange, old.Document.ID, revision, pageContext); problem != nil {
					problems = append(problems, problem)
					break
				}
			}
		}
		if len(problems) != 0 {
			for _, problem := range problems {
				for j := range problem.Details {
					if problem.Details[j].Label == "Поле JSON" {
						problem.Details[j].Value = "/document/pages/" + strconv.Itoa(change.Index) + problem.Details[j].Value
					}
				}
			}
			return problems
		}
	}
	return nil
}

// replacePageNodes копирует Nodes одной страницы кандидата, затем назначает
// всем нодам новые номера из общего nextNode и суффикс revision.
// ID и название страницы не меняет. Пустой лес не расходует счётчик.
// Названия позднее применяются через отдельные решения applyFields.
func replacePageNodes(result *lesson.Page, candidate lesson.Page, lessonID string, nextNode *big.Int, revision string, context diagnostic.Context) []*diagnostic.Error {
	context.LessonID = lessonID
	context.PageID = result.ID
	result.Nodes = cloneNodes(candidate.Nodes)
	return assignNodeIDs(result.Nodes, lessonID, nextNode, revision, context)
}

// replaceNodeOwn заменяет собственные Type, Flags и Data по адресу Path
// из кандидата, сохраняя дерево детей результата. Остальные собственные
// поля эквивалентны по неизменному плану; повторное сравнение не выполняется.
// Базовый номер берётся через identity.ParseNodeID старой ноды, новый ID —
// через identity.FormatNodeID с новой глобальной revision. Прежний суффикс
// заменяется, а не дописывается. Ноды без NodeChange сохраняют полный ID;
// изменение ребёнка само по себе не затрагивает собственные поля родителя.
func replaceNodeOwn(result *lesson.Page, old lesson.Page, candidate lesson.Page, change compare.NodeChange, lessonID string, revision string, context diagnostic.Context) *diagnostic.Error {
	context.LessonID = lessonID
	context.PageID = old.ID
	pointer := ""
	for _, index := range change.Path {
		pointer += "/nodes/" + strconv.Itoa(index)
	}
	pointer += "/id"
	oldNode := nodeAt(old.Nodes, change.Path)
	parts, problem := identity.ParseNodeID(oldNode.ID)
	if problem != nil {
		problem.Context = context
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: pointer})
		return problem
	}
	id, problem := identity.FormatNodeID(lessonID, parts.Number, revision)
	if problem != nil {
		problem.Context = context
		problem.Details = append(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: pointer})
		return problem
	}
	candidateNode := nodeAt(candidate.Nodes, change.Path)
	resultNode := nodeAt(result.Nodes, change.Path)
	resultNode.ID, resultNode.Type = id, candidateNode.Type
	resultNode.Flags = candidateNode.Flags
	if candidateNode.Flags.Speaking != nil {
		speaking := *candidateNode.Flags.Speaking
		resultNode.Flags.Speaking = &speaking
	}
	if candidateNode.Flags.Stepped != nil {
		stepped := *candidateNode.Flags.Stepped
		resultNode.Flags.Stepped = &stepped
	}
	if candidateNode.Flags.Checkable != nil {
		checkable := *candidateNode.Flags.Checkable
		resultNode.Flags.Checkable = &checkable
	}
	resultNode.Data = cloneData(candidateNode.Data)
	return nil
}

// nodeAt возвращает ноду по проверенной непустой цепочке индексов Path.
// Не использует ID для соответствия, не ограничивает глубину дерева.
func nodeAt(nodes []lesson.Node, path []int) *lesson.Node {
	var result *lesson.Node
	for _, index := range path {
		if index < 0 || index >= len(nodes) {
			return nil
		}
		result = &nodes[index]
		nodes = result.Nodes
	}
	return result
}
