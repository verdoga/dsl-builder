package app

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestCallbacksJoinNilIsNil(t *testing.T) {
	var typedNil *diagnostic.Error
	for _, test := range []struct {
		name   string
		issues []*diagnostic.Error
	}{
		{"nil", nil},
		{"empty", []*diagnostic.Error{}},
		{"nil element", []*diagnostic.Error{nil}},
		{"all typed nil", []*diagnostic.Error{typedNil, nil, typedNil}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := joinDiagnostics(test.issues); got != nil {
				t.Fatalf("all-nil diagnostics returned nonnil %T", got)
			}
		})
	}
}

func TestCallbacksJoinPreservesStructuredOrderAndCauses(t *testing.T) {
	pathCause := &os.PathError{Op: "read", Path: "источник.json", Err: os.ErrPermission}
	nested := &diagnostic.Error{Code: "owner.nested", Reason: "Вложенная причина", Cause: io.ErrUnexpectedEOF}
	first := &diagnostic.Error{
		Code: "owner.first", Reason: "Первая ошибка",
		Context: diagnostic.Context{Path: "модуль.zip", FileName: "l003.json", LessonID: "BA82", PageID: "ba82-p001", Line: 7, Column: 2},
		Related: []diagnostic.Context{{Path: "связанный.json", Line: 9}},
		Details: []diagnostic.Detail{{Label: "Исходный вес", Value: "0010"}},
		Cause:   errors.Join(pathCause, nested),
	}
	second := &diagnostic.Error{Code: "owner.second", Reason: "Вторая ошибка", Cause: io.ErrShortWrite}
	for _, test := range []struct {
		name string
		list []*diagnostic.Error
		want []*diagnostic.Error
	}{
		{"single", []*diagnostic.Error{first}, []*diagnostic.Error{first}},
		{"mixed", []*diagnostic.Error{nil, first, nil, second, nil}, []*diagnostic.Error{first, second}},
		{"duplicates preserved", []*diagnostic.Error{second, first, second}, []*diagnostic.Error{second, first, second}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := joinDiagnostics(test.list)
			children := callbacksFixtureChildren(t, got)
			if len(children) != len(test.want) {
				t.Fatalf("children = %d; want %d", len(children), len(test.want))
			}
			for index, want := range test.want {
				if children[index] != want || !errors.Is(got, want) {
					t.Fatalf("child %d lost identity, order, structure or cause", index)
				}
			}
			var diagnosticError *diagnostic.Error
			if !errors.As(got, &diagnosticError) || diagnosticError != test.want[0] {
				t.Fatal("errors.As lost the first diagnostic")
			}
			var pathError *os.PathError
			if !errors.Is(got, os.ErrPermission) || !errors.Is(got, io.ErrUnexpectedEOF) ||
				!errors.Is(got, nested) || !errors.As(got, &pathError) || pathError != pathCause {
				t.Fatal("nested structured and standard error chains collapsed")
			}
		})
	}
}
