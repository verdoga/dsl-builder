package resourcefind

import (
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestRFErrorsSelectionKnownContextAndExactDetails(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Входные JSON\A.json`, FileName: "l001.json", LessonID: "EfX7", PageID: "efx7-p001", Line: 35, Column: 14}
	directories := []string{`C:\Медиа\Аудио`, `D:\Другой каталог`, `C:\Медиа\Аудио`}
	for _, kind := range []string{"audio", "video", "image", "script", "subtitles"} {
		t.Run(kind, func(t *testing.T) {
			source := " \t003. День \\ demo \".vtt  "
			got := selectionError(CodeAmbiguous, "Найдено несколько подходящих ресурсов", context, kind, source, directories, nil)
			if got == nil {
				t.Fatal("selectionError returned nil")
			}
			if got.Code != CodeAmbiguous || got.Reason != "Найдено несколько подходящих ресурсов" || got.Context != context {
				t.Errorf("owner-provided diagnostic fields changed: %#v", got)
			}
			if got.Cause != nil || len(got.Related) != 0 {
				t.Errorf("unknown Cause or candidates fabricated: %#v", got)
			}
			rfErrorsRequireDetails(t, got.Details, append([]string{kind, source}, directories...))
		})
	}
}

func TestRFErrorsSelectionSortsAllCandidatesWithoutChangingInputs(t *testing.T) {
	paths := []string{`D:\Записи\clip.mp3`, `C:\Медиа\я.mp3`, `C:/Медиа/b.mp3`, `C:\Медиа\a.mp3`, `C:\Медиа\A.mp3`}
	wantPaths := []string{`C:\Медиа\A.mp3`, `C:\Медиа\a.mp3`, `C:/Медиа/b.mp3`, `C:\Медиа\я.mp3`, `D:\Записи\clip.mp3`}
	directories := []string{`D:\Записи`, `C:\Медиа`}
	context := diagnostic.Context{Path: `E:\JSON\вход.json`, LessonID: "A001", Line: 7}
	for iteration := 0; iteration < 3; iteration++ {
		beforePaths := slices.Clone(paths)
		beforeDirectories := slices.Clone(directories)
		got := selectionError(CodeAmbiguous, "Неоднозначный ресурс", context, "audio", " .mp3 ", directories, paths)
		if got == nil {
			t.Fatal("selectionError returned nil")
		}
		if !slices.Equal(paths, beforePaths) || !slices.Equal(directories, beforeDirectories) {
			t.Error("selectionError mutated caller-owned slices")
		}
		if len(got.Related) != len(wantPaths) {
			t.Fatalf("Related = %#v, want all %d candidates", got.Related, len(wantPaths))
		}
		for i, path := range wantPaths {
			if got.Related[i] != (diagnostic.Context{Path: path}) {
				t.Errorf("Related[%d] = %#v, want exact candidate path %q only", i, got.Related[i], path)
			}
		}
		if got.Context != context {
			t.Error("JSON context changed")
		}
		rfErrorsRequireDetails(t, got.Details, []string{"audio", " .mp3 ", directories[0], directories[1]})
		paths[0], paths[1], paths[2] = paths[2], paths[0], paths[1]
	}
}

func TestRFErrorsSelectionDoesNotInventUnknownContext(t *testing.T) {
	for _, context := range []diagnostic.Context{{}, {Path: `C:\Вход\A.json`}, {LessonID: "EfX7", Line: 35}} {
		for _, tc := range []struct {
			code, reason, source string
			paths                []string
		}{
			{CodeNotFound, "Ресурс не найден", "", nil},
			{CodeExtension, "Расширение ресурса не подходит объявленному типу", `..\Медиа\001.wav`, []string{`C:\Медиа\001.wav`}},
			{CodeKind, "Ресурс не является обычным файлом", `C:\Медиа\Каталог`, []string{`C:\Медиа\Каталог`}},
		} {
			t.Run(tc.code+context.Path+context.LessonID, func(t *testing.T) {
				got := selectionError(tc.code, tc.reason, context, "video", tc.source, []string{}, tc.paths)
				if got == nil {
					t.Fatal("selectionError returned nil")
				}
				if got.Code != tc.code || got.Reason != tc.reason || got.Context != context || got.Cause != nil {
					t.Errorf("diagnostic = %#v; known fields must be preserved, unknown fields empty", got)
				}
				if len(got.Related) != len(tc.paths) {
					t.Fatalf("Related = %#v, want %#v", got.Related, tc.paths)
				}
				for i, path := range tc.paths {
					if got.Related[i] != (diagnostic.Context{Path: path}) {
						t.Errorf("candidate context fabricated: %#v", got.Related[i])
					}
				}
				rfErrorsRequireDetails(t, got.Details, []string{"video", tc.source})
			})
		}
	}
}

func TestRFErrorsSelectionOwnsItsResult(t *testing.T) {
	directories := []string{`C:\Ресурсы\ 001 `}
	paths := []string{`C:\Ресурсы\ A.mp3`}
	context := diagnostic.Context{Path: `C:\Вход\A.json`, Line: 35}
	first := selectionError(CodeAmbiguous, "Неоднозначный ресурс", context, "audio", " 001 ", directories, paths)
	second := selectionError(CodeAmbiguous, "Неоднозначный ресурс", context, "audio", " 001 ", directories, paths)
	if first == nil || second == nil || first == second {
		t.Fatal("each selectionError call must return an independent diagnostic")
	}
	if len(first.Related) != 1 || len(second.Related) != 1 || len(first.Details) != 3 || len(second.Details) != 3 {
		t.Fatalf("incomplete diagnostics: first=%#v, second=%#v", first, second)
	}
	directories[0], paths[0] = "changed directory", "changed candidate"
	if first.Related[0] != (diagnostic.Context{Path: `C:\Ресурсы\ A.mp3`}) {
		t.Error("mutating input changes returned candidates")
	}
	rfErrorsRequireDetails(t, first.Details, []string{"audio", " 001 ", `C:\Ресурсы\ 001 `})
	first.Related[0].Path = "changed first"
	first.Details[0] = diagnostic.Detail{Label: "Изменено", Value: "changed first"}
	first.Context.Line = 99
	if second.Context != context || second.Related[0] != (diagnostic.Context{Path: `C:\Ресурсы\ A.mp3`}) {
		t.Error("mutating one result changes another diagnostic")
	}
	rfErrorsRequireDetails(t, second.Details, []string{"audio", " 001 ", `C:\Ресурсы\ 001 `})
	if paths[0] != "changed candidate" || directories[0] != "changed directory" {
		t.Error("mutating result changes caller-owned slices")
	}
}
