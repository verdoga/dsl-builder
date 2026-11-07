package revision

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

func numberingTestJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func numberingTestNodesWithoutIDs(nodes []lesson.Node) []lesson.Node {
	if nodes == nil {
		return nil
	}
	result := make([]lesson.Node, len(nodes))
	for i, node := range nodes {
		result[i] = node
		result[i].ID = ""
		result[i].Nodes = numberingTestNodesWithoutIDs(node.Nodes)
	}
	return result
}

func numberingTestNode(children ...lesson.Node) lesson.Node {
	role, checkable := lesson.RoleBody, false
	if children == nil {
		children = []lesson.Node{}
	}
	return lesson.Node{ID: "прежний ID", Type: lesson.NodeQuestion,
		Flags: lesson.NodeFlags{Checkable: &checkable}, Nodes: children,
		Data: []lesson.Data{{Type: lesson.DataText, Role: &role,
			Value: "Текст\n\n[ответ]", Answers: []string{"ответ", " ещё "}, Hint: "Подсказка"},
			{Type: lesson.DataCollection, Role: &role, Answers: []string{}, Value: []lesson.Data{
				{Type: lesson.DataMedia, Answers: []string{}, Value: lesson.EmbeddedFile{FileName: "звук.mp3", MIMEType: "audio/mpeg", Content: "0YLQtdGB0YI="}}}}}}
}

func numberingTestRequireIDs(t *testing.T, nodes []lesson.Node, want []string) {
	t.Helper()
	var got []string
	var visit func([]lesson.Node)
	visit = func(values []lesson.Node) {
		for _, node := range values {
			got = append(got, node.ID)
			visit(node.Nodes)
		}
	}
	visit(nodes)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ID в порядке родитель → дети: %v; нужны %v", got, want)
	}
}

func TestNumberingInitialIndependentCountersAndPreorder(t *testing.T) {
	title := " Страница "
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "A:B", Version: "17", Title: "Урок", LineCount: "123.0", CreatedAt: "прежняя дата",
		Pages: []lesson.Page{{Title: &title, Nodes: []lesson.Node{
			numberingTestNode(numberingTestNode(numberingTestNode()), numberingTestNode()), numberingTestNode()}},
			{Nodes: []lesson.Node{}}, {Nodes: []lesson.Node{numberingTestNode()}}}}}
	metadata := value.Document
	metadata.Pages = nil
	before := numberingTestJSON(t, metadata)
	contents := make([]string, len(value.Document.Pages))
	for i, page := range value.Document.Pages {
		contents[i] = numberingTestJSON(t, numberingTestNodesWithoutIDs(page.Nodes))
	}
	if problems := assignInitialIDs(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatal(problems)
	}
	for i, page := range value.Document.Pages {
		if page.ID != fmt.Sprintf("a:b-p%03d", i+1) || numberingTestJSON(t, numberingTestNodesWithoutIDs(page.Nodes)) != contents[i] {
			t.Fatalf("страница %d: ID или содержимое нарушены", i)
		}
	}
	numberingTestRequireIDs(t, value.Document.Pages[0].Nodes, []string{"a:b-n001", "a:b-n002", "a:b-n003", "a:b-n004", "a:b-n005"})
	numberingTestRequireIDs(t, value.Document.Pages[2].Nodes, []string{"a:b-n006"})
	metadata = value.Document
	metadata.Pages = nil
	if numberingTestJSON(t, metadata) != before || value.Document.Pages[0].Title != &title || title != " Страница " {
		t.Fatal("нумерация изменила метаданные или название страницы")
	}
	other := &lesson.Lesson{Document: lesson.Document{ID: "A:B", Pages: []lesson.Page{{Nodes: []lesson.Node{numberingTestNode()}}}}}
	if problems := assignInitialIDs(other, diagnostic.Context{}); len(problems) != 0 || other.Document.Pages[0].ID != "a:b-p001" || other.Document.Pages[0].Nodes[0].ID != "a:b-n001" {
		t.Fatalf("новый урок должен начинать независимые счётчики с 1: %v", problems)
	}
}

func TestNumberingInitialBeyond999(t *testing.T) {
	value := &lesson.Lesson{Document: lesson.Document{ID: "ЭФ+1", Pages: make([]lesson.Page, 1001)}}
	for i := range value.Document.Pages {
		value.Document.Pages[i].Nodes = []lesson.Node{numberingTestNode()}
	}
	if problems := assignInitialIDs(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatal(problems)
	}
	for i, page := range value.Document.Pages {
		if page.ID != fmt.Sprintf("эф+1-p%03d", i+1) || page.Nodes[0].ID != fmt.Sprintf("эф+1-n%03d", i+1) {
			t.Fatalf("номер %d обрезан или счётчик нод сброшен: %s, %s", i+1, page.ID, page.Nodes[0].ID)
		}
	}
}

