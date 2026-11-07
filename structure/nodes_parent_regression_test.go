package structure_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/structure"
)

func TestTask36QuestionRejectsMixedSourceParents(t *testing.T) {
	for _, parents := range [][2]int{{1, 2}, {2, 1}} {
		t.Run(string(rune('0'+parents[0]))+"_to_"+string(rune('0'+parents[1])), func(t *testing.T) {
			lines := task36Prefix()
			lines = append(lines,
				task36Line(3, parents[0], parents[0], "question", "content", "Вопрос?"),
				task36Line(4, parents[1], parents[1], "answer", "content", "Ответ"))
			input, entries := task36LoadExpand(t, lines)
			if entries[2].Parent == entries[3].Parent {
				t.Fatal("Expand скрыл разных исходных родителей")
			}
			// Consumer независимо подтверждает, что эти исходные строки нельзя склеить.
			data, checkable, err := exercisedata.Build(lesson.NodeQuestion,
				[]model.Line{entries[2].Source, entries[3].Source}, diagnostic.Context{Path: input.Path})
			if err == nil || err.Code != exercisedata.CodeBinding || data != nil || checkable {
				t.Fatalf("consumer mixed-parent: data=%v, checkable=%v, error=%v", data, checkable, err)
			}
			pages, bindings, problems := structure.Build(input, entries)
			task36ContextFailure(t, pages, bindings, problems, input.Path, 4)
		})
	}
}

func TestTask36SameParentQuestionWorksWithinSemanticStep(t *testing.T) {
	for _, parent := range []int{1, 2} {
		t.Run(string(rune('0'+parent)), func(t *testing.T) {
			lines := task36Prefix()
			lines = append(lines,
				task36Line(3, parent, parent, "question", "content", "Вопрос?"),
				task36Line(4, parent, parent, "answer", "content", "Ответ"),
				task36Line(5, parent, parent, "hint", "content", "Подсказка"))
			input, entries := task36LoadExpand(t, lines)
			pages, bindings, problems := structure.Build(input, entries)
			if problems != nil || len(pages) != 1 || len(bindings) != 3 {
				t.Fatalf("Build: pages=%v, bindings=%v, errors=%v", pages, bindings, problems)
			}
			question := bindings[2]
			if question.Node(pages).Type != lesson.NodeQuestion || !slices.Equal(question.NodePath, []int{0, 0, 0}) {
				t.Fatalf("Вопрос вне task → step: %+v", question)
			}
			if len(question.Entries) != 3 {
				t.Fatalf("Entries=%v, нужны question/answer/hint", question.Entries)
			}
			for i, entry := range question.Entries {
				if entry != entries[i+2] {
					t.Fatalf("Entries[%d] потерял исходное вхождение", i)
				}
			}
			projected := question.Lines()
			if !bytes.Equal(task36JSON(t, projected), task36JSON(t, input.Result.Lines[2:])) {
				t.Fatal("Lines изменила исходные поля, включая ParentLine")
			}
			data, checkable, err := exercisedata.Build(lesson.NodeQuestion, projected, diagnostic.Context{Path: input.Path})
			if err != nil || !checkable || len(data) != 1 {
				t.Fatalf("consumer: data=%v, checkable=%v, error=%v", data, checkable, err)
			}
			body := data[0]
			if body.Type != lesson.DataText || body.Role == nil || *body.Role != lesson.RoleBody ||
				body.Value != "Вопрос?" || !slices.Equal(body.Answers, []string{"Ответ"}) || body.Hint != "Подсказка" {
				t.Fatalf("Связанные данные потеряны: %+v", body)
			}
		})
	}
}

