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

func exDataText(t *testing.T, got lesson.Data, role lesson.DataRole, value string, answers []string, hint string) {
	t.Helper()
	if got.Type != lesson.DataText || got.Role == nil || *got.Role != role || got.Value != value || got.Answers == nil || !slices.Equal(got.Answers, answers) || got.Hint != hint {
		t.Fatalf("неверные текстовые данные: %#v", got)
	}
}

func exDataLeaves(t *testing.T, got lesson.Data, role lesson.DataRole, count int) []lesson.Data {
	t.Helper()
	leaves, ok := got.Value.([]lesson.Data)
	if got.Type != lesson.DataCollection || got.Role == nil || *got.Role != role || !ok || leaves == nil || len(leaves) != count || got.Answers == nil || len(got.Answers) != 0 || got.Hint != "" {
		t.Fatalf("неверная коллекция: %#v", got)
	}
	return leaves
}

func TestExDataTextPreservesSemanticValuesAndOwnsAnswers(t *testing.T) {
	for _, role := range []lesson.DataRole{lesson.RoleBody, lesson.RoleInstruction, lesson.RoleItem, lesson.RoleOption, lesson.RolePosition} {
		t.Run(string(role), func(t *testing.T) {
			answers := []string{"  Ёж!  ", `C:\new\word`, "<b>A</b>\nB", "  Ёж!  "}
			want := slices.Clone(answers)
			value, hint := `  <i>Текст\n!</i>  `, "Подсказка\n<b>Да!</b>"
			first := textData(role, value, answers, hint)
			second := textData(role, value, answers, hint)
			exDataText(t, first, role, value, want, hint)
			answers[0] = "вход изменён"
			exDataText(t, first, role, value, want, hint)
			first.Answers[1] = "выход изменён"
			*first.Role = lesson.RoleHint
			exDataText(t, second, role, value, want, hint)
			if answers[1] != want[1] {
				t.Fatal("изменение выхода затронуло входные ответы")
			}
		})
	}
}

func TestExDataTextAbsentAnswersSerializeAsArray(t *testing.T) {
	for _, answers := range [][]string{nil, {}} {
		got := textData(lesson.RoleBody, "", answers, "")
		exDataText(t, got, lesson.RoleBody, "", []string{}, "")
		raw, err := json.Marshal(got)
		if err != nil || !strings.Contains(string(raw), `"answers":[]`) || !strings.Contains(string(raw), `"hint":""`) {
			t.Fatalf("ожидались обязательные пустые поля: %s, %v", raw, err)
		}
	}
}

func TestExDataCollectionsPreserveLeavesAndOwnership(t *testing.T) {
	for _, pair := range [][2]lesson.DataRole{{lesson.RoleItems, lesson.RoleItem}, {lesson.RoleOptions, lesson.RoleOption}, {lesson.RolePositions, lesson.RolePosition}} {
		t.Run(string(pair[0]), func(t *testing.T) {
			leafRole := pair[1]
			entries := []entry{
				{data: lesson.Data{Type: lesson.DataText, Role: &leafRole, Value: "  А!  ", Answers: []string{"Б!", "б!"}, Hint: "<b>Hint</b>"}},
				{data: lesson.Data{Type: lesson.DataText, Role: &leafRole, Value: "  А!  ", Answers: []string{}, Hint: ""}},
			}
			first := collectionData(pair[0], entries)
			second := collectionData(pair[0], entries)
			leaves := exDataLeaves(t, first, pair[0], 2)
			exDataText(t, leaves[0], pair[1], "  А!  ", []string{"Б!", "б!"}, "<b>Hint</b>")
			exDataText(t, leaves[1], pair[1], "  А!  ", []string{}, "")
			entries[0].data.Value = "вход изменён"
			entries[0].data.Answers[0] = "входной ответ изменён"
			leafRole = lesson.RoleHint
			exDataText(t, leaves[0], pair[1], "  А!  ", []string{"Б!", "б!"}, "<b>Hint</b>")
			leaves[0].Value, leaves[0].Hint = "выход изменён", "другая подсказка"
			leaves[0].Answers[1] = "другой ответ"
			*leaves[0].Role = lesson.RoleBody
			*first.Role = lesson.RoleHint
			other := exDataLeaves(t, second, pair[0], 2)
			exDataText(t, other[0], pair[1], "  А!  ", []string{"Б!", "б!"}, "<b>Hint</b>")
			exDataText(t, other[1], pair[1], "  А!  ", []string{}, "")
			if entries[0].data.Answers[1] != "б!" || leafRole != lesson.RoleHint {
				t.Fatal("выходная коллекция владеет данными входа")
			}
		})
	}
}

