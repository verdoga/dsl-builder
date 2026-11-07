package structure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestTask18ErrorCodes(t *testing.T) {
	for _, pair := range [][2]string{{CodeSource, "structure.source"}, {CodeBoundary, "structure.boundary"}, {CodeContext, "structure.context"}} {
		if pair[0] != pair[1] {
			t.Errorf("code = %q, want %q", pair[0], pair[1])
		}
	}
}

func TestTask18SourceContextPreservesKnownCoordinates(t *testing.T) {
	input, entries := task18Input(t, []model.Line{task18Tag(41, "speaking", nil, "", "", false)}, false)
	before := task18JSON(t, input)
	for _, column := range []int{0, entries[0].Source.Elements[0].Start} {
		got := sourceContext(input, entries[0], column)
		want := diagnostic.Context{Path: input.Path, LessonID: "TaSk18", Line: 41, Column: column}
		if got != want {
			t.Errorf("sourceContext = %#v, want %#v", got, want)
		}
	}
	if got := task18JSON(t, input); got != before {
		t.Fatal("sourceContext changed parsed.Input")
	}
}

func TestTask18StructureErrorIncludesEveryOccurrenceBoundary(t *testing.T) {
	input, entries := task18Nested(t, "endtask", 8, 9)
	beforeInput, beforeEntries := task18JSON(t, input), task33Entries(t, entries)
	first := entries[1].Included[0].Included[0]
	second := entries[2].Included[0].Included[0]
	if first == second || first.Source.Number != second.Source.Number {
		t.Fatal("fixture must contain distinct entries with repeated physical numbers")
	}
	state := &builder{input: input, page: -1}
	for i, site := range entries[1:] {
		for _, code := range []string{CodeSource, CodeBoundary, CodeContext} {
			reason := "Не найдена необходимая граница конструкции"
			problem := structureError(code, reason, site.Included[0].Included[0], []*fragments.Entry{entries[0]}, state)
			if problem == nil {
				t.Fatal("structureError returned nil")
			}
			if problem.Code != code || problem.Reason != reason {
				t.Errorf("error = %#v, want code %q and supplied Russian reason", problem, code)
			}
			task18Place(t, input, problem.Context, 2)
			for _, line := range []int{5, site.Source.Number, 7} {
				task18Related(t, input, problem.Related, line)
			}
			for _, place := range problem.Related {
				if place.Line == entries[2-i].Source.Number {
					t.Errorf("occurrence at %d leaked other include at %d", site.Source.Number, place.Line)
				}
			}
		}
	}
	if task18JSON(t, input) != beforeInput || task33Entries(t, entries) != beforeEntries {
		t.Fatal("structureError changed input or expanded entries")
	}
}

func TestTask18BuildErrorsFollowExpandedInputAndStaySilent(t *testing.T) {
	for _, tc := range []struct {
		name, root   string
		sites, order []int
	}{
		{"root and repeated includes", "endtask", []int{8, 9}, []int{7, 8, 9}},
		{"first occurrence", "editor", []int{8}, []int{8}},
		{"second occurrence", "editor", []int{9}, []int{9}},
		{"repeated occurrences", "editor", []int{8, 9}, []int{8, 9}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, entries := task18Nested(t, tc.root, tc.sites...)
			beforeInput, beforeEntries := task18JSON(t, input), task33Entries(t, entries)
			var problems []*diagnostic.Error
			task18Silent(t, func() {
				pages, bindings, found := Build(input, entries)
				problems = found
				if pages != nil || bindings != nil {
					t.Error("failed Build returned partial pages or bindings")
				}
			})
			if len(problems) == 0 {
				t.Fatal("Build must return structural errors")
			}
			previous := -1
			for i, problem := range problems {
				if problem == nil {
					t.Fatal("nil diagnostic")
				}
				if problem.Code != CodeContext || !strings.ContainsFunc(problem.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
					t.Errorf("diagnostic %d must have structure.context and Russian reason: %#v", i, problem)
				}
				site := problem.Context.Line
				if site == 2 {
					site = 0
					for _, place := range problem.Related {
						if place.Line == 8 || place.Line == 9 {
							if site != 0 && site != place.Line {
								t.Fatal("diagnostic leaked another occurrence's include")
							}
							site = place.Line
						}
					}
					task18Related(t, input, problem.Related, 5)
					task18Related(t, input, problem.Related, site)
					task18Place(t, input, problem.Context, 2)
				} else {
					task18Place(t, input, problem.Context, 7)
				}
				position := slices.Index(tc.order, site)
				if position < 0 || position < previous {
					t.Fatalf("diagnostic %d occurrence %d violates expanded order %v", i, site, tc.order)
				}
				previous = position
			}
			if task18JSON(t, input) != beforeInput || task33Entries(t, entries) != beforeEntries {
				t.Fatal("failed Build changed input or expanded entries")
			}
		})
	}
}

func task18Nested(t *testing.T, root string, sites ...int) (*parsed.Input, []*fragments.Entry) {
	t.Helper()
	one, four := 1, 4
	lines := []model.Line{
		task18Tag(1, "fragment", nil, "identifier", "leaf", true),
		task18Tag(2, "step", &one, "title", "Этап", false),
		task18End(3, one),
		task18Tag(4, "fragment", nil, "identifier", "middle", true),
		task18Tag(5, "include", &four, "identifier", "leaf", false),
		task18End(6, four),
		task18Tag(7, root, nil, "", "", false),
	}
	for _, site := range sites {
		lines = append(lines, task18Tag(site, "include", nil, "identifier", "middle", false))
	}
	return task18Input(t, lines, true)
}

