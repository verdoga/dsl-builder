package compare

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func task18Pages(t *testing.T, texts ...string) []lesson.Page {
	t.Helper()
	pages := make([]lesson.Page, len(texts))
	for i, text := range texts {
		pages[i] = lesson.Page{ID: fmt.Sprintf("qa-p%03d", i+1), Nodes: []lesson.Node{}}
		if text != "" {
			role := lesson.RoleBody
			pages[i].Nodes = []lesson.Node{{ID: fmt.Sprintf("qa-n%03d", i+1), Type: lesson.NodeText, Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: text, Answers: []string{}}}, Nodes: []lesson.Node{}}}
		}
	}
	task18Valid(t, pages)
	return pages
}

func task18Valid(t *testing.T, pages []lesson.Page) {
	t.Helper()
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{ID: "QA", Version: "1", LineCount: "0", CreatedAt: "2026-11-06T00:00:00Z", Pages: pages}}
	if errors := lesson.Validate(value, diagnostic.Context{}); len(errors) != 0 {
		t.Fatalf("invalid fixture: %v", errors)
	}
}

func task18MatchLists(t *testing.T, got, want [][]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("match lengths %v, want %v", got, want)
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Errorf("matches[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func task18Reasons(t *testing.T, reasons []Reason, code ReasonCode) {
	t.Helper()
	if len(reasons) == 0 {
		t.Fatalf("missing reason %q", code)
	}
	for _, reason := range reasons {
		if reason.Code != code || !strings.ContainsFunc(reason.Description, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) || !strings.ContainsFunc(reason.Description, unicode.IsDigit) {
			t.Errorf("reason must have stable code and concrete Russian description: %+v", reason)
		}
		for _, pointer := range []string{reason.OldPointer, reason.NewPointer} {
			if reason.OldPointer == "" && reason.NewPointer == "" {
				t.Error("reason has no address")
			}
			if pointer != "" && pointer != "/document/pages" && !strings.HasPrefix(pointer, "/document/pages/") {
				t.Errorf("unexpected RFC6901 pointer %q", pointer)
			}
		}
	}
}

func TestTask18FindAllPageMatches(t *testing.T) {
	old, next := task18Pages(t, "A", "A", "", "B", ""), task18Pages(t, "", "A", "B", "A", "", "C")
	title := "Переименовано"
	next[1].Title, next[1].ID, next[1].Nodes[0].ID = &title, "qa-p999", "qa-n999-v02"
	task18Valid(t, next)
	matches := findPageMatches(old, next)
	task18MatchLists(t, matches.OldForNew, [][]int{{2, 4}, {0, 1}, {3}, {0, 1}, {2, 4}, {}})
	task18MatchLists(t, matches.NewForOld, [][]int{{1, 3}, {1, 3}, {0, 4}, {2}, {0, 4}})
	matches.OldForNew[1][0] = 99
	if matches.OldForNew[3][0] != 0 || matches.NewForOld[0][0] != 1 || findPageMatches(old, next).OldForNew[1][0] != 0 {
		t.Fatal("match lists share writable storage")
	}
	for _, pair := range [][2][]lesson.Page{{task18Pages(t), task18Pages(t)}, {task18Pages(t), task18Pages(t, "A")}, {task18Pages(t, "A"), task18Pages(t)}} {
		got := findPageMatches(pair[0], pair[1])
		if len(got.OldForNew) != len(pair[1]) || len(got.NewForOld) != len(pair[0]) {
			t.Fatal("empty-side dimensions lost")
		}
	}
}

func TestTask18PageMatchingUsesExactForests(t *testing.T) {
	for _, mode := range []string{"root order", "data", "data order", "type", "flags", "nesting"} {
		t.Run(mode, func(t *testing.T) {
			old := task18Pages(t, "A")
			old[0].Nodes = append(old[0].Nodes, task18Pages(t, "B")[0].Nodes[0])
			old[0].Nodes[1].ID = "qa-n002"
			next := task18Pages(t, "A")
			next[0].Nodes = append(next[0].Nodes, task18Pages(t, "B")[0].Nodes[0])
			next[0].Nodes[1].ID = "qa-n002"
			switch mode {
			case "root order":
				slices.Reverse(next[0].Nodes)
			case "data":
				next[0].Nodes[1].Data[0].Value = " B"
			case "data order":
				old[0].Nodes[0].Data = append(old[0].Nodes[0].Data, old[0].Nodes[1].Data[0])
				next[0].Nodes[0].Data = []lesson.Data{old[0].Nodes[0].Data[1], old[0].Nodes[0].Data[0]}
			case "type":
				next[0].Nodes[0].Type = lesson.NodeNote
			case "flags":
				old[0].Nodes[0].Type, next[0].Nodes[0].Type = lesson.NodeTask, lesson.NodeTask
				flag := false
				next[0].Nodes[0].Flags.Speaking = &flag
			case "nesting":
				next[0].Nodes[0].Nodes = []lesson.Node{next[0].Nodes[1]}
				next[0].Nodes = next[0].Nodes[:1]
			}
			task18Valid(t, old)
			task18Valid(t, next)
			got := findPageMatches(old, next)
			if len(got.OldForNew[0]) != 0 || len(got.NewForOld[0]) != 0 {
				t.Fatalf("inexact forest matched: %+v", got)
			}
		})
	}
}

func TestTask18SequencePriorityAndEvidence(t *testing.T) {
	cases := []struct {
		name      string
		old, next []string
		code      ReasonCode
	}{
		{"empty", []string{}, []string{}, ""},
		{"duplicate positional", []string{"A", "A", ""}, []string{"A", "A", ""}, ""},
		{"unchanged duplicates beside edit", []string{"A", "A", "B"}, []string{"A", "A", "X"}, ""},
		{"all edited", []string{"A", "B", "C"}, []string{"X", "Y", "Z"}, ""},
		{"count before moved", []string{"A", "B"}, []string{"B", "A", "A"}, PageCountChanged},
		{"count before ambiguous", []string{"A", "A"}, []string{"B", "A", "A"}, PageCountChanged},
		{"mutual unique move", []string{"A", "B", "C"}, []string{"C", "B", "A"}, PageOrderChanged},
		{"move before ambiguity", []string{"A", "A", "B", "C"}, []string{"B", "A", "A", "X"}, PageOrderChanged},
		{"one sided unique is ambiguous", []string{"A", "A", "B"}, []string{"B edited", "X", "A"}, PageCorrespondenceAmbiguous},
		{"reverse duplicate ambiguity", []string{"B", "X", "A"}, []string{"A", "A", "B edited"}, PageCorrespondenceAmbiguous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old, next := task18Pages(t, tc.old...), task18Pages(t, tc.next...)
			title := "Новое название"
			for i := range next {
				next[i].Title = &title
			}
			matches, reasons := comparePageSequence(old, next)
			task18MatchLists(t, matches.OldForNew, findPageMatches(old, next).OldForNew)
			task18MatchLists(t, matches.NewForOld, findPageMatches(old, next).NewForOld)
			if tc.code == "" {
				if len(reasons) != 0 {
					t.Fatalf("unsupported replacement: %+v", reasons)
				}
			} else {
				task18Reasons(t, reasons, tc.code)
			}
			if tc.code == PageCountChanged {
				if len(reasons) != 1 || reasons[0].OldPointer != "/document/pages" || reasons[0].NewPointer != "/document/pages" {
					t.Fatalf("count reason: %+v", reasons)
				}
				for _, n := range []int{len(old), len(next)} {
					if !strings.Contains(reasons[0].Description, fmt.Sprint(n)) {
						t.Error("count missing from description")
					}
				}
			}
			baseline, _ := json.Marshal(reasons)
			for range 8 {
				again := classifyPageSequence(old, next, matches)
				encoded, _ := json.Marshal(again)
				if string(encoded) != string(baseline) {
					t.Fatal("nondeterministic reasons")
				}
			}
			if len(old) == len(next) && tc.code != PageCountChanged && pageCountReason(old, next) != nil {
				t.Error("false count reason")
			}
		})
	}
}

