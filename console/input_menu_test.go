package console

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode"
)

func inputTestMenu(t *testing.T, output, question string, labels []string, count int) {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	questionCount := 0
	for _, line := range lines {
		if line == question {
			questionCount++
		}
	}
	if questionCount != count {
		t.Errorf("question shown %d times; want %d; output=%q", questionCount, count, output)
	}
	for index, label := range labels {
		want := string(rune('1'+index)) + ". " + label
		seen := 0
		for _, line := range lines {
			if line == want {
				seen++
			}
		}
		if seen != count {
			t.Errorf("menu item %q on separate line %d times; want %d; output=%q", want, seen, count, output)
		}
	}
	last := strings.LastIndex(output, labels[len(labels)-1])
	if last >= 0 && strings.TrimSpace(output[last+len(labels[len(labels)-1]):]) == "" {
		t.Errorf("missing separate prompt after last item: %q", output)
	}
}

func TestInputChooseReturnsOnlyShownIndex(t *testing.T) {
	labels := []string{"Первый", "Второй 雪", "  Третий  "}
	for index, answer := range []string{"1\n", "2\r\n", "3\n"} {
		var out bytes.Buffer
		got, err := inputTestDialog(strings.NewReader(answer), &out).choose("Выберите пункт", labels)
		if err != nil || got != index {
			t.Errorf("choose(%q) = %d, %v; want %d, nil", answer, got, err, index)
		}
		inputTestMenu(t, out.String(), "Выберите пункт", labels, 1)
	}
}

func TestInputChooseRepeatsSameMenuAndExplainsInvalidAnswer(t *testing.T) {
	question, labels := "Вопрос", []string{"Альфа", "Бета"}
	var normal bytes.Buffer
	if _, err := inputTestDialog(strings.NewReader("2\n"), &normal).choose(question, labels); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", "текст", "0", "-1", "3", "1.0", "999999999999999999999999999999", "٢"} {
		t.Run(invalid, func(t *testing.T) {
			var out bytes.Buffer
			got, err := inputTestDialog(strings.NewReader(invalid+"\n2\n"), &out).choose(question, labels)
			if err != nil || got != 1 {
				t.Fatalf("choose = %d, %v; want index 1 after invalid answer", got, err)
			}
			inputTestMenu(t, out.String(), question, labels, 2)
			first := strings.Index(out.String(), question)
			second := strings.Index(out.String()[first+len(question):], question) + first + len(question)
			between := out.String()[first+len(question) : second]
			for _, line := range strings.Split(normal.String(), "\n") {
				between = strings.ReplaceAll(between, line, "")
			}
			// Кроме исходного меню пользователь получает объяснение по-русски.
			if !strings.ContainsFunc(between, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
				t.Errorf("invalid answer has no Russian explanation: %q", out.String())
			}
		})
	}
}

func TestInputConfirmAcceptsOnlyNumericYesAndNo(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"1\n", true}, {"2\r\n", false},
		{"\nyes\ny\nда\nнет\n0\n3\n1.0\n1\n", true},
		{"no\nn\n2\n", false},
	} {
		var out bytes.Buffer
		got, err := inputTestDialog(strings.NewReader(tc.input), &out).confirm("Продолжить?")
		if err != nil || got != tc.want {
			t.Errorf("confirm(%q) = %v, %v; want %v, nil", tc.input, got, err, tc.want)
		}
		inputTestMenu(t, out.String(), "Продолжить?", []string{"Да", "Нет"}, strings.Count(tc.input, "\n"))
	}
}

func TestInputReadValuePreservesRawText(t *testing.T) {
	for _, raw := range []string{"0", "  ", "  \"C:\\Курс 雪\"  ", "٠１２٣", "2", "Отменить", strings.Repeat("0123456789", 9000)} {
		var out bytes.Buffer
		got, accepted, err := inputTestDialog(strings.NewReader("1\n"+raw+"\n"), &out).readValue("Введите значение", false)
		if err != nil || !accepted || got != raw {
			t.Errorf("readValue length=%d = length %d, accepted=%v, error=%v", len(raw), len(got), accepted, err)
		}
		for _, label := range []string{"1. Ввести значение", "2. Отменить текущий ввод и вернуться"} {
			if !strings.Contains(out.String(), label+"\n") {
				t.Errorf("separate value/cancel menu missing %q: %q", label, out.String())
			}
		}
	}
}

func TestInputReadValueMandatoryEmptyRepeatsQuestion(t *testing.T) {
	var out bytes.Buffer
	// Запасной 1 допускает повтор отдельного меню перед новой попыткой.
	got, accepted, err := inputTestDialog(strings.NewReader("1\n\n1\nполный ответ\n"), &out).readValue("Обязательное название", false)
	if err != nil || !accepted || got == "" {
		t.Fatalf("mandatory empty accepted or could not retry: %q, %v, %v", got, accepted, err)
	}
	if strings.Count(out.String(), "Обязательное название") < 2 {
		t.Errorf("empty mandatory answer did not repeat same question: %q", out.String())
	}
}

