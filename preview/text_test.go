package preview

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestQuotedTextPreservesFullPrintableValue(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"case", "AbC абВ Ёё"},
		{"spaces", "  начало  середина   конец  "},
		{"unicode", "Кириллица 日本語 Ελληνικά 👩🏽‍💻 e\u0301 é"},
		{"quotes", `Он сказал: "Да", «текст», “ещё”, 'слово'`},
		{"windows path", `C:\Учебные материалы\Unit 1\A.json`},
		{"literal escapes", `\n \r \t \x1b \u001b`},
		{"long value", "Начало " + strings.Repeat("Жя🙂  Az-", 2048) + " Конец"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quotedText(tc.value)
			payload := textTestAssertQuoted(t, got)
			if decoded, ok := textTestDecode(got); ok {
				if decoded != tc.value {
					t.Fatalf("значение после снятия экранирования изменено: got %q, want %q", decoded, tc.value)
				}
			} else if payload != tc.value {
				t.Fatalf("полное печатное значение не сохранено: got %q, want %q", payload, tc.value)
			}
		})
	}
}

func TestQuotedTextMakesControlsVisibleAndSafe(t *testing.T) {
	controls := []rune{'\n', '\r', '\t', '\b', '\f', '\v', 0, 0x1b, 0x7f, 0x85, 0x9b}
	for r := rune(1); r < 0x20; r++ {
		if !strings.ContainsRune("\n\r\t\b\f\v\x1b", r) {
			controls = append(controls, r)
		}
	}
	seen := make(map[string]rune)
	for _, control := range controls {
		t.Run(fmt.Sprintf("U+%04X", control), func(t *testing.T) {
			value := "До " + string(control) + " После"
			got := quotedText(value)
			textTestAssertQuoted(t, got)
			if decoded, ok := textTestDecode(got); ok {
				if decoded != value {
					t.Fatalf("управляющий символ потерян или заменён: got %q, want %q", decoded, value)
				}
			} else if !strings.Contains(got, "До ") || !strings.Contains(got, " После") {
				t.Fatalf("окружение управляющего символа потеряно: %q", got)
			}
			if got == quotedText("До  После") {
				t.Fatalf("управляющий символ U+%04X исчез из представления", control)
			}
			if earlier, ok := seen[got]; ok {
				t.Fatalf("U+%04X и U+%04X имеют одинаковое представление %q", earlier, control, got)
			}
			seen[got] = control
		})
	}
}

func TestQuotedTextCannotInjectLinesOrTerminalCommands(t *testing.T) {
	value := "заголовок\r\nПоддельный отчёт\x1b[2J\x1b[H\x1b]0;подмена\a\u009b31m\tконец"
	got := quotedText(value)
	textTestAssertQuoted(t, got)
	if decoded, ok := textTestDecode(got); ok && decoded != value {
		t.Fatalf("значение с переносами и терминальными кодами изменено: got %q, want %q", decoded, value)
	}
	if got == quotedText("заголовокПоддельный отчёт[2J[H]0;подмена31mконец") {
		t.Fatalf("управляющие символы были удалены вместо видимого экранирования: %q", got)
	}
}

func TestOptionalTextDistinguishesNullEmptyAndValues(t *testing.T) {
	if got := optionalText(nil); got != "null" {
		t.Fatalf("nil должен обозначаться null, получено %q", got)
	}
	for _, value := range []string{"", "null", "  Русское Имя  ", "\"кавычки\"\\путь\n\x1b[2J"} {
		t.Run(fmt.Sprintf("value_%q", value), func(t *testing.T) {
			original := value
			got := optionalText(&value)
			if got != quotedText(value) {
				t.Fatalf("не-nil значение должно представляться через quotedText: got %q, want %q", got, quotedText(value))
			}
			if got == optionalText(nil) {
				t.Fatalf("указатель на строку не отличается от nil: %q", got)
			}
			payload := textTestAssertQuoted(t, got)
			if value == "" && payload != "" {
				t.Fatalf("пустая строка должна давать только пару кавычек: %q", got)
			}
			if value != original {
				t.Fatalf("optionalText изменил строку: got %q, want %q", value, original)
			}
		})
	}
}

func textTestAssertQuoted(t *testing.T, got string) string {
	t.Helper()
	if !utf8.ValidString(got) {
		t.Fatalf("результат не является UTF-8: %q", got)
	}
	for _, r := range got {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			t.Fatalf("в одной строке отчёта остался управляющий символ или перенос U+%04X: %q", r, got)
		}
	}
	for _, pair := range [][2]string{{`"`, `"`}, {"«", "»"}, {"“", "”"}, {"„", "“"}, {"'", "'"}} {
		if strings.HasPrefix(got, pair[0]) && strings.HasSuffix(got, pair[1]) && len(got) >= len(pair[0])+len(pair[1]) {
			return got[len(pair[0]) : len(got)-len(pair[1])]
		}
	}
	t.Fatalf("значение должно быть заключено в кавычки: %q", got)
	return ""
}

// textTestDecode проверяет обратимость распространённых видов экранирования.
// Если оформление иное, тесты отдельно проверяют безопасность и различимость,
// не объявляя конкретный синтаксис escape обязательным для production.
func textTestDecode(got string) (string, bool) {
	if value, err := strconv.Unquote(got); err == nil {
		return value, true
	}
	var value string
	if err := json.Unmarshal([]byte(got), &value); err == nil {
		return value, true
	}
	return "", false
}
