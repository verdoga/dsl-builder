package fragments

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task12Line(kind model.LineType, parent int, tag, name string) model.Line {
	line := model.Line{LineType: kind, LineEnding: model.LineEndingCRLF, Elements: []model.Element{}}
	if parent != 0 {
		line.ParentLine = &parent
	}
	add := func(kind model.ElementType, raw, value string) {
		if line.Raw != "" {
			line.Raw += " "
		}
		start := utf8.RuneCountInString(line.Raw) + 1
		line.Raw += raw
		line.Elements = append(line.Elements, model.Element{ElementType: kind, Raw: raw, Value: &value,
			Start: start, End: utf8.RuneCountInString(line.Raw) + 1, ErrorIDs: []string{}})
	}
	if tag != "" {
		// Contract проверяет диапазон Raw; семантический тег задаётся Value.
		add(model.ElementTypeTag, "@ДРУГОЕ", tag)
	}
	if name != "" {
		add(model.ElementTypeIdentifier, `Исходное\Имя`, name)
	}
	switch kind {
	case model.LineTypeBlockStart:
		add(model.ElementTypeBlockOpen, "{", "{")
	case model.LineTypeBlockEnd:
		add(model.ElementTypeBlockClose, "}", "}")
	case model.LineTypeContent:
		add(model.ElementTypeContent, "} текст", "} текст")
	}
	return line
}

func task12Input(t *testing.T, lines ...model.Line) *parsed.Input {
	t.Helper()
	for i := range lines {
		lines[i].Number = i + 1
		if lines[i].ParentLine != nil {
			lines[i].NestingLevel = lines[*lines[i].ParentLine-1].NestingLevel + 1
		}
	}
	id, count := "EFX7", len(lines)
	result := model.Result{Format: model.FormatVersion1, Lines: lines,
		Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Document: model.Document{LineCount: &count, Metadata: model.DocumentMetadata{DocumentID: &id}}}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "технический.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	input, issues := parsed.Load(path)
	if input == nil || len(issues) != 0 {
		t.Fatalf("Предусловие parsed.Load: %v", issues)
	}
	return input
}

func task12Shape(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) == 0 {
		t.Fatal("Ожидалась ошибка формы")
	}
	for _, issue := range issues {
		if issue == nil || issue.Code != CodeShape || issue.Reason == "" {
			t.Fatalf("Неверная диагностика: %+v", issue)
		}
	}
}

func TestTask12ReadIdentifier(t *testing.T) {
	for _, tag := range []string{"fragment", "include"} {
		kind := model.LineTypeBlockStart
		if tag == "include" {
			kind = model.LineTypeTag
		}
		for _, name := range []string{` Ω\Keep `, " ", "é", "e\u0301", "НЕ-ID!?"} {
			t.Run(tag+name, func(t *testing.T) {
				input := task12Input(t, task12Line(kind, 0, tag, name))
				line := input.Result.Lines[0]
				got, issue := readIdentifier(line, diagnostic.Context{Path: input.Path, LessonID: "EFX7", Line: 1})
				want := line.Elements[1]
				if issue != nil || got.Value == nil || *got.Value != name || got.Raw != want.Raw ||
					got.Start != want.Start || got.End != want.End || got.ElementType != want.ElementType {
					t.Fatalf("Потерян исходный identifier: %+v, %v", got, issue)
				}
			})
		}
		for _, fault := range []string{"missing", "null", "empty", "multiple", "wrong-type"} {
			t.Run(tag+"/"+fault, func(t *testing.T) {
				line := task12Line(kind, 0, tag, "Имя")
				switch fault {
				case "missing":
					line.Elements = append(line.Elements[:1], line.Elements[2:]...)
				case "null":
					line.Elements[1].Value = nil
				case "empty":
					empty := ""
					line.Elements[1].Value = &empty
				case "multiple":
					line.Raw += " X"
					value := "Другое"
					line.Elements = append(line.Elements, model.Element{ElementType: model.ElementTypeIdentifier,
						Raw: "X", Value: &value, Start: utf8.RuneCountInString(line.Raw), End: utf8.RuneCountInString(line.Raw) + 1, ErrorIDs: []string{}})
				case "wrong-type":
					line.LineType = model.LineTypeContent
				}
				// null identifier запрещён parsed; частный контракт явно требует его отклонить.
				if fault != "null" {
					line = task12Input(t, line).Result.Lines[0]
				}
				_, issue := readIdentifier(line, diagnostic.Context{Path: "input.json", LessonID: "EFX7", Line: 1})
				task12Shape(t, []*diagnostic.Error{issue})
				if tag == "fragment" && fault != "null" && fault != "wrong-type" {
					input := task12Input(t, line, task12Line(model.LineTypeBlockEnd, 1, "", ""))
					_, issues := collectDefinitions(input)
					task12Shape(t, issues)
				}
			})
		}
	}
}

