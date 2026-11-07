package resourcefind_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/resourcefind"
)

func rfiWrite(t *testing.T, root, relative string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("small resource\x00\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func rfiContext(path string) diagnostic.Context {
	return diagnostic.Context{Path: path, FileName: "l001.json", LessonID: "Тест-003", PageID: "тест-003-p001", Line: 35, Column: 14}
}

func rfiPrepare(t *testing.T, tree *filetree.Resolver, jsonPath string, dirs []string) (*resourcefind.Finder, []string) {
	t.Helper()
	f, paths, err := resourcefind.Prepare(tree, jsonPath, dirs, rfiContext(jsonPath))
	if err != nil {
		t.Fatalf("Prepare: %+v", err)
	}
	if f == nil {
		t.Fatal("Prepare returned nil Finder without error")
	}
	return f, paths
}

func rfiFind(t *testing.T, f *resourcefind.Finder, kind, raw, value string, ctx diagnostic.Context) resourcefind.Selection {
	t.Helper()
	got, err := f.Find(kind, raw, value, ctx)
	if err != nil {
		t.Fatalf("Find(%q, %q, %q): %+v", kind, raw, value, err)
	}
	return got
}

func rfiPaths(t *testing.T, got, want []string) {
	t.Helper()
	want = slices.Clone(want)
	slices.SortFunc(want, filetree.ComparePaths)
	if !slices.Equal(got, want) {
		t.Errorf("paths = %q; want %q", got, want)
	}
}

func rfiError(t *testing.T, err *diagnostic.Error, code string, ctx diagnostic.Context, values ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s", code)
	}
	if err.Code != code || err.Context != ctx {
		t.Errorf("diagnostic = %+v; want code=%s context=%+v", err, code, ctx)
	}
	for _, value := range values {
		found := false
		for _, d := range err.Details {
			if d.Value == value {
				found = true
			}
			if d.Label == "" {
				t.Error("empty detail label")
			}
		}
		if !found {
			t.Errorf("Details omit exact value %q: %+v", value, err.Details)
		}
	}
}

func TestResourcefindIntegrationCompanionOrderAndExternalAccounting(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "input/A.json")
	body := rfiWrite(t, root, "outside/Выпуск.003 MP4.MP4")
	base := "outside/Выпуск.003 MP4"
	script := rfiWrite(t, root, base+".TxT")
	extensions := []string{".VTT", ".SRT", ".TTML", ".DFXP", ".ASS", ".SSA"}
	mimes := []string{"text/vtt", "application/x-subrip", "application/ttml+xml", "application/ttml+xml", "text/plain", "text/plain"}
	want := []string{body, script}
	for _, ext := range extensions {
		want = append(want, rfiWrite(t, root, base+ext))
	}
	for _, name := range []string{base + ".json", base + " extra.txt", base + ".srt.bak", "outside/nested/Выпуск.003 MP4.txt", "outside/unselected.mp4"} {
		rfiWrite(t, root, name)
	}
	tree := new(filetree.Resolver)
	f, inventory := rfiPrepare(t, tree, jsonPath, nil)
	if len(inventory) != 0 {
		t.Fatalf("external files entered Prepare inventory: %q", inventory)
	}
	ctx := rfiContext(jsonPath)
	got := rfiFind(t, f, "video", `"different raw path"`, filepath.Join("..", "outside", filepath.Base(body)), ctx)
	if got.Body.Entry.Path != body || got.Body.MIMEType != "video/mp4" {
		t.Errorf("Body = %+v", got.Body)
	}
	if got.Script == nil || got.Script.Entry.Path != script || got.Script.MIMEType != "text/plain" {
		t.Fatalf("Script = %+v", got.Script)
	}
	if len(got.Subtitles) != 6 {
		t.Fatalf("Subtitles = %+v", got.Subtitles)
	}
	for i, sub := range got.Subtitles {
		if sub.Entry.Path != want[i+2] || sub.MIMEType != mimes[i] {
			t.Errorf("subtitle %d = %+v", i, sub)
		}
	}
	rfiPaths(t, got.AccountedPaths, want)
	got.AccountedPaths[0] = "changed"
	got.Subtitles[0].MIMEType = "changed"
	got.Script.MIMEType = "changed"
	again := rfiFind(t, f, "video", `"path"`, body, ctx)
	rfiPaths(t, again.AccountedPaths, want)
	if again.Script.MIMEType != "text/plain" || again.Subtitles[0].MIMEType != "text/vtt" {
		t.Error("returned selections share mutable state")
	}
	for _, path := range want {
		bytes, err := os.ReadFile(path)
		if err != nil || string(bytes) != "small resource\x00\r\n" {
			t.Errorf("resource changed: %q: %q, %v", path, bytes, err)
		}
	}
}

