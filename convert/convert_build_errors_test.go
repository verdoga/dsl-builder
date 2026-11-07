package convert_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/structure"
	"github.com/verdoga/dsl-builder/textdata"
)

func TestConvertBuildNilGuards(t *testing.T) {
	input, tree := convertBuildFixture(t, "metadata-first")
	for _, tc := range []struct {
		name              string
		nilInput, nilTree bool
	}{{"input", true, false}, {"tree", false, true}, {"both", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			in, resolver := input, tree
			if tc.nilInput {
				in = nil
			}
			if tc.nilTree {
				resolver = nil
			}
			value, usage, problems := convert.Build(in, resolver, convertBuildAt(), convertBuildContext())
			convertBuildFailed(t, value, usage, problems, convert.CodeInput)
		})
	}
}

func TestConvertBuildMetadataPrecedesFollowingStages(t *testing.T) {
	for _, name := range []string{"metadata-first", "unknown-linecount"} {
		t.Run(name, func(t *testing.T) {
			input, tree := convertBuildFixture(t, name)
			before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
			value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
			problem := convertBuildFailed(t, value, usage, problems, convert.CodeMetadata)
			field := "title"
			if name == "unknown-linecount" {
				field = "lineCount"
			}
			named := strings.Contains(problem.Reason, field)
			for _, detail := range problem.Details {
				named = named || strings.Contains(detail.Value, field) || strings.Contains(detail.Label, field)
			}
			if !named {
				t.Fatalf("missing metadata field %q not named: %+v", field, problem)
			}
			want := diagnostic.Context{Path: input.Path, FileName: "l016.json", LessonID: "BuIlD16"}
			if problem.Context != want {
				t.Fatalf("context=%+v; want %+v", problem.Context, want)
			}
			convertBuildUnchanged(t, input, before, fileBefore)
		})
	}
}

func TestConvertBuildOwnerStagesStopBeforeResources(t *testing.T) {
	for _, name := range []string{"fragments-first", "structure-first"} {
		t.Run(name, func(t *testing.T) {
			input, tree := convertBuildFixture(t, name)
			entries, expected := fragments.Expand(input)
			if name == "structure-first" {
				if len(expected) != 0 {
					t.Fatal(expected)
				}
				_, _, expected = structure.Build(input, entries)
			}
			if len(expected) == 0 {
				t.Fatal("fixture did not reach intended owner error")
			}
			before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
			value, usage, actual := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
			convertBuildFailed(t, value, usage, actual, expected[0].Code)
			if convertBuildJSON(t, actual) != convertBuildJSON(t, expected) {
				t.Fatalf("ordered owner errors changed:\ngot %s\nwant %s", convertBuildJSON(t, actual), convertBuildJSON(t, expected))
			}
			if name == "fragments-first" && len(actual) != 2 {
				t.Fatalf("want two ordered include errors: %v", actual)
			}
			convertBuildUnchanged(t, input, before, fileBefore)
		})
	}
}

func TestConvertBuildPrepareRunsBeforeDataAndWithoutMedia(t *testing.T) {
	for _, name := range []string{"prepare-first", "prepare-no-media"} {
		t.Run(name, func(t *testing.T) {
			input, tree := convertBuildFixture(t, name)
			before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
			value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
			problem := convertBuildFailed(t, value, usage, problems, filetree.CodeAccess)
			if problem.Context.Path != filepath.Join(filepath.Dir(input.Path), "absent") || !errors.Is(problem, os.ErrNotExist) {
				t.Fatalf("resource owner path/cause lost: %+v", problem)
			}
			want := diagnostic.Context{Path: input.Path, FileName: "l016.json", LessonID: "BuIlD16"}
			if !slices.Contains(problem.Related, want) {
				t.Fatalf("missing input context in %+v", problem.Related)
			}
			convertBuildUnchanged(t, input, before, fileBefore)
		})
	}
}

