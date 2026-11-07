package preview

import (
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
)

func TestUpdateReplacementsEmptyPlanDoesNotDecideOperation(t *testing.T) {
	before := updateTestFile(t, updateTestLesson(), true)
	final := updateTestLesson()
	final.Document.Title = "Подтверждённое итоговое название"
	final.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n002-v06"
	after := updateTestFile(t, final, false)
	for _, plan := range []*compare.Plan{
		{},
		{Fields: []compare.FieldChange{{Field: compare.DocumentTitle, OldPage: -1, NewPage: -1, Before: updateTestString("Отклонённое прежнее"), After: updateTestString("НЕПРИНЯТОЕ ПРЕДЛОЖЕНИЕ")}}},
	} {
		snapshot := updateTestSnapshot(t, before, after, plan)
		text, err := Replacements(before, after, plan)
		if err != nil {
			t.Fatal(err)
		}
		updateTestContains(t, text, "Замены нод не запланированы")
		updateTestExcludes(t, text, "НЕПРИНЯТОЕ ПРЕДЛОЖЕНИЕ", "efx7-n002-v06", "Подтверждённое итоговое название")
		updateTestClean(t, text)
		if updateTestSnapshot(t, before, after, plan) != snapshot {
			t.Fatal("изменились снимки или готовый план")
		}
	}
}

func TestUpdateReplacementsUsesOnlySelectedNodesAndFinalIDs(t *testing.T) {
	before := updateTestFile(t, updateTestLesson(), true)
	final := updateTestLesson()
	final.Document.Version = "123"
	final.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n002-v123"
	final.Document.Pages[0].Nodes[1].ID = "efx7-n035-v123"
	final.Document.Pages[1].Nodes[0].ID = "efx7-n099-v123"
	after := updateTestFile(t, final, false)
	plan := &compare.Plan{Pages: []compare.PageChange{
		{Index: 0, Nodes: []compare.NodeChange{
			{Path: []int{0, 0}, Fields: []compare.NodeField{compare.NodeDataChanged}},
			{Path: []int{1}, Fields: []compare.NodeField{compare.NodeTypeChanged, compare.NodeFlagsChanged, compare.NodeDataChanged}},
		}},
		{Index: 1, Nodes: []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}},
	}}
	snapshot := updateTestSnapshot(t, before, after, plan)
	text, err := Replacements(before, after, plan)
	if err != nil {
		t.Fatal(err)
	}
	updateTestOrder(t, text, "efx7-p001", "efx7-n002", "efx7-n002-v123", "efx7-n035-v03", "efx7-n035-v123", "efx7-p007", "efx7-n099", "efx7-n099-v123")
	updateTestContains(t, strings.ToLower(text), "тип", "флаг", "данн")
	updateTestExcludes(t, text, "efx7-n001", "-v124", "Замены нод не запланированы")
	updateTestClean(t, text)
	if updateTestSnapshot(t, before, after, plan) != snapshot {
		t.Fatal("изменились снимки или готовый план")
	}
}

func TestUpdatePageReplacementsPreservesSuppliedSequence(t *testing.T) {
	before := updateTestFile(t, updateTestLesson(), false)
	final := updateTestLesson()
	final.Document.Version = "6"
	final.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n002-v06"
	final.Document.Pages[0].Nodes[1].ID = "efx7-n035-v06"
	after := updateTestFile(t, final, false)
	changes := []compare.PageChange{{Index: 0, Nodes: []compare.NodeChange{
		{Path: []int{1}, Fields: []compare.NodeField{compare.NodeDataChanged}},
		{Path: []int{0, 0}, Fields: []compare.NodeField{compare.NodeDataChanged}},
	}}}
	text, err := pageReplacements(before, after, changes)
	if err != nil {
		t.Fatal(err)
	}
	updateTestOrder(t, text, "efx7-n035-v03", "efx7-n035-v06", "efx7-n002", "efx7-n002-v06")
	updateTestExcludes(t, text, "efx7-n001", "efx7-n099")
	updateTestClean(t, text)
}

