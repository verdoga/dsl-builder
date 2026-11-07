package preview

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
)

func replacementsTestNode(id string, children ...lesson.Node) lesson.Node {
	if children == nil {
		children = []lesson.Node{}
	}
	role := lesson.RoleBody
	return lesson.Node{ID: id, Type: lesson.NodeText, Nodes: children, Data: []lesson.Data{{
		Type: lesson.DataCollection, Role: &role, Answers: []string{}, Value: []lesson.Data{
			{Type: lesson.DataText, Value: "PRIVATE_DATA_秘密", Answers: []string{"PRIVATE_ANSWER"}, Hint: "PRIVATE_HINT"},
			{Type: lesson.DataMedia, Value: lesson.EmbeddedFile{FileName: "PRIVATE_MEDIA.mp3", MIMEType: "audio/mpeg", Content: "UFJJVkFURV9CWVRFUw=="}, Answers: []string{}},
		},
	}}}
}

func replacementsTestPage(id string, nodes ...lesson.Node) lesson.Page {
	if nodes == nil {
		nodes = []lesson.Node{}
	}
	title := " Страница 雪\r\n\t\x1b[31m\x00 конец "
	return lesson.Page{ID: id, Title: &title, Nodes: nodes}
}

func replacementsTestFile(t *testing.T, loaded bool, pages ...lesson.Page) *module.File {
	t.Helper()
	if pages == nil {
		pages = []lesson.Page{}
	}
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EFX7", Version: "123456789012345678901", Title: "Урок 雪", LineCount: "7",
		CreatedAt: "2026-11-07T12:00:00Z", Pages: pages,
	}}
	context := diagnostic.Context{Path: "C:\\Уроки 雪\\источник.json"}
	var file *module.File
	var problems []*diagnostic.Error
	if loaded {
		raw := replacementsTestJSON(t, value)
		raw = bytes.Replace(raw, []byte(`{"formatVersion"`), []byte(`{"originalOnly":"PRIVATE_ORIGINAL_BYTES","formatVersion"`), 1)
		if errors := lesson.ValidateJSON(raw, context); len(errors) != 0 {
			t.Fatalf("invalid loaded fixture: %v", errors)
		}
		if err := json.Unmarshal(raw, value); err != nil {
			t.Fatal(err)
		}
		file, problems = module.LoadedFile("l1001.json", value, raw, context)
	} else {
		file, problems = module.NewFile("l1001.json", value, context)
	}
	if len(problems) != 0 || file == nil {
		t.Fatalf("fixture failed: %v", problems)
	}
	return file
}

func replacementsTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func replacementsTestUnchanged(t *testing.T, file *module.File) func() {
	t.Helper()
	model, raw, context, name := replacementsTestJSON(t, file.Lesson()), file.OriginalBytes(), file.Context(), file.FileName()
	return func() {
		t.Helper()
		if !bytes.Equal(model, replacementsTestJSON(t, file.Lesson())) || !bytes.Equal(raw, file.OriginalBytes()) || context != file.Context() || name != file.FileName() {
			t.Error("replacement changed an input file")
		}
	}
}

func replacementsTestSafe(t *testing.T, text string) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Error("invalid UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			t.Errorf("raw control U+%04X", r)
		}
	}
	for _, secret := range []string{"PRIVATE_DATA", "PRIVATE_ANSWER", "PRIVATE_HINT", "PRIVATE_MEDIA", "UFJJVkFURV9CWVRFUw==", "PRIVATE_ORIGINAL_BYTES"} {
		if strings.Contains(text, secret) {
			t.Errorf("leaked %q", secret)
		}
	}
}

func replacementsTestOrder(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		index := strings.Index(text, value)
		if index < 0 {
			t.Errorf("missing ordered value %q in %q", value, text)
			return
		}
		text = text[index+len(value):]
	}
}

func replacementsTestError(t *testing.T, text string, err error) {
	t.Helper()
	if err == nil || text != "" {
		t.Fatalf("want empty text and error, got %q, %v", text, err)
	}
	replacementsTestSafe(t, err.Error())
	if !strings.ContainsFunc(err.Error(), func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("error lacks Russian explanation: %v", err)
	}
}

