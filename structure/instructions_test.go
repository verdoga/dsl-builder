package structure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestTask15SplitInstructionPresence(t *testing.T) {
	cases := []struct {
		name     string
		elements []model.Element
		want     bool
	}{
		{"absent", nil, false},
		{"empty", []model.Element{task15Element("content", "", task15String(""))}, false},
		{"empty_semantic_raw_placeholder", []model.Element{task15Element("content", "_____", task15String(""))}, false},
		{"empty_semantic_raw_instruction", []model.Element{task15Element("content", "Заполните", task15String(""))}, false},
		{"text", []model.Element{task15Element("content", "текст", task15String("текст"))}, true},
		{"whitespace_is_not_empty", []model.Element{task15Element("content", " \t ", task15String(" \t "))}, true},
		{"semantic_text_without_raw", []model.Element{task15Element("content", "", task15String("Текст"))}, true},
		{"markup_is_opaque", []model.Element{task15Element("content", `\@instr`, task15String(`<b>_____ {answer} @newpage</b>`))}, true},
		{"placeholder_without_answer", []model.Element{task15Element("placeholder", "_____", nil)}, true},
		{"placeholder_with_answer", []model.Element{task15Element("placeholder", "_____{ОТВЕТ}", task15String("ОТВЕТ"))}, true},
		{"placeholder_with_empty_answer", []model.Element{task15Element("placeholder", "_____{}", task15String(""))}, true},
		{"empty_and_placeholder", []model.Element{task15Element("content", "", task15String("")), task15Element("placeholder", "_____", nil)}, true},
		{"placeholder_between_text", []model.Element{
			task15Element("content", "До", task15String("До")),
			task15Element("placeholder", "_____", nil),
			task15Element("content", "после", task15String("после")),
		}, true},
		{"multiple_empty_contents", []model.Element{task15Element("content", "", task15String("")), task15Element("content", "x", task15String(""))}, false},
		{"later_content", []model.Element{task15Element("content", "", task15String("")), task15Element("content", "x", task15String("x"))}, true},
		{"title_is_not_instruction", []model.Element{task15Element("title", "Название", task15String("Название"))}, false},
	}
	for _, kind := range []lesson.NodeType{lesson.NodeMultifill, lesson.NodeOrdering, lesson.NodeMatching} {
		for _, tc := range cases {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				state, entries := task15Source(t, kind, tc.elements)
				before := task15Snapshot(t, state.input, entries)
				got, problem := splitInstruction(entries[0], kind, state)
				if problem != nil || got != tc.want {
					t.Fatalf("splitInstruction = %v, %v; требуется %v, nil", got, problem, tc.want)
				}
				if after := task15Snapshot(t, state.input, entries); after != before {
					t.Fatal("splitInstruction изменил проверенный вход или вхождение")
				}
			})
		}
	}
}

func TestTask15SplitInstructionOtherNodeTypes(t *testing.T) {
	kinds := []lesson.NodeType{
		lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants, lesson.NodeVariant,
		lesson.NodeHeading, lesson.NodeText, lesson.NodeInstruction, lesson.NodeNote,
		lesson.NodeAlt, lesson.NodeScript, lesson.NodeHint, lesson.NodeAnswerKey,
		lesson.NodeExample, lesson.NodeWordList, lesson.NodeTable, lesson.NodeMedia,
		lesson.NodeQuestion, lesson.NodeSingleChoice, lesson.NodeMultipleChoice,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			state, entries := task15Source(t, kind, []model.Element{
				task15Element("content", "Инструкция", task15String("Инструкция")),
				task15Element("placeholder", "_____", nil),
			})
			before := task15Snapshot(t, state.input, entries)
			got, problem := splitInstruction(entries[0], kind, state)
			if got || problem != nil {
				t.Fatalf("splitInstruction = %v, %v; требуется false, nil", got, problem)
			}
			if task15Snapshot(t, state.input, entries) != before {
				t.Fatal("Проверка другого типа изменила исходные данные")
			}
		})
	}
}

