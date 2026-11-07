package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestSaveReviewRejectsIncompleteStatesWithoutWriting(t *testing.T) {
	for _, name := range []string{"unanalysed", "no destination", "fields pending", "revision pending", "placement pending", "active edit", "catalog blockers"} {
		t.Run(name, func(t *testing.T) {
			var op *Operation
			var target string
			switch name {
			case "unanalysed":
				base := t.TempDir()
				var issues []*diagnostic.Error
				op, issues = NewCreate(base, base, "Курс", "")
				savePrefixOK(t, issues)
				target = filepath.Join(base, "result.zip")
			case "no destination":
				base := t.TempDir()
				savePrefixSource(t, filepath.Join(base, "source.json"), "", nil)
				var issues []*diagnostic.Error
				op, issues = NewCreate(base, base, "Курс", "Юнит")
				savePrefixOK(t, issues)
				savePrefixOK(t, op.Analyze(savePrefixTime()))
				snapshot := op.Snapshot()
				if snapshot.Target != nil || !snapshot.ManifestReady || snapshot.Module == nil || len(snapshot.Module.Files) != 1 || len(snapshot.Blockers) != 0 {
					t.Fatal("fixture must be ready except for missing destination")
				}
				target = filepath.Join(base, "Результат.zip")
			case "fields pending":
				op, _, target = savePrefixUpdate(t, "Другое название", nil)
			case "revision pending":
				op, _, target = savePrefixUpdate(t, "Другое название", nil)
				for index := range op.Snapshot().Plan.Fields {
					savePrefixOK(t, op.DecideField(op.Snapshot().Token, index, true))
				}
			case "placement pending":
				weight := "2"
				op, _, target = savePrefixUpdate(t, "", &weight)
				savePrefixOK(t, op.PrepareRevision(op.Snapshot().Token))
			case "active edit":
				op, _, target = savePrefixCreate(t)
				edit, issues := op.ProposeCourseTitle(op.Snapshot().Token, "Кандидат")
				savePrefixOK(t, issues)
				if edit == nil {
					t.Fatal("no edit staged")
				}
			case "catalog blockers":
				base := t.TempDir()
				source := filepath.Join(base, "old.json")
				savePrefixSource(t, source, "", nil)
				target = savePrefixArchive(t, base, source)
				added := filepath.Join(base, "new.json")
				raw := bytes.ReplaceAll(savePrefixRead(t, source), []byte("BA82"), []byte("ZZ99"))
				savePrefixWrite(t, added, raw)
				var issues []*diagnostic.Error
				op, issues = NewAdd(base, target, added)
				savePrefixOK(t, issues)
				issues = op.Analyze(savePrefixTime())
				if len(issues) == 0 || op.Snapshot().Catalog == nil || len(op.Snapshot().Blockers) == 0 {
					t.Fatal("fixture did not reach editable blockers")
				}
			}
			original, err := os.ReadFile(target)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			entriesBefore, err := os.ReadDir(filepath.Dir(target))
			if err != nil {
				t.Fatal(err)
			}
			preview, issues := op.ReviewSave(op.Snapshot().Token)
			if preview != nil || len(issues) == 0 {
				t.Fatal("incomplete state produced final preview")
			}
			if name == "no destination" {
				savePrefixCode(t, issues, CodeDestination)
			}
			_, issues = op.Save(savePrefixTime())
			if len(issues) == 0 {
				t.Fatal("incomplete state wrote ZIP")
			}
			if name == "no destination" {
				savePrefixCode(t, issues, CodeUnconfirmed)
			}
			entriesAfter, err := os.ReadDir(filepath.Dir(target))
			if err != nil {
				t.Fatal(err)
			}
			if len(entriesBefore) != len(entriesAfter) {
				t.Fatal("review/save guard created a file")
			}
			for index := range entriesBefore {
				if entriesBefore[index].Name() != entriesAfter[index].Name() {
					t.Fatal("directory changed")
				}
			}
			if original == nil {
				savePrefixAbsent(t, target)
			} else if !bytes.Equal(original, savePrefixRead(t, target)) {
				t.Fatal("guard changed target bytes")
			}
		})
	}
}

