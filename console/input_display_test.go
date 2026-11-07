package console

import (
	"strings"
	"testing"
	"unicode"
)

func TestInputDisplayValuePreservesUnicodeAndExactSpaces(t *testing.T) {
	for _, raw := range []string{"", "   ", "  Кириллица 雪 🙂  ", "  0 ٠１２  ", strings.Repeat("  длинный путь Ж🙂  ", 12000)} {
		got := displayValue(raw)
		if !strings.Contains(got, raw) {
			t.Errorf("displayValue changed/truncated ordinary value length=%d to length=%d", len(raw), len(got))
		}
		if strings.TrimSpace(raw) != "" && strings.Count(got, strings.TrimSpace(raw)) != 1 {
			t.Errorf("ordinary value was repeated or lost: input length=%d output length=%d", len(raw), len(got))
		}
	}
}

func TestInputDisplayValueMakesEveryControlVisible(t *testing.T) {
	for _, control := range []rune{'\x00', '\x07', '\b', '\t', '\n', '\v', '\f', '\r', '\x1b', '\x7f', '\u0085', '\u009b'} {
		t.Run(string([]rune{'U', control}), func(t *testing.T) {
			got := displayValue("  Начало雪" + string(control) + "Конец🙂  ")
			if strings.ContainsFunc(got, unicode.IsControl) {
				t.Errorf("raw terminal control remains: %q", got)
			}
			start, end := strings.Index(got, "  Начало雪"), strings.Index(got, "Конец🙂  ")
			if start < 0 || end < 0 || end <= start+len("  Начало雪") {
				t.Errorf("control silently deleted, surrounding Unicode/spaces lost or reordered: %q", got)
			}
		})
	}
}

func TestInputDisplayValueEscapesLineBreaksWithoutTruncating(t *testing.T) {
	raw := "  Первая🙂\r\nВторая 雪\nПоследняя  "
	got := displayValue(raw)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("displayValue permits line injection: %q", got)
	}
	for _, part := range []string{"  Первая🙂", "Вторая 雪", "Последняя  "} {
		if !strings.Contains(got, part) {
			t.Errorf("displayValue lost %q: %q", part, got)
		}
	}
}

func TestInputDestinationDirectoryRemovesExactlyOneExternalQuotePair(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"", ""},
		{`""`, ""},
		{`"C:\Курс 雪\Каталог"`, `C:\Курс 雪\Каталог`},
		{`""C:\Курс""`, `"C:\Курс"`},
		{`"""C:\Курс"""`, `""C:\Курс""`},
		{`"  C:\Курс  "`, `  C:\Курс  `},
		{` "C:\Курс" `, ` "C:\Курс" `},
		{`C:\Курс`, `C:\Курс`},
		{`"C:\Курс`, `"C:\Курс`},
		{`C:\Курс"`, `C:\Курс"`},
		{`"`, `"`},
		{`'C:\Курс'`, `'C:\Курс'`},
		{`«C:\Курс»`, `«C:\Курс»`},
		{`"C:\new\tab\u0041\"имя\""`, `C:\new\tab\u0041\"имя\"`},
		{"\"C:\\Курс\n  \"", "C:\\Курс\n  "},
	} {
		if got := destinationDirectory(tc.raw); got != tc.want {
			t.Errorf("destinationDirectory(%q) = %q; want %q", tc.raw, got, tc.want)
		}
	}
}
