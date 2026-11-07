package moduleio_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

func IntegrationProblem(t *testing.T, problems []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, problem := range problems {
		if problem == nil {
			t.Fatal("nil diagnostic")
		}
		if problem.Code == code {
			return problem
		}
	}
	t.Fatalf("missing %s in %v", code, problems)
	return nil
}

func IntegrationReject(t *testing.T, raw []byte, code string) *diagnostic.Error {
	t.Helper()
	path := IntegrationPath(t, raw)
	value, problems := moduleio.Read(path)
	if value != nil || len(problems) == 0 {
		t.Fatalf("invalid archive accepted: %v %v", value, problems)
	}
	problem := IntegrationProblem(t, problems, code)
	if problem.Context.Path != path || problem.Reason == "" {
		t.Fatalf("lost context/reason: %+v", problem)
	}
	verified := moduleio.Verify(path)
	IntegrationProblem(t, verified, code)
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("rejected source changed", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal("rejected archive descriptor remains open", err)
	}
	return problem
}

func TestIntegrationRejectZIPAndJSON(t *testing.T) {
	model := IntegrationModel(t, IntegrationNew(t, "l001.json", "AB88"))
	manifestRaw := IntegrationJSON(t, model.Manifest)
	lessonRaw := IntegrationJSON(t, model.Files[0].Lesson())
	valid := []IntegrationEntry{{Name: "manifest.json", Raw: manifestRaw}, {Name: "l001.json", Raw: lessonRaw}}
	for _, name := range []string{"../l001.json", "a/l001.json", `a\l001.json`, "/l001.json", "L001.json", "l000.json", "l01.json", "l0001.json", "extra.txt", "Manifest.json"} {
		t.Run(name, func(t *testing.T) {
			entries := append([]IntegrationEntry(nil), valid...)
			entries[1].Name = name
			IntegrationReject(t, IntegrationZIP(t, entries...), moduleio.CodeEntryName)
		})
	}
	for _, mode := range []os.FileMode{os.ModeDir | 0755, os.ModeSymlink | 0777, os.ModeNamedPipe | 0600} {
		t.Run(mode.String(), func(t *testing.T) {
			entries := append([]IntegrationEntry(nil), valid...)
			entries[1].Mode = mode
			IntegrationReject(t, IntegrationZIP(t, entries...), moduleio.CodeEntryType)
		})
	}
	t.Run("missing-manifest", func(t *testing.T) { IntegrationReject(t, IntegrationZIP(t, valid[1]), moduleio.CodeManifest) })
	t.Run("duplicate-manifest", func(t *testing.T) {
		entries := append(append([]IntegrationEntry(nil), valid...), valid[0])
		problem := IntegrationReject(t, IntegrationZIP(t, entries...), moduleio.CodeDuplicateEntry)
		if len(problem.Related) != 2 || len(problem.Details) < 2 {
			t.Fatalf("manifest duplicate group lost: %+v", problem)
		}
	})
	t.Run("not-zip", func(t *testing.T) { IntegrationReject(t, []byte("not a ZIP"), moduleio.CodeArchive) })
	t.Run("three-duplicate-participants", func(t *testing.T) {
		entries := append(append([]IntegrationEntry(nil), valid...), valid[1], valid[1])
		problem := IntegrationReject(t, IntegrationZIP(t, entries...), moduleio.CodeDuplicateEntry)
		if len(problem.Related) != 3 || len(problem.Details) < 3 {
			t.Fatalf("duplicate group lost: %+v", problem)
		}
		for _, related := range problem.Related {
			if related.FileName != "l001.json" || related.Path != problem.Context.Path {
				t.Fatal(related)
			}
		}
	})
	for _, tc := range []struct {
		name string
		raw  []byte
		code string
	}{
		{"bom", append([]byte{0xef, 0xbb, 0xbf}, lessonRaw...), moduleio.CodeEncoding},
		{"utf8", append(append([]byte(nil), lessonRaw...), 0xff), moduleio.CodeEncoding},
		{"second-json", append(append([]byte(nil), lessonRaw...), []byte(" {}")...), lesson.CodeJSON},
		{"garbage-tail", append(append([]byte(nil), lessonRaw...), []byte(" x")...), lesson.CodeJSON},
		{"schema", bytes.Replace(lessonRaw, []byte(`"pages": []`), []byte(`"pages": null`), 1), lesson.CodeJSON},
		{"id", bytes.Replace(lessonRaw, []byte(`"AB88"`), []byte(`"bad id"`), 1), identity.CodeInvalidLessonID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := append([]IntegrationEntry(nil), valid...)
			entries[1].Raw = tc.raw
			problem := IntegrationReject(t, IntegrationZIP(t, entries...), tc.code)
			if problem.Context.FileName != "l001.json" {
				t.Fatal(problem.Context)
			}
			if tc.name == "schema" {
				pointer := false
				for _, detail := range problem.Details {
					if detail.Value == "/document/pages" {
						pointer = true
					}
				}
				if !pointer {
					t.Fatalf("schema pointer lost: %+v", problem)
				}
			}
		})
	}
	for _, tc := range []struct {
		name string
		raw  []byte
		code string
	}{
		{"bom", append([]byte{0xef, 0xbb, 0xbf}, manifestRaw...), moduleio.CodeEncoding},
		{"utf8", append(append([]byte(nil), manifestRaw...), 0xff), moduleio.CodeEncoding},
		{"null-unit", bytes.Replace(manifestRaw, []byte(`"title": "Курс 日本語"`), []byte(`"title": "Курс 日本語", "unit": null`), 1), manifest.CodeShape},
		{"unknown", bytes.Replace(manifestRaw, []byte(`"course": {`), []byte(`"unknown": true, "course": {`), 1), manifest.CodeShape},
		{"missing-subtitle", bytes.Replace(manifestRaw, []byte(`"subtitle": null,`), nil, 1), manifest.CodeShape},
		{"second-json", append(append([]byte(nil), manifestRaw...), []byte(" []")...), moduleio.CodeJSON},
		{"metadata", bytes.Replace(manifestRaw, []byte(`"title": "Урок AB88"`), []byte(`"title": "Другое"`), 1), module.CodeMetadata},
		{"missing-file", bytes.Replace(manifestRaw, []byte("l001.json"), []byte("l002.json"), 1), module.CodeMissingFile},
	} {
		t.Run("manifest-"+tc.name, func(t *testing.T) {
			entries := append([]IntegrationEntry(nil), valid...)
			entries[0].Raw = tc.raw
			IntegrationReject(t, IntegrationZIP(t, entries...), tc.code)
		})
	}
}

