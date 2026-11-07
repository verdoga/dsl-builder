package app_test

import (
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
)

func TestIntegrationOwnerArchiveFixtureHasValidRawAndNumberingGap(t *testing.T) {
	path, raw := integrationArchive(t, t.TempDir())
	value, issues := moduleio.Read(path)
	integrationOK(t, issues)
	if len(value.Files) != 2 || len(raw) != 2 {
		t.Fatal("incorrect seed archive")
	}
	for _, name := range []string{"l001.json", "l003.json"} {
		file := integrationFile(t, value, name)
		if len(file.OriginalBytes()) == 0 || file.Lesson().Document.Version != "1" {
			t.Fatalf("invalid seed file: %s", name)
		}
	}
}

func TestIntegrationOwnerMediaFixtureConvertsWithoutLoadedInputMutation(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "source", "media.json")
	integrationWrite(t, filepath.Join(base, "media", "nested", "Clip.mp3"), []byte{0, 1, 255})
	integrationWriteInput(t, path, integrationMediaInput("MEDIA1"))
	raw := integrationRead(t, path)
	input, issues := parsed.Load(path)
	integrationOK(t, issues)
	var tree filetree.Resolver
	value, usage, issues := convert.Build(input, &tree, integrationAt(), diagnostic.Context{Path: path})
	integrationOK(t, issues)
	if value.Document.Version != "1" || usage.Snapshot().Total != 1 || usage.Snapshot().Used != 1 {
		t.Fatal("invalid synthetic media fixture")
	}
	integrationSameBytes(t, path, raw)
}
