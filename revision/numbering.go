package revision

import (
	"math/big"
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

// assignInitialIDs координирует assignPageIDs и assignNodeIDs с независимыми
// счётчиками с 1. Ноды всех страниц используют один nextNode и пустую revision.
// Prefix и минимальные ширины чисел определяет только identity.
func assignInitialIDs(result *lesson.Lesson, context diagnostic.Context) []*diagnostic.Error {
	context.LessonID = result.Document.ID
	nextPage, nextNode := big.NewInt(1), big.NewInt(1)
	if problems := assignPageIDs(result.Document.Pages, result.Document.ID, nextPage, context); len(problems) != 0 {
		return problems
	}
	for i := range result.Document.Pages {
		page := &result.Document.Pages[i]
		pageContext := context
		pageContext.PageID = page.ID
		if problems := assignNodeIDs(page.Nodes, result.Document.ID, nextNode, "", pageContext); len(problems) != 0 {
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

// assignPageIDs присваивает ID страницам в порядке массива через
// identity.FormatPageID, увеличивая nextPage после каждой страницы.
// Счётчик — math/big.Int, ограничения 999, int и uint64 не вводятся.
func assignPageIDs(pages []lesson.Page, lessonID string, nextPage *big.Int, context diagnostic.Context) []*diagnostic.Error {
	context.LessonID = lessonID
	one := big.NewInt(1)
	for i := range pages {
		id, problem := identity.FormatPageID(lessonID, nextPage.String())
		if problem != nil {
			problem.Context = context
			if pages[i].ID != "" {
				problem.Context.PageID = pages[i].ID
			}
			problem.Details = append(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: "/document/pages/" + strconv.Itoa(i) + "/id"})
			return []*diagnostic.Error{problem}
		}
		pages[i].ID = id
		nextPage.Add(nextPage, one)
	}
	return nil
}

// assignNodeIDs выдаёт базовые номера в порядке родитель → дети через
// identity.FormatNodeID и увеличивает общий nextNode. Применяется на всех
// уровнях Nodes; пустая revision даёт ID без суффикса, непустая — -vNN.
// Числа после 999 и версии после 99 не обрезаются. Содержимое не меняет.
func assignNodeIDs(nodes []lesson.Node, lessonID string, nextNode *big.Int, revision string, context diagnostic.Context) []*diagnostic.Error {
	context.LessonID = lessonID
	one := big.NewInt(1)
	for i := range nodes {
		pointer := "/nodes/" + strconv.Itoa(i)
		id, problem := identity.FormatNodeID(lessonID, nextNode.String(), revision)
		if problem != nil {
			problem.Context = context
			problem.Details = append(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: pointer + "/id"})
			return []*diagnostic.Error{problem}
		}
		nodes[i].ID = id
		nextNode.Add(nextNode, one)
		if problems := assignNodeIDs(nodes[i].Nodes, lessonID, nextNode, revision, context); len(problems) != 0 {
			for _, problem := range problems {
				for j := range problem.Details {
					if problem.Details[j].Label == "Поле JSON" {
						problem.Details[j].Value = pointer + problem.Details[j].Value
					}
				}
			}
			return problems
		}
	}
	return nil
}

// maxPageNumber разбирает ID всех страниц через identity.ParsePageID
// и возвращает наибольший номер как big.Int; пустой массив даёт 0.
// Перед вызовом lesson.Validate уже проверил ID и принадлежность уроку.
func maxPageNumber(pages []lesson.Page) *big.Int {
	maximum := new(big.Int)
	for _, page := range pages {
		parts, problem := identity.ParsePageID(page.ID)
		if problem != nil {
			continue
		}
		number, ok := new(big.Int).SetString(parts.Number, 10)
		if ok && number.Cmp(maximum) > 0 {
			maximum.Set(number)
		}
	}
	return maximum
}

// maxNodeNumber обходит все страницы и все уровни Nodes, используя
// maxNodeNumberInTree; возвращает максимум базовых номеров либо 0.
// Суффиксы не входят в номер. Максимум берётся из снимка до обновления.
func maxNodeNumber(pages []lesson.Page) *big.Int {
	maximum := new(big.Int)
	for _, page := range pages {
		maxNodeNumberInTree(page.Nodes, maximum)
	}
	return maximum
}

// maxNodeNumberInTree разбирает ID через identity.ParseNodeID и обновляет
// переданный максимум по всей ветви. Пропуски номеров не заполняет.
func maxNodeNumberInTree(nodes []lesson.Node, maximum *big.Int) {
	for _, node := range nodes {
		parts, problem := identity.ParseNodeID(node.ID)
		if problem == nil {
			number, ok := new(big.Int).SetString(parts.Number, 10)
			if ok && number.Cmp(maximum) > 0 {
				maximum.Set(number)
			}
		}
		maxNodeNumberInTree(node.Nodes, maximum)
	}
}
