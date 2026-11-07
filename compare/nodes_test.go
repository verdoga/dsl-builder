package compare

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func task15Node(t *testing.T, text string, children ...lesson.Node) lesson.Node {
	t.Helper()
	role := lesson.RoleBody
	return lesson.Node{Type: lesson.NodeText, Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: text, Answers: []string{}}}, Nodes: append([]lesson.Node{}, children...)}
}

func task15Forest(t *testing.T, nodes ...lesson.Node) []lesson.Node {
	t.Helper()
	forest := append([]lesson.Node{}, nodes...)
	number := 0
	var assign func([]lesson.Node)
	assign = func(branch []lesson.Node) {
		for i := range branch {
			number++
			branch[i].ID = fmt.Sprintf("qa-n%03d", number)
			assign(branch[i].Nodes)
		}
	}
	assign(forest)
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{ID: "QA", Version: "1", LineCount: "0", CreatedAt: "2026-11-06T00:00:00Z", Pages: []lesson.Page{{ID: "qa-p001", Nodes: forest}}}}
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("invalid fixture: %v", problems)
	}
	return forest
}

func task15Reason(t *testing.T, got Reason, code ReasonCode, pointer string, before, after int) {
	t.Helper()
	if got.Code != code || got.OldPointer != pointer || got.NewPointer != pointer {
		t.Fatalf("reason address/code: %+v; want %s at %s", got, code, pointer)
	}
	if !strings.ContainsFunc(got.Description, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("description must be Russian: %q", got.Description)
	}
	numbers := regexp.MustCompile(`[0-9]+`).FindAllString(got.Description, -1)
	if !slices.Contains(numbers, strconv.Itoa(before)) || !slices.Contains(numbers, strconv.Itoa(after)) {
		t.Errorf("description %q must contain exact counts %d and %d", got.Description, before, after)
	}
}

func task15Changes(t *testing.T, got, want []NodeChange) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("changes: %+v; want %+v", got, want)
	}
	for i := range want {
		if !slices.Equal(got[i].Path, want[i].Path) || !slices.Equal(got[i].Fields, want[i].Fields) {
			t.Errorf("change[%d]: %+v; want %+v", i, got[i], want[i])
		}
	}
}

func TestTask15ShapeCountsAndUnpairedArrays(t *testing.T) {
	old, candidate := make([]lesson.Node, 12), make([]lesson.Node, 23)
	for i := range old {
		old[i] = task15Node(t, "old", task15Node(t, "child"))
	}
	for i := range candidate {
		candidate[i] = task15Node(t, "new")
	}
	old, candidate = task15Forest(t, old...), task15Forest(t, candidate...)
	for _, reverse := range []bool{false, true} {
		left, right, before, after := old, candidate, 12, 23
		if reverse {
			left, right, before, after = right, left, after, before
		}
		got := compareNodeShape(left, right, 17, nil)
		if len(got) != 1 {
			t.Fatalf("unpaired roots must yield only one reason: %+v", got)
		}
		task15Reason(t, got[0], NodeCountChanged, "/document/pages/17/nodes", before, after)
	}
	left := task15Forest(t, task15Node(t, "a", task15Node(t, "x", task15Node(t, "deeper")), task15Node(t, "y")), task15Node(t, "b"))
	right := task15Forest(t, task15Node(t, "a"), task15Node(t, "b", task15Node(t, "x"), task15Node(t, "y"), task15Node(t, "z")))
	parent := make([]int, 2, 16)
	copy(parent, []int{4, 2})
	got := compareNodeShape(left, right, 9, parent)
	if len(got) != 2 {
		t.Fatalf("must skip unpaired children and continue next branch: %+v", got)
	}
	task15Reason(t, got[0], NodeNestingChanged, "/document/pages/9/nodes/4/nodes/2/nodes/0/nodes", 2, 0)
	task15Reason(t, got[1], NodeNestingChanged, "/document/pages/9/nodes/4/nodes/2/nodes/1/nodes", 0, 3)
	if !slices.Equal(parent, []int{4, 2}) {
		t.Fatal("shape mutated parent path")
	}
}

