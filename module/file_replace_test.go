package module

import (
	"bytes"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

func modFilePrevious(t *testing.T) (*File, []byte) {
	t.Helper()
	raw := modFileJSON(t, modFileRich(t))
	input := modFileDecode(t, raw, diagnostic.Context{})
	file, problems := LoadedFile("l042.json", input, raw, diagnostic.Context{Path: "старый модуль.zip"})
	if file == nil || len(problems) != 0 {
		t.Fatalf("предыдущий файл: %v", problems)
	}
	return file, raw
}

func TestModFileReplacePreservesOldSnapshotAndReadyRevision(t *testing.T) {
	previous, raw := modFilePrevious(t)
	oldModel := modFileJSON(t, previous.Lesson())
	oldContext := previous.Context()
	candidate := previous.Lesson()
	candidate.Document.Version = "123456789012345678901234567890e2"
	candidate.Document.CreatedAt = "2001-02-03T04:05:06Z"
	candidate.Document.LineCount = "9.0"
	candidate.Document.Title = "Принятое название"
	*candidate.Document.Subtitle = "Принятый подзаголовок"
	candidate.Document.Pages[0].ID = "efx7-p2001"
	candidate.Document.Pages[0].Nodes[0].ID = "efx7-n9999-v07"
	before := modFileJSON(t, candidate)
	context := diagnostic.Context{Path: "новый технический.json", Line: 11}
	replaced, problems := ReplaceFile(previous, candidate, context)
	if replaced == nil || replaced == previous || len(problems) != 0 {
		t.Fatalf("независимая замена: file=%v errors=%v", replaced, problems)
	}
	if replaced.FileName() != "l042.json" || replaced.OriginalBytes() != nil {
		t.Fatalf("замена должна сохранить имя и сбросить raw: %q / %q", replaced.FileName(), replaced.OriginalBytes())
	}
	modFileRequireModel(t, candidate, before)
	modFileRequireModel(t, replaced.Lesson(), before)
	modFileMutateRich(t, candidate)
	modFileRequireModel(t, replaced.Lesson(), before)
	returned := replaced.Lesson()
	modFileMutateRich(t, returned)
	modFileRequireModel(t, replaced.Lesson(), before)
	modFileRequireModel(t, previous.Lesson(), oldModel)
	if !bytes.Equal(previous.OriginalBytes(), raw) || previous.Context() != oldContext || previous.FileName() != "l042.json" {
		t.Fatal("замена изменила старый снимок")
	}
	if replaced.Context().Path != context.Path || replaced.Context().Line != context.Line {
		t.Fatalf("потерян известный источник замены: %+v", replaced.Context())
	}
}

func TestModFileReplaceRequiresExactStableID(t *testing.T) {
	for _, id := range []string{"efx7", "Efx7", "OTHER"} {
		t.Run(id, func(t *testing.T) {
			previous, raw := modFilePrevious(t)
			before := modFileJSON(t, previous.Lesson())
			candidate := previous.Lesson()
			candidate.Document.ID = id
			file, problems := ReplaceFile(previous, candidate, diagnostic.Context{Path: "замена.json"})
			modFileRequireCode(t, file, problems, CodeIdentityChange)
			modFileRequireModel(t, previous.Lesson(), before)
			if !bytes.Equal(previous.OriginalBytes(), raw) {
				t.Fatal("отклонённая замена испортила байты")
			}
		})
	}
}

func TestModFileReplaceRejectsMissingSnapshots(t *testing.T) {
	valid, _ := modFilePrevious(t)
	for _, tc := range []struct {
		name      string
		previous  *File
		candidate *lesson.Lesson
	}{
		{"nil-previous", nil, modFileFixture(t)},
		{"zero-previous", &File{}, modFileFixture(t)},
		{"missing-internal-value", &File{name: "l001.json"}, modFileFixture(t)},
		{"nil-candidate", valid, nil},
		{"both-nil", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, problems := ReplaceFile(tc.previous, tc.candidate, diagnostic.Context{})
			modFileRequireCode(t, file, problems, CodeFile)
		})
	}
}

func TestModFileReplacePreservesDependencyErrorsAndOldFile(t *testing.T) {
	previous, raw := modFilePrevious(t)
	before := modFileJSON(t, previous.Lesson())
	candidate := previous.Lesson()
	candidate.Document.Version = "0"
	candidate.Document.CreatedAt = "not a date"
	context := diagnostic.Context{Path: "ошибочная замена.json", FileName: previous.FileName()}
	expected := lesson.Validate(candidate, context)
	file, problems := ReplaceFile(previous, candidate, context)
	if file != nil || len(problems) == 0 {
		t.Fatalf("ошибка допускает частичный результат: %v / %v", file, problems)
	}
	modFileRequireDependencies(t, problems, expected)
	modFileRequireModel(t, previous.Lesson(), before)
	if !bytes.Equal(previous.OriginalBytes(), raw) {
		t.Fatal("ошибка замены изменила прежние raw")
	}
	brokenName := &File{name: "L001.json", value: modFileFixture(t)}
	file, problems = ReplaceFile(brokenName, modFileFixture(t), diagnostic.Context{})
	modFileRequireCode(t, file, problems, identity.CodeInvalidFileName)
}

func TestModFileLoadedInvalidNameDoesNotAttachBytes(t *testing.T) {
	raw := modFileJSON(t, modFileFixture(t))
	input := modFileDecode(t, raw, diagnostic.Context{})
	before := string(raw)
	file, problems := LoadedFile("dir/l001.json", input, raw, diagnostic.Context{})
	modFileRequireCode(t, file, problems, identity.CodeInvalidFileName)
	if string(raw) != before {
		t.Fatal("изменён исходный JSON")
	}
}
