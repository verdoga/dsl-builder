package textdata_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/textdata"
)

func TestDocBuildOwnsIndependentData(t *testing.T) {
	source := docQASource(t, "3")
	declaration, lines := source.Lines[36], source.Lines[37:39]
	before := docQAInput(t, &declaration, lines)
	first := docQABuild(t, lesson.NodeScript, &declaration, lines)
	second := docQABuild(t, lesson.NodeScript, &declaration, lines)
	docQAData(t, first, []lesson.DataRole{lesson.RoleTitle, lesson.RoleBody}, []string{"TITLE", "CONTENT A\nCONTENT B"}, "")
	docQAData(t, second, []lesson.DataRole{lesson.RoleTitle, lesson.RoleBody}, []string{"TITLE", "CONTENT A\nCONTENT B"}, "")
	if !bytes.Equal(before, docQAInput(t, &declaration, lines)) {
		t.Fatal("Build изменил входные строки или элементы")
	}

	// Изменение указателя и append проверяют наблюдаемое владение данными.
	*first[0].Role = lesson.RoleAfter
	if *first[1].Role != lesson.RoleBody || *second[0].Role != lesson.RoleTitle {
		t.Fatal("Role разделяется между элементами либо вызовами")
	}
	first[0].Answers = append(first[0].Answers, "первый")
	first[1].Answers = append(first[1].Answers, "второй")
	second[0].Answers = append(second[0].Answers, "другой вызов")
	if first[0].Answers[0] != "первый" || first[1].Answers[0] != "второй" || len(second[1].Answers) != 0 {
		t.Fatal("Answers разделяется между элементами либо вызовами")
	}
	first[1].Value = "изменённый результат"
	first[1].Hint = "локальная подсказка"
	if second[1].Value != "CONTENT A\nCONTENT B" || second[1].Hint != "" {
		t.Fatal("изменение одного результата затронуло другой")
	}
	if !bytes.Equal(before, docQAInput(t, &declaration, lines)) {
		t.Fatal("изменение результата затронуло вход")
	}
	*declaration.Elements[1].Value = "новое название источника"
	*lines[0].Elements[0].Value = "новый исходный текст"
	if second[0].Value != "TITLE" || second[1].Value != "CONTENT A\nCONTENT B" {
		t.Fatal("готовые данные зависят от дальнейших изменений входа")
	}
}

func TestDocBuildEqualLineNumbersKeepInstructionHintsLocal(t *testing.T) {
	left, right := docQASource(t, "3"), docQASource(t, "3")
	leftDeclaration, rightDeclaration := left.Lines[7], right.Lines[7]
	leftLines, rightLines := left.Lines[8:11], right.Lines[8:11]
	*rightLines[0].Elements[0].Value = "Другая инструкция"
	rightLines[0].Raw, rightLines[0].Elements[0].Raw = "Другая инструкция", "Другая инструкция"
	rightLines[0].Elements[0].End = 1 + len([]rune("Другая инструкция"))
	*rightLines[2].Elements[1].Value = "Другая подсказка"
	rightLines[2].Raw, rightLines[2].Elements[1].Raw = "@hint Другая подсказка", "Другая подсказка"
	rightLines[2].Elements[1].End = rightLines[2].Elements[1].Start + len([]rune("Другая подсказка"))
	leftBefore, rightBefore := docQAInput(t, &leftDeclaration, leftLines), docQAInput(t, &rightDeclaration, rightLines)
	first := docQABuild(t, lesson.NodeInstruction, &leftDeclaration, leftLines)
	second := docQABuild(t, lesson.NodeInstruction, &rightDeclaration, rightLines)
	docQAData(t, first, []lesson.DataRole{lesson.RoleBody}, []string{"INSTRUCTION TEXT A\nINSTRUCTION TEXT B"}, "HINT TEXT")
	docQAData(t, second, []lesson.DataRole{lesson.RoleBody}, []string{"Другая инструкция\nINSTRUCTION TEXT B"}, "Другая подсказка")
	again := docQABuild(t, lesson.NodeInstruction, &leftDeclaration, leftLines)
	docQAData(t, again, []lesson.DataRole{lesson.RoleBody}, []string{"INSTRUCTION TEXT A\nINSTRUCTION TEXT B"}, "HINT TEXT")
	*second[0].Role = lesson.RoleBefore
	second[0].Hint = "изменено"
	if *first[0].Role != lesson.RoleBody || *again[0].Role != lesson.RoleBody || first[0].Hint != "HINT TEXT" {
		t.Fatal("одинаковые номера строк смешали экземпляры включений")
	}
	if !bytes.Equal(leftBefore, docQAInput(t, &leftDeclaration, leftLines)) || !bytes.Equal(rightBefore, docQAInput(t, &rightDeclaration, rightLines)) {
		t.Fatal("Build изменил локальный вход инструкции")
	}
}

