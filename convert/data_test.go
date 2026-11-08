package convert

import (
	"encoding/base64"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/structure"
)

func TestDataFillDataDelegatesAndPreservesBindings(t *testing.T) {
	input, pages, bindings := dataLoad(t, "testdata/data/composite.parsed.json")
	finder, usage := dataResources(t, input)
	if len(pages) != 2 || len(bindings) != 50 {
		t.Fatalf("pages/bindings = %d/%d", len(pages), len(bindings))
	}
	source := dataJSON(t, input)
	nodes := make([]*lesson.Node, len(bindings))
	kinds := make([]lesson.NodeType, len(bindings))
	children := make([]int, len(bindings))
	flags := make([]lesson.NodeFlags, len(bindings))
	expected := make([][]lesson.Data, len(bindings))
	checkable := make([]*bool, len(bindings))
	for i, binding := range bindings {
		node := binding.Node(pages)
		nodes[i], flags[i] = node, node.Flags
		kinds[i], children[i] = node.Type, len(node.Nodes)
		if node.Type != lesson.NodeMedia {
			expected[i], checkable[i] = dataOwner(t, node, binding, dataBase(input))
		}
	}
	if problems := fillData(pages, bindings, finder, usage, dataBase(input)); len(problems) != 0 {
		t.Fatalf("fillData: %v", problems)
	}
	dataEqual(t, dataJSON(t, input), source)
	for i, binding := range bindings {
		node := binding.Node(pages)
		if node != nodes[i] || node.ID != "" || node.Type != kinds[i] || len(node.Nodes) != children[i] {
			t.Fatalf("binding %d lost its original node address or acquired ID", i)
		}
		if node.Data == nil || node.Nodes == nil {
			t.Fatalf("binding %d has nil output arrays", i)
		}
		if node.Type != lesson.NodeMedia {
			dataEqual(t, node.Data, expected[i])
		}
		if checkable[i] == nil {
			dataEqual(t, node.Flags, flags[i])
		} else {
			if node.Flags.Checkable == nil || *node.Flags.Checkable != *checkable[i] {
				t.Fatalf("binding %d Checkable: got %v want %t", i, node.Flags.Checkable, *checkable[i])
			}
			if node.Flags.Speaking != nil || node.Flags.Stepped != nil {
				t.Fatalf("exercise %d acquired task flags", i)
			}
		}
	}
	// Одинаковые физические строки трёх включений адресуют разные ноды.
	for _, i := range []int{4, 18, 34} {
		dataText(t, dataRole(t, nodes[i].Data, lesson.RoleBody), "Alpha <b>literal</b>\nBeta & literal", []string{}, "")
	}
	if bindings[4].Entries[0] == bindings[18].Entries[0] || nodes[4] == nodes[18] || nodes[18] == nodes[34] {
		t.Fatal("repeated inclusions share identity")
	}
	dataText(t, dataRole(t, nodes[5].Data, lesson.RoleBody), "Nested inclusion text", []string{}, "")
	dataText(t, dataRole(t, nodes[3].Data, lesson.RoleBody), "Instruction A\nInstruction B", []string{}, "instruction hint")
	task := nodes[1]
	if task.Flags.Speaking == nil || !*task.Flags.Speaking || task.Flags.Stepped == nil || !*task.Flags.Stepped || task.Flags.Checkable != nil {
		t.Fatalf("task flags lost: %s", dataJSON(t, task.Flags))
	}
	dataText(t, dataRole(t, nodes[6].Data, lesson.RoleBody), "Checked", []string{"answer one", "answer two"}, "question hint")
	dataText(t, dataRole(t, nodes[49].Data, lesson.RoleBody), "Unchecked", []string{}, "")
	if nodes[49].Flags.Checkable == nil || *nodes[49].Flags.Checkable {
		t.Fatal("false must be an explicit Checkable pointer")
	}
	choice := dataCollection(t, dataRole(t, nodes[7].Data, lesson.RoleOptions), lesson.RoleOption)
	dataText(t, choice[0], "wrong", []string{}, "")
	dataText(t, choice[1], "right", []string{"right"}, "choice hint")
	matching := dataCollection(t, dataRole(t, nodes[10].Data, lesson.RoleItems), lesson.RoleItem)
	dataText(t, matching[0], "item one", []string{"option A"}, "match hint")
	ordering := dataCollection(t, dataRole(t, nodes[12].Data, lesson.RoleItems), lesson.RoleItem)
	dataText(t, ordering[0], "item A", []string{"2"}, "")
	dataText(t, ordering[1], "item B", []string{"1"}, "order hint")
	for _, pair := range []struct {
		index int
		want  string
	}{{7, "Choose"}, {8, "Many"}} {
		dataText(t, dataRole(t, nodes[pair.index].Data, lesson.RoleInstruction), pair.want, []string{}, "")
	}
	for _, pair := range []struct {
		instruction, matching int
		page                  int
		instructionPath       []int
		matchingPath          []int
	}{
		{9, 10, 0, []int{1, 0, 6}, []int{1, 0, 7}},
		{23, 24, 0, []int{1, 1, 5}, []int{1, 1, 6}},
		{39, 40, 1, []int{0, 0, 0, 5}, []int{0, 0, 0, 6}},
	} {
		instruction, matching := bindings[pair.instruction], bindings[pair.matching]
		if instruction.Page != pair.page || matching.Page != pair.page ||
			nodes[pair.instruction].Type != lesson.NodeInstruction || nodes[pair.matching].Type != lesson.NodeMatching {
			t.Fatalf("instruction/matching bindings %d/%d: wrong page or type", pair.instruction, pair.matching)
		}
		dataEqual(t, instruction.NodePath, pair.instructionPath)
		dataEqual(t, matching.NodePath, pair.matchingPath)
		if len(instruction.Entries) != 1 || instruction.Entries[0] != matching.Entries[0] || instruction.Entries[0].Source.Number != 47 {
			t.Fatalf("instruction %d lost its matching declaration binding", pair.instruction)
		}
		dataText(t, dataRole(t, nodes[pair.instruction].Data, lesson.RoleBody), "Match", []string{}, "")
		for _, datum := range nodes[pair.matching].Data {
			if datum.Role != nil && *datum.Role == lesson.RoleInstruction {
				t.Fatalf("matching %d retained a local instruction", pair.matching)
			}
		}
	}
	if bindings[9].Entries[0] == bindings[23].Entries[0] || bindings[23].Entries[0] == bindings[39].Entries[0] {
		t.Fatal("repeated matching declarations share identity")
	}
	for _, i := range []int{14, 28, 44} {
		dataEqual(t, nodes[i].Flags, lesson.NodeFlags{})
		dataText(t, dataRole(t, nodes[i].Data, lesson.RoleBody), "I _____{am} here.\n\nWe _____{are} ready.", []string{}, "")
	}
	for _, i := range []int{11, 13, 25, 27, 41, 43} {
		if nodes[i].Type != lesson.NodeInstruction {
			t.Fatalf("binding %d: %s", i, nodes[i].Type)
		}
	}
	for _, i := range []int{15, 29, 45} {
		node := nodes[i]
		if node.Type != lesson.NodeMedia || len(node.Data) != 3 {
			t.Fatalf("media %d: %s", i, dataJSON(t, node))
		}
		for j, want := range []struct {
			role       lesson.DataRole
			name, mime string
			raw        []byte
		}{
			{lesson.RoleBody, "track.mp3", "audio/mpeg", []byte{0, 1, 254, 255, '\n'}},
			{lesson.RoleScript, "track.txt", "text/plain", []byte("\xef\xbb\xbfFile transcript\r\n")},
			{lesson.RoleSubtitles, "track.vtt", "text/vtt", []byte("WEBVTT\r\n\r\n")},
		} {
			datum := node.Data[j]
			if datum.Type != lesson.DataMedia || datum.Role == nil || *datum.Role != want.role || datum.Answers == nil || len(datum.Answers) != 0 || datum.Hint != "" {
				t.Fatalf("media datum: %s", dataJSON(t, datum))
			}
			file, ok := datum.Value.(lesson.EmbeddedFile)
			if !ok || file.FileName != want.name || file.MIMEType != want.mime {
				t.Fatalf("media file: %s", dataJSON(t, datum))
			}
			raw, err := base64.StdEncoding.DecodeString(file.Content)
			if err != nil {
				t.Fatal(err)
			}
			dataEqual(t, raw, want.raw)
		}
		if nodes[i+1].Type != lesson.NodeScript {
			t.Fatal("source script merged into media")
		}
		dataText(t, dataRole(t, nodes[i+1].Data, lesson.RoleBody), "File and source remain separate", []string{}, "")
	}
	report := usage.Snapshot()
	if report.Total != 3 || report.Used != 3 || report.Unused != 0 {
		t.Fatalf("repeated media count: %+v", report)
	}
	dataIndependentChecks(t, nodes, checkable)
	dataShape(t, pages, bindings)
}

