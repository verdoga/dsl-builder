package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func destinationWrite(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func destinationInfo(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func destinationAssertBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("bytes at %q = %q, %v; want %q", path, got, err, want)
	}
}

func TestDestinationInspectAbsentAndExistingDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Курс №1.ZIP")
	target, problem := inspectDestination(path)
	if problem != nil || target == nil {
		t.Fatalf("absent target: %#v, %v", target, problem)
	}
	want := Destination{Directory: dir, FileName: "Курс №1.ZIP", Path: path}
	if target.destination != want || target.info != nil {
		t.Fatalf("absent observation = %#v, %#v; want %#v and nil info", target.destination, target.info, want)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("inspection created absent file: %v", err)
	}
	raw := []byte("existing archive bytes are read only")
	destinationWrite(t, path, raw)
	at := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	before := destinationInfo(t, path)
	target, problem = inspectDestination(path)
	want.Exists = true
	if problem != nil || target == nil || target.destination != want || target.info == nil {
		t.Fatalf("existing observation = %#v, %v; want %#v and FileInfo", target, problem, want)
	}
	if !os.SameFile(before, target.info) || before.Size() != target.info.Size() || !before.ModTime().Equal(target.info.ModTime()) {
		t.Fatal("inspection did not preserve real file identity, size and mtime")
	}
	destinationAssertBytes(t, path, raw)
	after := destinationInfo(t, path)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("inspection changed existing file")
	}
}

func TestDestinationInspectRejectsUnsuitableParentsAndTargets(t *testing.T) {
	dir := t.TempDir()
	parentFile := filepath.Join(dir, "ordinary-parent")
	destinationWrite(t, parentFile, []byte("not a directory"))
	targetDir := filepath.Join(dir, "directory.zip")
	if err := os.Mkdir(targetDir, 0700); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		filepath.Join(dir, "missing-parent", "target.zip"),
		filepath.Join(parentFile, "target.zip"),
		targetDir,
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			target, problem := inspectDestination(path)
			if target != nil || problem == nil || problem.Code != CodeDestination || problem.Reason == "" {
				t.Fatalf("inspectDestination(%q) = %#v, %#v; want CodeDestination", path, target, problem)
			}
		})
	}
	if _, err := os.Lstat(filepath.Join(dir, "missing-parent")); !os.IsNotExist(err) {
		t.Fatalf("inspection created missing parent: %v", err)
	}
	destinationAssertBytes(t, parentFile, []byte("not a directory"))
}

func TestDestinationInspectUsesLstatForFileDirectoryAndDanglingLinks(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.zip")
	destinationWrite(t, realFile, []byte("protected bytes"))
	for _, tc := range []struct{ name, target string }{
		{"file-link.zip", realFile},
		{"directory-link.zip", dir},
		{"dangling-link.zip", filepath.Join(dir, "absent.zip")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if err := os.Symlink(tc.target, path); err != nil {
				t.Fatal(err)
			}
			target, problem := inspectDestination(path)
			if target != nil || problem == nil || problem.Code != CodeDestination {
				t.Fatalf("symlink accepted as destination: %#v, %#v", target, problem)
			}
			if destinationInfo(t, path).Mode()&os.ModeSymlink == 0 {
				t.Fatal("inspection changed the symlink")
			}
		})
	}
	destinationAssertBytes(t, realFile, []byte("protected bytes"))
}

func TestDestinationInspectPreservesDirectoryLinkAddress(t *testing.T) {
	base := t.TempDir()
	realDirectory := filepath.Join(base, "real")
	if err := os.Mkdir(realDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(realDirectory, alias); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(alias, "new.zip")
	target, problem := inspectDestination(path)
	if problem != nil || target == nil || target.destination.Directory != alias || target.destination.Path != path || target.destination.Exists {
		t.Fatalf("existing directory link did not retain shown address: %#v, %v", target, problem)
	}
	if _, err := os.Lstat(filepath.Join(realDirectory, "new.zip")); !os.IsNotExist(err) {
		t.Fatalf("inspection wrote through parent link: %v", err)
	}
}
