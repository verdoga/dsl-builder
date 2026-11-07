package moduleio_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

type VerifyEntry struct {
	Name   string
	Raw    []byte
	Method uint16
	Mode   fs.FileMode
}

func VerifyFixture(t *testing.T, unit bool) []VerifyEntry {
	t.Helper()
	raw, err := os.ReadFile("testdata/verify/lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	context := diagnostic.Context{Path: "fixture-source.json", FileName: "l007.json"}
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		t.Fatalf("fixture lesson: %v", problems)
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	file, problems := module.NewFile(context.FileName, &value, context)
	if len(problems) != 0 {
		t.Fatalf("fixture file: %v", problems)
	}
	order := "000123456789012345678901234567890"
	doc := value.Document
	entries := []manifest.Entry{{ID: doc.ID, FileName: context.FileName, Order: &order,
		Title: doc.Title, Subtitle: doc.Subtitle, LineCount: doc.LineCount,
		CreatedAt: doc.CreatedAt, Version: doc.Version}}
	sections := []manifest.Section{}
	course := manifest.Course{Title: "Курс", Lessons: &entries, Sections: &sections}
	if unit {
		course = manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит",
			Lessons: []manifest.Entry{}, Sections: []manifest.Section{{Title: "Секция", Lessons: entries}}}}
	}
	model := &module.Module{Manifest: manifest.Manifest{FormatVersion: "1.0",
		CreatedAt: "2026-11-06T12:00:00Z", Course: course}, Files: []*module.File{file}}
	if problems := module.Validate(model, context); len(problems) != 0 {
		t.Fatalf("fixture module: %v", problems)
	}
	manifestRaw, err := json.MarshalIndent(model.Manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.UseNumber()
	if err := decoder.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	if problems := manifest.ValidateShape(tree, context); len(problems) != 0 {
		t.Fatalf("fixture manifest shape: %v", problems)
	}
	return []VerifyEntry{{Name: "l007.json", Raw: raw, Method: zip.Deflate},
		{Name: "manifest.json", Raw: manifestRaw, Method: zip.Deflate}}
}

func VerifyZIP(t *testing.T, entries []VerifyEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: entry.Method}
		if entry.Mode != 0 {
			header.SetMode(entry.Mode)
		}
		stream, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Write(entry.Raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func VerifyPath(t *testing.T, raw []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "кандидат с пробелом.tmp")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func VerifyReadResult(t *testing.T, path string, codes ...string) []*diagnostic.Error {
	t.Helper()
	model, expected := moduleio.Read(path)
	actual := moduleio.Verify(path)
	if len(actual) != len(expected) || (actual == nil) != (expected == nil) {
		t.Fatalf("Verify diagnostic list differs from Read: %v / %v", actual, expected)
	}
	for i, problem := range actual {
		want := expected[i]
		if problem == nil || want == nil {
			t.Fatalf("nil diagnostic at %d", i)
		}
		if problem.Code != want.Code || problem.Reason != want.Reason || problem.Context != want.Context ||
			!slices.Equal(problem.Related, want.Related) || !slices.Equal(problem.Details, want.Details) {
			t.Fatalf("Verify changed diagnostic %d: %#v / %#v", i, problem, want)
		}
		VerifyCause(t, problem.Cause, want.Cause)
		if problem.Context.Path != path || problem.Reason == "" {
			t.Fatalf("missing path or reason: %#v", problem)
		}
		text := fmt.Sprint(problem, problem.Context, problem.Related, problem.Details, problem.Cause)
		for _, private := range []string{"VERIFY-PRIVATE-TEXT", "VkVSSUZZLVBSSVZBVEUtQllURVM=", "VERIFY-UNKNOWN-PROPERTY"} {
			if strings.Contains(text, private) {
				t.Fatalf("diagnostic disclosed fixture body: %s", problem.Code)
			}
		}
	}
	if len(codes) == 0 {
		if len(actual) != 0 || model == nil {
			t.Fatalf("valid archive rejected: %v", actual)
		}
	} else {
		if model != nil {
			t.Fatal("Read returned a partial model")
		}
		for _, code := range codes {
			VerifyHasCode(t, actual, code)
		}
	}
	return actual
}

func VerifyCause(t *testing.T, actual, expected error) {
	t.Helper()
	if (actual == nil) != (expected == nil) {
		t.Fatal("Verify changed Cause presence")
	}
	if actual == nil {
		return
	}
	if fmt.Sprintf("%T", actual) != fmt.Sprintf("%T", expected) || actual.Error() != expected.Error() {
		t.Fatalf("Verify changed Cause: %T / %T", actual, expected)
	}
	for _, sentinel := range []error{os.ErrNotExist, zip.ErrFormat, zip.ErrChecksum, zip.ErrAlgorithm} {
		if errors.Is(actual, sentinel) != errors.Is(expected, sentinel) {
			t.Fatal("Verify changed errors.Is chain")
		}
	}
	var gotDiagnostic, wantDiagnostic *diagnostic.Error
	if errors.As(actual, &gotDiagnostic) != errors.As(expected, &wantDiagnostic) {
		t.Fatal("Verify changed diagnostic Cause")
	}
	if gotDiagnostic != nil && (gotDiagnostic.Code != wantDiagnostic.Code ||
		gotDiagnostic.Context != wantDiagnostic.Context || !slices.Equal(gotDiagnostic.Details, wantDiagnostic.Details) ||
		!slices.Equal(gotDiagnostic.Related, wantDiagnostic.Related)) {
		t.Fatal("Verify changed owner Cause fields")
	}
	VerifyCause(t, errors.Unwrap(actual), errors.Unwrap(expected))
}

func VerifyHasCode(t *testing.T, problems []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, problem := range problems {
		if problem != nil && problem.Code == code {
			return problem
		}
	}
	t.Fatalf("missing independent expected code %s: %v", code, problems)
	return nil
}
