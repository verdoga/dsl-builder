package filetree_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

func TestWalk14AtomicErrorsAndRequiredRootChecks(t *testing.T) {
	for _, kind := range []string{"missing", "file", "broken", "self"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "bad")
			code := filetree.CodeAccess
			switch kind {
			case "file":
				walk14File(t, root, "bad")
				code = filetree.CodeKind
			case "broken":
				walk14Link(t, filepath.Join(root, "absent"), path, false)
				code = filetree.CodeLink
			case "self":
				walk14Link(t, path, path, false)
				code = filetree.CodeLink
			}
			var r filetree.Resolver
			for _, read := range []func(string) ([]filetree.Entry, *diagnostic.Error){r.ReadDir, func(p string) ([]filetree.Entry, *diagnostic.Error) { return r.Walk(p, nil) }} {
				entries, err := read(path)
				walk14Error(t, entries, err, code, path, kind != "file")
			}
		})
	}
	for _, kind := range []string{"broken", "self"} {
		t.Run("child-"+kind, func(t *testing.T) {
			root := t.TempDir()
			walk14File(t, root, "a-valid.json")
			bad := filepath.Join(root, "z-bad")
			target := filepath.Join(root, "absent")
			if kind == "self" {
				target = bad
			}
			walk14Link(t, target, bad, false)
			var r filetree.Resolver
			entries, err := r.ReadDir(root)
			walk14Error(t, entries, err, filetree.CodeLink, bad, true)
			entries, err = r.Walk(root, nil)
			walk14Error(t, entries, err, filetree.CodeLink, bad, true)
		})
	}
}

func TestWalk14UnsupportedObjectIsAtomicError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Log("Unix device fixture is host-specific; Windows checked by cross-compilation")
		return
	}
	root := t.TempDir()
	walk14File(t, root, "a-valid")
	path := filepath.Join(root, "z-device")
	info, err := os.Stat("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() || info.Mode().IsRegular() {
		t.Fatal("fixture must be an unusual object")
	}
	walk14Link(t, "/dev/null", path, false)
	var r filetree.Resolver
	entries, diagnosticErr := r.Walk(root, nil)
	walk14Error(t, entries, diagnosticErr, filetree.CodeKind, path, false)
}

func TestWalk14AccessMatchesActualHostPermissions(t *testing.T) {
	root := t.TempDir()
	walk14File(t, root, "a-valid")
	blocked := filepath.Join(root, "z-blocked")
	walk14File(t, blocked, "nested/file")
	if err := os.Chmod(blocked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0700) })
	_, probe := os.ReadDir(blocked)
	var r filetree.Resolver
	entries, err := r.Walk(root, nil)
	if errors.Is(probe, fs.ErrPermission) {
		walk14Error(t, entries, err, filetree.CodeAccess, blocked, true)
		entries, err = r.ReadDir(blocked)
		walk14Error(t, entries, err, filetree.CodeAccess, blocked, true)
		return
	}
	if probe != nil {
		t.Fatalf("unexpected permission probe: %v", probe)
	}
	t.Log("chmod(0000) directory remains readable on this host; denial branch unavailable")
	if err != nil {
		t.Fatal(err)
	}
	walk14Want(t, root, entries, "a-valid", "z-blocked/nested/file")
}