func TestTask15SplitInstructionUnknownContent(t *testing.T) {
	// Load не принимает content=null. Здесь проверяется явно объявленный
	// локальный guard splitInstruction на отдельной копии Entry, не Build API.
	cases := []struct {
		name     string
		elements []model.Element
		unknown  int
	}{
		{"only_content", []model.Element{task15Element("content", "неизвестно", task15String("исходное"))}, 0},
		{"after_nonempty", []model.Element{task15Element("content", "есть", task15String("есть")), task15Element("content", "нет", task15String("нет"))}, 1},
		{"after_placeholder", []model.Element{task15Element("placeholder", "_____", nil), task15Element("content", "нет", task15String("нет"))}, 1},
		{"before_placeholder", []model.Element{task15Element("content", "нет", task15String("нет")), task15Element("placeholder", "_____", nil)}, 0},
	}
	for _, kind := range []lesson.NodeType{lesson.NodeMultifill, lesson.NodeOrdering, lesson.NodeMatching} {
		for _, tc := range cases {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				state, entries := task15Source(t, kind, tc.elements)
				entry := *entries[0]
				entry.Source.Elements = append([]model.Element(nil), entry.Source.Elements...)
				content := &entry.Source.Elements[tc.unknown+1]
				content.Value = nil
				local := []*fragments.Entry{&entry}
				before := task15Snapshot(t, state.input, local)
				got, problem := splitInstruction(&entry, kind, state)
				if problem == nil {
					t.Fatalf("splitInstruction = %v, nil; требуется CodeSource", got)
				}
				if problem.Code != CodeSource || problem.Context.Path != state.input.Path ||
					problem.Context.LessonID != "Ta15" || problem.Context.Line != 37 ||
					problem.Context.Column != content.Start || problem.Context.PageID != "" {
					t.Fatalf("Неверный код или исходные координаты: %+v", problem)
				}
				if !strings.ContainsFunc(problem.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
					t.Fatalf("Причина должна быть на русском языке: %q", problem.Reason)
				}
				if task15Snapshot(t, state.input, local) != before {
					t.Fatal("Ошибка изменила исходные данные")
				}
			})
		}
	}
}

func task15String(value string) *string { return &value }

func task15Element(kind model.ElementType, raw string, value *string) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: value, ErrorIDs: []string{}}
}

func task15Source(t *testing.T, kind lesson.NodeType, parts []model.Element) (*builder, []*fragments.Entry) {
	t.Helper()
	tag := string(kind)
	switch kind {
	case lesson.NodeHeading:
		tag = "header"
	case lesson.NodeInstruction:
		tag = "instr"
	case lesson.NodeAnswerKey:
		tag = "key"
	case lesson.NodeWordList:
		tag = "wordlist"
	case lesson.NodeSingleChoice:
		tag = "choice"
	case lesson.NodeMultipleChoice:
		tag = "multichoice"
	}
	line := model.Line{Number: 37, LineType: model.LineTypeBlockStart, Raw: "  ", LineEnding: model.LineEndingCRLF}
	items := []model.Element{task15Element("tag", "@"+strings.ToUpper(tag), task15String(tag))}
	items = append(items, parts...)
	items = append(items, task15Element("block-open", "{", task15String("{")))
	for _, element := range items {
		element.Start = len([]rune(line.Raw)) + 1
		line.Raw += element.Raw
		element.End = len([]rune(line.Raw)) + 1
		line.Elements = append(line.Elements, element)
		line.Raw += " "
	}
	parent := 37
	end := model.Line{Number: 38, LineType: model.LineTypeBlockEnd, NestingLevel: 1, ParentLine: &parent, Raw: "}",
		Elements: []model.Element{{ElementType: model.ElementTypeBlockClose, Raw: "}", Value: task15String("}"), Start: 1, End: 2, ErrorIDs: []string{}}}}
	result := model.Result{
		Format:     model.FormatVersion1,
		Document:   model.Document{Metadata: model.DocumentMetadata{DocumentID: task15String("Ta15"), ResourceDirs: []string{}}},
		Processing: []model.Processing{}, Lines: []model.Line{line, end}, Diagnostics: []model.Diagnostic{},
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "технический.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	input, problems := parsed.Load(path)
	if len(problems) != 0 || input == nil {
		t.Fatalf("Load fixture: %v", problems)
	}
	entries, problems := fragments.Expand(input)
	if len(problems) != 0 || len(entries) != 2 {
		t.Fatalf("Expand fixture: %v; записей %d", problems, len(entries))
	}
	return &builder{input: input, page: -1}, entries
}

func task15Snapshot(t *testing.T, input *parsed.Input, entries []*fragments.Entry) string {
	t.Helper()
	raw, err := json.Marshal(struct {
		Input   *parsed.Input
		Entries []*fragments.Entry
	}{input, entries})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