func TestDocBuildKeepsMultifillOpaqueAndInstructionSeparate(t *testing.T) {
	source := docQASource(t, "1")
	declaration := source.Lines[7]
	markup := `<table>A | B</table>; один; два; @hint это текст; C:\lesson`
	prefix, placeholder, answer := "Я ", "_____{готов}", "готов"
	lines := []model.Line{
		{Number: 45, LineType: model.LineTypeContent, Raw: markup, Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: markup, Value: &markup, Start: 1, End: len([]rune(markup)) + 1}}},
		{Number: 46, LineType: model.LineTypeBlank, Raw: "\t"},
		{Number: 47, LineType: model.LineTypeContent, Raw: prefix + placeholder, Elements: []model.Element{
			{ElementType: model.ElementTypeContent, Raw: prefix, Value: &prefix, Start: 1, End: 3},
			{ElementType: model.ElementTypePlaceholder, Raw: placeholder, Value: &answer, Start: 3, End: 3 + len([]rune(placeholder))},
		}},
	}
	for i := range lines {
		lines[i].ParentLine = &declaration.Number
		lines[i].NestingLevel = declaration.NestingLevel + 1
		if i < len(lines)-1 {
			lines[i].LineEnding = model.LineEndingLF
		}
	}
	before := docQAInput(t, &declaration, lines)
	body := docQABuild(t, lesson.NodeMultifill, &declaration, lines)
	docQAData(t, body, []lesson.DataRole{lesson.RoleBody}, []string{markup + "\n\n" + prefix + placeholder}, "")
	instruction := docQABuild(t, lesson.NodeInstruction, &declaration, nil)
	docQAData(t, instruction, []lesson.DataRole{lesson.RoleBody}, []string{"Underline the correct words to complete the conversations."}, "")
	if !bytes.Equal(before, docQAInput(t, &declaration, lines)) {
		t.Fatal("непрозрачное содержимое было изменено во входе")
	}
}

func TestDocBuildNoOwnDataIsEmptyArray(t *testing.T) {
	source := docQASource(t, "7")
	for _, tc := range []struct {
		name        string
		kind        lesson.NodeType
		declaration *model.Line
	}{
		{"самостоятельный пустой текст", lesson.NodeText, nil},
		{"контейнер variants", lesson.NodeVariants, &source.Lines[4]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := docQABuild(t, tc.kind, tc.declaration, nil)
			if got == nil || len(got) != 0 {
				t.Fatalf("отсутствие данных = %#v, нужен ненулевой пустой срез", got)
			}
			raw, err := json.Marshal(got)
			if err != nil || string(raw) != "[]" {
				t.Fatalf("JSON отсутствующих данных = %s, ошибка %v", raw, err)
			}
		})
	}
}

