package fragments

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task15Tag(tag, raw, value string, parent int) model.Line {
	tagRaw := "@" + strings.ToUpper(tag)
	start := utf8.RuneCountInString(tagRaw) + 4
	line := model.Line{LineType: model.LineTypeTag, Raw: "я " + tagRaw + " " + raw, LineEnding: model.LineEndingLF,
		Elements: []model.Element{
			{ElementType: model.ElementTypeTag, Raw: tagRaw, Value: &tag, Start: 3, End: start - 1, ErrorIDs: []string{}},
			{ElementType: model.ElementTypeIdentifier, Raw: raw, Value: &value, Start: start, End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}},
		}}
	if parent != 0 {
		line.ParentLine, line.NestingLevel = &parent, 1
	}
	if tag == "fragment" {
		line.LineType = model.LineTypeBlockStart
		column, brace := utf8.RuneCountInString(line.Raw)+2, "{"
		line.Raw += " {"
		line.Elements = append(line.Elements, model.Element{ElementType: model.ElementTypeBlockOpen, Raw: brace, Value: &brace, Start: column, End: column + 1, ErrorIDs: []string{}})
	}
	return line
}

func task15Close(parent int) model.Line {
	brace := "}"
	return model.Line{LineType: model.LineTypeBlockEnd, ParentLine: &parent, NestingLevel: 1, Raw: brace, LineEnding: model.LineEndingLF,
		Elements: []model.Element{{ElementType: model.ElementTypeBlockClose, Raw: brace, Value: &brace, Start: 1, End: 2, ErrorIDs: []string{}}}}
}

