package resourcefind_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func TestResourcefindIntegrationFilterBeforeIdentityDeduplication(t *testing.T) {
	for _, linkType := range []string{"hardlink", "symlink"} {
		t.Run(linkType, func(t *testing.T) {
			root := t.TempDir()
			jsonPath := rfiWrite(t, root, "A.json")
			original := rfiWrite(t, root, "resources/a-unmatched.bin")
			alias := filepath.Join(root, "resources", "Нужный.003.MP3")
			var err error
			if linkType == "hardlink" {
				err = os.Link(original, alias)
			} else {
				err = os.Symlink(original, alias)
			}
			if err != nil {
				t.Skipf("%s unavailable: %v", linkType, err)
			}
			tree := new(filetree.Resolver)
			fixed, inspectErr := tree.Inspect(original)
			if inspectErr != nil {
				t.Fatal(inspectErr)
			}
			f, inventory := rfiPrepare(t, tree, jsonPath, []string{"resources", "resources"})
			rfiPaths(t, inventory, []string{fixed.IdentityPath})
			got := rfiFind(t, f, "audio", "нужный.003", "нужный.003", rfiContext(jsonPath))
			if got.Body.Entry.Path != alias || got.Body.Entry.IdentityPath != fixed.IdentityPath || got.Body.MIMEType != "audio/mpeg" {
				t.Fatalf("alias selection = %+v", got)
			}
			rfiPaths(t, got.AccountedPaths, []string{fixed.IdentityPath})
		})
	}
}

func TestResourcefindIntegrationCompanionsUseDiscoveryAddress(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "input/A.json")
	target := rfiWrite(t, root, "targets/original.bin")
	rfiWrite(t, root, "targets/original.txt")
	rfiWrite(t, root, "targets/visible.txt")
	alias := filepath.Join(root, "visible", "visible.MP4")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	script := rfiWrite(t, root, "visible/visible.TXT")
	subtitle := rfiWrite(t, root, "visible/visible.VTT")
	tree := new(filetree.Resolver)
	f, _ := rfiPrepare(t, tree, jsonPath, nil)
	got := rfiFind(t, f, "video", `"alias"`, alias, rfiContext(jsonPath))
	if got.Body.Entry.Path != alias || got.Body.Entry.IdentityPath != target {
		t.Errorf("discovery/identity = %+v", got.Body.Entry)
	}
	if got.Script == nil || got.Script.Entry.Path != script || len(got.Subtitles) != 1 || got.Subtitles[0].Entry.Path != subtitle {
		t.Fatalf("companions followed target: %+v", got)
	}
	rfiPaths(t, got.AccountedPaths, []string{target, script, subtitle})
}

func TestResourcefindIntegrationJSONDiscoveryAddressDefinesRelativeBase(t *testing.T) {
	root := t.TempDir()
	actualJSON := rfiWrite(t, root, "original/A.json")
	rfiWrite(t, root, "original/resources/wrong.mp3")
	aliasJSON := filepath.Join(root, "discovered", "A.json")
	if err := os.MkdirAll(filepath.Dir(aliasJSON), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actualJSON, aliasJSON); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	body := rfiWrite(t, root, "discovered/resources/right.mp3")
	local := rfiWrite(t, root, "discovered/local.mp3")
	tree := new(filetree.Resolver)
	entry, err := tree.JSONFile(aliasJSON)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Path != aliasJSON || entry.IdentityPath != actualJSON {
		t.Fatalf("dependency JSON entry = %+v", entry)
	}
	f, inventory := rfiPrepare(t, tree, entry.Path, []string{"resources"})
	rfiPaths(t, inventory, []string{body})
	got := rfiFind(t, f, "audio", "right", "right", rfiContext(entry.Path))
	if got.Body.Entry.Path != body {
		t.Errorf("substring used identity base: %+v", got)
	}
	got = rfiFind(t, f, "audio", `"local.mp3"`, "local.mp3", rfiContext(entry.Path))
	if got.Body.Entry.Path != local {
		t.Errorf("explicit path used identity base: %+v", got)
	}
}

