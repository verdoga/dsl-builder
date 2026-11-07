package fragments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestTask9CloneElementPreservesValuesAndOwnership(t *testing.T) {
	empty, semantic := "", "{Ё😀"
	for _, source := range []model.Element{
		{ElementType: model.ElementTypePlaceholder, Raw: "_____", Start: 2, End: 7},
		{ElementType: model.ElementTypeContent, Value: &empty, Start: 1, End: 1, ErrorIDs: []string{}},
		{ElementType: model.ElementTypeContent, Raw: `\{Ё😀`, Value: &semantic, Start: 2, End: 6, ErrorIDs: []string{"d1", "d2"}},
	} {
		t.Run(source.Raw, func(t *testing.T) {
			before := task9JSON(t, source)
			first, second := cloneElement(source), cloneElement(source)
			if task9JSON(t, first) != before || task9JSON(t, second) != before {
				t.Fatal("Клон изменил raw, value, диапазон либо nil/empty ErrorIDs")
			}
			if first.Value != nil {
				if first.Value == source.Value || first.Value == second.Value {
					t.Fatal("Value разделяется между экземплярами")
				}
				*first.Value = "другое значение"
			}
			if len(first.ErrorIDs) > 0 {
				first.ErrorIDs[0] = "замена"
			}
			if task9JSON(t, source) != before || task9JSON(t, second) != before {
				t.Fatal("Изменение клона затронуло источник или другой клон")
			}
		})
	}
}

func TestTask9CloneLinePreservesCompleteSource(t *testing.T) {
	parent, value := 3, "{Ё😀"
	for _, source := range []model.Line{
		{Number: 4, LineType: model.LineTypeBlank, Raw: " \t", LineEnding: model.LineEndingLF},
		{Number: 7, LineType: model.LineTypeBlank, LineEnding: model.LineEndingNone, Elements: []model.Element{}},
		{Number: 11, LineType: model.LineTypeContent, ParentLine: &parent, NestingLevel: 2,
			Raw: "\t\\{Ё😀", LineEnding: model.LineEndingCRLF, HasErrors: true,
			Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: `\{Ё😀`, Value: &value,
				Start: 2, End: 6, ErrorIDs: []string{"d1", "d2"}}}},
	} {
		t.Run(string(source.LineEnding)+source.Raw, func(t *testing.T) {
			before := task9JSON(t, source)
			first, second := cloneLine(source), cloneLine(source)
			if task9JSON(t, first) != before || task9JSON(t, second) != before {
				t.Fatal("Полная исходная строка не сохранена")
			}
			if first.ParentLine != nil {
				if first.ParentLine == source.ParentLine || first.ParentLine == second.ParentLine {
					t.Fatal("ParentLine разделяется между экземплярами")
				}
				*first.ParentLine = 99
			}
			if len(first.Elements) > 0 {
				first.Elements[0].Raw = "замена"
				*first.Elements[0].Value = "замена"
				first.Elements[0].ErrorIDs[0] = "замена"
			}
			if task9JSON(t, source) != before || task9JSON(t, second) != before {
				t.Fatal("ParentLine, Elements, Value или ErrorIDs не скопированы независимо")
			}
		})
	}
}

