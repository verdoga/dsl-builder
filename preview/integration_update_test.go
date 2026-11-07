package preview_test

import (
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/preview"
)

func TestIntegrationUpdateChildSelectedFieldsAndManifest(t *testing.T) {
	before := integrationTestFile(t, "l007.json", integrationTestLesson(lesson.Page{
		ID: "efx7-p007", Title: integrationTestString("Практика"),
		Nodes: []lesson.Node{integrationTestNode("efx7-n003", "NODE_DATA_SECRET",
			integrationTestNode("efx7-n035-v03", "старое содержимое"))},
	}), true)
	candidate := before.Lesson()
	candidate.Document.Title = "ОТКЛОНЁННОЕ НАЗВАНИЕ"
	candidate.Document.Subtitle = integrationTestString("")
	candidate.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value = "NODE_DATA_SECRET новое"
	plan, problems := compare.Build(before.Lesson(), candidate, before.Context(), before.Context())
	if len(problems) != 0 || plan.ReplaceLesson || len(plan.Fields) != 2 || len(plan.Pages) != 1 || len(plan.Pages[0].Nodes) != 1 {
		t.Fatalf("ready compare plan: %+v, %v", plan, problems)
	}
	// Явно подготовленный итог владельца операции, а не реализация revision.
	final := before.Lesson()
	final.Document.Version, final.Document.CreatedAt = "101", "2026-11-04T12:00:00Z"
	final.Document.Subtitle = integrationTestString("")
	final.Document.Pages[0].Nodes[0].Nodes[0] = integrationTestNode("efx7-n035-v101", "NODE_DATA_SECRET новое")
	after := integrationTestFile(t, "l007.json", final, false)
	accepted := []compare.FieldChange{plan.Fields[1]}
	t.Cleanup(integrationTestPreserve(t, func() any { return plan }))
	t.Cleanup(integrationTestPreserve(t, func() any { return candidate }))
	t.Cleanup(integrationTestPreserve(t, func() any { return accepted }))
	for _, field := range plan.Fields {
		text, err := preview.FieldProposal(before, field)
		text = integrationTestBlock(t, text, err)
		integrationTestContains(t, text, "l007.json", "EFX7", string(field.Field), "Было", "Станет")
	}
	heading, err := preview.UpdateHeading(`C:\Модули\Юнит.zip`, before, after)
	heading = integrationTestBlock(t, heading, err)
	integrationTestOrder(t, heading, "100", "101")
	integrationTestContains(t, heading, "Юнит.zip", "l007.json", "EFX7", "Сохранённое название")
	integrationTestAbsent(t, heading, "ОТКЛОНЁННОЕ НАЗВАНИЕ")
	fields, err := preview.AcceptedFields(before, after, accepted)
	fields = integrationTestBlock(t, fields, err)
	integrationTestContains(t, fields, "document.subtitle", "null", `""`)
	integrationTestAbsent(t, fields, "document.title", "ОТКЛОНЁННОЕ НАЗВАНИЕ")
	replacements, err := preview.Replacements(before, after, plan)
	replacements = integrationTestBlock(t, replacements, err)
	integrationTestOrder(t, replacements, "efx7-p007", "efx7-n035-v03", "efx7-n035-v101")
	integrationTestContains(t, strings.ToLower(replacements), "данн")
	integrationTestAbsent(t, replacements, "efx7-n003", "efx7-n035-v101-v", "старое содержимое")
	oldState := &catalog.State{CourseTitle: "Курс", Items: []catalog.Item{{
		Entry: catalog.EntryFromLesson(before.Lesson().Document, "l007.json", integrationTestString("00010")),
	}}}
	newState := &catalog.State{CourseTitle: "Курс", Items: []catalog.Item{{
		Entry: catalog.EntryFromLesson(after.Lesson().Document, "l007.json", integrationTestString("00010")),
	}}}
	changes := catalog.Changes(oldState, newState)
	t.Cleanup(integrationTestPreserve(t, func() any { return changes }))
	manifestText, err := preview.ManifestChanges(changes)
	manifestText = integrationTestBlock(t, manifestText, err)
	integrationTestOrder(t, manifestText, "subtitle", "createdAt", "version")
	integrationTestContains(t, manifestText, "l007.json", "100", "101", "2026-11-04T12:00:00Z")
	integrationTestAbsent(t, manifestText, "ОТКЛОНЁННОЕ НАЗВАНИЕ", "document.pages")
	// Отсутствие замен не отменяет отдельно принятые поля и правки манифеста.
	empty, err := preview.Replacements(before, after, &compare.Plan{Fields: plan.Fields})
	empty = integrationTestBlock(t, empty, err)
	integrationTestContains(t, empty, "Замены нод не запланированы")
	integrationTestAbsent(t, empty, "изменений нет", "Изменений нет", "архив оставлен", "efx7-n035")
}

