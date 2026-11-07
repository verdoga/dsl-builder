package revision

import (
	"fmt"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func fieldsTestString(value string) *string { return &value }

func fieldsTestText(t *testing.T, got, want *string) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("наличие строки: получено %v, ожидалось %v", got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("строка: получено %q, ожидалось %q", *got, *want)
	}
}

func TestFieldsRequireExplicitDecisions(t *testing.T) {
	if Accept != Decision("accept") || Reject != Decision("reject") || Decision("") == Reject {
		t.Fatal("нарушены значения явных решений")
	}
	fields := []compare.FieldChange{
		{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: fieldsTestString("А"), After: fieldsTestString("Б")},
		{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1, After: fieldsTestString("")},
		{Field: compare.PageTitle, OldPage: 0, NewPage: 0, Before: fieldsTestString("А")},
		{Field: compare.PageTitle, OldPage: -1, NewPage: 1},
	}
	context := diagnostic.Context{Path: "урок.zip", FileName: "l014.json", LessonID: "Урок"}
	cases := []struct {
		name      string
		decisions []Decision
		valid     bool
	}{
		{"все приняты", []Decision{Accept, Accept, Accept, Accept}, true},
		{"все отклонены", []Decision{Reject, Reject, Reject, Reject}, true},
		{"смешанные", []Decision{Accept, Reject, Accept, Reject}, true},
		{"отсутствуют", nil, false},
		{"пустой массив", []Decision{}, false},
		{"не хватает", []Decision{Accept, Accept, Reject}, false},
		{"лишнее", []Decision{Accept, Accept, Reject, Reject, Accept}, false},
	}
	for index := range fields {
		for _, invalid := range []Decision{"", "ACCEPT", "yes", " accept "} {
			decisions := []Decision{Reject, Accept, Reject, Accept}
			decisions[index] = invalid
			cases = append(cases, struct {
				name      string
				decisions []Decision
				valid     bool
			}{
				fmt.Sprintf("поле %d решение %q", index, invalid), decisions, false})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]Decision(nil), tc.decisions...)
			problems := validateDecisions(fields, tc.decisions, context)
			if (len(problems) == 0) != tc.valid {
				t.Fatalf("валидность %v, ошибки %v", tc.valid, problems)
			}
			for _, problem := range problems {
				if problem == nil || problem.Code != CodeDecision || problem.Context != context || problem.Reason == "" {
					t.Fatalf("неверная диагностика: %+v", problem)
				}
			}
			for index, decision := range before {
				if tc.decisions[index] != decision {
					t.Fatal("изменены решения")
				}
			}
		})
	}
	for _, decisions := range [][]Decision{nil, {}} {
		if problems := validateDecisions(nil, decisions, context); len(problems) != 0 {
			t.Fatalf("пустой план требует решений: %v", problems)
		}
	}
	if problems := validateDecisions(nil, []Decision{Accept}, context); len(problems) == 0 {
		t.Fatal("лишнее решение принято для пустого плана")
	}
}

