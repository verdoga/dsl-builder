package console

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// previewTestErrorNodes обходит текущий узел и обе стандартные формы Unwrap.
func previewTestErrorNodes(err error) []error {
	if err == nil {
		return nil
	}
	nodes := []error{err}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			nodes = append(nodes, previewTestErrorNodes(child)...)
		}
	case interface{ Unwrap() error }:
		nodes = append(nodes, previewTestErrorNodes(wrapped.Unwrap())...)
	}
	return nodes
}

// previewTestLeafKey сравнивает типы и полные причины независимо созданных
// owner errors; текст родителя не заменяет ни одну из дочерних причин.
func previewTestLeafKey(err error) string {
	var leaves []string
	for _, node := range previewTestErrorNodes(err) {
		switch wrapped := node.(type) {
		case interface{ Unwrap() []error }:
			if len(wrapped.Unwrap()) != 0 {
				continue
			}
		case interface{ Unwrap() error }:
			if wrapped.Unwrap() != nil {
				continue
			}
		}
		leaves = append(leaves, fmt.Sprintf("%T: %s", node, node.Error()))
	}
	sort.Strings(leaves)
	key, _ := json.Marshal(leaves)
	return string(key)
}

// previewTestPresentationCauses требует отдельную обёртку для каждой ошибки
// блока и сохраняет кратность всех листьев под каждой обёрткой CodePreview.
func previewTestPresentationCauses(t *testing.T, err error, expected ...error) {
	t.Helper()
	want := make(map[string]int)
	for _, cause := range expected {
		if cause == nil {
			t.Fatal("fixture owner must return a presentation error")
		}
		want[previewTestLeafKey(cause)]++
	}
	count := 0
	for _, node := range previewTestErrorNodes(err) {
		issue, ok := node.(*diagnostic.Error)
		if !ok || issue == nil || issue.Code != CodePreview {
			continue
		}
		count++
		if issue.Cause == nil || issue.Reason == "" {
			t.Errorf("presentation wrapper lost reason/cause: %+v", issue)
		}
		key := previewTestLeafKey(issue.Cause)
		if want[key] == 0 {
			t.Errorf("unexpected or repeated presentation cause: %s", key)
		} else {
			want[key]--
		}
	}
	if count != len(expected) {
		t.Errorf("CodePreview wrappers = %d; want %d", count, len(expected))
	}
	for key, missing := range want {
		if missing != 0 {
			t.Errorf("presentation cause missing %d times: %s", missing, key)
		}
	}
}
