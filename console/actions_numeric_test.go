package console

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestActionsNumericDialogBoundaries(t *testing.T) {
	// Только номера operation actions выбираются по текущим labels;
	// фиксированные main/confirm/readValue имеют собственные строгие границы.
	for want := 1; want <= 4; want++ {
		t.Run(fmt.Sprintf("main/%d", want), func(t *testing.T) {
			var out bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader(fmt.Sprintf("5\n0\nда\n%d\n", want))), output: &out}
			got, err := d.mainChoice()
			if err != nil || got != want {
				t.Fatalf("main=(%d,%v)", got, err)
			}
			for n, label := range []string{"Создать новый модуль", "Обновить существующий модуль", "Добавить уроки в существующий модуль", "Выход"} {
				if strings.Count(out.String(), fmt.Sprintf("%d. %s", n+1, label)) != 4 {
					t.Errorf("wrong/reused main numbering for %q", label)
				}
			}
		})
	}
	for _, answer := range []string{"1", "2"} {
		t.Run("confirm/"+answer, func(t *testing.T) {
			var out bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader("3\nда\n\n" + answer + "\n")), output: &out}
			got, err := d.confirm("Отдельное согласие?")
			if err != nil || got != (answer == "1") {
				t.Fatalf("confirm=(%v,%v)", got, err)
			}
			if strings.Count(out.String(), "1. Да") != 4 || strings.Count(out.String(), "2. Нет") != 4 {
				t.Fatal("confirmation list changed after invalid answers")
			}
		})
	}
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("value/cancel=%v", cancel), func(t *testing.T) {
			answer := "1\n0\n"
			if cancel {
				answer = "2\n"
			}
			var out bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader("3\n0\n" + answer)), output: &out}
			value, accepted, err := d.readValue("Значение", false)
			if err != nil || accepted == cancel {
				t.Fatalf("readValue=(%q,%v,%v)", value, accepted, err)
			}
			if cancel && value != "" || !cancel && value != "0" {
				t.Fatalf("raw/cancel value=%q", value)
			}
			if strings.Count(out.String(), "1. Ввести значение") != 3 || strings.Count(out.String(), "2. Отменить текущий ввод и вернуться") != 3 {
				t.Fatal("readValue accepted out-of-range menu answer")
			}
		})
	}
}
