package resourcefind

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
)

func TestScopeInventoryIncludesEveryExtensionAndDepth(t *testing.T) {
	root := t.TempDir()
	names := []string{"Z.mp3", "a.MP4", "b.jpg", "c.txt", "d.vtt", "e.xyz", "no-extension", ".hidden", "deep/ещё/001.bin"}
	want := make([]string, 0, len(names))
	for _, name := range names {
		want = append(want, scopeWrite(t, root, name))
	}
	slices.SortFunc(want, filetree.ComparePaths)
	tree := &filetree.Resolver{}
	f := &Finder{tree: tree, directories: []string{filepath.Join(root, "deep"), root, root}}
	context := scopeContext(root)
	for range 2 {
		got, err := f.inventory(context)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("inventory = %q, %v; want %q", got, err, want)
		}
		got[0] = "changed"
	}
	added := scopeWrite(t, root, "new.unknown")
	want = append(want, added)
	slices.SortFunc(want, filetree.ComparePaths)
	got, err := f.inventory(context)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("повторный inventory не проверил FS: %q, %v", got, err)
	}
}

func TestScopeInventoryEmptyScopeAndDirectory(t *testing.T) {
	root := t.TempDir()
	for _, dirs := range [][]string{nil, {}, {root, root}} {
		f := &Finder{tree: &filetree.Resolver{}, directories: dirs}
		got, err := f.inventory(scopeContext(root))
		if err != nil || len(got) != 0 {
			t.Fatalf("empty inventory = %q, %v", got, err)
		}
	}
}

func TestScopeInventoryAliasesShareOperationIdentity(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	original := scopeWrite(t, outside, "original.unknown")
	hard := filepath.Join(root, "hard.mp3")
	symbolic := filepath.Join(root, "alias.txt")
	if err := os.Link(original, hard); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, symbolic); err != nil {
		t.Fatal(err)
	}
	tree := &filetree.Resolver{}
	entry, err := tree.Inspect(original)
	if err != nil {
		t.Fatal(err)
	}
	f := &Finder{tree: tree, directories: []string{root, root}}
	got, issue := f.inventory(scopeContext(root))
	if issue != nil || !slices.Equal(got, []string{entry.IdentityPath}) {
		t.Fatalf("inventory aliases = %q, %v", got, issue)
	}
}

func TestScopeInventoryErrorsNeverReturnPartialPaths(t *testing.T) {
	root := t.TempDir()
	good := scopeWrite(t, root, "good.mp3")
	missing := filepath.Join(root, "missing")
	for _, bad := range []string{missing, good, "relative", "bad\x00path"} {
		t.Run(filepath.Base(bad), func(t *testing.T) {
			tree := &filetree.Resolver{}
			_, expected := tree.Walk(bad, nil)
			if expected == nil {
				t.Fatal("fixture не вызывает ошибку Walk")
			}
			f := &Finder{tree: tree, directories: []string{root, root, bad}}
			context := scopeContext(root)
			got, issue := f.inventory(context)
			if got != nil {
				t.Fatalf("частичный inventory: %q", got)
			}
			scopeError(t, issue, expected, context)
		})
	}
	brokenRoot := t.TempDir()
	scopeWrite(t, brokenRoot, "a.mp3")
	if err := os.Symlink(filepath.Join(brokenRoot, "absent"), filepath.Join(brokenRoot, "z-broken")); err != nil {
		t.Fatal(err)
	}
	tree := &filetree.Resolver{}
	_, expected := tree.Walk(brokenRoot, nil)
	f := &Finder{tree: tree, directories: []string{root, brokenRoot}}
	got, issue := f.inventory(scopeContext(root))
	if got != nil || expected == nil {
		t.Fatalf("broken inventory = %q, %v", got, issue)
	}
	scopeError(t, issue, expected, scopeContext(root))
}

func TestScopeInventoryRepeatedDirectoryRechecksAvailability(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "resources")
	scopeWrite(t, dir, "clip.mp3")
	f := &Finder{tree: &filetree.Resolver{}, directories: []string{dir, dir}}
	context := scopeContext(root)
	if _, err := f.inventory(context); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	_, expected := f.tree.Walk(dir, nil)
	got, issue := f.inventory(context)
	if got != nil {
		t.Fatalf("кэшированный успех: %q", got)
	}
	scopeError(t, issue, expected, context)
}