func TestTask18SequenceHelpersObserveMutualUniqueness(t *testing.T) {
	old, next := task18Pages(t, "A", "B", "C"), task18Pages(t, "C", "B", "A")
	matches := findPageMatches(old, next)
	if equalPageOrder(matches) || !equalPageOrder(findPageMatches(task18Pages(t), task18Pages(t))) {
		t.Fatal("wrong positional equality")
	}
	reasons := movedPageReasons(matches)
	task18Reasons(t, reasons, PageOrderChanged)
	if len(reasons) != 2 || reasons[0].OldPointer != "/document/pages/2" || reasons[0].NewPointer != "/document/pages/0" || reasons[1].OldPointer != "/document/pages/0" || reasons[1].NewPointer != "/document/pages/2" {
		t.Fatalf("moved pair order/pointers: %+v", reasons)
	}
	duplicate := findPageMatches(task18Pages(t, "A", "A", "B"), task18Pages(t, "X", "Y", "A"))
	if len(movedPageReasons(duplicate)) != 0 {
		t.Fatal("one-sided uniqueness treated as movement")
	}
	task18Reasons(t, ambiguousPageReasons(duplicate), PageCorrespondenceAmbiguous)
	if len(ambiguousPageReasons(findPageMatches(task18Pages(t, "A", "B"), task18Pages(t, "X", "Y")))) != 0 {
		t.Fatal("no matches treated as ambiguity")
	}
	positional := findPageMatches(task18Pages(t, "A", "A", ""), task18Pages(t, "A", "A", ""))
	if !equalPageOrder(positional) {
		t.Fatal("duplicates hide preserved positions")
	}
	if reason := pageCountReason(old, next[:2]); reason == nil || reason.Code != PageCountChanged {
		t.Fatal("count change omitted")
	}
}

