package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
)

func identifiersTestNode(id string, children ...lesson.Node) lesson.Node {
	return lesson.Node{ID: id, Type: lesson.NodeTask, Data: []lesson.Data{}, Nodes: append([]lesson.Node{}, children...)}
}

func identifiersTestLesson(pages ...lesson.Page) *lesson.Lesson {
	return &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EFX7", Version: "100", Title: "Урок 🌍", LineCount: "0",
		CreatedAt: "2026-11-07T10:00:00Z", Pages: append([]lesson.Page{}, pages...),
	}}
}

func identifiersTestJSON(t *testing.T, value *lesson.Lesson) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func identifiersTestFile(t *testing.T, value *lesson.Lesson, loaded bool) *module.File {
	t.Helper()
	context := diagnostic.Context{Path: "Архив или источник 🌍.json"}
	if !loaded {
		file, problems := module.NewFile("l1000.json", value, context)
		if file == nil || len(problems) != 0 {
			t.Fatalf("NewFile: %v", problems)
		}
		return file
	}
	raw := append([]byte(`{"ignoredOriginal":"ORIGINAL_SECRET",`), identifiersTestJSON(t, value)[1:]...)
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		t.Fatalf("ValidateJSON загружаемой фикстуры: %v", problems)
	}
	var decoded lesson.Lesson
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	file, problems := module.LoadedFile("l1000.json", &decoded, raw, context)
	if file == nil || len(problems) != 0 {
		t.Fatalf("LoadedFile: %v", problems)
	}
	return file
}

func identifiersTestError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("недоступный адрес должен возвращать error")
	}
	text := err.Error()
	if !utf8.ValidString(text) || !strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("нужна русская ошибка UTF-8: %q", text)
	}
	for _, secret := range []string{"DATA_SECRET", "ANSWER_SECRET", "HINT_SECRET", "ORIGINAL_SECRET", "U0VDUkVU"} {
		if strings.Contains(text, secret) {
			t.Errorf("в ошибку попали данные: %q", text)
		}
	}
}

func TestIdentifiersExactOrderAndSnapshotOwnership(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		t.Run(fmt.Sprintf("loaded=%t", loaded), func(t *testing.T) {
			root := identifiersTestNode("efx7-n1000-v100",
				identifiersTestNode("efx7-n003-v06", identifiersTestNode("efx7-n035-v03")),
				identifiersTestNode("efx7-n002", identifiersTestNode("efx7-n999999999999999999999-v09")))
			body := lesson.RoleBody
			root.Data = []lesson.Data{
				{Type: lesson.DataText, Role: &body, Value: "DATA_SECRET", Answers: []string{"ANSWER_SECRET"}, Hint: "HINT_SECRET"},
				{Type: lesson.DataMedia, Role: &body, Value: lesson.EmbeddedFile{FileName: "секрет.bin", MIMEType: "application/octet-stream", Content: "U0VDUkVU"}, Answers: []string{}},
			}
			input := identifiersTestLesson(
				lesson.Page{ID: "efx7-p1000", Nodes: []lesson.Node{root, identifiersTestNode("efx7-n001")}},
				lesson.Page{ID: "efx7-p002", Nodes: []lesson.Node{}},
				lesson.Page{ID: "efx7-p999999999999999999999", Nodes: []lesson.Node{identifiersTestNode("efx7-n777-v99")}},
			)
			file := identifiersTestFile(t, input, loaded)
			beforeInput, beforeFile, beforeRaw := identifiersTestJSON(t, input), identifiersTestJSON(t, file.Lesson()), file.OriginalBytes()
			wantPages := []string{"efx7-p1000", "efx7-p002", "efx7-p999999999999999999999"}
			pages, err := pageIDs(file)
			if err != nil || !slices.Equal(pages, wantPages) {
				t.Fatalf("pageIDs = %q, %v; want %q", pages, err, wantPages)
			}
			wantNodes := []string{"efx7-n1000-v100", "efx7-n003-v06", "efx7-n035-v03", "efx7-n002", "efx7-n999999999999999999999-v09", "efx7-n001"}
			nodes, err := nodeIDs(file, 0)
			if err != nil || !slices.Equal(nodes, wantNodes) {
				t.Fatalf("nodeIDs DFS = %q, %v; want %q", nodes, err, wantNodes)
			}
			for _, tc := range []struct {
				page int
				path []int
				want string
			}{
				{0, []int{0}, wantNodes[0]},
				{0, []int{0, 0, 0}, wantNodes[2]},
				{0, []int{0, 1, 0}, wantNodes[4]},
				{0, []int{1}, wantNodes[5]},
				{2, []int{0}, "efx7-n777-v99"},
			} {
				pathBefore := slices.Clone(tc.path)
				got, err := nodeID(file, tc.page, tc.path)
				if err != nil || got != tc.want || !slices.Equal(tc.path, pathBefore) {
					t.Errorf("nodeID(%d,%v) = %q, %v; want %q; path after %v", tc.page, pathBefore, got, err, tc.want, tc.path)
				}
			}
			if empty, err := nodeIDs(file, 1); err != nil || len(empty) != 0 {
				t.Errorf("пустая страница = %q, %v", empty, err)
			}
			missing, err := nodeID(file, 0, []int{0, 0, 7})
			if missing != "" {
				t.Errorf("нельзя подобрать ближайшую ноду вместо отсутствующего ребёнка: %q", missing)
			}
			identifiersTestError(t, err)
			pages[0], nodes[0] = "испорчен результат", "испорчен результат"
			pagesAgain, pageErr := pageIDs(file)
			nodesAgain, nodeErr := nodeIDs(file, 0)
			if pageErr != nil || nodeErr != nil || !slices.Equal(pagesAgain, wantPages) || !slices.Equal(nodesAgain, wantNodes) {
				t.Error("правка возвращённого списка изменила последующее извлечение")
			}
			if !bytes.Equal(identifiersTestJSON(t, input), beforeInput) || !bytes.Equal(identifiersTestJSON(t, file.Lesson()), beforeFile) || !bytes.Equal(file.OriginalBytes(), beforeRaw) {
				t.Error("извлечение ID изменило вход, снимок либо исходные байты")
			}
		})
	}
}