func task15Input(t *testing.T, lines []model.Line) *parsed.Input {
	t.Helper()
	for i := range lines {
		lines[i].Number = i + 1
	}
	id, name, path, count := "QA15", "source.txt", `C:\old\source.txt`, len(lines)
	result := model.Result{Format: model.FormatVersion1, Lines: lines, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{},
		Document: model.Document{FileName: &name, FilePath: &path, LineCount: &count, Metadata: model.DocumentMetadata{DocumentID: &id, ResourceDirs: []string{}}}}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(t.TempDir(), "technical.json")
	if err := os.WriteFile(jsonPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	input, issues := parsed.Load(jsonPath)
	if input == nil || issues != nil {
		t.Fatalf("fixture не прошёл настоящий parsed.Load: %v", issues)
	}
	return input
}

func task15Problem(t *testing.T, issue *diagnostic.Error, code, name string, context diagnostic.Context) {
	t.Helper()
	if issue == nil || issue.Code != code || issue.Context != context {
		t.Fatalf("ошибка = %+v; нужны %s, %+v", issue, code, context)
	}
	if !strings.ContainsFunc(issue.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("нет русской причины: %q", issue.Reason)
	}
	if !slices.ContainsFunc(issue.Details, func(d diagnostic.Detail) bool { return d.Value == name }) {
		t.Errorf("нет исходного имени %q в Details: %+v", name, issue.Details)
	}
}

func TestTask15ResolveIncludeSemanticCandidates(t *testing.T) {
	input := task15Input(t, []model.Line{task15Tag("fragment", "ОДИН", "Один", 0), task15Close(1), task15Tag("fragment", "один", "один", 0), task15Close(3)})
	first := &definition{Header: 1, Identifier: input.Result.Lines[0].Elements[1]}
	second := &definition{Header: 3, Identifier: input.Result.Lines[2].Elements[1]}
	context := diagnostic.Context{Path: input.Path, LessonID: "QA15", Line: 17, Column: 12}
	for _, test := range []struct {
		name, raw, value, key, code string
		candidates                  []*definition
	}{
		{"нет", "Один", "Один", "один", CodeMissing, nil},
		{"один Unicode", `О\#ДИН`, "О#ДИН", "о#дин", "", []*definition{first}},
		{"пробелы значимы", " Один ", " Один ", "один", CodeMissing, []*definition{first}},
		{"пробелы сохранены", " Один ", " Один ", " один ", "", []*definition{first}},
		{"без нормализации", "é", "é", "é", CodeMissing, []*definition{first}},
		{"несколько", "ОДИН", "ОДИН", "один", CodeAmbiguous, []*definition{first, second}},
	} {
		t.Run(test.name, func(t *testing.T) {
			identifier := task15Tag("include", test.raw, test.value, 0).Elements[1]
			target, issue := resolveInclude(identifier, map[string][]*definition{test.key: test.candidates}, context)
			if test.code == "" {
				if issue != nil || target != first || *identifier.Value != test.value {
					t.Fatalf("target=%p issue=%v identifier=%+v", target, issue, identifier)
				}
				return
			}
			if target != nil {
				t.Fatal("ошибка вернула пригодный target")
			}
			task15Problem(t, issue, test.code, test.value, context)
			if test.code == CodeAmbiguous {
				want := []diagnostic.Context{context, context}
				want[0].Line, want[0].Column = 1, first.Identifier.Start
				want[1].Line, want[1].Column = 3, second.Identifier.Start
				if !slices.Equal(issue.Related, want) {
					t.Errorf("Related=%+v, нужны все объявления %+v", issue.Related, want)
				}
			}
		})
	}
}

func TestTask15ResolveIncludesWholeDocument(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			name := "Цель"
			if bad {
				name = "нет"
			}
			input := task15Input(t, []model.Line{task15Tag("include", "ЦЕЛЬ", "ЦЕЛЬ", 0), task15Tag("fragment", "unused", "unused", 0),
				task15Tag("include", name, name, 2), task15Close(2), task15Tag("fragment", "Цель", "Цель", 0), task15Close(5),
				task15Tag("task", "foreign", "foreign", 0), task15Tag("include", "foreign", "foreign", 0), task15Tag("include", "QA15", "QA15", 0)})
			target := &definition{Header: 5, Identifier: input.Result.Lines[4].Elements[1]}
			definitions := map[string][]*definition{"цель": {target}, "unused": {{Header: 2, Identifier: input.Result.Lines[1].Elements[1], Body: []int{3}}}}
			index, issues := resolveIncludes(input, definitions)
			wantLines := []int{8, 9}
			if bad {
				wantLines = []int{3, 8, 9}
			}
			if index != nil || len(issues) != len(wantLines) {
				t.Fatalf("нельзя возвращать частичный индекс: index=%v issues=%v", index, issues)
			}
			for i, number := range wantLines {
				identifier := input.Result.Lines[number-1].Elements[1]
				task15Problem(t, issues[i], CodeMissing, *identifier.Value, diagnostic.Context{Path: input.Path, LessonID: "QA15", Line: number, Column: identifier.Start})
			}
		})
	}
	for _, shape := range []string{"без имени", "пустое имя", "два имени", "блочный include"} {
		t.Run(shape, func(t *testing.T) {
			line := task15Tag("include", "F", "F", 0)
			switch shape {
			case "без имени":
				line.Elements = line.Elements[:1]
			case "пустое имя":
				*line.Elements[1].Value = ""
			case "два имени":
				second := line.Elements[1]
				second.Start, second.End = second.End+1, second.End+2
				line.Raw += " F"
				line.Elements = append(line.Elements, second)
			case "блочный include":
				line = task15Tag("fragment", "F", "F", 0)
				*line.Elements[0].Value = "include"
			}
			input := task15Input(t, []model.Line{line})
			index, issues := resolveIncludes(input, map[string][]*definition{})
			if index != nil || len(issues) != 1 || issues[0].Code != CodeShape || issues[0].Context.Line != 1 || issues[0].Context.Path != input.Path {
				t.Fatalf("shape: index=%v issues=%v", index, issues)
			}
		})
	}
	forward := task15Input(t, []model.Line{task15Tag("include", "F", "F", 0), task15Tag("fragment", "F", "F", 0), task15Close(2)})
	target := &definition{Header: 2, Identifier: forward.Result.Lines[1].Elements[1]}
	index, issues := resolveIncludes(forward, map[string][]*definition{"f": {target}})
	if issues != nil || len(index) != 1 || index[1] != target {
		t.Fatalf("forward include: index=%v issues=%v", index, issues)
	}

	input, definitions, want := task15Graph(t, [][]int{{1, 1}, {}})
	got, issues := resolveIncludes(input, definitions)
	if issues != nil || len(got) != len(want) {
		t.Fatalf("успешный индекс=%v issues=%v", got, issues)
	}
	for number, target := range want {
		if got[number] != target {
			t.Errorf("include %d разрешён в %p вместо %p", number, got[number], target)
		}
	}
}

