package exercisedata

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exOrderingSnapshot(t *testing.T, value any) string {
	t.Helper()
	if items, ok := value.([]entry); ok {
		var state strings.Builder
		for _, item := range items {
			state.WriteString(exOrderingSnapshot(t, item.data))
			state.WriteString(exOrderingSnapshot(t, item.source))
			state.WriteString(exOrderingSnapshot(t, item.answerSource))
			state.WriteString(exOrderingSnapshot(t, item.answerSources))
		}
		return state.String()
	}
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func exOrderingError(t *testing.T, err *diagnostic.Error, code string, context diagnostic.Context) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got nil", code)
	}
	if err.Code != code || err.Context != context || !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Fatalf("diagnostic = %+v, want %s at %+v with Russian reason", err, code, context)
	}
}
func exOrderingContext() diagnostic.Context {
	return diagnostic.Context{Path: "технический.json", FileName: "урок.json", LessonID: "AbCd", PageID: "AbCd-p001", Line: 70, Column: 11}
}
func exOrderingEntries(answers [][]string) []entry {
	items := make([]entry, len(answers))
	for i, values := range answers {
		source := exOrderingContext()
		source.Line, source.Column = source.Line+i*10, 4
		answer := source
		answer.Line, answer.Column = source.Line+1, 9
		role := lesson.RoleItem
		items[i] = entry{data: lesson.Data{Type: lesson.DataText, Role: &role, Value: "одинаковый текст", Answers: slices.Clone(values), Hint: "подсказка"}, source: source, answerSource: &answer}
		for range values {
			items[i].answerSources = append(items[i].answerSources, answer)
		}
	}
	return items
}
func TestExOrderingPositionIndex(t *testing.T) {
	context := exOrderingContext()
	for _, count := range []int{1, 2, 10, int(^uint(0) >> 1)} {
		value := strconv.Itoa(count)
		if got, err := positionIndex(value, count, context); err != nil || got != count-1 {
			t.Errorf("%q/%d = %d, %v", value, count, got, err)
		}
	}
	for _, value := range []string{"", "0", "00", "01", "002", "+1", "-1", "1.0", "1e0", " 1", "1 ", "1\n", "١", "１", "²", "1_0", "1\x00", "3", "10", "999999999999999999999999999999999999", strings.Repeat("9", 10000)} {
		t.Run(strconv.Quote(value[:min(len(value), 30)]), func(t *testing.T) {
			_, err := positionIndex(value, 2, context)
			exOrderingError(t, err, CodeReference, context)
		})
	}
}
func TestExOrderingNumberedChoices(t *testing.T) {
	items := exOrderingEntries([][]string{{"2", "1", "2"}, {"1"}})
	before := exOrderingSnapshot(t, items)
	choices, err := numberedChoices(items, exOrderingContext())
	if err != nil || len(choices) != 2 {
		t.Fatalf("choices = %v, %v", choices, err)
	}
	for i, want := range [][]int{{0, 1}, {0}} {
		got := slices.Clone(choices[i])
		slices.Sort(got)
		got = slices.Compact(got)
		if !slices.Equal(got, want) {
			t.Errorf("choices[%d] = %v", i, choices[i])
		}
	}
	if exOrderingSnapshot(t, items) != before {
		t.Fatal("numbered validation mutated entries")
	}
	items[0].data.Answers[1] = "01"
	_, err = numberedChoices(items, exOrderingContext())
	exOrderingError(t, err, CodeReference, items[0].answerSources[1])
	exOrderingError(t, checkNumberedPositions(items, exOrderingContext()), CodeReference, items[0].answerSources[1])
}
func TestExOrderingPermutationExhaustive(t *testing.T) {
	for graphID := 0; graphID < 343; graphID++ {
		masks := []int{graphID%7 + 1, graphID/7%7 + 1, graphID/49 + 1}
		choices := make([][]int, 3)
		values := make([][]string, 3)
		for i, mask := range masks {
			for p := 0; p < 3; p++ {
				if mask&(1<<p) != 0 {
					choices[i] = append(choices[i], p)
					values[i] = append(values[i], strconv.Itoa(p+1))
				}
			}
		}
		want := false
		for _, permutation := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
			if masks[0]&(1<<permutation[0]) != 0 && masks[1]&(1<<permutation[1]) != 0 && masks[2]&(1<<permutation[2]) != 0 {
				want = true
			}
		}
		items := exOrderingEntries(values)
		before, graph := exOrderingSnapshot(t, items), exOrderingSnapshot(t, choices)
		err := requirePermutation(choices, items, exOrderingContext())
		if (err == nil) != want {
			t.Fatalf("masks %v: want permutation %v, got %v", masks, want, err)
		}
		if !want {
			exOrderingError(t, err, CodeContradiction, exOrderingContext())
			for _, item := range items {
				if !slices.Contains(err.Related, item.source) && !slices.Contains(err.Related, *item.answerSource) {
					t.Fatal("contradiction lost participant", item.source)
				}
			}
		}
		if before != exOrderingSnapshot(t, items) || graph != exOrderingSnapshot(t, choices) {
			t.Fatal("permutation search mutated source")
		}
	}
}
func TestExOrderingAssignPositionAugmentingPath(t *testing.T) {
	const n = 2048
	choices, owner, visited := make([][]int, n), make([]int, n), make([]bool, n)
	for i := 0; i < n-1; i++ {
		choices[i] = []int{i, i + 1}
		owner[i] = i
	}
	choices[n-1] = []int{0}
	owner[n-1] = -1
	before := exOrderingSnapshot(t, choices)
	if !assignPosition(n-1, choices, owner, visited) {
		t.Fatal("long augmenting path rejected")
	}
	seen := make([]bool, n)
	for p, item := range owner {
		if item < 0 || item >= n || seen[item] || !slices.Contains(choices[item], p) {
			t.Fatalf("invalid ownership: position %d -> %d", p, item)
		}
		seen[item] = true
	}
	if exOrderingSnapshot(t, choices) != before {
		t.Fatal("choices mutated")
	}
	owner = []int{0, -1}
	visited = []bool{true, false}
	if assignPosition(1, [][]int{{0}, {0}}, owner, visited) || !slices.Equal(owner, []int{0, -1}) {
		t.Fatal("visited position reused or failed search changed owner")
	}
}
func exOrderingLines(answers [][]string, positions []string) []model.Line {
	var lines []model.Line
	levels := map[int]int{}
	add := func(kind model.LineType, parent int, tag, value string) int {
		number := 100 + len(lines)*3
		line := model.Line{Number: number, LineType: kind, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
		if parent != 0 {
			line.ParentLine = &parent
			line.NestingLevel = levels[parent] + 1
		}
		appendElement := func(kind model.ElementType, raw, value string) {
			start := len([]rune(line.Raw)) + 1
			line.Raw += raw
			line.Elements = append(line.Elements, model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: len([]rune(line.Raw)) + 1, ErrorIDs: []string{}})
		}
		if tag != "" {
			appendElement(model.ElementTypeTag, "@"+tag, tag)
			line.Raw += " "
		}
		if value != "" {
			appendElement(model.ElementTypeContent, value, value)
			if kind == model.LineTypeBlockStart {
				line.Raw += " "
			}
		}
		if kind == model.LineTypeBlockStart {
			appendElement(model.ElementTypeBlockOpen, "{", "{")
		}
		if kind == model.LineTypeBlockEnd {
			appendElement(model.ElementTypeBlockClose, "}", "}")
		}
		if kind == model.LineTypeSeparator {
			line.Raw = "---"
		}
		levels[number] = line.NestingLevel
		lines = append(lines, line)
		return number
	}
	root := add(model.LineTypeBlockStart, 0, "ordering", "Внешняя инструкция")
	for _, values := range answers {
		add(model.LineTypeContent, root, "", "2. <b>Ё\\x</b> ")
		if values != nil {
			answer := add(model.LineTypeBlockStart, root, "answer", "")
			for _, value := range values {
				add(model.LineTypeContent, answer, "", value)
			}
			add(model.LineTypeBlockEnd, answer, "", "")
			add(model.LineTypeTag, root, "hint", "<i>Подсказка\\x</i>")
		}
	}
	if positions != nil {
		add(model.LineTypeSeparator, root, "", "")
		for _, value := range positions {
			add(model.LineTypeContent, root, "", value)
		}
	}
	add(model.LineTypeBlockEnd, root, "", "")
	return lines
}
func TestExOrderingDataConfigurations(t *testing.T) {
	for _, tc := range []struct {
		name      string
		answers   [][]string
		positions []string
		check     bool
		code      string
	}{
		{"none", [][]string{nil, nil}, nil, false, ""},
		{"permutation", [][]string{{"2"}, {"1"}}, nil, true, ""},
		{"alternatives", [][]string{{"1", "2", "1"}, {"1"}}, nil, true, ""},
		{"contradiction", [][]string{{"1"}, {"1"}}, nil, false, CodeContradiction},
		{"bad-reference", [][]string{{"3"}, {"1"}}, nil, false, CodeReference},
		{"partial", [][]string{{"1"}, nil}, nil, false, CodeIncompleteAnswers},
		{"empty-items", nil, nil, false, CodeCollection},
		{"one-item", [][]string{nil}, nil, false, CodeCollection},
		{"empty-positions", [][]string{nil, nil}, []string{}, false, CodeCollection},
		{"one-position", [][]string{nil, nil}, []string{"a"}, false, CodeCollection},
		{"labels-many-to-one", [][]string{{"КЛАСС"}, {"класс"}, {"Класс"}}, []string{"Класс", "Другое"}, true, ""},
		{"labels-none", [][]string{nil, nil}, []string{"a", "A"}, false, ""},
		{"labels-ambiguous", [][]string{{"a"}, {"a"}}, []string{"a", "A"}, false, CodeReference},
		{"labels-missing", [][]string{{"a "}, {"a"}}, []string{"a", "b"}, false, CodeReference},
		{"labels-partial", [][]string{{"a"}, nil}, []string{"a", "b"}, false, CodeIncompleteAnswers},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := exOrderingLines(tc.answers, tc.positions)
			before := exOrderingSnapshot(t, lines)
			context := exOrderingContext()
			data, check, err := orderingData(lines, context)
			if tc.code != "" {
				if err == nil || err.Code != tc.code || data != nil || check {
					t.Fatalf("got %v, %v, %v; want %s", data, check, err, tc.code)
				}
				if err.Context.Path != context.Path || err.Context.FileName != context.FileName || err.Context.LessonID != context.LessonID || err.Context.PageID != context.PageID {
					t.Fatalf("lost context: %+v", err)
				}
				return
			}
			if err != nil || check != tc.check || len(data) != 1+map[bool]int{true: 1}[tc.positions != nil] {
				t.Fatalf("got %v, %v, %v", data, check, err)
			}
			for k, collection := range data {
				role, leaf := []lesson.DataRole{lesson.RoleItems, lesson.RolePositions}[k], []lesson.DataRole{lesson.RoleItem, lesson.RolePosition}[k]
				rows, ok := collection.Value.([]lesson.Data)
				if !ok || rows == nil || len(rows) != []int{len(tc.answers), len(tc.positions)}[k] || collection.Type != lesson.DataCollection || collection.Role == nil || *collection.Role != role || collection.Answers == nil || len(collection.Answers) != 0 || collection.Hint != "" {
					t.Fatalf("bad collection: %+v", collection)
				}
				for i, row := range rows {
					value, hint := "2. <b>Ё\\x</b> ", ""
					answers := []string{}
					if k == 1 {
						value = tc.positions[i]
					} else if tc.answers[i] != nil {
						answers = tc.answers[i]
						hint = "<i>Подсказка\\x</i>"
					}
					if row.Type != lesson.DataText || row.Role == nil || *row.Role != leaf || row.Value != value || row.Answers == nil || !slices.Equal(row.Answers, answers) || row.Hint != hint {
						t.Fatalf("bad leaf[%d]: %+v", i, row)
					}
				}
			}
			first := exOrderingSnapshot(t, data)
			data[0].Value.([]lesson.Data)[0].Hint = "changed"
			if len(data[0].Value.([]lesson.Data)[0].Answers) > 0 {
				data[0].Value.([]lesson.Data)[0].Answers[0] = "changed"
			}
			again, _, err := orderingData(lines, context)
			if err != nil || first != exOrderingSnapshot(t, again) {
				t.Fatal("output aliases input or another invocation")
			}
			if before != exOrderingSnapshot(t, lines) {
				t.Fatal("input mutated")
			}
		})
	}
}
