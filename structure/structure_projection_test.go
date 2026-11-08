package structure_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/textdata"
)

func TestTask3LinesPreserveCompleteSourceAndOccurrences(t *testing.T) {
	input, entries := task3Load(t,
		task3Open("fragment", 0, task3Element("identifier", "f", task3Value("f"))),
		task3Open("multifill", 1, task3Element("content", `Реши\@ `, task3Value("Реши@ ")),
			task3Element("placeholder", "_____", nil)),
		task3Line(model.LineTypeContent, 2, task3Element("content", `Текст\{`, task3Value("Текст{")),
			task3Element("placeholder", "_____{А}", task3Value("А"))),
		task3Line(model.LineTypeBlank, 2), task3Close(2), task3Close(1),
		task3Line(model.LineTypeTag, 0, task3Tag("include"), task3Element("identifier", "f", task3Value("f"))),
		task3Line(model.LineTypeTag, 0, task3Tag("include"), task3Element("identifier", "f", task3Value("f"))))
	// Сравнение всех полей включает nullable, Raw, LF/CRLF и координаты.
	// Доступ только для чтения: независимости вложенных указателей не требуем.
	before := task3JSON(t, input)
	first, second := entries[0].Included, entries[1].Included
	firstSources := []model.Line{first[0].Source, first[1].Source, first[2].Source, first[3].Source}
	secondSources := []model.Line{second[0].Source, second[1].Source, second[2].Source, second[3].Source}
	sourceSnapshot := task3JSON(t, firstSources)
	secondSnapshot := task3JSON(t, secondSources)
	pages, bindings := task3Build(t, input, entries)
	if len(bindings) != 4 {
		t.Fatalf("Ожидались instruction/multifill для двух включений: %d", len(bindings))
	}
	for occurrence, sources := range [][]model.Line{firstSources, secondSources} {
		binding := bindings[occurrence*2+1]
		if binding.Node(pages).Type != lesson.NodeMultifill || !bytes.Equal(task3JSON(t, binding.Lines()), task3JSON(t, sources)) {
			t.Fatalf("Полная проекция включения %d потеряна: %+v", occurrence, binding.Lines())
		}
		for i, entry := range binding.Entries {
			want := first[i]
			if occurrence == 1 {
				want = second[i]
			}
			if entry != want {
				t.Fatalf("Подменён экземпляр %d, строка %d", occurrence, i)
			}
		}
	}
	if first[0] == second[0] || first[0].Source.Number != second[0].Source.Number || first[0].Parent != entries[0] || second[0].Parent != entries[1] {
		t.Fatal("Утрачен адрес конкретного вхождения include")
	}
	if !bytes.Equal(sourceSnapshot, task3JSON(t, bindings[1].Lines())) || !bytes.Equal(secondSnapshot, task3JSON(t, bindings[3].Lines())) || !bytes.Equal(before, task3JSON(t, input)) {
		t.Fatal("Build/Lines изменил исходные значения")
	}
	if bindings[1].Lines()[0].Elements[2].Value != nil {
		t.Fatal("null placeholder превратился в строку")
	}
}

func TestTask3TextSourcePreservesNestedHintAndOwnBoundaries(t *testing.T) {
	input, entries := task3Load(t,
		task3Open("instr", 0),
		task3Line(model.LineTypeContent, 1, task3Text("Первая")),
		task3Open("hint", 1),
		task3Line(model.LineTypeContent, 3, task3Text("Подсказка")), task3Close(3),
		task3Line(model.LineTypeContent, 1, task3Text("Последняя")), task3Close(1),
		task3Line(model.LineTypeContent, 0, task3Element("content", `\@не тег`, task3Value("@не тег"))),
		task3Line(model.LineTypeTag, 0, task3Tag("note"), task3Text("Заметка")))
	before := task3JSON(t, input)
	pages, bindings := task3Build(t, input, entries)
	if len(bindings) != 3 {
		t.Fatalf("Получено %d привязок вместо трёх", len(bindings))
	}
	for i, binding := range bindings {
		declaration, lines := binding.TextSource()
		wantBody := []model.Line{}
		switch i {
		case 0:
			for _, entry := range entries[1:6] {
				wantBody = append(wantBody, entry.Source)
			}
			if declaration == nil || !bytes.Equal(task3JSON(t, declaration), task3JSON(t, entries[0].Source)) {
				t.Fatal("Потеряна декларация instr")
			}
		case 1:
			wantBody = append(wantBody, entries[7].Source)
			if declaration != nil {
				t.Fatal("Самостоятельный text получил декларацию")
			}
		case 2:
			if declaration == nil || declaration.Number != 9 {
				t.Fatal("Потеряна однострочная декларация")
			}
		}
		if len(lines) != len(wantBody) || len(lines) > 0 && !bytes.Equal(task3JSON(t, lines), task3JSON(t, wantBody)) {
			t.Fatalf("Тело TextSource[%d]=%+v", i, lines)
		}
		data, problem := textdata.Build(binding.Node(pages).Type, declaration, lines, diagnostic.Context{Path: input.Path})
		if problem != nil || len(data) != 1 || data[0].Role == nil || *data[0].Role != lesson.RoleBody {
			t.Fatalf("textdata[%d]: data=%+v error=%v", i, data, problem)
		}
		wantText := []string{"Первая\nПоследняя", "@не тег", "Заметка"}[i]
		if data[0].Value != wantText || i == 0 && data[0].Hint != "Подсказка" {
			t.Fatalf("textdata[%d]=%+v", i, data)
		}
	}
	if !bytes.Equal(before, task3JSON(t, input)) {
		t.Fatal("TextSource/textdata изменил Input")
	}
}

