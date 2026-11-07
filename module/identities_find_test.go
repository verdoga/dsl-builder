package module

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

func idsFile(t *testing.T, name, id string) *File {
	t.Helper()
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: id, Version: json.Number("19"), Title: "Урок " + id, LineCount: json.Number("7"),
		CreatedAt: "2026-11-04T09:00:00Z", Pages: []lesson.Page{},
	}}
	context := diagnostic.Context{Path: "архив с пробелом.zip", FileName: name, LessonID: id}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		t.Fatalf("fixture lesson JSON invalid: %+v", problems)
	}
	var decoded lesson.Lesson
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	file, problems := LoadedFile(name, &decoded, raw, context)
	if len(problems) != 0 || file == nil {
		t.Fatalf("LoadedFile fixture: file=%v errors=%+v", file, problems)
	}
	return file
}

func TestIDsFindOnePreservesStoredIdentityNameBytesAndInputs(t *testing.T) {
	first := idsFile(t, "l019.json", "Другой")
	want := idsFile(t, "l1007.json", "УРОК+А:B")
	files := []*File{first, want}
	beforeFiles := slices.Clone(files)
	beforeRaw := want.OriginalBytes()
	beforeLesson, err := json.Marshal(want.Lesson())
	if err != nil {
		t.Fatal(err)
	}
	context := diagnostic.Context{Path: "вход/новый.json", LessonID: "урок+а:b", Line: 3, Column: 5}
	got, problems := FindByID(files, "урок+а:b", context)
	idsRequireCodes(t, problems)
	if got != want || got.FileName() != "l1007.json" || got.Lesson().Document.ID != "УРОК+А:B" {
		t.Fatalf("FindByID = %v, want stored file/name/ID", got)
	}
	if !slices.Equal(files, beforeFiles) || !bytes.Equal(got.OriginalBytes(), beforeRaw) {
		t.Fatal("find changed files or original bytes")
	}
	copyOfLesson := got.Lesson()
	copyOfLesson.Document.ID = "изменено"
	copyOfRaw := got.OriginalBytes()
	copyOfRaw[0] = '!'
	afterLesson, err := json.Marshal(want.Lesson())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterLesson, beforeLesson) || !bytes.Equal(want.OriginalBytes(), beforeRaw) {
		t.Fatal("find exposed mutable stored data")
	}
}

func TestIDsFindMissingAndAmbiguousRejectPartialSelection(t *testing.T) {
	a := idsFile(t, "l001.json", "Alpha")
	b := idsFile(t, "l002.json", "ALPHA")
	c := idsFile(t, "l003.json", "alpha")
	context := diagnostic.Context{Path: "вход.json", LessonID: "aLpHa", Line: 4}
	for _, files := range [][]*File{nil, {}, {a}} {
		got, problems := FindByID(files, "Missing", diagnostic.Context{Path: "вход.json", LessonID: "Missing"})
		if got != nil {
			t.Fatalf("missing returned partial file: %v", got)
		}
		idsRequireCodes(t, problems, CodeMissingLesson)
		if problems[0].Context.Path != "вход.json" || problems[0].Context.LessonID != "Missing" {
			t.Fatalf("missing lost query context: %+v", problems[0].Context)
		}
	}
	for _, files := range [][]*File{{a, b}, {c, a, b}} {
		got, problems := FindByID(files, "aLpHa", context)
		if got != nil {
			t.Fatalf("ambiguous returned first match: %v", got)
		}
		idsRequireCodes(t, problems, CodeAmbiguousLesson)
		want := make([]diagnostic.Context, len(files))
		for i, file := range files {
			want[i] = file.Context()
		}
		idsRequireRelated(t, problems[0], want)
		first := idsDiagnosticJSON(t, problems)
		for iteration := 0; iteration < 10; iteration++ {
			got, problems = FindByID(files, "aLpHa", context)
			if got != nil || idsDiagnosticJSON(t, problems) != first {
				t.Fatal("ambiguous selection is unstable")
			}
		}
	}
}

func TestIDsFindInvalidQueryPreservesDependencyCode(t *testing.T) {
	files := []*File{idsFile(t, "l001.json", "Valid")}
	for _, id := range []string{"", " Valid", "Valid\n", "A\u00a0B", string([]byte{0xff})} {
		context := diagnostic.Context{Path: "технический.json", LessonID: id, Line: 7, Column: 9}
		got, problems := FindByID(files, id, context)
		if got != nil {
			t.Fatal("invalid query returned file")
		}
		idsRequireCodes(t, problems, identity.CodeInvalidLessonID)
		want := identity.ValidateLessonID(id)
		if problems[0].Reason != want.Reason || problems[0].Context != context {
			t.Fatalf("invalid query lost dependency diagnostic: %+v", problems[0])
		}
	}
}

func TestIDsFindAnyNilOrZeroCandidateBlocksOtherwiseSuccessfulMatch(t *testing.T) {
	match := idsFile(t, "l008.json", "Target")
	other := idsFile(t, "l009.json", "Other")
	for _, bad := range []*File{nil, {}, {name: "l004.json", context: diagnostic.Context{Path: "архив.zip", FileName: "l004.json"}}} {
		for _, files := range [][]*File{{bad}, {bad, match}, {match, bad}, {other, match, bad}, {bad, other, match}} {
			before := slices.Clone(files)
			got, problems := FindByID(files, "target", diagnostic.Context{Path: "новый.json", LessonID: "target"})
			if got != nil {
				t.Fatalf("bad candidate silently skipped: %v", got)
			}
			found := false
			for _, problem := range problems {
				if problem != nil && problem.Code == CodeFile && problem.Reason != "" {
					found = true
				}
			}
			if !found {
				t.Fatalf("bad candidate did not produce CodeFile: %+v", problems)
			}
			if !slices.Equal(files, before) {
				t.Fatal("find changed candidate slice")
			}
		}
	}
}

func TestIDsFindLowercaseKeysDoNotNormalizeOrUseEqualFold(t *testing.T) {
	accent := idsFile(t, "l001.json", "É")
	sigma := idsFile(t, "l002.json", "Σ")
	for _, query := range []string{"e\u0301", "ς"} {
		got, problems := FindByID([]*File{accent, sigma}, query, diagnostic.Context{LessonID: query})
		if got != nil {
			t.Fatalf("query %q selected unrelated identity", query)
		}
		idsRequireCodes(t, problems, CodeMissingLesson)
	}
	got, problems := FindByID([]*File{accent, sigma}, "é", diagnostic.Context{LessonID: "é"})
	idsRequireCodes(t, problems)
	if got != accent {
		t.Fatal("Unicode case variant did not select stored identity")
	}
}
