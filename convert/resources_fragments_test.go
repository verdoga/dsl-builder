package convert

import (
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestResourcesFillMediaUsesPhysicalSourceInIndependentIncludes(t *testing.T) {
	root := t.TempDir()
	resourcesWrite(t, filepath.Join(root, "media", "other.mp3"), []byte("other"))
	declaration := resourcesLine(t, 1, model.LineTypeBlockStart,
		resourcesElement(t, model.ElementTypeTag, "@fragment", "fragment"),
		resourcesElement(t, model.ElementTypeIdentifier, "clip", "clip"),
		resourcesElement(t, model.ElementTypeBlockOpen, "{", "{"))
	media := resourcesMediaLine(t, 2, "audio", "missing", "missing")
	close := resourcesLine(t, 3, model.LineTypeBlockEnd, resourcesElement(t, model.ElementTypeBlockClose, "}", "}"))
	parent := 1
	media.ParentLine, close.ParentLine, media.NestingLevel, close.NestingLevel = &parent, &parent, 1, 1
	include := func(number int) model.Line {
		return resourcesLine(t, number, model.LineTypeTag,
			resourcesElement(t, model.ElementTypeTag, "@include", "include"),
			resourcesElement(t, model.ElementTypeIdentifier, "clip", "clip"))
	}
	input, tree := resourcesLoad(t, root, []string{"media"}, declaration, media, close, include(4), include(5))
	pages, bindings := resourcesBindings(t, input)
	if len(bindings) != 2 || bindings[0].Entries[0] == bindings[1].Entries[0] {
		t.Fatal("expected independent real fragment instances")
	}
	beforeInput, beforePages := resourcesJSON(t, input), resourcesJSON(t, pages)
	usage := &resourceusage.Registry{}
	finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
	if problem != nil {
		t.Fatal(problem)
	}
	for _, binding := range bindings {
		if binding.Node(pages).Type != lesson.NodeMedia {
			t.Fatal("expected media node")
		}
		problem := fillMedia(binding.Node(pages), binding, finder, usage, resourcesContext(input))
		if problem == nil || problem.Code != resourcefind.CodeNotFound || problem.Context.Line != 2 || problem.Context.Column != 14 {
			t.Fatalf("include replaced original source coordinates: %+v", problem)
		}
	}
	if resourcesJSON(t, input) != beforeInput || resourcesJSON(t, pages) != beforePages || usage.Snapshot().Used != 0 {
		t.Fatal("failed media in fragment changed input, instances or usage")
	}
}
