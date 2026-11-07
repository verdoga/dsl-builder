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

func weightsTestEmptySection(t *testing.T, op *app.Operation) {
	t.Helper()
	edit, issues := op.ProposeSection(op.Snapshot().Token, "Пустая рабочая секция")
	weightsTestOK(t, issues)
	if edit == nil {
		t.Fatal("owner did not create empty working section")
	}
	weightsTestOK(t, op.AcceptEdit(edit.Token))
}

func TestWeightsSectionsCollectsAllTitlesIncludingEmptyThenReviews(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(fmt.Sprint(accept), func(t *testing.T) {
			op, paths := weightsTestAdd(t, true)
			weightsTestEmptySection(t, op)
			weightsTestConsent(t, op)
			shown := op.Snapshot()
			acceptedCatalog := weightsTestJSON(t, shown.Catalog)
			unchanged := weightsTestCapture(t, op, paths)
			huge := strings.Repeat("9", 140)
			values := []string{huge, "0002", "1"}
			want := make(map[string]string)
			var lines []string
			for index, section := range shown.Catalog.Sections {
				want[section.Title] = values[index]
				lines = append(lines, "1\n", values[index]+"\n")
			}
			boundary := len(lines)
			answer := "2\n"
			if accept {
				answer = "1\n"
			}
			reader := &weightsTestLinesReader{lines: append(lines, answer), before: func(index int) {
				if index < boundary {
					unchanged()
					return
				}
				pending := op.Snapshot()
				if pending.Token != shown.Token || pending.Actions.EditCatalog || pending.Actions.Save ||
					!bytes.Equal(weightsTestJSON(t, pending.Catalog), acceptedCatalog) {
					t.Fatal("section set was not staged for a separate decision")
				}
			}}
			var output bytes.Buffer
			d := weightsTestDialog(op, reader, &output)
			if err := d.editSectionWeights(shown); err != nil {
				t.Fatal(err)
			}
			if reader.index != boundary+1 || !strings.Contains(output.String(), huge) ||
				!strings.Contains(output.String(), "Пустая рабочая секция") {
				t.Fatal("full section candidate was not collected and shown")
			}
			after := op.Snapshot()
			if !accept {
				if after.Token != shown.Token || !bytes.Equal(weightsTestJSON(t, after.Catalog), acceptedCatalog) || !after.Actions.EditCatalog {
					t.Fatal("rejected section set changed accepted state or left proposal open")
				}
				return
			}
			if after.Token == shown.Token || after.Actions.Save || len(after.Catalog.Sections) != len(want) {
				t.Fatal("accepted full set lost sections or kept consent")
			}
			for _, section := range after.Catalog.Sections {
				if section.Weight == nil || *section.Weight != want[section.Title] {
					t.Fatalf("title or independent raw weight lost: %+v", section)
				}
			}
			if after.Catalog.Sections[0].Title != "Пустая рабочая секция" ||
				after.Catalog.Sections[1].Title != "Бета" || after.Catalog.Sections[2].Title != "Альфа" {
				t.Fatal("owner numeric sorting was not applied to complete section set")
			}
		})
	}
}

func TestWeightsSectionsCancelAtEveryQuestionPreservesAcceptedStateAndConsent(t *testing.T) {
	for stop := 0; stop < 3; stop++ {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			op, paths := weightsTestAdd(t, true)
			weightsTestEmptySection(t, op)
			weightsTestConsent(t, op)
			unchanged := weightsTestCapture(t, op, paths)
			d := weightsTestDialog(op, strings.NewReader(strings.Repeat("1\n999\n", stop)+"2\n"), io.Discard)
			if err := d.editSectionWeights(op.Snapshot()); err != nil {
				t.Fatal(err)
			}
			unchanged()
		})
	}
}

func TestWeightsSectionsInvalidRawWaitsForCompleteSet(t *testing.T) {
	op, paths := weightsTestAdd(t, true)
	weightsTestConsent(t, op)
	before := op.Snapshot()
	unchanged := weightsTestCapture(t, op, paths)
	reader := &weightsTestLinesReader{lines: []string{"1\n", " 007 \n", "1\n", "8\n"},
		before: func(int) { unchanged() }}
	var output bytes.Buffer
	d := weightsTestDialog(op, reader, &output)
	if err := d.editSectionWeights(before); err != nil {
		t.Fatal(err)
	}
	if reader.index != 4 || !strings.Contains(output.String(), order.CodeInvalidWeight) ||
		!strings.Contains(output.String(), " 007 ") {
		t.Fatal("section input was validated early or changed before owner validation")
	}
	unchanged()
}

func TestWeightsSectionsEqualNumericWeightsUseOwnerTitleOrder(t *testing.T) {
	op, _ := weightsTestAdd(t, true)
	before := op.Snapshot()
	d := weightsTestDialog(op, strings.NewReader("1\n0002\n1\n2\n1\n"), io.Discard)
	if err := d.editSectionWeights(before); err != nil {
		t.Fatal(err)
	}
	after := op.Snapshot()
	if after.Catalog.Sections[0].Title != "Альфа" || after.Catalog.Sections[1].Title != "Бета" ||
		*after.Catalog.Sections[0].Weight != "0002" || *after.Catalog.Sections[1].Weight != "2" {
		t.Fatal("equal section weights rejected, rewritten or sorted incorrectly")
	}
}