func TestExDataEmptyCollectionHasNonNilValueAndAnswers(t *testing.T) {
	for _, role := range []lesson.DataRole{lesson.RoleItems, lesson.RoleOptions, lesson.RolePositions} {
		for _, entries := range [][]entry{nil, {}} {
			got := collectionData(role, entries)
			exDataLeaves(t, got, role, 0)
			raw, err := json.Marshal(got)
			if err != nil || !strings.Contains(string(raw), `"value":[]`) || !strings.Contains(string(raw), `"answers":[]`) {
				t.Fatalf("пустая коллекция должна сериализоваться массивами: %s, %v", raw, err)
			}
		}
	}
}

func exDataDeclaration(t *testing.T, tag string, content *string) model.Line {
	t.Helper()
	line := model.Line{Number: 127, LineType: model.LineTypeBlockStart, Raw: "@" + tag, LineEnding: model.LineEndingLF}
	line.Elements = []model.Element{{ElementType: model.ElementTypeTag, Raw: "@" + tag, Value: &tag, Start: 1, End: len(tag) + 2}}
	if content != nil {
		start := len(tag) + 3
		line.Raw += " " + *content
		line.Elements = append(line.Elements, model.Element{ElementType: model.ElementTypeContent, Raw: *content, Value: content, Start: start, End: start + len([]rune(*content))})
	}
	start := len([]rune(line.Raw)) + 2
	open := "{"
	line.Raw += " {"
	line.Elements = append(line.Elements, model.Element{ElementType: model.ElementTypeBlockOpen, Raw: open, Value: &open, Start: start, End: start + 1})
	return line
}

func TestExDataLocalInstructionChoiceKinds(t *testing.T) {
	for _, tag := range []string{"choice", "multichoice"} {
		t.Run(tag, func(t *testing.T) {
			value := `  <b>Выбери\n!</b>  `
			line := exDataDeclaration(t, tag, &value)
			before, err := json.Marshal(line)
			if err != nil {
				t.Fatal(err)
			}
			first, problem := localInstruction(line, diagnostic.Context{Path: "вход/parsed.json"})
			if problem != nil || first == nil {
				t.Fatalf("инструкция не извлечена: %v", problem)
			}
			second, problem := localInstruction(line, diagnostic.Context{})
			if problem != nil || second == nil {
				t.Fatalf("повторная инструкция: %v", problem)
			}
			exDataText(t, *first, lesson.RoleInstruction, value, []string{}, "")
			after, err := json.Marshal(line)
			if err != nil || string(before) != string(after) {
				t.Fatal("вход изменён")
			}
			*first.Role, first.Value = lesson.RoleHint, "изменено"
			exDataText(t, *second, lesson.RoleInstruction, value, []string{}, "")
			value = "изменён вход"
			if second.Value == value {
				t.Fatal("выход связан с входным Value")
			}
			absent, problem := localInstruction(exDataDeclaration(t, tag, nil), diagnostic.Context{})
			if problem != nil || absent != nil {
				t.Fatalf("отсутствующая инструкция: %#v, %v", absent, problem)
			}
		})
	}
}