func TestNodeReplacementExactNestedPair(t *testing.T) {
	oldID, newID := "efx7-n999999999999999999999-v12", "efx7-n999999999999999999999-v123456789012345678901"
	oldNode, newNode := replacementsTestNode(oldID), replacementsTestNode(newID)
	oldFlag, newFlag := false, true
	oldNode.Type, oldNode.Flags.Speaking = lesson.NodeTask, &oldFlag
	newNode.Type, newNode.Flags.Checkable = lesson.NodeQuestion, &newFlag
	before := replacementsTestFile(t, true, replacementsTestPage("efx7-p001"), replacementsTestPage("efx7-p9001",
		replacementsTestNode("efx7-n077", replacementsTestNode("efx7-n088"), oldNode), replacementsTestNode("efx7-n042")))
	after := replacementsTestFile(t, false, replacementsTestPage("efx7-p001"), replacementsTestPage("efx7-p9001",
		replacementsTestNode("efx7-n077", replacementsTestNode("efx7-n088"), newNode), replacementsTestNode("efx7-n042")))
	defer replacementsTestUnchanged(t, before)()
	defer replacementsTestUnchanged(t, after)()
	change := compare.NodeChange{Path: []int{0, 1}, Fields: []compare.NodeField{compare.NodeTypeChanged, compare.NodeFlagsChanged, compare.NodeDataChanged}}
	original := replacementsTestJSON(t, change)
	text, err := nodeReplacement(before, after, 1, change)
	if err != nil {
		t.Fatal(err)
	}
	replacementsTestOrder(t, text, "efx7-p9001", oldID, newID)
	for _, field := range []string{"тип", "флаг", "данн"} {
		if !strings.Contains(strings.ToLower(text), field) {
			t.Errorf("missing Russian field name %q: %s", field, text)
		}
	}
	for _, unrelated := range []string{"efx7-p001", "efx7-n077", "efx7-n088", "efx7-n042", "task", "question", "speaking", "checkable", "true", "false"} {
		if strings.Contains(text, unrelated) {
			t.Errorf("unrelated ID %q included", unrelated)
		}
	}
	if !bytes.Equal(original, replacementsTestJSON(t, change)) {
		t.Error("changed NodeChange")
	}
	replacementsTestSafe(t, text)
}

func TestPageReplacementIndependentDFSLists(t *testing.T) {
	oldIDs := []string{"efx7-n090-v03", "efx7-n004", "efx7-n7007", "efx7-n032"}
	newIDs := []string{"efx7-n999999999999999999999-v100", "efx7-n8001-v100"}
	oldTree := []lesson.Node{replacementsTestNode(oldIDs[0], replacementsTestNode(oldIDs[1], replacementsTestNode(oldIDs[2]))), replacementsTestNode(oldIDs[3])}
	newTree := []lesson.Node{replacementsTestNode(newIDs[0], replacementsTestNode(newIDs[1]))}
	for _, tc := range []struct {
		name             string
		old, next        []lesson.Node
		wantOld, wantNew []string
	}{
		{"different lengths", oldTree, newTree, oldIDs, newIDs},
		{"empty old", []lesson.Node{}, newTree, []string{}, newIDs},
		{"empty new", oldTree, []lesson.Node{}, oldIDs, []string{}},
		{"both empty", []lesson.Node{}, []lesson.Node{}, []string{}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := replacementsTestFile(t, true, replacementsTestPage("efx7-p1009", tc.old...))
			after := replacementsTestFile(t, false, replacementsTestPage("efx7-p1009", tc.next...))
			defer replacementsTestUnchanged(t, before)()
			defer replacementsTestUnchanged(t, after)()
			change := compare.PageChange{Index: 0, ReplaceNodes: true, Reasons: []compare.Reason{
				{Code: compare.NodeCountChanged, Description: "Первое: число нод 4 и 2", OldPointer: "/document/pages/0/nodes"},
				{Code: compare.NodeNestingChanged, Description: "Второе: изменена вложенность", NewPointer: "/document/pages/0/nodes/1/nodes"},
			}}
			original := replacementsTestJSON(t, change)
			text, err := pageReplacement(before, after, change)
			if err != nil {
				t.Fatal(err)
			}
			replacementsTestOrder(t, text, "efx7-p1009", "Страница 雪", "Первое:", "Второе:", "Будут удалены", "Будут созданы")
			replacementsTestOrder(t, text, change.Reasons[0].OldPointer, change.Reasons[1].NewPointer)
			lower := strings.ToLower(text)
			if !strings.Contains(lower, "id страниц") || !strings.Contains(lower, "сохран") {
				t.Error("missing page-ID preservation")
			}
			removed, created, found := strings.Cut(text, "Будут созданы")
			if !found {
				t.Fatal("missing new-node list")
			}
			re := regexp.MustCompile(`efx7-n[0-9]+(?:-v[0-9]+)?`)
			if got := re.FindAllString(removed, -1); !slices.Equal(got, tc.wantOld) {
				t.Errorf("old DFS IDs = %v, want %v", got, tc.wantOld)
			}
			if got := re.FindAllString(created, -1); !slices.Equal(got, tc.wantNew) {
				t.Errorf("new DFS IDs = %v, want %v", got, tc.wantNew)
			}
			if len(tc.wantOld) == 0 && !strings.Contains(removed, "нет") {
				t.Error("empty old list is not explicit")
			}
			if len(tc.wantNew) == 0 && !strings.Contains(created, "нет") {
				t.Error("empty new list is not explicit")
			}
			if strings.ContainsAny(removed+created, "→") || strings.Contains(removed+created, "->") {
				t.Error("full replacement invents node pairs")
			}
			if !bytes.Equal(original, replacementsTestJSON(t, change)) {
				t.Error("changed PageChange")
			}
			replacementsTestSafe(t, text)
		})
	}
}

