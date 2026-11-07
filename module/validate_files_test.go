package module

import (
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
)

func TestValidateFilesMissingSnapshots(t *testing.T) {
	for _, scenario := range []struct {
		name string
		file *File
	}{
		{"nil file", nil},
		{"nil snapshot", &File{name: "l001.json"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			problems := validateFiles([]*File{scenario.file}, diagnostic.Context{Path: "архив.zip"})
			if len(problems) == 0 {
				t.Fatal("missing snapshot accepted")
			}
			if scenario.file != nil {
				context := diagnostic.Context{Path: "архив.zip", FileName: "l001.json"}
				wantOwner := lesson.Validate(nil, context)
				ValidateWantCodes(t, wantOwner, lesson.CodeContract)
				var gotOwner []*diagnostic.Error
				fileCount := 0
				for _, problem := range problems {
					if problem == nil {
						t.Fatal("nil diagnostic")
					}
					if problem.Code == CodeFile {
						fileCount++
						if problem.Context != context {
							t.Errorf("missing snapshot context = %+v, want %+v", problem.Context, context)
						}
					} else {
						gotOwner = append(gotOwner, problem)
					}
				}
				if fileCount != 1 {
					t.Errorf("missing snapshot CodeFile count = %d, want 1", fileCount)
				}
				if !slices.EqualFunc(gotOwner, wantOwner, func(got, want *diagnostic.Error) bool {
					return got.Code == want.Code && got.Reason == want.Reason && got.Context == want.Context &&
						slices.Equal(got.Details, want.Details) && slices.Equal(got.Related, want.Related) && got.Cause == want.Cause
				}) {
					t.Errorf("nil snapshot owner diagnostics = %v, want %v", gotOwner, wantOwner)
				}
				return
			}
			for _, problem := range problems {
				if problem.Code != CodeFile {
					t.Errorf("missing snapshot produced %s, want %s", problem.Code, CodeFile)
				}
				if problem.Context.Path != "архив.zip" {
					t.Errorf("archive context lost: %+v", problem.Context)
				}
			}
		})
	}
}

func TestValidateFilesDelegatesNames(t *testing.T) {
	for _, name := range []string{"", "manifest.json", "l000.json", "l01.json", "l0001.json", "L001.json", "l001.JSON", "dir/l001.json", `dir\l001.json`} {
		t.Run(name, func(t *testing.T) {
			file := &File{name: name, value: ValidateLessonFixture(t, "Alpha")}
			_, ownerProblem := identity.ParseFileName(name)
			if ownerProblem == nil {
				t.Fatal("invalid-name scenario is valid")
			}
			problems := validateFiles([]*File{file}, diagnostic.Context{Path: "архив.zip"})
			ValidateWantCodes(t, problems, identity.CodeInvalidFileName)
			if problems[0].Reason != ownerProblem.Reason {
				t.Errorf("identity reason changed: %q, want %q", problems[0].Reason, ownerProblem.Reason)
			}
			if file.name != name {
				t.Fatal("invalid name repaired")
			}
		})
	}
}

func TestValidateFilesKeepsLessonDiagnosticsAndSource(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		breakLesson func(*lesson.Lesson)
	}{
		{"version", func(value *lesson.Lesson) { value.Document.Version = "0" }},
		{"line count", func(value *lesson.Lesson) { value.Document.LineCount = "1.5" }},
		{"pages", func(value *lesson.Lesson) { value.Document.Pages = nil }},
		{"node type", func(value *lesson.Lesson) { value.Document.Pages[0].Nodes[0].Type = "неизвестно" }},
		{"node data", func(value *lesson.Lesson) { value.Document.Pages[0].Nodes[0].Data = nil }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			value := ValidateLessonFixture(t, "Alpha")
			scenario.breakLesson(value)
			context := diagnostic.Context{Path: "вход/урок.json", FileName: "l001.json", LessonID: "Alpha", Line: 17, Column: 3}
			file := &File{name: "l001.json", value: value, context: context}
			want := lesson.Validate(value, context)
			if len(want) == 0 {
				t.Fatal("invalid-lesson scenario is valid")
			}
			got := validateFiles([]*File{file}, diagnostic.Context{Path: "не источник.zip"})
			if len(got) != len(want) {
				t.Fatalf("lesson diagnostics = %v, want %v", got, want)
			}
			for i := range want {
				if got[i].Code != want[i].Code || got[i].Reason != want[i].Reason ||
					got[i].Context != want[i].Context || !slices.Equal(got[i].Details, want[i].Details) {
					t.Errorf("diagnostic %d = %+v, want owner diagnostic %+v", i, got[i], want[i])
				}
			}
		})
	}
}