func TestSaveReviewRejectsStaleAcceptedToken(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	old := op.Snapshot().Token
	savePrefixOK(t, op.SetDestination(old, op.Snapshot().Target.Directory, "Новая цель"))
	preview, issues := op.ReviewSave(old)
	if preview != nil {
		t.Fatal("stale review produced save proposal")
	}
	savePrefixCode(t, issues, CodeStaleReview)
	savePrefixAbsent(t, target)
}

func TestSaveBaselineChangeCannotBeLegitimizedByReview(t *testing.T) {
	for _, kind := range []string{"identity", "size", "mtime"} {
		t.Run(kind, func(t *testing.T) {
			op, _, archive := savePrefixUpdate(t, "Новое название", nil)
			savePrefixDecide(t, op, true)
			original := savePrefixRead(t, archive)
			info, err := os.Stat(archive)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "identity":
				if err := os.Rename(archive, archive+".held"); err != nil {
					t.Fatal(err)
				}
				savePrefixWrite(t, archive, original)
				if err := os.Chtimes(archive, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "size":
				savePrefixWrite(t, archive, append(original, []byte("external append")...))
			case "mtime":
				changed := info.ModTime().Add(24 * time.Hour)
				if err := os.Chtimes(archive, changed, changed); err != nil {
					t.Fatal(err)
				}
			}
			external := savePrefixRead(t, archive)
			preview, issues := op.ReviewSave(op.Snapshot().Token)
			if preview != nil {
				t.Fatal("new review legitimized changed baseline")
			}
			savePrefixCode(t, issues, CodeTargetChanged)
			snapshot := op.Snapshot()
			savePrefixCode(t, snapshot.Blockers, CodeTargetChanged)
			if !snapshot.Actions.Retry || snapshot.Actions.ReviewSave || snapshot.Actions.Save {
				t.Fatal("changed baseline must require a new analysis")
			}
			preview, issues = op.ReviewSave(snapshot.Token)
			if preview != nil || len(issues) == 0 {
				t.Fatal("repeated review bypassed required analysis")
			}
			result, issues := op.Save(savePrefixTime())
			if len(issues) == 0 || result.Installed || result.TargetPath != "" || result.TemporaryPath != "" || result.OriginalPath != "" || op.Snapshot().WriteResult != nil {
				t.Fatal("changed baseline permitted a write attempt")
			}
			if !bytes.Equal(external, savePrefixRead(t, archive)) {
				t.Fatal("review touched externally changed target")
			}
		})
	}
}

func TestSaveTargetChangeAfterConsentRevokesConsent(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	preview := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
	external := []byte("external file")
	savePrefixWrite(t, target, external)
	result, issues := op.Save(savePrefixTime())
	savePrefixCode(t, issues, CodeTargetChanged)
	if result.Installed || !bytes.Equal(external, savePrefixRead(t, target)) {
		t.Fatal("stale consent overwrote target")
	}
	if len(op.ConfirmSave(preview.Token, true, true)) == 0 {
		t.Fatal("changed target retained save token")
	}
	savePrefixOK(t, op.SetDestination(op.Snapshot().Token, filepath.Dir(target), "Результат"))
	fresh := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(fresh.Token, true, true))
	result, issues = op.Save(savePrefixTime())
	savePrefixOK(t, issues)
	if !result.Installed {
		t.Fatal("fresh observation and consent failed")
	}
}

func TestSavePreinstallValidationFailureRequiresFreshConsent(t *testing.T) {
	op, _, target := savePrefixCreate(t)
	preview := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(preview.Token, true, false))
	result, issues := op.Save(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))
	if result.Installed || len(issues) == 0 {
		t.Fatal("invalid manifest time was installed")
	}
	owner := false
	for _, issue := range issues {
		if issue != nil && strings.HasPrefix(issue.Code, "manifest.") {
			owner = true
		}
	}
	if !owner {
		t.Fatalf("owner error lost: %v", issues)
	}
	savePrefixAbsent(t, target)
	_, issues = op.Save(savePrefixTime())
	if len(issues) == 0 {
		t.Fatal("failed attempt retained consent")
	}
	fresh := savePrefixReview(t, op)
	savePrefixOK(t, op.ConfirmSave(fresh.Token, true, false))
	result, issues = op.Save(savePrefixTime())
	savePrefixOK(t, issues)
	if !result.Installed {
		t.Fatal("fresh consent did not recover from preinstall error")
	}
}
