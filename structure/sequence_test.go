package structure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func task24Line(kind model.LineType, parent int, tag string, fields ...string) model.Line {
	line := model.Line{LineType: kind, LineEnding: model.LineEndingLF, Elements: []model.Element{}}
	if parent != 0 {
		line.ParentLine = &parent
	}
	add := func(kind model.ElementType, raw, value string) {
		if line.Raw != "" {
			line.Raw += " "
		}
		start := utf8.RuneCountInString(line.Raw) + 1
		line.Raw += raw
		line.Elements = append(line.Elements, model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: utf8.RuneCountInString(line.Raw) + 1, ErrorIDs: []string{}})
	}
	if tag != "" {
		add(model.ElementTypeTag, "@"+tag, tag)
	}
	for i := 0; i < len(fields); i += 2 {
		raw := fields[i+1]
		if fields[i] == "heading-level" {
			raw = strings.Repeat("#", int(raw[0]-'0'))
		}
		add(model.ElementType(fields[i]), raw, fields[i+1])
	}
	if kind == model.LineTypeBlockStart {
		add(model.ElementTypeBlockOpen, "{", "{")
	}
	if kind == model.LineTypeBlockEnd {
		add(model.ElementTypeBlockClose, "}", "}")
	}
	return line
}

func task24Fixture(t *testing.T, lines ...model.Line) (*parsed.Input, []*fragments.Entry) {
	t.Helper()
	id := "TASK24"
	for i := range lines {
		lines[i].Number = i + 1
		if p := lines[i].ParentLine; p != nil {
			lines[i].NestingLevel = lines[*p-1].NestingLevel + 1
		}
	}
	inputModel := model.Result{Format: model.FormatVersion1, Document: model.Document{Metadata: model.DocumentMetadata{DocumentID: &id, ResourceDirs: []string{}}}, Processing: []model.Processing{}, Lines: lines, Diagnostics: []model.Diagnostic{}}
	raw, err := json.Marshal(inputModel)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sequence.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	input, problems := parsed.Load(path)
	if len(problems) != 0 {
		t.Fatalf("Load: %v", problems)
	}
	entries, problems := fragments.Expand(input)
	if len(problems) != 0 {
		t.Fatalf("Expand: %v", problems)
	}
	return input, entries
}

func task24State(input *parsed.Input) *builder { return &builder{input: input, page: -1} }

func task24NoErrors(t *testing.T, problems []*diagnostic.Error) {
	t.Helper()
	if len(problems) != 0 {
		t.Fatalf("unexpected diagnostics: %v", problems)
	}
}

func task24Code(t *testing.T, problems []*diagnostic.Error, code string, input *parsed.Input, line int) {
	t.Helper()
	if len(problems) == 0 {
		t.Fatalf("missing %s", code)
	}
	p := problems[0]
	if p.Code != code || p.Context.Path != input.Path || p.Context.Line != line || p.Context.PageID != "" || p.Reason == "" {
		t.Fatalf("diagnostic: %+v", p)
	}
}

func task24Paths(t *testing.T, state *builder, want ...string) {
	t.Helper()
	got := make([]string, len(state.bindings))
	for i, b := range state.bindings {
		got[i] = fmt.Sprintf("%d:%v:%s", b.Page, b.NodePath, b.Node(state.pages).Type)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("bindings %v, want %v", got, want)
	}
}

func TestTask24BlockEntriesPreservesWholeBlockAndOneLine(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("tag", 0, "note", "content", "one"),
		task24Line("block-start", 0, "instr"),
		task24Line("blank", 2, ""),
		task24Line("block-start", 2, "hint"),
		task24Line("content", 4, "", "content", "} @endtask {"),
		task24Line("block-end", 4, ""),
		task24Line("block-end", 2, ""),
		task24Line("tag", 0, "note", "content", "after"))
	state := task24State(input)
	for _, tc := range []struct{ at, next int }{{0, 1}, {1, 7}, {7, 8}} {
		body, next, problems := blockEntries(entries, tc.at, state)
		task24NoErrors(t, problems)
		if next != tc.next || !slices.Equal(body, entries[tc.at:tc.next]) {
			t.Fatalf("at %d: body=%v next=%d", tc.at, body, next)
		}
	}
}

func TestTask24BlockEntriesBoundaryGuardsUseEntryIdentity(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("block-start", 0, "instr"),
		task24Line("block-start", 1, "hint"),
		task24Line("block-end", 2, ""),
		task24Line("block-end", 1, ""))
	for _, name := range []string{"missing", "crossed", "foreign occurrence"} {
		t.Run(name, func(t *testing.T) {
			// Локальные отрицательные guards blockEntries, не вход полного Build.
			local := append([]*fragments.Entry(nil), entries...)
			switch name {
			case "missing":
				local = local[:3]
			case "crossed":
				local[2], local[3] = local[3], local[2]
			case "foreign occurrence":
				foreign := *entries[0]
				end := *entries[3]
				end.Parent = &foreign
				local[3] = &end
			}
			_, _, problems := blockEntries(local, 0, task24State(input))
			if len(problems) == 0 {
				t.Fatal("missing boundary diagnostic")
			}
			p := problems[0]
			if p.Code != CodeBoundary || p.Context.Path != input.Path || p.Context.Line < 1 || p.Context.Line > 4 || p.Reason == "" {
				t.Fatalf("boundary diagnostic: %+v", p)
			}
		})
	}
}