func TestFieldsApplyExactValuesAndOwnStrings(t *testing.T) {
	values := []*string{nil, fieldsTestString(""), fieldsTestString("  Ещё\nНазвание  ")}
	for _, field := range []compare.Field{compare.DocumentTitle, compare.DocumentSubtitle, compare.PageTitle} {
		for oldIndex := -1; oldIndex <= 0; oldIndex++ {
			if field != compare.PageTitle && oldIndex != -1 {
				continue
			}
			for beforeIndex, before := range values {
				if (field == compare.DocumentTitle && before == nil) || (field == compare.PageTitle && oldIndex == -1 && before != nil) {
					continue
				}
				for afterIndex, after := range values {
					if field == compare.DocumentTitle && after == nil {
						continue
					}
					if !(field == compare.PageTitle && oldIndex == -1) && beforeIndex == afterIndex {
						continue
					}
					for _, decision := range []Decision{Accept, Reject} {
						name := fmt.Sprintf("%s/%d/%d/%d/%s", field, oldIndex, beforeIndex, afterIndex, decision)
						t.Run(name, func(t *testing.T) {
							beforeText, afterText := "", ""
							if before != nil {
								beforeText = *before
							}
							if after != nil {
								afterText = *after
							}
							result := lesson.Lesson{Document: lesson.Document{ID: "Урок", Version: "7", Title: "исходное", Subtitle: fieldsTestString("подзаголовок"), LineCount: "42", CreatedAt: "2025-01-02T03:04:05Z", Pages: []lesson.Page{
								{ID: "урок-p009", Title: fieldsTestString("чужое название")},
								{ID: "урок-p010", Title: fieldsTestString("кандидат")},
							}}}
							change := compare.FieldChange{Field: field, OldPage: oldIndex, NewPage: -1, Before: before, After: after}
							if field == compare.PageTitle {
								change.NewPage = 1
							}
							want := before
							if decision == Accept {
								want = after
							}
							applyField(&result, change, decision)
							got := &result.Document.Title
							if field == compare.DocumentSubtitle {
								got = result.Document.Subtitle
							}
							if field == compare.PageTitle {
								got = result.Document.Pages[1].Title
							}
							fieldsTestText(t, got, want)
							if got != nil {
								if got == before || got == after {
									t.Fatal("результат разделяет указатель с планом")
								}
								*got = "изменён результат"
								if before != nil && *before != beforeText || after != nil && *after != afterText {
									t.Fatal("изменена строка плана")
								}
							}
							if field != compare.DocumentTitle && result.Document.Title != "исходное" {
								t.Fatal("затронуто название урока")
							}
							if field != compare.DocumentSubtitle {
								fieldsTestText(t, result.Document.Subtitle, fieldsTestString("подзаголовок"))
							}
							fieldsTestText(t, result.Document.Pages[0].Title, fieldsTestString("чужое название"))
							if result.Document.ID != "Урок" || result.Document.Version != "7" || result.Document.LineCount != "42" || result.Document.CreatedAt != "2025-01-02T03:04:05Z" || result.Document.Pages[1].ID != "урок-p010" {
								t.Fatal("применение поля изменило служебные данные")
							}
						})
					}
				}
			}
		}
	}
}

func TestFieldsApplyIndependentDecisions(t *testing.T) {
	shared := fieldsTestString("общее новое")
	fields := []compare.FieldChange{
		{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: fieldsTestString("прежнее"), After: shared},
		{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1, Before: fieldsTestString("оставить"), After: shared},
		{Field: compare.PageTitle, OldPage: 1, NewPage: 0, Before: fieldsTestString("старое второй"), After: shared},
		{Field: compare.PageTitle, OldPage: -1, NewPage: 1, After: shared},
		{Field: compare.PageTitle, OldPage: 0, NewPage: 2, Before: fieldsTestString("старое первой"), After: shared},
		{Field: compare.PageTitle, OldPage: -1, NewPage: 4, After: shared},
	}
	result := &lesson.Lesson{Document: lesson.Document{Pages: []lesson.Page{{ID: "новая-0"}, {ID: "новая-1"}, {ID: "новая-2"}, {Title: fieldsTestString("без изменений")}, {}}}}
	applyFields(result, fields, []Decision{Accept, Reject, Reject, Reject, Accept, Accept})
	if result.Document.Title != "общее новое" {
		t.Fatal("не принято название урока")
	}
	fieldsTestText(t, result.Document.Subtitle, fieldsTestString("оставить"))
	fieldsTestText(t, result.Document.Pages[0].Title, fieldsTestString("старое второй"))
	fieldsTestText(t, result.Document.Pages[1].Title, nil)
	fieldsTestText(t, result.Document.Pages[2].Title, shared)
	fieldsTestText(t, result.Document.Pages[3].Title, fieldsTestString("без изменений"))
	fieldsTestText(t, result.Document.Pages[4].Title, shared)
	*shared = "изменён план"
	if result.Document.Title != "общее новое" || *result.Document.Pages[2].Title != "общее новое" {
		t.Fatal("строки связаны с планом")
	}
	*result.Document.Pages[2].Title = "отдельное значение"
	if result.Document.Title != "общее новое" || *result.Document.Subtitle != "оставить" || *result.Document.Pages[4].Title != "общее новое" {
		t.Fatal("поля связаны между собой")
	}
	if result.Document.Pages[0].ID != "новая-0" || result.Document.Pages[1].ID != "новая-1" || result.Document.Pages[2].ID != "новая-2" {
		t.Fatal("перенесены старые ID")
	}
}

