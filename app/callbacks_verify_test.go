package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

func callbacksVerifyRaw(t *testing.T) ([]byte, []byte) {
	t.Helper()
	value := callbacksFixtureModule(t, false, "")
	return callbacksFixtureJSON(t, value.Manifest), callbacksFixtureJSON(t, value.Files[0].Lesson())
}

func callbacksVerifyUnchanged(t *testing.T, path string, raw []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("verification modified file: %v", err)
	}
	if runtime.GOOS == "linux" {
		files, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			target, err := os.Readlink(filepath.Join("/proc/self/fd", file.Name()))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if target == path {
				t.Fatal("verification retained an open archive descriptor")
			}
		}
	}
	// Переименование и удаление после возврата проверяют доступность пути для следующего владельца.
	renamed := path + ".renamed"
	if err := os.Rename(path, renamed); err != nil {
		t.Fatalf("descriptor still blocks rename: %v", err)
	}
	if err := os.Remove(renamed); err != nil {
		t.Fatalf("descriptor still blocks remove: %v", err)
	}
}

func TestCallbacksVerifyRealArchivesAndAllStructuredIssues(t *testing.T) {
	manifestRaw, lessonRaw := callbacksVerifyRaw(t)
	valid := callbacksFixtureZIP(t, manifestRaw, lessonRaw)
	corruptCRC := append([]byte(nil), valid...)
	reader, err := zip.NewReader(bytes.NewReader(corruptCRC), int64(len(corruptCRC)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range reader.File {
		offset, err := entry.DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		corruptCRC[int(offset)] ^= 1
	}
	value := callbacksFixtureModule(t, false, "")
	(*value.Manifest.Course.Lessons)[0].FileName = "l002.json"
	badLinks := callbacksFixtureZIP(t, callbacksFixtureJSON(t, value.Manifest), lessonRaw)
	for _, test := range []struct {
		name  string
		raw   []byte
		codes []string
		cause error
	}{
		{"valid", valid, nil, nil},
		{"invalid CRC in every entry", corruptCRC, []string{moduleio.CodeRead, moduleio.CodeRead}, zip.ErrChecksum},
		{"invalid JSON in every entry", callbacksFixtureZIP(t, []byte("{]"), []byte("{]")), []string{moduleio.CodeJSON, lesson.CodeJSON}, nil},
		{"invalid module links", badLinks, []string{module.CodeMissingFile, module.CodeUnlistedFile}, nil},
		{"invalid ZIP", []byte("это не ZIP"), []string{moduleio.CodeArchive}, zip.ErrFormat},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "временный модуль.zip")
			if err := os.WriteFile(path, test.raw, 0600); err != nil {
				t.Fatal(err)
			}
			fixed := time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)
			if err := os.Chtimes(path, fixed, fixed); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			got := verifyCallback(path)
			if len(test.codes) == 0 {
				if got != nil {
					t.Fatalf("valid archive returned %v", got)
				}
			} else {
				children := callbacksFixtureChildren(t, got)
				if len(children) != len(test.codes) {
					t.Fatalf("diagnostics = %d; want %d: %v", len(children), len(test.codes), got)
				}
				for index, code := range test.codes {
					problem := children[index]
					if problem.Code != code || problem.Context.Path != path || problem.Reason == "" {
						t.Fatalf("child %d lost owner code/context/reason: %+v", index, problem)
					}
					if index < 2 && test.name != "invalid ZIP" && problem.Context.FileName == "" {
						t.Fatalf("child %d lost archive entry name", index)
					}
					if !errors.Is(got, problem) {
						t.Fatalf("child %d cannot be traversed by errors.Is", index)
					}
				}
				var problem *diagnostic.Error
				if !errors.As(got, &problem) || problem != children[0] {
					t.Fatal("errors.As lost first structured issue")
				}
				if test.cause != nil && !errors.Is(got, test.cause) {
					t.Fatal("callback lost standard ZIP cause")
				}
				if test.name == "invalid JSON in every entry" {
					var syntax *json.SyntaxError
					if !errors.As(got, &syntax) {
						t.Fatal("callback lost JSON syntax cause")
					}
				}
			}
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("verification changed archive identity/size/time: %v", err)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
				t.Fatalf("verification extracted/created files: %v / %v", entries, err)
			}
			callbacksVerifyUnchanged(t, path, test.raw)
		})
	}
}

func TestCallbacksVerifyMissingPathPreservesPathError(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "нет модуля.zip")
	err := verifyCallback(path)
	children := callbacksFixtureChildren(t, err)
	if len(children) != 1 || children[0].Code != moduleio.CodeOpen || children[0].Context.Path != path {
		t.Fatalf("missing path issue: %+v", children)
	}
	var pathError *os.PathError
	if !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &pathError) || pathError.Path != path {
		t.Fatal("callback lost missing-file cause or its path")
	}
	entries, readErr := os.ReadDir(directory)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("missing-path verification changed directory: %v / %v", entries, readErr)
	}
}
