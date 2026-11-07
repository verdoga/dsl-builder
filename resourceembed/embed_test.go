package resourceembed

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestTask9EmbedSelectedFiles(t *testing.T) {
	for _, tc := range []struct {
		name, media, body string
		extensions        []string
		mimes             []string
	}{
		{"audio_body", "audio", ".MP3", nil, nil},
		{"audio_script", "audio", ".WAV", []string{".TxT"}, []string{"text/plain"}},
		{"video_subtitles_without_script", "video", ".webm", []string{".srt", ".ssa"}, []string{"application/x-subrip", "text/plain"}},
		{"all_companions", "video", ".MP4", []string{".TxT", ".vtt", ".srt", ".ttml", ".dfxp", ".ass", ".ssa"}, []string{"text/plain", "text/vtt", "application/x-subrip", "application/ttml+xml", "application/ttml+xml", "text/plain", "text/plain"}},
		{"image_ignores_companions", "image", ".PnG", []string{".txt", ".vtt", ".srt", ".ttml", ".dfxp", ".ass", ".ssa"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			name := "Трек 01" + tc.body
			payload := []byte{0, 0xff, 0xfe, '\r', '\n', 1, 2}
			sources := map[string][]byte{name: payload}
			for i, ext := range tc.extensions {
				sources["Трек 01"+ext] = append([]byte{0xef, 0xbb, 0xbf, byte(i)}, payload...)
			}
			for name, content := range sources {
				task9Write(t, dir, name, content)
			}
			selection, context := task9Find(t, new(filetree.Resolver), dir, tc.media, name)
			before := task9Clone(selection)
			var usage resourceusage.Registry
			usage.Register(selection.AccountedPaths)
			got, err := Embed(selection, context)
			if err != nil || len(got) != 1+len(tc.mimes) {
				t.Fatalf("Embed: len=%d, error=%v", len(got), err)
			}
			mime := map[string]string{".MP3": "audio/mpeg", ".WAV": "audio/wav", ".webm": "video/webm", ".MP4": "video/mp4", ".PnG": "image/png"}[tc.body]
			task9Data(t, got[0], lesson.RoleBody, name, mime, payload)
			for i, mime := range tc.mimes {
				role := lesson.RoleSubtitles
				if tc.extensions[i] == ".TxT" {
					role = lesson.RoleScript
				}
				task9Data(t, got[i+1], role, "Трек 01"+tc.extensions[i], mime, append([]byte{0xef, 0xbb, 0xbf, byte(i)}, payload...))
			}
			again, err := Embed(selection, context)
			if err != nil || len(again) != len(got) {
				t.Fatalf("repeat Embed: %v", err)
			}
			for i := range got {
				for j := i + 1; j < len(got); j++ {
					if got[i].Role == got[j].Role {
						t.Fatal("elements share a role pointer")
					}
				}
				if got[i].Role == again[i].Role {
					t.Fatal("calls share a role pointer")
				}
			}
			*got[0].Role = lesson.RoleScript
			got[0].Value = lesson.EmbeddedFile{}
			task9Data(t, again[0], lesson.RoleBody, name, mime, payload)
			task9Unchanged(t, selection, before)
			for name, expected := range sources {
				content, readErr := os.ReadFile(filepath.Join(dir, name))
				if readErr != nil || !bytes.Equal(content, expected) {
					t.Fatalf("source changed: %s, %v", name, readErr)
				}
			}
			if snapshot := usage.Snapshot(); snapshot.Used != 0 || snapshot.Total != len(selection.AccountedPaths) {
				t.Fatalf("read result affected usage: %+v", snapshot)
			}
			node := lesson.Node{Type: lesson.NodeMedia, Data: again}
			usage.MarkUsed(selection.AccountedPaths)
			if len(node.Data) == 0 || usage.Snapshot().Used != len(selection.AccountedPaths) {
				t.Fatal("caller failed to account for included files")
			}
		})
	}
}

func TestTask9EmbedKeepsEqualFilesAndIdentities(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "equal_bytes", true: "hardlinks"}[linked], func(t *testing.T) {
			dir := t.TempDir()
			payload := []byte{0xfb, 0xff, 0, '\r', '\n'}
			task9Write(t, dir, "Общий.mp4", payload)
			for _, ext := range []string{".txt", ".vtt", ".srt", ".ttml", ".dfxp", ".ass", ".ssa"} {
				if linked {
					if err := os.Link(filepath.Join(dir, "Общий.mp4"), filepath.Join(dir, "Общий"+ext)); err != nil {
						t.Skipf("hardlinks unavailable: %v", err)
					}
				} else {
					task9Write(t, dir, "Общий"+ext, payload)
				}
			}
			selection, context := task9Find(t, new(filetree.Resolver), dir, "video", "Общий.mp4")
			if selection.Script == nil || len(selection.Subtitles) != 6 || linked && len(selection.AccountedPaths) != 1 || !linked && len(selection.AccountedPaths) != 8 {
				t.Fatalf("real Find precondition: %+v", selection)
			}
			got, err := Embed(selection, context)
			if err != nil || len(got) != 8 {
				t.Fatalf("files were combined: len=%d, error=%v", len(got), err)
			}
			for i, file := range append([]resourcefind.File{selection.Body, *selection.Script}, selection.Subtitles...) {
				role := lesson.RoleSubtitles
				if i == 0 {
					role = lesson.RoleBody
				} else if i == 1 {
					role = lesson.RoleScript
				}
				task9Data(t, got[i], role, filepath.Base(file.Entry.Path), file.MIMEType, payload)
			}
		})
	}
}

