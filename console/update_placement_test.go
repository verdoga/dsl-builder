package console

import (
	"bytes"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/preview"
)

func TestConsoleUpdatePlacementShowsOneChangeAndAnswersOne(t *testing.T) {
	for _, answer := range []string{"1\n", "2\n"} {
		t.Run(answer[:1], func(t *testing.T) {
			before, after := consoleUpdateTestPlain(), consoleUpdateTestPlain()
			after.Document.Metadata.Section = consoleUpdateTestText("Beta")
			after.Document.Metadata.Order = consoleUpdateTestText("030")
			op := consoleUpdateTestOperation(t, before, after, true)
			snapshot := op.Snapshot()
			if len(snapshot.PlacementProposals) != 2 || snapshot.Actions.EditCatalog || snapshot.Actions.EditSections {
				t.Fatal("fixture needs two pending proposals before manual edits")
			}
			var out bytes.Buffer
			d := consoleUpdateTestDialog(op, answer+"sentinel\n", &out)
			if err := d.decidePlacement(snapshot); err != nil {
				t.Fatal(err)
			}
			want, err := preview.ManifestChanges(snapshot.PlacementProposals[:1])
			if err != nil {
				t.Fatal(err)
			}
			text := out.String()
			position := strings.Index(text, want)
			if position < 0 || strings.Count(text, "Поле:") != 1 ||
				strings.Index(text, "1. Да\n") < position+len(want) || strings.Index(text, "2. Нет\n") < position+len(want) {
				t.Fatalf("one complete proposal must precede its own question: %q", text)
			}
			got := op.Snapshot()
			if !got.PlacementAnswered[0] || got.PlacementAnswered[1] || got.Actions.EditCatalog || got.Actions.EditSections {
				t.Fatal("first placement decision answered second or unlocked manual edits")
			}
			for _, item := range got.Catalog.Items {
				if item.Entry.FileName != "l001.json" {
					continue
				}
				if item.Entry.Order == nil || *item.Entry.Order != "010" || item.Section == nil ||
					(*item.Section == "Beta") != (answer == "1\n") {
					t.Fatal("section answer implicitly changed order or ignored rejection")
				}
			}
			if rest, _ := d.input.ReadString('\n'); rest != "sentinel\n" {
				t.Fatal("placement helper consumed more than one answer")
			}
			// Перед вторым решением необходим новый снимок, даже при отказе.
			out.Reset()
			d = consoleUpdateTestDialog(op, "2\n", &out)
			if err := d.decidePlacement(got); err != nil {
				t.Fatal(err)
			}
			want, err = preview.ManifestChanges(got.PlacementProposals[1:2])
			if err != nil || !strings.Contains(out.String(), want) || strings.Count(out.String(), "Поле:") != 1 {
				t.Fatalf("second helper did not show only pending order: %v %q", err, out.String())
			}
			if !op.Snapshot().PlacementAnswered[1] || !op.Snapshot().Actions.EditCatalog {
				t.Fatal("resolved placement did not restore owner edit permission")
			}
		})
	}
}

func TestConsoleUpdatePlacementUsesAnsweredRatherThanChangedCatalog(t *testing.T) {
	before, after := consoleUpdateTestPlain(), consoleUpdateTestPlain()
	after.Document.Metadata.Section, after.Document.Metadata.Order = consoleUpdateTestText("Beta"), consoleUpdateTestText("030")
	op := consoleUpdateTestOperation(t, before, after, true)
	first := op.Snapshot()
	consoleUpdateTestOK(t, op.DecidePlacement(first.Token, 0, false))
	snapshot := op.Snapshot()
	var out bytes.Buffer
	if err := consoleUpdateTestDialog(op, "1\n", &out).decidePlacement(snapshot); err != nil {
		t.Fatal(err)
	}
	want, err := preview.ManifestChanges([]catalog.Change{snapshot.PlacementProposals[1]})
	if err != nil || !strings.Contains(out.String(), want) {
		t.Fatalf("rejected section was asked again: %v %q", err, out.String())
	}
	for _, item := range op.Snapshot().Catalog.Items {
		if item.Entry.FileName == "l001.json" && (item.Section == nil || *item.Section != "Alpha" || *item.Entry.Order != "030") {
			t.Fatal("independent order decision did not preserve rejected section")
		}
	}
}