func TestTask12CollectBodiesAndDocument(t *testing.T) {
	input := task12Input(t,
		task12Line(model.LineTypeTag, 0, "include", "Z"), task12Line(model.LineTypeBlockStart, 0, "variants", ""),
		task12Line(model.LineTypeTag, 2, "variant", ""), task12Line(model.LineTypeBlockStart, 3, "fragment", "Outer"),
		task12Line(model.LineTypeBlank, 4, "", ""), task12Line(model.LineTypeBlockStart, 4, "text", ""),
		task12Line(model.LineTypeContent, 6, "", ""), task12Line(model.LineTypeBlockEnd, 6, "", ""),
		task12Line(model.LineTypeBlockStart, 4, "fragment", "Inner"), task12Line(model.LineTypeContent, 9, "", ""),
		task12Line(model.LineTypeBlockEnd, 9, "", ""), task12Line(model.LineTypeContent, 4, "", ""),
		task12Line(model.LineTypeBlockEnd, 4, "", ""), task12Line(model.LineTypeTag, 2, "variant", ""),
		task12Line(model.LineTypeBlockStart, 14, "fragment", "Z"), task12Line(model.LineTypeBlockEnd, 15, "", ""),
		task12Line(model.LineTypeBlockEnd, 2, "", ""), task12Line(model.LineTypeTag, 0, "task", "Outer"),
		task12Line(model.LineTypeTag, 0, "document-id", "Inner"), task12Line(model.LineTypeBlockStart, 0, "script", ""),
		task12Line(model.LineTypeContent, 20, "", ""), task12Line(model.LineTypeBlockEnd, 20, "", ""),
		task12Line(model.LineTypeBlank, 0, "", ""))
	before, _ := json.Marshal(input)
	definitions, issues := collectDefinitions(input)
	if len(issues) != 0 || len(definitions) != 3 {
		t.Fatalf("Глобальный индекс: %v, %v", definitions, issues)
	}
	for _, tc := range []struct {
		key    string
		header int
		body   []int
	}{
		{"outer", 4, []int{5, 6, 7, 8, 12}}, {"inner", 9, []int{10}}, {"z", 15, []int{}},
	} {
		candidates := definitions[tc.key]
		if len(candidates) != 1 || candidates[0].Header != tc.header || !slices.Equal(candidates[0].Body, tc.body) {
			t.Fatalf("Определение %s: %+v", tc.key, candidates)
		}
		got, issues := bodyLines(input, tc.header)
		if len(issues) != 0 || !slices.Equal(got, tc.body) {
			t.Fatalf("Тело %s: %v, %v", tc.key, got, issues)
		}
		want := input.Result.Lines[tc.header-1].Elements[1]
		gotID := candidates[0].Identifier
		if gotID.Value == nil || *gotID.Value != *want.Value || gotID.Raw != want.Raw || gotID.Start != want.Start || gotID.End != want.End {
			t.Fatal("Индекс потерял исходный identifier")
		}
	}
	if got := documentLines(input); !slices.Equal(got, []int{1, 2, 3, 14, 17, 18, 19, 20, 21, 22, 23}) {
		t.Fatalf("Строки документа: %v", got)
	}
	other, errs := collectDefinitions(task12Input(t, task12Line(model.LineTypeTag, 0, "task", "Outer")))
	if len(other) != 0 || len(errs) != 0 {
		t.Fatal("Пространство имён вышло за текущий документ")
	}
	after, _ := json.Marshal(input)
	if !bytes.Equal(before, after) {
		t.Fatal("Изменён проверенный Input")
	}
}

