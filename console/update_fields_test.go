package console

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/preview"
)

func consoleUpdateTestFieldProposal(t *testing.T, snapshot app.Snapshot, index int, text string) {
	t.Helper()
	want, err := preview.FieldProposal(snapshot.Before, snapshot.Plan.Fields[index])
	if err != nil {
		t.Fatal(err)
	}
	question := strings.Index(text, "Применить это изменение?")
	position := strings.Index(text, want)
	if position < 0 || question < position+len(want) ||
		!strings.Contains(text[question:], "1. Да\n") || !strings.Contains(text[question:], "2. Нет\n") {
		t.Fatalf("complete old-file proposal must precede separate confirmation: %q", text)
	}
	if strings.Count(text, want) != 1 {
		t.Fatalf("one invocation must show one proposal: %q", text)
	}
}

func TestConsoleUpdateFieldUsesAnsweredAndDecidesExactlyOne(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(map[bool]string{false: "refuse", true: "accept"}[accept], func(t *testing.T) {
			before := consoleUpdateTestResult("Старое", consoleUpdateTestText("Подзаголовок"), []*string{nil}, [][]string{{"Материал"}})
			after := consoleUpdateTestResult("Новое", nil, []*string{consoleUpdateTestText("Страница")}, [][]string{{"Материал"}})
			op := consoleUpdateTestOperation(t, before, after, false)
			first := op.Snapshot()
			consoleUpdateTestFieldCount(t, first, 3)
			consoleUpdateTestOK(t, op.DecideField(first.Token, 0, false))
			snapshot := op.Snapshot()
			if !snapshot.FieldAnswered[0] || snapshot.FieldAccepted[0] {
				t.Fatal("fixture must have answered but refused first field")
			}
			var out bytes.Buffer
			answer := "2\n"
			if accept {
				answer = "1\n"
			}
			d := consoleUpdateTestDialog(op, answer+"sentinel\n", &out)
			if err := d.decideField(snapshot); err != nil {
				t.Fatal(err)
			}
			consoleUpdateTestFieldProposal(t, snapshot, 1, out.String())
			got := op.Snapshot()
			if !got.FieldAnswered[0] || !got.FieldAnswered[1] || got.FieldAnswered[2] ||
				got.FieldAccepted[0] || got.FieldAccepted[1] != accept || got.RevisionReady || got.Actions.Save {
				t.Fatalf("one answer changed more than the first pending field: %+v", got)
			}
			if remaining, _ := d.input.ReadString('\n'); remaining != "sentinel\n" {
				t.Fatalf("helper consumed the next answer: %q", remaining)
			}
			// Следующий вызов получает новый снимок; отказ не равен ожиданию.
			out.Reset()
			d = consoleUpdateTestDialog(op, "2\n", &out)
			if err := d.decideField(got); err != nil {
				t.Fatal(err)
			}
			consoleUpdateTestFieldProposal(t, got, 2, out.String())
			if !op.Snapshot().FieldAnswered[2] || op.Snapshot().FieldAccepted[2] {
				t.Fatal("fresh decision did not reach the remaining field")
			}
		})
	}
}

func TestConsoleUpdateFieldKnownNullAndUnmatchedNullStillNeedAnswer(t *testing.T) {
	for _, kind := range []string{"known-null", "known-value-to-null", "unmatched-null"} {
		for _, answer := range []string{"1\n", "2\n"} {
			t.Run(kind+answer[:1], func(t *testing.T) {
				before, after := consoleUpdateTestPlain(), consoleUpdateTestPlain()
				switch kind {
				case "known-null":
					after = consoleUpdateTestResult("Прежнее", nil, []*string{consoleUpdateTestText("Название")}, [][]string{{"Материал", "Второй"}})
				case "known-value-to-null":
					before.Document.Metadata.Subtitle = consoleUpdateTestText("Прежний подзаголовок")
				case "unmatched-null":
					after = consoleUpdateTestResult("Прежнее", nil, []*string{nil, nil}, [][]string{{"Материал", "Второй"}, {"Новая страница"}})
				}
				op := consoleUpdateTestOperation(t, before, after, false)
				snapshot := op.Snapshot()
				consoleUpdateTestFieldCount(t, snapshot, 1)
				field := snapshot.Plan.Fields[0]
				if kind == "unmatched-null" && (field.Field != compare.PageTitle || field.OldPage != -1 || field.After != nil) {
					t.Fatalf("fixture did not exercise unmatched null page: %+v", field)
				}
				var out bytes.Buffer
				d := consoleUpdateTestDialog(op, answer, &out)
				if err := d.decideField(snapshot); err != nil {
					t.Fatal(err)
				}
				consoleUpdateTestFieldProposal(t, snapshot, 0, out.String())
				got := op.Snapshot()
				if !got.FieldAnswered[0] || got.FieldAccepted[0] != (answer == "1\n") || got.RevisionReady {
					t.Fatal("null proposal was hidden or prepared implicitly")
				}
			})
		}
	}
}

func TestConsoleUpdateFieldDoesNotRefreshStaleToken(t *testing.T) {
	before := consoleUpdateTestPlain()
	after := consoleUpdateTestResult("Новое", consoleUpdateTestText("Подзаголовок"), []*string{nil}, [][]string{{"Материал", "Второй"}})
	op := consoleUpdateTestOperation(t, before, after, false)
	stale := op.Snapshot()
	consoleUpdateTestFieldCount(t, stale, 2)
	consoleUpdateTestOK(t, op.DecideField(stale.Token, 1, true))
	var out bytes.Buffer
	d := consoleUpdateTestDialog(op, "1\n", &out)
	if err := d.decideField(stale); err != nil {
		t.Fatal(err)
	}
	got := op.Snapshot()
	if got.FieldAnswered[0] || !got.FieldAnswered[1] || !strings.Contains(out.String(), app.CodeStaleReview) {
		t.Fatal("stale shown token was silently replaced or owner error hidden")
	}
}

func consoleUpdateTestDialog(op *app.Operation, input string, output io.Writer) *dialog {
	return &dialog{operation: op, input: bufio.NewReader(strings.NewReader(input)), output: output}
}