func TestUpdateReplacementsPageHasIndependentCompleteLists(t *testing.T) {
	before := updateTestFile(t, updateTestLesson(), true)
	final := updateTestLesson()
	final.Document.Version = "101"
	final.Document.Pages[0].Nodes = []lesson.Node{updateTestNode("efx7-n1000-v101", updateTestNode("efx7-n1007-v101"))}
	after := updateTestFile(t, final, false)
	reasons := []compare.Reason{
		{Code: compare.NodeCountChanged, Description: "Число корневых нод: 2 вместо 3", OldPointer: "/document/pages/0/nodes", NewPointer: "/document/pages/0/nodes"},
		{Code: compare.NodeNestingChanged, Description: "Изменилась вложенность выбранной ветви", OldPointer: "/document/pages/0/nodes/0/nodes", NewPointer: "/document/pages/0/nodes/1/nodes"},
	}
	plan := &compare.Plan{Pages: []compare.PageChange{{Index: 0, ReplaceNodes: true, Reasons: reasons}}}
	snapshot := updateTestSnapshot(t, before, after, plan)
	text, err := Replacements(before, after, plan)
	if err != nil {
		t.Fatal(err)
	}
	updateTestContains(t, text, "efx7-p001", "Первая страница", "Будут удалены", "Будут созданы")
	updateTestContains(t, strings.ToLower(text), "сохран")
	updateTestOrder(t, text, reasons[0].Description, reasons[1].Description)
	updateTestContains(t, text, reasons[0].OldPointer, reasons[1].NewPointer)
	updateTestOrder(t, text, "Будут удалены", "efx7-n001", "efx7-n002", "efx7-n035-v03", "Будут созданы", "efx7-n1000-v101", "efx7-n1007-v101")
	updateTestExcludes(t, text, "efx7-n099", "efx7-n1001", "efx7-n1006")
	updateTestClean(t, text)
	if updateTestSnapshot(t, before, after, plan) != snapshot {
		t.Fatal("изменились снимки или готовый план")
	}
}

func TestUpdateReplacementsLessonListsEachSideWithoutMatching(t *testing.T) {
	before := updateTestFile(t, updateTestLesson(), true)
	final := updateTestLesson()
	final.Document.Version = "1001"
	final.Document.Pages = []lesson.Page{
		{ID: "efx7-p010", Nodes: []lesson.Node{updateTestNode("efx7-n5000-v1001", updateTestNode("efx7-n5007-v1001"))}},
		{ID: "efx7-p020", Nodes: []lesson.Node{}},
		{ID: "efx7-p030", Nodes: []lesson.Node{updateTestNode("efx7-n9000-v1001")}},
	}
	after := updateTestFile(t, final, false)
	plan := &compare.Plan{ReplaceLesson: true, Reasons: []compare.Reason{
		{Code: compare.PageCountChanged, Description: "Число страниц изменилось с 2 на 3", OldPointer: "/document/pages", NewPointer: "/document/pages"},
		{Code: compare.PageCorrespondenceAmbiguous, Description: "Соответствие страниц неоднозначно", NewPointer: "/document/pages/2"},
	}}
	snapshot := updateTestSnapshot(t, before, after, plan)
	text, err := Replacements(before, after, plan)
	if err != nil {
		t.Fatal(err)
	}
	updateTestContains(t, strings.ToLower(text), "полн", "замен", "урок", "сохран", "кандидат")
	updateTestContains(t, text, "l042.json", "EFX7", "/document/pages/2")
	updateTestOrder(t, text, plan.Reasons[0].Description, plan.Reasons[1].Description)
	updateTestOrder(t, text, "efx7-p001", "efx7-p007")
	updateTestOrder(t, text, "efx7-p010", "efx7-p020", "efx7-p030")
	updateTestOrder(t, text, "efx7-n001", "efx7-n002", "efx7-n035-v03", "efx7-n099")
	updateTestOrder(t, text, "efx7-n5000-v1001", "efx7-n5007-v1001", "efx7-n9000-v1001")
	updateTestExcludes(t, text, "efx7-n5001", "efx7-p011", "efx7-p001 -> efx7-p010", "efx7-n001 → efx7-n5000-v1001")
	updateTestClean(t, text)
	if updateTestSnapshot(t, before, after, plan) != snapshot {
		t.Fatal("изменились снимки или готовый план")
	}
}