func TestTask15OwnFieldsAndShapeIndependence(t *testing.T) {
	left := task15Node(t, "before", task15Node(t, "unchanged"))
	left.Type = lesson.NodeTask
	no := false
	left.Flags.Speaking = &no
	right := task15Node(t, "after", task15Node(t, "changed"))
	right.Type = lesson.NodeMultifill
	yes := true
	right.Flags.Checkable = &yes
	right.Data = append(right.Data, right.Data[0])
	old, candidate := task15Forest(t, left), task15Forest(t, right)
	if got := compareNodeShape(old, candidate, 0, nil); len(got) != 0 {
		t.Fatalf("type/flags/data length are not shape: %+v", got)
	}
	want := []NodeField{NodeTypeChanged, NodeFlagsChanged, NodeDataChanged}
	if got := compareNodeOwnFields(old[0], candidate[0]); !slices.Equal(got, want) {
		t.Fatalf("own fields order: %v; want %v", got, want)
	}
	childOnly := task15Node(t, "before", task15Node(t, "changed"))
	childOnly.Type, childOnly.Flags = left.Type, left.Flags
	childOnly = task15Forest(t, childOnly)[0]
	childOnly.ID = "qa-n999-v02"
	if got := compareNodeOwnFields(old[0], childOnly); len(got) != 0 {
		t.Fatalf("ID/children are not own fields: %v", got)
	}
	for _, field := range []NodeField{NodeTypeChanged, NodeFlagsChanged, NodeDataChanged} {
		a, b := task15Node(t, "body"), task15Node(t, "body")
		switch field {
		case NodeTypeChanged:
			b.Type = lesson.NodeNote
		case NodeFlagsChanged:
			a.Type, b.Type = lesson.NodeMultifill, lesson.NodeMultifill
			b.Flags.Checkable = &no
		case NodeDataChanged:
			b.Data[0].Type, b.Data[0].Value = lesson.DataCollection, []lesson.Data{}
		}
		a, b = task15Forest(t, a)[0], task15Forest(t, b)[0]
		if got := compareNodeOwnFields(a, b); !slices.Equal(got, []NodeField{field}) {
			t.Errorf("single own change %s: %v", field, got)
		}
	}
}

func TestTask15ChangesPreorderAndPositionalSwap(t *testing.T) {
	old := task15Forest(t, task15Node(t, "root", task15Node(t, "child", task15Node(t, "leaf")), task15Node(t, "sibling")), task15Node(t, "last"))
	new := task15Forest(t, task15Node(t, "ROOT", task15Node(t, "CHILD", task15Node(t, "LEAF")), task15Node(t, "SIBLING")), task15Node(t, "LAST"))
	before, err := json.Marshal([][]lesson.Node{old, new})
	if err != nil {
		t.Fatal(err)
	}
	prefix := make([]int, 1, 16)
	prefix[0] = 7
	got := compareNodeChanges(old, new, prefix)
	want := []NodeChange{{[]int{7, 0}, []NodeField{NodeDataChanged}}, {[]int{7, 0, 0}, []NodeField{NodeDataChanged}}, {[]int{7, 0, 0, 0}, []NodeField{NodeDataChanged}}, {[]int{7, 0, 1}, []NodeField{NodeDataChanged}}, {[]int{7, 1}, []NodeField{NodeDataChanged}}}
	task15Changes(t, got, want)
	after, err := json.Marshal([][]lesson.Node{old, new})
	if err != nil || string(after) != string(before) {
		t.Fatal("comparison mutated input forests", err)
	}
	got[0].Path[0], got[0].Fields[0] = 99, NodeTypeChanged
	if prefix[0] != 7 || got[1].Path[0] != 7 || got[1].Fields[0] != NodeDataChanged {
		t.Fatal("result paths/fields share mutable storage")
	}
	prefix[0] = 8
	if got[2].Path[0] != 7 {
		t.Fatal("result path aliases caller parentPath")
	}
	parent := task15Forest(t, task15Node(t, "root", task15Node(t, "child")))
	changed := task15Forest(t, task15Node(t, "root", task15Node(t, "CHILD")))
	task15Changes(t, compareNodeChanges(parent, changed, nil), []NodeChange{{[]int{0, 0}, []NodeField{NodeDataChanged}}})
	a, b := task15Node(t, "A"), task15Node(t, "B")
	left, right := task15Forest(t, a, b), task15Forest(t, b, a)
	right[0].ID, right[1].ID = left[1].ID, left[0].ID
	task15Changes(t, compareNodeChanges(left, right, nil), []NodeChange{{[]int{0}, []NodeField{NodeDataChanged}}, {[]int{1}, []NodeField{NodeDataChanged}}})
}

