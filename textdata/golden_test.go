package textdata_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/textdata"
)

type goldenQACase struct {
	Name              string          `json:"name"`
	Golden            int             `json:"golden"`
	Kind              lesson.NodeType `json:"kind"`
	Declaration       int             `json:"declaration"`
	OwnLines          []int           `json:"ownLines"`
	Historical        []int           `json:"historical"`
	Source            string          `json:"source"`
	DirectDeclaration *model.Line     `json:"directDeclaration"`
	DirectLines       []model.Line    `json:"directLines"`
	ExpectedData      []lesson.Data   `json:"expectedData"`
}

// TestGoldenQAProjections проверяет только явно выбранные собственные Data.
// Все семь технических JSON декодируются непосредственно в настоящую модель.
// Фикстуры задают kind/номера строк и исторические пути; дерево не выводится
// из тегов, ParentLine или NestingLevel, TXT не используется.
func TestGoldenQAProjections(t *testing.T) {
	var fixtures struct {
		Cases          []goldenQACase `json:"cases"`
		NormativeCases []goldenQACase `json:"normativeCases"`
	}
	goldenQARead(t, filepath.Join("testdata", "golden_cases.json"), &fixtures)
	var parsed [8]model.Result
	var historical [8]lesson.Lesson
	for n := 1; n <= 7; n++ {
		goldenQARead(t, filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", n)), &parsed[n])
		goldenQARead(t, filepath.Join("..", "golden", fmt.Sprintf("%d. lesson.json", n)), &historical[n])
	}
	var covered [8]int
	names := map[string]bool{}
	for _, tc := range append(fixtures.Cases, fixtures.NormativeCases...) {
		t.Run(tc.Name, func(t *testing.T) {
			if names[tc.Name] || tc.Name == "" || tc.Source == "" {
				t.Fatal("фикстура должна иметь уникальное имя и явный источник ожидания")
			}
			names[tc.Name] = true
			if tc.Golden < 0 || tc.Golden > 7 {
				t.Fatal("номер golden вне 1–7")
			}
			declaration, lines := tc.DirectDeclaration, tc.DirectLines
			if tc.Golden != 0 {
				covered[tc.Golden]++
				if declaration != nil || lines != nil {
					t.Fatal("golden-вход нельзя подменять прямой фикстурой")
				}
				if tc.Declaration != 0 {
					declaration = goldenQALine(t, parsed[tc.Golden].Lines, tc.Declaration)
				}
				lines = make([]model.Line, 0, len(tc.OwnLines))
				for _, number := range tc.OwnLines {
					lines = append(lines, *goldenQALine(t, parsed[tc.Golden].Lines, number))
				}
			}
			if len(tc.Historical) != 0 {
				node := goldenQAHistoricalNode(t, historical[tc.Golden], tc.Historical)
				if node.Type != tc.Kind {
					t.Fatalf("исторический тип %q вместо %q", node.Type, tc.Kind)
				}
				goldenQAData(t, tc.ExpectedData, node.Data)
			}
			before := goldenQASnapshot(t, declaration, lines)
			context := diagnostic.Context{Path: fmt.Sprintf("golden/%d. parsed.json", tc.Golden)}
			got, problem := textdata.Build(tc.Kind, declaration, lines, context)
			if problem != nil {
				t.Fatalf("Build: %+v; source: %s", problem, tc.Source)
			}
			goldenQAData(t, got, tc.ExpectedData)
			second, problem := textdata.Build(tc.Kind, declaration, lines, context)
			if problem != nil {
				t.Fatalf("повторный Build: %+v", problem)
			}
			goldenQAData(t, second, tc.ExpectedData)
			if !bytes.Equal(before, goldenQASnapshot(t, declaration, lines)) {
				t.Fatal("Build изменил выбранный технический JSON")
			}
			for i := range got {
				if got[i].Role == second[i].Role {
					t.Errorf("Data[%d].Role разделяется независимыми вызовами", i)
				}
			}
		})
	}
	for n := 1; n <= 7; n++ {
		if covered[n] == 0 {
			t.Errorf("golden %d не покрыт", n)
		}
	}
	if len(fixtures.Cases) == 0 || len(fixtures.NormativeCases) == 0 {
		t.Fatal("исторические и дополнительные нормативные примеры обязательны")
	}
}

func goldenQARead(t *testing.T, path string, destination any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func goldenQALine(t *testing.T, lines []model.Line, number int) *model.Line {
	t.Helper()
	var selected *model.Line
	for i := range lines {
		if lines[i].Number == number {
			if selected != nil {
				t.Fatalf("повтор номера строки %d в golden", number)
			}
			selected = &lines[i]
		}
	}
	if selected == nil {
		t.Fatalf("нет явно назначенной строки %d", number)
	}
	return selected
}

func goldenQAHistoricalNode(t *testing.T, value lesson.Lesson, indexes []int) lesson.Node {
	t.Helper()
	if len(indexes) < 2 || indexes[0] < 0 || indexes[0] >= len(value.Document.Pages) {
		t.Fatal("некорректный явный путь исторической ноды")
	}
	nodes := value.Document.Pages[indexes[0]].Nodes
	var selected lesson.Node
	for _, index := range indexes[1:] {
		if index < 0 || index >= len(nodes) {
			t.Fatalf("некорректный индекс исторической ноды: %v", indexes)
		}
		selected = nodes[index]
		nodes = selected.Nodes
	}
	return selected
}

func goldenQAData(t *testing.T, got, expected []lesson.Data) {
	t.Helper()
	if got == nil || expected == nil {
		t.Fatal("Data должен быть ненулевым срезом")
	}
	if len(got) != len(expected) {
		t.Fatalf("Data: получено %d элементов, ожидалось %d: %#v", len(got), len(expected), got)
	}
	for i, item := range got {
		want := expected[i]
		value, isText := item.Value.(string)
		wantValue, wantText := want.Value.(string)
		if item.Type != lesson.DataText || want.Type != lesson.DataText || !isText || !wantText {
			t.Fatalf("Data[%d]: требуется текстовая модель, got=%#v want=%#v", i, item, want)
		}
		if item.Role == nil || want.Role == nil || *item.Role != *want.Role {
			t.Errorf("Data[%d]: роли различаются: got=%#v want=%#v", i, item.Role, want.Role)
		}
		if value != wantValue || item.Hint != want.Hint {
			t.Errorf("Data[%d]: value/hint\ngot:  %q / %q\nwant: %q / %q", i, value, item.Hint, wantValue, want.Hint)
		}
		if item.Answers == nil || want.Answers == nil || !slices.Equal(item.Answers, want.Answers) || len(item.Answers) != 0 {
			t.Errorf("Data[%d]: ожидается ненулевой пустой срез Answers: got=%#v want=%#v", i, item.Answers, want.Answers)
		}
	}
}

func goldenQASnapshot(t *testing.T, declaration *model.Line, lines []model.Line) []byte {
	t.Helper()
	raw, err := json.Marshal(struct {
		Declaration *model.Line
		Lines       []model.Line
	}{declaration, lines})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