func TestInputReadValueOptionalUnitAllowsEmptyAndExplains(t *testing.T) {
	var out bytes.Buffer
	got, accepted, err := inputTestDialog(strings.NewReader("1\n\n"), &out).readValue("Название юнита (Enter — без юнита)", true)
	if err != nil || !accepted || got != "" {
		t.Fatalf("optional unit = %q, %v, %v; want empty accepted", got, accepted, err)
	}
	if !strings.Contains(out.String(), "Enter — без юнита") {
		t.Errorf("empty unit has no explicit explanation: %q", out.String())
	}
}

func TestInputReadValueOffersCancellationForEveryValue(t *testing.T) {
	var out bytes.Buffer
	d := inputTestDialog(strings.NewReader("1\nпервое\n2\nНЕ ЧИТАТЬ\n"), &out)
	first, accepted, err := d.readValue("Первое поле", false)
	if err != nil || !accepted || first != "первое" {
		t.Fatalf("first value = %q, %v, %v", first, accepted, err)
	}
	_, accepted, err = d.readValue("Второе поле", false)
	if err != nil || accepted {
		t.Fatalf("cancelled second value accepted=%v error=%v", accepted, err)
	}
	if strings.Count(out.String(), "2. Отменить текущий ввод и вернуться") != 2 {
		t.Errorf("each value needs its own cancellation menu: %q", out.String())
	}
	remaining, err := d.readLine()
	if err != nil || remaining != "НЕ ЧИТАТЬ" {
		t.Errorf("cancel consumed free input: %q, %v", remaining, err)
	}
}

func TestInputMenusPropagateEOFAndStreamFailures(t *testing.T) {
	methods := []struct {
		name string
		call func(*dialog) error
	}{
		{"choose", func(d *dialog) error { _, err := d.choose("Вопрос", []string{"Пункт"}); return err }},
		{"confirm", func(d *dialog) error { _, err := d.confirm("Вопрос"); return err }},
		{"readValue", func(d *dialog) error { _, _, err := d.readValue("Значение", false); return err }},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			for _, text := range []string{"", "1"} {
				err := method.call(inputTestDialog(strings.NewReader(text), io.Discard))
				if err != io.EOF {
					t.Errorf("incomplete menu answer %q error=%v; want direct EOF", text, err)
				}
			}
			cause := errors.New("read failed")
			r := &inputTestReader{err: cause}
			err := method.call(inputTestDialog(r, io.Discard))
			inputTestStreamError(t, err, CodeInput, cause)
			if r.calls != 1 {
				t.Errorf("read failure retried: %d", r.calls)
			}
			w := &inputTestWriter{err: cause}
			r = &inputTestReader{text: "1\n"}
			err = method.call(inputTestDialog(r, w))
			inputTestStreamError(t, err, CodeOutput, cause)
			if w.calls != 1 || r.calls != 0 {
				t.Errorf("output failure continued: writes=%d reads=%d", w.calls, r.calls)
			}
		})
	}
}

func TestInputReadValueIncompleteFreeInputIsNotAccepted(t *testing.T) {
	for _, text := range []string{"1\n", "1\n0", "1\nчасть\r"} {
		_, accepted, err := inputTestDialog(strings.NewReader(text), io.Discard).readValue("Значение", false)
		if accepted || err != io.EOF {
			t.Errorf("readValue(%q) accepted=%v error=%v; want false, direct EOF", text, accepted, err)
		}
	}
}

func TestInputReadValueFreeInputFailureStopsImmediately(t *testing.T) {
	cause := errors.New("input failed in free value")
	r := &inputTestReader{text: "1\nчастичное значение", err: cause}
	_, accepted, err := inputTestDialog(r, io.Discard).readValue("Значение", false)
	if accepted || r.calls != 1 {
		t.Errorf("failed free input accepted=%v reads=%d", accepted, r.calls)
	}
	inputTestStreamError(t, err, CodeInput, cause)
}

func TestInputChooseEscapesExternalLabels(t *testing.T) {
	var out bytes.Buffer
	_, err := inputTestDialog(strings.NewReader("1\n"), &out).choose("Выбор", []string{"  Начало雪\x1b[2J\nКонец🙂  "})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b") || strings.Contains(out.String(), "\nКонец") {
		t.Errorf("external label can control terminal or inject menu line: %q", out.String())
	}
	for _, part := range []string{"  Начало雪", "Конец🙂  "} {
		if !strings.Contains(out.String(), part) {
			t.Errorf("label changed/truncated %q: %q", part, out.String())
		}
	}
}
