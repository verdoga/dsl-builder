package preview

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/module"
)

func TestLessonReplacementListsOwnPagesAndNodes(t *testing.T) {
	oldPageA, oldPageB := "efx7-p909", "efx7-p003"
	newPageB, newPageA, newPageC := "efx7-p999999999999999999999", "efx7-p7001", "efx7-p8003"
	oldA, oldChild, oldB := "efx7-n5001-v09", "efx7-n003", "efx7-n7001-v101"
	newB, newA, newChild := "efx7-n999999999999999999999-v9999", "efx7-n8008-v9999", "efx7-n9009-v9999"
	before := replacementsTestFile(t, true,
		replacementsTestPage(oldPageA, replacementsTestNode(oldA, replacementsTestNode(oldChild))),
		replacementsTestPage(oldPageB, replacementsTestNode(oldB)))
	// Page content is reordered; ready after IDs deliberately do not correspond by position.
	after := replacementsTestFile(t, false,
		replacementsTestPage(newPageB, replacementsTestNode(newB)),
		replacementsTestPage(newPageA, replacementsTestNode(newA, replacementsTestNode(newChild))),
		replacementsTestPage(newPageC))
	defer replacementsTestUnchanged(t, before)()
	defer replacementsTestUnchanged(t, after)()
	reasons := []compare.Reason{
		{Code: compare.PageOrderChanged, Description: "Готовая причина: переставлены страницы", OldPointer: "/document/pages/1", NewPointer: "/document/pages/0"},
		{Code: compare.PageCountChanged, Description: "Готовая причина: добавлена страница", NewPointer: "/document/pages/2"},
	}
	original := replacementsTestJSON(t, reasons)
	text, err := lessonReplacement(before, after, reasons)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "полн") || !strings.Contains(lower, "замен") || !strings.Contains(lower, "урок") {
		t.Errorf("missing explicit full-lesson replacement: %s", text)
	}
	for _, identity := range []string{"EFX7", "l1001.json"} {
		index := strings.Index(text, identity)
		if index < 0 {
			t.Errorf("missing preserved identity %s", identity)
			continue
		}
		start, end := strings.LastIndex(text[:index], "\n")+1, strings.Index(text[index:], "\n")
		if end < 0 {
			end = len(text) - index
		}
		if !strings.Contains(strings.ToLower(text[start:index+end]), "сохран") {
			t.Errorf("identity %s lacks explicit preservation: %s", identity, text[start:index+end])
		}
	}
	replacementsTestOrder(t, text, reasons[0].Description, reasons[1].Description)
	replacementsTestOrder(t, text, reasons[0].OldPointer, reasons[0].NewPointer, reasons[1].NewPointer)
	replacementsTestOrder(t, text, oldPageA, oldPageB, newPageB, newPageA, newPageC)
	// Locate node groups by the node IDs themselves; headings and punctuation are free.
	for _, group := range []struct {
		page, first string
		ids         []string
	}{
		{oldPageA, oldA, []string{oldA, oldChild}},
		{oldPageB, oldB, []string{oldB}},
		{newPageB, newB, []string{newB}},
		{newPageA, newA, []string{newA, newChild}},
	} {
		index := strings.Index(text, group.first)
		if index < 0 {
			t.Errorf("missing node %s", group.first)
			continue
		}
		prefix := text[:index]
		pages := regexp.MustCompile(`efx7-p[0-9]+`).FindAllString(prefix, -1)
		if len(pages) == 0 || pages[len(pages)-1] != group.page {
			t.Errorf("nodes %v grouped under wrong page: %v", group.ids, pages)
		}
		replacementsTestOrder(t, text[index:], group.ids...)
	}
	nodes := regexp.MustCompile(`efx7-n[0-9]+(?:-v[0-9]+)?`).FindAllString(text, -1)
	if want := []string{oldA, oldChild, oldB, newB, newA, newChild}; !slices.Equal(nodes, want) {
		t.Errorf("independent DFS node lists = %v, want %v", nodes, want)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "efx7-") && (strings.Contains(line, "->") || strings.Contains(line, "→")) {
			t.Errorf("invented positional ID pair: %s", line)
		}
	}
	if !bytes.Equal(original, replacementsTestJSON(t, reasons)) {
		t.Error("changed ready reasons")
	}
	replacementsTestSafe(t, text)
}