func task18Input(t *testing.T, lines []model.Line, notices bool) (*parsed.Input, []*fragments.Entry) {
	t.Helper()
	id, name, source := "TaSk18", "исходник.txt", `C:\старый путь\исходник.txt`
	result := model.Result{Format: model.FormatVersion1, Document: model.Document{
		FileName: &name, FilePath: &source, Metadata: model.DocumentMetadata{DocumentID: &id, ResourceDirs: []string{}},
	}, Processing: []model.Processing{}, Lines: lines, Diagnostics: []model.Diagnostic{}}
	if notices {
		result.Processing = []model.Processing{{ID: "parser", Tool: "dsl-parser"}}
		for i, severity := range []model.Severity{model.SeverityWarning, model.SeverityRecommendation} {
			result.Diagnostics = append(result.Diagnostics, model.Diagnostic{ID: []string{"warning", "recommendation"}[i], Source: "parser",
				DiagnosticCode: "parser.notice", SeverityLevel: severity, Message: "Сообщение парсера", DiagnosticScope: model.ScopeDocument, RelatedLocations: []model.Location{}})
		}
	}
	path := filepath.Join(t.TempDir(), "технический JSON.json")
	if err := os.WriteFile(path, []byte(task18JSON(t, result)), 0600); err != nil {
		t.Fatal(err)
	}
	input, problems := parsed.Load(path)
	if len(problems) != 0 {
		t.Fatalf("fixture Load: %#v", problems)
	}
	entries, problems := fragments.Expand(input)
	if len(problems) != 0 {
		t.Fatalf("fixture Expand: %#v", problems)
	}
	return input, entries
}

func task18Tag(number int, tag string, parent *int, extra model.ElementType, value string, block bool) model.Line {
	raw := "  @" + tag
	elements := []model.Element{{ElementType: model.ElementTypeTag, Raw: "@" + tag, Value: &tag, Start: 3, End: 4 + len(tag), ErrorIDs: []string{}}}
	if extra != "" {
		start := len([]rune(raw)) + 2
		raw += " " + value
		elements = append(elements, model.Element{ElementType: extra, Raw: value, Value: &value, Start: start, End: start + len([]rune(value)), ErrorIDs: []string{}})
	}
	kind := model.LineTypeTag
	if block {
		brace := "{"
		start := len([]rune(raw)) + 2
		raw += " {"
		kind = model.LineTypeBlockStart
		elements = append(elements, model.Element{ElementType: model.ElementTypeBlockOpen, Raw: brace, Value: &brace, Start: start, End: start + 1, ErrorIDs: []string{}})
	}
	level := 0
	if parent != nil {
		level = 1
	}
	return model.Line{Number: number, LineType: kind, ParentLine: parent, NestingLevel: level, Raw: raw, LineEnding: model.LineEndingLF, Elements: elements}
}

func task18End(number, parent int) model.Line {
	brace := "}"
	return model.Line{Number: number, LineType: model.LineTypeBlockEnd, ParentLine: &parent, NestingLevel: 1, Raw: brace,
		LineEnding: model.LineEndingLF, Elements: []model.Element{{ElementType: model.ElementTypeBlockClose, Raw: brace, Value: &brace, Start: 1, End: 2, ErrorIDs: []string{}}}}
}

func task18Place(t *testing.T, input *parsed.Input, got diagnostic.Context, line int) {
	t.Helper()
	if got.Path != input.Path || got.LessonID != "TaSk18" || got.Line != line || got.PageID != "" || got.FileName != "" {
		t.Errorf("context = %#v; want technical JSON path, exact metadata ID, physical line %d, no invented page/archive", got, line)
	}
	if got.Column != 0 {
		for _, element := range input.Result.Lines[input.Index.ByNumber[line]].Elements {
			if element.Start == got.Column {
				return
			}
		}
		t.Errorf("context column = %d, want known element start or unknown 0", got.Column)
	}
}

func task18Related(t *testing.T, input *parsed.Input, places []diagnostic.Context, line int) {
	t.Helper()
	for _, place := range places {
		if place.Line == line {
			task18Place(t, input, place, line)
			return
		}
	}
	t.Errorf("Related lacks known declaration/include at physical line %d: %#v", line, places)
}

func task18JSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// task33Entries фиксирует значения Source и точные связи графа без обхода циклов.
func task33Entries(t *testing.T, entries []*fragments.Entry) string {
	t.Helper()
	var snapshot strings.Builder
	links := func(entries []*fragments.Entry) {
		fmt.Fprintf(&snapshot, "nil=%t[", entries == nil)
		for _, entry := range entries {
			fmt.Fprintf(&snapshot, "%p,", entry)
		}
		snapshot.WriteString("]")
	}
	links(entries)
	seen := make(map[*fragments.Entry]bool)
	var visit func(*fragments.Entry)
	visit = func(entry *fragments.Entry) {
		if entry == nil || seen[entry] {
			return
		}
		seen[entry] = true
		fmt.Fprintf(&snapshot, "%p:%s;parent=%p;included=", entry, task18JSON(t, entry.Source), entry.Parent)
		links(entry.Included)
		visit(entry.Parent)
		for _, included := range entry.Included {
			visit(included)
		}
	}
	for _, entry := range entries {
		visit(entry)
	}
	return snapshot.String()
}

func task18Silent(t *testing.T, run func()) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, out
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	run()
	info, err := out.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("Build printed %d bytes to stdout/stderr", info.Size())
	}
}
