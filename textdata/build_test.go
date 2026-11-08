package textdata

import (
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func buildQAElement(kind model.ElementType, value string) model.Element {
	return model.Element{ElementType: kind, Raw: value, Value: &value}
}

func buildQALine(number int, kind model.LineType, elements ...model.Element) model.Line {
	line := model.Line{Number: number, LineType: kind, Elements: elements}
	for i := range line.Elements {
		if i > 0 {
			line.Raw += " "
		}
		line.Elements[i].Start = len([]rune(line.Raw)) + 1
		line.Raw += line.Elements[i].Raw
		line.Elements[i].End = len([]rune(line.Raw)) + 1
	}
	return line
}

func buildQADeclaration(tag string, kind model.LineType, extra ...model.Element) model.Line {
	elements := []model.Element{buildQAElement(model.ElementTypeTag, tag)}
	elements[0].Raw = "@" + tag
	elements = append(elements, extra...)
	if kind == model.LineTypeBlockStart {
		elements = append(elements, buildQAElement(model.ElementTypeBlockOpen, "{"))
	}
	return buildQALine(41, kind, elements...)
}

func buildQAData(t *testing.T, got []lesson.Data, roles []lesson.DataRole, values []string) {
	t.Helper()
	if got == nil || len(got) != len(values) {
		t.Fatalf("data = %#v, want %d nonnil entries", got, len(values))
	}
	for i, d := range got {
		if d.Type != lesson.DataText || d.Role == nil || *d.Role != roles[i] || d.Value != values[i] || d.Answers == nil || len(d.Answers) != 0 || d.Hint != "" {
			t.Errorf("data[%d] = %#v, want role=%s value=%q, empty answers and hint", i, d, roles[i], values[i])
		}
	}
}

func buildQAError(t *testing.T, err *diagnostic.Error, code string) {
	t.Helper()
	if err == nil || err.Code != code || err.Reason == "" {
		t.Fatalf("error = %#v, want %s", err, code)
	}
}

func TestBuildQARoutes(t *testing.T) {
	cases := []struct {
		kind  lesson.NodeType
		tag   string
		field model.ElementType
		title string
		body  bool
	}{
		{lesson.NodeTask, "task", model.ElementTypeIdentifier, "Ab-007", false},
		{lesson.NodeStep, "step", model.ElementTypeTitle, "Шаг", false},
		{lesson.NodeVariants, "variants", "", "", false},
		{lesson.NodeVariant, "variant", model.ElementTypeName, "Ветка", false},
		{lesson.NodeHeading, "header", model.ElementTypeTitle, "Заголовок", false},
		{lesson.NodeText, "text", model.ElementTypeTitle, "Текст", true},
		{lesson.NodeInstruction, "instr", "", "", true},
		{lesson.NodeNote, "note", model.ElementTypeTitle, "Примечание", true},
		{lesson.NodeAlt, "alt", model.ElementTypeTitle, "Альтернатива", true},
		{lesson.NodeScript, "script", model.ElementTypeTitle, "Сценарий", true},
		{lesson.NodeHint, "hint", "", "", true},
		{lesson.NodeAnswerKey, "key", model.ElementTypeTitle, "Ключ", true},
		{lesson.NodeExample, "example", model.ElementTypeTitle, "Пример", true},
		{lesson.NodeWordList, "wordlist", model.ElementTypeTitle, "Слова", true},
		{lesson.NodeTable, "table", model.ElementTypeTitle, "Таблица", true},
		{lesson.NodeMultifill, "multifill", "", "", true},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			lt := model.LineTypeTag
			if tc.body || tc.kind == lesson.NodeVariants {
				lt = model.LineTypeBlockStart
			}
			var fields []model.Element
			if tc.field != "" {
				fields = append(fields, buildQAElement(tc.field, tc.title))
			}
			dec := buildQADeclaration(tc.tag, lt, fields...)
			var lines []model.Line
			var roles []lesson.DataRole
			var values []string
			if tc.field != "" {
				roles = append(roles, lesson.RoleTitle)
				values = append(values, tc.title)
			}
			if tc.body {
				lines = []model.Line{buildQALine(43, model.LineTypeContent, buildQAElement(model.ElementTypeContent, "Один")), buildQALine(900, model.LineTypeContent, buildQAElement(model.ElementTypeContent, "Два"))}
				for i := range lines {
					lines[i].ParentLine = &dec.Number
					lines[i].NestingLevel = 1
				}
				roles = append(roles, lesson.RoleBody)
				values = append(values, "Один\nДва")
			}
			ctx := diagnostic.Context{Path: "технический.json"}
			if err := checkInput(tc.kind, &dec, lines, ctx); err != nil {
				t.Fatalf("checkInput: %v", err)
			}
			got, err := Build(tc.kind, &dec, lines, ctx)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			buildQAData(t, got, roles, values)
		})
	}
}

