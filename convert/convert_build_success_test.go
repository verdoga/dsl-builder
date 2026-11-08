package convert_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
)

func TestConvertBuildEmptyKnownMetadataAndNullPlacement(t *testing.T) {
	input, tree := convertBuildFixture(t, "empty")
	before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
	value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
	if value == nil || usage == nil || problems != nil {
		t.Fatalf("value=%v usage=%v errors=%v", value, usage, problems)
	}
	if value.FormatVersion != lesson.FormatVersion || value.Document.ID != "BuIlD16" || value.Document.Title != "" || value.Document.Subtitle != nil || value.Document.LineCount.String() != "0" || value.Document.Version.String() != "1" || value.Document.CreatedAt != "2024-02-28T18:45:16Z" || value.Document.Pages == nil {
		t.Fatalf("wrong primary document: %+v", value.Document)
	}
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatal(problems)
	}
	if usage.Snapshot().Total != 0 {
		t.Fatal("empty resourceDirs created an automatic scope")
	}
	metadata := input.Result.Document.Metadata
	if metadata.Section != nil || metadata.Order != nil || metadata.ResourceDirs != nil {
		t.Fatalf("null metadata changed: %+v", metadata)
	}
	convertBuildUnchanged(t, input, before, fileBefore)
}

func TestConvertBuildGoldenStructureAndPrimaryIDs(t *testing.T) {
	for _, n := range []string{"4", "6", "7"} {
		t.Run(n, func(t *testing.T) {
			tree := &filetree.Resolver{}
			path, err := filepath.Abs(filepath.Join("..", "golden", n+". parsed.json"))
			if err != nil {
				t.Fatal(err)
			}
			input := convertBuildLoad(t, path, tree)
			expectedPath := filepath.Join("..", "golden", n+". lesson.json")
			if n == "6" {
				expectedPath = filepath.Join("testdata", "integration", "golden-6-tz.expected.json")
			}
			var expected lesson.Lesson
			if err := json.Unmarshal(convertBuildRead(t, expectedPath), &expected); err != nil {
				t.Fatal(err)
			}
			expected.Document.CreatedAt = "2024-02-28T18:45:16Z"
			before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
			value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
			if value == nil || usage == nil || problems != nil {
				t.Fatalf("value=%v usage=%v errors=%v", value, usage, problems)
			}
			if convertBuildJSON(t, value) != convertBuildJSON(t, expected) {
				t.Fatalf("converted golden %s differs from independent expected model", n)
			}
			if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
				t.Fatal(problems)
			}
			if usage.Snapshot().Total != 0 {
				t.Fatal("no media golden unexpectedly accounted resources")
			}
			convertBuildUnchanged(t, input, before, fileBefore)
		})
	}
}

func TestConvertBuildRepeatedCallsReturnIndependentResults(t *testing.T) {
	input, tree := convertBuildFixture(t, "question")
	before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
	first, firstUsage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
	if first == nil || firstUsage == nil || problems != nil {
		t.Fatalf("first: %v", problems)
	}
	want := convertBuildJSON(t, first)
	question := &first.Document.Pages[0].Nodes[0]
	if question.Flags.Checkable == nil || *question.Flags.Checkable {
		t.Fatal("owner false must be an explicit flag")
	}
	if first.Document.Subtitle == input.Result.Document.Metadata.Subtitle {
		t.Fatal("subtitle aliases parsed input")
	}
	*first.Document.Subtitle = "changed"
	*question.Flags.Checkable = true
	question.Data[0].Value = "changed"
	role := lesson.RoleHint
	question.Data[0].Role = &role
	question.Data[0].Answers = append(question.Data[0].Answers, "changed")
	first.Document.Pages[0].ID = "changed"
	firstUsage.MarkUsed([]string{filepath.Join(filepath.Dir(input.Path), "extra")})
	second, secondUsage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
	if second == nil || secondUsage == nil || problems != nil {
		t.Fatalf("second: %v", problems)
	}
	if convertBuildJSON(t, second) != want || second == first || secondUsage == firstUsage || secondUsage.Snapshot().Total != 0 {
		t.Fatal("repeat call shared result or usage state")
	}
	metadata := input.Result.Document.Metadata
	if metadata.Section == nil || *metadata.Section != "MiXeD Section" || metadata.Order == nil || *metadata.Order != "000123456789012345678901234567890" {
		t.Fatalf("caller placement metadata changed: %+v", metadata)
	}
	encoded := convertBuildJSON(t, second)
	for _, forbidden := range []string{"resourceDirs", "section", "order", "processing", "diagnostics", "filePath"} {
		if strings.Contains(encoded, `"`+forbidden+`"`) {
			t.Fatalf("service metadata %q leaked into lesson", forbidden)
		}
	}
	convertBuildUnchanged(t, input, before, fileBefore)
}

