package resourcefind

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
)

func TestRF17CompanionsSearchAddressDirectoryNotSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	address, targetDir := filepath.Join(root, "address"), filepath.Join(root, "target")
	target := rf17Write(t, filepath.Join(targetDir, "Different.mp4"))
	rf17Write(t, filepath.Join(targetDir, "Different.txt"))
	rf17Write(t, filepath.Join(targetDir, "Track.txt"))
	rf17Write(t, filepath.Join(targetDir, "Track.vtt"))
	scriptPath := rf17Write(t, filepath.Join(address, "Track.txt"))
	subtitlePath := rf17Write(t, filepath.Join(address, "Track.srt"))
	alias := filepath.Join(address, "Track.mp4")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	tree := new(filetree.Resolver)
	body := File{Entry: rf17Inspect(t, tree, alias), MIMEType: "video/mp4"}
	if body.Entry.Path == body.Entry.IdentityPath || body.Entry.Info.Name() == filepath.Base(alias) {
		t.Fatalf("fixture must distinguish address, identity and Info.Name: %+v", body)
	}
	script, subtitles, err := (&Finder{tree: tree, baseDir: targetDir, directories: []string{targetDir}}).companions(body, rf17Context(root))
	if err != nil || script == nil || script.Entry.Path != scriptPath {
		t.Fatalf("script = %+v; err = %v", script, err)
	}
	if len(subtitles) != 1 || subtitles[0].Entry.Path != subtitlePath {
		t.Fatalf("subtitles = %+v", subtitles)
	}
}

func TestRF17CompanionsDoNotRecurseOrSelectNearbyNames(t *testing.T) {
	dir := t.TempDir()
	tree := new(filetree.Resolver)
	body := File{Entry: rf17Inspect(t, tree, rf17Write(t, filepath.Join(dir, "Track.part.mp4")))}
	for _, name := range []string{
		"nested/Track.part.txt", "nested/Track.part.vtt", "Track.txt", "Track.srt",
		"Track.part-extra.txt", "extra-Track.part.vtt", "Track.part.sbv", "Track.part.lrc",
	} {
		rf17Write(t, filepath.Join(dir, filepath.FromSlash(name)))
	}
	for _, name := range []string{"Track.part.txt", "Track.part.srt"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	script, subtitles, err := (&Finder{tree: tree}).companions(body, rf17Context(dir))
	if err != nil || script != nil || len(subtitles) != 0 {
		t.Fatalf("non-companions selected: %+v, %+v, %v", script, subtitles, err)
	}
}

func TestRF17CompanionsKeepDirectorySymlinkAddress(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	rf17Write(t, filepath.Join(realDir, "Track.mp3"))
	rf17Write(t, filepath.Join(realDir, "Track.txt"))
	address := filepath.Join(root, "alias")
	if err := os.Symlink(realDir, address); err != nil {
		t.Fatal(err)
	}
	tree := new(filetree.Resolver)
	body := File{Entry: rf17Inspect(t, tree, filepath.Join(address, "Track.mp3"))}
	script, _, err := (&Finder{tree: tree}).companions(body, rf17Context(root))
	if err != nil || script == nil || script.Entry.Path != filepath.Join(address, "Track.txt") {
		t.Fatalf("directory address lost: %+v, %v", script, err)
	}
}

func TestRF17CompanionsReadDirErrorsRetainDiagnosticAndJSONContext(t *testing.T) {
	for _, scenario := range []string{"missing directory", "directory became file", "broken neighboring link"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "media")
			tree := new(filetree.Resolver)
			body := File{Entry: rf17Inspect(t, tree, rf17Write(t, filepath.Join(dir, "Track.mp3")))}
			if scenario == "broken neighboring link" {
				rf17Write(t, filepath.Join(dir, "Track.txt"))
				if err := os.Symlink(filepath.Join(root, "absent.txt"), filepath.Join(dir, "unrelated.bin")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				if scenario == "directory became file" {
					rf17Write(t, dir)
				}
			}
			_, original := tree.ReadDir(dir)
			if original == nil {
				t.Fatal("fixture did not produce ReadDir error")
			}
			context := rf17Context(root)
			_, _, got := (&Finder{tree: tree}).companions(body, context)
			if got == nil {
				t.Fatal("ReadDir error was ignored")
			}
			if got.Code != original.Code || got.Reason != original.Reason || got.Context != original.Context {
				t.Fatalf("changed filetree diagnostic: got %+v; original %+v", got, original)
			}
			if !slices.Contains(got.Related, context) {
				t.Fatalf("JSON context missing: %+v", got)
			}
			if !slices.Equal(got.Details, original.Details) {
				t.Fatalf("filetree details changed: %+v", got.Details)
			}
			if (got.Cause == nil) != (original.Cause == nil) {
				t.Fatalf("cause presence changed: %v / %v", got.Cause, original.Cause)
			}
			if original.Cause != nil {
				var gotPath, originalPath *os.PathError
				if !errors.As(got, &gotPath) || !errors.As(original, &originalPath) ||
					gotPath.Op != originalPath.Op || gotPath.Path != originalPath.Path || !errors.Is(gotPath.Err, originalPath.Err) {
					t.Fatalf("cause chain changed: %v / %v", got, original)
				}
				if !errors.Is(got, fs.ErrNotExist) {
					t.Fatalf("missing-object cause lost: %v", got)
				}
			}
		})
	}
}

func TestRF17CompanionsCaseAmbiguityBlocksSelection(t *testing.T) {
	for _, extension := range []string{"txt", "srt"} {
		t.Run(extension, func(t *testing.T) {
			dir := t.TempDir()
			tree := new(filetree.Resolver)
			body := File{Entry: rf17Inspect(t, tree, rf17Write(t, filepath.Join(dir, "Track.mp4")))}
			paths := []string{filepath.Join(dir, "Track."+extension), filepath.Join(dir, "TRACK."+extension)}
			for _, path := range paths {
				rf17Write(t, path)
			}
			first, second := rf17Inspect(t, tree, paths[0]), rf17Inspect(t, tree, paths[1])
			if os.SameFile(first.Info, second.Info) {
				t.Skip("файловая система не поддерживает разные файлы, отличающиеся только регистром")
			}
			context := rf17Context(dir)
			_, _, err := (&Finder{tree: tree}).companions(body, context)
			if err == nil || err.Code != CodeAmbiguous || err.Context != context {
				t.Fatalf("case ambiguity = %+v", err)
			}
			slices.SortFunc(paths, filetree.ComparePaths)
			var got []string
			for _, related := range err.Related {
				got = append(got, related.Path)
			}
			if !slices.Equal(got, paths) {
				t.Fatalf("candidate paths = %v; want %v", got, paths)
			}
		})
	}
}
