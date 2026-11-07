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

func revisionTestText(value string) *string                   { return &value }
func revisionTestBool(value bool) *bool                       { return &value }
func revisionTestRole(value lesson.DataRole) *lesson.DataRole { return &value }
func revisionTestAt() time.Time {
	return time.Date(2026, 1, 2, 1, 4, 5, 987654321, time.FixedZone("UTC+3", 3*3600))
}
func revisionTestData(value string) lesson.Data {
	return lesson.Data{Type: lesson.DataText, Role: revisionTestRole(lesson.RoleBody), Value: value, Answers: []string{"ответ"}, Hint: " Подсказка\n"}
}
func revisionTestLeaf(id, value string) lesson.Node {
	return lesson.Node{ID: id, Type: lesson.NodeText, Data: []lesson.Data{revisionTestData(value)}, Nodes: []lesson.Node{}}
}
func revisionTestLesson() *lesson.Lesson {
	child := revisionTestLeaf("efx7-n081-v03", " Ребёнок\n\nтекст ")
	child.Type, child.Flags.Checkable = lesson.NodeQuestion, revisionTestBool(false)
	nested := lesson.Data{Type: lesson.DataCollection, Value: []lesson.Data{revisionTestData("вложенный")}, Answers: []string{}}
	parent := lesson.Node{ID: "efx7-n035", Type: lesson.NodeTask,
		Flags: lesson.NodeFlags{Speaking: revisionTestBool(true), Stepped: revisionTestBool(false)},
		Data:  []lesson.Data{{Type: lesson.DataCollection, Role: revisionTestRole(lesson.RoleBody), Value: []lesson.Data{nested}, Answers: []string{"собственный"}}}, Nodes: []lesson.Node{child}}
	media := revisionTestLeaf("efx7-n1000", "медиа")
	media.Type = lesson.NodeMedia
	media.Data[0].Type = lesson.DataMedia
	media.Data[0].Value = lesson.EmbeddedFile{FileName: "ёж.bin", MIMEType: "application/octet-stream", Content: "AQIDBA=="}
	return &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EfX7", Version: "5.0", Title: " Название\n", Subtitle: revisionTestText("подзаголовок"),
		LineCount: "9007199254740993e0", CreatedAt: "2020-02-29T03:04:05Z",
		Pages: []lesson.Page{
			{ID: "efx7-p009", Title: revisionTestText("первая"), Nodes: []lesson.Node{parent, media}},
			{ID: "efx7-p1000", Nodes: []lesson.Node{revisionTestLeaf("efx7-n002", "вторая")}},
		},
	}}
}
func revisionTestJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func revisionTestCopy(t *testing.T, value *lesson.Lesson) *lesson.Lesson {
	t.Helper()
	var result lesson.Lesson
	if err := json.Unmarshal([]byte(revisionTestJSON(t, value)), &result); err != nil {
		t.Fatal(err)
	}
	return &result
}
func revisionTestEqual(t *testing.T, got any, want string) {
	t.Helper()
	if actual := revisionTestJSON(t, got); actual != want {
		t.Errorf("модель изменилась:\n%s\nожидалось:\n%s", actual, want)
	}
}
func revisionTestValid(t *testing.T, value *lesson.Lesson) {
	t.Helper()
	if errors := lesson.Validate(value, diagnostic.Context{}); len(errors) != 0 {
		t.Fatalf("lesson.Validate: %v", errors)
	}
}
func revisionTestPlan(t *testing.T, old, candidate *lesson.Lesson) *compare.Plan {
	t.Helper()
	plan, errors := compare.Build(old, candidate, diagnostic.Context{Path: "old.json"}, diagnostic.Context{Path: "new.json"})
	if len(errors) != 0 || plan == nil {
		t.Fatalf("compare.Build: plan=%v errors=%v", plan, errors)
	}
	return plan
}
func revisionTestDecisions(plan *compare.Plan, decision revision.Decision) []revision.Decision {
	result := make([]revision.Decision, len(plan.Fields))
	for i := range result {
		result[i] = decision
	}
	return result
}
func revisionTestCode(t *testing.T, errors []*diagnostic.Error, code string) {
	t.Helper()
	for _, problem := range errors {
		if problem != nil && problem.Code == code {
			return
		}
	}
	t.Fatalf("не найден код %q: %v", code, errors)
}
func revisionTestMutate(value *lesson.Lesson) {
	value.Document.Title = "изменено"
	*value.Document.Subtitle = "изменено"
	*value.Document.Pages[0].Title = "изменено"
	parent := &value.Document.Pages[0].Nodes[0]
	*parent.Flags.Speaking, *parent.Flags.Stepped = false, true
	*parent.Data[0].Role = lesson.RoleHint
	parent.Data[0].Answers[0] = "изменено"
	collection := parent.Data[0].Value.([]lesson.Data)
	deep := collection[0].Value.([]lesson.Data)
	*deep[0].Role = lesson.RoleHint
	deep[0].Answers[0], deep[0].Value = "изменено", "изменено"
	collection[0].Hint = "изменено"
	*parent.Nodes[0].Flags.Checkable = true
	parent.Nodes[0].Data[0].Answers[0] = "изменено"
	parent.Nodes[0].ID = "изменено"
	media := &value.Document.Pages[0].Nodes[1].Data[0]
	file := media.Value.(lesson.EmbeddedFile)
	file.Content = ""
	media.Value = file
	value.Document.Pages[1].Nodes[0].Data[0].Hint = "изменено"
}