func TestLessonReplacementEmptySides(t *testing.T) {
	empty := replacementsTestFile(t, true)
	nonempty := replacementsTestFile(t, false, replacementsTestPage("efx7-p555"))
	for _, tc := range []struct {
		name          string
		before, after *module.File
	}{
		{"no old pages", empty, nonempty},
		{"no new pages", nonempty, empty},
		{"no pages on either side", empty, empty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, err := lessonReplacement(tc.before, tc.after, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text, "нет") {
				t.Error("empty page/node list is not explicit")
			}
			if tc.before == empty && tc.after == empty && strings.Contains(text, "efx7-p") {
				t.Error("invented pages for empty lesson")
			}
			replacementsTestSafe(t, text)
		})
	}
}

func TestLessonReplacementMissingSnapshots(t *testing.T) {
	valid := replacementsTestFile(t, true)
	for _, bad := range []*module.File{nil, {}} {
		text, err := lessonReplacement(bad, valid, nil)
		replacementsTestError(t, text, err)
		text, err = lessonReplacement(valid, bad, nil)
		replacementsTestError(t, text, err)
	}
}

func TestReplacementReasonsAllPreparedDescriptionsAndPointers(t *testing.T) {
	reasons := []compare.Reason{
		{Code: compare.NodeNestingChanged, Description: "Первая причина: детей было 7, будет 3 雪", OldPointer: "/document/pages/17/nodes/3/nodes", NewPointer: "/document/pages/11/nodes/1/nodes"},
		{Code: compare.PageCorrespondenceAmbiguous, Description: "Вторая причина: соответствие неоднозначно", OldPointer: "/document/pages/4"},
		{Code: compare.PageOrderChanged, Description: "Третья причина: страницы переставлены", NewPointer: "/document/pages/2"},
		{Code: compare.PageCountChanged, Description: "Четвёртая причина: без адресов"},
	}
	original := replacementsTestJSON(t, reasons)
	text := replacementReasons(reasons)
	replacementsTestOrder(t, text, reasons[0].Description, reasons[0].OldPointer, reasons[0].NewPointer,
		reasons[1].Description, reasons[1].OldPointer, reasons[2].Description, reasons[2].NewPointer, reasons[3].Description)
	for _, reason := range reasons {
		if strings.Count(text, reason.Description) != 1 {
			t.Errorf("description omitted or repeated: %s", reason.Description)
		}
		for _, pointer := range []struct {
			value string
			label string
		}{
			{reason.OldPointer, "стар"}, {reason.NewPointer, "кандидат"},
		} {
			if pointer.value == "" {
				continue
			}
			index := strings.Index(text, pointer.value)
			if index < 0 {
				continue
			}
			start := strings.LastIndex(text[:index], "\n") + 1
			prefix := strings.ToLower(text[start:index])
			if !strings.Contains(prefix, pointer.label) {
				t.Errorf("pointer %s lacks explicit %s label: %s", pointer.value, pointer.label, prefix)
			}
			if strings.Contains(prefix, "новый id") || strings.Contains(prefix, "итоговый id") {
				t.Error("candidate pointer presented as final ID")
			}
		}
	}
	for _, invented := range []string{"%", "большие изменения", "слишком много изменений", "efx7-"} {
		if strings.Contains(text, invented) {
			t.Errorf("unprepared explanation or ID: %q", invented)
		}
	}
	if !bytes.Equal(original, replacementsTestJSON(t, reasons)) {
		t.Error("mutated reasons")
	}
	replacementsTestSafe(t, text)
}

func TestReplacementReasonsExternalValuesEscaped(t *testing.T) {
	reasons := []compare.Reason{{Description: "Причина 雪\n\r\t\x1b[2J\x00 конец", OldPointer: "/старый\n\x1b[31m", NewPointer: "/кандидат\r\t\x00"}}
	original := replacementsTestJSON(t, reasons)
	text := replacementReasons(reasons)
	replacementsTestOrder(t, text, "Причина 雪", "конец", "/старый", "/кандидат")
	replacementsTestSafe(t, text)
	if strings.Contains(text, "Причина 雪\n") || strings.Contains(text, "/старый\n") {
		t.Error("external newline became a structural line")
	}
	if !bytes.Equal(original, replacementsTestJSON(t, reasons)) {
		t.Error("mutated external values")
	}
	for _, empty := range [][]compare.Reason{nil, {}} {
		text := replacementReasons(empty)
		replacementsTestSafe(t, text)
		if strings.Contains(text, "%") || strings.Contains(text, "больш") {
			t.Error("invented reason for empty input")
		}
	}
}
