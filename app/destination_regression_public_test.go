package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
)

func TestDestinationRegressionReadyUpdateAddCannotChooseNewDestination(t *testing.T) {
	for _, mode := range []Mode{Update, Add} {
		t.Run(string(mode), func(t *testing.T) {
			op, archive, source := destinationRegressionReady(t, mode)
			archiveBytes := destinationRegressionRead(t, archive)
			sourceBytes := destinationRegressionRead(t, source)
			preview, issues := op.ReviewSave(op.Snapshot().Token)
			destinationRegressionOK(t, issues)
			if preview == nil {
				t.Fatal("ready operation did not produce save preview")
			}
			destinationRegressionOK(t, op.ConfirmSave(preview.Token, true, true))
			before := op.Snapshot()
			model := destinationRegressionModel(t, before)
			target, saving := op.target, op.saving
			if !before.Actions.Save {
				t.Fatal("setup did not reach confirmed save")
			}
			alternative := filepath.Join(filepath.Dir(archive), "other.ZIP")
			otherBytes := []byte("existing alternative remains untouched")
			destinationRegressionWrite(t, alternative, otherBytes)
			issues = op.SetDestination(before.Token, filepath.Dir(archive), "other.ZIP")
			destinationRegressionCode(t, issues, CodeState)
			after := op.Snapshot()
			if after.Token != before.Token || after.Target == nil || *after.Target != *before.Target ||
				op.target != target || op.saving != saving || !after.Actions.Save ||
				!bytes.Equal(model, destinationRegressionModel(t, after)) {
				t.Fatal("Create-only rejection mutated ready target, model, token or consent")
			}
			destinationRegressionBytes(t, archive, archiveBytes)
			destinationRegressionBytes(t, alternative, otherBytes)
			destinationRegressionBytes(t, source, sourceBytes)
		})
	}
}

func TestDestinationRegressionChangedUpdateAddRequireFreshAnalyze(t *testing.T) {
	for _, mode := range []Mode{Update, Add} {
		t.Run(string(mode), func(t *testing.T) {
			op, archive, source := destinationRegressionReady(t, mode)
			raw := destinationRegressionRead(t, archive)
			input := destinationRegressionRead(t, source)
			before, err := os.Stat(archive)
			if err != nil {
				t.Fatal(err)
			}
			changed := before.ModTime().Add(20 * time.Second)
			if err := os.Chtimes(archive, changed, changed); err != nil {
				t.Fatal(err)
			}
			actual, err := os.Stat(archive)
			if err != nil || actual.ModTime().Equal(before.ModTime()) || !os.SameFile(before, actual) || before.Size() != actual.Size() {
				t.Fatalf("setup did not isolate real modification time change: %v", err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				preview, issues := op.ReviewSave(op.Snapshot().Token)
				if attempt == 0 {
					destinationRegressionCode(t, issues, CodeTargetChanged)
				} else if len(issues) == 0 {
					t.Fatal("second review accepted changed baseline without Analyze")
				}
				if preview != nil || op.Snapshot().Actions.Save {
					t.Fatal("recheck legitimized modified loaded archive")
				}
			}
			result, issues := op.Save(destinationRegressionAt())
			if len(issues) == 0 || result.Installed {
				t.Fatal("Save accepted changed baseline without fresh analysis")
			}
			destinationRegressionBytes(t, archive, raw)
			issues = op.Retry(destinationRegressionAt().Add(time.Hour))
			if mode == Update {
				destinationRegressionOK(t, issues)
			} else {
				destinationRegressionCode(t, issues, catalog.CodeMissingWeight)
			}
			destinationRegressionFinish(t, op, mode)
			preview, issues := op.ReviewSave(op.Snapshot().Token)
			destinationRegressionOK(t, issues)
			if preview == nil || preview.Destination.Path != archive || !preview.Destination.Exists {
				t.Fatal("fresh analysis did not allow a new review of original path")
			}
			destinationRegressionBytes(t, archive, raw)
			destinationRegressionBytes(t, source, input)
			entries, err := os.ReadDir(filepath.Dir(archive))
			if err != nil || len(entries) != 3 {
				t.Fatalf("review/rejection created transaction files: %v, %v", entries, err)
			}
		})
	}
}