func TestBuildQAInputBoundary(t *testing.T) {
	ctx := diagnostic.Context{Path: "вход.json", LessonID: "X", PageID: "x-p001"}
	body := []model.Line{buildQALine(52, model.LineTypeContent, buildQAElement(model.ElementTypeContent, "Тело"))}
	supported := []lesson.NodeType{lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants, lesson.NodeVariant, lesson.NodeHeading, lesson.NodeText, lesson.NodeInstruction, lesson.NodeNote, lesson.NodeAlt, lesson.NodeScript, lesson.NodeHint, lesson.NodeAnswerKey, lesson.NodeExample, lesson.NodeWordList, lesson.NodeTable, lesson.NodeMultifill}
	for _, kind := range supported {
		t.Run("nil/"+string(kind), func(t *testing.T) {
			if kind == lesson.NodeText {
				if err := checkInput(kind, nil, body, ctx); err != nil {
					t.Fatal(err)
				}
				got, err := Build(kind, nil, body, ctx)
				if err != nil {
					t.Fatal(err)
				}
				buildQAData(t, got, []lesson.DataRole{lesson.RoleBody}, []string{"Тело"})
				return
			}
			buildQAError(t, checkInput(kind, nil, nil, ctx), CodeInput)
			got, err := Build(kind, nil, nil, ctx)
			buildQAError(t, err, CodeInput)
			if got != nil {
				t.Fatalf("partial result: %#v", got)
			}
		})
	}
	for _, kind := range []lesson.NodeType{lesson.NodeMedia, lesson.NodeQuestion, lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering, "", "UNKNOWN"} {
		t.Run("unsupported/"+string(kind), func(t *testing.T) {
			buildQAError(t, checkInput(kind, nil, nil, ctx), CodeUnsupportedType)
			got, err := Build(kind, nil, nil, ctx)
			buildQAError(t, err, CodeUnsupportedType)
			if got != nil || err.Context != ctx {
				t.Fatalf("got %#v, context %#v", got, err.Context)
			}
		})
	}
	for _, tag := range []string{"instr", "multifill", "ordering", "matching"} {
		t.Run("instruction/"+tag, func(t *testing.T) {
			dec := buildQADeclaration(tag, model.LineTypeTag, buildQAElement(model.ElementTypeContent, "Делай"))
			if err := checkInput(lesson.NodeInstruction, &dec, nil, ctx); err != nil {
				t.Fatal(err)
			}
			got, err := Build(lesson.NodeInstruction, &dec, nil, ctx)
			if err != nil {
				t.Fatal(err)
			}
			buildQAData(t, got, []lesson.DataRole{lesson.RoleBody}, []string{"Делай"})
		})
	}
}

