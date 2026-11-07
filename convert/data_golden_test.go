package convert

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
)

func TestDataFillDataMatchesGoldenDataAndStructure(t *testing.T) {
	for _, number := range []int{2, 4, 5, 6, 7} {
		t.Run(fmt.Sprint(number), func(t *testing.T) {
			input, pages, bindings := dataLoad(t, filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", number)))
			finder, usage := dataResources(t, input)
			raw, err := os.ReadFile(filepath.Join("..", "golden", fmt.Sprintf("%d. lesson.json", number)))
			if err != nil {
				t.Fatal(err)
			}
			var expected lesson.Lesson
			if err := json.Unmarshal(raw, &expected); err != nil {
				t.Fatal(err)
			}
			for i := range expected.Document.Pages {
				page := &expected.Document.Pages[i]
				page.ID = ""
				dataUnnumberExpected(page.Nodes)
			}
			source := dataJSON(t, input)
			if problems := fillData(pages, bindings, finder, usage, dataBase(input)); len(problems) != 0 {
				t.Fatalf("fillData: %v", problems)
			}
			dataEqual(t, pages, expected.Document.Pages)
			dataEqual(t, dataJSON(t, input), source)
			report := usage.Snapshot()
			if report.Total != 0 || report.Used != 0 || report.Unused != 0 {
				t.Fatalf("text data registered resources: %+v", report)
			}
		})
	}
}

func dataUnnumberExpected(nodes []lesson.Node) {
	for i := range nodes {
		nodes[i].ID = ""
		// ТЗ 10.7: старый эталон содержит отменённый multifill.checkable=false.
		if nodes[i].Type == lesson.NodeMultifill {
			nodes[i].Flags.Checkable = nil
		}
		dataUnnumberExpected(nodes[i].Nodes)
	}
}

func TestDataFillExerciseFalseIsExplicitAndIndependentForEveryKind(t *testing.T) {
	input, pages, bindings := dataLoad(t, "testdata/data/unchecked.parsed.json")
	wantKinds := []lesson.NodeType{lesson.NodeQuestion, lesson.NodeQuestion, lesson.NodeSingleChoice,
		lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering}
	if len(bindings) != len(wantKinds) {
		t.Fatalf("bindings = %d", len(bindings))
	}
	pointers := make([]*bool, 0, len(bindings))
	for i, binding := range bindings {
		node := binding.Node(pages)
		if node.Type != wantKinds[i] {
			t.Fatalf("binding %d kind = %s", i, node.Type)
		}
		expected, checkable := dataOwner(t, node, binding, dataBase(input))
		if checkable == nil || *checkable {
			t.Fatal("unchecked fixture unexpectedly checkable")
		}
		if problem := fillExercise(node, binding, dataBase(input)); problem != nil {
			t.Fatal(problem)
		}
		dataEqual(t, node.Data, expected)
		if node.Flags.Checkable == nil || *node.Flags.Checkable || node.Flags.Speaking != nil || node.Flags.Stepped != nil {
			t.Fatalf("false flags: %s", dataJSON(t, node.Flags))
		}
		for _, previous := range pointers {
			if previous == node.Flags.Checkable {
				t.Fatal("Checkable pointer reused")
			}
		}
		pointers = append(pointers, node.Flags.Checkable)
	}
	*pointers[0] = true
	for _, pointer := range pointers[1:] {
		if *pointer {
			t.Fatal("changing one false result changed another")
		}
	}
}
