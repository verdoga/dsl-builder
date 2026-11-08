package lesson

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func nodeValidationContext() diagnostic.Context {
	return diagnostic.Context{Path: "урок.json", FileName: "lesson.json", LessonID: "Ab", PageID: "ab-p001", Line: 7, Column: 3}
}

func nodeValidationCheck(t *testing.T, got []*diagnostic.Error, want ...string) {
	t.Helper()
	var pointers []string
	for _, err := range got {
		if err == nil {
			t.Fatal("nil diagnostic")
		}
		if err.Code != CodeContract || err.Context != nodeValidationContext() || err.Reason == "" {
			t.Errorf("invalid diagnostic: %+v", err)
		}
		if strings.IndexFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) < 0 {
			t.Errorf("reason must be Russian: %q", err.Reason)
		}
		found := false
		for _, detail := range err.Details {
			if detail.Label == "Поле JSON" {
				pointers = append(pointers, detail.Value)
				found = true
			}
		}
		if !found {
			t.Errorf("missing JSON pointer: %+v", err)
		}
	}
	if !slices.Equal(pointers, want) {
		t.Fatalf("pointers = %q, want %q", pointers, want)
	}
}

func nodeValidationTypes() []NodeType {
	return []NodeType{NodeTask, NodeStep, NodeVariants, NodeVariant, NodeHeading, NodeText, NodeInstruction, NodeNote, NodeAlt, NodeScript, NodeHint, NodeAnswerKey, NodeExample, NodeWordList, NodeTable, NodeMedia, NodeQuestion, NodeMultifill, NodeSingleChoice, NodeMultipleChoice, NodeMatching, NodeOrdering}
}

func nodeValidationData(role DataRole, count int) Data {
	d := Data{Type: DataText, Role: &role, Value: "<b>текст</b>\n\n_____{ANSWER}", Answers: []string{"не вычислять"}, Hint: "произвольная подсказка"}
	childRole := map[DataRole]DataRole{RoleItems: RoleItem, RoleOptions: RoleOption, RolePositions: RolePosition}[role]
	if childRole != "" {
		children := make([]Data, count)
		for i := range children {
			children[i] = Data{Type: DataText, Role: &childRole, Value: "", Answers: []string{}}
		}
		d.Type, d.Value = DataCollection, children
	}
	return d
}

func nodeValidationNode(kind NodeType) Node {
	n := Node{Type: kind, Data: []Data{}, Nodes: []Node{}}
	if kind == NodeSingleChoice || kind == NodeMultipleChoice || kind == NodeMatching {
		n.Data = append(n.Data, nodeValidationData(RoleOptions, 2))
	}
	if kind == NodeMatching || kind == NodeOrdering {
		n.Data = append(n.Data, nodeValidationData(RoleItems, 2))
	}
	return n
}

func TestLessonNodeValidationTypesAndFlags(t *testing.T) {
	c := nodeValidationContext()
	for _, kind := range append(nodeValidationTypes(), "", "Task", "unknown") {
		t.Run(string(kind), func(t *testing.T) {
			err := validateNodeType(kind, "/node/type", c)
			known := slices.Contains(nodeValidationTypes(), kind)
			if known {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				nodeValidationCheck(t, []*diagnostic.Error{err}, "/node/type")
				return
			}
			nodeValidationCheck(t, validateNode(nodeValidationNode(kind), "/node", c))
			f, tr := false, true
			for i, speaking := range []*bool{nil, &f, &tr} {
				for j, stepped := range []*bool{nil, &f, &tr} {
					for k, checkable := range []*bool{nil, &f, &tr} {
						t.Run(fmt.Sprintf("%d%d%d", i, j, k), func(t *testing.T) {
							var want []string
							if speaking != nil && kind != NodeTask {
								want = append(want, "/flags/speaking")
							}
							if stepped != nil && kind != NodeTask {
								want = append(want, "/flags/stepped")
							}
							interactive := slices.Contains([]NodeType{NodeMultifill, NodeSingleChoice, NodeMultipleChoice, NodeOrdering, NodeMatching, NodeQuestion}, kind)
							if checkable != nil && !interactive {
								want = append(want, "/flags/checkable")
							}
							nodeValidationCheck(t, validateFlags(kind, NodeFlags{speaking, stepped, checkable}, "/flags", c), want...)
						})
					}
				}
			}
		})
	}
}