func TestTask15EqualFlagsPresenceAndValues(t *testing.T) {
	for _, field := range []string{"speaking", "stepped", "checkable"} {
		t.Run(field, func(t *testing.T) {
			flags := func(value *bool) lesson.NodeFlags {
				result := lesson.NodeFlags{}
				switch field {
				case "speaking":
					result.Speaking = value
				case "stepped":
					result.Stepped = value
				case "checkable":
					result.Checkable = value
				}
				return result
			}
			no, otherNo, yes, otherYes := false, false, true, true
			for _, pair := range [][2]lesson.NodeFlags{{flags(nil), flags(&no)}, {flags(&no), flags(&yes)}, {flags(&yes), flags(nil)}} {
				if equalFlags(pair[0], pair[1]) || equalFlags(pair[1], pair[0]) {
					t.Fatal("presence/value difference ignored")
				}
			}
			if !equalFlags(flags(&no), flags(&otherNo)) || !equalFlags(flags(&yes), flags(&otherYes)) || !equalFlags(flags(nil), flags(nil)) {
				t.Fatal("equal values at distinct addresses differ")
			}
		})
	}
}

func TestTask15EqualTreesWholeForest(t *testing.T) {
	for _, scenario := range []string{"equal", "id", "root order", "root count", "child count", "child order", "child data", "type", "flags", "flag values"} {
		t.Run(scenario, func(t *testing.T) {
			left := task15Forest(t, task15Node(t, "A", task15Node(t, "child"), task15Node(t, "second")), task15Node(t, "B"))
			right := task15Forest(t, task15Node(t, "A", task15Node(t, "child"), task15Node(t, "second")), task15Node(t, "B"))
			switch scenario {
			case "id":
				right[0].ID, right[0].Nodes[0].ID, right[1].ID = "qa-n991-v02", "qa-n992-v03", "qa-n993"
			case "root order":
				right[0], right[1] = right[1], right[0]
			case "root count":
				right = right[:1]
			case "child count":
				right[0].Nodes = []lesson.Node{}
			case "child order":
				right[0].Nodes[0], right[0].Nodes[1] = right[0].Nodes[1], right[0].Nodes[0]
			case "child data":
				right[0].Nodes[0].Data[0].Value = "CHILD"
			case "type":
				right[1].Type = lesson.NodeNote
			case "flags", "flag values":
				left[1].Type, right[1].Type = lesson.NodeTask, lesson.NodeTask
				no, otherNo := false, false
				right[1].Flags.Speaking = &no
				if scenario == "flag values" {
					left[1].Flags.Speaking = &otherNo
				}
			}
			want := scenario == "equal" || scenario == "id" || scenario == "flag values"
			if equalNodeTrees(left, right) != want || equalNodeTrees(right, left) != want {
				t.Fatalf("whole forest equality: want %v", want)
			}
		})
	}
	if !equalNodeTrees([]lesson.Node{}, []lesson.Node{}) {
		t.Fatal("empty forests differ")
	}
}

func TestTask15DeepAndWideForests(t *testing.T) {
	old, new := task15Node(t, "old"), task15Node(t, "new")
	const depth = 1025
	for i := 1; i < depth; i++ {
		old, new = task15Node(t, "parent", old), task15Node(t, "parent", new)
	}
	left, right := task15Forest(t, old), task15Forest(t, new)
	if reasons := compareNodeShape(left, right, 0, nil); len(reasons) != 0 || equalNodeTrees(left, right) {
		t.Fatal("deep equal shape and changed leaf not distinguished")
	}
	task15Changes(t, compareNodeChanges(left, right, nil), []NodeChange{{make([]int, depth), []NodeField{NodeDataChanged}}})
	leaf := &right[0]
	for len(leaf.Nodes) > 0 {
		leaf = &leaf.Nodes[0]
	}
	leaf.Nodes = []lesson.Node{task15Node(t, "extra")}
	right = task15Forest(t, right...)
	deepReasons := compareNodeShape(left, right, 11, nil)
	if len(deepReasons) != 1 {
		t.Fatalf("deep structural difference lost: %+v", deepReasons)
	}
	task15Reason(t, deepReasons[0], NodeNestingChanged, "/document/pages/11"+strings.Repeat("/nodes/0", depth)+"/nodes", 0, 1)
	wideOld, wideNew := make([]lesson.Node, depth), make([]lesson.Node, depth)
	for i := range wideOld {
		wideOld[i], wideNew[i] = task15Node(t, "old"), task15Node(t, "new")
	}
	changes := compareNodeChanges(task15Forest(t, wideOld...), task15Forest(t, wideNew...), nil)
	if len(changes) != depth || !slices.Equal(changes[depth-1].Path, []int{depth - 1}) {
		t.Fatal("wide forest comparison stopped early")
	}
}
