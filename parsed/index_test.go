package parsed

import (
	"maps"
	"slices"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func indexTestInt(value int) *int { return &value }

func indexTestByNumber(lines []model.Line) map[int]int {
	result := make(map[int]int, len(lines))
	for i, line := range lines {
		result[line.Number] = i
	}
	return result
}

func indexTestDiagnostic(t *testing.T, item *diagnostic.Error, code string, context diagnostic.Context) {
	t.Helper()
	if item == nil {
		t.Fatal("нет ожидаемой диагностики")
	}
	if item.Code != code || item.Context.Path != context.Path || item.Context.LessonID != context.LessonID {
		t.Errorf("код/контекст: %#v", item)
	}
	if !slices.ContainsFunc([]rune(item.Reason), func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("нет русской причины: %q", item.Reason)
	}
}

func TestIndexNumbersContract(t *testing.T) {
	for _, test := range []struct {
		name    string
		numbers []int
		count   *int
		valid   bool
	}{
		{"empty known", nil, indexTestInt(0), true},
		{"empty unknown", nil, nil, true},
		{"complete", []int{1, 2, 3}, indexTestInt(3), true},
		{"unknown permits gaps", []int{3, 8, 21}, nil, true},
		{"unknown permits large number", []int{int(^uint(0) >> 1)}, nil, true},
		{"zero", []int{0}, nil, false},
		{"negative", []int{-1}, nil, false},
		{"duplicate", []int{1, 1, 2}, nil, false},
		{"descending", []int{3, 2}, nil, false},
		{"late descending", []int{1, 4, 3}, nil, false},
		{"known gap", []int{1, 3}, indexTestInt(3), false},
		{"known shifted equal length", []int{2, 3}, indexTestInt(2), false},
		{"known longer than lines", []int{1, 2}, indexTestInt(3), false},
		{"known shorter than lines", []int{1, 2}, indexTestInt(1), false},
		{"known zero with line", []int{1}, indexTestInt(0), false},
		{"known nonzero without lines", nil, indexTestInt(1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			lines := make([]model.Line, len(test.numbers))
			for i, number := range test.numbers {
				lines[i].Number = number
			}
			context := diagnostic.Context{Path: `C:\Курс\input.json`, LessonID: "AbC+1"}
			got, errs := indexNumbers(lines, test.count, context)
			if test.valid {
				if len(errs) != 0 || !maps.Equal(got, indexTestByNumber(lines)) {
					t.Fatalf("индекс = %v, ошибки = %v", got, errs)
				}
			} else {
				if len(errs) == 0 {
					t.Fatal("некорректные физические номера приняты")
				}
				for _, err := range errs {
					indexTestDiagnostic(t, err, CodeContract, context)
				}
			}
			for i, number := range test.numbers {
				if lines[i].Number != number {
					t.Fatal("исходные номера изменены")
				}
			}
		})
	}
}

func TestIndexNumbersKeepsFirstDuplicate(t *testing.T) {
	got, errs := indexNumbers([]model.Line{{Number: 4}, {Number: 4}}, nil, diagnostic.Context{})
	if len(errs) == 0 {
		t.Fatal("дубль не обнаружен")
	}
	if position, ok := got[4]; !ok || position != 0 {
		t.Fatalf("первое вхождение перезаписано: %v", got)
	}
}

func TestIndexValidateParentContract(t *testing.T) {
	root := model.Line{Number: 3, LineType: model.LineTypeBlockStart}
	for _, test := range []struct {
		name   string
		line   model.Line
		parent model.Line
		valid  bool
	}{
		{"root", model.Line{Number: 8, LineType: model.LineTypeContent}, root, true},
		{"child", model.Line{Number: 8, ParentLine: indexTestInt(3), NestingLevel: 1}, root, true},
		{"root depth", model.Line{Number: 8, NestingLevel: 1}, root, false},
		{"missing", model.Line{Number: 8, ParentLine: indexTestInt(2), NestingLevel: 1}, root, false},
		{"parent zero", model.Line{Number: 8, ParentLine: indexTestInt(0), NestingLevel: 1}, root, false},
		{"self", model.Line{Number: 8, ParentLine: indexTestInt(8), NestingLevel: 1}, root, false},
		{"later", model.Line{Number: 2, ParentLine: indexTestInt(3), NestingLevel: 1}, root, false},
		{"same depth", model.Line{Number: 8, ParentLine: indexTestInt(3)}, root, false},
		{"skipped depth", model.Line{Number: 8, ParentLine: indexTestInt(3), NestingLevel: 2}, root, false},
		{"deep child", model.Line{Number: 8, ParentLine: indexTestInt(3), NestingLevel: 8}, model.Line{Number: 3, NestingLevel: 7}, true},
		{"heading root", model.Line{Number: 8, LineType: model.LineTypeHeading}, root, true},
		{"heading parent", model.Line{Number: 8, LineType: model.LineTypeHeading, ParentLine: indexTestInt(3), NestingLevel: 1}, root, false},
		{"heading depth", model.Line{Number: 8, LineType: model.LineTypeHeading, NestingLevel: 1}, root, false},
		{"block end", model.Line{Number: 8, LineType: model.LineTypeBlockEnd, ParentLine: indexTestInt(3), NestingLevel: 1}, root, true},
		{"block end root", model.Line{Number: 8, LineType: model.LineTypeBlockEnd}, root, false},
		{"block end wrong parent", model.Line{Number: 8, LineType: model.LineTypeBlockEnd, ParentLine: indexTestInt(3), NestingLevel: 1}, model.Line{Number: 3, LineType: model.LineTypeTag}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			lines := []model.Line{test.parent, test.line}
			context := diagnostic.Context{Path: `C:\Урок\parsed.json`, LessonID: "EFX7"}
			err := validateParent(test.line, lines, indexTestByNumber(lines), context)
			if test.valid {
				if err != nil {
					t.Fatalf("допустимая связь отклонена: %v", err)
				}
				return
			}
			indexTestDiagnostic(t, err, CodeParent, context)
			if err.Context.Line != test.line.Number {
				t.Errorf("строка диагностики = %d, нужна %d", err.Context.Line, test.line.Number)
			}
		})
	}
}

