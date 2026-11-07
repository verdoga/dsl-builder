package resourcefind_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
	rf "github.com/verdoga/dsl-builder/resourcefind"
)

func TestFinderPrepareValidatesAllDeclarationsWithoutFind(t *testing.T) {
	for _, bad := range []string{"missing", "Audio/missing", "not-directory", ""} {
		t.Run(bad, func(t *testing.T) {
			root := t.TempDir()
			json := finderWrite(t, root, "A.json")
			finderWrite(t, root, "Audio/unused.mp3")
			finderWrite(t, root, "not-directory")
			var tree filetree.Resolver
			dirs := []string{"Audio", "Audio", bad}
			before := slices.Clone(dirs)
			f, paths, err := rf.Prepare(&tree, json, dirs, finderContext(json))
			if err == nil || f != nil || paths != nil {
				t.Fatalf("Prepare must fail atomically without media calls: f=%v paths=%q err=%v", f, paths, err)
			}
			if !slices.Equal(dirs, before) {
				t.Fatal("Prepare изменил входные объявления")
			}
		})
	}
}

func TestFinderPrepareOwnsInputsAndInventoryResults(t *testing.T) {
	root := t.TempDir()
	json := finderWrite(t, root, "A.json")
	first := finderWrite(t, root, "Audio/a.mp3")
	unknown := finderWrite(t, root, "Audio/deep/unknown.xyz")
	second := finderWrite(t, root, "Other/b.mp3")
	var tree filetree.Resolver
	dirs := []string{"Audio", "Audio/deep", "Audio"}
	f, inventory := finderPrepare(t, &tree, json, dirs)
	finderPaths(t, inventory, []string{first, unknown})
	dirs[0], dirs[1], dirs[2] = "Other", "Other", "Other"
	inventory[0] = second
	selected := finderSelect(t, f, "audio", "a.mp3", "a.mp3", json)
	if selected.Body.Entry.Path != first {
		t.Fatalf("Prepare retained caller slices: %+v", selected)
	}
	got, err := f.Find("audio", "b.mp3", "b.mp3", finderContext(json))
	finderZero(t, got, err, rf.CodeNotFound)
	_, again := finderPrepare(t, &tree, json, []string{"Audio"})
	finderPaths(t, again, []string{first, unknown})
}

func TestFinderEmptyDirectoriesAllowOnlyExplicitPaths(t *testing.T) {
	for _, dirs := range [][]string{nil, {}} {
		t.Run("empty", func(t *testing.T) {
			root := t.TempDir()
			json := finderWrite(t, root, "A.json")
			image := finderWrite(t, root, "рядом 001.png")
			var tree filetree.Resolver
			f, paths := finderPrepare(t, &tree, json, dirs)
			if len(paths) != 0 {
				t.Fatalf("empty scope scanned neighbours: %q", paths)
			}
			got, err := f.Find("image", "001", "001", finderContext(json))
			finderZero(t, got, err, rf.CodeNotFound)
			selected := finderSelect(t, f, "image", `"рядом 001.png"`, filepath.Base(image), json)
			finderPaths(t, selected.AccountedPaths, []string{image})
			got, err = f.Find("image", "001", "001", finderContext(json))
			finderZero(t, got, err, rf.CodeNotFound)
		})
	}
}

func TestFinderNewAnalysisUsesFreshResolverAfterRepair(t *testing.T) {
	root := t.TempDir()
	json := finderWrite(t, root, "A.json")
	old := finderWrite(t, root, "Audio/old.mp3")
	var first filetree.Resolver
	f, paths := finderPrepare(t, &first, json, []string{"Audio"})
	finderPaths(t, paths, []string{old})
	got, err := f.Find("audio", "new", "new", finderContext(json))
	finderZero(t, got, err, rf.CodeNotFound)
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	fresh := finderWrite(t, root, "Audio/new.mp3")
	var second filetree.Resolver
	repaired, paths := finderPrepare(t, &second, json, []string{"Audio"})
	finderPaths(t, paths, []string{fresh})
	selected := finderSelect(t, repaired, "audio", "new", "new", json)
	if selected.Body.Entry.Path != fresh {
		t.Fatalf("повторный анализ не увидел исправление: %+v", selected)
	}
}