func TestTask9EmbedReadFailureIsAtomic(t *testing.T) {
	for _, target := range []string{"Клип.mp4", "Клип.txt", "Клип.ssa"} {
		for _, directory := range []bool{false, true} {
			t.Run(target+map[bool]string{false: "_removed", true: "_directory"}[directory], func(t *testing.T) {
				dir := t.TempDir()
				for _, name := range []string{"Клип.mp4", "Клип.txt", "Клип.vtt", "Клип.ssa"} {
					task9Write(t, dir, name, []byte("secret bytes\x00\xff\r\n"))
				}
				path := filepath.Join(dir, target)
				backup := filepath.Join(dir, "другой адрес.bin")
				tree := new(filetree.Resolver)
				linked := os.Link(path, backup) == nil
				if linked {
					if _, err := tree.Inspect(backup); err != nil {
						t.Fatal(err)
					}
				} else {
					t.Log("hardlink fallback fixture unavailable; selected-path failure still tested")
				}
				selection, context := task9Find(t, tree, dir, "video", "Клип.mp4")
				before := task9Clone(selection)
				file, role := selection.Body, lesson.RoleBody
				if target == "Клип.txt" {
					file, role = *selection.Script, lesson.RoleScript
				} else if target == "Клип.ssa" {
					file, role = selection.Subtitles[len(selection.Subtitles)-1], lesson.RoleSubtitles
				}
				if linked && file.Entry.IdentityPath != backup {
					t.Fatal("fallback fixture does not have distinct identity address")
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if directory {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
				got, problem := Embed(selection, context)
				if got != nil || problem == nil {
					t.Fatalf("partial result/fallback: data=%v, error=%v", got, problem)
				}
				if problem.Code != CodeRead || problem.Reason != "Не удалось прочитать выбранный ресурс" || problem.Context.Path != path || !slices.Contains(problem.Related, context) {
					t.Fatalf("diagnostic lost resource or JSON context: %+v", problem)
				}
				var cause *os.PathError
				if !errors.As(problem, &cause) || cause.Path != path || !directory && !errors.Is(problem, os.ErrNotExist) {
					t.Fatalf("original read cause missing: %v", problem.Cause)
				}
				for _, detail := range problem.Details {
					if bytes.Contains([]byte(detail.Value), []byte("secret bytes")) || bytes.Contains([]byte(detail.Value), []byte(base64.StdEncoding.EncodeToString([]byte("secret bytes\x00\xff\r\n")))) {
						t.Fatal("diagnostic exposes resource content")
					}
				}
				task9Unchanged(t, selection, before)
				data, err := embedFile(file, role, context)
				if err == nil || data.Type != "" || data.Role != nil || data.Value != nil || data.Answers != nil || data.Hint != "" {
					t.Fatalf("embedFile must discard failed data: %+v, %v", data, err)
				}
			})
		}
	}
}

func TestTask9EmbedFileUsesGivenRole(t *testing.T) {
	dir := t.TempDir()
	task9Write(t, dir, "Пустой.mp3", nil)
	selection, context := task9Find(t, new(filetree.Resolver), dir, "audio", "Пустой.mp3")
	for _, role := range []lesson.DataRole{lesson.RoleBody, lesson.RoleScript, lesson.RoleSubtitles} {
		got, err := embedFile(selection.Body, role, context)
		if err != nil {
			t.Fatal(err)
		}
		task9Data(t, got, role, "Пустой.mp3", "audio/mpeg", nil)
	}
}

func task9Write(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
		t.Fatal(err)
	}
}

func task9Find(t *testing.T, tree *filetree.Resolver, dir, media, name string) (resourcefind.Selection, diagnostic.Context) {
	t.Helper()
	context := diagnostic.Context{Path: filepath.Join(dir, "исходный урок.json"), FileName: "l001.json", LessonID: "AB12", PageID: "ab12-p001", Line: 35, Column: 14}
	finder, _, err := resourcefind.Prepare(tree, context.Path, nil, context)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := finder.Find(media, `"`+name+`"`, name, context)
	if err != nil {
		t.Fatal(err)
	}
	return selection, context
}

func task9Data(t *testing.T, got lesson.Data, role lesson.DataRole, name, mime string, content []byte) {
	t.Helper()
	file, ok := got.Value.(lesson.EmbeddedFile)
	if got.Type != lesson.DataMedia || got.Role == nil || *got.Role != role || !ok || got.Answers == nil || len(got.Answers) != 0 || got.Hint != "" {
		t.Fatalf("invalid media data: %+v", got)
	}
	decoded, err := base64.StdEncoding.DecodeString(file.Content)
	if file.FileName != name || file.MIMEType != mime || file.Content != base64.StdEncoding.EncodeToString(content) || err != nil || !bytes.Equal(decoded, content) {
		t.Fatalf("changed name/MIME/bytes: %+v, decode=%v", file, err)
	}
}

func task9Clone(selection resourcefind.Selection) resourcefind.Selection {
	selection.Subtitles = slices.Clone(selection.Subtitles)
	selection.AccountedPaths = slices.Clone(selection.AccountedPaths)
	if selection.Script != nil {
		script := *selection.Script
		selection.Script = &script
	}
	return selection
}

func task9Unchanged(t *testing.T, got, before resourcefind.Selection) {
	t.Helper()
	if got.Body != before.Body || !slices.Equal(got.Subtitles, before.Subtitles) || !slices.Equal(got.AccountedPaths, before.AccountedPaths) || (got.Script == nil) != (before.Script == nil) || got.Script != nil && *got.Script != *before.Script {
		t.Fatal("Embed changed Selection or its path lists")
	}
}