func TestTask36QuestionLeavesDifferentParentHintStandalone(t *testing.T) {
	lines := append(task36Prefix(),
		task36Line(3, 1, 1, "question", "content", "Вопрос?"),
		task36Line(4, 1, 1, "answer", "content", "Ответ"),
		task36Line(5, 2, 2, "hint", "content", "Отдельная подсказка"))
	input, entries := task36LoadExpand(t, lines)
	pages, bindings, problems := structure.Build(input, entries)
	if problems != nil || len(bindings) != 4 {
		t.Fatalf("Build: bindings=%v, errors=%v", bindings, problems)
	}
	question, hint := bindings[2], bindings[3]
	if len(question.Entries) != 2 || question.Entries[0] != entries[2] || question.Entries[1] != entries[3] ||
		hint.Node(pages).Type != lesson.NodeHint || len(hint.Entries) != 1 || hint.Entries[0] != entries[4] ||
		!slices.Equal(hint.NodePath, []int{0, 0, 1}) {
		t.Fatalf("Чужая подсказка поглощена или перемещена: question=%+v, hint=%+v", question, hint)
	}
	data, checkable, err := exercisedata.Build(lesson.NodeQuestion, question.Lines(), diagnostic.Context{Path: input.Path})
	if err != nil || !checkable || len(data) != 1 || data[0].Hint != "" || !slices.Equal(data[0].Answers, []string{"Ответ"}) {
		t.Fatalf("consumer: data=%v, checkable=%v, error=%v", data, checkable, err)
	}
}

func TestTask36QuestionTailKeepsSemanticAndIncludeBarriers(t *testing.T) {
	cases := []struct {
		name  string
		lines []model.Line
		bad   int
	}{
		{"step", []model.Line{task36Line(4, 1, 1, "step", "title", "Следующий"), task36Line(5, 1, 1, "answer", "content", "Ответ")}, 5},
		{"task", []model.Line{task36Line(4, 0, 0, "task", "identifier", "b"), task36Line(5, 4, 1, "answer", "content", "Ответ")}, 5},
		{"page", []model.Line{task36Line(4, 0, 0, "newpage", "", ""), task36Line(5, 0, 0, "answer", "content", "Ответ")}, 5},
		{"endtask", []model.Line{task36Line(4, 1, 1, "endtask", "", ""), task36Line(5, 0, 0, "answer", "content", "Ответ")}, 5},
		{"variant", []model.Line{
			task36Block(4, 1, 1, "variants", "", ""), task36Line(5, 4, 2, "variant", "name", "Ветка"),
			task36Line(6, 4, 2, "answer", "content", "Ответ"), task36End(7, 4, 2),
		}, 6},
		{"empty_include", []model.Line{
			task36Line(4, 1, 1, "include", "identifier", "f"), task36Line(5, 1, 1, "answer", "content", "Ответ"),
			task36Block(6, 0, 0, "fragment", "identifier", "f"), task36End(7, 6, 1),
		}, 5},
	}
	t.Run("block_end", func(t *testing.T) {
		input, entries := task36LoadExpand(t, []model.Line{
			task36Block(1, 0, 0, "variants", "", ""), task36Line(2, 1, 1, "variant", "name", "Ветка"),
			task36Line(3, 1, 1, "question", "content", "Вопрос?"), task36End(4, 1, 1),
			task36Line(5, 0, 0, "answer", "content", "Ответ"),
		})
		pages, bindings, problems := structure.Build(input, entries)
		task36ContextFailure(t, pages, bindings, problems, input.Path, 5)
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := append(task36Prefix(), task36Line(3, 1, 1, "question", "content", "Вопрос?"))
			input, entries := task36LoadExpand(t, append(lines, tc.lines...))
			pages, bindings, problems := structure.Build(input, entries)
			task36ContextFailure(t, pages, bindings, problems, input.Path, tc.bad)
		})
	}
}

func task36ContextFailure(t *testing.T, pages []lesson.Page, bindings []structure.Binding, problems []*diagnostic.Error, path string, line int) {
	t.Helper()
	if pages != nil || bindings != nil || len(problems) == 0 {
		t.Fatalf("Нужен атомарный отказ orphan answer: pages=%v, bindings=%v, errors=%v", pages, bindings, problems)
	}
	for _, problem := range problems {
		if problem.Code == structure.CodeContext && problem.Context.Line == line {
			if problem.Context.Path != path || problem.Context.LessonID != "QA36" || problem.Context.PageID != "" || problem.Reason == "" {
				t.Fatalf("Потерян исходный контекст: %+v", problem)
			}
			return
		}
	}
	t.Fatalf("Нет CodeContext на исходной строке %d: %v", line, problems)
}