func TestLessonNodeValidationDirectRoles(t *testing.T) {
	roles := []DataRole{"", "BODY", "unknown", RoleTitle, RoleBody, RoleInstruction, RoleBefore, RoleAfter, RoleItems, RoleItem, RoleOptions, RoleOption, RolePositions, RolePosition, RoleHint, RoleScript, RoleSubtitles}
	for _, kind := range nodeValidationTypes() {
		for _, role := range roles {
			t.Run(string(kind)+"/"+string(role), func(t *testing.T) {
				allowed := role != "" && role != "BODY" && role != "unknown" && role != RoleItem && role != RoleOption && role != RolePosition
				if role == RoleInstruction {
					allowed = kind == NodeSingleChoice || kind == NodeMultipleChoice
				}
				if role == RoleOptions {
					allowed = kind == NodeSingleChoice || kind == NodeMultipleChoice || kind == NodeMatching
				}
				if role == RolePositions {
					allowed = kind == NodeOrdering
				}
				err := validateDirectRole(kind, &role, "/data/0/role", nodeValidationContext())
				if allowed {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				nodeValidationCheck(t, []*diagnostic.Error{err}, "/data/0/role")
			})
		}
		nodeValidationCheck(t, []*diagnostic.Error{validateDirectRole(kind, nil, "/data/0/role", nodeValidationContext())}, "/data/0/role")
	}
}

func TestLessonNodeValidationCollectionCounts(t *testing.T) {
	c := nodeValidationContext()
	for _, kind := range nodeValidationTypes() {
		base := nodeValidationNode(kind).Data
		t.Run(string(kind), func(t *testing.T) {
			nodeValidationCheck(t, validateCollectionCounts(kind, base, "/data", c))
			for i, d := range base {
				t.Run(string(*d.Role), func(t *testing.T) {
					missing := append(slices.Clone(base[:i]), base[i+1:]...)
					nodeValidationCheck(t, validateCollectionCounts(kind, missing, "/data", c), "/data")
					duplicate := append(slices.Clone(base), d)
					nodeValidationCheck(t, validateCollectionCounts(kind, duplicate, "/data", c), "/data")
				})
			}
			if kind == NodeSingleChoice || kind == NodeMultipleChoice {
				d := append(slices.Clone(base), nodeValidationData(RoleInstruction, 0))
				nodeValidationCheck(t, validateCollectionCounts(kind, d, "/data", c))
				nodeValidationCheck(t, validateCollectionCounts(kind, append(d, d[len(d)-1]), "/data", c), "/data")
			}
			if kind == NodeOrdering {
				d := append(slices.Clone(base), nodeValidationData(RolePositions, 2))
				nodeValidationCheck(t, validateCollectionCounts(kind, d, "/data", c))
				nodeValidationCheck(t, validateCollectionCounts(kind, append(d, d[len(d)-1]), "/data", c), "/data")
			}
			if kind != NodeMatching && kind != NodeOrdering {
				d := append(slices.Clone(base), nodeValidationData(RoleItems, 0), nodeValidationData(RoleItems, 0))
				nodeValidationCheck(t, validateNodeData(kind, d, "/data", c))
			}
		})
	}
	for _, tc := range []struct {
		kind NodeType
		size int
		bad  bool
	}{{NodeMatching, 0, true}, {NodeMatching, 1, false}, {NodeOrdering, 0, true}, {NodeOrdering, 1, true}, {NodeOrdering, 2, false}} {
		t.Run(fmt.Sprintf("minimum/%s/%d", tc.kind, tc.size), func(t *testing.T) {
			d := []Data{nodeValidationData(RoleItems, tc.size)}
			if tc.kind == NodeMatching {
				d = append(d, nodeValidationData(RoleOptions, 2))
			}
			var want []string
			if tc.bad {
				want = []string{"/data/0/value"}
			}
			nodeValidationCheck(t, validateCollectionCounts(tc.kind, d, "/data", c), want...)
		})
	}
}

func TestLessonNodeValidationDelegatesData(t *testing.T) {
	c := nodeValidationContext()
	for _, tc := range []struct {
		kind NodeType
		role DataRole
	}{{NodeSingleChoice, RoleOptions}, {NodeMultipleChoice, RoleOptions}, {NodeMatching, RoleOptions}, {NodeOrdering, RolePositions}} {
		for size := 0; size <= 2; size++ {
			t.Run(fmt.Sprintf("%s/%s/%d", tc.kind, tc.role, size), func(t *testing.T) {
				d := nodeValidationNode(tc.kind).Data
				index := 0
				if tc.role == RoleOptions {
					d[0] = nodeValidationData(tc.role, size)
				} else {
					d = append(d, nodeValidationData(tc.role, size))
					index = len(d) - 1
				}
				nodeValidationCheck(t, validateCollectionCounts(tc.kind, d, "/data", c))
				var want []string
				if size < 2 {
					want = []string{fmt.Sprintf("/data/%d/value", index)}
				}
				nodeValidationCheck(t, validateNodeData(tc.kind, d, "/data", c), want...)
			})
		}
	}
	body := RoleBody
	for _, tc := range []struct {
		name, pointer string
		d             Data
	}{
		{"nil-role", "/data/0/role", Data{Type: DataText, Value: "", Answers: []string{}}},
		{"wrong-value", "/data/0/value", Data{Type: DataText, Role: &body, Value: 17, Answers: []string{}}},
		{"nil-answers", "/data/0/answers", Data{Type: DataText, Role: &body, Value: ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodeValidationCheck(t, validateNodeData(NodeText, []Data{tc.d}, "/data", c), tc.pointer)
		})
	}
	ordinary := nodeValidationData(RoleBody, 0)
	ordinary.Type, ordinary.Value = DataCollection, []Data{{Type: DataText, Value: "", Answers: []string{}}, nodeValidationData(RoleItems, 0)}
	nodeValidationCheck(t, validateNodeData(NodeTable, []Data{ordinary}, "/data", c))
	ordinary.Value = []Data{nodeValidationData(RoleOption, 0)}
	nodeValidationCheck(t, validateNodeData(NodeText, []Data{ordinary}, "/data", c), "/data/0/value/0/role")
}

func TestLessonNodeValidationRecursionOrderAndNonmutation(t *testing.T) {
	c := nodeValidationContext()
	n := nodeValidationNode(NodeText)
	n.Data, n.Nodes = nil, nil
	nodeValidationCheck(t, validateNode(n, "/node", c), "/node/data", "/node/nodes")
	f := false
	n.Type, n.Flags.Checkable = "invalid", &f
	nodeValidationCheck(t, validateNode(n, "/node", c), "/node/type", "/node/flags/checkable", "/node/data", "/node/nodes")
	n = nodeValidationNode(NodeText)
	for _, kind := range nodeValidationTypes() {
		parent := nodeValidationNode(kind)
		parent.Nodes = []Node{n}
		n = parent
	}
	nodeValidationCheck(t, validateNode(n, "/node", c))
	tr := true
	for _, kind := range []NodeType{NodeMultifill, NodeSingleChoice, NodeMultipleChoice, NodeOrdering, NodeMatching, NodeQuestion} {
		for _, flag := range []*bool{nil, &f, &tr} {
			interactive := nodeValidationNode(kind)
			interactive.Flags.Checkable = flag
			interactive.Data = append(interactive.Data, nodeValidationData(RoleBody, 0))
			nodeValidationCheck(t, validateNode(interactive, "/node", c))
		}
	}
	leaf := nodeValidationNode(NodeMultifill)
	leaf.Data = []Data{nodeValidationData(RoleBody, 0)}
	leaf.Flags.Checkable = &f
	nodeValidationCheck(t, validateNode(leaf, "/node", c))
	leaf.Flags.Checkable = &tr
	leaf.Data[0].Answers = []string{}
	nodeValidationCheck(t, validateNode(leaf, "/node", c))
	leaf.Type = "invalid"
	const depth = 1024
	for range depth {
		parent := nodeValidationNode(NodeText)
		parent.Nodes = []Node{leaf}
		leaf = parent
	}
	before, err := json.Marshal(leaf)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		nodeValidationCheck(t, validateNode(leaf, "/node", c), "/node"+strings.Repeat("/nodes/0", depth)+"/type", "/node"+strings.Repeat("/nodes/0", depth)+"/flags/checkable")
	}
	after, err := json.Marshal(leaf)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("validation mutated the model")
	}
	n = nodeValidationNode(NodeText)
	a, b := nodeValidationNode(NodeText), nodeValidationNode(NodeText)
	a.Type, b.Data = "invalid", nil
	n.Nodes = []Node{a, b}
	nodeValidationCheck(t, validateNode(n, "/node", c), "/node/nodes/0/type", "/node/nodes/1/data")
}