func TestResourcefindIntegrationOverlapAndSharedResolverUsage(t *testing.T) {
	root := t.TempDir()
	firstJSON := rfiWrite(t, root, "first/A.json")
	secondJSON := rfiWrite(t, root, "second/A.json")
	original := rfiWrite(t, root, "first/resources/deep/track.mp3")
	unused := rfiWrite(t, root, "first/resources/unused.unsupported")
	alias := filepath.Join(root, "second", "resources", "same.mp3")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, alias); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	tree := new(filetree.Resolver)
	first, firstInventory := rfiPrepare(t, tree, firstJSON, []string{"resources", "resources/deep", "resources"})
	second, secondInventory := rfiPrepare(t, tree, secondJSON, []string{"resources"})
	rfiPaths(t, firstInventory, []string{original, unused})
	rfiPaths(t, secondInventory, []string{original})
	_, repeatedInventory := rfiPrepare(t, tree, firstJSON, []string{"resources", "resources/deep"})
	rfiPaths(t, repeatedInventory, firstInventory)
	var registry resourceusage.Registry
	registry.Register(firstInventory)
	registry.Register(secondInventory)
	one := rfiFind(t, first, "audio", "track", "track", rfiContext(firstJSON))
	two := rfiFind(t, second, "audio", "same", "same", rfiContext(secondJSON))
	if two.Body.Entry.Path != alias || one.Body.Entry.IdentityPath != two.Body.Entry.IdentityPath {
		t.Fatalf("shared resolver identities: first=%+v second=%+v", one, two)
	}
	registry.Register(one.AccountedPaths)
	registry.Register(two.AccountedPaths)
	before := registry.Snapshot()
	if before.Total != 2 || before.Used != 0 || before.Unused != 2 {
		t.Fatalf("selection implied use or double count: %+v", before)
	}
	// Имитируется только решение владельца учёта после успешного встраивания.
	// Само встраивание не проверяется: resourceembed здесь не вызывается.
	registry.MarkUsed(one.AccountedPaths)
	registry.MarkUsed(two.AccountedPaths)
	after := registry.Snapshot()
	if after.Total != 2 || after.Used != 1 || after.Unused != 1 {
		t.Fatalf("unique operation accounting: %+v", after)
	}
	rfiPaths(t, after.UsedPaths, []string{original})
	rfiPaths(t, after.UnusedPaths, []string{unused})
	again := rfiFind(t, first, "audio", "track", "track", rfiContext(firstJSON))
	rfiPaths(t, again.AccountedPaths, one.AccountedPaths)
}

func TestResourcefindIntegrationCompanionAliasesPreserveRoles(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "A.json")
	body := rfiWrite(t, root, "external/track.mp3")
	original := rfiWrite(t, root, "external/a-unmatched.bin")
	script := filepath.Join(root, "external", "track.txt")
	subtitle := filepath.Join(root, "external", "track.vtt")
	for _, path := range []string{script, subtitle} {
		if err := os.Link(original, path); err != nil {
			t.Skipf("hardlink unavailable: %v", err)
		}
	}
	tree := new(filetree.Resolver)
	_, err := tree.Inspect(original)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := rfiPrepare(t, tree, jsonPath, nil)
	got := rfiFind(t, f, "audio", `"path"`, body, rfiContext(jsonPath))
	if got.Script == nil || got.Script.Entry.Path != script || len(got.Subtitles) != 1 || got.Subtitles[0].Entry.Path != subtitle {
		t.Fatalf("identity removed roles or matched wrong alias: %+v", got)
	}
	if got.Script.MIMEType != "text/plain" || got.Subtitles[0].MIMEType != "text/vtt" {
		t.Errorf("MIME must follow discovery name: %+v", got)
	}
	rfiPaths(t, got.AccountedPaths, []string{body, original})
}

func TestResourcefindIntegrationMultipleMatchingAliasesAreOneCandidate(t *testing.T) {
	root := t.TempDir()
	jsonPath := rfiWrite(t, root, "A.json")
	original := rfiWrite(t, root, "resources/a.bin")
	for _, name := range []string{"clip-one.mp3", "clip-two.wav"} {
		if err := os.Link(original, filepath.Join(root, "resources", name)); err != nil {
			t.Skipf("hardlink unavailable: %v", err)
		}
	}
	tree := new(filetree.Resolver)
	f, inventory := rfiPrepare(t, tree, jsonPath, []string{"resources"})
	got := rfiFind(t, f, "audio", "clip", "clip", rfiContext(jsonPath))
	if got.Body.Entry.Path != filepath.Join(root, "resources", "clip-one.mp3") {
		t.Errorf("unexpected first matching address: %+v", got.Body)
	}
	rfiPaths(t, inventory, []string{original})
	rfiPaths(t, got.AccountedPaths, []string{original})
}
