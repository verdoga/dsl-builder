package diagnostic_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestDetailPreservesExactLabelAndValue(t *testing.T) {
	tests := []struct {
		name  string
		label string
		value string
	}{
		{
			name:  "unicode",
			label: "Сообщение парсера — 日本語 🧩",
			value: "Ошибка: ё и е\u0308, café и cafe\u0301, 中文 🧩",
		},
		{
			name:  "whitespace",
			label: " \tПодстрока поиска\u00a0 ",
			value: " \t0012.01 начало\u00a0 конец\r\n ",
		},
		{
			name:  "empty_value",
			label: "Исходное сообщение",
			value: "",
		},
		{
			name:  "weight_beyond_machine_integer",
			label: "Исходный вес",
			value: "000000000000000000000000000000000000001844674407370955161612345678901234567890",
		},
		{
			name:  "original_parser_message",
			label: "Сообщение парсера",
			value: "  Unexpected token '@include': ожидалось «имя»\nsource: E:\\Уроки с пробелами\\001.json\t",
		},
		{
			name:  "windows_search_directory",
			label: "Каталог поиска",
			value: `C:\Учебные материалы\Юнит 01\..\Аудио\`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &diagnostic.Error{
				Code:   "parsed.parser",
				Reason: "Обнаружена ошибка исходных данных",
				Details: []diagnostic.Detail{
					{Label: tt.label, Value: tt.value},
				},
			}
			wrapped := fmt.Errorf("проверка входа: %w", issue)
			_ = issue.Error()
			_ = issue.Unwrap()

			var got *diagnostic.Error
			if !errors.As(wrapped, &got) {
				t.Fatal("подробность недоступна через структурированную ошибку")
			}
			if len(got.Details) != 1 {
				t.Fatalf("число подробностей = %d, требуется 1", len(got.Details))
			}
			if got.Details[0].Label != tt.label {
				t.Errorf("Label = %q, требуется %q", got.Details[0].Label, tt.label)
			}
			if got.Details[0].Value != tt.value {
				t.Errorf("Value = %q, требуется %q", got.Details[0].Value, tt.value)
			}
		})
	}
}

func TestDetailsPreserveRepeatedLabelsAndOrder(t *testing.T) {
	want := []diagnostic.Detail{
		{Label: "Каталог поиска", Value: `C:\Уроки\Аудио 2`},
		{Label: "Каталог поиска", Value: `C:\Уроки\Аудио 1`},
		{Label: "Сообщение парсера", Value: "Первое исходное сообщение"},
		{Label: "Сообщение парсера", Value: ""},
		{Label: "Сообщение парсера", Value: "Первое исходное сообщение"},
		{Label: "Каталог поиска", Value: `C:\Уроки\Аудио 2`},
	}
	issue := &diagnostic.Error{
		Code:    "parsed.parser",
		Reason:  "Обнаружены ошибки исходных данных",
		Details: append([]diagnostic.Detail(nil), want...),
	}
	_ = issue.Error()
	_ = issue.Unwrap()

	if len(issue.Details) != len(want) {
		t.Fatalf("число подробностей = %d, требуется %d", len(issue.Details), len(want))
	}
	for i, expected := range want {
		if issue.Details[i] != expected {
			t.Errorf("Details[%d] = %#v, требуется %#v", i, issue.Details[i], expected)
		}
	}
}