func TestTask18ComparePagesAllChangesAndOwnership(t *testing.T) {
	old, next := task18Pages(t, "A", "B", "C", "D"), task18Pages(t, "X", "B", "Y", "Z")
	title := "Другое название"
	next[1].Title = &title
	beforeOld, _ := json.Marshal(old)
	beforeNext, _ := json.Marshal(next)
	changes := comparePages(old, next)
	if len(changes) != 3 {
		t.Fatalf("changes: %+v", changes)
	}
	for i, index := range []int{0, 2, 3} {
		change := changes[i]
		if change.Index != index || change.ReplaceNodes || len(change.Reasons) != 0 || len(change.Nodes) != 1 || !slices.Equal(change.Nodes[0].Path, []int{0}) || !slices.Equal(change.Nodes[0].Fields, []NodeField{NodeDataChanged}) {
			t.Fatalf("positional change: %+v", change)
		}
	}
	if comparePage(old[1], next[1], 1) != nil {
		t.Fatal("title-only page became node change")
	}
	next[0].Nodes = []lesson.Node{}
	mixed := comparePages(old, next)
	if len(mixed) != 3 || !mixed[0].ReplaceNodes || mixed[1].Index != 2 || mixed[2].Index != 3 {
		t.Fatalf("stopped after structural change: %+v", mixed)
	}
	next[0].Nodes = task18Pages(t, "X")[0].Nodes
	changes[0].Nodes[0].Path[0] = 99
	changes[0].Nodes[0].Fields[0] = NodeTypeChanged
	if changes[1].Nodes[0].Path[0] != 0 || comparePages(old, next)[0].Nodes[0].Path[0] != 0 {
		t.Fatal("changes share writable storage")
	}
	afterOld, _ := json.Marshal(old)
	afterNext, _ := json.Marshal(next)
	if string(beforeOld) != string(afterOld) || string(beforeNext) != string(afterNext) {
		t.Fatal("inputs mutated")
	}
}

func TestTask18ComparePageShapeAndOwnFields(t *testing.T) {
	old, next := task18Pages(t, "A"), task18Pages(t, "B")
	next[0].Nodes[0].Type = lesson.NodeNote
	change := comparePage(old[0], next[0], 0)
	if change == nil || change.Index != 0 || change.ReplaceNodes || len(change.Reasons) != 0 || len(change.Nodes) != 1 || !slices.Equal(change.Nodes[0].Fields, []NodeField{NodeTypeChanged, NodeDataChanged}) {
		t.Fatalf("same shape: %+v", change)
	}
	for _, nested := range []bool{false, true} {
		candidate := task18Pages(t, "B")[0]
		child := task18Pages(t, "C")[0].Nodes[0]
		child.ID = "qa-n002"
		code, pointer := NodeCountChanged, "/document/pages/0/nodes"
		if nested {
			candidate.Nodes[0].Nodes = []lesson.Node{child}
			code, pointer = NodeNestingChanged, pointer+"/0/nodes"
		} else {
			candidate.Nodes = append(candidate.Nodes, child)
		}
		task18Valid(t, []lesson.Page{candidate})
		changed := comparePage(old[0], candidate, 0)
		if changed == nil || !changed.ReplaceNodes || len(changed.Nodes) != 0 || len(changed.Reasons) != 1 {
			t.Fatalf("shape replacement: %+v", changed)
		}
		task18Reasons(t, changed.Reasons, code)
		if changed.Reasons[0].OldPointer != pointer || changed.Reasons[0].NewPointer != pointer {
			t.Fatalf("shape address: %+v", changed.Reasons)
		}
	}
}
