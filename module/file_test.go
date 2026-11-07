package module

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

func modFileFixture(t *testing.T) *lesson.Lesson {
	t.Helper()
	return &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "EFX7", Version: "900719925474099312345678901234567890.0",
		Title: "Урок <&>\n", LineCount: "1e30", CreatedAt: "2026-11-04T12:15:00Z",
		Pages: []lesson.Page{},
	}}
}

func modFileJSON(t *testing.T, value *lesson.Lesson) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func modFileRequireModel(t *testing.T, got *lesson.Lesson, want []byte) {
	t.Helper()
	if got == nil || !bytes.Equal(modFileJSON(t, got), want) {
		t.Fatalf("модель изменилась: got %s, want %s", modFileJSON(t, got), want)
	}
}

func modFileRequireCode(t *testing.T, got *File, problems []*diagnostic.Error, code string) {
	t.Helper()
	if got != nil || len(problems) == 0 {
		t.Fatalf("ошибка должна отвергать весь файл: file=%v, errors=%v", got, problems)
	}
	for _, problem := range problems {
		if problem != nil && problem.Code == code && problem.Reason != "" {
			return
		}
	}
	t.Fatalf("нет ошибки %q с причиной: %v", code, problems)
}

func modFileRequireDependencies(t *testing.T, problems, expected []*diagnostic.Error) {
	t.Helper()
	if len(expected) == 0 {
		t.Fatal("сценарий должен быть отклонён реальной зависимостью")
	}
	for _, want := range expected {
		found := false
		for _, got := range problems {
			if got.Code != want.Code || got.Reason != want.Reason {
				continue
			}
			found = true
			for _, detail := range want.Details {
				retained := false
				for _, actual := range got.Details {
					retained = retained || actual == detail
				}
				if !retained {
					t.Errorf("потеряна подробность зависимости %+v", detail)
				}
			}
		}
		if !found {
			t.Errorf("потерян код/причина зависимости %s: %s", want.Code, want.Reason)
		}
	}
}

func TestModFileNewNamesAndUnchangedMetadata(t *testing.T) {
	for _, name := range []string{"l001.json", "l999.json", "l1000.json", "l184467440737095516160000.json"} {
		t.Run(name, func(t *testing.T) {
			input := modFileFixture(t)
			before := modFileJSON(t, input)
			file, problems := NewFile(name, input, diagnostic.Context{Path: "несуществующий источник.json"})
			if len(problems) != 0 || file == nil {
				t.Fatalf("валидный файл отвергнут: %v", problems)
			}
			if file.FileName() != name || file.OriginalBytes() != nil {
				t.Fatalf("имя/raw нового файла: %q / %q", file.FileName(), file.OriginalBytes())
			}
			modFileRequireModel(t, input, before)
			modFileRequireModel(t, file.Lesson(), before)
			if file.Lesson().Document.Pages == nil {
				t.Fatal("пустой ненулевой массив страниц потерян")
			}
		})
	}
}

func TestModFileNewRejectsNamesWithIdentityDiagnostics(t *testing.T) {
	for _, name := range []string{"", "manifest.json", "l000.json", "l01.json", "l0001.json", "L001.json", "l001.JSON", "dir/l001.json", `dir\l001.json`, "l001.json\n"} {
		t.Run(name, func(t *testing.T) {
			_, expected := identity.ParseFileName(name)
			if expected == nil {
				t.Fatal("ошибочный fixture имени")
			}
			file, problems := NewFile(name, modFileFixture(t), diagnostic.Context{Path: "технический.json"})
			modFileRequireCode(t, file, problems, identity.CodeInvalidFileName)
			modFileRequireDependencies(t, problems, []*diagnostic.Error{expected})
		})
	}
}

func TestModFileNewPreservesLessonValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		edit func(*lesson.Lesson) *lesson.Lesson
	}{
		{"nil", func(*lesson.Lesson) *lesson.Lesson { return nil }},
		{"version", func(v *lesson.Lesson) *lesson.Lesson { v.Document.Version = "0"; return v }},
		{"format", func(v *lesson.Lesson) *lesson.Lesson { v.FormatVersion = "2.0"; return v }},
		{"ID", func(v *lesson.Lesson) *lesson.Lesson { v.Document.ID = "bad id"; return v }},
		{"date", func(v *lesson.Lesson) *lesson.Lesson { v.Document.CreatedAt = "2026-02-30T12:00:00Z"; return v }},
		{"pages", func(v *lesson.Lesson) *lesson.Lesson { v.Document.Pages = nil; return v }},
		{"lineCount", func(v *lesson.Lesson) *lesson.Lesson { v.Document.LineCount = "-1"; return v }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.edit(modFileFixture(t))
			context := diagnostic.Context{Path: "входной.json", FileName: "l007.json"}
			expected := lesson.Validate(input, context)
			file, problems := NewFile("l007.json", input, context)
			if file != nil || len(problems) == 0 {
				t.Fatalf("частичный успех: file=%v, errors=%v", file, problems)
			}
			modFileRequireDependencies(t, problems, expected)
			for _, problem := range problems {
				if problem.Context.Path != context.Path {
					t.Errorf("потерян путь источника: %+v", problem.Context)
				}
			}
		})
	}
}