func dataIndependentChecks(t *testing.T, nodes []*lesson.Node, expected []*bool) {
	t.Helper()
	for i, node := range nodes {
		if expected[i] == nil {
			continue
		}
		for j := 0; j < i; j++ {
			if expected[j] != nil && node.Flags.Checkable == nodes[j].Flags.Checkable {
				t.Fatalf("bindings %d/%d share Checkable pointer", j, i)
			}
		}
	}
	// Изменение одного результата не изменяет другой экземпляр включения.
	before := *nodes[20].Flags.Checkable
	*nodes[6].Flags.Checkable = !*nodes[6].Flags.Checkable
	if *nodes[20].Flags.Checkable != before {
		t.Fatal("Checkable aliases another node")
	}
}

func dataShape(t *testing.T, pages []lesson.Page, bindings []structure.Binding) {
	t.Helper()
	if len(pages) != 2 || len(pages[0].Nodes) != 2 || len(pages[1].Nodes) != 1 {
		t.Fatal("page/root shape changed")
	}
	if len(pages[0].Nodes[1].Nodes) != 2 || len(pages[1].Nodes[0].Nodes) != 2 {
		t.Fatal("steps/variants shape changed")
	}
	for i, binding := range bindings {
		if binding.Page < 0 || binding.Page >= len(pages) {
			t.Fatalf("binding %d page changed", i)
		}
	}
}
