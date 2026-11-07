package resourcefind

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestSourceQASearchFileNeedsExactlyOneCandidate(t *testing.T) {
	base := t.TempDir()
	scope := filepath.Join(base, "область")
	if err := os.Mkdir(scope, 0755); err != nil {
		t.Fatal(err)
	}
	context := sourceQAContext(base)
	sourceQAWrite(t, filepath.Join(base, "outside NEEDLE.mp3"))
	for _, dirs := range [][]string{nil, {}, {scope}} {
		f := sourceQAFinder(base, dirs...)
		_, issue := f.searchFile("audio", "NEEDLE", context)
		sourceQASelectionError(t, issue, CodeNotFound, context, "audio", "NEEDLE", dirs, nil)
		if issue.Reason != "Ресурс не найден" {
			t.Errorf("причина отсутствия: %q", issue.Reason)
		}
	}
	audio := sourceQAWrite(t, filepath.Join(scope, "deep", "beforeNeedleAfter.MP3"))
	sourceQAWrite(t, filepath.Join(scope, "needle.MP4"))
	sourceQAWrite(t, filepath.Join(scope, "needle.PNG"))
	sourceQAWrite(t, filepath.Join(scope, "needle.txt"))
	sourceQAWrite(t, filepath.Join(scope, "needle.mp3.backup"))
	f := sourceQAFinder(base, scope)
	file, issue := f.searchFile("audio", "nEeDlE", context)
	sourceQAFile(t, file, issue, audio, "audio/mpeg")
	file, issue = f.searchFile("video", "needle", context)
	sourceQAFile(t, file, issue, filepath.Join(scope, "needle.MP4"), "video/mp4")
	file, issue = f.searchFile("image", "needle", context)
	sourceQAFile(t, file, issue, filepath.Join(scope, "needle.PNG"), "image/png")
}

func TestSourceQASearchFileReportsEveryCandidateDeterministically(t *testing.T) {
	base := t.TempDir()
	a, z := filepath.Join(base, "a"), filepath.Join(base, "z")
	first := sourceQAWrite(t, filepath.Join(a, "Alpha needle.mp3"))
	second := sourceQAWrite(t, filepath.Join(a, "beta needle.WAV"))
	third := sourceQAWrite(t, filepath.Join(z, "Zed needle.OPUS"))
	want := []string{first, second, third}
	context := sourceQAContext(base)
	var previous []diagnostic.Context
	for i, dirs := range [][]string{{z, a}, {a, z}, {z, a, a}, {z, a}} {
		_, issue := sourceQAFinder(base, dirs...).searchFile("audio", "needle", context)
		sourceQASelectionError(t, issue, CodeAmbiguous, context, "audio", "needle", dirs, want)
		if i != 0 && !slices.Equal(issue.Related, previous) {
			t.Errorf("порядок зависит от объявлений: %#v; ранее %#v", issue.Related, previous)
		}
		previous = slices.Clone(issue.Related)
	}
}

func TestSourceQASearchFileDeduplicatesOverlapsAndMatchingAliases(t *testing.T) {
	base := t.TempDir()
	scope := filepath.Join(base, "scope")
	nested := filepath.Join(scope, "nested")
	target := sourceQAWrite(t, filepath.Join(nested, "00-nonmatching.bin"))
	alias := filepath.Join(nested, "10-NEEDLE.MP3")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, filepath.Join(nested, "20-needle.mp3")); err != nil {
		t.Fatal(err)
	}
	context := sourceQAContext(base)
	for _, dirs := range [][]string{{scope}, {scope, nested, scope}, {nested, scope}} {
		t.Run(filepath.Base(dirs[0]), func(t *testing.T) {
			f := sourceQAFinder(base, dirs...)
			file, issue := f.searchFile("audio", "needle", context)
			sourceQAFile(t, file, issue, alias, "audio/mpeg")
			if file.Entry.IdentityPath != target {
				t.Errorf("потеряна идентичность цели: %#v", file.Entry)
			}
		})
	}
}

func TestSourceQASearchFilePreservesExactSubstring(t *testing.T) {
	base := t.TempDir()
	scope := filepath.Join(base, "scope")
	context := sourceQAContext(base)
	for _, tc := range []struct{ value, name string }{
		{" 12.01 ", "before 12.01 after.mp3"},
		{"003", "before003after.mp3"},
		{"буквы без номера", "начало БУКВЫ БЕЗ НОМЕРА конец.mp3"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			path := sourceQAWrite(t, filepath.Join(scope, tc.name))
			file, issue := sourceQAFinder(base, scope).searchFile("audio", tc.value, context)
			sourceQAFile(t, file, issue, path, "audio/mpeg")
		})
	}
	_, issue := sourceQAFinder(base, scope).searchFile("audio", "0003", context)
	sourceQASelectionError(t, issue, CodeNotFound, context, "audio", "0003", []string{scope}, nil)
}
