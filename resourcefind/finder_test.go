package resourcefind_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	rf "github.com/verdoga/dsl-builder/resourcefind"
)

func finderWrite(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0, 255, 1}, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func finderContext(path string) diagnostic.Context {
	return diagnostic.Context{Path: path, LessonID: "УРОК01", PageID: "УРОК01-p001", Line: 35, Column: 14}
}

func finderPrepare(t *testing.T, tree *filetree.Resolver, path string, dirs []string) (*rf.Finder, []string) {
	t.Helper()
	f, paths, err := rf.Prepare(tree, path, dirs, finderContext(path))
	if err != nil || f == nil {
		t.Fatalf("Prepare: finder=%v paths=%v err=%v", f, paths, err)
	}
	return f, paths
}

func finderSelect(t *testing.T, f *rf.Finder, kind, raw, value, json string) rf.Selection {
	t.Helper()
	got, err := f.Find(kind, raw, value, finderContext(json))
	if err != nil {
		t.Fatalf("Find(%q, %q, %q): %v", kind, raw, value, err)
	}
	return got
}

func finderPaths(t *testing.T, got, want []string) {
	t.Helper()
	expected := slices.Clone(want)
	slices.SortFunc(expected, filetree.ComparePaths)
	if !slices.Equal(got, expected) {
		t.Fatalf("paths = %q, want %q", got, expected)
	}
}

func finderZero(t *testing.T, got rf.Selection, err *diagnostic.Error, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("ожидалась блокирующая ошибка")
	}
	if code != "" && err.Code != code {
		t.Errorf("code = %q, want %q", err.Code, code)
	}
	if got.Body.Entry.Path != "" || got.Body.Entry.IdentityPath != "" || got.Body.Entry.Info != nil || got.Body.MIMEType != "" || got.Script != nil || len(got.Subtitles) != 0 || len(got.AccountedPaths) != 0 {
		t.Errorf("ошибка оставила частичный Selection: %+v", got)
	}
}

func TestFinderSharedResolverAcrossLessonsKeepsDiscoveryBase(t *testing.T) {
	root := t.TempDir()
	original := finderWrite(t, root, "original/source.bin")
	shared := finderWrite(t, root, "original/common.bin")
	var tree filetree.Resolver
	fixed, err := tree.Inspect(shared)
	if err != nil {
		t.Fatal(err)
	}
	jsonFixed, err := tree.Inspect(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, lesson := range []string{"SB", "WB"} {
		jsonPath := filepath.Join(root, lesson, "A.JSON")
		media := finderWrite(t, root, lesson+"/Audio/temporary")
		if err := os.Remove(media); err != nil {
			t.Fatal(err)
		}
		media = filepath.Join(filepath.Dir(media), "Звук 001.MP3")
		if err := os.Link(shared, media); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(original, jsonPath); err != nil {
			t.Fatal(err)
		}
		entry, err := tree.JSONFile(jsonPath)
		if err != nil {
			t.Fatal(err)
		}
		if entry.IdentityPath != jsonFixed.IdentityPath {
			t.Fatal("fixture потерял общий объект JSON")
		}
		f, paths := finderPrepare(t, &tree, entry.Path, []string{"Audio", "Audio/..//Audio"})
		finderPaths(t, paths, []string{fixed.IdentityPath})
		for _, raw := range []string{"001", `"Audio/Звук 001.MP3"`} {
			value := "001"
			if raw[0] == '"' {
				value = filepath.Join("Audio", "Звук 001.MP3")
			}
			selected := finderSelect(t, f, "audio", raw, value, jsonPath)
			if selected.Body.Entry.Path != media || selected.Body.Entry.IdentityPath != fixed.IdentityPath || selected.Body.MIMEType != "audio/mpeg" {
				t.Fatalf("discovery base/shared identity: %+v", selected.Body)
			}
			finderPaths(t, selected.AccountedPaths, []string{fixed.IdentityPath})
		}
	}
}