func TestNumberingAssignCountersAndRevisionWidths(t *testing.T) {
	for _, start := range []string{"1", "999", "184467440737095516169999999999999999999999"} {
		for _, revision := range []string{"", "6", "100", "184467440737095516169999999999999"} {
			t.Run(start+"/"+revision, func(t *testing.T) {
				nextPage, _ := new(big.Int).SetString(start, 10)
				nextNode := new(big.Int).Set(nextPage)
				pages := []lesson.Page{{Nodes: []lesson.Node{numberingTestNode()}}, {Nodes: []lesson.Node{}}}
				beforePages := numberingTestJSON(t, pages)
				if problems := assignPageIDs(pages, "A:B", nextPage, diagnostic.Context{}); len(problems) != 0 {
					t.Fatal(problems)
				}
				for i := range pages {
					number, _ := new(big.Int).SetString(start, 10)
					number.Add(number, big.NewInt(int64(i)))
					if pages[i].ID != fmt.Sprintf("a:b-p%03s", number.String()) {
						t.Fatal(pages[i].ID)
					}
					pages[i].ID = ""
				}
				if numberingTestJSON(t, pages) != beforePages {
					t.Fatal("assignPageIDs изменил содержимое")
				}
				nodes := []lesson.Node{numberingTestNode(numberingTestNode(numberingTestNode()), numberingTestNode()), numberingTestNode()}
				before := numberingTestJSON(t, numberingTestNodesWithoutIDs(nodes))
				if problems := assignNodeIDs(nodes, "A:B", nextNode, revision, diagnostic.Context{}); len(problems) != 0 {
					t.Fatal(problems)
				}
				var ids []string
				number, _ := new(big.Int).SetString(start, 10)
				for i := 0; i < 5; i++ {
					id := fmt.Sprintf("a:b-n%03s", number.String())
					if revision != "" {
						id += fmt.Sprintf("-v%02s", revision)
					}
					ids = append(ids, id)
					number.Add(number, big.NewInt(1))
				}
				numberingTestRequireIDs(t, nodes, ids)
				pageEnd, _ := new(big.Int).SetString(start, 10)
				pageEnd.Add(pageEnd, big.NewInt(2))
				if nextNode.Cmp(number) != 0 || nextPage.Cmp(pageEnd) != 0 || numberingTestJSON(t, numberingTestNodesWithoutIDs(nodes)) != before {
					t.Fatal("счётчики не увеличены точно либо содержимое изменено")
				}
			})
		}
	}
}

func TestNumberingEmptyAssignmentsAndMaxima(t *testing.T) {
	for _, nodes := range [][]lesson.Node{nil, {}} {
		next := big.NewInt(123)
		if problems := assignNodeIDs(nodes, "A:B", next, "100", diagnostic.Context{}); len(problems) != 0 || next.String() != "123" {
			t.Fatalf("пустое дерево расходует счётчик: %v, %v", next, problems)
		}
		maximum := big.NewInt(77)
		maxNodeNumberInTree(nodes, maximum)
		if maximum.String() != "77" {
			t.Fatal("пустое дерево изменило максимум")
		}
	}
	for _, pages := range [][]lesson.Page{nil, {}, {{Nodes: []lesson.Node{}}}} {
		if len(pages) == 1 {
			pages[0].ID = "a:b-p001"
		}
		wantPage := "0"
		if len(pages) == 1 {
			wantPage = "1"
		}
		if maxPageNumber(pages).String() != wantPage || maxNodeNumber(pages).Sign() != 0 {
			t.Fatal("неверный максимум пустого леса")
		}
		if len(pages) == 0 {
			next := big.NewInt(42)
			if problems := assignPageIDs(pages, "A:B", next, diagnostic.Context{}); len(problems) != 0 || next.String() != "42" {
				t.Fatal("пустой массив страниц расходует счётчик")
			}
		}
	}
}