func TestTask12BodyRejectsBoundaries(t *testing.T) {
	for _, fault := range []string{"missing", "extra", "late-child", "late-grandchild"} {
		t.Run(fault, func(t *testing.T) {
			lines := []model.Line{task12Line(model.LineTypeBlockStart, 0, "fragment", "F"),
				task12Line(model.LineTypeBlockStart, 1, "text", ""), task12Line(model.LineTypeBlockEnd, 2, "", "")}
			if fault != "missing" {
				lines = append(lines, task12Line(model.LineTypeBlockEnd, 1, "", ""))
			}
			switch fault {
			case "extra":
				lines = append(lines, task12Line(model.LineTypeBlockEnd, 1, "", ""))
			case "late-child":
				lines = append(lines, task12Line(model.LineTypeContent, 1, "", ""))
			case "late-grandchild":
				lines = append(lines, task12Line(model.LineTypeContent, 2, "", ""))
			}
			input := task12Input(t, lines...)
			_, issues := bodyLines(input, 1)
			task12Shape(t, issues)
			_, issues = collectDefinitions(input)
			task12Shape(t, issues)
		})
	}
}

func TestTask12BodyPhysicalOrderAndDepth(t *testing.T) {
	input := task12Input(t, task12Line(model.LineTypeBlockStart, 0, "fragment", "F"),
		task12Line(model.LineTypeBlockStart, 1, "text", ""), task12Line(model.LineTypeContent, 1, "", ""),
		task12Line(model.LineTypeContent, 2, "", ""), task12Line(model.LineTypeBlockEnd, 2, "", ""),
		task12Line(model.LineTypeBlockEnd, 1, "", ""))
	if got, issues := bodyLines(input, 1); len(issues) != 0 || !slices.Equal(got, []int{2, 3, 4, 5}) {
		t.Fatalf("Физический порядок отличается от DFS: %v, %v", got, issues)
	}
	const depth = 1536
	lines := []model.Line{task12Line(model.LineTypeBlockStart, 0, "fragment", "F")}
	for parent := 1; parent <= depth; parent++ {
		lines = append(lines, task12Line(model.LineTypeBlockStart, parent, "text", ""))
	}
	for parent := depth + 1; parent >= 1; parent-- {
		lines = append(lines, task12Line(model.LineTypeBlockEnd, parent, "", ""))
	}
	input = task12Input(t, lines...)
	got, issues := bodyLines(input, 1)
	if len(issues) != 0 || len(got) != 2*depth {
		t.Fatalf("Глубокое тело: %d строк, %v", len(got), issues)
	}
	for i, number := range got {
		if number != i+2 {
			t.Fatalf("Порядок глубокой строки %d: %d", i, number)
		}
	}
}

func TestTask12NamesSemanticStableAndUnused(t *testing.T) {
	var lines []model.Line
	for _, name := range []string{"Я", "Alpha", "я", "ALPHA", "Я", " я ", "é", "e\u0301", "НЕ-ID!?"} {
		lines = append(lines, task12Line(model.LineTypeBlockStart, 0, "fragment", name),
			task12Line(model.LineTypeBlockEnd, len(lines)+1, "", ""))
	}
	input := task12Input(t, lines...)
	definitions, issues := collectDefinitions(input)
	if len(issues) != 0 || len(definitions) != 6 || len(definitions["я"]) != 3 || len(definitions["alpha"]) != 2 {
		t.Fatalf("Семантические ключи: %v, %v", definitions, issues)
	}
	for repeat := 0; repeat < 20; repeat++ {
		issues = checkDefinitionNames(input, definitions)
		if len(issues) != 2 {
			t.Fatalf("По одной ошибке на невостребованную группу: %v", issues)
		}
		for i, expected := range [][]int{{1, 5, 9}, {3, 7}} {
			issue := issues[i]
			if issue.Code != CodeAmbiguous || issue.Reason == "" || len(issue.Related) != len(expected) {
				t.Fatalf("Группа %d: %+v", i, issue)
			}
			for j, number := range expected {
				place := issue.Related[j]
				if place.Line != number || place.Column != input.Result.Lines[number-1].Elements[1].Start ||
					place.Path != input.Path || place.LessonID != "EFX7" || place.PageID != "" {
					t.Fatalf("Связанный участник %d: %+v", number, place)
				}
			}
			name := *definitions[[]string{"я", "alpha"}[i]][0].Identifier.Value
			found := false
			for _, detail := range issue.Details {
				found = found || detail.Value == name
			}
			if !found {
				t.Fatalf("Нет исходного имени %q в Details", name)
			}
		}
	}
}