func TestBuildQARejectsLocalShape(t *testing.T) {
	text := buildQADeclaration("text", model.LineTypeBlockStart, buildQAElement(model.ElementTypeTitle, "Уже готовый title"))
	task := buildQADeclaration("task", model.LineTypeTag, buildQAElement(model.ElementTypeIdentifier, "1"))
	inline := buildQADeclaration("text", model.LineTypeTag, buildQAElement(model.ElementTypeContent, "inline"))
	ordering := buildQADeclaration("ordering", model.LineTypeBlockStart)
	matching := buildQADeclaration("matching", model.LineTypeBlockStart)
	body := buildQALine(77, model.LineTypeContent, buildQAElement(model.ElementTypeContent, "body"))
	foreign := buildQADeclaration("question", model.LineTypeTag)
	for _, tc := range []struct {
		name  string
		kind  lesson.NodeType
		dec   *model.Line
		lines []model.Line
	}{
		{"wrong declaration", lesson.NodeNote, &text, nil},
		{"title-only body", lesson.NodeTask, &task, []model.Line{body}},
		{"foreign own tag", lesson.NodeText, &text, []model.Line{foreign}},
		{"inline body", lesson.NodeText, &inline, []model.Line{body}},
		{"ordering instruction body", lesson.NodeInstruction, &ordering, []model.Line{body}},
		{"matching instruction body", lesson.NodeInstruction, &matching, []model.Line{body}},
		{"nil text foreign tag", lesson.NodeText, nil, []model.Line{foreign}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := diagnostic.Context{Path: "original.json"}
			buildQAError(t, checkInput(tc.kind, tc.dec, tc.lines, ctx), CodeInput)
			got, err := Build(tc.kind, tc.dec, tc.lines, ctx)
			buildQAError(t, err, CodeInput)
			if got != nil || err.Context.Path != ctx.Path {
				t.Fatalf("partial=%#v context=%#v", got, err.Context)
			}
		})
	}
}

func TestBuildQAPropagatesErrorsWithoutTitle(t *testing.T) {
	dec := buildQADeclaration("text", model.LineTypeBlockStart, buildQAElement(model.ElementTypeTitle, "Название"))
	bad := buildQALine(82, model.LineTypeContent, buildQAElement(model.ElementTypeContent, "неизвестно"))
	bad.Elements[0].Value = nil
	got, err := Build(lesson.NodeText, &dec, []model.Line{bad}, diagnostic.Context{Path: "текст.json"})
	buildQAError(t, err, CodeMissingValue)
	if got != nil || err.Context.Line != 82 || err.Context.Column != 1 {
		t.Fatalf("partial=%#v error=%#v", got, err)
	}
	example := buildQADeclaration("example", model.LineTypeBlockStart, buildQAElement(model.ElementTypeTitle, "Название"))
	sep := model.Line{Number: 83, LineType: model.LineTypeSeparator, Raw: "---"}
	got, err = Build(lesson.NodeExample, &example, []model.Line{sep}, diagnostic.Context{})
	buildQAError(t, err, CodeExampleSeparator)
	if got != nil {
		t.Fatalf("partial=%#v", got)
	}
}

func TestBuildQASeparateInstructionAndMultifill(t *testing.T) {
	dec := buildQADeclaration("multifill", model.LineTypeBlockStart, buildQAElement(model.ElementTypeContent, "Инструкция"))
	body := []model.Line{buildQALine(43, model.LineTypeContent, buildQAElement(model.ElementTypeContent, "Тело"))}
	got, err := Build(lesson.NodeMultifill, &dec, body, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	buildQAData(t, got, []lesson.DataRole{lesson.RoleBody}, []string{"Тело"})
	dec = buildQADeclaration("multifill", model.LineTypeTag, buildQAElement(model.ElementTypeContent, "Инструкция"))
	got, err = Build(lesson.NodeMultifill, &dec, nil, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	buildQAData(t, got, []lesson.DataRole{lesson.RoleBody}, []string{""})
	for _, tag := range []string{"multifill", "ordering", "matching"} {
		dec = buildQADeclaration(tag, model.LineTypeBlockStart)
		got, err = Build(lesson.NodeInstruction, &dec, nil, diagnostic.Context{})
		if err != nil {
			t.Fatal(err)
		}
		buildQAData(t, got, nil, nil)
		buildQAError(t, checkInput(lesson.NodeInstruction, &dec, body, diagnostic.Context{}), CodeInput)
	}
}

func TestBuildQARejectsEveryTitleOnlyBody(t *testing.T) {
	for _, tag := range []string{"task", "step", "header", "variant", "variants"} {
		t.Run(tag, func(t *testing.T) {
			kind := lesson.NodeType(tag)
			if tag == "header" {
				kind = lesson.NodeHeading
			}
			dec := buildQADeclaration(tag, model.LineTypeTag)
			body := []model.Line{buildQALine(42, model.LineTypeBlank)}
			buildQAError(t, checkInput(kind, &dec, body, diagnostic.Context{}), CodeInput)
			got, err := Build(kind, &dec, body, diagnostic.Context{})
			buildQAError(t, err, CodeInput)
			if got != nil {
				t.Fatalf("partial=%#v", got)
			}
		})
	}
}
