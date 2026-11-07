package resourcefind_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
	rf "github.com/verdoga/dsl-builder/resourcefind"
)

func TestFinderImageDoesNotEnumerateCompanionDirectory(t *testing.T) {
	root := t.TempDir()
	json := finderWrite(t, root, "A.json")
	image := finderWrite(t, root, "external/Кадр.PNG")
	finderWrite(t, root, "external/Кадр.txt")
	finderWrite(t, root, "external/Кадр.vtt")
	broken := filepath.Join(root, "external", "broken-link")
	if err := os.Symlink(filepath.Join(root, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	var tree filetree.Resolver
	f, _ := finderPrepare(t, &tree, json, nil)
	selected := finderSelect(t, f, "image", `"external/Кадр.PNG"`, filepath.Join("external", "Кадр.PNG"), json)
	if selected.Body.Entry.Path != image || selected.Body.MIMEType != "image/png" || selected.Script != nil || len(selected.Subtitles) != 0 {
		t.Fatalf("image must bypass companions entirely: %+v", selected)
	}
	finderPaths(t, selected.AccountedPaths, []string{image})
	// Этот реальный ReadDir подтверждает, что ошибочный сосед различает ветви.
	if _, err := tree.ReadDir(filepath.Dir(image)); err == nil {
		t.Fatal("fixture ReadDir должен завершаться ошибкой ссылки")
	}
}

func TestFinderAudioVideoHaveAllCompanionsAndOwnedResults(t *testing.T) {
	for _, media := range []struct{ kind, ext, mime string }{
		{"audio", ".MP3", "audio/mpeg"}, {"video", ".MP4", "video/mp4"},
	} {
		t.Run(media.kind, func(t *testing.T) {
			root := t.TempDir()
			json := finderWrite(t, root, "A.json")
			body := finderWrite(t, root, "external/Тема 001.part"+media.ext)
			script := finderWrite(t, root, "external/ТЕМА 001.PART.TXT")
			suffixes := []string{".vtt", ".srt", ".ttml", ".dfxp", ".ass", ".ssa"}
			mimes := []string{"text/vtt", "application/x-subrip", "application/ttml+xml", "application/ttml+xml", "text/plain", "text/plain"}
			expected := []string{body, script}
			subtitles := make([]string, len(suffixes))
			for i, ext := range suffixes {
				subtitles[i] = finderWrite(t, root, "external/Тема 001.part"+ext)
				expected = append(expected, subtitles[i])
			}
			finderWrite(t, root, "external/extra Тема 001.part.txt")
			finderWrite(t, root, "external/Тема 001.part.xyz")
			finderWrite(t, root, "external/deeper/Тема 001.part.txt")
			var tree filetree.Resolver
			f, inventory := finderPrepare(t, &tree, json, nil)
			if len(inventory) != 0 {
				t.Fatal("external files belong only to selected paths")
			}
			value := filepath.Join("external", filepath.Base(body))
			check := func(selected rf.Selection) {
				t.Helper()
				if selected.Body.Entry.Path != body || selected.Body.MIMEType != media.mime || selected.Script == nil || selected.Script.Entry.Path != script || selected.Script.MIMEType != "text/plain" {
					t.Fatalf("body/script = %+v", selected)
				}
				if len(selected.Subtitles) != 6 {
					t.Fatalf("subtitles = %+v", selected.Subtitles)
				}
				for i, sub := range selected.Subtitles {
					if sub.Entry.Path != subtitles[i] || sub.MIMEType != mimes[i] {
						t.Errorf("subtitle[%d] = %+v", i, sub)
					}
				}
				finderPaths(t, selected.AccountedPaths, expected)
			}
			first := finderSelect(t, f, media.kind, `"готовое значение пути"`, value, json)
			check(first)
			second := finderSelect(t, f, media.kind, `"готовое значение пути"`, value, json)
			first.Script.MIMEType = "испорчено"
			first.Subtitles[0].Entry.Path = "испорчено"
			first.AccountedPaths[0] = "испорчено"
			check(second)
			check(finderSelect(t, f, media.kind, `"готовое значение пути"`, value, json))
			got, err := f.Find(media.kind, "001", "001", finderContext(json))
			finderZero(t, got, err, rf.CodeNotFound)
		})
	}
}

func TestFinderMainErrorsReturnZeroSelectionAndAllowNextRequest(t *testing.T) {
	root := t.TempDir()
	json := finderWrite(t, root, "A.json")
	finderWrite(t, root, "Audio/one-match.mp3")
	finderWrite(t, root, "Audio/two-match.wav")
	finderWrite(t, root, "Audio/wrong.txt")
	image := finderWrite(t, root, "outside/image.png")
	if err := os.Mkdir(filepath.Join(root, "directory.mp3"), 0700); err != nil {
		t.Fatal(err)
	}
	var tree filetree.Resolver
	f, inventory := finderPrepare(t, &tree, json, []string{"Audio"})
	saved := slices.Clone(inventory)
	for _, tc := range []struct{ name, kind, raw, value, code string }{
		{"not found", "audio", "absent", "absent", rf.CodeNotFound},
		{"ambiguous", "audio", "match", "match", rf.CodeAmbiguous},
		{"wrong extension", "audio", `"Audio/wrong.txt"`, "Audio/wrong.txt", rf.CodeExtension},
		{"directory", "audio", `"directory.mp3"`, "directory.mp3", rf.CodeKind},
		{"missing explicit no fallback", "audio", `"one-match.mp3"`, "one-match.mp3", filetree.CodeAccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.Find(tc.kind, tc.raw, tc.value, finderContext(json))
			finderZero(t, got, err, tc.code)
			if !slices.Equal(inventory, saved) {
				t.Fatal("Find изменил возвращённую область Prepare")
			}
			selected := finderSelect(t, f, "image", `"outside/image.png"`, filepath.Join("outside", "image.png"), json)
			if selected.Body.Entry.Path != image {
				t.Fatal("ошибка повредила следующий выбор")
			}
		})
	}
	got, err := f.Find("image", "image", "image", finderContext(json))
	finderZero(t, got, err, rf.CodeNotFound)
}

func TestFinderCompanionFailureReturnsZeroSelection(t *testing.T) {
	root := t.TempDir()
	json := finderWrite(t, root, "A.json")
	finderWrite(t, root, "external/clip.mp4")
	finderWrite(t, root, "external/clip.txt")
	finderWrite(t, root, "external/clip.vtt")
	broken := filepath.Join(root, "external", "clip.srt")
	if err := os.Symlink(filepath.Join(root, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	var tree filetree.Resolver
	f, _ := finderPrepare(t, &tree, json, nil)
	got, err := f.Find("video", `"external/clip.mp4"`, filepath.Join("external", "clip.mp4"), finderContext(json))
	finderZero(t, got, err, filetree.CodeLink)
	if err.Cause == nil {
		t.Error("утрачена исходная причина ошибки доступа к companion")
	}
}

func TestFinderOptionalScriptAndSubtitlesAreIndependent(t *testing.T) {
	for _, companion := range []string{"", ".txt", ".ssa"} {
		t.Run(companion, func(t *testing.T) {
			root := t.TempDir()
			json := finderWrite(t, root, "A.json")
			body := finderWrite(t, root, "Audio/clip.mp3")
			expected := []string{body}
			if companion != "" {
				expected = append(expected, finderWrite(t, root, "Audio/clip"+companion))
			}
			var tree filetree.Resolver
			f, _ := finderPrepare(t, &tree, json, []string{"Audio"})
			selected := finderSelect(t, f, "audio", "clip", "clip", json)
			if (selected.Script != nil) != (companion == ".txt") {
				t.Fatalf("script = %+v", selected.Script)
			}
			wantSubs := 0
			if companion == ".ssa" {
				wantSubs = 1
			}
			if len(selected.Subtitles) != wantSubs {
				t.Fatalf("subtitles = %+v", selected.Subtitles)
			}
			finderPaths(t, selected.AccountedPaths, expected)
		})
	}
}
