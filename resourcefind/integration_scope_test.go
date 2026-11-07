package resourcefind_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/resourcefind"
)

func TestResourcefindIntegrationSearchScopesAndExactValues(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "unit/Технический JSON/A.json")
	media := rfiWrite(t, root, "unit/Технический JSON/ Audio /deep/ПРЕФИКС 00.003 хвост.MP3")
	rfiWrite(t, root, "unit/Технический JSON/ Audio /deep/ПРЕФИКС 00.003 хвост.mp4")
	rfiWrite(t, root, "unit/Технический JSON/ Audio /only-directory-token/other.wav")
	rfiWrite(t, root, "unit/Технический JSON/ Audio /unused.bin")
	rfiWrite(t, root, "unit/Технический JSON/neighbors/unique-outside.mp3")
	dirs := []string{" Audio "}
	originalDirs := slices.Clone(dirs)
	f, inventory := rfiPrepare(t, new(filetree.Resolver), jsonPath, dirs)
	if !slices.Equal(dirs, originalDirs) {
		t.Fatalf("resourceDirs mutated: %q", dirs)
	}
	if len(inventory) != 4 {
		t.Fatalf("inventory omitted unsupported extension: %q", inventory)
	}
	dirs[0] = "neighbors"
	inventory[0] = "mutated return"
	ctx := rfiContext(jsonPath)
	for _, value := range []string{"рЕфИкС", "00.003", " 00.003 ", ".MP3", "хвост.MP"} {
		got := rfiFind(t, f, "audio", value, value, ctx)
		if got.Body.Entry.Path != media {
			t.Errorf("%q chose %q instead of %q", value, got.Body.Entry.Path, media)
		}
		rfiPaths(t, got.AccountedPaths, []string{media})
	}
	for _, value := range []string{"only-directory-token", "unique-outside", "0.3", "00.003  "} {
		_, err := f.Find("audio", value, value, ctx)
		rfiError(t, err, resourcefind.CodeNotFound, ctx, "audio", value, filepath.Join(filepath.Dir(jsonPath), " Audio "))
		if err.Reason != "Ресурс не найден" {
			t.Errorf("missing resource reason: %q", err.Reason)
		}
	}
}

func TestResourcefindIntegrationQuotedPathUsesJSONBaseAndHasNoFallback(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "unit/A.json")
	local := rfiWrite(t, root, "unit/точка.003 с пробелами.mp3")
	rfiWrite(t, root, "unit/resources/точка.003 с пробелами.mp3")
	rfiWrite(t, root, "unit/resources/missing.mp3")
	wrong := rfiWrite(t, root, "unit/wrong.mp4")
	unknown := rfiWrite(t, root, "unit/unknown.bin")
	directory := filepath.Join(root, "unit", "folder.mp3")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	f, _ := rfiPrepare(t, new(filetree.Resolver), jsonPath, []string{"resources"})
	ctx := rfiContext(jsonPath)
	got := rfiFind(t, f, "audio", `"raw only establishes quotes"`, filepath.Base(local), ctx)
	if got.Body.Entry.Path != local {
		t.Errorf("relative path base: %+v", got.Body)
	}
	got = rfiFind(t, f, "audio", `"absolute"`, local, ctx)
	if got.Body.Entry.Path != local {
		t.Errorf("absolute path: %+v", got.Body)
	}
	_, err := f.Find("audio", `"missing.mp3"`, "missing.mp3", ctx)
	if err == nil || err.Code != filetree.CodeAccess || err.Context.Path != filepath.Join(root, "unit", "missing.mp3") || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("explicit path fell back or lost cause: %+v", err)
	}
	for _, path := range []string{wrong, unknown} {
		_, err := f.Find("audio", `"path"`, path, ctx)
		rfiError(t, err, resourcefind.CodeExtension, ctx, "audio", path)
		if len(err.Related) != 1 || err.Related[0].Path != path {
			t.Errorf("extension path: %+v", err)
		}
	}
	_, err = f.Find("audio", `"folder.mp3"`, "folder.mp3", ctx)
	rfiError(t, err, resourcefind.CodeKind, ctx, "audio", "folder.mp3")
	if len(err.Related) != 1 || err.Related[0].Path != directory {
		t.Errorf("kind path: %+v", err)
	}
}

func TestResourcefindIntegrationNoAutomaticScope(t *testing.T) {
	for _, dirs := range [][]string{nil, {}} {
		root := t.TempDir()
		jsonPath := rfiWrite(t, root, "A.json")
		body := rfiWrite(t, root, "track.mp3")
		f, accounted := rfiPrepare(t, new(filetree.Resolver), jsonPath, dirs)
		if len(accounted) != 0 {
			t.Errorf("no resourceDirs inventory = %q", accounted)
		}
		ctx := rfiContext(jsonPath)
		_, err := f.Find("audio", "track", "track", ctx)
		rfiError(t, err, resourcefind.CodeNotFound, ctx, "audio", "track")
		got := rfiFind(t, f, "audio", `"track.mp3"`, "track.mp3", ctx)
		rfiPaths(t, got.AccountedPaths, []string{body})
	}
}

func TestResourcefindIntegrationAmbiguityHasAllCandidatesInPathOrder(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "A.json")
	a := rfiWrite(t, root, "scope/a/003-first.MP3")
	b := rfiWrite(t, root, "scope/B/003-second.wav")
	c := rfiWrite(t, root, "scope/z/003-third.flac")
	rfiWrite(t, root, "scope/003.mp4")
	f, _ := rfiPrepare(t, new(filetree.Resolver), jsonPath, []string{"scope/z", "scope/B", "scope/a", "scope"})
	ctx := rfiContext(jsonPath)
	_, err := f.Find("audio", "003", "003", ctx)
	rfiError(t, err, resourcefind.CodeAmbiguous, ctx, "audio", "003")
	var candidates []string
	for _, place := range err.Related {
		candidates = append(candidates, place.Path)
	}
	rfiPaths(t, candidates, []string{a, b, c})
}

func TestResourcefindIntegrationPrepareValidatesEveryDirectoryAtomically(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "A.json")
	rfiWrite(t, root, "resources/track.mp3")
	ordinaryFile := rfiWrite(t, root, "not-a-directory")
	for _, tc := range []struct {
		name       string
		dirs       []string
		code, path string
	}{
		{"missing after valid", []string{"resources", "missing"}, filetree.CodeAccess, filepath.Join(root, "missing")},
		{"overlap does not hide missing", []string{"resources", "resources/missing"}, filetree.CodeAccess, filepath.Join(root, "resources", "missing")},
		{"ordinary file", []string{"resources", ordinaryFile}, filetree.CodeKind, ordinaryFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := rfiContext(jsonPath)
			f, paths, err := resourcefind.Prepare(new(filetree.Resolver), jsonPath, tc.dirs, ctx)
			if f != nil || paths != nil || err == nil {
				t.Fatalf("partial Prepare result: f=%v paths=%q err=%+v", f, paths, err)
			}
			if err.Code != tc.code || err.Context.Path != tc.path {
				t.Errorf("directory diagnostic: %+v", err)
			}
			found := false
			for _, place := range err.Related {
				if place == ctx {
					found = true
				}
			}
			if !found {
				t.Errorf("missing source context: %+v", err)
			}
		})
	}
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0755); err != nil {
		t.Fatal(err)
	}
	_, inventory := rfiPrepare(t, new(filetree.Resolver), jsonPath, []string{empty, empty})
	if len(inventory) != 0 {
		t.Fatalf("empty inventory: %q", inventory)
	}
}
