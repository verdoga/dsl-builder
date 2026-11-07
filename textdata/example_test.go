package textdata

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exampleQAElement(kind model.ElementType, raw, value string, start int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start,
		End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}}
}

func exampleQALine(number int, raw, value string) model.Line {
	parent := 10
	return model.Line{Number: number, LineType: model.LineTypeContent, Raw: raw,
		NestingLevel: 1, ParentLine: &parent, LineEnding: model.LineEndingCRLF,
		Elements: []model.Element{exampleQAElement(model.ElementTypeContent, raw, value, 1)}}
}

func exampleQABlank(number int) model.Line {
	line := exampleQALine(number, "", "")
	line.LineType, line.Elements = model.LineTypeBlank, []model.Element{}
	return line
}

func exampleQADeclaration(block bool, text string) model.Line {
	line := model.Line{Number: 10, LineType: model.LineTypeTag, Raw: "@example",
		LineEnding: model.LineEndingLF,
		Elements:   []model.Element{exampleQAElement(model.ElementTypeTag, "@example", "example", 1)}}
	if block {
		line.LineType = model.LineTypeBlockStart
		if text != "" {
			line.Raw += " " + text
			line.Elements = append(line.Elements, exampleQAElement(model.ElementTypeTitle, text, text, 10))
		}
		start := utf8.RuneCountInString(line.Raw) + 2
		line.Raw += " {"
		line.Elements = append(line.Elements, exampleQAElement(model.ElementTypeBlockOpen, "{", "{", start))
	} else {
		line.Raw += " " + text
		line.Elements = append(line.Elements, exampleQAElement(model.ElementTypeContent, text, text, 10))
	}
	return line
}

func exampleQASnapshot(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func exampleQAData(t *testing.T, got []lesson.Data, roles []lesson.DataRole, values []string) {
	t.Helper()
	if got == nil || len(got) != len(roles) {
		t.Fatalf("data = %#v, want %d elements in non-nil slice", got, len(roles))
	}
	for i, datum := range got {
		value, ok := datum.Value.(string)
		if datum.Type != lesson.DataText || datum.Role == nil || *datum.Role != roles[i] || !ok || value != values[i] {
			t.Errorf("data[%d] = %#v, want text/%s/%q", i, datum, roles[i], values[i])
		}
		if datum.Answers == nil || len(datum.Answers) != 0 || datum.Hint != "" {
			t.Errorf("data[%d] has answers/hint: %#v", i, datum)
		}
	}
	value := lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EX14", Version: json.Number("1"), Title: "Пример", LineCount: json.Number("100"),
		CreatedAt: "2026-11-06T00:00:00Z", Pages: []lesson.Page{{ID: "ex14-p001", Nodes: []lesson.Node{{
			ID: "ex14-n001", Type: lesson.NodeExample, Data: got, Nodes: []lesson.Node{},
		}}}},
	}}
	if problems := lesson.Validate(&value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatalf("real lesson.Validate rejected example: %v", problems)
	}
}

