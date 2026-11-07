package filetree_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

func walk14File(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not JSON: \xff"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func walk14Link(t *testing.T, target, path string, hard bool) {
	t.Helper()
	var err error
	if hard {
		err = os.Link(target, path)
	} else {
		err = os.Symlink(target, path)
	}
	if err != nil {
		t.Fatalf("create link fixture: %v", err)
	}
}

func walk14Names(t *testing.T, root string, entries []filetree.Entry) []string {
	t.Helper()
	names := make([]string, len(entries))
	for i, entry := range entries {
		if entry.Info == nil || entry.IdentityPath == "" || !filepath.IsAbs(entry.Path) || !filepath.IsAbs(entry.IdentityPath) {
			t.Fatalf("incomplete entry: %#v", entry)
		}
		rel, err := filepath.Rel(root, entry.Path)
		if err != nil {
			t.Fatal(err)
		}
		names[i] = filepath.ToSlash(rel)
	}
	return names
}

func walk14Want(t *testing.T, root string, entries []filetree.Entry, want ...string) {
	t.Helper()
	if got := walk14Names(t, root, entries); !slices.Equal(got, want) {
		t.Fatalf("paths = %q, want %q", got, want)
	}
}

func walk14Error(t *testing.T, entries []filetree.Entry, err *diagnostic.Error, code, path string, cause bool) {
	t.Helper()
	if entries != nil {
		t.Errorf("partial result on error: %#v", entries)
	}
	if err == nil {
		t.Fatal("expected diagnostic error")
	}
	if err.Code != code || err.Context.Path != path || err.Reason == "" {
		t.Errorf("diagnostic = %#v; want %s at %q", err, code, path)
	}
	if err.Context.Line != 0 || err.Context.Column != 0 {
		t.Error("invented coordinates")
	}
	if cause && err.Cause == nil {
		t.Error("missing underlying cause")
	}
}

func TestWalk14ReadDirImmediateChildrenAndAliases(t *testing.T) {
	root := t.TempDir()
	file := walk14File(t, root, "z.bin")
	walk14File(t, root, "Каталог с пробелом/deep.json")
	walk14Link(t, file, filepath.Join(root, "A.JSON"), false)
	walk14Link(t, file, filepath.Join(root, "b.hard"), true)
	walk14Link(t, filepath.Join(root, "Каталог с пробелом"), filepath.Join(root, "c-dir"), false)
	var r filetree.Resolver
	entries, err := r.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	walk14Want(t, root, entries, "A.JSON", "b.hard", "c-dir", "z.bin", "Каталог с пробелом")
	for _, pair := range [][2]int{{0, 1}, {0, 3}, {2, 4}} {
		if entries[pair[0]].IdentityPath != entries[pair[1]].IdentityPath || !os.SameFile(entries[pair[0]].Info, entries[pair[1]].Info) {
			t.Error("aliases lost target identity or Info")
		}
	}
	if entries[0].Info.IsDir() || !entries[2].Info.IsDir() || !entries[4].Info.IsDir() {
		t.Error("Info does not describe target kind")
	}
}

