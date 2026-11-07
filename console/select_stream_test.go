package console

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

func selectTestChoose(d *dialog, snapshot app.Snapshot, selector string) (bool, error) {
	switch selector {
	case "lesson":
		_, accepted, err := d.chooseLesson(snapshot)
		return accepted, err
	case "section":
		_, accepted, err := d.chooseSection(snapshot)
		return accepted, err
	default:
		_, accepted, err := d.chooseGroup(snapshot)
		return accepted, err
	}
}

type selectTestReader struct {
	calls int
	err   error
}

func (r *selectTestReader) Read([]byte) (int, error) {
	r.calls++
	if r.calls > 1 {
		return 0, errors.New("чтение после первоначального сбоя")
	}
	return 0, r.err
}

type selectTestWriter struct {
	calls int
	err   error
}

func (w *selectTestWriter) Write(raw []byte) (int, error) {
	w.calls++
	if w.err != nil {
		return 0, w.err
	}
	return len(raw) - 1, nil
}

func TestSelectCancellationIsLastNumericChoice(t *testing.T) {
	for _, test := range []struct{ selector, input string }{
		{"lesson", "4\n"}, {"section", "5\n"}, {"group", "4\n"},
	} {
		t.Run(test.selector, func(t *testing.T) {
			snapshot := selectTestSnapshot()
			before := selectTestBytes(t, snapshot)
			d, output := selectTestDialog(test.input)
			accepted, err := selectTestChoose(d, snapshot, test.selector)
			if err != nil || accepted {
				t.Fatalf("последний пункт не отменил выбор: (%v, %v)", accepted, err)
			}
			if !strings.Contains(strings.ToLower(output.String()), "отмен") {
				t.Fatal("меню не обозначает отмену")
			}
			selectTestUnchanged(t, before, snapshot)
		})
	}
}

func TestSelectInvalidNumbersRepeatSameQuestionAndList(t *testing.T) {
	for _, selector := range []string{"lesson", "section", "group"} {
		t.Run(selector, func(t *testing.T) {
			snapshot := selectTestSnapshot()
			before := selectTestBytes(t, snapshot)
			direct, output := selectTestDialog("2\n")
			if accepted, err := selectTestChoose(direct, snapshot, selector); err != nil || !accepted {
				t.Fatalf("исходный выбор: (%v, %v)", accepted, err)
			}
			retried, repeated := selectTestDialog("\n0\n-1\nне число\n999\n2\n")
			if accepted, err := selectTestChoose(retried, snapshot, selector); err != nil || !accepted {
				t.Fatalf("выбор после неверного ввода: (%v, %v)", accepted, err)
			}
			markers := []string{"l701.json", "l003.json", "l2000.json"}
			if selector == "section" {
				markers = []string{"Я — первая", "Пустая рабочая", "А — вторая", "Без секции"}
			} else if selector == "group" {
				markers = []string{"Я — первая", "А — вторая", "Не распределены"}
			}
			lines := strings.Split(output.String(), "\n")
			for markerIndex, marker := range markers {
				found := false
				for lineIndex, line := range lines {
					if !strings.Contains(line, marker) {
						continue
					}
					found = true
					if strings.Count(repeated.String(), line) != 6 {
						t.Fatalf("изменился список после неверного ответа: %q", line)
					}
					if markerIndex == 0 {
						question := lineIndex - 1
						for question >= 0 && strings.TrimSpace(lines[question]) == "" {
							question--
						}
						if question < 0 || strings.Count(repeated.String(), lines[question]) != 6 {
							t.Fatal("вопрос не повторён после каждого неверного ответа")
						}
					}
					break
				}
				if !found {
					t.Fatalf("в исходном меню нет подписи %q", marker)
				}
			}
			selectTestUnchanged(t, before, snapshot)
		})
	}
}

func TestSelectEOFDiscardsIncompleteAnswer(t *testing.T) {
	for _, selector := range []string{"lesson", "section", "group"} {
		for _, input := range []string{"", "1"} {
			t.Run(selector+"/"+input, func(t *testing.T) {
				snapshot := selectTestSnapshot()
				before := selectTestBytes(t, snapshot)
				d, _ := selectTestDialog(input)
				accepted, err := selectTestChoose(d, snapshot, selector)
				if accepted || !errors.Is(err, io.EOF) {
					t.Fatalf("EOF превратился в ответ: (%v, %v)", accepted, err)
				}
				selectTestUnchanged(t, before, snapshot)
			})
		}
	}
}

func TestSelectReadFailureIsFatalWithoutRetry(t *testing.T) {
	for _, selector := range []string{"lesson", "section", "group"} {
		t.Run(selector, func(t *testing.T) {
			snapshot := selectTestSnapshot()
			before := selectTestBytes(t, snapshot)
			cause := errors.New("вход закрыт")
			reader := &selectTestReader{err: cause}
			d := &dialog{input: bufio.NewReader(reader), output: new(bytes.Buffer)}
			accepted, err := selectTestChoose(d, snapshot, selector)
			var issue *diagnostic.Error
			if accepted || !errors.Is(err, cause) || !errors.As(err, &issue) || issue.Code != CodeInput || reader.calls != 1 {
				t.Fatalf("ошибка чтения не поднята сразу: accepted=%v err=%v calls=%d", accepted, err, reader.calls)
			}
			selectTestUnchanged(t, before, snapshot)
		})
	}
}

func TestSelectOutputFailureStopsBeforeReading(t *testing.T) {
	for _, selector := range []string{"lesson", "section", "group"} {
		for _, short := range []bool{false, true} {
			name := selector + "/error"
			if short {
				name = selector + "/short"
			}
			t.Run(name, func(t *testing.T) {
				snapshot := selectTestSnapshot()
				before := selectTestBytes(t, snapshot)
				cause := errors.New("выход закрыт")
				writer := &selectTestWriter{err: cause}
				if short {
					writer.err, cause = nil, io.ErrShortWrite
				}
				reader := &selectTestReader{err: errors.New("чтение запрещено после сбоя вывода")}
				d := &dialog{input: bufio.NewReader(reader), output: writer}
				accepted, err := selectTestChoose(d, snapshot, selector)
				var issue *diagnostic.Error
				if accepted || !errors.Is(err, cause) || !errors.As(err, &issue) || issue.Code != CodeOutput || reader.calls != 0 || writer.calls != 1 {
					t.Fatalf("поток не остановлен: accepted=%v err=%v reads=%d writes=%d", accepted, err, reader.calls, writer.calls)
				}
				selectTestUnchanged(t, before, snapshot)
			})
		}
	}
}
