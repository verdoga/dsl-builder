package filetree_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

func TestIntegrationJSONFilesUnitLayout(t *testing.T) {
	root := t.TempDir()
	want := []string{"U1/DV/Work.json", "U1/SB/A.json", "U1/SB/deep/ещё глубже/Урок.JSON", "U1/VOX/Work.json", "U1/WB/A.json"}
	for i, name := range want {
		path := integrationWrite(t, root, name, string([]byte{0xff, byte(i)}))
		stamp := time.Unix(int64(len(want)-i), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	integrationWrite(t, root, "U1/SB/Audio/ignored.json.txt", "{}")
	integrationWrite(t, root, "U1/WB/Audio/01.mp3", "not media")
	integrationWrite(t, root, "U1/DV/Video/clip.mp4", "")
	integrationWrite(t, root, "U1/VOX/Video/clip.mp4", "")
	var tree filetree.Resolver
	for attempt := 0; attempt < 2; attempt++ {
		entries, err := tree.JSONFiles(root)
		if err != nil {
			t.Fatal(err)
		}
		integrationPaths(t, root, entries, want)
		for _, entry := range entries {
			if entry.Info.Size() != 2 || !entry.Info.Mode().IsRegular() {
				t.Fatalf("invalid JSON must remain an ordinary discovered file: %+v", entry)
			}
		}
	}
}

func TestIntegrationSharedResolverKeepsJSONDiscoveryBase(t *testing.T) {
	root := t.TempDir()
	target := integrationWrite(t, root, "original/source.bin", "not JSON or DSL")
	local := integrationWrite(t, root, "lessons/SB/Audio/звук 01.mp3", "local")
	integrationWrite(t, root, "original/Audio/звук 01.mp3", "wrong base")
	alias := filepath.Join(root, "lessons", "SB", "A.JSON")
	integrationSymlink(t, target, alias)
	otherAlias := filepath.Join(root, "lessons", "WB", "A.JSON")
	integrationSymlink(t, target, otherAlias)
	var tree filetree.Resolver
	first, err := tree.Inspect(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{alias, otherAlias} {
		lesson, err := tree.JSONFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if lesson.Path != path || lesson.IdentityPath != first.IdentityPath || !os.SameFile(lesson.Info, first.Info) {
			t.Fatalf("shared identity must retain discovery address: %+v, first=%+v", lesson, first)
		}
	}
	lesson, err := tree.JSONFile(alias)
	if err != nil {
		t.Fatal(err)
	}
	resourcePath, err := filetree.Resolve(filepath.Dir(lesson.Path), filepath.Join("Audio", "звук 01.mp3"))
	if err != nil || resourcePath != local {
		t.Fatalf("resource base must be JSON Entry.Path directory: path=%q err=%v", resourcePath, err)
	}
	explicit, err := tree.Inspect(resourcePath)
	if err != nil {
		t.Fatal(err)
	}
	area, err := tree.Walk(filepath.Dir(local), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(area) != 1 || area[0].IdentityPath != explicit.IdentityPath {
		t.Fatalf("explicit path and resource area must share Resolver identity: %+v", area)
	}
	inputs, err := tree.JSONFiles(filepath.Join(root, "lessons"))
	if err != nil {
		t.Fatal(err)
	}
	integrationPaths(t, root, inputs, []string{"lessons/SB/A.JSON"})
}

func TestIntegrationSingleJSONDoesNotExpandDirectory(t *testing.T) {
	root := t.TempDir()
	integrationWrite(t, root, "lesson.JSON/inside.json", "")
	var tree filetree.Resolver
	directory := filepath.Join(root, "lesson.JSON")
	_, err := tree.JSONFile(directory)
	if err == nil || err.Code != filetree.CodeKind || err.Context.Path != directory {
		t.Fatalf("single-file input must reject a directory rather than expand it: %v", err)
	}
	entries, err := tree.JSONFiles(directory)
	if err != nil {
		t.Fatal(err)
	}
	integrationPaths(t, root, entries, []string{"lesson.JSON/inside.json"})
	if entries[0].Info.Size() != 0 {
		t.Fatal("empty JSON content belongs to parsed validation, not filetree")
	}
}

func TestIntegrationAliasesAreFilteredBeforeUnique(t *testing.T) {
	root := t.TempDir()
	target := integrationWrite(t, root, "a.bin", "invalid JSON")
	integrationSymlink(t, target, filepath.Join(root, "Z.JSON"))
	if err := os.Link(target, filepath.Join(root, "zz.json")); err != nil {
		t.Fatal(err)
	}
	var tree filetree.Resolver
	children, err := tree.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	integrationPaths(t, root, children, []string{"a.bin", "Z.JSON", "zz.json"})
	if len(filetree.Unique(children)) != 1 {
		t.Fatalf("symlink and hardlink must share one identity: %+v", children)
	}
	seen := map[string]bool{}
	files, err := tree.Walk(root, func(entry filetree.Entry) bool {
		name := filepath.Base(entry.Path)
		seen[name] = true
		return strings.HasPrefix(name, "Z")
	})
	if err != nil {
		t.Fatal(err)
	}
	integrationPaths(t, root, files, []string{"Z.JSON"})
	if len(seen) != 3 {
		t.Fatalf("filter must inspect each address before deduplication: %v", seen)
	}
	jsonFiles, err := tree.JSONFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	integrationPaths(t, root, jsonFiles, []string{"Z.JSON"})
	if jsonFiles[0].IdentityPath != children[0].IdentityPath {
		t.Fatal("JSON extension filter must retain the already established identity")
	}
}

func TestIntegrationOverlappingAreasAndFreshWalk(t *testing.T) {
	root := t.TempDir()
	media := integrationWrite(t, root, "Audio/01.mp3", "one")
	integrationWrite(t, root, "Audio/deeper/unknown.xyz", "unsupported but accounted")
	if err := os.Link(media, filepath.Join(root, "Audio", "02.mp3")); err != nil {
		t.Fatal(err)
	}
	integrationSymlink(t, root, filepath.Join(root, "Audio", "back"))
	var tree filetree.Resolver
	var combined []filetree.Entry
	for _, scope := range []string{root, filepath.Join(root, "Audio"), root} {
		files, err := tree.Walk(scope, nil)
		if err != nil || len(files) != 2 {
			t.Fatalf("each intersecting root must be walked: files=%+v err=%v", files, err)
		}
		combined = append(combined, files...)
	}
	if len(filetree.Unique(combined)) != 2 {
		t.Fatal("overlaps, hardlinks and an ancestor link must not duplicate physical files")
	}
	if err := os.WriteFile(media, []byte("changed bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	integrationWrite(t, root, "Audio/new.dat", "new")
	fresh, err := tree.Walk(root, nil)
	if err != nil || len(fresh) != 3 {
		t.Fatalf("same Resolver must enumerate changed tree again: %+v, %v", fresh, err)
	}
	for _, entry := range fresh {
		if entry.IdentityPath == combined[0].IdentityPath && entry.Info.Size() != int64(len("changed bytes")) {
			t.Fatalf("Inspect must refresh metadata: %+v", entry)
		}
	}
	var nextAnalysis filetree.Resolver
	again, err := nextAnalysis.Walk(root, nil)
	if err != nil || len(again) != 3 {
		t.Fatalf("new analysis must observe current filesystem: %+v, %v", again, err)
	}
	if err := os.RemoveAll(filepath.Join(root, "Audio")); err != nil {
		t.Fatal(err)
	}
	files, err := tree.Walk(filepath.Join(root, "Audio"), nil)
	integrationError(t, files, err, filetree.CodeAccess, filepath.Join(root, "Audio"))
}

func TestIntegrationBrokenEntryPreventsPartialResults(t *testing.T) {
	root := t.TempDir()
	integrationWrite(t, root, "a.json", "{}")
	loop := filepath.Join(root, "z-loop")
	integrationSymlink(t, loop, loop)
	var tree filetree.Resolver
	for _, operation := range []struct {
		name string
		run  func() ([]filetree.Entry, *diagnostic.Error)
	}{
		{"ReadDir", func() ([]filetree.Entry, *diagnostic.Error) { return tree.ReadDir(root) }},
		{"WalkAll", func() ([]filetree.Entry, *diagnostic.Error) { return tree.Walk(root, nil) }},
		{"JSONFiles", func() ([]filetree.Entry, *diagnostic.Error) { return tree.JSONFiles(root) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			entries, err := operation.run()
			integrationError(t, entries, err, filetree.CodeLink, loop)
		})
	}
	if err := os.Remove(loop); err != nil {
		t.Fatal(err)
	}
	var repaired filetree.Resolver
	entries, err := repaired.JSONFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	integrationPaths(t, root, entries, []string{"a.json"})
}

func integrationWrite(t *testing.T, root, relative, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func integrationSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("real symlink fixture requires filesystem support and permission: %v", err)
	}
}

func integrationPaths(t *testing.T, root string, entries []filetree.Entry, want []string) {
	t.Helper()
	got := make([]string, len(entries))
	for i, entry := range entries {
		relative, err := filetree.Relative(root, entry.Path)
		if err != nil {
			t.Fatal(err)
		}
		got[i] = relative
		if entry.IdentityPath == "" || !filepath.IsAbs(entry.IdentityPath) || entry.Info == nil {
			t.Fatalf("entry must provide full accessible identity and target metadata: %+v", entry)
		}
		info, statErr := os.Stat(entry.IdentityPath)
		if statErr != nil || !os.SameFile(info, entry.Info) {
			t.Fatalf("identity must address the actual object: %+v, %v", entry, statErr)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relative addresses: got %q, want %q", got, want)
	}
}

func integrationError(t *testing.T, entries []filetree.Entry, err *diagnostic.Error, code, path string) {
	t.Helper()
	if entries != nil || err == nil {
		t.Fatalf("operation must return nil entries and an error: entries=%+v err=%v", entries, err)
	}
	if err.Code != code || err.Context.Path != path || err.Reason == "" || err.Cause == nil {
		t.Fatalf("diagnostic must retain exact problem and cause: %+v", err)
	}
}
