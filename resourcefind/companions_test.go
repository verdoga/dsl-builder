package resourcefind

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

func rf17Write(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("малый реальный ресурс\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func rf17Inspect(t *testing.T, tree *filetree.Resolver, path string) filetree.Entry {
	t.Helper()
	entry, err := tree.Inspect(path)
	if err != nil {
		t.Fatalf("Inspect(%q): %v", path, err)
	}
	return entry
}

func rf17ReadDir(t *testing.T, tree *filetree.Resolver, dir string) []filetree.Entry {
	t.Helper()
	entries, err := tree.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	return entries
}

func rf17Context(dir string) diagnostic.Context {
	return diagnostic.Context{
		Path: filepath.Join(dir, "исходный урок.json"), FileName: "l017.json",
		LessonID: "R017", PageID: "R017-p002", Line: 35, Column: 14,
	}
}

func TestRF17SubtitleExtensionsOwnsFixedOrder(t *testing.T) {
	want := []string{".vtt", ".srt", ".ttml", ".dfxp", ".ass", ".ssa"}
	first, second := subtitleExtensions(), subtitleExtensions()
	if !slices.Equal(first, want) || !slices.Equal(second, want) {
		t.Fatalf("extensions = %v, %v; want %v", first, second, want)
	}
	first[0] = ".broken"
	if !slices.Equal(second, want) || !slices.Equal(subtitleExtensions(), want) {
		t.Fatal("изменение одного результата затронуло другой вызов")
	}
}

func TestRF17CompanionsIndependentOptionalFiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		txt  bool
		sub  bool
	}{
		{"neither", false, false}, {"txt only", true, false},
		{"subtitles only", false, true}, {"both", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tree := new(filetree.Resolver)
			body := File{Entry: rf17Inspect(t, tree, rf17Write(t, filepath.Join(dir, "Урок 01.part.MP4"))), MIMEType: "video/mp4"}
			if tc.txt {
				rf17Write(t, filepath.Join(dir, "урок 01.PART.TxT"))
			}
			if tc.sub {
				rf17Write(t, filepath.Join(dir, "УРОК 01.part.SrT"))
			}
			finder := &Finder{tree: tree, baseDir: t.TempDir(), directories: []string{t.TempDir()}}
			script, subtitles, err := finder.companions(body, rf17Context(dir))
			if err != nil {
				t.Fatal(err)
			}
			if (script != nil) != tc.txt {
				t.Fatalf("script = %v; txt exists = %v", script, tc.txt)
			}
			if tc.txt && (script.Entry.Path != filepath.Join(dir, "урок 01.PART.TxT") || script.MIMEType != "text/plain") {
				t.Fatalf("script = %+v", script)
			}
			wantCount := 0
			if tc.sub {
				wantCount = 1
			}
			if len(subtitles) != wantCount {
				t.Fatalf("subtitles = %+v", subtitles)
			}
			if tc.sub && (subtitles[0].Entry.Path != filepath.Join(dir, "УРОК 01.part.SrT") || subtitles[0].MIMEType != "application/x-subrip") {
				t.Fatalf("subtitle = %+v", subtitles[0])
			}
		})
	}
}

func TestRF17CompanionsAllFormatsInStrictOrder(t *testing.T) {
	dir := t.TempDir()
	tree := new(filetree.Resolver)
	body := File{Entry: rf17Inspect(t, tree, rf17Write(t, filepath.Join(dir, "Track.mp3"))), MIMEType: "audio/mpeg"}
	extensions := []string{".vtt", ".srt", ".ttml", ".dfxp", ".ass", ".ssa"}
	mimes := []string{"text/vtt", "application/x-subrip", "application/ttml+xml", "application/ttml+xml", "text/plain", "text/plain"}
	for i := len(extensions) - 1; i >= 0; i-- {
		rf17Write(t, filepath.Join(dir, "Track"+extensions[i]))
	}
	rf17Write(t, filepath.Join(dir, "Track.txt"))
	for _, suffix := range []string{".json", ".lrc", ".sbv", ".mp3.bak", ".TXT.bak", ".srt.bak"} {
		rf17Write(t, filepath.Join(dir, "Track"+suffix))
	}
	script, subtitles, err := (&Finder{tree: tree}).companions(body, rf17Context(dir))
	if err != nil || script == nil || script.Entry.Path != filepath.Join(dir, "Track.txt") {
		t.Fatalf("script = %+v; err = %v", script, err)
	}
	if len(subtitles) != len(extensions) {
		t.Fatalf("subtitles = %+v", subtitles)
	}
	for i, extension := range extensions {
		if subtitles[i].Entry.Path != filepath.Join(dir, "Track"+extension) || subtitles[i].MIMEType != mimes[i] {
			t.Errorf("subtitle[%d] = %+v; want extension %s, MIME %s", i, subtitles[i], extension, mimes[i])
		}
	}
}
