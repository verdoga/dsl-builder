package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func savePrefixCreate(t *testing.T) (*Operation, string, string) {
	t.Helper()
	base := t.TempDir()
	sourceDir := filepath.Join(base, "источники")
	if err := os.Mkdir(sourceDir, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "source.json")
	savePrefixSource(t, source, "", nil)
	op, issues := NewCreate(base, sourceDir, "Курс", "Юнит")
	savePrefixOK(t, issues)
	savePrefixOK(t, op.Analyze(savePrefixTime()))
	savePrefixOK(t, op.SetDestination(op.Snapshot().Token, base, "Результат"))
	return op, source, filepath.Join(base, "Результат.zip")
}

func savePrefixUpdate(t *testing.T, title string, order *string) (*Operation, string, string) {
	t.Helper()
	base := t.TempDir()
	source := filepath.Join(base, "source.json")
	savePrefixSource(t, source, "", nil)
	archive := savePrefixArchive(t, base, source)
	savePrefixSource(t, source, title, order)
	op, issues := NewUpdate(base, archive, source)
	savePrefixOK(t, issues)
	savePrefixOK(t, op.Analyze(savePrefixTime().Add(time.Hour)))
	return op, source, archive
}

func savePrefixDecide(t *testing.T, op *Operation, accept bool) {
	t.Helper()
	snapshot := op.Snapshot()
	if snapshot.Plan == nil {
		t.Fatal("missing real compare plan")
	}
	for index := range snapshot.Plan.Fields {
		savePrefixOK(t, op.DecideField(op.Snapshot().Token, index, accept))
	}
	for index := range snapshot.PlacementProposals {
		savePrefixOK(t, op.DecidePlacement(op.Snapshot().Token, index, accept))
	}
	savePrefixOK(t, op.PrepareRevision(op.Snapshot().Token))
}

func savePrefixReview(t *testing.T, op *Operation) *SavePreview {
	t.Helper()
	preview, issues := op.ReviewSave(op.Snapshot().Token)
	savePrefixOK(t, issues)
	if preview == nil {
		t.Fatal("nil SavePreview")
	}
	return preview
}

func savePrefixModel(t *testing.T, snapshot Snapshot) []byte {
	t.Helper()
	first, err := json.Marshal(snapshot.Module)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(snapshot.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	return append(first, second...)
}