func TestReplacementMissingSnapshotsNodeAddressesAndPageSelectors(t *testing.T) {
	valid := replacementsTestFile(t, true, replacementsTestPage("efx7-p001", replacementsTestNode("efx7-n001")))
	defer replacementsTestUnchanged(t, valid)()
	empty := replacementsTestFile(t, false)
	for _, bad := range []*module.File{nil, {}} {
		text, err := nodeReplacement(bad, valid, 0, compare.NodeChange{Path: []int{0}})
		replacementsTestError(t, text, err)
		text, err = nodeReplacement(valid, bad, 0, compare.NodeChange{Path: []int{0}})
		replacementsTestError(t, text, err)
		text, err = pageReplacement(bad, valid, compare.PageChange{Index: 0})
		replacementsTestError(t, text, err)
		text, err = pageReplacement(valid, bad, compare.PageChange{Index: 0})
		replacementsTestError(t, text, err)
	}
	for _, index := range []int{-1, 1} {
		text, err := pageReplacement(valid, valid, compare.PageChange{Index: index})
		replacementsTestError(t, text, err)
		text, err = nodeReplacement(valid, valid, index, compare.NodeChange{Path: []int{0}})
		replacementsTestError(t, text, err)
	}
	for _, path := range [][]int{nil, {}, {-1}, {1}, {0, 0}} {
		text, err := nodeReplacement(valid, valid, 0, compare.NodeChange{Path: path})
		replacementsTestError(t, text, err)
	}
	text, err := nodeReplacement(valid, empty, 0, compare.NodeChange{Path: []int{0}})
	replacementsTestError(t, text, err)
	text, err = pageReplacement(empty, valid, compare.PageChange{Index: 0})
	replacementsTestError(t, text, err)
	text, err = pageReplacement(valid, empty, compare.PageChange{Index: 0})
	replacementsTestError(t, text, err)
	nested := replacementsTestFile(t, false, replacementsTestPage("efx7-p001", replacementsTestNode("efx7-n001", replacementsTestNode("efx7-n002"))))
	text, err = nodeReplacement(nested, valid, 0, compare.NodeChange{Path: []int{0, 0}})
	replacementsTestError(t, text, err)
}

func TestNodeReplacementOnlyPreparedFieldNames(t *testing.T) {
	before := replacementsTestFile(t, true, replacementsTestPage("efx7-p001", replacementsTestNode("efx7-n001")))
	after := replacementsTestFile(t, false, replacementsTestPage("efx7-p001", replacementsTestNode("efx7-n001-v99")))
	for _, tc := range []struct {
		field   compare.NodeField
		present string
		absent  []string
	}{
		{compare.NodeTypeChanged, "тип", []string{"флаг", "данн"}},
		{compare.NodeFlagsChanged, "флаг", []string{"тип", "данн"}},
		{compare.NodeDataChanged, "данн", []string{"тип", "флаг"}},
	} {
		text, err := nodeReplacement(before, after, 0, compare.NodeChange{Path: []int{0}, Fields: []compare.NodeField{tc.field}})
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(text)
		if !strings.Contains(lower, tc.present) {
			t.Errorf("missing prepared field %s: %s", tc.field, text)
		}
		for _, absent := range tc.absent {
			if strings.Contains(lower, absent) {
				t.Errorf("added unprepared field %s: %s", absent, text)
			}
		}
		replacementsTestSafe(t, text)
	}
}
