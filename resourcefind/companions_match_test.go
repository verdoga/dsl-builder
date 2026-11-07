package resourcefind

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
)

func TestRF17CompanionFullBasenameAndRegularFiles(t *testing.T) {
	for _, tc := range []struct {
		name, base, extension, file string
		match, directory            bool
	}{
		{"full basename", "Track.part", ".txt", "Track.part.txt", true, false},
		{"case folded", "Урок.part", ".TxT", "урок.PART.tXt", true, false},
		{"prefix rejected", "Track", ".txt", "Track-extra.txt", false, false},
		{"suffix rejected", "Track", ".txt", "extra-Track.txt", false, false},
		{"first extension retained", "Track.part", ".txt", "Track.txt", false, false},
		{"last extension only", "Track", ".txt", "Track.txt.bak", false, false},
		{"other extension", "Track", ".txt", "Track.vtt", false, false},
		{"spaces preserved", " Track ", ".txt", "Track.txt", false, false},
		{"directory rejected", "Track", ".txt", "Track.txt", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.file)
			if tc.directory {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				rf17Write(t, path)
			}
			tree := new(filetree.Resolver)
			entries := rf17ReadDir(t, tree, dir)
			got, err := companion(entries, tc.base, tc.extension, rf17Context(dir))
			if err != nil || (got != nil) != tc.match {
				t.Fatalf("companion = %+v, %v; match = %v", got, err, tc.match)
			}
			if tc.match && (got.Entry.Path != path || got.MIMEType != "text/plain") {
				t.Fatalf("companion = %+v", got)
			}
		})
	}
	if got, err := companion(nil, "Track", ".txt", rf17Context(t.TempDir())); got != nil || err != nil {
		t.Fatalf("empty input = %+v, %v", got, err)
	}
}

func TestRF17CompanionUsesEntriesWithoutFilesystemReads(t *testing.T) {
	dir := t.TempDir()
	path := rf17Write(t, filepath.Join(dir, "Track.txt"))
	entries := rf17ReadDir(t, new(filetree.Resolver), dir)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	got, err := companion(entries, "Track", ".txt", rf17Context(dir))
	if err != nil || got == nil || got.Entry.Path != path {
		t.Fatalf("companion must use supplied metadata: %+v, %v", got, err)
	}
}

func TestRF17CompanionAliasesDoNotCreateAmbiguity(t *testing.T) {
	for _, linkKind := range []string{"symlink", "hardlink"} {
		t.Run(linkKind, func(t *testing.T) {
			dir := t.TempDir()
			target := rf17Write(t, filepath.Join(dir, "Track.txt"))
			alias := filepath.Join(t.TempDir(), "TRACK.TXT")
			var err error
			if linkKind == "symlink" {
				err = os.Symlink(target, alias)
			} else {
				err = os.Link(target, alias)
			}
			if err != nil {
				t.Fatal(err)
			}
			tree := new(filetree.Resolver)
			entries := append(rf17ReadDir(t, tree, dir), rf17ReadDir(t, tree, filepath.Dir(alias))...)
			if entries[0].IdentityPath != entries[1].IdentityPath {
				t.Fatal("fixture aliases did not receive common identity")
			}
			got, findErr := companion(entries, "Track", ".txt", rf17Context(dir))
			if findErr != nil || got == nil || got.Entry.IdentityPath != entries[0].IdentityPath {
				t.Fatalf("aliases = %+v, %v", got, findErr)
			}
		})
	}
}

func TestRF17CompanionFiltersBeforeUnique(t *testing.T) {
	dir := t.TempDir()
	target := rf17Write(t, filepath.Join(dir, "000-other.bin"))
	alias := filepath.Join(dir, "Track.txt")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	entries := rf17ReadDir(t, new(filetree.Resolver), dir)
	if len(entries) != 2 || entries[0].Path != target || entries[0].IdentityPath != entries[1].IdentityPath {
		t.Fatalf("fixture = %+v", entries)
	}
	got, err := companion(entries, "Track", ".txt", rf17Context(dir))
	if err != nil || got == nil || got.Entry.Path != alias || got.MIMEType != "text/plain" {
		t.Fatalf("matching alias hidden by unrelated name: %+v, %v", got, err)
	}
	if entries[0].Path != target || entries[1].Path != alias {
		t.Fatal("companion changed input entries")
	}
}

func TestRF17CompanionAmbiguityListsAllDistinctPaths(t *testing.T) {
	root := t.TempDir()
	tree := new(filetree.Resolver)
	var entries []filetree.Entry
	var want []string
	for _, name := range []string{"c", "a", "b"} {
		dir := filepath.Join(root, name)
		path := rf17Write(t, filepath.Join(dir, "Track.txt"))
		entries = append(entries, rf17ReadDir(t, tree, dir)...)
		want = append(want, path)
	}
	slices.SortFunc(want, filetree.ComparePaths)
	context := rf17Context(root)
	got, err := companion(entries, "Track", ".txt", context)
	if got != nil || err == nil {
		t.Fatalf("ambiguity = %+v, %v", got, err)
	}
	if err.Code != CodeAmbiguous || err.Context != context || err.Reason == "" {
		t.Fatalf("diagnostic = %+v", err)
	}
	var paths []string
	for _, related := range err.Related {
		paths = append(paths, related.Path)
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("candidates = %v; want %v", paths, want)
	}
	for _, value := range []string{"script", "Track.txt"} {
		found := false
		for _, detail := range err.Details {
			found = found || detail.Value == value
		}
		if !found {
			t.Errorf("details omit %q: %+v", value, err.Details)
		}
	}
}
