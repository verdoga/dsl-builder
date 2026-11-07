package moduleio

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func EntriesZIP(t *testing.T, names ...string) *zip.Reader {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		if _, err := writer.Create(name); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func EntriesProblem(t *testing.T, problems []*diagnostic.Error, code, name string) *diagnostic.Error {
	t.Helper()
	for _, problem := range problems {
		if problem != nil && problem.Code == code && problem.Context.FileName == name {
			if problem.Reason == "" {
				t.Fatal("empty diagnostic reason")
			}
			return problem
		}
	}
	t.Fatalf("missing diagnostic %s for %q: %v", code, name, problems)
	return nil
}

func TestEntriesRootNamesAndModes(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Модули\Unit 1.zip`, FileName: "old-name"}
	for _, name := range []string{"manifest.json", "l001.json", "l999.json", "l1000.json", "l999999999999999999999999999999999999999999.json"} {
		t.Run(name, func(t *testing.T) {
			entry := EntriesZIP(t, name).File[0]
			before := entry.FileHeader
			if problems := validateEntry(entry, context); len(problems) != 0 {
				t.Fatalf("valid root name rejected: %v", problems)
			}
			if !reflect.DeepEqual(before, entry.FileHeader) {
				t.Fatal("validation changed ZIP header")
			}
		})
	}
	for _, name := range []string{"", "Manifest.json", "MANIFEST.JSON", "L001.json", "l001.JSON", "l000.json", "l01.json", "l0001.json", "l１01.json", "l001.json ", " l001.json", "notes.txt", "audio.mp3", "./l001.json", "../l001.json", "dir/l001.json", `dir\l001.json`, "/l001.json", `C:\l001.json`, `C:l001.json`, `\\server\l001.json`, "l001.json\n"} {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			entry := EntriesZIP(t, name).File[0]
			problem := EntriesProblem(t, validateEntry(entry, context), CodeEntryName, name)
			if problem.Context.Path != context.Path {
				t.Fatalf("archive path lost: %#v", problem.Context)
			}
			var cause *diagnostic.Error
			if !errors.As(problem.Cause, &cause) || cause.Code != identity.CodeInvalidFileName {
				t.Fatalf("identity cause lost: %v", problem.Cause)
			}
		})
	}
	for _, mode := range []fs.FileMode{fs.ModeDir | 0755, fs.ModeSymlink | 0777, fs.ModeNamedPipe | 0600, fs.ModeSocket | 0600, fs.ModeDevice | 0600, fs.ModeCharDevice | fs.ModeDevice | 0600} {
		t.Run(mode.String(), func(t *testing.T) {
			entry := EntriesZIP(t, "l001.json").File[0]
			entry.SetMode(mode)
			problem := EntriesProblem(t, validateEntry(entry, context), CodeEntryType, entry.Name)
			if problem.Context.Path != context.Path {
				t.Fatal("type diagnostic lost archive path")
			}
		})
	}
	entry := EntriesZIP(t, "l001.json/").File[0]
	EntriesProblem(t, validateEntry(entry, context), CodeEntryType, entry.Name)
}

func TestEntriesCompositionAndErrorOrder(t *testing.T) {
	context := diagnostic.Context{Path: "archive.zip"}
	reader := EntriesZIP(t, "l1000.json", "manifest.json", "l003.json")
	before := append([]*zip.File(nil), reader.File...)
	if problems := validateEntries(reader.File, context); len(problems) != 0 {
		t.Fatalf("unordered noncontiguous filenames rejected: %v", problems)
	}
	if !reflect.DeepEqual(before, reader.File) {
		t.Fatal("ZIP order changed")
	}
	if problems := validateEntries(EntriesZIP(t, "manifest.json").File, context); len(problems) != 0 {
		t.Fatalf("physical checker must not impose lesson/model requirements: %v", problems)
	}
	for _, entries := range [][]*zip.File{nil, EntriesZIP(t, "l001.json").File} {
		EntriesProblem(t, validateEntries(entries, context), CodeManifest, "manifest.json")
	}
	problems := validateEntries(EntriesZIP(t, "first.txt", "Second.JSON", "l000.json").File, context)
	wantNames := []string{"first.txt", "Second.JSON", "l000.json", "manifest.json"}
	if len(problems) != len(wantNames) {
		t.Fatalf("independent entry errors and missing manifest: %v", problems)
	}
	for index, problem := range problems {
		wantCode := CodeEntryName
		if index == len(wantNames)-1 {
			wantCode = CodeManifest
		}
		if problem.Code != wantCode || problem.Context.Path != context.Path || problem.Context.FileName != wantNames[index] {
			t.Fatalf("error %d out of ZIP order/context: %#v", index, problem)
		}
	}
}

func TestEntriesDuplicateGroupsRetainEveryOccurrence(t *testing.T) {
	context := diagnostic.Context{Path: "same.zip"}
	for _, name := range []string{"l001.json", "manifest.json", "extra.txt"} {
		t.Run(name, func(t *testing.T) {
			names := []string{"l002.json", name, "l003.json", name, "l004.json", name}
			if name != "manifest.json" {
				names = append(names, "manifest.json")
			}
			problems := validateEntries(EntriesZIP(t, names...).File, context)
			EntriesProblem(t, problems, CodeDuplicateEntry, name)
			for _, problem := range problems {
				if problem.Code != CodeDuplicateEntry {
					continue
				}
				if problem.Context.Path != context.Path || len(problem.Related) != 3 {
					t.Fatalf("duplicate group lost occurrences: %#v", problem)
				}
				for _, related := range problem.Related {
					if related.Path != context.Path || related.FileName != name {
						t.Fatalf("wrong duplicate participant: %#v", related)
					}
				}
				// The contract does not prescribe labels, grouping, or a zero/one
				// origin for positions. Both coordinate conventions must identify
				// these three physical slots, rather than the count or filename.
				positions := make(map[int]bool)
				for _, detail := range problem.Details {
					for _, number := range regexp.MustCompile(`[0-9]+`).FindAllString(detail.Value, -1) {
						position, _ := strconv.Atoi(number)
						positions[position] = true
					}
				}
				oneBased := positions[2] && positions[4] && positions[6]
				zeroBased := positions[1] && positions[3] && positions[5]
				if !oneBased && !zeroBased {
					t.Fatalf("missing physical ZIP positions: %#v", problem.Details)
				}
			}
		})
	}
}