func TestWalk14EmptyAndAllExtensionsDeepTree(t *testing.T) {
	root := t.TempDir()
	var r filetree.Resolver
	for _, read := range []func(string) ([]filetree.Entry, *diagnostic.Error){r.ReadDir, func(p string) ([]filetree.Entry, *diagnostic.Error) { return r.Walk(p, nil) }} {
		entries, err := read(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("empty directory = %v, %v", entries, err)
		}
	}
	want := []string{"U1/DV/Work.json", "U1/SB/A.JSON", "U1/SB/Audio/track.mp3", "U1/WB/A.json", "no-extension", "unknown.xyz", "я файл.TXT"}
	for _, name := range want {
		walk14File(t, root, name)
	}
	deep := strings.Repeat("d/", 350) + "leaf.any"
	walk14File(t, root, deep)
	want = []string{deep, "no-extension", "U1/DV/Work.json", "U1/SB/A.JSON", "U1/SB/Audio/track.mp3", "U1/WB/A.json", "unknown.xyz", "я файл.TXT"}
	entries, err := r.Walk(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	walk14Want(t, root, entries, want...)
	for _, entry := range entries {
		if !entry.Info.Mode().IsRegular() {
			t.Error("Walk returned non-regular file")
		}
	}
}

func TestWalk14FilterEveryFileAddressBeforeUnique(t *testing.T) {
	root := t.TempDir()
	target := walk14File(t, root, "a.bin")
	walk14Link(t, target, filepath.Join(root, "b.JSON"), false)
	walk14Link(t, target, filepath.Join(root, "c.json"), true)
	walk14File(t, root, "nested/d.JSON")
	var r filetree.Resolver
	var seen []string
	entries, err := r.Walk(root, func(entry filetree.Entry) bool {
		if !entry.Info.Mode().IsRegular() {
			t.Error("filter called for a directory")
		}
		seen = append(seen, filepath.Base(entry.Path))
		return strings.EqualFold(filepath.Ext(entry.Path), ".json")
	})
	if err != nil {
		t.Fatal(err)
	}
	walk14Want(t, root, entries, "b.JSON", "nested/d.JSON")
	slices.Sort(seen)
	if !slices.Equal(seen, []string{"a.bin", "b.JSON", "c.json", "d.JSON"}) {
		t.Fatalf("filter calls = %q", seen)
	}
	all, err := r.Walk(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	walk14Want(t, root, all, "a.bin", "nested/d.JSON")
	if entries[0].IdentityPath != all[0].IdentityPath {
		t.Error("identity changed across filters")
	}
	none, err := r.Walk(root, func(filetree.Entry) bool { return false })
	if err != nil || len(none) != 0 {
		t.Fatalf("exclude all = %v, %v", none, err)
	}
}

func TestWalk14DirectoryAliasesBacklinkAndRepeatedRoots(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	walk14File(t, outside, "leaf.bin")
	walk14File(t, outside, "nested/child.json")
	walk14Link(t, outside, filepath.Join(root, "A-dir"), false)
	walk14Link(t, outside, filepath.Join(root, "z-dir"), false)
	walk14Link(t, root, filepath.Join(outside, "back"), false)
	var r filetree.Resolver
	for i := 0; i < 2; i++ {
		entries, err := r.Walk(root, nil)
		if err != nil {
			t.Fatal(err)
		}
		walk14Want(t, root, entries, "A-dir/leaf.bin", "A-dir/nested/child.json")
	}
	entries, err := r.Walk(filepath.Join(root, "z-dir", "nested"), nil)
	if err != nil {
		t.Fatal(err)
	}
	walk14Want(t, root, entries, "z-dir/nested/child.json")
	walk14File(t, outside, "new.ext")
	entries, err = r.Walk(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	walk14Want(t, root, entries, "A-dir/leaf.bin", "A-dir/nested/child.json", "A-dir/new.ext")
	if err := os.Remove(filepath.Join(outside, "nested", "child.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(outside, "nested")); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "z-dir", "nested")
	entries, err = r.Walk(missing, nil)
	walk14Error(t, entries, err, filetree.CodeAccess, missing, true)
}

func TestWalk14OrderingIgnoresDatesAndUsesCaseTieBreaker(t *testing.T) {
	root := t.TempDir()
	names := []string{"a-dir/z", "a.txt", "B.txt", "z.txt"}
	if runtime.GOOS != "windows" {
		names = []string{"A.txt", "a-dir/z", "a.txt", "B.txt", "z.txt"}
	}
	for i := len(names) - 1; i >= 0; i-- {
		path := walk14File(t, root, names[i])
		stamp := time.Unix(int64(len(names)-i), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	var r filetree.Resolver
	entries, err := r.Walk(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	// '-' precedes '.', so a-dir/z precedes a.txt.
	if runtime.GOOS != "windows" {
		names = []string{"a-dir/z", "A.txt", "a.txt", "B.txt", "z.txt"}
	}
	walk14Want(t, root, entries, names...)
}

func TestWalk14MetadataDoesNotRequireReadingFileContent(t *testing.T) {
	root := t.TempDir()
	path := walk14File(t, root, "invalid.JSON")
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	var r filetree.Resolver
	entries, err := r.Walk(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	walk14Want(t, root, entries, "invalid.JSON")
}
