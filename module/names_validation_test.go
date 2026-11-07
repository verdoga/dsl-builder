package module

import (
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func TestNamesValidationAcceptsOrdinaryNamedFiles(t *testing.T) {
	for _, files := range [][]*File{
		nil, {},
		{{name: "l001.json"}},
		{{name: "l1000.json"}, {name: "l999.json"}, {name: "l002.json"}},
	} {
		if problems := validateFileNames(files, diagnostic.Context{}); len(problems) != 0 {
			t.Fatalf("проверка имён не должна добавлять проверку модели урока: %v", problems)
		}
	}
}

func TestNamesValidationPreservesIdentityDiagnostics(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip", Line: 3, Column: 7}
	for _, name := range []string{
		"", "manifest.json", "l000.json", "l01.json", "l0001.json",
		"L001.json", "l001.JSON", "dir/l001.json", `dir\l001.json`,
		" l001.json", "l001.json\n", "l-01.json", "l1e3.json", "l１２３.json",
	} {
		t.Run(name, func(t *testing.T) {
			_, expected := identity.ParseFileName(name)
			if expected == nil {
				t.Fatalf("неверное предусловие для имени %q", name)
			}
			problems := validateFileNames([]*File{{name: name}}, context)
			if len(problems) != 1 || problems[0] == nil {
				t.Fatalf("problems=%v, want one identity error", problems)
			}
			got := problems[0]
			if got.Code != expected.Code || got.Reason != expected.Reason {
				t.Fatalf("изменены код/причина identity: %+v, want %+v", got, expected)
			}
			for _, detail := range expected.Details {
				if !slices.Contains(got.Details, detail) {
					t.Fatalf("потеряна подробность identity %+v: %+v", detail, got)
				}
			}
			if got.Context.Path != context.Path || got.Context.FileName != name {
				t.Fatalf("потеряны известные архив/имя: %+v", got.Context)
			}
		})
	}
}

func TestNamesValidationRejectsNilWithoutHidingOtherErrors(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip"}
	problems := validateFileNames([]*File{nil, {name: "l000.json"}, nil}, context)
	counts := map[string]int{}
	for _, problem := range problems {
		if problem == nil || problem.Reason == "" || problem.Context.Path != context.Path {
			t.Fatalf("неполная диагностика: %+v", problem)
		}
		counts[problem.Code]++
	}
	if counts[CodeFile] != 2 || counts[identity.CodeInvalidFileName] != 1 {
		t.Fatalf("независимые ошибки не собраны: %v", counts)
	}
}

func TestNamesValidationReportsAllDuplicateContexts(t *testing.T) {
	contexts := []diagnostic.Context{
		{Path: "Я/first.json", FileName: "l007.json", LessonID: "First", Line: 2, Column: 3},
		{Path: "А/second.json", FileName: "l007.json", LessonID: "Second", Line: 4, Column: 5},
		{Path: "М/third.json", FileName: "l007.json", LessonID: "Third", Line: 6, Column: 7},
	}
	files := []*File{
		{name: "l007.json", context: contexts[0]},
		{name: "l001.json"},
		{name: "l007.json", context: contexts[1]},
		{name: "l007.json", context: contexts[2]},
	}
	before := slices.Clone(files)
	problems := validateFileNames(files, diagnostic.Context{Path: "fallback.zip"})
	if len(problems) == 0 {
		t.Fatal("повтор полного имени не обнаружен")
	}
	for _, problem := range problems {
		if problem == nil || problem.Code != CodeDuplicateFile || problem.Reason == "" {
			t.Fatalf("неверная диагностика повтора: %+v", problem)
		}
		if !slices.Equal(problem.Related, contexts) {
			t.Fatalf("Related=%+v, want all contexts in input order %+v", problem.Related, contexts)
		}
	}
	if !slices.Equal(files, before) {
		t.Fatal("проверка повторов изменила срез файлов")
	}
	second := validateFileNames(files, diagnostic.Context{Path: "fallback.zip"})
	if len(second) != len(problems) {
		t.Fatal("число ошибок неустойчиво")
	}
	for index, got := range second {
		want := problems[index]
		if got == nil || got.Code != want.Code || got.Reason != want.Reason || got.Context != want.Context ||
			!slices.Equal(got.Related, want.Related) || !slices.Equal(got.Details, want.Details) {
			t.Fatalf("диагностика неустойчива: index=%d", index)
		}
	}
}