func TestRevisionInitialAssignsOnlyServiceFields(t *testing.T) {
	candidate := revisionTestLesson()
	candidate.Document.Version, candidate.Document.CreatedAt = "0", "не дата"
	candidate.Document.Pages[0].ID, candidate.Document.Pages[0].Nodes[0].ID = "", "чужой ID"
	before := revisionTestJSON(t, candidate)
	expected := revisionTestCopy(t, candidate)
	expected.Document.Version, expected.Document.CreatedAt = "1", "2026-01-01T22:04:05Z"
	expected.Document.Pages[0].ID, expected.Document.Pages[1].ID = "efx7-p001", "efx7-p002"
	expected.Document.Pages[0].Nodes[0].ID = "efx7-n001"
	expected.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n002"
	expected.Document.Pages[0].Nodes[1].ID, expected.Document.Pages[1].Nodes[0].ID = "efx7-n003", "efx7-n004"
	at := revisionTestAt()
	atBefore := at
	result, errors := revision.Initial(candidate, at, diagnostic.Context{Path: "кандидат.json"})
	if len(errors) != 0 || result == nil {
		t.Fatalf("Initial: %v", errors)
	}
	revisionTestValid(t, result)
	revisionTestEqual(t, result, revisionTestJSON(t, expected))
	revisionTestEqual(t, candidate, before)
	resultBefore := revisionTestJSON(t, result)
	revisionTestMutate(candidate)
	revisionTestEqual(t, result, resultBefore)
	candidateBefore := revisionTestJSON(t, candidate)
	revisionTestMutate(result)
	revisionTestEqual(t, candidate, candidateBefore)
	if at != atBefore {
		t.Fatal("изменён at")
	}
}

