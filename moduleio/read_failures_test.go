package moduleio

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func TestReadRejectsCorruptionAndUnsupportedCompression(t *testing.T) {
	raw := ReadFixture(t)
	for _, target := range []string{"l001.json", "manifest.json"} {
		for _, defect := range []string{"crc", "compression"} {
			t.Run(target+"/"+defect, func(t *testing.T) {
				archive := ReadZIP(t, ReadZIPEntry{name: "manifest.json", raw: ReadManifest(t, raw, "l001.json")}, ReadZIPEntry{name: "l001.json", raw: append(append([]byte(nil), raw...), bytes.Repeat([]byte(" "), 65536)...)})
				reader := ReadReader(t, archive)
				for _, entry := range reader.File {
					if entry.Name != target {
						continue
					}
					if defect == "crc" {
						offset, err := entry.DataOffset()
						if err != nil {
							t.Fatal(err)
						}
						// Corrupt trailing whitespace: an initial JSON decoder can succeed.
						archive[int(offset)+int(entry.UncompressedSize64)-1] = '\t'
					} else {
						// Both local and central headers identify this compression method.
						for offset := 0; offset+46 <= len(archive); offset++ {
							if bytes.Equal(archive[offset:offset+4], []byte{'P', 'K', 1, 2}) {
								nameLen := int(archive[offset+28]) | int(archive[offset+29])<<8
								if offset+46+nameLen <= len(archive) && string(archive[offset+46:offset+46+nameLen]) == target {
									archive[offset+10] = 99
									archive[offset+11] = 0
								}
							}
							if bytes.Equal(archive[offset:offset+4], []byte{'P', 'K', 3, 4}) {
								nameLen := int(archive[offset+26]) | int(archive[offset+27])<<8
								if offset+30+nameLen <= len(archive) && string(archive[offset+30:offset+30+nameLen]) == target {
									archive[offset+8] = 99
									archive[offset+9] = 0
								}
							}
						}
					}
				}
				path := ReadPath(t, archive)
				value, problems := Read(path)
				if value != nil {
					t.Fatal("corrupt archive returned a model")
				}
				problem := ReadRequireCode(t, problems, CodeRead)
				wantCause := zip.ErrChecksum
				if defect == "compression" {
					wantCause = zip.ErrAlgorithm
				}
				if !errors.Is(problem, wantCause) || problem.Context.Path != path || problem.Context.FileName != target {
					t.Fatalf("source/cause: %+v", problem)
				}
			})
		}
	}
}

func TestReadRejectsEntryFailuresBeforeLinks(t *testing.T) {
	raw := ReadFixture(t)
	manifestRaw := ReadManifest(t, raw, "l001.json")
	cases := []struct {
		name             string
		lesson, manifest []byte
		code             string
	}{
		{"BOM", append([]byte{0xef, 0xbb, 0xbf}, raw...), manifestRaw, CodeEncoding},
		{"invalid-utf8", append(raw, 0xff), manifestRaw, CodeEncoding},
		{"second-value", append(append([]byte(nil), raw...), []byte(" {}")...), manifestRaw, "lesson.json"},
		{"lesson-schema", bytes.Replace(raw, []byte(`"pages":[`), []byte(`"pages":null,"unused":[`), 1), manifestRaw, "lesson.json"},
		{"manifest-unknown", raw, bytes.Replace(manifestRaw, []byte(`"course":`), []byte(`"unexpected":1,"course":`), 1), manifest.CodeShape},
		{"manifest-unit-null", raw, []byte(`{"formatVersion":"1.0","createdAt":"2026-11-04T09:00:00Z","course":{"title":"Курс","unit":null}}`), manifest.CodeShape},
		{"manifest-missing-order", raw, bytes.Replace(manifestRaw, []byte(`"order":"1",`), nil, 1), manifest.CodeShape},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := ReadPath(t, ReadZIP(t, ReadZIPEntry{name: "l001.json", raw: test.lesson}, ReadZIPEntry{name: "manifest.json", raw: test.manifest}))
			value, problems := Read(path)
			if value != nil {
				t.Fatal("partial model")
			}
			ReadRequireCode(t, problems, test.code)
			for _, problem := range problems {
				if strings.HasPrefix(problem.Code, "module.") {
					t.Fatalf("links ran before contents success: %+v", problem)
				}
				if problem.Context.Path != path {
					t.Fatalf("lost path: %+v", problem)
				}
				for _, detail := range problem.Details {
					if strings.Contains(detail.Value, "AAEC/w==") || strings.Contains(detail.Value, "_____") {
						t.Fatal("lesson body leaked")
					}
				}
				if strings.Contains(problem.Reason, "AAEC/w==") {
					t.Fatal("Base64 leaked")
				}
			}
		})
	}
}