func TestUpdateReplacementsEmptyPageListsAreExplicit(t *testing.T) {
	for _, emptyOld := range []bool{true, false} {
		old, final := updateTestLesson(), updateTestLesson()
		if emptyOld {
			old.Document.Pages[0].Nodes = []lesson.Node{}
		} else {
			final.Document.Pages[0].Nodes = []lesson.Node{}
		}
		before, after := updateTestFile(t, old, false), updateTestFile(t, final, false)
		plan := &compare.Plan{Pages: []compare.PageChange{{Index: 0, ReplaceNodes: true, Reasons: []compare.Reason{{Code: compare.NodeCountChanged, Description: "Число нод изменилось"}}}}}
		text, err := Replacements(before, after, plan)
		if err != nil {
			t.Fatal(err)
		}
		updateTestContains(t, text, "Будут удалены", "Будут созданы", "нет")
		updateTestClean(t, text)
	}
}

func TestUpdateReplacementsEscapesPageNamesAndReasons(t *testing.T) {
	value := updateTestLesson()
	value.Document.Pages[0].Title = updateTestString("Страница\r\nПродолжение\x1b[31m")
	file := updateTestFile(t, value, true)
	reason := compare.Reason{
		Code: compare.NodeNestingChanged, Description: "Причина\r\nПодробность\x1b[2J",
		OldPointer: "/document/pages/0/nodes", NewPointer: "/document/pages/0/nodes/0/nodes",
	}
	plan := &compare.Plan{Pages: []compare.PageChange{{Index: 0, ReplaceNodes: true, Reasons: []compare.Reason{reason}}}}
	text, err := Replacements(file, file, plan)
	if err != nil {
		t.Fatal(err)
	}
	updateTestQuoted(t, text, *value.Document.Pages[0].Title)
	updateTestQuoted(t, text, reason.Description)
	updateTestContains(t, text, reason.OldPointer, reason.NewPointer)
	updateTestContains(t, strings.ToLower(text), "стар", "кандидат")
	updateTestClean(t, text)
}

func TestUpdateReplacementsRejectsUnrepresentableInputs(t *testing.T) {
	file := updateTestFile(t, updateTestLesson(), true)
	for _, pair := range [][2]*module.File{{nil, file}, {file, nil}, {new(module.File), file}, {file, new(module.File)}} {
		text, err := Replacements(pair[0], pair[1], &compare.Plan{})
		updateTestError(t, text, err)
	}
	text, err := Replacements(file, file, nil)
	updateTestError(t, text, err)
	for _, change := range []compare.PageChange{
		{Index: -1, Nodes: []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}},
		{Index: 7, ReplaceNodes: true, Reasons: []compare.Reason{{Description: "Недоступная страница"}}},
		{Index: 0, Nodes: []compare.NodeChange{{Fields: []compare.NodeField{compare.NodeDataChanged}}}},
		{Index: 0, Nodes: []compare.NodeChange{{Path: []int{-1}, Fields: []compare.NodeField{compare.NodeDataChanged}}}},
		{Index: 0, Nodes: []compare.NodeChange{{Path: []int{99}, Fields: []compare.NodeField{compare.NodeDataChanged}}}},
		{Index: 0, Nodes: []compare.NodeChange{{Path: []int{0, 0, 0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}},
		{Index: 0, ReplaceNodes: true, Nodes: []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}},
	} {
		text, err := Replacements(file, file, &compare.Plan{Pages: []compare.PageChange{change}})
		updateTestError(t, text, err)
	}
	plan := &compare.Plan{ReplaceLesson: true, Pages: []compare.PageChange{{Index: 0}}, Reasons: []compare.Reason{{Description: "Полная замена"}}}
	text, err = Replacements(file, file, plan)
	updateTestError(t, text, err)
	final := updateTestLesson()
	final.Document.Pages = []lesson.Page{}
	after := updateTestFile(t, final, false)
	text, err = Replacements(file, after, &compare.Plan{Pages: []compare.PageChange{{Index: 1, Nodes: []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}}}})
	updateTestError(t, text, err)
	final = updateTestLesson()
	final.Document.Pages[0].Nodes = []lesson.Node{}
	after = updateTestFile(t, final, false)
	text, err = Replacements(file, after, &compare.Plan{Pages: []compare.PageChange{{Index: 0, Nodes: []compare.NodeChange{{Path: []int{0}, Fields: []compare.NodeField{compare.NodeDataChanged}}}}}})
	updateTestError(t, text, err)
}