func task36Prefix() []model.Line {
	return []model.Line{task36Line(1, 0, 0, "task", "identifier", "a"), task36Line(2, 1, 1, "step", "title", "Этап")}
}

func task36Line(number, parent, depth int, tag, kind, value string) model.Line {
	line := model.Line{Number: number, LineType: model.LineTypeTag, NestingLevel: depth, LineEnding: model.LineEndingCRLF, Elements: []model.Element{}}
	if parent != 0 {
		line.ParentLine = &parent
	}
	for _, token := range []struct{ kind, raw, value string }{{"tag", "@" + tag, tag}, {kind, value, value}} {
		if token.kind == "" {
			continue
		}
		if line.Raw != "" {
			line.Raw += " "
		}
		start := utf8.RuneCountInString(line.Raw) + 1
		line.Raw += token.raw
		semantic := token.value
		line.Elements = append(line.Elements, model.Element{ElementType: model.ElementType(token.kind), Raw: token.raw, Value: &semantic, Start: start, End: utf8.RuneCountInString(line.Raw) + 1, ErrorIDs: []string{}})
	}
	return line
}

func task36Block(number, parent, depth int, tag, kind, value string) model.Line {
	line := task36Line(number, parent, depth, tag, kind, value)
	line.LineType = model.LineTypeBlockStart
	line.Raw += " {"
	open := "{"
	line.Elements = append(line.Elements, model.Element{ElementType: model.ElementTypeBlockOpen, Raw: open, Value: &open, Start: utf8.RuneCountInString(line.Raw), End: utf8.RuneCountInString(line.Raw) + 1, ErrorIDs: []string{}})
	return line
}

func task36End(number, parent, depth int) model.Line {
	close := "}"
	return model.Line{Number: number, LineType: model.LineTypeBlockEnd, ParentLine: &parent, NestingLevel: depth, Raw: close, LineEnding: model.LineEndingLF,
		Elements: []model.Element{{ElementType: model.ElementTypeBlockClose, Raw: close, Value: &close, Start: 1, End: 2, ErrorIDs: []string{}}}}
}

func task36LoadExpand(t *testing.T, lines []model.Line) (*parsed.Input, []*fragments.Entry) {
	t.Helper()
	id, title := "QA36", "Регрессия родителей"
	lineCount := len(lines)
	result := model.Result{Format: model.FormatVersion1, Document: model.Document{LineCount: &lineCount, Metadata: model.DocumentMetadata{DocumentID: &id, Title: &title, ResourceDirs: []string{}}},
		Processing: []model.Processing{}, Lines: lines, Diagnostics: []model.Diagnostic{}}
	path := filepath.Join(t.TempDir(), "source.json")
	if err := os.WriteFile(path, task36JSON(t, result), 0600); err != nil {
		t.Fatal(err)
	}
	input, problems := parsed.Load(path)
	if problems != nil {
		t.Fatalf("Load: %v", problems)
	}
	entries, problems := fragments.Expand(input)
	if problems != nil {
		t.Fatalf("Expand: %v", problems)
	}
	before := task36JSON(t, input.Result)
	sources := make([][]byte, len(entries))
	parents := make([]*fragments.Entry, len(entries))
	for i, entry := range entries {
		sources[i], parents[i] = task36JSON(t, entry.Source), entry.Parent
	}
	t.Cleanup(func() {
		if !bytes.Equal(before, task36JSON(t, input.Result)) {
			t.Error("Build/consumer изменил parsed.Input")
		}
		for i, entry := range entries {
			if !bytes.Equal(sources[i], task36JSON(t, entry.Source)) || entry.Parent != parents[i] {
				t.Errorf("Build/consumer изменил Source/Parent исходного Entry %d", i)
			}
		}
	})
	return input, entries
}

func task36JSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
