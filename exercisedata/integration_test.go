package exercisedata_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestExIntegrationGoldenExerciseProjections(t *testing.T) {
	// Границы установлены по полным parsed JSON, включая block-end.
	// Порядковый номер относится только к пяти типам упражнений в lesson.
	cases := []struct {
		fixture, first, last, ordinal int
		kind                          lesson.NodeType
	}{
		{3, 18, 22, 0, lesson.NodeSingleChoice},
		{3, 23, 27, 1, lesson.NodeSingleChoice},
		{3, 28, 28, 2, lesson.NodeQuestion},
		{3, 29, 29, 3, lesson.NodeQuestion},
		{5, 9, 10, 0, lesson.NodeQuestion},
		{5, 25, 29, 1, lesson.NodeSingleChoice},
		{6, 7, 10, 0, lesson.NodeMultipleChoice},
		{6, 15, 21, 1, lesson.NodeMultipleChoice},
		{6, 25, 30, 2, lesson.NodeMatching},
		{6, 38, 45, 3, lesson.NodeMatching},
		{7, 9, 12, 0, lesson.NodeOrdering},
		{7, 17, 23, 1, lesson.NodeOrdering},
		{7, 28, 38, 2, lesson.NodeOrdering},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("golden%d/line%d", tc.fixture, tc.first), func(t *testing.T) {
			var historical lesson.Lesson
			exIntegrationRead(t, tc.fixture, "lesson", &historical)
			var exercises []lesson.Node
			for _, page := range historical.Document.Pages {
				exIntegrationExercises(t, page.Nodes, &exercises)
			}
			want := exercises[tc.ordinal]
			if want.Type != tc.kind || want.Flags.Checkable == nil {
				t.Fatal("Неверное сопоставление исторической проекции")
			}
			// Эти data/flags сохраняют силу по ТЗ 10.8–10.9; ID, script
			// и multifill исторического дерева здесь не сравниваются.
			for _, mode := range []string{"create", "update", "add"} {
				t.Run(mode, func(t *testing.T) {
					var input model.Result
					exIntegrationRead(t, tc.fixture, "parsed", &input)
					var projection []model.Line
					for _, line := range input.Lines {
						if line.Number >= tc.first && line.Number <= tc.last {
							projection = append(projection, line)
						}
					}
					if len(projection) != tc.last-tc.first+1 {
						t.Fatal("Неполная проекция fixture")
					}
					before := exIntegrationJSON(t, input)
					context := diagnostic.Context{Path: fmt.Sprintf("../golden/%d. parsed.json", tc.fixture)}
					data, checkable, err := exercisedata.Build(tc.kind, projection, context)
					if err != nil {
						t.Fatalf("Build: %+v", err)
					}
					exIntegrationEqual(t, data, want.Data)
					if checkable != *want.Flags.Checkable {
						t.Fatalf("checkable=%v, ожидается %v", checkable, *want.Flags.Checkable)
					}
					exIntegrationValidate(t, tc.kind, data, checkable)
					if !bytes.Equal(before, exIntegrationJSON(t, input)) {
						t.Fatal("Build изменил исходную модель")
					}
					other, otherCheckable, otherErr := exercisedata.Build(tc.kind, projection, context)
					if otherErr != nil || otherCheckable != checkable {
						t.Fatalf("Повторный Build: %v, %v", otherCheckable, otherErr)
					}
					exIntegrationMutate(t, data)
					exIntegrationEqual(t, other, want.Data)
					if !bytes.Equal(before, exIntegrationJSON(t, input)) {
						t.Fatal("Выходной объект разделяет память с входом")
					}
					// Изменения semantic Value/ParentLine/координат во входе
					// также не должны менять уже выданный второй результат.
					for i := range projection {
						if projection[i].ParentLine != nil {
							*projection[i].ParentLine = 999
						}
						for j := range projection[i].Elements {
							element := &projection[i].Elements[j]
							if element.Value != nil {
								*element.Value = "Изменено"
							}
							element.Start = 999
						}
					}
					exIntegrationEqual(t, other, want.Data)
				})
			}
		})
	}
}

