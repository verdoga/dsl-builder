package moduleio_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/moduleio"
)

func VerifyCorruptTail(t *testing.T, entries []VerifyEntry, index int) []byte {
	t.Helper()
	entries[index].Method = zip.Store
	entries[index].Raw = append(entries[index].Raw, bytes.Repeat([]byte(" "), 256*1024)...)
	raw := VerifyZIP(t, entries)
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	entry := reader.File[index]
	offset, err := entry.DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	last := int(offset + int64(entry.CompressedSize64) - 1)
	if raw[last] != ' ' {
		t.Fatal("CRC fixture does not end in whitespace")
	}
	raw[last] = '\t' // JSON остаётся корректным; меняется только CRC полного хвоста.
	reader, err = zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	stream, err := reader.File[index].Open()
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, stream)
	closeErr := stream.Close()
	if !errors.Is(readErr, zip.ErrChecksum) || closeErr != nil {
		t.Fatalf("fixture must reach EOF with checksum error: %v / %v", readErr, closeErr)
	}
	return raw
}

func TestVerifyReadsCRCThroughFullJSONTail(t *testing.T) {
	for _, index := range []int{0, 1} {
		t.Run([]string{"lesson", "manifest"}[index], func(t *testing.T) {
			path := VerifyPath(t, VerifyCorruptTail(t, VerifyFixture(t, false), index))
			problems := VerifyReadResult(t, path, moduleio.CodeRead)
			problem := VerifyHasCode(t, problems, moduleio.CodeRead)
			if !errors.Is(problem, zip.ErrChecksum) {
				t.Fatal("Verify lost CRC Cause")
			}
			if problem.Context.FileName != []string{"l007.json", "manifest.json"}[index] {
				t.Fatalf("wrong CRC source: %#v", problem.Context)
			}
		})
	}
}

func TestVerifyDoesNotInstallExtractOrChangeSource(t *testing.T) {
	for _, scenario := range []string{"valid", "crc-error", "json-error", "unsafe-path", "archive-error"} {
		t.Run(scenario, func(t *testing.T) {
			entries := VerifyFixture(t, false)
			var raw []byte
			switch scenario {
			case "archive-error":
				raw = []byte("not a ZIP")
			case "crc-error":
				raw = VerifyCorruptTail(t, entries, 0)
			case "json-error":
				entries[0].Raw = []byte("{")
			case "unsafe-path":
				entries[0].Name = "../outside.json"
			}
			if raw == nil {
				raw = VerifyZIP(t, entries)
			}
			dir := t.TempDir()
			candidateDir := filepath.Join(dir, "временный каталог")
			if err := os.Mkdir(candidateDir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(candidateDir, "кандидат.tmp")
			target := filepath.Join(dir, "target.zip")
			outside := filepath.Join(dir, "outside.json")
			for _, sentinel := range []string{target, outside} {
				if err := os.WriteFile(sentinel, []byte("KEEP-ORIGINAL"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, raw, 0400); err != nil {
				t.Fatal(err)
			}
			stamp := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			VerifyOwnedDescriptors(t, path)
			for i := 0; i < 4; i++ {
				problems := moduleio.Verify(path)
				if (len(problems) == 0) != (scenario == "valid") {
					t.Fatalf("unexpected verification: %v", problems)
				}
				VerifyOwnedDescriptors(t, path)
			}
			after, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, raw) || !os.SameFile(before, after) ||
				before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("Verify altered or replaced candidate")
			}
			for _, sentinel := range []string{target, outside} {
				content, err := os.ReadFile(sentinel)
				if err != nil || string(content) != "KEEP-ORIGINAL" {
					t.Fatalf("Verify altered sentinel: %s", sentinel)
				}
			}
			files, err := os.ReadDir(candidateDir)
			if err != nil || len(files) != 1 || files[0].Name() != filepath.Base(path) {
				t.Fatalf("Verify extracted or created files: %v / %v", files, err)
			}
			parent, err := os.ReadDir(dir)
			if err != nil || len(parent) != 3 {
				t.Fatalf("Verify changed transaction directory: %v / %v", parent, err)
			}
			renamed := path + ".closed"
			if err := os.Rename(path, renamed); err != nil {
				t.Fatalf("candidate cannot be renamed after Verify: %v", err)
			}
			if err := os.Remove(renamed); err != nil {
				t.Fatalf("candidate cannot be removed after Verify: %v", err)
			}
		})
	}
}

func VerifyOwnedDescriptors(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		return
	}
	files, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", file.Name()))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if target == path || strings.TrimSuffix(target, " (deleted)") == path {
			t.Fatalf("Verify retained its descriptor %s for candidate", file.Name())
		}
	}
}