func task15Graph(t *testing.T, edges [][]int) (*parsed.Input, map[string][]*definition, map[int]*definition) {
	t.Helper()
	var lines []model.Line
	definitions, references := make(map[string][]*definition), make(map[int]*definition)
	ordered := make([]*definition, len(edges))
	for i, targets := range edges {
		name, header := fmt.Sprintf("F%d", i), len(lines)+1
		line := task15Tag("fragment", name, name, 0)
		ordered[i] = &definition{Header: header, Identifier: line.Elements[1], Body: []int{}}
		definitions[strings.ToLower(name)] = []*definition{ordered[i]}
		lines = append(lines, line)
		for _, target := range targets {
			name := fmt.Sprintf("F%d", target)
			ordered[i].Body = append(ordered[i].Body, len(lines)+1)
			lines = append(lines, task15Tag("include", name, name, header))
		}
		lines = append(lines, task15Close(header))
	}
	input := task15Input(t, lines)
	for i, targets := range edges {
		for j, target := range targets {
			references[ordered[i].Body[j]] = ordered[target]
		}
	}
	return input, definitions, references
}

func TestTask15CheckIncludeCycles(t *testing.T) {
	deep := make([][]int, 2048)
	for i := 0; i+1 < len(deep); i++ {
		deep[i] = []int{i + 1}
	}
	for _, test := range []struct {
		name   string
		edges  [][]int
		cycles int
	}{
		{"прямой unused", [][]int{{0}}, 1},
		{"косвенный unused", [][]int{{1}, {2}, {0}}, 1},
		{"disconnected unused", [][]int{{}, {1}, {2}}, 2},
		{"diamond reuse", [][]int{{1, 2}, {3}, {3}, {}}, 0},
		{"глубокий ациклический", deep, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, definitions, references := task15Graph(t, test.edges)
			var previous []*diagnostic.Error
			for run := 0; run < 5; run++ {
				issues := checkIncludeCycles(input, definitions, references)
				if len(issues) != test.cycles {
					t.Fatalf("cycles=%v, нужно %d", issues, test.cycles)
				}
				for i, issue := range issues {
					if issue.Code != CodeCycle || issue.Context.Path != input.Path || issue.Context.LessonID != "QA15" || issue.Context.PageID != "" {
						t.Fatalf("неверная диагностика цикла: %+v", issue)
					}
					if i > 0 && issues[i-1].Context.Line >= issue.Context.Line {
						t.Error("компоненты проверены не в физическом порядке")
					}
					var chain []int
					for _, related := range issue.Related {
						if related.Path != input.Path || related.LessonID != "QA15" || related.PageID != "" || related.Line < 1 || related.Line > len(input.Result.Lines) {
							t.Fatalf("нефизический Related: %+v", related)
						}
						line := input.Result.Lines[related.Line-1]
						if related.Column != line.Elements[1].Start {
							t.Errorf("неверная Unicode-колонка: %+v", related)
						}
						chain = append(chain, related.Line)
					}
					members := []int{1, 2}
					if test.name == "косвенный unused" {
						members = []int{1, 2, 4, 5, 7, 8}
					} else if test.name == "disconnected unused" {
						members = []int{3 + i*3, 4 + i*3}
					}
					if len(chain) < len(members) || len(chain) > len(members)+1 || !slices.Contains(members, chain[0]) {
						t.Fatalf("неполная цепочка Related=%v; участники %v", chain, members)
					}
					start := slices.Index(members, chain[0])
					for j, number := range chain {
						if number != members[(start+j)%len(members)] {
							t.Errorf("Related не цепочка include+definition: %v", chain)
						}
					}
					if run > 0 && (issue.Context != previous[i].Context || !slices.Equal(issue.Related, previous[i].Related)) {
						t.Error("нестабильный порядок диагностики")
					}
				}
				previous = issues
			}
		})
	}
}
