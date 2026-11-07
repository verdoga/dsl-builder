package parsed

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestReadBytesPreservesCompleteFile(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"unicode", []byte(" \t{\r\n\"текст\":\"Ёж 🦔 e\u0301\"}\n ")},
		{"bom_and_invalid_utf8", []byte{0xef, 0xbb, 0xbf, 0, 0xff, 0x80, '\r', '\n'}},
		{"large", bytes.Repeat([]byte("Строка 🦔\x00\r\n"), 20000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Технический JSON 🦔.json")
			if err := os.WriteFile(path, tt.data, 0600); err != nil {
				t.Fatal(err)
			}
			got, problems := readBytes(path)
			if len(problems) != 0 {
				t.Fatalf("readBytes: %v", problems)
			}
			if !bytes.Equal(got, tt.data) {
				t.Fatalf("прочитанные байты изменены: длина %d, ожидалась %d", len(got), len(tt.data))
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, tt.data) {
				t.Fatal("исходный файл изменён")
			}
		})
	}
}

func TestReadBytesReportsFilesystemCauseAndExactPath(t *testing.T) {
	dir := t.TempDir()
	// Встроенный /./ проверяет сохранение адреса обращения в диагностике.
	missing := dir + string(os.PathSeparator) + "." + string(os.PathSeparator) + "Нет файла 🦔.json"
	tests := []struct {
		name    string
		path    string
		missing bool
	}{
		{"missing_file", missing, true},
		{"missing_parent", filepath.Join(dir, "нет каталога", "вход.json"), true},
		{"directory", dir, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, problems := readBytes(tt.path)
			if len(problems) == 0 {
				t.Fatal("ошибка обязательного файла не возвращена")
			}
			for _, problem := range problems {
				readTestDiagnostic(t, problem, CodeRead, diagnostic.Context{Path: tt.path})
				if problem.Cause == nil {
					t.Fatal("исходная Cause потеряна")
				}
				var pathError *os.PathError
				if !errors.As(problem.Cause, &pathError) {
					t.Fatalf("Cause имеет тип %T, ожидалась исходная файловая ошибка", problem.Cause)
				}
				if pathError.Path != tt.path {
					t.Errorf("Cause.Path = %q, ожидался %q", pathError.Path, tt.path)
				}
				if !errors.Is(problem, problem.Cause) {
					t.Error("Cause недоступна через цепочку error")
				}
				if tt.missing && !errors.Is(problem, fs.ErrNotExist) {
					t.Errorf("ошибка отсутствия файла потеряна: %v", problem)
				}
			}
		})
	}
}

func TestPrepareUTF8PreservesBytesAndRemovesOnlyInitialBOM(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []byte
	}{
		{"nil", nil, nil},
		{"empty", []byte{}, []byte{}},
		{"unicode", []byte("Ёж 🦔 e\u0301 \ufffd"), []byte("Ёж 🦔 e\u0301 \ufffd")},
		{"whitespace", []byte(" \t\r\n{\n\t\"raw\":\"  текст  \"\r\n}\n "), []byte(" \t\r\n{\n\t\"raw\":\"  текст  \"\r\n}\n ")},
		{"bom_only", []byte("\ufeff"), []byte{}},
		{"initial_bom", []byte("\ufeff \t{}\r\n"), []byte(" \t{}\r\n")},
		{"two_boms", []byte("\ufeff\ufeff{}"), []byte("\ufeff{}")},
		{"three_boms", []byte("\ufeff\ufeff\ufeff{}"), []byte("\ufeff\ufeff{}")},
		{"bom_after_space", []byte(" \ufeff{}"), []byte(" \ufeff{}")},
		{"internal_bom", []byte("{\"raw\":\"а\ufeffб\"}\ufeff"), []byte("{\"raw\":\"а\ufeffб\"}\ufeff")},
		{"initial_and_internal_bom", []byte("\ufeff{\"value\":\"\ufeff\"}"), []byte("{\"value\":\"\ufeff\"}")},
		{"valid_non_json", []byte("\x00\u007f\u0080\u07ff\u0800\ud7ff\ue000\U00010000\U0010ffff"), []byte("\x00\u007f\u0080\u07ff\u0800\ud7ff\ue000\U00010000\U0010ffff")},
		{"escaped_bom", []byte(`{"raw":"\uFEFF"}`), []byte(`{"raw":"\uFEFF"}`)},
	}
	context := diagnostic.Context{Path: "Каталог/вход 🦔.json"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := bytes.Clone(tt.data)
			got, problem := prepareUTF8(tt.data, context)
			if problem != nil {
				t.Fatalf("корректный UTF-8 отклонён: %v", problem)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("получены байты %x, ожидались %x", got, tt.want)
			}
			if !bytes.Equal(tt.data, before) {
				t.Error("байты переданного входа изменены")
			}
		})
	}
}