func TestTask3SyntheticInstructionTextSourceUsesOnlyDeclaration(t *testing.T) {
	for _, tag := range []string{"multifill", "ordering"} {
		t.Run(tag, func(t *testing.T) {
			input, entries := task3Load(t, task3Open(tag, 0, task3Text("Инструкция")),
				task3Line(model.LineTypeContent, 1, task3Text("Содержимое")), task3Close(1))
			pages, bindings := task3Build(t, input, entries)
			if len(bindings) != 2 || bindings[0].Node(pages).Type != lesson.NodeInstruction || bindings[0].Page != bindings[1].Page || !slices.Equal(bindings[0].NodePath[:len(bindings[0].NodePath)-1], bindings[1].NodePath[:len(bindings[1].NodePath)-1]) {
				t.Fatalf("Выделенная инструкция: %+v", bindings)
			}
			if len(bindings[0].Entries) != 1 || bindings[0].Entries[0] != entries[0] {
				t.Fatal("Инструкция получила тело упражнения")
			}
			declaration, lines := bindings[0].TextSource()
			if declaration == nil || declaration.LineType != model.LineTypeBlockStart || len(lines) != 0 {
				t.Fatalf("TextSource выделенной инструкции: %+v %+v", declaration, lines)
			}
			data, problem := textdata.Build(lesson.NodeInstruction, declaration, lines, diagnostic.Context{Path: input.Path})
			if problem != nil || len(data) != 1 || data[0].Value != "Инструкция" {
				t.Fatalf("textdata выделенной инструкции: %+v %v", data, problem)
			}
		})
	}
}

