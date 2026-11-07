package convert

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/resourceembed"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestResourcesFillMediaSelectionErrorsKeepModelAndUsage(t *testing.T) {
	for _, scenario := range []struct {
		name, raw, value, code string
		files                  []string
		makeDirectory          bool
	}{
		{"missing-search", "missing", "missing", resourcefind.CodeNotFound, []string{"other.mp3"}, false},
		{"ambiguous-search", "003", "003", resourcefind.CodeAmbiguous, []string{"a003.mp3", "b003.mp3"}, false},
		{"wrong-extension", `"media/video.mp4"`, "media/video.mp4", resourcefind.CodeExtension, []string{"video.mp4"}, false},
		{"missing-explicit-no-search-fallback", `"003.mp3"`, "003.mp3", filetree.CodeAccess, []string{"003.mp3"}, false},
		{"directory-explicit", `"media/folder.mp3"`, "media/folder.mp3", resourcefind.CodeKind, nil, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "media"), 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range scenario.files {
				resourcesWrite(t, filepath.Join(root, "media", name), []byte("unread"))
			}
			if scenario.makeDirectory {
				if err := os.Mkdir(filepath.Join(root, "media", "folder.mp3"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			input, tree := resourcesLoad(t, root, []string{"media"}, resourcesMediaLine(t, 1, "audio", scenario.raw, scenario.value))
			pages, bindings := resourcesBindings(t, input)
			usage := &resourceusage.Registry{}
			finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
			if problem != nil {
				t.Fatal(problem)
			}
			beforeModel, beforeInput := resourcesJSON(t, pages), resourcesJSON(t, input)
			beforeUsage := resourcesJSON(t, usage.Snapshot())
			problem = fillMedia(bindings[0].Node(pages), bindings[0], finder, usage, resourcesContext(input))
			if problem == nil || problem.Code != scenario.code {
				t.Fatalf("expected %s, got %+v", scenario.code, problem)
			}
			sourceContext := resourcesContext(input)
			sourceContext.Line, sourceContext.Column = 1, input.Result.Lines[0].Elements[2].Start
			if scenario.code == filetree.CodeAccess {
				if problem.Cause == nil || problem.Context.Path != filepath.Join(root, "003.mp3") || !slices.Contains(problem.Related, sourceContext) {
					t.Fatalf("lost path/cause/source coordinates: %+v", problem)
				}
			} else if problem.Context != sourceContext {
				t.Fatalf("wrong source coordinates: got %+v want %+v", problem.Context, sourceContext)
			}
			if scenario.code == resourcefind.CodeAmbiguous {
				want := []diagnostic.Context{{Path: filepath.Join(root, "media", "a003.mp3")}, {Path: filepath.Join(root, "media", "b003.mp3")}}
				if !slices.Equal(problem.Related, want) {
					t.Fatalf("lost candidates: %+v", problem.Related)
				}
			}
			if resourcesJSON(t, pages) != beforeModel || resourcesJSON(t, input) != beforeInput || resourcesJSON(t, usage.Snapshot()) != beforeUsage {
				t.Fatal("failed selection changed model, source or resource registry")
			}
		})
	}
}

func TestResourcesFillMediaRejectsForeignBodyFromRealBinding(t *testing.T) {
	root := t.TempDir()
	declaration := resourcesMediaLine(t, 1, "audio", `"clip.mp3"`, "clip.mp3")
	declaration.LineType = model.LineTypeBlockStart
	declaration = resourcesLine(t, 1, declaration.LineType, append(declaration.Elements,
		resourcesElement(t, model.ElementTypeBlockOpen, "{", "{"))...)
	body := resourcesLine(t, 2, model.LineTypeContent, resourcesElement(t, model.ElementTypeContent, "foreign body", "foreign body"))
	close := resourcesLine(t, 3, model.LineTypeBlockEnd, resourcesElement(t, model.ElementTypeBlockClose, "}", "}"))
	parent := 1
	body.ParentLine, close.ParentLine, body.NestingLevel, close.NestingLevel = &parent, &parent, 1, 1
	input, tree := resourcesLoad(t, root, nil, declaration, body, close)
	pages, bindings := resourcesBindings(t, input)
	if len(bindings) != 1 || len(bindings[0].Entries) != 3 {
		t.Fatalf("unexpected real media binding: %+v", bindings)
	}
	usage := &resourceusage.Registry{}
	finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
	if problem != nil {
		t.Fatal(problem)
	}
	before := resourcesJSON(t, pages)
	problem = fillMedia(bindings[0].Node(pages), bindings[0], finder, usage, resourcesContext(input))
	if problem == nil || problem.Code != CodeMediaSource {
		t.Fatalf("foreign body must not be dropped: %+v", problem)
	}
	if resourcesJSON(t, pages) != before || usage.Snapshot().Total != 0 {
		t.Fatal("invalid media changed node or usage")
	}
}

func TestResourcesFillMediaReadFailureRegistersBeforeEmbeddingWithoutUsage(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("реальная ошибка чтения /proc/self/mem проверяется только в Linux")
	}
	root := t.TempDir()
	path := filepath.Join(root, "unreadable.mp3")
	if err := os.Symlink("/proc/self/mem", path); err != nil {
		t.Fatal(err)
	}
	input, tree := resourcesLoad(t, root, nil, resourcesMediaLine(t, 1, "audio", `"unreadable.mp3"`, "unreadable.mp3"))
	pages, bindings := resourcesBindings(t, input)
	usage := &resourceusage.Registry{}
	finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
	if problem != nil {
		t.Fatal(problem)
	}
	if usage.Snapshot().Total != 0 {
		t.Fatal("explicit resource should not be accounted before Find")
	}
	before := resourcesJSON(t, pages)
	problem = fillMedia(bindings[0].Node(pages), bindings[0], finder, usage, resourcesContext(input))
	if problem == nil || problem.Code != resourceembed.CodeRead || problem.Context.Path != path || problem.Cause == nil {
		t.Fatalf("expected actual Embed read failure, got %+v", problem)
	}
	var pathError *os.PathError
	if !errors.As(problem, &pathError) || pathError.Op != "read" {
		t.Fatalf("expected underlying read failure, got %+v", problem)
	}
	context := resourcesContext(input)
	context.Line, context.Column = 1, input.Result.Lines[0].Elements[2].Start
	if !slices.Contains(problem.Related, context) {
		t.Fatalf("source coordinates lost: %+v", problem.Related)
	}
	report := usage.Snapshot()
	if report.Total != 1 || report.Used != 0 || report.Unused != 1 || !slices.Equal(report.UnusedPaths, []string{resourcesIdentity(t, tree, path)}) {
		t.Fatalf("selection must register before Embed and remain unused: %+v", report)
	}
	if resourcesJSON(t, pages) != before {
		t.Fatal("failed Embed changed the node")
	}
}