func TestIntegrationPageAndWholeLessonUseCompleteIndependentIDLists(t *testing.T) {
	oldPages := []lesson.Page{
		{ID: "efx7-p007", Title: integrationTestString("Практика"), Nodes: []lesson.Node{
			integrationTestNode("efx7-n003", "NODE_DATA_SECRET", integrationTestNode("efx7-n035-v03", "NODE_DATA_SECRET"))}},
		{ID: "efx7-p1001", Title: nil, Nodes: []lesson.Node{integrationTestNode("efx7-n900-v05", "NODE_DATA_SECRET")}},
	}
	for _, test := range []struct {
		name           string
		pages          []lesson.Page
		oldIDs, newIDs []string
		whole          bool
	}{
		{"page", []lesson.Page{
			{ID: "efx7-p007", Title: oldPages[0].Title, Nodes: []lesson.Node{
				integrationTestNode("efx7-n5000-v101", "NODE_DATA_SECRET", integrationTestNode("efx7-n5003-v101", "NODE_DATA_SECRET")),
				integrationTestNode("efx7-n7000-v101", "NODE_DATA_SECRET")}}, oldPages[1]},
			[]string{"efx7-n003", "efx7-n035-v03"}, []string{"efx7-n5000-v101", "efx7-n5003-v101", "efx7-n7000-v101"}, false},
		{"lesson", []lesson.Page{
			{ID: "efx7-p4000", Nodes: []lesson.Node{integrationTestNode("efx7-n9000-v101", "NODE_DATA_SECRET")}},
			{ID: "efx7-p4007", Nodes: []lesson.Node{integrationTestNode("efx7-n9999-v101", "NODE_DATA_SECRET", integrationTestNode("efx7-n10002-v101", "NODE_DATA_SECRET"))}},
			{ID: "efx7-p5001", Nodes: []lesson.Node{}}},
			[]string{"efx7-n003", "efx7-n035-v03", "efx7-n900-v05"}, []string{"efx7-n9000-v101", "efx7-n9999-v101", "efx7-n10002-v101"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := integrationTestFile(t, "l007.json", integrationTestLesson(oldPages...), true)
			final := integrationTestLesson(test.pages...)
			final.Document.Version = "101"
			after := integrationTestFile(t, "l007.json", final, false)
			plan, problems := compare.Build(before.Lesson(), after.Lesson(), before.Context(), after.Context())
			if len(problems) != 0 || plan.ReplaceLesson != test.whole {
				t.Fatalf("ready owner plan: %+v %v", plan, problems)
			}
			t.Cleanup(integrationTestPreserve(t, func() any { return plan }))
			text, err := preview.Replacements(before, after, plan)
			text = integrationTestBlock(t, text, err)
			integrationTestOrder(t, text, test.oldIDs...)
			integrationTestOrder(t, text, test.newIDs...)
			integrationTestNoPairs(t, text, test.oldIDs, test.newIDs)
			reasons := plan.Reasons
			if test.whole {
				integrationTestContains(t, strings.ToLower(text), "полн", "замен", "урока")
				integrationTestContains(t, text, "l007.json", "EFX7")
				integrationTestOrder(t, text, "efx7-p007", "efx7-p1001")
				integrationTestOrder(t, text, "efx7-p4000", "efx7-p4007", "efx7-p5001")
				integrationTestNoPairs(t, text, []string{"efx7-p007", "efx7-p1001"}, []string{"efx7-p4000", "efx7-p4007", "efx7-p5001"})
			} else {
				if len(plan.Pages) != 1 || !plan.Pages[0].ReplaceNodes {
					t.Fatalf("expected page replacement: %+v", plan)
				}
				reasons = plan.Pages[0].Reasons
				integrationTestContains(t, text, "efx7-p007", "Практика", "Будут удалены", "Будут созданы")
				integrationTestAbsent(t, text, "efx7-n900-v05")
			}
			for _, reason := range reasons {
				integrationTestContains(t, text, reason.Description, reason.OldPointer, reason.NewPointer)
			}
			heading, err := preview.UpdateHeading("не существующий архив.zip", before, after)
			integrationTestBlock(t, heading, err)
			fields, err := preview.AcceptedFields(before, after, nil)
			fields = integrationTestBlock(t, fields, err)
			integrationTestAbsent(t, fields, "изменений нет", "Изменений нет")
			oldState := &catalog.State{CourseTitle: "Курс", Items: []catalog.Item{{
				Entry: catalog.EntryFromLesson(before.Lesson().Document, "l007.json", integrationTestString("0010")),
			}}}
			newState := &catalog.State{CourseTitle: "Курс", Items: []catalog.Item{{
				Entry: catalog.EntryFromLesson(after.Lesson().Document, "l007.json", integrationTestString("0010")),
			}}}
			changes := catalog.Changes(oldState, newState)
			t.Cleanup(integrationTestPreserve(t, func() any { return changes }))
			manifestText, err := preview.ManifestChanges(changes)
			manifestText = integrationTestBlock(t, manifestText, err)
			integrationTestContains(t, manifestText, "l007.json", "version", "100", "101")
		})
	}
}