func TestExDataLocalInstructionRejectsMatchingWithoutChangingInput(t *testing.T) {
	value := `  <b>Сопоставь\n!</b>  `
	for _, tc := range []struct {
		name    string
		content *string
	}{{"with content", &value}, {"without content", nil}} {
		t.Run(tc.name, func(t *testing.T) {
			line := exDataDeclaration(t, "matching", tc.content)
			before, err := json.Marshal(line)
			if err != nil {
				t.Fatal(err)
			}
			context := diagnostic.Context{Path: "русский/parsed.json", FileName: "урок.json", LessonID: "lesson", PageID: "page", Line: 9, Column: 5}
			got, problem := localInstruction(line, context)
			if got != nil || problem == nil || problem.Code != CodeSource {
				t.Fatalf("matching принят как локальная инструкция: %#v, %v", got, problem)
			}
			wantContext := context
			wantContext.Line, wantContext.Column = line.Number, 0
			if problem.Context != wantContext || problem.Reason == "" {
				t.Fatalf("утрачен контекст ошибки matching: %#v", problem)
			}
			after, err := json.Marshal(line)
			if err != nil || string(before) != string(after) {
				t.Fatal("отклонение matching изменило вход")
			}
		})
	}
}

func TestExDataLocalInstructionRejectsForeignAndUnknownValues(t *testing.T) {
	for _, tag := range []string{"ordering", "question", "instr", "multifill", ""} {
		t.Run(tag, func(t *testing.T) {
			value := "Не локальная инструкция"
			got, err := localInstruction(exDataDeclaration(t, tag, &value), diagnostic.Context{})
			if got != nil || err == nil {
				t.Fatalf("чужой тег принят: %#v, %v", got, err)
			}
		})
	}
	for _, mode := range []string{"nil content", "duplicate content", "nil tag"} {
		t.Run(mode, func(t *testing.T) {
			value := "Текст"
			line := exDataDeclaration(t, "choice", &value)
			switch mode {
			case "nil content":
				line.Elements[1].Value = nil
			case "duplicate content":
				line.Elements = append(line.Elements, line.Elements[1])
			case "nil tag":
				line.Elements[0].Value = nil
			}
			context := diagnostic.Context{Path: "русский/parsed.json", LessonID: "lesson", PageID: "page"}
			got, err := localInstruction(line, context)
			if got != nil || err == nil || err.Code != CodeSource {
				t.Fatalf("неизвестное значение принято: %#v, %v", got, err)
			}
			if err.Context.Path != context.Path || err.Context.Line != 127 || err.Reason == "" {
				t.Fatalf("утрачен контекст ошибки: %#v", err)
			}
		})
	}
}

func TestExDataRequireCountUsesMinimumWithoutMaximum(t *testing.T) {
	context := diagnostic.Context{Path: "каталог/parsed.json", Line: 127, Column: 9}
	for _, tc := range []struct {
		role    lesson.DataRole
		minimum int
	}{{lesson.RoleItems, 1}, {lesson.RoleItems, 2}, {lesson.RoleOptions, 2}, {lesson.RolePositions, 2}} {
		t.Run(string(tc.role)+strconv.Itoa(tc.minimum), func(t *testing.T) {
			for _, count := range []int{0, tc.minimum - 1, tc.minimum, tc.minimum + 1, 10001} {
				entries := make([]entry, count)
				err := requireCount(tc.role, entries, tc.minimum, context)
				if count >= tc.minimum {
					if err != nil {
						t.Fatalf("допустимое количество %d: %v", count, err)
					}
					continue
				}
				if err == nil || err.Code != CodeCollection || err.Context != context {
					t.Fatalf("неверная ошибка минимума %d/%d: %#v", count, tc.minimum, err)
				}
				if !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
					t.Fatalf("причина не по-русски: %q", err.Reason)
				}
				info := err.Reason
				for _, detail := range err.Details {
					info += " " + detail.Label + " " + detail.Value
				}
				for _, want := range []string{string(tc.role), strconv.Itoa(count), strconv.Itoa(tc.minimum)} {
					if !strings.Contains(info, want) {
						t.Fatalf("нет роли/количества %q в %q", want, info)
					}
				}
			}
		})
	}
	if err := requireCount(lesson.RoleItems, nil, 0, context); err != nil {
		t.Fatalf("нулевой минимум: %v", err)
	}
}