func TestConvertBuildMediaRegistersInventoryAndOnlyEmbeddedPaths(t *testing.T) {
	input, tree := convertBuildFixture(t, "media")
	before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
	original := map[string][]byte{}
	for _, name := range []string{"Clip 003.mp4", "Clip 003.txt", "Clip 003.vtt", "unused.bin"} {
		original[name] = convertBuildRead(t, filepath.Join(filepath.Dir(input.Path), "assets", name))
	}
	value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
	if value == nil || usage == nil || problems != nil {
		t.Fatalf("value=%v usage=%v errors=%v", value, usage, problems)
	}
	if problems := lesson.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatal(problems)
	}
	nodes := value.Document.Pages[0].Nodes
	if len(nodes) != 3 || nodes[0].Type != lesson.NodeMedia || nodes[1].Type != lesson.NodeScript || nodes[2].Type != lesson.NodeMedia {
		t.Fatalf("source script was not preserved independently: %+v", nodes)
	}
	for _, index := range []int{0, 2} {
		if len(nodes[index].Data) != 3 {
			t.Fatal("selected companions missing")
		}
		for i, name := range []string{"Clip 003.mp4", "Clip 003.txt", "Clip 003.vtt"} {
			datum := nodes[index].Data[i]
			wantRole := []lesson.DataRole{lesson.RoleBody, lesson.RoleScript, lesson.RoleSubtitles}[i]
			file, ok := datum.Value.(lesson.EmbeddedFile)
			if !ok || file.FileName != name || datum.Role == nil || *datum.Role != wantRole {
				t.Fatalf("wrong embedded file: %+v", datum)
			}
			bytes, err := base64.StdEncoding.DecodeString(file.Content)
			if err != nil || string(bytes) != string(original[name]) {
				t.Fatalf("file bytes changed: %s", name)
			}
		}
	}
	if nodes[1].Data[0].Value != "source script" {
		t.Fatalf("wrong source script: %+v", nodes[1])
	}
	report := usage.Snapshot()
	if report.Total != 4 || report.Used != 3 || report.Unused != 1 || !slices.Equal(report.UnusedPaths, []string{filepath.Join(filepath.Dir(input.Path), "assets", "unused.bin")}) {
		t.Fatalf("wrong unique usage: %+v", report)
	}
	for name, bytes := range original {
		if string(convertBuildRead(t, filepath.Join(filepath.Dir(input.Path), "assets", name))) != string(bytes) {
			t.Fatalf("Build changed resource file %s", name)
		}
	}
	convertBuildUnchanged(t, input, before, fileBefore)
}

func TestConvertBuildUsesSameResolverIdentity(t *testing.T) {
	input, tree := convertBuildFixture(t, "media")
	canonical := filepath.Join(filepath.Dir(input.Path), "canonical.mp4")
	if err := os.Link(filepath.Join(filepath.Dir(input.Path), "assets", "Clip 003.mp4"), canonical); err != nil {
		t.Fatal(err)
	}
	entry, problem := tree.Inspect(canonical)
	if problem != nil {
		t.Fatal(problem)
	}
	value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
	if value == nil || usage == nil || problems != nil {
		t.Fatalf("Build: %v", problems)
	}
	if !slices.Contains(usage.Snapshot().UsedPaths, entry.IdentityPath) {
		t.Fatalf("Build replaced the operation Resolver: %+v", usage.Snapshot())
	}
}

func TestConvertBuildInventoryWithoutMediaIsReturnedUnused(t *testing.T) {
	input, tree := convertBuildFixture(t, "inventory-no-media")
	before, fileBefore := convertBuildJSON(t, input), convertBuildRead(t, input.Path)
	value, usage, problems := convert.Build(input, tree, convertBuildAt(), convertBuildContext())
	if value == nil || usage == nil || problems != nil {
		t.Fatalf("Build: %v", problems)
	}
	report := usage.Snapshot()
	if report.Total != 4 || report.Used != 0 || report.Unused != 4 {
		t.Fatalf("Prepare inventory was skipped without media: %+v", report)
	}
	convertBuildUnchanged(t, input, before, fileBefore)
}
