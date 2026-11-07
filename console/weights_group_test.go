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

func TestWeightsGroupCompleteSwapStagesOnceThenAcceptsOrRejects(t *testing.T) {
	for _, sectioned := range []bool{false, true} {
		for _, accept := range []bool{false, true} {
			t.Run(fmt.Sprintf("sectioned_%v_accept_%v", sectioned, accept), func(t *testing.T) {
				op, paths := weightsTestAdd(t, sectioned)
				weightsTestConsent(t, op)
				before := op.Snapshot()
				unchanged := weightsTestCapture(t, op, paths)
				var section *string
				choice := "1\n"
				if sectioned {
					section, choice = weightsTestString("Бета"), "2\n"
				}
				var names, values []string
				for _, item := range before.Catalog.Items {
					if (section == nil && item.Section == nil) ||
						(section != nil && item.Section != nil && *section == *item.Section) {
						names = append(names, item.Entry.FileName)
						values = append(values, *item.Entry.Order)
					}
				}
				values[0], values[1] = values[1], values[0]
				lines := []string{choice}
				for _, raw := range values {
					lines = append(lines, "1\n", raw+"\n")
				}
				answer := "2\n"
				if accept {
					answer = "1\n"
				}
				boundary := len(lines)
				lines = append(lines, answer)
				reader := &weightsTestLinesReader{lines: lines, before: func(index int) {
					if index < boundary {
						unchanged()
						return
					}
					pending := op.Snapshot()
					if pending.Token != before.Token || pending.Actions.EditCatalog || pending.Actions.Save ||
						!bytes.Equal(weightsTestJSON(t, before.Catalog), weightsTestJSON(t, pending.Catalog)) {
						t.Fatal("review must expose one staged edit without accepting partial values")
					}
				}}
				var output bytes.Buffer
				d := weightsTestDialog(op, reader, &output)
				if err := d.editGroupWeights(before); err != nil {
					t.Fatal(err)
				}
				if reader.index != len(lines) || strings.Contains(output.String(), app.CodeState) {
					t.Fatal("group did not complete one proposal and one review")
				}
				after := op.Snapshot()
				if !accept {
					if after.Token != before.Token || !bytes.Equal(weightsTestJSON(t, before.Catalog), weightsTestJSON(t, after.Catalog)) {
						t.Fatal("rejected group changed accepted state")
					}
					if !after.Actions.EditCatalog {
						t.Fatal("rejection left proposal open")
					}
					return
				}
				for index, name := range names {
					if got := weightsTestOrder(t, after, name); got == nil || *got != values[index] {
						t.Fatalf("wrong complete set for %s", name)
					}
				}
				for _, item := range before.Catalog.Items {
					if section != nil && (item.Section == nil || *item.Section != *section) &&
						*weightsTestOrder(t, after, item.Entry.FileName) != *item.Entry.Order {
						t.Fatal("changed foreign group")
					}
				}
				if len(after.Blockers) != 0 || len(after.Conflicts) != 0 || after.Actions.Save || after.Token == before.Token {
					t.Fatal("temporary swap conflict prevented complete acceptance or kept consent")
				}
			})
		}
	}
}

func TestWeightsGroupCancelAtEveryQuestionPreservesAcceptedStateAndConsent(t *testing.T) {
	for stop := -1; stop < 5; stop++ {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			op, paths := weightsTestAdd(t, false)
			weightsTestConsent(t, op)
			unchanged := weightsTestCapture(t, op, paths)
			script := "2\n"
			if stop >= 0 {
				script = "1\n" + strings.Repeat("1\n999\n", stop) + "2\n"
			}
			d := weightsTestDialog(op, strings.NewReader(script), io.Discard)
			if err := d.editGroupWeights(op.Snapshot()); err != nil {
				t.Fatal(err)
			}
			unchanged()
		})
	}
}

func TestWeightsGroupInvalidRawIsValidatedOnlyAfterFullInput(t *testing.T) {
	op, paths := weightsTestAdd(t, false)
	weightsTestConsent(t, op)
	before := op.Snapshot()
	unchanged := weightsTestCapture(t, op, paths)
	lines := []string{"1\n", "1\n", " 0007 \n", "1\n", "98\n", "1\n", "97\n", "1\n", "96\n", "1\n", "95\n"}
	reader := &weightsTestLinesReader{lines: lines, before: func(int) { unchanged() }}
	var output bytes.Buffer
	d := weightsTestDialog(op, reader, &output)
	if err := d.editGroupWeights(before); err != nil {
		t.Fatal(err)
	}
	if reader.index != len(lines) || !strings.Contains(output.String(), order.CodeInvalidWeight) ||
		!strings.Contains(output.String(), " 0007 ") {
		t.Fatal("console validated early or lost owner's raw diagnostic")
	}
	unchanged()
}

func TestWeightsGroupSelectsUnallocatedNilGroupSeparately(t *testing.T) {
	op, _ := weightsTestAdd(t, true)
	before := op.Snapshot()
	name := before.Catalog.Items[0].Entry.FileName
	edit, issues := op.ProposeMove(before.Token, name, nil)
	if edit == nil {
		t.Fatalf("missing editable candidate: %v", issues)
	}
	_ = op.AcceptEdit(edit.Token)
	shown := op.Snapshot()
	d := weightsTestDialog(op, strings.NewReader("3\n1\n000999\n1\n"), io.Discard)
	if err := d.editGroupWeights(shown); err != nil {
		t.Fatal(err)
	}
	after := op.Snapshot()
	if raw := weightsTestOrder(t, after, name); raw == nil || *raw != "000999" {
		t.Fatal("nil group was confused with cancel or a named section")
	}
	for _, item := range shown.Catalog.Items {
		if item.Entry.FileName != name && *weightsTestOrder(t, after, item.Entry.FileName) != *item.Entry.Order {
			t.Fatal("nil group included a foreign file")
		}
	}
}