func TestIntegrationVerifyReadsPastValidJSON(t *testing.T) {
	model := IntegrationModel(t, IntegrationNew(t, "l001.json", "AB88"))
	lessonRaw := append(IntegrationJSON(t, model.Files[0].Lesson()), bytes.Repeat([]byte(" "), 128*1024)...)
	archive := IntegrationZIP(t, IntegrationEntry{Name: "manifest.json", Raw: IntegrationJSON(t, model.Manifest)}, IntegrationEntry{Name: "l001.json", Raw: lessonRaw})
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := reader.File[1].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	archive[int(offset)+len(lessonRaw)-1] = '\n'
	problem := IntegrationReject(t, archive, moduleio.CodeRead)
	if !errors.Is(problem, zip.ErrChecksum) || problem.Context.FileName != "l001.json" {
		t.Fatalf("CRC cause/context lost: %+v", problem)
	}
}

func TestIntegrationUnsupportedCompressionAndOpenCause(t *testing.T) {
	model := IntegrationModel(t, IntegrationNew(t, "l001.json", "AB88"))
	archive := IntegrationZIP(t, IntegrationEntry{Name: "manifest.json", Raw: IntegrationJSON(t, model.Manifest)}, IntegrationEntry{Name: "l001.json", Raw: IntegrationJSON(t, model.Files[0].Lesson())})
	// Оба заголовка меняются согласованно: структура ZIP остаётся корректной.
	for i := 0; i+46 < len(archive); i++ {
		if bytes.Equal(archive[i:i+4], []byte{'P', 'K', 3, 4}) {
			archive[i+8], archive[i+9] = 99, 0
		}
		if bytes.Equal(archive[i:i+4], []byte{'P', 'K', 1, 2}) {
			archive[i+10], archive[i+11] = 99, 0
		}
	}
	IntegrationReject(t, archive, moduleio.CodeRead)
	path := IntegrationPath(t, nil)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	value, problems := moduleio.Read(path)
	if value != nil {
		t.Fatal("partial model on open failure")
	}
	problem := IntegrationProblem(t, problems, moduleio.CodeOpen)
	if !errors.Is(problem, os.ErrNotExist) || problem.Context.Path != path {
		t.Fatalf("open cause/context lost: %+v", problem)
	}
}
