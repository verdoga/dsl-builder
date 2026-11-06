package lesson

import (
	"encoding/json"
	"testing"
)

func TestLessonNodeModelTypes(t *testing.T) {
	cases := []struct {
		value NodeType
		want  string
	}{
		{NodeTask, "task"},
		{NodeStep, "step"},
		{NodeVariants, "variants"},
		{NodeVariant, "variant"},
		{NodeHeading, "heading"},
		{NodeText, "text"},
		{NodeInstruction, "instruction"},
		{NodeNote, "note"},
		{NodeAlt, "alt"},
		{NodeScript, "script"},
		{NodeHint, "hint"},
		{NodeAnswerKey, "answerKey"},
		{NodeExample, "example"},
		{NodeWordList, "wordList"},
		{NodeTable, "table"},
		{NodeMedia, "media"},
		{NodeQuestion, "question"},
		{NodeMultifill, "multifill"},
		{NodeSingleChoice, "singleChoice"},
		{NodeMultipleChoice, "multipleChoice"},
		{NodeMatching, "matching"},
		{NodeOrdering, "ordering"},
	}
	seen := make(map[NodeType]bool)
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if string(tc.value) != tc.want {
				t.Errorf("NodeType = %q, want %q", tc.value, tc.want)
			}
			if seen[tc.value] {
				t.Errorf("duplicate NodeType %q", tc.value)
			}
			seen[tc.value] = true
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != `"`+tc.want+`"` {
				t.Errorf("JSON = %s, want string %q", raw, tc.want)
			}
		})
	}
	if len(seen) != 22 {
		t.Errorf("distinct required NodeType values = %d, want 22", len(seen))
	}
}

func TestLessonNodeModelRequiredJSONFields(t *testing.T) {
	node := Node{Data: []Data{}, Nodes: []Node{}}
	fields := nodeModelObject(t, nodeModelMarshal(t, node))
	want := map[string]string{
		"id": `""`, "type": `""`, "flags": `{}`, "data": `[]`, "nodes": `[]`,
	}
	if len(fields) != len(want) {
		t.Fatalf("Node JSON has %d fields, want exactly five: %s", len(fields), nodeModelMarshal(t, node))
	}
	for key, value := range want {
		if string(fields[key]) != value {
			t.Errorf("field %q = %s, want %s", key, fields[key], value)
		}
	}
}

func TestLessonNodeModelFlags(t *testing.T) {
	falseValue, trueValue := false, true
	cases := []struct {
		name  string
		flags NodeFlags
		want  map[string]bool
	}{
		{"absent", NodeFlags{}, map[string]bool{}},
		{"speaking_false", NodeFlags{Speaking: &falseValue}, map[string]bool{"speaking": false}},
		{"speaking_true", NodeFlags{Speaking: &trueValue}, map[string]bool{"speaking": true}},
		{"stepped_false", NodeFlags{Stepped: &falseValue}, map[string]bool{"stepped": false}},
		{"stepped_true", NodeFlags{Stepped: &trueValue}, map[string]bool{"stepped": true}},
		{"checkable_false", NodeFlags{Checkable: &falseValue}, map[string]bool{"checkable": false}},
		{"checkable_true", NodeFlags{Checkable: &trueValue}, map[string]bool{"checkable": true}},
		{"all_false", NodeFlags{Speaking: &falseValue, Stepped: &falseValue, Checkable: &falseValue}, map[string]bool{"speaking": false, "stepped": false, "checkable": false}},
		{"all_true", NodeFlags{Speaking: &trueValue, Stepped: &trueValue, Checkable: &trueValue}, map[string]bool{"speaking": true, "stepped": true, "checkable": true}},
		{"mixed", NodeFlags{Speaking: &trueValue, Stepped: &falseValue, Checkable: &trueValue}, map[string]bool{"speaking": true, "stepped": false, "checkable": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := nodeModelMarshal(t, tc.flags)
			fields := nodeModelObject(t, raw)
			if len(fields) != len(tc.want) {
				t.Fatalf("flags JSON = %s, want %v", raw, tc.want)
			}
			for key, value := range tc.want {
				want := "false"
				if value {
					want = "true"
				}
				if string(fields[key]) != want {
					t.Errorf("flag %q = %s, want %s", key, fields[key], want)
				}
			}
			var decoded NodeFlags
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			for key, flag := range map[string]*bool{
				"speaking": decoded.Speaking, "stepped": decoded.Stepped, "checkable": decoded.Checkable,
			} {
				value, present := tc.want[key]
				if present && (flag == nil || *flag != value) {
					t.Errorf("decoded flag %q does not preserve %v", key, value)
				}
				if !present && flag != nil {
					t.Errorf("decoded absent flag %q is non-nil", key)
				}
			}
		})
	}
}