func TestPrepareUTF8RejectsInvalidBytesThroughoutInput(t *testing.T) {
	invalid := []struct {
		name string
		data []byte
	}{
		{"continuation_low", []byte{0x80}},
		{"continuation_high", []byte{0xbf}},
		{"overlong_two", []byte{0xc0, 0x80}},
		{"illegal_two_leader", []byte{0xc1, 0xbf}},
		{"truncated_two", []byte{0xc2}},
		{"wrong_continuation", []byte{0xc2, 'x'}},
		{"overlong_three", []byte{0xe0, 0x80, 0x80}},
		{"surrogate", []byte{0xed, 0xa0, 0x80}},
		{"truncated_three", []byte{0xe2, 0x82}},
		{"overlong_four", []byte{0xf0, 0x80, 0x80, 0x80}},
		{"above_unicode_max", []byte{0xf4, 0x90, 0x80, 0x80}},
		{"illegal_four_leader", []byte{0xf5, 0x80, 0x80, 0x80}},
		{"truncated_four", []byte{0xf0, 0x9f, 0xa6}},
		{"illegal_fe", []byte{0xfe}},
		{"illegal_ff", []byte{0xff}},
		{"partial_bom_one", []byte{0xef}},
		{"partial_bom_two", []byte{0xef, 0xbb}},
	}
	positions := []struct {
		name   string
		prefix string
		suffix string
	}{
		{"start", "", `{"value":"текст"}`},
		{"inside_string", `{"value":"до`, `после"}`},
		{"after_json", `{"value":"текст"}`, ""},
		{"after_initial_bom", "\ufeff", `{}`},
		{"after_bom_and_json", "\ufeff{}\n", ""},
	}
	contexts := []diagnostic.Context{
		{},
		{Path: "Путь/вход 🦔.json", FileName: "l001.json", LessonID: "Урок+A", PageID: "страница", Line: 17, Column: 4},
	}
	for _, bad := range invalid {
		for _, position := range positions {
			t.Run(bad.name+"/"+position.name, func(t *testing.T) {
				data := append([]byte(position.prefix), bad.data...)
				data = append(data, position.suffix...)
				before := bytes.Clone(data)
				for _, context := range contexts {
					_, problem := prepareUTF8(data, context)
					readTestDiagnostic(t, problem, CodeUTF8, context)
					if !bytes.Equal(data, before) {
						t.Error("некорректные байты исправлены или удалены во входе")
					}
				}
			})
		}
	}
}

// readTestDiagnostic проверяет только оговорённые свойства диагностики.
func readTestDiagnostic(t *testing.T, problem *diagnostic.Error, code string, context diagnostic.Context) {
	t.Helper()
	if problem == nil {
		t.Fatal("ожидалась блокирующая диагностика")
	}
	if problem.Code != code {
		t.Errorf("Code = %q, ожидался %q", problem.Code, code)
	}
	if problem.Context != context {
		t.Errorf("Context = %+v, ожидался %+v", problem.Context, context)
	}
	if !utf8.ValidString(problem.Reason) || strings.IndexFunc(problem.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) < 0 {
		t.Errorf("ожидалась понятная русская Reason, получено %q", problem.Reason)
	}
}