func TestFieldsApplyInSliceOrder(t *testing.T) {
	// Прямой вызов проверяет порядок исполнителя; такой список не передаётся Apply как план.
	first, last := fieldsTestString("первое"), fieldsTestString("последнее")
	fields := []compare.FieldChange{
		{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: first, After: last},
		{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: first, After: last},
	}
	result := &lesson.Lesson{}
	applyFields(result, fields, []Decision{Accept, Reject})
	if result.Document.Title != "первое" {
		t.Fatalf("нарушен порядок: %q", result.Document.Title)
	}
	applyFields(result, nil, nil)
	if result.Document.Title != "первое" {
		t.Fatal("пустой список изменил результат")
	}
}

func TestFieldsAcceptedChangesFollowPlan(t *testing.T) {
	known := compare.FieldChange{Field: compare.DocumentSubtitle, OldPage: -1, NewPage: -1, After: fieldsTestString("")}
	cases := []struct {
		name      string
		plan      compare.Plan
		decisions []Decision
		want      bool
	}{
		{"нет изменений", compare.Plan{}, nil, false},
		{"отказ", compare.Plan{Fields: []compare.FieldChange{known}}, []Decision{Reject}, false},
		{"принят пустой текст", compare.Plan{Fields: []compare.FieldChange{known}}, []Decision{Accept}, true},
		{"принят null", compare.Plan{Fields: []compare.FieldChange{{Field: compare.PageTitle, OldPage: 0, NewPage: 0, Before: fieldsTestString("было")}}}, []Decision{Accept}, true},
		{"одно из двух", compare.Plan{Fields: []compare.FieldChange{known, {Field: compare.PageTitle, OldPage: 0, NewPage: 0, Before: fieldsTestString("было")}}}, []Decision{Reject, Accept}, true},
		{"полная замена с отказом неизвестного null", compare.Plan{ReplaceLesson: true, Fields: []compare.FieldChange{{Field: compare.PageTitle, OldPage: -1, NewPage: 0}}}, []Decision{Reject}, true},
		{"полная замена с принятым неизвестным null", compare.Plan{ReplaceLesson: true, Fields: []compare.FieldChange{{Field: compare.PageTitle, OldPage: -1, NewPage: 0}}}, []Decision{Accept}, true},
		{"замена нод", compare.Plan{Pages: []compare.PageChange{{Index: 0, ReplaceNodes: true}}}, nil, true},
		{"точечная замена", compare.Plan{Pages: []compare.PageChange{{Index: 0, Nodes: []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}}}}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasAcceptedChanges(&tc.plan, tc.decisions); got != tc.want {
				t.Fatalf("изменение %v, ожидалось %v", got, tc.want)
			}
		})
	}
	old := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{ID: "Урок", Version: "7", Title: "то же", LineCount: "11", CreatedAt: "2025-01-01T00:00:00Z", Pages: []lesson.Page{{ID: "урок-p020", Nodes: []lesson.Node{}}}}}
	candidate := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{ID: "УРОК", Version: "1", Title: "то же", LineCount: "999", CreatedAt: "2026-01-01T00:00:00Z", Pages: []lesson.Page{{ID: "урок-p001", Nodes: []lesson.Node{}}}}}
	plan, problems := compare.Build(old, candidate, diagnostic.Context{}, diagnostic.Context{})
	if len(problems) != 0 || plan == nil {
		t.Fatalf("план служебных различий: %v", problems)
	}
	if hasAcceptedChanges(plan, nil) {
		t.Fatal("служебные различия создали изменение")
	}
}
