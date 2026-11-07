package compare

import (
	"fmt"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func task12Text(value string) *string { return &value }

func task12SameText(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func task12Fields(t *testing.T, got, want []FieldChange) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("число предложений = %d, нужно %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Field != w.Field || g.OldPage != w.OldPage || g.NewPage != w.NewPage || !task12SameText(g.Before, w.Before) || !task12SameText(g.After, w.After) {
			t.Errorf("предложение %d = %+v, нужно %+v", i, g, w)
		}
	}
}

func task12Document(t *testing.T, title string, subtitle *string, pages []lesson.Page) lesson.Document {
	t.Helper()
	doc := lesson.Document{ID: "QA12", Version: "1", Title: title, Subtitle: subtitle, LineCount: "0", CreatedAt: "2026-11-06T00:00:00Z", Pages: pages}
	if problems := lesson.Validate(&lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: doc}, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("невалидная fixture: %v", problems)
	}
	return doc
}

func task12Pages(t *testing.T, contents []string, titles ...*string) []lesson.Page {
	t.Helper()
	pages := make([]lesson.Page, len(contents))
	for i, content := range contents {
		role := lesson.RoleBody
		pages[i] = lesson.Page{ID: fmt.Sprintf("qa12-p%03d", i+1), Title: titles[i], Nodes: []lesson.Node{{
			ID: fmt.Sprintf("qa12-n%03d", i+1), Type: lesson.NodeText, Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: content, Answers: []string{}}}, Nodes: []lesson.Node{},
		}}}
	}
	task12Document(t, "", nil, pages)
	return pages
}

func TestTask12OptionalTextExact(t *testing.T) {
	values := []*string{nil, task12Text(""), task12Text("Текст"), task12Text("текст"), task12Text("Текст "), task12Text("Текст\n")}
	for i, left := range values {
		for j, right := range values {
			t.Run(fmt.Sprintf("%d_%d", i, j), func(t *testing.T) {
				if got := equalOptionalText(left, right); got != (i == j) {
					t.Errorf("равенство = %v, нужно %v", got, i == j)
				}
			})
		}
	}
	if !equalOptionalText(task12Text("одинаково"), task12Text("одинаково")) {
		t.Error("адреса равных строк не должны влиять на равенство")
	}
}

func TestTask12DocumentFieldsExactAndOrdered(t *testing.T) {
	for _, pair := range [][2]string{{"", ""}, {"", "Название"}, {"Название", ""}, {"Title", "title"}, {"Title", "Title "}, {"A\nB", "A B"}} {
		t.Run(fmt.Sprintf("title_%q_%q", pair[0], pair[1]), func(t *testing.T) {
			old := task12Document(t, pair[0], nil, []lesson.Page{})
			candidate := task12Document(t, pair[1], nil, []lesson.Page{})
			var want []FieldChange
			if pair[0] != pair[1] {
				want = []FieldChange{{DocumentTitle, -1, -1, task12Text(pair[0]), task12Text(pair[1])}}
			}
			task12Fields(t, compareDocumentFields(old, candidate), want)
		})
	}
	values := []*string{nil, task12Text(""), task12Text("Text"), task12Text("text"), task12Text("Text ")}
	for i, before := range values {
		for j, after := range values {
			t.Run(fmt.Sprintf("subtitle_%d_%d", i, j), func(t *testing.T) {
				old := task12Document(t, "старое", before, []lesson.Page{})
				candidate := task12Document(t, "новое", after, []lesson.Page{})
				want := []FieldChange{{DocumentTitle, -1, -1, task12Text("старое"), task12Text("новое")}}
				if i != j {
					want = append(want, FieldChange{DocumentSubtitle, -1, -1, before, after})
				}
				task12Fields(t, compareDocumentFields(old, candidate), want)
			})
		}
	}
	old := task12Document(t, "", task12Text("same"), []lesson.Page{})
	candidate := task12Document(t, "", task12Text("same"), []lesson.Page{})
	candidate.Version, candidate.LineCount, candidate.CreatedAt = "2", "99", "2026-11-07T00:00:00Z"
	task12Fields(t, compareDocumentFields(old, candidate), nil)
}