func TestConvertBuildDataOwnerErrorPreservedAndStopsAtFirstBinding(t *testing.T) {
	for _, tc := range []struct{ name, code string }{
		{"data-first", exercisedata.CodeSource},
		{"textdata-first", textdata.CodeExampleSeparator},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, tree := convertBuildFixture(t, tc.name)
			entries, expandErrors := fragments.Expand(input)
			if len(expandErrors) != 0 {
				t.Fatal(expandErrors)
			}
			_, bindings, structureErrors := structure.Build(input, entries)
			if len(structureErrors) != 0 || len(bindings) == 0 {
				t.Fatalf("fixture structure: %v", structureErrors)
			}
			binding := bindings[0]
			context := diagnostic.Context{Path: input.Path, FileName: "l016.json", LessonID: "BuIlD16", Line: binding.Entries[0].Source.Number}
			var expected *diagnostic.Error
			if tc.name == "data-first" {
				_, _, expected = exercisedata.Build(lesson.NodeQuestion, binding.Lines(), context)
			} else {
				declaration, lines := binding.TextSource()
				_, expected = textdata.Build(lesson.NodeExample, declaration, lines, context)
			}
			if expected == nil {
				t.Fatal("fixture did not cause a data owner error")
			}
			before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
			value, usage, problems := convert.Build(input, tree, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), convertBuildContext())
			problem := convertBuildFailed(t, value, usage, problems, tc.code)
			if problem.Code != expected.Code || problem.Reason != expected.Reason || problem.Context != expected.Context || problem.Cause != expected.Cause {
				t.Fatalf("owner error fields changed: got %+v want %+v", problem, expected)
			}
			if len(problem.Related) < len(expected.Related) || !slices.Equal(problem.Related[:len(expected.Related)], expected.Related) || len(problem.Details) < len(expected.Details) || !slices.Equal(problem.Details[:len(expected.Details)], expected.Details) {
				t.Fatal("owner Related/Details order was not preserved")
			}
			if len(problems) != 1 || problem.Context.Path != input.Path || problem.Context.LessonID != "BuIlD16" || problem.Context.FileName != "l016.json" || problem.Context.PageID != "" || problem.Context.Line == 0 {
				t.Fatalf("data owner error/context changed: %+v", problem)
			}
			if len(problem.Details) == 0 {
				t.Fatal("missing binding address details")
			}
			convertBuildUnchanged(t, input, before, fileBefore)
		})
	}
}

func TestConvertBuildLateInitialErrorDoesNotPublishUsage(t *testing.T) {
	input, tree := convertBuildFixture(t, "media")
	before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
	invalidAt := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	value, usage, problems := convert.Build(input, tree, invalidAt, convertBuildContext())
	problem := convertBuildFailed(t, value, usage, problems, lesson.CodeContract)
	if !slices.Contains(problem.Details, diagnostic.Detail{Label: "Поле JSON", Value: "/document/createdAt"}) {
		t.Fatalf("late date validation not propagated: %+v", problem)
	}
	convertBuildUnchanged(t, input, before, fileBefore)
	value, usage, problems = convert.Build(input, tree, convertBuildAt(), convertBuildContext())
	if value == nil || usage == nil || len(problems) != 0 || usage.Snapshot().Used != 3 {
		t.Fatalf("new local usage after failed call: value=%v usage=%v errors=%v", value, usage, problems)
	}
}

func TestConvertBuildOriginalGoldenMediaHaveNoSearchScope(t *testing.T) {
	for _, n := range []string{"1", "3"} {
		t.Run(n, func(t *testing.T) {
			tree := &filetree.Resolver{}
			path, err := filepath.Abs(filepath.Join("..", "golden", n+". parsed.json"))
			if err != nil {
				t.Fatal(err)
			}
			input := convertBuildLoad(t, path, tree)
			before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
			value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
			problem := convertBuildFailed(t, value, usage, problems, resourcefind.CodeNotFound)
			if len(problems) != 1 || problem.Context.Path != path || problem.Context.PageID != "" {
				t.Fatalf("unexpected error: %+v", problem)
			}
			convertBuildUnchanged(t, input, before, fileBefore)
		})
	}
}

func TestConvertBuildLaterMediaFailureDiscardsEarlierSuccessfulUse(t *testing.T) {
	input, tree := convertBuildFixture(t, "media-then-error")
	before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
	value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
	problem := convertBuildFailed(t, value, usage, problems, resourcefind.CodeNotFound)
	if len(problems) != 1 || problem.Context.Line != 4 || problem.Context.Column != 14 || problem.Context.Path != input.Path || problem.Context.PageID != "" {
		t.Fatalf("later resource failure lost its source: %+v", problem)
	}
	convertBuildUnchanged(t, input, before, fileBefore)
}