func TestTask9CopyEntriesUsesSparseIndexWithoutRenumbering(t *testing.T) {
	input := task9Input(t, []model.Line{
		{Number: 3, LineType: model.LineTypeContent, Raw: "Ё", LineEnding: model.LineEndingLF,
			Elements: []model.Element{task9Element(t, model.ElementTypeContent, "Ё", "Ё", 1)}},
		{Number: 11, LineType: model.LineTypeBlank, Raw: "\t", LineEnding: model.LineEndingCRLF, Elements: []model.Element{}},
		{Number: 24, LineType: model.LineTypeContent, Raw: `\{😀`, LineEnding: model.LineEndingNone,
			Elements: []model.Element{task9Element(t, model.ElementTypeContent, `\{😀`, "{😀", 1)}},
	})
	before := task9JSON(t, input)
	numbers := []int{11, 24}
	first, second := copyEntries(input, numbers), copyEntries(input, numbers)
	if len(first) != 2 || len(second) != 2 {
		t.Fatal("Копирование потеряло выбранные строки")
	}
	for i, number := range numbers {
		if first[i] == nil || second[i] == nil || first[i] == second[i] {
			t.Fatal("Каждый вызов должен создавать отдельные Entry")
		}
		if task9JSON(t, first[i].Source) != task9JSON(t, input.Result.Lines[input.Index.ByNumber[number]]) {
			t.Fatalf("Строка %d не найдена через индекс или изменена", number)
		}
		if first[i].Parent != nil || first[i].Included != nil {
			t.Fatal("copyEntries преждевременно связал Parent/Included")
		}
	}
	*first[1].Source.Elements[0].Value = "изменено"
	first[1].Source.Elements[0].Raw = "изменено"
	if task9JSON(t, input) != before || *second[1].Source.Elements[0].Value != "{😀" {
		t.Fatal("Копии разделяют изменяемые данные с Input или другой копией")
	}
	if numbers[0] != 11 || numbers[1] != 24 {
		t.Fatal("Изменён список физических номеров")
	}
}

func TestTask9BindParentsDocumentAndIndependentBoundaries(t *testing.T) {
	input := task9GraphInput(t)
	before := task9JSON(t, input)
	document := copyEntries(input, []int{3, 8, 12, 17, 21, 29, 40})
	if issues := bindParents(input, document, 0, nil); issues != nil {
		t.Fatalf("Корректные родители документа: %v", issues)
	}
	for i, parent := range []int{-1, 0, 1, 1, 0, 0, -1} {
		var want *Entry
		if parent >= 0 {
			want = document[parent]
		}
		if document[i].Parent != want {
			t.Fatalf("Родитель строки %d не является её копией в документе", document[i].Source.Number)
		}
	}
	firstBoundary, secondBoundary := copyEntries(input, []int{40})[0], copyEntries(input, []int{40})[0]
	firstBoundary.Parent = document[0]
	first := copyEntries(input, []int{8, 12, 17, 21})
	second := copyEntries(input, []int{8, 12, 17, 21})
	for i, entries := range [][]*Entry{first, second} {
		boundary := []*Entry{firstBoundary, secondBoundary}[i]
		sources := []string{task9JSON(t, entries[0].Source), task9JSON(t, entries[1].Source), task9JSON(t, entries[2].Source), task9JSON(t, entries[3].Source)}
		if issues := bindParents(input, entries, 3, boundary); issues != nil {
			t.Fatalf("Корректное тело фрагмента: %v", issues)
		}
		if entries[0].Parent != boundary || entries[3].Parent != boundary || entries[1].Parent != entries[0] || entries[2].Parent != entries[0] {
			t.Fatal("Неверные boundary, внутренний родитель или block-end")
		}
		for j, entry := range entries {
			if task9JSON(t, entry.Source) != sources[j] || entry.Included != nil {
				t.Fatal("Связывание переписало исходные координаты/глубину или Included")
			}
		}
	}
	if first[0] == second[0] || first[1].Parent == second[1].Parent || firstBoundary.Parent != document[0] {
		t.Fatal("Связи двух вставок объединены либо изменён внешний родитель include")
	}
	if task9JSON(t, input) != before {
		t.Fatal("Связывание изменило Input, метаданные или индекс")
	}
}

