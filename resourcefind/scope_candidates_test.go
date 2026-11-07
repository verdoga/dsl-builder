package resourcefind

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
)

func TestScopeCandidatesFixedExtensionsAndExactSubstring(t *testing.T) {
	root := t.TempDir()
	groups := []struct{ kind, extensions string }{
		{"audio", ".mp3 .wav .ogg .opus .m4a .aac .flac"},
		{"video", ".mp4 .m4v .webm .ogv .mov .avi .mkv .wmv"},
		{"image", ".jpg .jpeg .png .gif .webp .bmp .svg .tif .tiff .avif"},
	}
	for _, group := range groups {
		for _, extension := range strings.Fields(group.extensions) {
			scopeWrite(t, root, "deep/Prefix 00.Ёж middle SUFFIX"+strings.ToUpper(extension))
		}
	}
	for _, extension := range []string{".txt", ".vtt", ".srt", ".ttml", ".dfxp", ".ass", ".ssa", ".exe", ".mp30", ""} {
		scopeWrite(t, root, "Prefix 00.Ёж middle SUFFIX"+extension)
	}
	scopeWrite(t, root, "00.Ёж directory/unrelated.mp3")
	f := &Finder{tree: &filetree.Resolver{}, directories: []string{root, filepath.Join(root, "deep"), root}}
	for _, group := range groups {
		t.Run(group.kind, func(t *testing.T) {
			want := make([]string, 0)
			for _, extension := range strings.Fields(group.extensions) {
				want = append(want, filepath.Join(root, "deep", "Prefix 00.Ёж middle SUFFIX"+strings.ToUpper(extension)))
			}
			slices.SortFunc(want, filetree.ComparePaths)
			for _, value := range []string{"00.ёЖ middle", "PREFIX", "suffix."} {
				got, err := f.candidates(group.kind, value, scopeContext(root))
				if err != nil || !slices.Equal(scopePaths(got), want) {
					t.Fatalf("candidates(%q, %q) = %q, %v; want %q", group.kind, value, scopePaths(got), err, want)
				}
			}
			for _, value := range []string{"000.Ёж", "00.Ёж  middle", "00.Ёж directory"} {
				got, err := f.candidates(group.kind, value, scopeContext(root))
				if err != nil || len(got) != 0 {
					t.Fatalf("лишние кандидаты для %q: %q, %v", value, scopePaths(got), err)
				}
			}
		})
	}
}

func TestScopeCandidatesFilterBeforeUniqueAndPreserveAliasName(t *testing.T) {
	for _, excluded := range []string{"a-no-match.mp3", "a-wanted.txt", "a-wanted.mp4"} {
		t.Run(excluded, func(t *testing.T) {
			root := t.TempDir()
			original := scopeWrite(t, root, excluded)
			alias := filepath.Join(root, "z-WANTED.MP3")
			if err := os.Symlink(original, alias); err != nil {
				t.Fatal(err)
			}
			tree := &filetree.Resolver{}
			anchored, err := tree.Inspect(original)
			if err != nil {
				t.Fatal(err)
			}
			f := &Finder{tree: tree, directories: []string{root, root}}
			got, issue := f.candidates("audio", "wanted", scopeContext(root))
			if issue != nil || len(got) != 1 {
				t.Fatalf("candidates = %#v, %v", got, issue)
			}
			if got[0].Path != alias || got[0].IdentityPath != anchored.IdentityPath || got[0].Info.Name() != filepath.Base(original) {
				t.Fatalf("адрес и имя цели смешаны: %#v", got[0])
			}
		})
	}
}

func TestScopeCandidatesCommonResolverHardLinksAndOverlap(t *testing.T) {
	root := t.TempDir()
	outside := scopeWrite(t, t.TempDir(), "original.bin")
	first := filepath.Join(root, "A wanted.mp3")
	second := filepath.Join(root, "nested", "B wanted.mp3")
	if err := os.MkdirAll(filepath.Dir(second), 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{first, second} {
		if err := os.Link(outside, path); err != nil {
			t.Fatal(err)
		}
	}
	tree := &filetree.Resolver{}
	anchored, err := tree.Inspect(outside)
	if err != nil {
		t.Fatal(err)
	}
	f := &Finder{tree: tree, directories: []string{root, filepath.Dir(second), root}}
	if _, issue := f.inventory(scopeContext(root)); issue != nil {
		t.Fatal(issue)
	}
	for range 2 {
		got, issue := f.candidates("audio", "wanted", scopeContext(root))
		if issue != nil || len(got) != 1 || got[0].Path != first || got[0].IdentityPath != anchored.IdentityPath {
			t.Fatalf("один физический файл: %#v, %v", got, issue)
		}
	}
}

func TestScopeCandidatesDeterministicGlobalPathOrder(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "A")
	z := filepath.Join(root, "z")
	want := []string{scopeWrite(t, z, "b wanted.mp3"), scopeWrite(t, a, "z wanted.mp3"), scopeWrite(t, a, "B wanted.mp3"), scopeWrite(t, a, "a wanted.mp3")}
	slices.SortFunc(want, filetree.ComparePaths)
	f := &Finder{tree: &filetree.Resolver{}, directories: []string{z, a, z}}
	for range 3 {
		got, issue := f.candidates("audio", "wanted", scopeContext(root))
		if issue != nil || !slices.Equal(scopePaths(got), want) {
			t.Fatalf("order = %q, %v; want %q", scopePaths(got), issue, want)
		}
	}
}

func TestScopeCandidatesNoDirectoriesDoNotInspectNeighbors(t *testing.T) {
	root := t.TempDir()
	scopeWrite(t, root, "wanted.mp3")
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}
	for _, dirs := range [][]string{nil, {}} {
		f := &Finder{tree: &filetree.Resolver{}, baseDir: root, directories: dirs}
		got, err := f.candidates("audio", "wanted", scopeContext(root))
		if err != nil || len(got) != 0 {
			t.Fatalf("обойдены соседи без области: %#v, %v", got, err)
		}
	}
}

func TestScopeCandidatesWalkFailureIsAtomicEvenForNonmatchingNames(t *testing.T) {
	root := t.TempDir()
	scopeWrite(t, root, "wanted.mp3")
	broken := t.TempDir()
	if err := os.Symlink(filepath.Join(broken, "missing"), filepath.Join(broken, "unrelated.txt")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{filepath.Join(root, "missing"), filepath.Join(root, "wanted.mp3"), broken} {
		tree := &filetree.Resolver{}
		_, expected := tree.Walk(bad, nil)
		if expected == nil {
			t.Fatal("fixture не вызывает ошибку Walk")
		}
		f := &Finder{tree: tree, directories: []string{root, root, bad}}
		context := scopeContext(root)
		got, issue := f.candidates("audio", "wanted", context)
		if got != nil {
			t.Fatalf("частичные кандидаты: %#v", got)
		}
		scopeError(t, issue, expected, context)
	}
}