func TestValidateFilesSuppliesKnownArchiveContext(t *testing.T) {
	value := ValidateLessonFixture(t, "Alpha")
	value.Document.Version = "0"
	file := &File{name: "l003.json", value: value}
	got := validateFiles([]*File{file}, diagnostic.Context{Path: "модуль с пробелами.zip"})
	if len(got) == 0 {
		t.Fatal("invalid revision accepted")
	}
	for _, problem := range got {
		want := diagnostic.Context{Path: "модуль с пробелами.zip", FileName: "l003.json", LessonID: "Alpha"}
		if problem.Context != want {
			t.Errorf("fallback context = %+v, want %+v", problem.Context, want)
		}
	}
}

func TestValidateFilesDuplicateIDsIncludeEverySource(t *testing.T) {
	files := []*File{
		ValidateFileFixture(t, "l001.json", "Alpha"),
		ValidateFileFixture(t, "l002.json", "aLPHA"),
		ValidateFileFixture(t, "l003.json", "ALPHA"),
	}
	problems := validateFiles(files, diagnostic.Context{})
	ValidateWantCodes(t, problems, CodeDuplicateID, CodeDuplicateID)
	for _, problem := range problems {
		ValidateWantRelated(t, problem, files[0].Context(), files[1].Context(), files[2].Context())
	}
}

func TestValidateFilesDoesNotCollapseDuplicateNames(t *testing.T) {
	files := []*File{
		ValidateFileFixture(t, "l001.json", "Alpha"),
		ValidateFileFixture(t, "l001.json", "Beta"),
		ValidateFileFixture(t, "l001.json", "Gamma"),
	}
	problems := validateFiles(files, diagnostic.Context{})
	if len(problems) == 0 {
		t.Fatal("duplicate technical names accepted")
	}
	for _, problem := range problems {
		if problem.Code != CodeDuplicateFile {
			t.Errorf("duplicate name produced %s", problem.Code)
		}
		ValidateWantRelated(t, problem, files[0].Context(), files[1].Context(), files[2].Context())
	}
}

func TestValidateFileAndSchemaErrorsRemainIndependent(t *testing.T) {
	good := ValidateFileFixture(t, "l001.json", "Alpha")
	brokenLesson := ValidateLessonFixture(t, "aLPHA")
	brokenLesson.Document.Version = "0"
	broken := &File{name: "l002.json", value: brokenLesson, context: diagnostic.Context{Path: "ошибка.json"}}
	value := &Module{Files: []*File{good, broken}, Manifest: ValidateFlatFixture(t, ValidateItemFixture(t, good, "1").Entry)}
	value.Manifest.FormatVersion = "нет"
	problems := Validate(value, diagnostic.Context{Path: "архив.zip"})
	owner := lesson.Validate(brokenLesson, diagnostic.Context{})
	if len(owner) != 1 {
		t.Fatalf("fixture lesson errors = %v", owner)
	}
	ValidateWantCodes(t, problems, owner[0].Code, CodeDuplicateID, manifest.CodeVersion)
	if problems[0].Reason != owner[0].Reason {
		t.Error("lesson owner reason replaced")
	}
}

func TestValidateBadFilesDoNotHideOtherFilesOrSchema(t *testing.T) {
	good := ValidateFileFixture(t, "l001.json", "Alpha")
	bad := &File{name: "dir/l002.json", value: ValidateLessonFixture(t, "Beta")}
	value := &Module{Files: []*File{nil, bad, good}, Manifest: ValidateFlatFixture(t, ValidateItemFixture(t, good, "1").Entry)}
	value.Manifest.FormatVersion = "нет"
	ValidateWantCodes(t, Validate(value, diagnostic.Context{Path: "архив.zip"}),
		CodeFile, identity.CodeInvalidFileName, manifest.CodeVersion)
}
