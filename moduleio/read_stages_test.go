package moduleio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func TestReadArchiveStopsAfterPhysicalErrors(t *testing.T) {
	raw := ReadFixture(t)
	archive := ReadZIP(t,
		ReadZIPEntry{name: "l002.json", raw: []byte("bad json")},
		ReadZIPEntry{name: "sub/l001.json", raw: raw},
		ReadZIPEntry{name: "manifest.json", raw: []byte("bad manifest")})
	value, problems := readArchive(ReadReader(t, archive), diagnostic.Context{Path: "physical.zip"})
	if value != nil {
		t.Fatal("partial model")
	}
	ReadRequireCode(t, problems, CodeEntryName)
	for _, problem := range problems {
		if problem.Code == CodeJSON || strings.HasPrefix(problem.Code, "lesson.") || strings.HasPrefix(problem.Code, "manifest.") || strings.HasPrefix(problem.Code, "module.") {
			t.Fatalf("later stage ran: %+v", problem)
		}
	}
}

func TestReadContentsCollectsEntryErrorsInZIPOrder(t *testing.T) {
	raw := ReadFixture(t)
	badLesson := bytes.Replace(raw, []byte(`"title":"Урок Ω",`), nil, 1)
	badManifest := bytes.Replace(ReadManifest(t, raw, "l001.json"), []byte(`"subtitle":"",`), nil, 1)
	reader := ReadReader(t, ReadZIP(t,
		ReadZIPEntry{name: "l003.json", raw: []byte{0xff}},
		ReadZIPEntry{name: "manifest.json", raw: badManifest},
		ReadZIPEntry{name: "l001.json", raw: badLesson},
		ReadZIPEntry{name: "l002.json", raw: raw}))
	context := diagnostic.Context{Path: "entries.zip", Line: 7, Column: 3}
	value, problems := readContents(reader.File, context)
	if value != nil {
		t.Fatal("partial contents")
	}
	var got []string
	for _, problem := range problems {
		if problem == nil {
			t.Fatal("nil diagnostic")
		}
		got = append(got, problem.Context.FileName)
		if problem.Context.Path != context.Path || problem.Context.Line != 7 || problem.Context.Column != 3 {
			t.Fatalf("lost context: %+v", problem.Context)
		}
		if problem.Code == module.CodeMetadata || problem.Code == module.CodeUnlistedFile {
			t.Fatal("module validation ran after entry errors")
		}
	}
	if !reflect.DeepEqual(got, []string{"l003.json", "l001.json", "l001.json"}) {
		t.Fatalf("entry error order: %v", got)
	}
	// The shape owner associates the manifest error with its referenced lesson.
	ReadRequireCode(t, problems, CodeEncoding)
	ReadRequireCode(t, problems, manifest.CodeShape)
	ReadRequireCode(t, problems, lesson.CodeJSON)
	owner := lesson.ValidateJSON(badLesson, diagnostic.Context{Path: context.Path, FileName: "l001.json", Line: 7, Column: 3})
	for _, want := range owner {
		found := false
		for _, got := range problems {
			if reflect.DeepEqual(got, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("lost lesson diagnostic: %+v", want)
		}
	}
}

func TestReadArchiveDelegatesModuleValidationOnlyAfterContents(t *testing.T) {
	raw := ReadFixture(t)
	manifestRaw := bytes.Replace(ReadManifest(t, raw, "l001.json"), []byte(`"title":"Урок Ω"`), []byte(`"title":"Different"`), 1)
	reader := ReadReader(t, ReadZIP(t, ReadZIPEntry{name: "manifest.json", raw: manifestRaw}, ReadZIPEntry{name: "l001.json", raw: raw}))
	context := diagnostic.Context{Path: "links.zip"}
	contents, problems := readContents(reader.File, context)
	if contents == nil || len(problems) != 0 {
		t.Fatalf("contents must precede semantic validation: %v", problems)
	}
	expected := module.Validate(contents, context)
	ReadRequireCode(t, expected, module.CodeMetadata)
	value, problems := readArchive(reader, context)
	if value != nil || !reflect.DeepEqual(problems, expected) {
		t.Fatalf("module owner diagnostics: got %+v; want %+v", problems, expected)
	}
}

func TestReadLessonAndManifestEntryPreserveKnownSource(t *testing.T) {
	raw := ReadFixture(t)
	reader := ReadReader(t, ReadZIP(t, ReadZIPEntry{name: "l1000.json", raw: raw}, ReadZIPEntry{name: "manifest.json", raw: ReadManifest(t, raw, "l1000.json")}))
	context := diagnostic.Context{Path: "source.zip", FileName: "stale", LessonID: "old", PageID: "known-page", Line: 17, Column: 4}
	file, problems := readLessonEntry(reader.File[0], context)
	if file == nil || len(problems) != 0 {
		t.Fatalf("lesson: %v", problems)
	}
	expected := context
	expected.FileName = "l1000.json"
	expected.LessonID = "MiXeD+1"
	if file.Context() != expected || !bytes.Equal(file.OriginalBytes(), raw) {
		t.Fatalf("source/raw: %+v", file.Context())
	}
	m, problems := readManifestEntry(reader.File[1], context)
	if m == nil || len(problems) != 0 || (*m.Course.Lessons)[0].FileName != "l1000.json" {
		t.Fatalf("manifest: %v", problems)
	}
	bad := ReadReader(t, ReadZIP(t, ReadZIPEntry{name: "manifest.json", raw: []byte(`{`)}))
	result, problems := readManifestEntry(bad.File[0], context)
	if result != nil {
		t.Fatal("partial manifest")
	}
	for _, problem := range problems {
		if problem.Context.FileName != module.ManifestFileName || problem.Context.Path != context.Path {
			t.Fatalf("manifest source: %+v", problem.Context)
		}
	}
}

type ReadStream struct {
	reader     io.Reader
	closes     *int
	closeError error
}

func (stream *ReadStream) Read(p []byte) (int, error) { return stream.reader.Read(p) }
func (stream *ReadStream) Close() error               { *stream.closes++; return stream.closeError }

func TestReadEntryPipelinesCloseStreamsAndKeepReadThenCloseErrors(t *testing.T) {
	raw := ReadFixture(t)
	for _, target := range []string{"l001.json", "manifest.json"} {
		t.Run(target, func(t *testing.T) {
			payload := raw
			if target == "manifest.json" {
				payload = ReadManifest(t, raw, "l001.json")
			}
			for _, closeFails := range []bool{false, true} {
				t.Run(map[bool]string{false: "success", true: "close-error"}[closeFails], func(t *testing.T) {
					reader := ReadReader(t, ReadZIP(t, ReadZIPEntry{name: target, raw: payload, method: zip.Deflate}))
					closes := 0
					sentinel := errors.New("read test close")
					var closeError error
					if closeFails {
						closeError = sentinel
					}
					reader.RegisterDecompressor(zip.Deflate, func(io.Reader) io.ReadCloser {
						return &ReadStream{reader: bytes.NewReader(payload), closes: &closes, closeError: closeError}
					})
					var value any
					var problems []*diagnostic.Error
					if target == "manifest.json" {
						m, p := readManifestEntry(reader.File[0], diagnostic.Context{})
						if m != nil {
							value = m
						}
						problems = p
					} else {
						f, p := readLessonEntry(reader.File[0], diagnostic.Context{})
						if f != nil {
							value = f
						}
						problems = p
					}
					if closes != 1 {
						t.Fatalf("stream closes=%d", closes)
					}
					if closeFails {
						if value != nil {
							t.Fatal("model after close failure")
						}
						if !errors.Is(ReadRequireCode(t, problems, CodeClose), sentinel) {
							t.Fatal("lost close cause")
						}
					} else if value == nil || len(problems) != 0 {
						t.Fatalf("valid stream: %v", problems)
					}
				})
			}
		})
	}
}

func TestReadContentsPreservesDuplicateIDsUntilModuleValidation(t *testing.T) {
	raw := ReadFixture(t)
	reader := ReadReader(t, ReadZIP(t, ReadZIPEntry{name: "l002.json", raw: raw}, ReadZIPEntry{name: "manifest.json", raw: ReadManifest(t, raw, "l001.json", "l002.json")}, ReadZIPEntry{name: "l001.json", raw: raw}))
	contents, problems := readContents(reader.File, diagnostic.Context{})
	if contents == nil || len(problems) != 0 || len(contents.Files) != 2 {
		t.Fatalf("contents collapsed or validated IDs: %v", problems)
	}
	value, problems := readArchive(reader, diagnostic.Context{})
	if value != nil {
		t.Fatal("duplicate ID module accepted")
	}
	ReadRequireCode(t, problems, module.CodeDuplicateID)
}

func TestReadManifestNumbersRemainJSONNumbers(t *testing.T) {
	raw := ReadFixture(t)
	reader := ReadReader(t, ReadZIP(t, ReadZIPEntry{name: "manifest.json", raw: ReadManifest(t, raw, "l001.json")}))
	m, problems := readManifestEntry(reader.File[0], diagnostic.Context{})
	if m == nil || len(problems) != 0 {
		t.Fatalf("number decode: %v", problems)
	}
	entry := (*m.Course.Lessons)[0]
	if entry.Version != json.Number("9007199254740993.0") || entry.LineCount != json.Number("1e30") {
		t.Fatal("number precision or lexeme changed")
	}
}