func TestNumberingMaximaAcrossAllPagesAndDepths(t *testing.T) {
	huge := "184467440737095516169999999999999999999999"
	for depth := 0; depth < 8; depth++ {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			node := numberingTestNode()
			node.ID = "a:b-n" + huge + "-v02"
			for level := 0; level < depth; level++ {
				parent := numberingTestNode(node)
				parent.ID = fmt.Sprintf("a:b-n%03d-v999999999999999999999999999999999999999999999999999", level+10)
				node = parent
			}
			low := numberingTestNode()
			low.ID = "a:b-n998"
			tail := numberingTestNode()
			tail.ID = "a:b-n997"
			pages := []lesson.Page{{ID: "a:b-p999", Nodes: []lesson.Node{low}},
				{ID: "a:b-p" + huge, Nodes: []lesson.Node{node, tail}}, {ID: "a:b-p001", Nodes: []lesson.Node{}}}
			old := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
				ID: "A:B", Version: "1e60", LineCount: "0", CreatedAt: "2026-01-02T03:04:05Z", Pages: pages}}
			if problems := lesson.Validate(old, diagnostic.Context{}); len(problems) != 0 {
				t.Fatalf("предусловие максимумов не выполнено: %v", problems)
			}
			before := numberingTestJSON(t, pages)
			if maxPageNumber(pages).String() != huge || maxNodeNumber(pages).String() != huge {
				t.Fatal("максимум не учитывает все страницы и глубины или включает суффикс")
			}
			maximum := big.NewInt(1000)
			maxNodeNumberInTree(pages[1].Nodes, maximum)
			if maximum.String() != huge {
				t.Fatal("переданный максимум не обновлён")
			}
			maximum.Add(maximum, big.NewInt(1))
			wantMaximum := maximum.String()
			maxNodeNumberInTree(pages[1].Nodes, maximum)
			if maximum.String() != wantMaximum {
				t.Fatal("больший входной максимум уменьшен")
			}
			if numberingTestJSON(t, pages) != before {
				t.Fatal("поиск максимума изменил старое дерево")
			}
		})
	}
}

func TestNumberingIdentityErrorsKeepCodeDetailsAndContext(t *testing.T) {
	t.Run("initial", func(t *testing.T) {
		context := diagnostic.Context{Path: "вход.json", LessonID: "плохой ID"}
		value := &lesson.Lesson{Document: lesson.Document{ID: context.LessonID,
			Pages: []lesson.Page{{Nodes: []lesson.Node{numberingTestNode()}}}}}
		problems := assignInitialIDs(value, context)
		if len(problems) == 0 {
			t.Fatal("первичная нумерация потеряла ошибку identity")
		}
		for _, problem := range problems {
			if problem == nil || problem.Code != identity.CodeInvalidLessonID || problem.Context.Path != context.Path || problem.Context.LessonID != context.LessonID {
				t.Fatalf("ошибка первичной нумерации: %+v", problem)
			}
		}
	})
	for _, tc := range []struct {
		name, lessonID, number, revision string
		page                             bool
	}{
		{"page lesson", "плохой ID", "1", "", true}, {"page zero", "A:B", "0", "", true},
		{"node lesson", "плохой ID", "1", "", false}, {"node zero", "A:B", "0", "", false},
		{"revision zero", "A:B", "1", "0", false}, {"revision invalid", "A:B", "1", "-2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			context := diagnostic.Context{Path: "архив.zip", FileName: "l7.json", LessonID: tc.lessonID, Line: 12, Column: 3}
			next, _ := new(big.Int).SetString(tc.number, 10)
			var problems []*diagnostic.Error
			var source *diagnostic.Error
			if tc.page {
				_, source = identity.FormatPageID(tc.lessonID, tc.number)
				problems = assignPageIDs([]lesson.Page{{Nodes: []lesson.Node{}}}, tc.lessonID, next, context)
			} else {
				context.PageID = "a:b-p123"
				_, source = identity.FormatNodeID(tc.lessonID, tc.number, tc.revision)
				problems = assignNodeIDs([]lesson.Node{numberingTestNode()}, tc.lessonID, next, tc.revision, context)
			}
			if len(problems) == 0 || problems[0] == nil || source == nil {
				t.Fatal("ошибка identity потеряна")
			}
			problem := problems[0]
			if problem.Code != source.Code || problem.Reason == "" || problem.Context != context {
				t.Fatalf("код или контекст не сохранён: %+v", problem)
			}
			for _, detail := range source.Details {
				found := false
				for _, actual := range problem.Details {
					found = found || actual == detail
				}
				if !found {
					t.Fatalf("исходная подробность потеряна: %+v", detail)
				}
			}
		})
	}
}
