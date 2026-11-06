package diagnostic_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestContextPreservesKnownAndUnknownInformation(t *testing.T) {
	tests := []struct {
		name    string
		context diagnostic.Context
	}{
		{
			name: "неизвестные сведения остаются нулевыми",
		},
		{
			name: "технический JSON и известные координаты",
			context: diagnostic.Context{
				Path:     `C:\Учебные материалы\Unit 01\Урок Ёж 🦔.parsed.json`,
				LessonID: "eFx0007",
				PageID:   "eFx0007.p0001",
				Line:     1,
				Column:   1,
			},
		},
		{
			name: "известная строка без колонки",
			context: diagnostic.Context{
				Path:     `C:\Курс\Источники\A.json`,
				LessonID: "EFX7",
				Line:     35,
			},
		},
		{
			name: "колонка Unicode передана владельцем",
			context: diagnostic.Context{
				Path:   "C:\\Курс\\е\u0308 и ё\\A.json",
				Line:   12,
				Column: 4,
			},
		},
		{
			name: "архив отдельно от технического имени",
			context: diagnostic.Context{
				Path:     `D:\Модули\Мой курс.ZIP`,
				FileName: "l0007.json",
				LessonID: "Ab0007",
				PageID:   "Ab0007.p0000001001",
			},
		},
		{
			name: "техническое имя без вымышленного пути",
			context: diagnostic.Context{
				FileName: "manifest.json",
			},
		},
		{
			name: "сетевой путь ресурса без вымышленных ID",
			context: diagnostic.Context{
				Path: `\\Сервер\Общие материалы\Аудио\001.ЁЖ.MP3`,
			},
		},
		{
			name: "путь не очищается и регистр сохраняется",
			context: diagnostic.Context{
				Path:     `c:\Курс\Раздел\.\..\Медиа\ЗвУк.mp3`,
				LessonID: "aB000000000000000000000000000007",
				PageID:   "aB000000000000000000000000000007.p0002",
			},
		},
		{
			name: "каталог не превращается в имя урока",
			context: diagnostic.Context{
				Path: `C:\Курс\Каталог ресурсов\`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &diagnostic.Error{
				Code:    "parsed.parser",
				Reason:  "Обнаружена ошибка исходных данных",
				Context: tt.context,
			}

			// Обычное использование error не должно дополнять или менять контекст.
			_ = issue.Error()
			_ = issue.Unwrap()
			wrapped := fmt.Errorf("Анализ остановлен: %w", issue)
			var recovered *diagnostic.Error
			if !errors.As(wrapped, &recovered) {
				t.Fatal("errors.As не вернул структурированную диагностику")
			}
			if recovered.Context != tt.context {
				t.Errorf("контекст изменён: получено %#v, ожидалось %#v", recovered.Context, tt.context)
			}
		})
	}
}

func TestContextPreservesDistinctRelatedLocations(t *testing.T) {
	primary := diagnostic.Context{
		Path:     `C:\Исходники\Урок.json`,
		LessonID: "EFx007",
		PageID:   "EFx007.p0001",
		Line:     42,
	}
	first := diagnostic.Context{
		Path:     `D:\Модуль.ZIP`,
		FileName: "l001.json",
		LessonID: "EFx007",
	}
	second := diagnostic.Context{
		Path:     `D:\модуль.zip`,
		FileName: "l002.json",
		LessonID: "efX0007",
		Line:     7,
		Column:   3,
	}
	issue := &diagnostic.Error{
		Code:    "catalog.conflict",
		Reason:  "Обнаружен конфликт уроков",
		Context: primary,
		Related: []diagnostic.Context{first, second, {}},
	}

	_ = issue.Error()
	_ = issue.Unwrap()
	var recovered *diagnostic.Error
	if !errors.As(fmt.Errorf("Проверка модуля: %w", issue), &recovered) {
		t.Fatal("errors.As не вернул структурированную диагностику")
	}
	if recovered.Context != primary {
		t.Errorf("основной контекст изменён: получено %#v, ожидалось %#v", recovered.Context, primary)
	}
	want := []diagnostic.Context{first, second, {}}
	if len(recovered.Related) != len(want) {
		t.Fatalf("число связанных мест = %d, ожидалось %d", len(recovered.Related), len(want))
	}
	for i, context := range want {
		if recovered.Related[i] != context {
			t.Errorf("связанное место %d изменено: получено %#v, ожидалось %#v", i, recovered.Related[i], context)
		}
	}
}