func TestTask3LinesFeedRealExerciseConsumers(t *testing.T) {
	cases := []struct {
		tag  string
		kind lesson.NodeType
	}{
		{"question", lesson.NodeQuestion}, {"choice", lesson.NodeSingleChoice},
		{"multichoice", lesson.NodeMultipleChoice}, {"matching", lesson.NodeMatching}, {"ordering", lesson.NodeOrdering},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			var lines []model.Line
			switch tc.tag {
			case "question":
				lines = []model.Line{task3Line(model.LineTypeTag, 0, task3Tag("question"), task3Text("Вопрос")),
					task3Open("answer", 0), task3Line(model.LineTypeContent, 2, task3Text("A")), task3Close(2),
					task3Open("hint", 0), task3Line(model.LineTypeContent, 5, task3Text("H")), task3Close(5)}
			case "choice", "multichoice":
				lines = []model.Line{task3Open(tc.tag, 0, task3Text("Инструкция")),
					task3Line(model.LineTypeContent, 1, task3Text("A")),
					task3Line(model.LineTypeTag, 1, task3Tag("answer"), task3Text("B")),
					task3Line(model.LineTypeTag, 1, task3Tag("hint"), task3Text("H")), task3Close(1)}
			case "matching":
				lines = []model.Line{task3Open(tc.tag, 0, task3Text("  Инструкция <b>сопоставь</b>!  ")),
					task3Line(model.LineTypeContent, 1, task3Text("Элемент")),
					task3Line(model.LineTypeTag, 1, task3Tag("answer"), task3Text("A")),
					task3Line(model.LineTypeTag, 1, task3Tag("hint"), task3Text("H")),
					task3Line(model.LineTypeSeparator, 1), task3Line(model.LineTypeContent, 1, task3Text("A")),
					task3Line(model.LineTypeContent, 1, task3Text("B")), task3Close(1)}
				lines[4].Raw = "---"
			case "ordering":
				lines = []model.Line{task3Open(tc.tag, 0, task3Text("Инструкция")),
					task3Line(model.LineTypeContent, 1, task3Text("A")),
					task3Line(model.LineTypeTag, 1, task3Tag("answer"), task3Text("2")),
					task3Line(model.LineTypeContent, 1, task3Text("B")),
					task3Line(model.LineTypeTag, 1, task3Tag("answer"), task3Text("1")),
					task3Line(model.LineTypeTag, 1, task3Tag("hint"), task3Text("H")), task3Close(1)}
			}
			input, entries := task3Load(t, lines...)
			pages, bindings := task3Build(t, input, entries)
			found := false
			for index, binding := range bindings {
				if binding.Node(pages).Type != tc.kind {
					continue
				}
				found = true
				if tc.tag == "matching" {
					if len(bindings) != 2 || index != 1 {
						t.Fatalf("Ожидались инструкция непосредственно перед matching: %+v", bindings)
					}
					instruction := bindings[index-1]
					if instruction.Node(pages).Type != lesson.NodeInstruction || instruction.Page != binding.Page ||
						!slices.Equal(instruction.NodePath[:len(instruction.NodePath)-1], binding.NodePath[:len(binding.NodePath)-1]) ||
						instruction.NodePath[len(instruction.NodePath)-1]+1 != binding.NodePath[len(binding.NodePath)-1] {
						t.Fatalf("Инструкция и matching не соседствуют у одного родителя: %+v", bindings)
					}
					if len(instruction.Entries) != 1 || instruction.Entries[0] != entries[0] ||
						!bytes.Equal(task3JSON(t, instruction.Lines()), task3JSON(t, input.Result.Lines[:1])) {
						t.Fatal("Инструкция matching получила строки за пределами объявления")
					}
					declaration, ownLines := instruction.TextSource()
					if declaration == nil || !bytes.Equal(task3JSON(t, declaration), task3JSON(t, entries[0].Source)) || len(ownLines) != 0 {
						t.Fatalf("TextSource инструкции matching: %+v %+v", declaration, ownLines)
					}
					body, problem := textdata.Build(lesson.NodeInstruction, declaration, ownLines, diagnostic.Context{Path: input.Path})
					if problem != nil || len(body) != 1 || body[0].Type != lesson.DataText || body[0].Role == nil || *body[0].Role != lesson.RoleBody ||
						body[0].Value != "  Инструкция <b>сопоставь</b>!  " || len(body[0].Answers) != 0 || body[0].Hint != "" {
						t.Fatalf("textdata инструкции matching: %+v %v", body, problem)
					}
				}
				projected := binding.Lines()
				if len(projected) != len(lines) || !bytes.Equal(task3JSON(t, projected), task3JSON(t, input.Result.Lines)) {
					t.Fatalf("Упражнение получило неполную проекцию: %+v", projected)
				}
				before := task3JSON(t, projected)
				data, checkable, problem := exercisedata.Build(tc.kind, projected, diagnostic.Context{Path: input.Path})
				if problem != nil || !checkable || len(data) == 0 {
					t.Fatalf("exercisedata: data=%+v checkable=%v error=%v", data, checkable, problem)
				}
				if !bytes.Equal(before, task3JSON(t, binding.Lines())) {
					t.Fatal("Потребитель изменил проекцию")
				}
				if tc.tag == "question" && (data[0].Value != "Вопрос" || !slices.Equal(data[0].Answers, []string{"A"}) || data[0].Hint != "H") {
					t.Fatalf("Вопрос потерял answer/hint: %+v", data)
				}
				if tc.tag == "matching" {
					if len(data) != 2 || data[0].Type != lesson.DataCollection || data[0].Role == nil || *data[0].Role != lesson.RoleItems ||
						data[1].Type != lesson.DataCollection || data[1].Role == nil || *data[1].Role != lesson.RoleOptions {
						t.Fatalf("matching потерял items/options: %+v", data)
					}
					items, itemsOK := data[0].Value.([]lesson.Data)
					options, optionsOK := data[1].Value.([]lesson.Data)
					if !itemsOK || len(items) != 1 || items[0].Type != lesson.DataText || items[0].Role == nil || *items[0].Role != lesson.RoleItem ||
						items[0].Value != "Элемент" || !slices.Equal(items[0].Answers, []string{"A"}) || items[0].Hint != "H" || !optionsOK || len(options) != 2 {
						t.Fatalf("matching потерял элементы/ответы/подсказки: %+v", data)
					}
					for i, want := range []string{"A", "B"} {
						if options[i].Type != lesson.DataText || options[i].Role == nil || *options[i].Role != lesson.RoleOption ||
							options[i].Value != want || len(options[i].Answers) != 0 || options[i].Hint != "" {
							t.Fatalf("matching option[%d]=%+v", i, options[i])
						}
					}
				}
			}
			if !found {
				t.Fatal("Привязка упражнения отсутствует")
			}
		})
	}
}
