package console

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/order"
)

func TestWeightsSingleExactFileAndRawAcceptedOrRejected(t *testing.T) {
	for _, raw := range []string{"000007", strings.Repeat("8", 135)} {
		for _, accept := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_%v", raw[:3], accept), func(t *testing.T) {
				op, paths := weightsTestAdd(t, false)
				before := op.Snapshot()
				name := before.Catalog.Items[1].Entry.FileName
				unchanged := weightsTestCapture(t, op, paths)
				answer := "2\n"
				if accept {
					answer = "1\n"
				}
				var output bytes.Buffer
				d := weightsTestDialog(op, strings.NewReader("2\n1\n"+raw+"\n"+answer), &output)
				if err := d.editLessonWeight(before); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(output.String(), raw) {
					t.Fatal("candidate raw weight was not shown")
				}
				if !accept {
					unchanged()
					return
				}
				after := op.Snapshot()
				if got := weightsTestOrder(t, after, name); got == nil || *got != raw {
					t.Fatalf("selected file lost exact weight: %v", got)
				}
				for _, item := range before.Catalog.Items {
					if item.Entry.FileName != name && *weightsTestOrder(t, after, item.Entry.FileName) != *item.Entry.Order {
						t.Fatal("single weight changed a different file")
					}
				}
				if after.Token == before.Token || after.Actions.Save {
					t.Fatal("accepted edit retained old token or granted save")
				}
			})
		}
	}
}

func TestWeightsSingleInvalidRawReachesOwnerUnchanged(t *testing.T) {
	for _, raw := range []string{"0", "-2", "+3", "1.2", "1e9", " 0007 ", "１２"} {
		t.Run(raw, func(t *testing.T) {
			op, paths := weightsTestAdd(t, false)
			weightsTestConsent(t, op)
			unchanged := weightsTestCapture(t, op, paths)
			snapshot := op.Snapshot()
			var output bytes.Buffer
			d := weightsTestDialog(op, strings.NewReader("1\n1\n"+raw+"\n"), &output)
			if err := d.editLessonWeight(snapshot); err != nil {
				t.Fatal(err)
			}
			text := output.String()
			if !strings.Contains(text, order.CodeInvalidWeight) || !strings.Contains(text, raw) ||
				!strings.Contains(text, snapshot.Catalog.Items[0].Entry.FileName) {
				t.Fatalf("missing owner diagnostic for exact raw %q: %s", raw, text)
			}
			unchanged()
		})
	}
}

func TestWeightsSingleCancelAtSelectionOrValuePreservesConsent(t *testing.T) {
	for _, script := range []string{"6\n", "1\n2\n"} {
		t.Run(script, func(t *testing.T) {
			op, paths := weightsTestAdd(t, false)
			weightsTestConsent(t, op)
			unchanged := weightsTestCapture(t, op, paths)
			d := weightsTestDialog(op, strings.NewReader(script), io.Discard)
			if err := d.editLessonWeight(op.Snapshot()); err != nil {
				t.Fatal(err)
			}
			unchanged()
		})
	}
}

// weightsTestLinesReader делает каждую завершённую строку отдельной границей ввода.
type weightsTestLinesReader struct {
	lines  []string
	index  int
	before func(int)
}

func (reader *weightsTestLinesReader) Read(dst []byte) (int, error) {
	if reader.index == len(reader.lines) {
		return 0, io.EOF
	}
	if reader.before != nil {
		reader.before(reader.index)
	}
	line := reader.lines[reader.index]
	n := copy(dst, line)
	reader.lines[reader.index] = line[n:]
	if n == len(line) {
		reader.index++
	}
	return n, nil
}

func TestWeightsSingleOriginalShownTokenCannotBeReplacedByFreshToken(t *testing.T) {
	op, _ := weightsTestAdd(t, false)
	shown := op.Snapshot()
	edit, issues := op.ProposeCourseTitle(shown.Token, "Новый принятый курс")
	weightsTestOK(t, issues)
	weightsTestOK(t, op.AcceptEdit(edit.Token))
	current := op.Snapshot()
	var output bytes.Buffer
	d := weightsTestDialog(op, strings.NewReader("1\n1\n999\n"), &output)
	if err := d.editLessonWeight(shown); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), app.CodeStaleReview) {
		t.Fatal("stale shown token was replaced")
	}
	if op.Snapshot().Token != current.Token ||
		!bytes.Equal(weightsTestJSON(t, current.Catalog), weightsTestJSON(t, op.Snapshot().Catalog)) {
		t.Fatal("stale dialog changed current accepted state")
	}
}