func TestTask12PageTitlesKnownExact(t *testing.T) {
	values := []*string{nil, task12Text(""), task12Text("Text"), task12Text("text"), task12Text("Text ")}
	for _, replacement := range []bool{false, true} {
		for i, before := range values {
			for j, after := range values {
				t.Run(fmt.Sprintf("replacement_%v_%d_%d", replacement, i, j), func(t *testing.T) {
					old := task12Pages(t, []string{"A"}, before)
					candidate := task12Pages(t, []string{"A"}, after)
					var want []FieldChange
					if i != j {
						want = []FieldChange{{PageTitle, 0, 0, before, after}}
					}
					task12Fields(t, comparePageTitles(old, candidate, pageMatches{[][]int{{0}}, [][]int{{0}}}, replacement), want)
				})
			}
		}
	}
	old := task12Pages(t, []string{"A", "B", "C"}, task12Text("a"), nil, task12Text("c"))
	candidate := task12Pages(t, []string{"X", "Y", "Z"}, task12Text("A"), task12Text(""), task12Text("c"))
	// Нет совпадений содержимого: при сохранённой последовательности поля позиционные.
	matches := pageMatches{make([][]int, 3), make([][]int, 3)}
	task12Fields(t, comparePageTitles(old, candidate, matches, false), []FieldChange{
		{PageTitle, 0, 0, task12Text("a"), task12Text("A")}, {PageTitle, 1, 1, nil, task12Text("")},
	})
}

