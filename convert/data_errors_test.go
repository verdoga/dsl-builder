package convert

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/textdata"
)

func TestDataFillDataStopsAtFirstBindingError(t *testing.T) {
	input, pages, bindings := dataLoad(t, "testdata/data/first-error.parsed.json")
	finder, usage := dataResources(t, input)
	if len(bindings) != 7 {
		t.Fatalf("bindings = %d", len(bindings))
	}
	first := bindings[2]
	if first.Node(pages).Type != lesson.NodeQuestion || first.Entries[0].Source.Number != 11 {
		t.Fatal("fixture does not exercise first included failure")
	}
	ownerData, _, owner := exercisedata.Build(lesson.NodeQuestion, first.Lines(), dataBase(input))
	if owner == nil || owner.Code != exercisedata.CodeSource || ownerData != nil {
		t.Fatalf("owner failure not reached: %v", owner)
	}
	source := dataJSON(t, input)
	beforeUsage := usage.Snapshot()
	beforeLater := make([][]byte, len(bindings)-2)
	for i := 2; i < len(bindings); i++ {
		beforeLater[i-2] = dataJSON(t, bindings[i].Node(pages))
	}
	problems := fillData(pages, bindings, finder, usage, dataBase(input))
	if len(problems) != 1 {
		t.Fatalf("expected exactly first failure, got %v", problems)
	}
	problem := problems[0]
	dataEqual(t, problem, bindingError(owner, first, dataBase(input)))
	if problem == owner || problem.Code != owner.Code || problem.Reason != owner.Reason || problem.Cause != owner.Cause || problem.Context != owner.Context {
		t.Fatalf("owner diagnostic was replaced: %+v", problem)
	}
	if problem.Context.Path != input.Path || problem.Context.FileName != "l013.json" || problem.Context.LessonID != "DATA-QA" || problem.Context.Line != 11 || problem.Context.PageID != "" {
		t.Fatalf("binding context: %+v", problem.Context)
	}
	var sites []int
	for _, place := range problem.Related {
		if place.Line == 7 || place.Line == 2 {
			sites = append(sites, place.Line)
		}
	}
	dataEqual(t, sites, []int{7, 2})
	if len(problem.Details) < len(owner.Details)+2 {
		t.Fatalf("missing page and NodePath details: %+v", problem.Details)
	}
	for i := 2; i < len(bindings); i++ {
		dataEqual(t, dataJSON(t, bindings[i].Node(pages)), beforeLater[i-2])
	}
	dataEqual(t, usage.Snapshot(), beforeUsage)
	dataEqual(t, dataJSON(t, input), source)
	dataText(t, dataRole(t, bindings[0].Node(pages).Data, lesson.RoleBody), "before root", []string{}, "")
	dataText(t, dataRole(t, bindings[1].Node(pages).Data, lesson.RoleBody), "before included", []string{}, "")
}

func TestDataFillExerciseFailureDoesNotAssignDataOrFlags(t *testing.T) {
	input, pages, bindings := dataLoad(t, "testdata/data/first-error.parsed.json")
	binding := bindings[2]
	node := binding.Node(pages)
	oldCheckable := true
	node.Flags.Checkable = &oldCheckable
	node.Data = []lesson.Data{dataSentinel()}
	before := dataJSON(t, node)
	_, _, want := exercisedata.Build(node.Type, binding.Lines(), dataBase(input))
	problem := fillExercise(node, binding, dataBase(input))
	if problem == nil || want == nil {
		t.Fatal("expected real owner error")
	}
	dataEqual(t, problem, want)
	dataEqual(t, dataJSON(t, node), before)
	if node.Flags.Checkable != &oldCheckable {
		t.Fatal("failure replaced Checkable pointer")
	}
}

func TestDataFillTextFailureAndFillNodeKeepOwnerErrorOrder(t *testing.T) {
	input, pages, bindings := dataLoad(t, "testdata/data/text-error.parsed.json")
	if len(bindings) != 1 {
		t.Fatalf("bindings = %d", len(bindings))
	}
	binding := bindings[0]
	node := binding.Node(pages)
	node.Data = []lesson.Data{dataSentinel()}
	before := dataJSON(t, node)
	declaration, lines := binding.TextSource()
	_, owner := textdata.Build(node.Type, declaration, lines, dataBase(input))
	if owner == nil || owner.Code != textdata.CodeInput || len(owner.Related) != 2 {
		t.Fatalf("expected owner ambiguity with ordered places: %+v", owner)
	}
	raw := fillText(node, binding, dataBase(input))
	dataEqual(t, raw, owner)
	dataEqual(t, dataJSON(t, node), before)
	finder, usage := dataResources(t, input)
	wrapped := fillNode(pages, binding, finder, usage, dataBase(input))
	if wrapped == nil {
		t.Fatal("fillNode dropped textdata failure")
	}
	dataEqual(t, wrapped, bindingError(owner, binding, dataBase(input)))
	dataEqual(t, wrapped.Related[:len(owner.Related)], owner.Related)
	dataEqual(t, dataJSON(t, node), before)
}

func TestDataFillDataEmptyBindingsHasNoEffect(t *testing.T) {
	input, pages, bindings := dataLoad(t, "testdata/data/empty.parsed.json")
	finder, usage := dataResources(t, input)
	if len(bindings) != 0 {
		t.Fatal("expected empty real Build bindings")
	}
	before := dataJSON(t, pages)
	if problems := fillData(pages, bindings, finder, usage, dataBase(input)); len(problems) != 0 {
		t.Fatalf("empty bindings: %v", problems)
	}
	dataEqual(t, dataJSON(t, pages), before)
}

func dataSentinel() lesson.Data {
	role := lesson.RoleBody
	return lesson.Data{Type: lesson.DataText, Role: &role, Value: "previous data", Answers: []string{}, Hint: "keep"}
}

func TestDataFillNodePreservesResourceErrorAndIncludeContext(t *testing.T) {
	input, pages, bindings := dataLoad(t, "testdata/data/media-error.parsed.json")
	finder, usage := dataResources(t, input)
	binding := bindings[2]
	node := binding.Node(pages)
	if node.Type != lesson.NodeMedia {
		t.Fatalf("kind = %s", node.Type)
	}
	node.Data = []lesson.Data{dataSentinel()}
	before := dataJSON(t, node)
	context := dataBase(input)
	context.Line, context.Column = 11, 14
	_, owner := finder.Find("audio", `"absent.mp3"`, "absent.mp3", context)
	if owner == nil {
		t.Fatal("missing file unexpectedly found")
	}
	problem := fillNode(pages, binding, finder, usage, dataBase(input))
	if problem == nil {
		t.Fatal("resource error dropped")
	}
	dataEqual(t, problem, bindingError(owner, binding, context))
	if problem.Code != owner.Code || problem.Reason != owner.Reason || problem.Context != owner.Context ||
		problem.Context.Path != filepath.Join(filepath.Dir(input.Path), "absent.mp3") || !errors.Is(problem, os.ErrNotExist) {
		t.Fatalf("resource cause/path replaced: %+v", problem)
	}
	var sites []int
	for _, place := range problem.Related {
		if place.Line == 7 || place.Line == 2 {
			sites = append(sites, place.Line)
		}
	}
	dataEqual(t, sites, []int{7, 2})
	dataEqual(t, dataJSON(t, node), before)
	if report := usage.Snapshot(); report.Total != 0 || report.Used != 0 {
		t.Fatalf("failed media changed usage: %+v", report)
	}
}
