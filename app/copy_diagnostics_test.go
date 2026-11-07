package app

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func copyDiagnosticsFixture(sentinel error) []*diagnostic.Error {
	pathInner := &diagnostic.Error{Code: "owner.path", Reason: "Причина чтения",
		Context: diagnostic.Context{Path: "ресурс.bin", FileName: "l001.json", LessonID: "COPY", PageID: "copy-p001", Line: 7, Column: 2},
		Related: []diagnostic.Context{{Path: "первый.bin"}, {Path: "второй.bin"}},
		Details: []diagnostic.Detail{{Label: "Вес", Value: "0001"}, {Label: "Вес", Value: ""}}, Cause: os.ErrPermission}
	linkInner := &diagnostic.Error{Code: "owner.link", Reason: "Причина замены",
		Related: []diagnostic.Context{{Path: "исходный.zip"}}, Details: []diagnostic.Detail{{Label: "Файл", Value: "итог.zip"}}, Cause: sentinel}
	third := &diagnostic.Error{Code: "owner.third", Reason: "Ещё одна причина",
		Related: []diagnostic.Context{}, Details: []diagnostic.Detail{}, Cause: syscall.EIO}
	return []*diagnostic.Error{{Code: "owner.outer", Reason: "Операция не завершена",
		Context: diagnostic.Context{Path: "итог.zip", LessonID: "COPY"},
		Related: []diagnostic.Context{{Path: "исходный.zip"}}, Details: []diagnostic.Detail{{Label: "Причина", Value: "не обрезать  "}},
		Cause: errors.Join(
			&os.PathError{Op: "open", Path: "ресурс.bin", Err: &os.SyscallError{Syscall: "read", Err: pathInner}},
			errors.Join(&os.LinkError{Op: "rename", Old: "исходный.zip", New: "итог.zip", Err: linkInner}, third),
		)}, nil, {Code: "owner.other", Reason: "Отдельная ошибка", Related: []diagnostic.Context{}, Details: []diagnostic.Detail{}}}
}

func TestCopyDiagnosticsPreservesStructuredTreeAndErrorQueries(t *testing.T) {
	sentinel := errors.New("immutable technical sentinel")
	source := copyDiagnosticsFixture(sentinel)
	first, second := cloneDiagnostics(source), cloneDiagnostics(source)
	if !reflect.DeepEqual(first, source) || !reflect.DeepEqual(second, source) {
		t.Fatal("потеряны данные, порядок ветвей, nil-элемент или цепочка причин")
	}
	for _, copied := range [][]*diagnostic.Error{first, second} {
		if !errors.Is(copied[0], sentinel) || !errors.Is(copied[0], os.ErrPermission) || !errors.Is(copied[0], syscall.EIO) {
			t.Fatal("errors.Is потерял исходные неизменяемые причины")
		}
		var path *os.PathError
		var call *os.SyscallError
		var link *os.LinkError
		if !errors.As(copied[0], &path) || !errors.As(copied[0], &call) || !errors.As(copied[0], &link) {
			t.Fatal("errors.As потерял стандартные файловые ошибки")
		}
		var inner *diagnostic.Error
		if !errors.As(call.Err, &inner) || inner.Code != "owner.path" || inner.Context.Line != 7 || inner.Context.Column != 2 {
			t.Fatal("структурированная диагностика внутри SyscallError потеряна")
		}
	}
	var firstPath, sourcePath *os.PathError
	var firstCall, sourceCall *os.SyscallError
	var firstLink, sourceLink *os.LinkError
	errors.As(first[0], &firstPath)
	errors.As(source[0], &sourcePath)
	errors.As(first[0], &firstCall)
	errors.As(source[0], &sourceCall)
	errors.As(first[0], &firstLink)
	errors.As(source[0], &sourceLink)
	if firstPath == sourcePath || firstCall == sourceCall || firstLink == sourceLink || first[0] == source[0] {
		t.Fatal("изменяемый wrapper выдан без независимой копии")
	}
	first[0].Context.Path = "правка"
	first[0].Reason = "правка"
	first[0].Related[0].Path = "правка"
	first[0].Details[0].Value = "правка"
	firstPath.Op, firstPath.Path = "правка", "правка"
	firstCall.Syscall = "правка"
	firstInner := firstCall.Err.(*diagnostic.Error)
	firstInner.Related[0].Path = "правка"
	firstInner.Details[0].Value = "правка"
	firstInner.Context.Line = 99
	firstLink.Op, firstLink.Old, firstLink.New = "правка", "правка", "правка"
	firstLink.Err.(*diagnostic.Error).Details[0].Value = "правка"
	joined := first[0].Cause.(interface{ Unwrap() []error }).Unwrap()
	branches := joined[1].(interface{ Unwrap() []error }).Unwrap()
	branches[1].(*diagnostic.Error).Reason = "правка третьей ветви"
	branches[0] = sentinel
	joined[0] = sentinel
	first[2].Code = "правка списка"
	first[1] = &diagnostic.Error{Code: "новая", Reason: "Новая"}
	if !reflect.DeepEqual(source, copyDiagnosticsFixture(sentinel)) || !reflect.DeepEqual(second, source) {
		t.Fatal("копия изменила внутреннюю диагностику, wrapper, joined-список или вторую копию")
	}
	sourceCall.Err.(*diagnostic.Error).Details[1].Value = "правка источника"
	sourceLink.Old = "правка источника"
	source[0].Related[0].Path = "правка источника"
	source[0].Cause.(interface{ Unwrap() []error }).Unwrap()[0] = sentinel
	if !reflect.DeepEqual(second, copyDiagnosticsFixture(sentinel)) {
		t.Fatal("источник изменил ранее выданный снимок цепочки")
	}
}

func TestCopyDiagnosticsDirectRecursionAndNilEmptyLists(t *testing.T) {
	if cloneDiagnostics(nil) != nil {
		t.Fatal("nil diagnostic list")
	}
	if got := cloneDiagnostics([]*diagnostic.Error{}); got == nil || len(got) != 0 {
		t.Fatal("empty diagnostic list collapsed")
	}
	for _, empty := range []bool{false, true} {
		var related []diagnostic.Context
		var details []diagnostic.Detail
		if empty {
			related, details = []diagnostic.Context{}, []diagnostic.Detail{}
		}
		inner := &diagnostic.Error{Code: "owner.inner", Reason: "Внутренняя",
			Related: related, Details: details}
		source := []*diagnostic.Error{{Code: "owner.root", Reason: "Внешняя", Related: related,
			Details: details, Cause: &diagnostic.Error{Code: "owner.middle", Reason: "Средняя", Cause: inner}}, nil}
		got := cloneDiagnostics(source)
		if !reflect.DeepEqual(got, source) {
			t.Fatal("потеряна прямая рекурсия или форма nil/empty")
		}
		copiedInner := got[0].Cause.(*diagnostic.Error).Cause.(*diagnostic.Error)
		copiedInner.Code = "изменено"
		if inner.Code != "owner.inner" {
			t.Fatal("рекурсивная причина разделяет память")
		}
		inner.Reason = "изменено у источника"
		if copiedInner.Reason != "Внутренняя" {
			t.Fatal("копия зависит от источника на глубине два")
		}
	}
}
