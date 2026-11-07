package convert

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestResourcesPrepareRegistersAllDeclaredFilesWithoutMedia(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "ресурсы", "a.mp3")
	b := filepath.Join(root, "ресурсы", "подкаталог", "unsupported.bin")
	resourcesWrite(t, a, []byte{0, 1})
	resourcesWrite(t, b, []byte{2, 3})
	input, tree := resourcesLoad(t, root, []string{"ресурсы", "ресурсы/подкаталог", "ресурсы"})
	pages, bindings := resourcesBindings(t, input)
	if len(bindings) != 0 || len(pages) == 0 {
		t.Fatal("expected a real lesson structure without media")
	}
	before := resourcesJSON(t, input)
	usage := &resourceusage.Registry{}
	finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
	if problem != nil || finder == nil {
		t.Fatalf("prepareResources: %+v", problem)
	}
	want := []string{resourcesIdentity(t, tree, a), resourcesIdentity(t, tree, b)}
	slices.Sort(want)
	report := usage.Snapshot()
	if report.Total != 2 || report.Used != 0 || report.Unused != 2 || !slices.Equal(report.UnusedPaths, want) {
		t.Fatalf("unexpected inventory: %+v, want %v", report, want)
	}
	if resourcesJSON(t, input) != before {
		t.Fatal("prepareResources changed loaded input")
	}
}

func TestResourcesPrepareValidatesEveryDirectoryWithoutMedia(t *testing.T) {
	for _, mode := range []string{"missing", "file"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			resourcesWrite(t, filepath.Join(root, "good", "unused.bin"), []byte("unused"))
			bad := filepath.Join(root, "bad")
			if mode == "file" {
				resourcesWrite(t, bad, []byte("not a directory"))
			}
			input, tree := resourcesLoad(t, root, []string{"good", "good", "bad"})
			resourcesBindings(t, input)
			usage := &resourceusage.Registry{}
			finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
			wantCode := filetree.CodeAccess
			if mode == "file" {
				wantCode = filetree.CodeKind
			}
			if problem == nil || finder != nil || problem.Code != wantCode || problem.Context.Path != bad {
				t.Fatalf("expected %s at %s, got finder=%v problem=%+v", wantCode, bad, finder, problem)
			}
			if mode == "missing" && !errors.Is(problem, fs.ErrNotExist) {
				t.Fatalf("missing filesystem cause: %+v", problem)
			}
			if usage.Snapshot().Total != 0 {
				t.Fatal("failed Prepare registered partial inventory")
			}
		})
	}
}

func TestResourcesPrepareWithoutDirectoriesDoesNotSearchNeighbours(t *testing.T) {
	for _, dirs := range [][]string{nil, {}} {
		root := t.TempDir()
		resourcesWrite(t, filepath.Join(root, "003.mp3"), []byte("neighbour"))
		input, tree := resourcesLoad(t, root, dirs, resourcesMediaLine(t, 1, "audio", "003", "003"))
		usage := &resourceusage.Registry{}
		finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
		if problem != nil {
			t.Fatal(problem)
		}
		if usage.Snapshot().Total != 0 {
			t.Fatal("implicit inventory appeared")
		}
		pages, bindings := resourcesBindings(t, input)
		node := bindings[0].Node(pages)
		before := resourcesJSON(t, pages)
		problem = fillMedia(node, bindings[0], finder, usage, resourcesContext(input))
		if problem == nil || problem.Code != resourcefind.CodeNotFound {
			t.Fatalf("unexpected fallback: %+v", problem)
		}
		if resourcesJSON(t, pages) != before || usage.Snapshot().Used != 0 {
			t.Fatal("failed selection changed model or usage")
		}
	}
}

func TestResourcesPrepareUsesProvidedResolverIdentity(t *testing.T) {
	root := t.TempDir()
	body := filepath.Join(root, "media", "clip.mp3")
	alias := filepath.Join(root, "first.mp3")
	resourcesWrite(t, body, []byte{0, 1, 2})
	if err := os.Link(body, alias); err != nil {
		t.Fatal(err)
	}
	input, tree := resourcesLoad(t, root, []string{"media"}, resourcesMediaLine(t, 1, "audio", "clip", "clip"))
	identity := resourcesIdentity(t, tree, alias)
	usage := &resourceusage.Registry{}
	finder, problem := prepareResources(input, tree, usage, resourcesContext(input))
	if problem != nil {
		t.Fatal(problem)
	}
	if !slices.Equal(usage.Snapshot().UnusedPaths, []string{identity}) {
		t.Fatalf("caller identity replaced: %+v", usage.Snapshot())
	}
	pages, bindings := resourcesBindings(t, input)
	if problem := fillMedia(bindings[0].Node(pages), bindings[0], finder, usage, resourcesContext(input)); problem != nil {
		t.Fatal(problem)
	}
	if !slices.Equal(usage.Snapshot().UsedPaths, []string{identity}) {
		t.Fatalf("selection identity replaced: %+v", usage.Snapshot())
	}
}