func TestIdentifiersDeepDFSAndExactPath(t *testing.T) {
	const depth = 256
	want := make([]string, depth)
	path := make([]int, depth)
	var chain lesson.Node
	for i := depth - 1; i >= 0; i-- {
		want[i] = fmt.Sprintf("efx7-n%d-v100", 1000+i)
		if i == depth-1 {
			chain = identifiersTestNode(want[i])
		} else {
			chain = identifiersTestNode(want[i], chain)
		}
	}
	file := identifiersTestFile(t, identifiersTestLesson(lesson.Page{ID: "efx7-p1000", Nodes: []lesson.Node{chain}}), false)
	got, err := nodeIDs(file, 0)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("потеря уровней или порядка DFS: got %d IDs, err %v", len(got), err)
	}
	if got, err := nodeID(file, 0, path); err != nil || got != want[depth-1] {
		t.Errorf("глубокий путь = %q, %v; want %q", got, err, want[depth-1])
	}
}

func TestIdentifiersUnavailableSnapshotsAndAddresses(t *testing.T) {
	for _, file := range []*module.File{nil, {}} {
		if got, err := pageIDs(file); err == nil || len(got) != 0 {
			t.Errorf("pageIDs отсутствующего снимка = %q, %v", got, err)
		} else {
			identifiersTestError(t, err)
		}
		if got, err := nodeIDs(file, 0); err == nil || len(got) != 0 {
			t.Errorf("nodeIDs отсутствующего снимка = %q, %v", got, err)
		} else {
			identifiersTestError(t, err)
		}
		got, err := nodeID(file, 0, []int{0})
		if got != "" {
			t.Errorf("текст при error = %q", got)
		}
		identifiersTestError(t, err)
	}
	empty := identifiersTestFile(t, identifiersTestLesson(), false)
	if got, err := pageIDs(empty); err != nil || len(got) != 0 {
		t.Errorf("пустой список страниц = %q, %v", got, err)
	}
	file := identifiersTestFile(t, identifiersTestLesson(
		lesson.Page{ID: "efx7-p001", Nodes: []lesson.Node{identifiersTestNode("efx7-n001", identifiersTestNode("efx7-n002"))}},
		lesson.Page{ID: "efx7-p002", Nodes: []lesson.Node{}},
	), true)
	for _, page := range []int{-1, 2, int(^uint(0) >> 1)} {
		got, err := nodeIDs(file, page)
		if len(got) != 0 {
			t.Errorf("частичный список при недоступной странице: %q", got)
		}
		identifiersTestError(t, err)
	}
	for _, tc := range []struct {
		file *module.File
		page int
		path []int
	}{
		{empty, 0, []int{0}},
		{file, -1, []int{0}},
		{file, 2, []int{0}},
		{file, 0, nil},
		{file, 0, []int{}},
		{file, 0, []int{-1}},
		{file, 0, []int{1}},
		{file, 0, []int{0, -1}},
		{file, 0, []int{0, 1}},
		{file, 0, []int{0, 0, 0}},
		{file, 1, []int{0}},
	} {
		got, err := nodeID(tc.file, tc.page, tc.path)
		if got != "" {
			t.Errorf("nodeID(%d,%v) вернул ближайший/частичный ID %q", tc.page, tc.path, got)
		}
		identifiersTestError(t, err)
	}
	if got, err := nodeIDs(empty, 0); err == nil || len(got) != 0 {
		t.Errorf("страница отсутствует в пустом уроке: %q, %v", got, err)
	}
}

func TestIdentifiersIDListPreservesFullValuesAndOrder(t *testing.T) {
	for _, values := range [][]string{nil, {}} {
		if got := strings.TrimSpace(idList(values)); got != "нет" {
			t.Errorf("пустой список = %q; want слово нет", got)
		}
	}
	values := []string{"efx7-n1000-v100", "efx7-n003-v06", "efx7-n003-v06", "efx7-n999999999999999999999-v09"}
	before := slices.Clone(values)
	text := idList(values)
	got := regexp.MustCompile(`efx7-n[0-9]+(?:-v[0-9]+)?`).FindAllString(text, -1)
	if !slices.Equal(got, values) || !slices.Equal(values, before) {
		t.Errorf("idList утратил полный ID, порядок, повтор или изменил вход: %q", text)
	}
}
