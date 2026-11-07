package resourcefind

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

func TestRFErrorsContextualizePreservesAndAppendsJSON(t *testing.T) {
	cause := &os.PathError{Op: "open", Path: `C:\Ресурсы\нет.mp3`, Err: fs.ErrNotExist}
	relatedBacking := []diagnostic.Context{{Path: `C:\Старое\один`}, {Path: "sentinel"}}
	detailBacking := []diagnostic.Detail{{Label: "Старая подробность", Value: " 001 "}, {Label: "Хвост", Value: "sentinel"}}
	original := &diagnostic.Error{
		Code: filetree.CodeAccess, Reason: "Не удалось открыть ресурс",
		Context: diagnostic.Context{Path: cause.Path, FileName: "known.bin", LessonID: "OLD", PageID: "OLD-p001", Line: 2, Column: 3},
		Related: relatedBacking[:1], Details: detailBacking[:1], Cause: cause,
	}
	wantOriginal := *original
	wantOriginal.Related = slices.Clone(original.Related)
	wantOriginal.Details = slices.Clone(original.Details)
	jsonContext := diagnostic.Context{Path: `D:\Новый JSON\A.json`, FileName: "l003.json", LessonID: "EfX7", PageID: "efx7-p003", Line: 35, Column: 14}
	got := contextualize(original, jsonContext)
	if got == original {
		t.Fatal("contextualize returned the original pointer")
	}
	want := wantOriginal
	want.Related = append(slices.Clone(wantOriginal.Related), jsonContext)
	rfErrorsRequirePreserved(t, got, &want)
	rfErrorsRequirePreserved(t, original, &wantOriginal)
	if relatedBacking[1].Path != "sentinel" || detailBacking[1].Value != "sentinel" {
		t.Error("contextualize changed storage beyond original slice lengths")
	}
	if !errors.Is(got, fs.ErrNotExist) || !errors.Is(got, cause) {
		t.Error("contextualize broke errors.Is through the original Cause")
	}
	var technical *os.PathError
	if !errors.As(got, &technical) || technical != cause {
		t.Errorf("errors.As PathError = %#v, want original Cause", technical)
	}
	var diagnosticCopy *diagnostic.Error
	if !errors.As(got, &diagnosticCopy) || diagnosticCopy != got {
		t.Error("result is not available through errors.As")
	}
	got.Code, got.Reason, got.Context.Path, got.Cause = "changed", "Изменено", "changed", nil
	got.Related[0].Path = "changed"
	got.Related[1].LessonID = "changed"
	got.Details[0].Value = "changed"
	rfErrorsRequirePreserved(t, original, &wantOriginal)
	if jsonContext.LessonID != "EfX7" {
		t.Error("result aliases the caller's context")
	}
}

func TestRFErrorsContextualizeCopiesBothDirectionsAndBetweenCalls(t *testing.T) {
	original := &diagnostic.Error{
		Code: filetree.CodeLink, Reason: "Не удалось разрешить ссылку",
		Context: diagnostic.Context{Path: `C:\Каталог\ссылка`},
		Related: []diagnostic.Context{{Path: `C:\Каталог\цель`}},
		Details: []diagnostic.Detail{{Label: "Обращение", Value: "исходное"}},
	}
	firstContext := diagnostic.Context{Path: `C:\Вход\A.json`, LessonID: "A", Line: 35}
	secondContext := diagnostic.Context{Path: `C:\Вход\B.json`, PageID: "b-p001"}
	first := contextualize(original, firstContext)
	second := contextualize(original, secondContext)
	if first == nil || second == nil || first == second {
		t.Fatal("each call must return an independent diagnostic")
	}
	original.Context.Path = "changed original"
	original.Related[0].Path = "changed original"
	original.Details[0].Label = "Изменено"
	if first.Context.Path != `C:\Каталог\ссылка` || first.Related[0].Path != `C:\Каталог\цель` || first.Details[0].Label != "Обращение" {
		t.Error("mutating original changes an existing copy")
	}
	if len(first.Related) != 2 || len(second.Related) != 2 {
		t.Fatalf("JSON contexts missing: first=%#v, second=%#v", first.Related, second.Related)
	}
	first.Related[0].Path, first.Related[1].Path = "changed first", "changed first JSON"
	first.Details[0].Value = "changed first"
	if second.Related[0].Path != `C:\Каталог\цель` || second.Related[1] != secondContext || second.Details[0].Value != "исходное" {
		t.Error("independent contextualize results share mutable slices")
	}
}

func TestRFErrorsContextualizePreservesPartialKnownContext(t *testing.T) {
	for _, context := range []diagnostic.Context{
		{Path: `C:\JSON\A.json`},
		{LessonID: "001A", Line: 35},
		{Path: `C:\JSON\B.json`, PageID: "001a-p001", Column: 9},
	} {
		t.Run(context.Path+context.LessonID, func(t *testing.T) {
			original := &diagnostic.Error{Code: filetree.CodePath, Reason: "Путь не задан", Context: diagnostic.Context{Path: "  "}}
			got := contextualize(original, context)
			want := *original
			want.Related = []diagnostic.Context{context}
			rfErrorsRequirePreserved(t, got, &want)
			if len(original.Related) != 0 || len(original.Details) != 0 {
				t.Error("original empty slices changed")
			}
		})
	}
}

func TestRFErrorsContextualizeRealFiletreeError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "несуществующий ресурс.mp3")
	var resolver filetree.Resolver
	_, original := resolver.Inspect(path)
	if original == nil {
		t.Fatal("missing file should produce a real filetree error")
	}
	want := *original
	want.Related = slices.Clone(original.Related)
	want.Details = slices.Clone(original.Details)
	context := diagnostic.Context{Path: filepath.Join(filepath.Dir(path), "A.json"), LessonID: "EFX7", Line: 35}
	got := contextualize(original, context)
	rfErrorsRequirePreserved(t, original, &want)
	want.Related = append(want.Related, context)
	rfErrorsRequirePreserved(t, got, &want)
	if got.Context.Path != path || got.Code != filetree.CodeAccess || !errors.Is(got, fs.ErrNotExist) {
		t.Errorf("real filesystem failure lost its path, code or chain: %#v", got)
	}
	var technical *os.PathError
	if !errors.As(got, &technical) {
		t.Error("real filetree technical error no longer available through errors.As")
	}
}
