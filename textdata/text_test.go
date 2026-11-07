package textdata

import (
	"encoding/json"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func textQAPtr(value string) *string { return &value }

func textQAElement(kind model.ElementType, raw, value string, start int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: textQAPtr(value), Start: start, End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}}
}

func textQAContent(number int, value string) model.Line {
	return model.Line{Number: number, LineType: model.LineTypeContent, Raw: value, LineEnding: model.LineEndingLF,
		Elements: []model.Element{textQAElement(model.ElementTypeContent, value, value, 1)}}
}

func textQAAssertDatum(t *testing.T, got lesson.Data, role lesson.DataRole, value, hint string) {
	t.Helper()
	if got.Type != lesson.DataText || got.Role == nil || *got.Role != role {
		t.Fatalf("тип/роль Data = %#v, нужна text/%s", got, role)
	}
	actual, ok := got.Value.(string)
	if !ok || actual != value || got.Hint != hint || got.Answers == nil || len(got.Answers) != 0 {
		t.Fatalf("Data = %#v, нужны Value=%q Hint=%q Answers=[]", got, value, hint)
	}
}

func textQAAssertError(t *testing.T, got *diagnostic.Error, code string, line int) {
	t.Helper()
	if got == nil || got.Code != code || got.Reason == "" || got.Context.Path != "input/урок.parsed.json" || got.Context.Line != line {
		t.Fatalf("диагностика = %#v, нужны %s и строка %d", got, code, line)
	}
}

func textQAContext() diagnostic.Context {
	return diagnostic.Context{Path: "input/урок.parsed.json", LessonID: "Test", PageID: "test-p001"}
}

func textQAGolden(t *testing.T, name string) model.Result {
	t.Helper()
	raw, err := os.ReadFile("../golden/" + name + ". parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	var result model.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestTextQABlockGoldenOwnData(t *testing.T) {
	cases := []struct {
		name, golden             string
		kind                     lesson.NodeType
		declaration, first, last int
		title, body              string
	}{
		{"inline-alt", "2", lesson.NodeAlt, 12, 0, 0, "", "ALT TEXT"},
		{"note", "2", lesson.NodeNote, 16, 17, 18, "TITLE", "CONTENT A\nCONTENT B"},
		{"script", "3", lesson.NodeScript, 37, 38, 39, "TITLE", "CONTENT A\nCONTENT B"},
		{"inline-wordlist", "3", lesson.NodeWordList, 43, 0, 0, "", "ITEM A; ITEM B; ITEM C."},
		{"wordlist", "4", lesson.NodeWordList, 6, 7, 9, "TITLE", "ITEM A\nITEM B\nITEM C"},
		{"table", "4", lesson.NodeTable, 14, 15, 18, "TITLE", "header | header\n---\na | b\nc | d"},
		{"text", "4", lesson.NodeText, 23, 24, 26, "TITLE", "CONTENT A\nCONTENT b\nCONTENT C"},
		{"hint", "6", lesson.NodeHint, 31, 32, 33, "", "CONTENT A\nCONTENT B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := textQAGolden(t, tc.golden)
			declaration := input.Lines[tc.declaration-1]
			var lines []model.Line
			if tc.first != 0 {
				lines = input.Lines[tc.first-1 : tc.last]
			}
			before, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			got, problem := buildTextBlock(tc.kind, &declaration, lines, textQAContext())
			if problem != nil {
				t.Fatal(problem)
			}
			wantCount := 1
			if tc.title != "" {
				wantCount++
			}
			if len(got) != wantCount {
				t.Fatalf("Data = %#v", got)
			}
			if tc.title != "" {
				textQAAssertDatum(t, got[0], lesson.RoleTitle, tc.title, "")
			}
			textQAAssertDatum(t, got[len(got)-1], lesson.RoleBody, tc.body, "")
			after, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("вход изменён")
			}
		})
	}
}

func TestTextQABlockStandaloneAndEmpty(t *testing.T) {
	blank := model.Line{Number: 8, LineType: model.LineTypeBlank, Raw: " \t"}
	for _, lines := range [][]model.Line{nil, {}, {blank}, {textQAContent(9, "")}} {
		got, problem := buildTextBlock(lesson.NodeText, nil, lines, textQAContext())
		if problem != nil || got == nil || len(got) != 0 {
			t.Fatalf("пустой текст = %#v, %v", got, problem)
		}
	}
	lines := []model.Line{textQAContent(3, "<b>Привет</b>"), blank, textQAContent(20, "<br>"), textQAContent(91, "x;y|z")}
	got, problem := buildTextBlock(lesson.NodeText, nil, lines, textQAContext())
	if problem != nil || len(got) != 1 {
		t.Fatalf("Data=%#v, %v", got, problem)
	}
	textQAAssertDatum(t, got[0], lesson.RoleBody, "<b>Привет</b>\n<br>\nx;y|z", "")
}