func TestExIntegrationCreateUpdateAddIndependentCandidates(t *testing.T) {
	var input model.Result
	exIntegrationRead(t, 5, "parsed", &input)
	// Известная граница question+answer golden5; контейнер task сюда не входит.
	lines := input.Lines[8:10]
	created, checked, err := exercisedata.Build(lesson.NodeQuestion, lines, diagnostic.Context{Path: "create.json"})
	if err != nil || !checked {
		t.Fatalf("Создание: %v, %v", checked, err)
	}
	original := exIntegrationJSON(t, created)
	text := "Новый вопрос: <b>Ёж</b>?"
	content := &lines[0].Elements[1]
	content.Value = &text
	content.Raw = text
	content.End = content.Start + utf8.RuneCountInString(text)
	lines[0].Raw = "@question " + text
	updated, checked, err := exercisedata.Build(lesson.NodeQuestion, lines, diagnostic.Context{Path: "update.json"})
	if err != nil || !checked || updated[0].Value != text {
		t.Fatalf("Обновление: data=%+v, checkable=%v, err=%v", updated, checked, err)
	}
	if !bytes.Equal(original, exIntegrationJSON(t, created)) {
		t.Fatal("Подготовка обновления изменила прежние данные")
	}
	if !bytes.Equal(exIntegrationJSON(t, updated[0].Answers), exIntegrationJSON(t, created[0].Answers)) {
		t.Fatal("Изменение формулировки изменило ответы")
	}
	// Добавление использует тот же Build; существующий кандидат остаётся прежним.
	var addition model.Result
	exIntegrationRead(t, 5, "parsed", &addition)
	added, checked, err := exercisedata.Build(lesson.NodeQuestion, addition.Lines[8:10], diagnostic.Context{Path: "add.json"})
	if err != nil || !checked {
		t.Fatalf("Добавление: %v, %v", checked, err)
	}
	exIntegrationEqual(t, added, created)
	exIntegrationValidate(t, lesson.NodeQuestion, updated, true)
	exIntegrationMutate(t, added)
	if !bytes.Equal(original, exIntegrationJSON(t, created)) || updated[0].Value != text {
		t.Fatal("Добавленный кандидат разделяет память с прежним или обновлённым")
	}
}

func exIntegrationRead(t *testing.T, number int, suffix string, target any) {
	t.Helper()
	raw, err := os.ReadFile(fmt.Sprintf("../golden/%d. %s.json", number, suffix))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func exIntegrationExercises(t *testing.T, nodes []lesson.Node, output *[]lesson.Node) {
	t.Helper()
	for _, node := range nodes {
		switch node.Type {
		case lesson.NodeQuestion, lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering:
			*output = append(*output, node)
		}
		exIntegrationExercises(t, node.Nodes, output)
	}
}

func exIntegrationJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func exIntegrationEqual(t *testing.T, got, want []lesson.Data) {
	t.Helper()
	// Сериализация Go-моделей задаёт одинаковый порядок ключей;
	// пробелы и исходный порядок ключей golden не участвуют в сравнении.
	if !bytes.Equal(exIntegrationJSON(t, got), exIntegrationJSON(t, want)) {
		t.Fatalf("data=%s\nожидается=%s", exIntegrationJSON(t, got), exIntegrationJSON(t, want))
	}
}

func exIntegrationValidate(t *testing.T, kind lesson.NodeType, data []lesson.Data, checkable bool) {
	t.Helper()
	value := lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "BA83", Version: "1", Title: "Интеграция", LineCount: "1", CreatedAt: "2026-11-06T00:00:00Z",
		Pages: []lesson.Page{{ID: "ba83-p001", Nodes: []lesson.Node{{
			ID: "ba83-n001", Type: kind, Flags: lesson.NodeFlags{Checkable: &checkable}, Data: data, Nodes: []lesson.Node{},
		}}}},
	}}
	if problems := lesson.Validate(&value, diagnostic.Context{Path: "integration.lesson.json"}); len(problems) != 0 {
		t.Fatalf("lesson.Validate: %+v", problems)
	}
	raw := exIntegrationJSON(t, value)
	var restored lesson.Lesson
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if problems := lesson.Validate(&restored, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("lesson.Validate после JSON roundtrip: %+v", problems)
	}
	decoded := restored.Document.Pages[0].Nodes[0]
	if decoded.Flags.Checkable == nil || *decoded.Flags.Checkable != checkable || decoded.Flags.Speaking != nil || decoded.Flags.Stepped != nil {
		t.Fatalf("Неверные flags после JSON roundtrip: %+v", decoded.Flags)
	}
	exIntegrationEqual(t, decoded.Data, data)
	exIntegrationMutate(t, decoded.Data)
	if !bytes.Equal(raw, exIntegrationJSON(t, value)) {
		t.Fatal("JSON roundtrip разделяет изменяемые данные с исходной моделью")
	}
}

func exIntegrationMutate(t *testing.T, data []lesson.Data) {
	t.Helper()
	for i := range data {
		if data[i].Role != nil {
			*data[i].Role = lesson.RoleTitle
		}
		for j := range data[i].Answers {
			data[i].Answers[j] = "Изменённый ответ"
		}
		data[i].Hint = "Изменённая подсказка"
		if children, ok := data[i].Value.([]lesson.Data); ok {
			exIntegrationMutate(t, children)
		} else {
			data[i].Value = "Изменённый текст"
		}
	}
}