func TestTask12ReplacementPageTitlesSafePairsOnly(t *testing.T) {
	cases := []struct {
		name           string
		old, candidate []lesson.Page
		matches        pageMatches
		want           []FieldChange
	}{
		{"moved_unique_and_new_nil", task12Pages(t, []string{"A", "B", "C"}, task12Text("a"), task12Text("foreign"), nil),
			task12Pages(t, []string{"C", "A", "X"}, task12Text("c"), task12Text("a"), nil),
			pageMatches{[][]int{{2}, {0}, {}}, [][]int{{1}, {}, {0}}},
			[]FieldChange{{PageTitle, 2, 0, nil, task12Text("c")}, {PageTitle, -1, 2, nil, nil}}},
		{"duplicates_both_sides", task12Pages(t, []string{"A", "A", "B"}, task12Text("a0"), task12Text("a1"), task12Text("b")),
			task12Pages(t, []string{"A", "B", "A", "X"}, task12Text("a0"), task12Text("b"), nil, task12Text("b")),
			pageMatches{[][]int{{0, 1}, {2}, {0, 1}, {}}, [][]int{{0, 2}, {0, 2}, {1}}},
			[]FieldChange{{PageTitle, -1, 0, nil, task12Text("a0")}, {PageTitle, -1, 2, nil, nil}, {PageTitle, -1, 3, nil, task12Text("b")}}},
		{"old_unique_new_duplicate", task12Pages(t, []string{"A", "B"}, task12Text("same"), nil),
			task12Pages(t, []string{"A", "A"}, task12Text("same"), nil),
			pageMatches{[][]int{{0}, {0}}, [][]int{{0, 1}, {}}},
			[]FieldChange{{PageTitle, -1, 0, nil, task12Text("same")}, {PageTitle, -1, 1, nil, nil}}},
		{"old_duplicate_new_unique", task12Pages(t, []string{"A", "A"}, task12Text("a"), task12Text("a")),
			task12Pages(t, []string{"A"}, task12Text("a")), pageMatches{[][]int{{0, 1}}, [][]int{{0}, {0}}},
			[]FieldChange{{PageTitle, -1, 0, nil, task12Text("a")}}},
		{"same_position_title_id_are_not_evidence", task12Pages(t, []string{"A"}, task12Text("same")),
			task12Pages(t, []string{"X"}, task12Text("same")), pageMatches{[][]int{{}}, [][]int{{}}},
			[]FieldChange{{PageTitle, -1, 0, nil, task12Text("same")}}},
		{"removed_page_has_no_field", task12Pages(t, []string{"A", "B"}, nil, task12Text("removed")),
			task12Pages(t, []string{"A"}, nil), pageMatches{[][]int{{0}}, [][]int{{0}, {}}}, nil},
		{"all_removed", task12Pages(t, []string{"A"}, task12Text("removed")),
			[]lesson.Page{}, pageMatches{[][]int{}, [][]int{{}}}, nil},
		{"first_new_page", []lesson.Page{}, task12Pages(t, []string{"X"}, task12Text("")),
			pageMatches{[][]int{{}}, [][]int{}}, []FieldChange{{PageTitle, -1, 0, nil, task12Text("")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task12Fields(t, comparePageTitles(tc.old, tc.candidate, tc.matches, true), tc.want)
		})
	}
}

func TestTask12FieldsOwnCopiesAndDoNotApplyDecisions(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(fmt.Sprintf("replacement_%v", replacement), func(t *testing.T) {
			before, after := task12Text("before"), task12Text("after")
			oldPages := task12Pages(t, []string{"A", "B"}, before, before)
			newPages := task12Pages(t, []string{"A", "B"}, after, after)
			old := task12Document(t, "before", before, oldPages)
			candidate := task12Document(t, "after", after, newPages)
			docFields := compareDocumentFields(old, candidate)
			pageFields := comparePageTitles(oldPages, newPages, pageMatches{[][]int{{0}, {1}}, [][]int{{0}, {1}}}, replacement)
			fields := append(docFields, pageFields...)
			task12Fields(t, fields, []FieldChange{
				{DocumentTitle, -1, -1, before, after}, {DocumentSubtitle, -1, -1, before, after},
				{PageTitle, 0, 0, before, after}, {PageTitle, 1, 1, before, after},
			})
			if *before != "before" || *after != "after" || oldPages[0].Title != before || newPages[1].Title != after {
				t.Fatal("сравнение применило изменение к исходным названиям")
			}
			var pointers []*string
			for i, field := range fields {
				for _, pointer := range []*string{field.Before, field.After} {
					if pointer == nil || pointer == before || pointer == after {
						t.Fatalf("поле %d не владеет отдельной копией", i)
					}
					for _, seen := range pointers {
						if pointer == seen {
							t.Fatal("предложения разделяют изменяемую строку")
						}
					}
					pointers = append(pointers, pointer)
				}
			}
			*before, *after = "input old changed", "input new changed"
			for i, field := range fields {
				if *field.Before != "before" || *field.After != "after" {
					t.Fatalf("изменение снимка изменило предложение %d", i)
				}
			}
			for i, pointer := range pointers {
				*pointer = fmt.Sprintf("result %d", i)
			}
			if old.Title != "before" || candidate.Title != "after" || *before != "input old changed" || *after != "input new changed" || oldPages[0].ID != "qa12-p001" || newPages[1].ID != "qa12-p002" {
				t.Fatal("сравнение или изменение результата изменило исходные поля/ID")
			}
		})
	}
}

func TestTask12UnknownPageAfterIsAnIndependentCopy(t *testing.T) {
	after := task12Text("new")
	old := task12Pages(t, []string{"A"}, task12Text("old"))
	candidate := task12Pages(t, []string{"B", "B"}, after, after)
	matches := pageMatches{[][]int{{}, {}}, [][]int{{}}}
	fields := comparePageTitles(old, candidate, matches, true)
	task12Fields(t, fields, []FieldChange{{PageTitle, -1, 0, nil, task12Text("new")}, {PageTitle, -1, 1, nil, task12Text("new")}})
	if fields[0].After == after || fields[1].After == after || fields[0].After == fields[1].After {
		t.Fatal("неизвестные прежние поля разделяют изменяемое After")
	}
	*fields[0].After = "result changed"
	if *after != "new" || *fields[1].After != "new" || *old[0].Title != "old" {
		t.Fatal("изменение одного предложения изменило снимок или другое предложение")
	}
	*after = "input changed"
	if *fields[1].After != "new" {
		t.Fatal("изменение кандидата изменило независимое предложение")
	}
}