func TestTask24SkipEditorOpaqueAndVisitConsumesOnce(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("tag", 0, "editor", "content", "@task x"),
		task24Line("block-start", 0, "editor"),
		task24Line("tag", 2, "newpage"),
		task24Line("tag", 2, "answer", "content", "hidden"),
		task24Line("block-start", 2, "variants"),
		task24Line("block-end", 5, ""),
		task24Line("block-end", 2, ""),
		task24Line("tag", 0, "note", "content", "visible"))
	state := task24State(input)
	for _, tc := range []struct{ at, next int }{{0, 1}, {1, 7}} {
		next, problems := skipEditor(state, entries, tc.at)
		task24NoErrors(t, problems)
		if next != tc.next || len(state.bindings) != 0 || state.page != -1 {
			t.Fatalf("editor next=%d state=%+v", next, state)
		}
	}
	task24NoErrors(t, walkSequence(state, entries, nil))
	task24Paths(t, state, "0:[0]:note")
	if state.bindings[0].Entries[0] != entries[7] {
		t.Fatal("visible entry lost")
	}
}

func TestTask24VisitSkipsServicesAndRejectsSignificantInput(t *testing.T) {
	lines := []model.Line{task24Line("blank", 0, ""), task24Line("heading", 0, "", "heading-level", "1", "title", "Title"), task24Line("heading", 0, "", "heading-level", "2", "title", "Sub")}
	for _, tag := range []string{"dsl-version", "document-id", "section", "order", "resource-dir"} {
		lines = append(lines, task24Line("tag", 0, tag))
	}
	input, entries := task24Fixture(t, lines...)
	state := task24State(input)
	for at := range entries {
		next, problems := visitEntry(state, entries, at, nil)
		task24NoErrors(t, problems)
		if next != at+1 || len(state.bindings) != 0 {
			t.Fatalf("service at=%d next=%d", at, next)
		}
	}
	for _, tc := range []struct {
		line model.Line
		code string
	}{{task24Line("tag", 0, "answer", "content", "a"), CodeContext}, {task24Line("separator", 0, "", "content", "---"), CodeSource}, {task24Line("tag", 0, ""), CodeSource}} {
		input, entries := task24Fixture(t, tc.line)
		_, problems := visitEntry(task24State(input), entries, 0, nil)
		task24Code(t, problems, tc.code, input, 1)
	}
	// fragment после Expand — явное нарушение private предусловия диспетчера.
	fragment := &fragments.Entry{Source: task24Line("block-start", 0, "fragment", "identifier", "F")}
	fragment.Source.Number = 19
	_, problems := visitEntry(state, []*fragments.Entry{fragment}, 0, nil)
	task24Code(t, problems, CodeSource, input, 19)
}

func TestTask24WalkSequenceConsumesLeafAndClosesDocumentFrames(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("tag", 0, "task", "identifier", "a"),
		task24Line("tag", 1, "step", "title", "s"),
		task24Line("block-start", 1, "choice"),
		task24Line("content", 3, "", "content", "option"),
		task24Line("tag", 3, "answer", "content", "right"),
		task24Line("tag", 3, "hint", "content", "hint"),
		task24Line("block-end", 3, ""),
		task24Line("tag", 1, "note", "content", "after"))
	state := task24State(input)
	next, problems := visitEntry(state, entries, 0, nil)
	task24NoErrors(t, problems)
	if next != 1 {
		t.Fatalf("task next=%d", next)
	}
	next, problems = visitEntry(state, entries, 1, nil)
	task24NoErrors(t, problems)
	if next != 2 {
		t.Fatalf("step next=%d", next)
	}
	next, problems = visitEntry(state, entries, 2, nil)
	task24NoErrors(t, problems)
	if next != 7 || !slices.Equal(state.bindings[2].Entries, entries[2:7]) {
		t.Fatalf("leaf next=%d", next)
	}
	task24NoErrors(t, walkSequence(state, entries[next:], nil))
	task24Paths(t, state, "0:[0]:task", "0:[0 0]:step", "0:[0 0 0]:singleChoice", "0:[0 0 1]:note")
	if len(state.frames) != 0 {
		t.Fatalf("document frames: %+v", state.frames)
	}
}

func task24VisitPrefix(t *testing.T, state *builder, entries []*fragments.Entry, end int) {
	t.Helper()
	for at := 0; at < end; {
		next, problems := visitEntry(state, entries, at, nil)
		task24NoErrors(t, problems)
		if next <= at || next > end {
			t.Fatalf("prefix next=%d at=%d end=%d", next, at, end)
		}
		at = next
	}
}

func TestTask24DocumentEndClosesEverySemanticFrame(t *testing.T) {
	input, entries := task24Fixture(t,
		task24Line("block-start", 0, "variants"),
		task24Line("tag", 1, "variant", "name", "branch"),
		task24Line("tag", 1, "task", "identifier", "a"),
		task24Line("tag", 3, "step", "title", "s"),
		task24Line("block-end", 1, ""))
	state := task24State(input)
	task24VisitPrefix(t, state, entries, 4)
	if len(state.frames) != 4 {
		t.Fatalf("active frames=%d", len(state.frames))
	}
	task24NoErrors(t, walkSequence(state, nil, nil))
	if len(state.frames) != 0 {
		t.Fatalf("document retained frames: %+v", state.frames)
	}
	task24Paths(t, state, "0:[0]:variants", "0:[0 0]:variant", "0:[0 0 0]:task", "0:[0 0 0 0]:step")
}

func TestTask24DispatchSpeakingDoesNotCreateNode(t *testing.T) {
	input, entries := task24Fixture(t, task24Line("tag", 0, "task", "identifier", "a"), task24Line("tag", 1, "speaking"))
	state := task24State(input)
	task24VisitPrefix(t, state, entries, len(entries))
	task24Paths(t, state, "0:[0]:task")
	flag := state.bindings[0].Node(state.pages).Flags.Speaking
	if flag == nil || !*flag {
		t.Fatal("speaking was not dispatched to task")
	}
}