func TestLessonNodeModelRecursiveOrder(t *testing.T) {
	body := RoleBody
	node := Node{
		ID: "lesson-p001-n001-v7", Type: NodeTask, Data: []Data{},
		Nodes: []Node{
			{ID: "lesson-p001-n009", Type: NodeStep, Data: []Data{}, Nodes: []Node{
				{ID: "lesson-p001-n003-v2", Type: NodeVariants, Data: []Data{}, Nodes: []Node{
					{ID: "lesson-p001-n010", Type: NodeVariant, Data: []Data{}, Nodes: []Node{
						{ID: "lesson-p001-n007", Type: NodeText, Nodes: []Node{}, Data: []Data{
							{Type: DataText, Role: &body, Value: "Первый\nтекст 🐈", Answers: []string{}, Hint: ""},
							{Type: DataText, Role: &body, Value: "Второй", Answers: []string{}, Hint: ""},
						}},
					}},
				}},
			}},
			{ID: "lesson-p001-n002", Type: NodeNote, Data: []Data{}, Nodes: []Node{}},
		},
	}
	nodeModelCheckTree(t, nodeModelMarshal(t, node), node)
}

func nodeModelMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func nodeModelObject(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields == nil {
		t.Fatalf("expected JSON object, got %s", raw)
	}
	return fields
}

func nodeModelCheckTree(t *testing.T, raw []byte, want Node) {
	t.Helper()
	fields := nodeModelObject(t, raw)
	if len(fields) != 5 {
		t.Fatalf("node %q has %d fields, want five", want.ID, len(fields))
	}
	for key, value := range map[string]string{"id": want.ID, "type": string(want.Type)} {
		var got string
		if err := json.Unmarshal(fields[key], &got); err != nil {
			t.Fatalf("node %q field %s: %v", want.ID, key, err)
		}
		if got != value {
			t.Errorf("node %q field %s = %q, want %q", want.ID, key, got, value)
		}
	}
	if string(fields["flags"]) != `{}` {
		t.Errorf("node %q flags = %s, want {}", want.ID, fields["flags"])
	}
	var children, data []json.RawMessage
	if err := json.Unmarshal(fields["nodes"], &children); err != nil {
		t.Fatal(err)
	}
	if children == nil || len(children) != len(want.Nodes) {
		t.Fatalf("node %q children = %s, want %d ordered children", want.ID, fields["nodes"], len(want.Nodes))
	}
	if err := json.Unmarshal(fields["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data == nil || len(data) != len(want.Data) {
		t.Fatalf("node %q data = %s, want %d ordered entries", want.ID, fields["data"], len(want.Data))
	}
	for i, entry := range data {
		var got string
		if err := json.Unmarshal(nodeModelObject(t, entry)["value"], &got); err != nil {
			t.Fatal(err)
		}
		if got != want.Data[i].Value {
			t.Errorf("node %q data[%d].value = %q, want %q", want.ID, i, got, want.Data[i].Value)
		}
	}
	for i, child := range children {
		nodeModelCheckTree(t, child, want.Nodes[i])
	}
}