func TestTask9BindParentsMissingMemberReportsPhysicalSource(t *testing.T) {
	input := task9GraphInput(t)
	before := task9JSON(t, input)
	entries := copyEntries(input, []int{12}) // Родитель 8 существует в Input, но не в этом экземпляре.
	source := task9JSON(t, entries[0].Source)
	issues := bindParents(input, entries, 3, copyEntries(input, []int{40})[0])
	if len(issues) != 1 || issues[0] == nil || issues[0].Code != CodeParent {
		t.Fatalf("Ожидалась одна CodeParent: %v", issues)
	}
	context := issues[0].Context
	if context.Path != input.Path || context.LessonID != "Ab12" || context.Line != 12 || context.PageID != "" {
		t.Fatalf("Утрачено физическое место ошибки: %+v", context)
	}
	russian := false
	for _, r := range issues[0].Reason {
		russian = russian || unicode.Is(unicode.Cyrillic, r)
	}
	if !russian || entries[0].Parent != nil || task9JSON(t, entries[0].Source) != source || task9JSON(t, input) != before {
		t.Fatal("Диагностика/неизменность источника или отсутствие чужого родителя нарушены")
	}
}

func task9GraphInput(t *testing.T) *parsed.Input {
	t.Helper()
	root, block := 3, 8
	return task9Input(t, []model.Line{
		{Number: 3, LineType: model.LineTypeBlockStart, Raw: "@fragment F {", LineEnding: model.LineEndingLF,
			Elements: []model.Element{task9Element(t, model.ElementTypeTag, "@fragment", "fragment", 1), task9Element(t, model.ElementTypeIdentifier, "F", "F", 11), task9Element(t, model.ElementTypeBlockOpen, "{", "{", 13)}},
		{Number: 8, LineType: model.LineTypeBlockStart, ParentLine: &root, NestingLevel: 1, Raw: "@text {", LineEnding: model.LineEndingCRLF,
			Elements: []model.Element{task9Element(t, model.ElementTypeTag, "@text", "text", 1), task9Element(t, model.ElementTypeBlockOpen, "{", "{", 7)}},
		{Number: 12, LineType: model.LineTypeContent, ParentLine: &block, NestingLevel: 2, Raw: "Ё😀", LineEnding: model.LineEndingLF,
			Elements: []model.Element{task9Element(t, model.ElementTypeContent, "Ё😀", "Ё😀", 1)}},
		{Number: 17, LineType: model.LineTypeBlockEnd, ParentLine: &block, NestingLevel: 2, Raw: "}", LineEnding: model.LineEndingLF,
			Elements: []model.Element{task9Element(t, model.ElementTypeBlockClose, "}", "}", 1)}},
		{Number: 21, LineType: model.LineTypeBlank, ParentLine: &root, NestingLevel: 1, Raw: " \t", LineEnding: model.LineEndingLF, Elements: []model.Element{}},
		{Number: 29, LineType: model.LineTypeBlockEnd, ParentLine: &root, NestingLevel: 1, Raw: "}", LineEnding: model.LineEndingLF,
			Elements: []model.Element{task9Element(t, model.ElementTypeBlockClose, "}", "}", 1)}},
		{Number: 40, LineType: model.LineTypeTag, Raw: "@include F", LineEnding: model.LineEndingNone,
			Elements: []model.Element{task9Element(t, model.ElementTypeTag, "@include", "include", 1), task9Element(t, model.ElementTypeIdentifier, "F", "F", 10)}},
	})
}

func task9Input(t *testing.T, lines []model.Line) *parsed.Input {
	t.Helper()
	id, sourcePath, sourceName := "Ab12", `C:\source\old.txt`, "old.txt"
	result := model.Result{Format: model.FormatVersion1, Lines: lines, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Document: model.Document{FileName: &sourceName, FilePath: &sourcePath,
			Metadata: model.DocumentMetadata{DocumentID: &id, ResourceDirs: []string{}}}}
	path := filepath.Join(t.TempDir(), "технический.json")
	if err := os.WriteFile(path, []byte(task9JSON(t, result)), 0600); err != nil {
		t.Fatal(err)
	}
	input, issues := parsed.Load(path)
	if input == nil || issues != nil {
		t.Fatalf("Предусловие проверенного Input не выполнено: %v", issues)
	}
	return input
}

func task9Element(t *testing.T, kind model.ElementType, raw, value string, start int) model.Element {
	t.Helper()
	return model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}}
}

func task9JSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