func TestIntegrationUnknownPageProposalRefusalAndKnownNullEmpty(t *testing.T) {
	before := integrationTestFile(t, "l007.json", integrationTestLesson(
		lesson.Page{ID: "efx7-p007", Title: integrationTestString("ЧУЖОЕ НАЗВАНИЕ"), Nodes: []lesson.Node{}},
		lesson.Page{ID: "efx7-p008", Nodes: []lesson.Node{}},
		lesson.Page{ID: "efx7-p009", Title: integrationTestString(""), Nodes: []lesson.Node{}},
	), true)
	for _, test := range []struct {
		name    string
		change  compare.FieldChange
		unknown bool
	}{
		{"unknown with title", compare.FieldChange{Field: compare.PageTitle, OldPage: -1, NewPage: 2, After: integrationTestString("НОВОЕ НАЗВАНИЕ")}, true},
		{"unknown with null", compare.FieldChange{Field: compare.PageTitle, OldPage: -1, NewPage: 2}, true},
		{"known null to empty", compare.FieldChange{Field: compare.PageTitle, OldPage: 1, NewPage: 0, After: integrationTestString("")}, false},
		{"known empty to null", compare.FieldChange{Field: compare.PageTitle, OldPage: 2, NewPage: 1, Before: integrationTestString("")}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Cleanup(integrationTestPreserve(t, func() any { return test.change }))
			text, err := preview.FieldProposal(before, test.change)
			text = integrationTestBlock(t, text, err)
			integrationTestContains(t, text, "page.title", "Было", "Станет", "null")
			integrationTestAbsent(t, text, "ЧУЖОЕ НАЗВАНИЕ")
			if test.unknown {
				integrationTestContains(t, text, "прежнее поле не установлено", "title=null", "3")
				integrationTestAbsent(t, text, "efx7-p007", "efx7-p009")
			} else {
				integrationTestContains(t, text, `""`)
				integrationTestAbsent(t, text, "прежнее поле не установлено")
			}
		})
	}
	// Первый переход принят, оба неизвестных названия отклонены владельцем.
	final := integrationTestLesson(
		lesson.Page{ID: "efx7-p900", Title: integrationTestString(""), Nodes: []lesson.Node{}},
		lesson.Page{ID: "efx7-p901", Nodes: []lesson.Node{}},
		lesson.Page{ID: "efx7-p902", Nodes: []lesson.Node{}},
	)
	final.Document.Version = "101"
	after := integrationTestFile(t, "l007.json", final, false)
	accepted := []compare.FieldChange{{Field: compare.PageTitle, OldPage: 1, NewPage: 0, After: integrationTestString("")}}
	text, err := preview.AcceptedFields(before, after, accepted)
	text = integrationTestBlock(t, text, err)
	integrationTestContains(t, text, "efx7-p008", "efx7-p900", "null", `""`)
	integrationTestAbsent(t, text, "НОВОЕ НАЗВАНИЕ", "ЧУЖОЕ НАЗВАНИЕ", "efx7-p902")
}

func TestIntegrationManifestOnlyKeepsRevisionAndEmptyReplacementScope(t *testing.T) {
	file := integrationTestFile(t, "l007.json", integrationTestLesson(), true)
	before := &catalog.State{CourseTitle: "Прежний курс", Items: []catalog.Item{{
		Entry: catalog.EntryFromLesson(file.Lesson().Document, file.FileName(), integrationTestString("00010")),
	}}}
	after := catalog.Clone(before)
	after.CourseTitle = "Новый курс"
	after.UnitTitle = integrationTestString("Новый юнит")
	after.Items[0].Entry.Order = integrationTestString("9007199254740993123450001")
	changes := catalog.Changes(before, after)
	t.Cleanup(integrationTestPreserve(t, func() any { return changes }))
	heading, err := preview.UpdateHeading("Архив.zip", file, file)
	heading = integrationTestBlock(t, heading, err)
	if strings.Count(heading, "100") != 2 {
		t.Errorf("literal unchanged revision must appear on both sides: %s", heading)
	}
	text, err := preview.Replacements(file, file, &compare.Plan{})
	text = integrationTestBlock(t, text, err)
	integrationTestContains(t, text, "Замены нод не запланированы")
	manifestText, err := preview.ManifestChanges(changes)
	manifestText = integrationTestBlock(t, manifestText, err)
	integrationTestOrder(t, manifestText, "course.title", "course.unit", "order")
	integrationTestContains(t, manifestText, "Прежний курс", "Новый курс", "Новый юнит", "00010", "9007199254740993123450001")
	integrationTestAbsent(t, text+manifestText, "Содержательных изменений не обнаружено", "архив оставлен без изменений")
}