func TestIndexParentCyclesAllComponents(t *testing.T) {
	for _, test := range []struct {
		name    string
		parents []int
		cycle   []int
	}{
		{"empty", nil, nil},
		{"forest and shared ancestors", []int{0, 1, 1, 2, 2, 0}, nil},
		{"forward acyclic", []int{2, 3, 0}, nil},
		{"self without root", []int{1}, []int{1}},
		{"pair without root", []int{2, 1}, []int{1, 2}},
		{"root and disconnected cycle with tail", []int{0, 1, 4, 3, 4}, []int{3, 4}},
		{"tail enters cycle", []int{2, 3, 4, 2}, []int{2, 3, 4}},
		{"two disconnected cycles", []int{2, 1, 4, 5, 3, 0}, []int{1, 2, 3, 4, 5}},
	} {
		t.Run(test.name, func(t *testing.T) {
			lines := make([]model.Line, len(test.parents))
			for i, parent := range test.parents {
				lines[i].Number = i + 1
				if parent != 0 {
					lines[i].ParentLine = indexTestInt(parent)
				}
			}
			indexTestCycles(t, lines, test.cycle)
		})
	}
}

func indexTestCycles(t *testing.T, lines []model.Line, want []int) {
	t.Helper()
	context := diagnostic.Context{Path: `C:\Вход\циклы.json`, LessonID: "Lesson+1"}
	errs := validateParentCycles(lines, indexTestByNumber(lines), context)
	if len(want) == 0 && len(errs) != 0 {
		t.Fatalf("конечные цепочки отклонены: %v", errs)
	}
	if len(want) != 0 && len(errs) == 0 {
		t.Fatal("цикл не обнаружен")
	}
	seen := make(map[int]bool)
	for _, err := range errs {
		indexTestDiagnostic(t, err, CodeParentCycle, context)
		for _, related := range err.Related {
			if related.Path != context.Path || related.LessonID != context.LessonID {
				t.Errorf("утрачен контекст участника: %#v", related)
			}
			seen[related.Line] = true
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("участников Related = %d, нужно %d", len(seen), len(want))
	}
	for _, number := range want {
		if !seen[number] {
			t.Errorf("нет участника цикла %d в Related", number)
		}
	}
}

func TestIndexParentCyclesWithoutDepthLimit(t *testing.T) {
	const size = 16385
	lines := make([]model.Line, size)
	for i := range lines {
		lines[i] = model.Line{Number: i + 1, NestingLevel: i}
		if i > 0 {
			lines[i].ParentLine = indexTestInt(i)
		}
	}
	indexTestCycles(t, lines, nil)
	lines[0].ParentLine = indexTestInt(size)
	want := make([]int, size)
	for i := range want {
		want[i] = i + 1
	}
	indexTestCycles(t, lines, want)
}

func TestIndexChildrenPreservesImmediatePhysicalLines(t *testing.T) {
	lines := []model.Line{
		{Number: 1, LineType: model.LineTypeHeading},
		{Number: 2, LineType: model.LineTypeBlockStart},
		{Number: 3, LineType: model.LineTypeBlank, ParentLine: indexTestInt(2), NestingLevel: 1},
		{Number: 4, LineType: model.LineTypeBlockStart, ParentLine: indexTestInt(2), NestingLevel: 1},
		{Number: 5, LineType: model.LineTypeContent, ParentLine: indexTestInt(4), NestingLevel: 2},
		{Number: 6, LineType: model.LineTypeBlockEnd, ParentLine: indexTestInt(4), NestingLevel: 2},
		{Number: 7, LineType: model.LineTypeSeparator, ParentLine: indexTestInt(2), NestingLevel: 1},
		{Number: 8, LineType: model.LineTypeBlockEnd, ParentLine: indexTestInt(2), NestingLevel: 1},
		{Number: 9, LineType: model.LineTypeBlank},
		{Number: 10, LineType: model.LineTypeTag},
	}
	got := indexChildren(lines)
	want := map[int][]int{0: {1, 2, 9, 10}, 2: {3, 4, 7, 8}, 4: {5, 6}}
	if !maps.EqualFunc(got, want, slices.Equal[[]int]) {
		t.Fatalf("Children = %v, нужно %v", got, want)
	}
	if len(indexChildren(nil)) != 0 || len(indexChildren([]model.Line{})) != 0 {
		t.Fatal("пустой документ получил фиктивные связи")
	}
}