func TestReadArchiveReturnsExactOwnerLinksDiagnostics(t *testing.T) {
	raw := ReadFixture(t)
	for _, test := range []struct {
		name     string
		manifest []byte
		code     string
	}{
		{"missing-file", ReadManifest(t, raw, "l002.json"), module.CodeMissingFile},
		{"missing-final-weight", bytes.Replace(ReadManifest(t, raw, "l001.json"), []byte(`"order":"1"`), []byte(`"order":null`), 1), "catalog.missing_order"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := ReadReader(t, ReadZIP(t, ReadZIPEntry{name: "manifest.json", raw: test.manifest}, ReadZIPEntry{name: "l001.json", raw: raw}))
			context := diagnostic.Context{Path: "owner.zip"}
			contents, problems := readContents(reader.File, context)
			if contents == nil || len(problems) != 0 {
				t.Fatalf("contents: %v", problems)
			}
			expected := module.Validate(contents, context)
			if len(expected) == 0 {
				t.Fatal("fixture must violate final module validation")
			}
			if test.name == "missing-file" {
				ReadRequireCode(t, expected, test.code)
			}
			value, problems := readArchive(reader, context)
			if value != nil || !reflect.DeepEqual(problems, expected) {
				t.Fatalf("delegated diagnostics differ: got %v, want %v", problems, expected)
			}
		})
	}
}

type ReadErrorStream struct{}

func (ReadErrorStream) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestReadLessonEntryClosesAfterFullReadError(t *testing.T) {
	raw := ReadFixture(t)
	reader := ReadReader(t, ReadZIP(t, ReadZIPEntry{name: "l001.json", raw: raw, method: zip.Deflate}))
	closes := 0
	closeError := errors.New("close after read")
	reader.RegisterDecompressor(zip.Deflate, func(io.Reader) io.ReadCloser {
		return &ReadStream{reader: io.MultiReader(bytes.NewReader(raw), ReadErrorStream{}), closes: &closes, closeError: closeError}
	})
	value, problems := readLessonEntry(reader.File[0], diagnostic.Context{Path: "streams.zip"})
	if value != nil || closes != 1 {
		t.Fatalf("partial result or missing close: %v, %d", value, closes)
	}
	if len(problems) != 2 || problems[0].Code != CodeRead || problems[1].Code != CodeClose || !errors.Is(problems[0], io.ErrUnexpectedEOF) || !errors.Is(problems[1], closeError) {
		t.Fatalf("read/close order and causes: %+v", problems)
	}
}

func TestReadDoesNotUseReaderReturnedWithOpenError(t *testing.T) {
	t.Setenv("GODEBUG", "zipinsecurepath=0")
	raw := ReadFixture(t)
	path := ReadPath(t, ReadZIP(t, ReadZIPEntry{name: "manifest.json", raw: ReadManifest(t, raw, "l001.json")}, ReadZIPEntry{name: "l001.json", raw: raw}, ReadZIPEntry{name: "../bad.json", raw: []byte("bad json")}))
	probe, err := zip.OpenReader(path)
	if probe == nil || !errors.Is(err, zip.ErrInsecurePath) {
		t.Fatalf("stdlib fixture: reader=%v, error=%v", probe != nil, err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	before := 0
	if runtime.GOOS == "linux" {
		before = ReadOpenDescriptors(t, path)
	}
	value, problems := Read(path)
	if runtime.GOOS == "linux" && ReadOpenDescriptors(t, path) != before {
		t.Fatal("reader returned with open error leaked")
	}
	if value != nil {
		t.Fatal("open error allowed a model")
	}
	if len(problems) != 1 || problems[0].Code != CodeArchive || !errors.Is(problems[0], zip.ErrInsecurePath) {
		t.Fatalf("used reader after open error: %+v", problems)
	}
}

func ReadOpenDescriptors(t *testing.T, path string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err == nil && target == path {
			count++
		}
	}
	return count
}

func TestReadClosesArchiveOnSuccessAndFailure(t *testing.T) {
	raw := ReadFixture(t)
	for _, valid := range []bool{false, true} {
		archive := ReadZIP(t, ReadZIPEntry{name: "manifest.json", raw: ReadManifest(t, raw, "l001.json")}, ReadZIPEntry{name: "l001.json", raw: raw})
		if !valid {
			archive = ReadZIP(t, ReadZIPEntry{name: "manifest.json", raw: []byte("{")})
		}
		path := ReadPath(t, archive)
		before := 0
		if runtime.GOOS == "linux" {
			before = ReadOpenDescriptors(t, path)
		}
		for range 4 {
			_, problems := Read(path)
			if (len(problems) == 0) != valid {
				t.Fatalf("valid=%v: %v", valid, problems)
			}
		}
		if runtime.GOOS == "linux" && ReadOpenDescriptors(t, path) != before {
			t.Fatal("archive descriptor leaked")
		}
		// Windows additionally makes an open archive observable through rename/delete.
		renamed := path + ".moved"
		if err := os.Rename(path, renamed); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(renamed); err != nil {
			t.Fatal(err)
		}
	}
}
