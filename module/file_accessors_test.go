package module

import (
	"bytes"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestModFileNilAccessors(t *testing.T) {
	var file *File
	if file.FileName() != "" || file.Lesson() != nil || file.OriginalBytes() != nil || file.Context() != (diagnostic.Context{}) {
		t.Fatal("nil-получатель должен возвращать нулевые значения")
	}
	zero := &File{}
	if zero.FileName() != "" || zero.Lesson() != nil || zero.OriginalBytes() != nil || zero.Context() != (diagnostic.Context{}) {
		t.Fatal("нулевой снимок должен возвращать нулевые значения")
	}
}

func TestModFileContextContainsOnlyKnownSourceAndSnapshotIdentity(t *testing.T) {
	for _, source := range []diagnostic.Context{
		{},
		{Path: `C:\Материалы\несуществующий.json`},
		{Path: "проверенный архив.zip", FileName: "старое имя", LessonID: "старый ID", PageID: "известная страница", Line: 17, Column: 4},
	} {
		file, problems := NewFile("l003.json", modFileFixture(t), source)
		if file == nil || len(problems) != 0 {
			t.Fatalf("NewFile: %v", problems)
		}
		expected := source
		expected.FileName, expected.LessonID = "l003.json", "EFX7"
		if got := file.Context(); got != expected {
			t.Fatalf("Context=%+v, want=%+v", got, expected)
		}
		copyContext := file.Context()
		copyContext.Path, copyContext.FileName, copyContext.LessonID = "changed", "changed", "changed"
		if file.Context() != expected {
			t.Fatal("возвращённый контекст изменил снимок")
		}
	}
}

func TestModFileAttachOriginalCreatesIndependentSnapshot(t *testing.T) {
	raw := modFileJSON(t, modFileRich(t))
	input := modFileDecode(t, raw, diagnostic.Context{})
	plain, problems := NewFile("l081.json", input, diagnostic.Context{Path: "источник.zip"})
	if plain == nil || len(problems) != 0 {
		t.Fatalf("NewFile: %v", problems)
	}
	before := modFileJSON(t, plain.Lesson())
	context := plain.Context()
	attached, problem := attachOriginal(plain, raw)
	if problem != nil || attached == nil || attached == plain {
		t.Fatalf("attachOriginal: file=%v error=%v", attached, problem)
	}
	if plain.OriginalBytes() != nil || attached.FileName() != plain.FileName() || attached.Context() != context {
		t.Fatal("attachOriginal изменил старый снимок/имя/контекст")
	}
	modFileRequireModel(t, attached.Lesson(), before)
	want := append([]byte(nil), raw...)
	raw[0] = '!'
	if !bytes.Equal(attached.OriginalBytes(), want) {
		t.Fatal("attachOriginal сохранил внешний буфер")
	}
	returned := attached.OriginalBytes()
	returned[0] = '!'
	if !bytes.Equal(attached.OriginalBytes(), want) {
		t.Fatal("attachOriginal предоставляет внутренний буфер")
	}
	secondRaw := append(append([]byte(nil), want...), '\n')
	second, problem := attachOriginal(attached, secondRaw)
	if problem != nil || second == nil || second == attached {
		t.Fatalf("повторное прикрепление: %v", problem)
	}
	if !bytes.Equal(attached.OriginalBytes(), want) || !bytes.Equal(second.OriginalBytes(), secondRaw) {
		t.Fatal("прикрепление заменило байты предыдущего снимка")
	}
	for _, empty := range [][]byte{nil, {}} {
		failed, problem := attachOriginal(attached, empty)
		if failed != nil || problem == nil || problem.Code != CodeOriginalBytes || problem.Reason == "" {
			t.Fatalf("пустые байты: file=%v error=%v", failed, problem)
		}
		if !bytes.Equal(attached.OriginalBytes(), want) {
			t.Fatal("неуспешное прикрепление испортило снимок")
		}
	}
}