func TestRevisionInitialRejectsInvalidWithoutRepairOrMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*lesson.Lesson)
		code   string
	}{
		{"format", func(v *lesson.Lesson) { v.FormatVersion = "2.0" }, lesson.CodeContract},
		{"lesson ID", func(v *lesson.Lesson) { v.Document.ID = "bad id" }, "identity.invalid_lesson_id"},
		{"lineCount", func(v *lesson.Lesson) { v.Document.LineCount = "-1" }, lesson.CodeContract},
		{"pages null", func(v *lesson.Lesson) { v.Document.Pages = nil }, lesson.CodeContract},
		{"page nodes null", func(v *lesson.Lesson) { v.Document.Pages[0].Nodes = nil }, lesson.CodeContract},
		{"node data null", func(v *lesson.Lesson) { v.Document.Pages[0].Nodes[0].Data = nil }, lesson.CodeContract},
		{"children null", func(v *lesson.Lesson) { v.Document.Pages[0].Nodes[0].Nodes = nil }, lesson.CodeContract},
		{"answers null", func(v *lesson.Lesson) { v.Document.Pages[0].Nodes[0].Data[0].Answers = nil }, lesson.CodeContract},
		{"collection null", func(v *lesson.Lesson) { v.Document.Pages[0].Nodes[0].Data[0].Value = []lesson.Data(nil) }, lesson.CodeContract},
		{"type", func(v *lesson.Lesson) { v.Document.Pages[0].Nodes[0].Type = "unknown" }, lesson.CodeContract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := revisionTestLesson()
			tc.mutate(candidate)
			before := revisionTestJSON(t, candidate)
			result, errors := revision.Initial(candidate, revisionTestAt(), diagnostic.Context{Path: "new.json"})
			if result != nil {
				t.Fatal("опубликован частичный результат")
			}
			revisionTestCode(t, errors, tc.code)
			revisionTestEqual(t, candidate, before)
		})
	}
	result, errors := revision.Initial(nil, revisionTestAt(), diagnostic.Context{})
	if result != nil {
		t.Fatal("nil: опубликован результат")
	}
	revisionTestCode(t, errors, revision.CodeInput)
}

func TestRevisionInitialNumberingBeyond999AndEmptyArrays(t *testing.T) {
	candidate := revisionTestLesson()
	candidate.Document.Pages = make([]lesson.Page, 1001)
	for i := range candidate.Document.Pages {
		candidate.Document.Pages[i].Nodes = []lesson.Node{revisionTestLeaf("", "")}
	}
	result, errors := revision.Initial(candidate, revisionTestAt(), diagnostic.Context{})
	if len(errors) != 0 || result == nil {
		t.Fatalf("Initial: %v", errors)
	}
	for _, i := range []int{0, 998, 999, 1000} {
		page := result.Document.Pages[i]
		if page.ID != fmt.Sprintf("efx7-p%03d", i+1) || page.Nodes[0].ID != fmt.Sprintf("efx7-n%03d", i+1) {
			t.Fatalf("страница %d: %+v", i, page)
		}
	}
	candidate.Document.Pages = []lesson.Page{}
	result, errors = revision.Initial(candidate, revisionTestAt(), diagnostic.Context{})
	if len(errors) != 0 || result == nil || result.Document.Pages == nil {
		t.Fatalf("пустые страницы: %v %v", result, errors)
	}
}

func TestRevisionApplyNoOpPreservesOldAndOwnsCopy(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(fmt.Sprint(rename), func(t *testing.T) {
			old := revisionTestLesson()
			candidate := revisionTestCandidate(t, old)
			candidate.Document.ID, candidate.Document.Version = "eFx7", "1"
			candidate.Document.CreatedAt, candidate.Document.LineCount = "2026-01-01T00:00:00Z", "999999"
			candidate.Document.Pages[0].ID = "efx7-p123"
			if rename {
				candidate.Document.Title = "отклонено"
				candidate.Document.Subtitle = nil
				candidate.Document.Pages[0].Title = nil
			}
			plan := revisionTestPlan(t, old, candidate)
			decisions := revisionTestDecisions(plan, revision.Reject)
			before, candidateBefore, planBefore := revisionTestJSON(t, old), revisionTestJSON(t, candidate), revisionTestJSON(t, plan)
			decisionBefore := revisionTestJSON(t, decisions)
			result, changed, errors := revision.Apply(old, candidate, plan, decisions, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), diagnostic.Context{}, diagnostic.Context{})
			if len(errors) != 0 || result == nil || changed {
				t.Fatalf("no-op: changed=%v errors=%v", changed, errors)
			}
			revisionTestEqual(t, result, before)
			revisionTestValid(t, result)
			revisionTestMutate(result)
			revisionTestEqual(t, old, before)
			revisionTestEqual(t, candidate, candidateBefore)
			revisionTestEqual(t, plan, planBefore)
			revisionTestEqual(t, decisions, decisionBefore)
		})
	}
}