func TestDocBuildTechnicalContextDoesNotBecomeScriptData(t *testing.T) {
	source := docQASource(t, "3")
	declaration, lines := source.Lines[36], source.Lines[37:39]
	context := diagnostic.Context{Path: "C:\\секрет\\parsed.json", FileName: "l999.json", LessonID: "TECH", PageID: "tech-p999", Line: 1000, Column: 999}
	got, problem := textdata.Build(lesson.NodeScript, &declaration, lines, context)
	if problem != nil {
		t.Fatal(problem)
	}
	raw, err := json.Marshal(got)
	want := `[{"type":"text","role":"title","value":"TITLE","answers":[],"hint":""},{"type":"text","role":"body","value":"CONTENT A\nCONTENT B","answers":[],"hint":""}]`
	if err != nil || string(raw) != want {
		t.Fatalf("script содержит лишние сведения или не является текстом: %s; ошибка %v", raw, err)
	}
	parent := 700
	declaration.Number, declaration.ParentLine, declaration.NestingLevel = 701, &parent, 1
	for i := range lines {
		lines[i].Number = 702 + i*100
		lines[i].ParentLine = &declaration.Number
		lines[i].LineEnding = model.LineEndingLF
	}
	changed := docQABuild(t, lesson.NodeScript, &declaration, lines)
	changedRaw, err := json.Marshal(changed)
	if err != nil || !bytes.Equal(raw, changedRaw) {
		t.Fatalf("технические координаты изменили собственный текст: %s; ошибка %v", changedRaw, err)
	}
}

func TestDocBuildFailureIsAtomicAndDoesNotMutateInput(t *testing.T) {
	source := docQASource(t, "3")
	declaration, lines := source.Lines[36], source.Lines[37:39]
	lines[1].Elements[0].Value = nil
	before := docQAInput(t, &declaration, lines)
	got, problem := textdata.Build(lesson.NodeScript, &declaration, lines, diagnostic.Context{Path: "технический.json"})
	if got != nil || problem == nil {
		t.Fatalf("нужна одна ошибка без частичного title/body: data=%#v, error=%v", got, problem)
	}
	if problem.Code != textdata.CodeMissingValue || problem.Context.Path != "технический.json" || problem.Context.Line != 39 || problem.Context.Column != 1 || problem.Reason == "" {
		t.Fatalf("неточная локальная диагностика: %#v", problem)
	}
	if !bytes.Equal(before, docQAInput(t, &declaration, lines)) {
		t.Fatal("ошибка Build изменила вход")
	}
	value := "исправленный текст"
	lines[1].Elements[0].Value = &value
	lines[1].Raw, lines[1].Elements[0].Raw = value, value
	lines[1].Elements[0].End = 1 + len([]rune(value))
	recovered := docQABuild(t, lesson.NodeScript, &declaration, lines)
	docQAData(t, recovered, []lesson.DataRole{lesson.RoleTitle, lesson.RoleBody}, []string{"TITLE", "CONTENT A\nисправленный текст"}, "")
}

func docQASource(t *testing.T, number string) model.Result {
	t.Helper()
	raw, err := os.ReadFile("../golden/" + number + ". parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	var source model.Result
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	return source
}

func docQAInput(t *testing.T, declaration *model.Line, lines []model.Line) []byte {
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

func docQABuild(t *testing.T, kind lesson.NodeType, declaration *model.Line, lines []model.Line) []lesson.Data {
	t.Helper()
	got, problem := textdata.Build(kind, declaration, lines, diagnostic.Context{})
	if problem != nil {
		t.Fatalf("Build(%s): %v", kind, problem)
	}
	return got
}

func docQAData(t *testing.T, got []lesson.Data, roles []lesson.DataRole, values []string, hint string) {
	t.Helper()
	if len(got) != len(values) {
		t.Fatalf("получено %d элементов данных, нужно %d: %#v", len(got), len(values), got)
	}
	for i, datum := range got {
		value, isString := datum.Value.(string)
		if datum.Type != lesson.DataText || !isString || value != values[i] || datum.Role == nil || *datum.Role != roles[i] || datum.Answers == nil || len(datum.Answers) != 0 || datum.Hint != hint {
			t.Fatalf("неверные текстовые данные [%d]: %#v", i, datum)
		}
	}
}