func TestTextQABlockTitleOnlyAndFailureHasNoPartialData(t *testing.T) {
	declaration := textQAGolden(t, "4").Lines[22]
	got, problem := buildTextBlock(lesson.NodeText, &declaration, nil, textQAContext())
	if problem != nil || len(got) != 1 {
		t.Fatalf("Data=%#v, %v", got, problem)
	}
	textQAAssertDatum(t, got[0], lesson.RoleTitle, "TITLE", "")
	bad := textQAContent(24, "неизвестно")
	bad.Elements[0].Value = nil
	got, problem = buildTextBlock(lesson.NodeText, &declaration, []model.Line{bad}, textQAContext())
	textQAAssertError(t, problem, CodeMissingValue, 24)
	if got != nil {
		t.Fatalf("частичные данные: %#v", got)
	}
	declaration.Elements[1].Value = nil
	got, problem = buildTextBlock(lesson.NodeText, &declaration, nil, textQAContext())
	textQAAssertError(t, problem, CodeMissingValue, 23)
	if got != nil {
		t.Fatalf("частичные данные: %#v", got)
	}
}

func TestTextQADatumOwnsRoleAndAnswers(t *testing.T) {
	for _, role := range []lesson.DataRole{lesson.RoleTitle, lesson.RoleBody, lesson.RoleBefore, lesson.RoleAfter} {
		t.Run(string(role), func(t *testing.T) {
			first := textDatum(role, " \nПривет😀\\@x ", "подсказка\n<br>")
			second := textDatum(role, " \nПривет😀\\@x ", "подсказка\n<br>")
			textQAAssertDatum(t, first, role, " \nПривет😀\\@x ", "подсказка\n<br>")
			textQAAssertDatum(t, second, role, " \nПривет😀\\@x ", "подсказка\n<br>")
			if first.Role == second.Role {
				t.Fatal("указатель Role разделён между вызовами")
			}
			*first.Role = lesson.RoleHint
			first.Answers = append(first.Answers, "чужой ответ")
			first.Hint = "изменено"
			textQAAssertDatum(t, second, role, " \nПривет😀\\@x ", "подсказка\n<br>")
		})
	}
}

func TestTextQABlockInlineKindsRemainOpaque(t *testing.T) {
	cases := []struct {
		kind lesson.NodeType
		tag  string
	}{
		{lesson.NodeText, "text"}, {lesson.NodeNote, "note"}, {lesson.NodeAlt, "alt"},
		{lesson.NodeScript, "script"}, {lesson.NodeHint, "hint"}, {lesson.NodeAnswerKey, "key"},
		{lesson.NodeWordList, "wordlist"}, {lesson.NodeTable, "table"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			prefix := "@" + tc.tag
			value := "<b>Я😀</b>; a|b; @hint; \\@script"
			declaration := model.Line{Number: 5, LineType: model.LineTypeTag, Raw: prefix + " " + value,
				Elements: []model.Element{textQAElement(model.ElementTypeTag, prefix, tc.tag, 1), textQAElement(model.ElementTypeContent, value, value, len(prefix)+2)}}
			first, problem := buildTextBlock(tc.kind, &declaration, nil, textQAContext())
			if problem != nil || len(first) != 1 {
				t.Fatalf("Data=%#v, %v", first, problem)
			}
			textQAAssertDatum(t, first[0], lesson.RoleBody, value, "")
			second, problem := buildTextBlock(tc.kind, &declaration, nil, textQAContext())
			if problem != nil || len(second) != 1 {
				t.Fatalf("Data=%#v, %v", second, problem)
			}
			if first[0].Role == second[0].Role {
				t.Fatal("Role разделён между вызовами")
			}
			*first[0].Role = lesson.RoleTitle
			first[0].Answers = append(first[0].Answers, "не извлекать")
			textQAAssertDatum(t, second[0], lesson.RoleBody, value, "")
			if *declaration.Elements[1].Value != value {
				t.Fatal("входное Value изменено")
			}
		})
	}
}