func TestExampleQABuildForms(t *testing.T) {
	for _, text := range []string{"---", "A --- B", "A -> B → C", ""} {
		t.Run("inline/"+text, func(t *testing.T) {
			got, err := buildExample(exampleQADeclaration(false, text), nil, diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			exampleQAData(t, got, []lesson.DataRole{lesson.RoleBody}, []string{text})
		})
	}
	for _, tc := range []struct {
		name  string
		title string
		lines []model.Line
		roles []lesson.DataRole
		texts []string
	}{
		{"body", "Заголовок", []model.Line{exampleQABlank(11), exampleQALine(12, `\@hint`, "@hint"), exampleQABlank(13), exampleQALine(20, "<br> → ---", "<br> → ---")}, []lesson.DataRole{lesson.RoleTitle, lesson.RoleBody}, []string{"Заголовок", "@hint\n<br> → ---"}},
		{"split", "Заголовок", []model.Line{exampleQALine(11, "До", "До"), exampleQABlank(12), exampleQALine(13, "---", "---"), exampleQABlank(14), exampleQALine(21, "После", "После")}, []lesson.DataRole{lesson.RoleTitle, lesson.RoleBefore, lesson.RoleAfter}, []string{"Заголовок", "До", "После"}},
		{"normative-split", "", []model.Line{exampleQALine(11, "До", "До"), exampleQANormative(12), exampleQALine(13, "После", "После")}, []lesson.DataRole{lesson.RoleBefore, lesson.RoleAfter}, []string{"До", "После"}},
		{"escaped-separator-is-body", "", []model.Line{exampleQALine(11, "До", "До"), exampleQALine(12, `\---`, "---"), exampleQALine(13, "После", "После")}, []lesson.DataRole{lesson.RoleBody}, []string{"До\n---\nПосле"}},
		{"empty", "", nil, []lesson.DataRole{}, []string{}},
		{"title-only", "Заголовок", []model.Line{exampleQABlank(11)}, []lesson.DataRole{lesson.RoleTitle}, []string{"Заголовок"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildExample(exampleQADeclaration(true, tc.title), tc.lines, diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			exampleQAData(t, got, tc.roles, tc.texts)
		})
	}
}

func TestExampleQAIndependentDataAndUnchangedInput(t *testing.T) {
	decl := exampleQADeclaration(true, "Исходное название")
	lines := []model.Line{exampleQALine(11, "До", "До"), exampleQALine(12, "---", "---"), exampleQALine(13, "После", "После")}
	beforeDecl, beforeLines := exampleQASnapshot(t, decl), exampleQASnapshot(t, lines)
	first, err := buildExample(decl, lines, diagnostic.Context{Path: "вход.json"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildExample(decl, lines, diagnostic.Context{Path: "другой.json"})
	if err != nil {
		t.Fatal(err)
	}
	exampleQAData(t, first, []lesson.DataRole{lesson.RoleTitle, lesson.RoleBefore, lesson.RoleAfter}, []string{"Исходное название", "До", "После"})
	for i := range first {
		for j := range second {
			if first[i].Role == second[j].Role || (i != j && first[i].Role == first[j].Role) {
				t.Fatal("Role pointers alias between data or calls")
			}
		}
		*first[i].Role, first[i].Value, first[i].Hint = lesson.RoleBody, "изменено", "изменено"
		first[i].Answers = append(first[i].Answers, "изменено")
	}
	exampleQAData(t, second, []lesson.DataRole{lesson.RoleTitle, lesson.RoleBefore, lesson.RoleAfter}, []string{"Исходное название", "До", "После"})
	if !bytes.Equal(beforeDecl, exampleQASnapshot(t, decl)) || !bytes.Equal(beforeLines, exampleQASnapshot(t, lines)) {
		t.Fatal("buildExample changed input")
	}
}

func TestExampleQAMissingValueHasNoPartialData(t *testing.T) {
	for _, inTitle := range []bool{true, false} {
		t.Run(map[bool]string{true: "title", false: "body"}[inTitle], func(t *testing.T) {
			decl := exampleQADeclaration(true, "Название")
			lines := []model.Line{exampleQALine(11, "Текст", "Текст")}
			if inTitle {
				decl.Elements[1].Value = nil
			} else {
				lines[0].Elements[0].Value = nil
			}
			declBefore, linesBefore := exampleQASnapshot(t, decl), exampleQASnapshot(t, lines)
			got, err := buildExample(decl, lines, diagnostic.Context{Path: "вход.json", LessonID: "EX14"})
			if got != nil || err == nil || err.Code != CodeMissingValue {
				t.Fatalf("got %#v, %v; want nil + CodeMissingValue", got, err)
			}
			if err.Context.Path != "вход.json" || err.Context.LessonID != "EX14" {
				t.Fatalf("lost diagnostic context: %#v", err.Context)
			}
			wantLine, wantColumn := 11, 1
			if inTitle {
				wantLine, wantColumn = 10, 10
			}
			if err.Context.Line != wantLine || err.Context.Column != wantColumn {
				t.Errorf("coordinates = %#v, want %d:%d", err.Context, wantLine, wantColumn)
			}
			if !bytes.Equal(declBefore, exampleQASnapshot(t, decl)) || !bytes.Equal(linesBefore, exampleQASnapshot(t, lines)) {
				t.Fatal("error changed input")
			}
		})
	}
}

func TestExampleQAGoldenInputsAndData(t *testing.T) {
	for _, number := range []string{"2", "3"} {
		t.Run(number, func(t *testing.T) {
			parsedBytes, err := os.ReadFile("../golden/" + number + ". parsed.json")
			if err != nil {
				t.Fatal(err)
			}
			lessonBytes, err := os.ReadFile("../golden/" + number + ". lesson.json")
			if err != nil {
				t.Fatal(err)
			}
			var source model.Result
			var expected lesson.Lesson
			if err := json.Unmarshal(parsedBytes, &source); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(lessonBytes, &expected); err != nil {
				t.Fatal(err)
			}
			// Явные локальные срезы: inline 2:11 и блок 3:13 с телом 14–16.
			decl, lines := source.Lines[10], []model.Line(nil)
			var want []lesson.Data
			if number == "3" {
				decl, lines = source.Lines[12], source.Lines[13:16]
				want = expected.Document.Pages[0].Nodes[1].Nodes[1].Data
				if !isExampleSeparator(source.Lines[14]) {
					t.Fatal("golden 3:15 must be recognized as legacy separator")
				}
			} else {
				want = expected.Document.Pages[0].Nodes[0].Nodes[1].Data
			}
			before := exampleQASnapshot(t, source)
			got, problem := buildExample(decl, lines, diagnostic.Context{Path: number + ". parsed.json"})
			if problem != nil {
				t.Fatal(problem)
			}
			if !bytes.Equal(exampleQASnapshot(t, got), exampleQASnapshot(t, want)) {
				t.Fatalf("data differs: got %#v, want %#v", got, want)
			}
			if !bytes.Equal(before, exampleQASnapshot(t, source)) {
				t.Fatal("golden input changed")
			}
		})
	}
}
