package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/revision"
)

func TestUpdateGoldenSevenNestedNodeUsesOwnerRevision(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "golden", "7. parsed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var before, after model.Result
	if err := json.Unmarshal(raw, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &after); err != nil {
		t.Fatal(err)
	}
	// Изменяется точный известный source-адрес ДО parsed.Load, не LoadedInput.
	line := &after.Lines[7]
	if line.Number != 8 || line.Raw != "@instr INSTRUCTION TEXT" || len(line.Elements) != 2 ||
		line.Elements[1].ElementType != model.ElementTypeContent || line.Elements[1].Start != 8 {
		t.Fatal("golden seven instruction fixture changed")
	}
	body := "CHANGED INSTRUCTION"
	line.Raw, line.Elements[1].Raw, line.Elements[1].Value = "@instr "+body, body, &body
	line.Elements[1].End = 8 + len([]rune(body))
	op, _, _, _ := updateFlowStart(t, before, after, []*string{nil, nil}, []string{"10", "20"})
	plan := op.Snapshot().Plan
	if plan.ReplaceLesson || len(plan.Fields) != 0 || len(plan.Pages) != 1 || plan.Pages[0].ReplaceNodes ||
		len(plan.Pages[0].Nodes) != 1 || len(plan.Pages[0].Nodes[0].Path) < 3 {
		t.Fatalf("golden nested edit did not stay one nested node: %+v", plan)
	}
	if fields := plan.Pages[0].Nodes[0].Fields; len(fields) != 1 || fields[0] != compare.NodeDataChanged {
		t.Fatalf("unexpected nested node field: %+v", fields)
	}
	want, changed, issues := revision.Apply(op.oldFile.Lesson(), op.newFile.Lesson(), plan, nil, op.at, op.oldFile.Context(), op.newFile.Context())
	updateFixtureOK(t, issues)
	if !changed {
		t.Fatal("nested edit did not change owner revision")
	}
	updateFixtureOK(t, op.PrepareRevision(op.Snapshot().Token))
	updateFlowEqual(t, op.Snapshot().After.Lesson(), want)
	updateFlowEqual(t, op.Snapshot().Plan, plan)
	if op.Snapshot().After.Lesson().Document.Version != "6" {
		t.Fatal("nested golden update was not exactly one revision")
	}
}
