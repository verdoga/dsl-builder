package module

import (
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func testNamesFile(t *testing.T, name, id, path string) *File {
	t.Helper()
	value := &lesson.Lesson{
		FormatVersion: lesson.FormatVersion,
		Document: lesson.Document{
			ID: id, Version: "1", Title: "Урок", LineCount: "0",
			CreatedAt: "2026-11-04T09:00:00Z", Pages: []lesson.Page{},
		},
	}
	file, problems := NewFile(name, value, diagnostic.Context{Path: path})
	if len(problems) != 0 || file == nil {
		t.Fatalf("NewFile(%q): file=%v, problems=%v", name, file, problems)
	}
	return file
}

func TestNamesAssignNewModule(t *testing.T) {
	for _, existing := range [][]*File{nil, {}} {
		names, problems := AssignFileNames(existing, 3, diagnostic.Context{Path: "Новая сборка"})
		if len(problems) != 0 || !slices.Equal(names, []string{"l001.json", "l002.json", "l003.json"}) {
			t.Fatalf("AssignFileNames: names=%v, problems=%v", names, problems)
		}
	}
}

func TestNamesAssignContinuesAfterMaximumWithoutMutation(t *testing.T) {
	files := []*File{
		testNamesFile(t, "l010.json", "A:B", "Я/последний.json"),
		testNamesFile(t, "l002.json", "lesson+1", "А/первый.json"),
		testNamesFile(t, "l007.json", "Кириллица", "М/средний.json"),
	}
	before := slices.Clone(files)
	for attempt := 0; attempt < 2; attempt++ {
		names, problems := AssignFileNames(files, 2, diagnostic.Context{Path: "Модуль.zip"})
		if len(problems) != 0 || !slices.Equal(names, []string{"l011.json", "l012.json"}) {
			t.Fatalf("attempt %d: names=%v, problems=%v", attempt, names, problems)
		}
		if !slices.Equal(files, before) {
			t.Fatal("исходный срез файлов переставлен или заменён")
		}
		for index, want := range []string{"l010.json", "l002.json", "l007.json"} {
			if files[index].FileName() != want || files[index].Lesson().Document.Version != "1" {
				t.Fatalf("изменён существующий файл %d", index)
			}
		}
		names[0] = "изменён только результат"
	}
}

func TestNamesAssignCarriesWithoutMachineLimit(t *testing.T) {
	for _, tc := range []struct {
		maximum, next string
	}{
		{"999", "1000"},
		{"9999", "10000"},
		{"18446744073709551615", "18446744073709551616"},
		{"18446744073709551616", "18446744073709551617"},
		{strings.Repeat("9", 180), "1" + strings.Repeat("0", 180)},
	} {
		t.Run(tc.maximum, func(t *testing.T) {
			file := testNamesFile(t, "l"+tc.maximum+".json", "Existing", "архив.zip")
			names, problems := AssignFileNames([]*File{file}, 1, diagnostic.Context{})
			if len(problems) != 0 || !slices.Equal(names, []string{"l" + tc.next + ".json"}) {
				t.Fatalf("names=%v, problems=%v", names, problems)
			}
		})
	}
}

func TestNamesAssignRejectsNonpositiveCountAtomically(t *testing.T) {
	file := testNamesFile(t, "l012.json", "Existing", "архив.zip")
	context := diagnostic.Context{Path: "операция", LessonID: "Known", Line: 4, Column: 2}
	for _, count := range []int{0, -1, -100} {
		names, problems := AssignFileNames([]*File{file}, count, context)
		if names != nil || len(problems) != 1 || problems[0] == nil || problems[0].Code != CodeEmpty {
			t.Fatalf("count=%d: names=%v, problems=%v", count, names, problems)
		}
		if problems[0].Context != context || problems[0].Reason == "" {
			t.Fatalf("count=%d: потерян контекст или причина: %+v", count, problems[0])
		}
		if file.FileName() != "l012.json" {
			t.Fatal("изменено старое имя при отказе")
		}
	}
}

func TestNamesAssignRejectsWholeInvalidSet(t *testing.T) {
	valid := testNamesFile(t, "l010.json", "Existing", "архив.zip")
	for _, files := range [][]*File{
		{valid, nil},
		{valid, {name: "l000.json"}},
		{valid, valid},
	} {
		names, problems := AssignFileNames(files, 2, diagnostic.Context{Path: "архив.zip"})
		if names != nil || len(problems) == 0 {
			t.Fatalf("ошибка должна отклонить весь набор: names=%v, problems=%v", names, problems)
		}
		if valid.FileName() != "l010.json" {
			t.Fatal("изменено старое имя при отказе")
		}
	}
}
