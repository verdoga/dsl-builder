package console

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestWeightsMissingLessonWeightCanBeEntered(t *testing.T) {
	op, _ := weightsTestAddWithWeight(t, false, nil)
	shown := op.Snapshot()
	selected := -1
	for index, item := range shown.Catalog.Items {
		if item.Entry.ID == "WT99" {
			selected = index
			if item.Entry.Order != nil {
				t.Fatal("fixture does not have a missing weight")
			}
		}
	}
	if selected < 0 {
		t.Fatal("incoming lesson missing")
	}
	script := strconv.Itoa(selected+1) + "\n1\n00060\n1\n"
	d := weightsTestDialog(op, strings.NewReader(script), io.Discard)
	if err := d.editLessonWeight(shown); err != nil {
		t.Fatal(err)
	}
	after := op.Snapshot()
	if len(after.Blockers) != 0 || *weightsTestOrder(t, after, shown.Catalog.Items[selected].Entry.FileName) != "00060" {
		t.Fatal("missing weight could not be repaired")
	}
}

func TestWeightsGroupAndSectionsUseOriginalShownToken(t *testing.T) {
	for _, sections := range []bool{false, true} {
		op, paths := weightsTestAdd(t, sections)
		shown := op.Snapshot()
		edit, issues := op.ProposeCourseTitle(shown.Token, "Текущий принятый курс")
		weightsTestOK(t, issues)
		weightsTestOK(t, op.AcceptEdit(edit.Token))
		unchanged := weightsTestCapture(t, op, paths)
		var output bytes.Buffer
		script := "1\n1\n91\n1\n92\n1\n93\n1\n94\n1\n95\n"
		if sections {
			script = "1\n91\n1\n92\n"
		}
		d := weightsTestDialog(op, strings.NewReader(script), &output)
		var err error
		if sections {
			err = d.editSectionWeights(shown)
		} else {
			err = d.editGroupWeights(shown)
		}
		if err != nil || !strings.Contains(output.String(), app.CodeStaleReview) {
			t.Fatalf("original shown token was not forwarded: %v, %s", err, output.String())
		}
		unchanged()
	}
}

func TestWeightsInterruptedEditNeverAppliesPartialSet(t *testing.T) {
	failure := errors.New("weights read failed")
	for _, test := range []struct {
		name      string
		sectioned bool
		script    string
		invoke    func(*dialog, app.Snapshot) error
	}{
		{"single", false, "1\n1\n999", (*dialog).editLessonWeight},
		{"group", false, "1\n1\n999\n1\n888", (*dialog).editGroupWeights},
		{"sections", true, "1\n999\n1\n888", (*dialog).editSectionWeights},
	} {
		for _, streamError := range []bool{false, true} {
			t.Run(test.name+strconv.FormatBool(streamError), func(t *testing.T) {
				op, paths := weightsTestAdd(t, test.sectioned)
				weightsTestConsent(t, op)
				before := op.Snapshot()
				acceptedCatalog := weightsTestJSON(t, before.Catalog)
				var input io.Reader = strings.NewReader(test.script)
				want := io.EOF
				if streamError {
					want = failure
					input = &weightsTestFaultReader{input: strings.NewReader(test.script), cause: failure}
				}
				originals := make(map[string][]byte)
				for _, path := range paths {
					originals[path] = weightsTestRead(t, path)
				}
				d := weightsTestDialog(op, input, io.Discard)
				if err := test.invoke(d, before); !errors.Is(err, want) {
					t.Fatalf("interruption lost: %v", err)
				}
				after := op.Snapshot()
				if !bytes.Equal(acceptedCatalog, weightsTestJSON(t, after.Catalog)) || after.WriteResult != nil {
					t.Fatal("interruption partially accepted or wrote data")
				}
				if after.Token == before.Token && !after.Actions.Save {
					t.Fatal("incomplete input created a proposal and revoked consent")
				}
				for path, raw := range originals {
					if !bytes.Equal(raw, weightsTestRead(t, path)) {
						t.Fatalf("interruption changed %s", path)
					}
				}
			})
		}
	}
}

func TestWeightsEOFAtReviewCancelsWholeOperationWithoutAccepting(t *testing.T) {
	for _, test := range []struct {
		name      string
		sectioned bool
		script    string
		invoke    func(*dialog, app.Snapshot) error
	}{
		{"single", false, "1\n1\n999\n", (*dialog).editLessonWeight},
		{"group", false, "1\n1\n91\n1\n92\n1\n93\n1\n94\n1\n95\n", (*dialog).editGroupWeights},
		{"sections", true, "1\n91\n1\n92\n", (*dialog).editSectionWeights},
	} {
		t.Run(test.name, func(t *testing.T) {
			op, paths := weightsTestAdd(t, test.sectioned)
			shown := op.Snapshot()
			before := weightsTestJSON(t, shown.Catalog)
			archive := paths[len(paths)-2]
			raw := weightsTestRead(t, archive)
			d := weightsTestDialog(op, strings.NewReader(test.script), io.Discard)
			if err := test.invoke(d, shown); !errors.Is(err, io.EOF) {
				t.Fatalf("review EOF: %v", err)
			}
			after := op.Snapshot()
			if !bytes.Equal(before, weightsTestJSON(t, after.Catalog)) || after.Actions.Save ||
				after.Actions.EditCatalog || after.Token == shown.Token ||
				!bytes.Equal(raw, weightsTestRead(t, archive)) {
				t.Fatal("review EOF failed to cancel whole operation without applying candidate")
			}
		})
	}
}