func TestResourcefindIntegrationIndependentOptionalCompanions(t *testing.T) {
	for _, tc := range []struct {
		name             string
		script, subtitle bool
	}{
		{"neither", false, false}, {"script only", true, false}, {"subtitle only", false, true}, {"both", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			jsonPath := rfiWrite(t, root, "A.json")
			body := rfiWrite(t, root, "external/episode.part.mp3")
			if tc.script {
				rfiWrite(t, root, "external/EPISODE.PART.txt")
			}
			if tc.subtitle {
				rfiWrite(t, root, "external/episode.part.srt")
			}
			rfiWrite(t, root, "external/sub/episode.part.vtt")
			rfiWrite(t, root, "external/episode.part-extra.txt")
			rfiWrite(t, root, "elsewhere/episode.part.ass")
			f, _ := rfiPrepare(t, new(filetree.Resolver), jsonPath, nil)
			got := rfiFind(t, f, "audio", `"path"`, body, rfiContext(jsonPath))
			subtitleCount := 0
			if tc.subtitle {
				subtitleCount = 1
			}
			if (got.Script != nil) != tc.script || len(got.Subtitles) != subtitleCount {
				t.Fatalf("optional selection = %+v", got)
			}
			count := 1
			if tc.script {
				count++
			}
			if tc.subtitle {
				count++
			}
			if len(got.AccountedPaths) != count {
				t.Errorf("unexpected external neighbors accounted: %q", got.AccountedPaths)
			}
		})
	}
}

func TestResourcefindIntegrationCompanionAmbiguity(t *testing.T) {
	for _, ext := range []string{"txt", "srt"} {
		t.Run(ext, func(t *testing.T) {
			root := t.TempDir()
			jsonPath := rfiWrite(t, root, "A.json")
			body := rfiWrite(t, root, "external/episode.mp4")
			first := rfiWrite(t, root, "external/Episode."+ext)
			second := rfiWrite(t, root, "external/episode."+ext)
			a, _ := os.Stat(first)
			b, _ := os.Stat(second)
			if os.SameFile(a, b) {
				t.Skip("filesystem does not support distinct case-sensitive names")
			}
			f, _ := rfiPrepare(t, new(filetree.Resolver), jsonPath, nil)
			ctx := rfiContext(jsonPath)
			_, err := f.Find("video", `"path"`, body, ctx)
			kind := "subtitles"
			if ext == "txt" {
				kind = "script"
			}
			rfiError(t, err, resourcefind.CodeAmbiguous, ctx, kind, "episode."+ext)
			var paths []string
			for _, c := range err.Related {
				paths = append(paths, c.Path)
			}
			rfiPaths(t, paths, []string{first, second})
		})
	}
}

func TestResourcefindIntegrationImageDoesNotReadCompanionDirectory(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "A.json")
	image := rfiWrite(t, root, "outside/cover.PNG")
	rfiWrite(t, root, "outside/cover.txt")
	rfiWrite(t, root, "outside/cover.vtt")
	broken := filepath.Join(root, "outside", "broken.txt")
	if err := os.Symlink(filepath.Join(root, "absent"), broken); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	f, _ := rfiPrepare(t, new(filetree.Resolver), jsonPath, nil)
	got := rfiFind(t, f, "image", `"path"`, image, rfiContext(jsonPath))
	if got.Script != nil || len(got.Subtitles) != 0 || got.Body.MIMEType != "image/png" {
		t.Fatalf("image selection = %+v", got)
	}
	rfiPaths(t, got.AccountedPaths, []string{image})
}

func TestResourcefindIntegrationCompanionDirectoryErrorPreservesFiletreeCause(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "A.json")
	body := rfiWrite(t, root, "external/episode.mp4")
	missing := filepath.Join(root, "missing")
	broken := filepath.Join(root, "external", "episode.srt")
	if err := os.Symlink(missing, broken); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	tree := new(filetree.Resolver)
	_, original := tree.ReadDir(filepath.Dir(body))
	if original == nil {
		t.Fatal("dependency accepted broken symlink")
	}
	f, _ := rfiPrepare(t, tree, jsonPath, nil)
	ctx := rfiContext(jsonPath)
	_, err := f.Find("video", `"path"`, body, ctx)
	if err == nil || err.Code != original.Code || err.Reason != original.Reason || err.Context != original.Context || err.Cause == nil {
		t.Fatalf("lost filetree diagnostic: got=%+v original=%+v", err, original)
	}
	found := false
	for _, related := range err.Related {
		if related == ctx {
			found = true
		}
	}
	if !found {
		t.Errorf("JSON context missing: %+v", err)
	}
	if original.Context.Path == jsonPath || len(original.Related) != 0 {
		t.Error("dependency error changed")
	}
	if !strings.Contains(err.Error(), err.Code) {
		t.Errorf("error lacks code: %v", err)
	}
}
