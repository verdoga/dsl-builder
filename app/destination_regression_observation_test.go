package app

import (
	"os"
	"testing"
	"time"
)

// TestDestinationRegressionSavingObservationIsCheckedWithUnchangedBaseline
// проверяет закрытый контракт двух наблюдений на синтетическом saveReview.
// Публичный ReviewSave не создаёт такое рассогласование без внешнего изменения;
// здесь сохранённое наблюдение построено реальным Stat, а не поддельным FileInfo.
func TestDestinationRegressionSavingObservationIsCheckedWithUnchangedBaseline(t *testing.T) {
	for _, mode := range []Mode{Update, Add} {
		t.Run(string(mode), func(t *testing.T) {
			op, archive, _ := destinationRegressionReady(t, mode)
			raw := destinationRegressionRead(t, archive)
			preview, issues := op.ReviewSave(op.Snapshot().Token)
			destinationRegressionOK(t, issues)
			if preview == nil {
				t.Fatal("missing public save preview")
			}
			destinationRegressionOK(t, op.ConfirmSave(preview.Token, true, true))
			baseline := op.target
			if baseline == nil || baseline.info == nil || op.saving == nil {
				t.Fatal("missing loaded baseline or confirmed save review")
			}
			at := baseline.info.ModTime()
			otherTime := at.Add(time.Minute)
			if err := os.Chtimes(archive, otherTime, otherTime); err != nil {
				t.Fatal(err)
			}
			stale, issue := inspectDestination(archive)
			if issue != nil || stale == nil || stale.info.ModTime().Equal(at) {
				t.Fatalf("cannot create distinct real saved observation: %v", issue)
			}
			if err := os.Chtimes(archive, at, at); err != nil {
				t.Fatal(err)
			}
			current, err := os.Stat(archive)
			if err != nil || !os.SameFile(baseline.info, current) || baseline.info.Size() != current.Size() || !current.ModTime().Equal(at) {
				t.Fatalf("baseline must be unchanged before Save: %v", err)
			}
			op.saving.target = stale
			result, issues := op.Save(destinationRegressionAt())
			destinationRegressionCode(t, issues, CodeTargetChanged)
			if result.Installed {
				t.Fatal("stale saving.target allowed installation")
			}
			destinationRegressionBytes(t, archive, raw)
			result, issues = op.Save(destinationRegressionAt().Add(time.Hour))
			if result.Installed || len(issues) == 0 {
				t.Fatal("repeated Save accepted stale saved observation")
			}
			destinationRegressionBytes(t, archive, raw)
		})
	}
}
