package resourcefind

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
)

func TestSourceQAMatchesMediaUsesCompleteAddressName(t *testing.T) {
	base := t.TempDir()
	for _, tc := range []struct {
		name, mediaType, value string
		want                   bool
	}{
		{"beforeNEEDLEafter.MP3", "audio", "needle", true},
		{"beforeNEEDLEafter.MP3", "audio", "before", true},
		{"beforeNEEDLEafter.MP3", "audio", "after.mp3", true},
		{"beforeNEEDLEafter.MP3", "audio", ".mp3", true},
		{"beforeNEEDLEafter.MP3", "video", "needle", false},
		{"beforeNEEDLEafter.mp3.backup", "audio", "needle", false},
		{"ЁЖ café 日本語 003.01 .Mp3", "audio", "ёж CAFÉ 日本語", true},
		{"ЁЖ café 日本語 003.01 .Mp3", "audio", "003.01 ", true},
		{"ЁЖ café 日本語 003.01 .Mp3", "audio", "00301", false},
		{"before003after.mp3", "audio", "0003", false},
		{"before003after.mp3", "audio", "3", true},
		{"before3after.mp3", "audio", "003", false},
		{"before 003 after.mp3", "audio", " 003 ", true},
		{"before003after.mp3", "audio", " 003 ", false},
		{"before  003 after.mp3", "audio", "before 003", false},
		{"track.mp3", "audio", "", true},
		{"x.AVIF", "image", "avif", true},
		{"x.WEBM", "video", "webm", true},
		{"x.txt", "audio", "x", false},
		{"track.MP3", "audio", "directory-only-needle", false},
		{"слово.mp3", "audio", "о.mp", true},
	} {
		t.Run(tc.name+"/"+tc.value+"/"+tc.mediaType, func(t *testing.T) {
			path := sourceQAWrite(t, filepath.Join(base, "directory-only-needle", tc.name))
			entry, issue := (&filetree.Resolver{}).Inspect(path)
			if issue != nil {
				t.Fatal(issue)
			}
			if got := matchesMedia(entry, tc.mediaType, tc.value); got != tc.want {
				t.Errorf("matchesMedia(%q, %q, %q) = %v, ожидалось %v", entry.Path, tc.mediaType, tc.value, got, tc.want)
			}
		})
	}
}

func TestSourceQAMatchesMediaUsesAliasExtensionAndName(t *testing.T) {
	base := t.TempDir()
	target := sourceQAWrite(t, filepath.Join(base, "target-only-needle.MP4"))
	alias := filepath.Join(base, "Адрес ЁЖ 003.MP3")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	tree := &filetree.Resolver{}
	entry, issue := tree.Inspect(alias)
	if issue != nil {
		t.Fatal(issue)
	}
	if entry.IdentityPath != target || entry.Info.Name() != filepath.Base(target) {
		t.Fatalf("сценарий не различает имя и цель: %#v", entry)
	}
	for _, tc := range []struct {
		kind, value string
		want        bool
	}{
		{"audio", "ёж 003.mp3", true},
		{"audio", "target-only-needle", false},
		{"video", "ЁЖ", false},
		{"video", "target-only-needle", false},
	} {
		if got := matchesMedia(entry, tc.kind, tc.value); got != tc.want {
			t.Errorf("matchesMedia по ссылке: kind=%q value=%q got=%v want=%v", tc.kind, tc.value, got, tc.want)
		}
	}
	otherTarget := sourceQAWrite(t, filepath.Join(base, "actual-audio.MP3"))
	wrongAlias := filepath.Join(base, "wrong-name.MP4")
	if err := os.Symlink(otherTarget, wrongAlias); err != nil {
		t.Fatal(err)
	}
	entry, issue = tree.Inspect(wrongAlias)
	if issue != nil {
		t.Fatal(issue)
	}
	if matchesMedia(entry, "audio", "wrong-name") {
		t.Fatal("расширение цели подменило расширение адреса")
	}
	if !matchesMedia(entry, "video", "wrong-name") {
		t.Fatal("расширение адреса не учтено")
	}
}
