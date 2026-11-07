package app

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestDestinationPublicRegressionLeadingEdgesPreserveReviewAndConsent
// проверяет DEST-T03 через публичную Create, без доступа к состоянию Operation.
func TestDestinationPublicRegressionLeadingEdgesPreserveReviewAndConsent(t *testing.T) {
	for _, name := range []struct{ label, raw string }{
		{"leading_dot", ".unit"},
		{"leading_space", " unit"},
	} {
		for _, confirmed := range []bool{false, true} {
			stage := "reviewed"
			if confirmed {
				stage = "confirmed"
			}
			t.Run(name.label+"/"+stage, func(t *testing.T) {
				op, source, sourceBytes := destinationCreate(t)
				base := filepath.Dir(filepath.Dir(source))
				archive := filepath.Join(base, "Принятая цель.ZIP")
				archiveBytes := destinationRegressionArchive(t, archive, source)
				destinationRegressionOK(t, op.SetDestination(op.Snapshot().Token, base, filepath.Base(archive)))
				ready := op.Snapshot()
				if ready.Mode != Create || !ready.ManifestReady || ready.NoChanges ||
					!ready.Actions.ReviewSave || len(ready.Blockers) != 0 || ready.Module == nil ||
					len(ready.Module.Files) != 1 || ready.Target == nil || !ready.Target.Exists || ready.Target.Path != archive {
					t.Fatal("setup did not reach a healthy public Create with an existing target")
				}
				preview, issues := op.ReviewSave(ready.Token)
				destinationRegressionOK(t, issues)
				if preview == nil || preview.Destination != *ready.Target || preview.Snapshot.Token != ready.Token {
					t.Fatal("setup did not show the accepted target and current model")
				}
				if confirmed {
					destinationRegressionOK(t, op.ConfirmSave(preview.Token, true, true))
				}
				before := op.Snapshot()
				if before.Actions.Save != confirmed {
					t.Fatal("setup did not reach the requested review/consent stage")
				}
				modelBefore := destinationRegressionModel(t, before)
				snapshotBefore, err := json.Marshal(before)
				if err != nil {
					t.Fatal(err)
				}
				var paths []string
				var infos []os.FileInfo
				if err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					info, err := entry.Info()
					if err != nil {
						return err
					}
					paths = append(paths, path)
					infos = append(infos, info)
					return nil
				}); err != nil {
					t.Fatal(err)
				}

				issues = op.SetDestination(before.Token, base, name.raw)
				destinationRegressionCode(t, issues, CodeArchiveName)
				if !strings.ContainsFunc(issues[0].Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
					t.Fatal("invalid archive name did not return a Russian reason")
				}
				after := op.Snapshot()
				snapshotAfter, err := json.Marshal(after)
				if err != nil {
					t.Fatal(err)
				}
				if after.Token != before.Token || after.Target == nil || *after.Target != *before.Target ||
					after.Actions.Save != confirmed || !bytes.Equal(snapshotBefore, snapshotAfter) ||
					!bytes.Equal(modelBefore, destinationRegressionModel(t, after)) {
					t.Fatal("rejected leading edge changed the accepted model, target, token, blockers or consent")
				}
				if !confirmed {
					destinationRegressionOK(t, op.ConfirmSave(preview.Token, true, true))
					if !op.Snapshot().Actions.Save {
						t.Fatal("the previously shown SavePreview could no longer authorize its original plan")
					}
				}
				destinationAssertBytes(t, source, sourceBytes)
				destinationAssertBytes(t, archive, archiveBytes)
				index := 0
				if err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if index >= len(paths) || path != paths[index] {
						t.Fatalf("destination input/review created or removed filesystem entries at %s", path)
					}
					info, err := entry.Info()
					if err != nil {
						return err
					}
					old := infos[index]
					if !os.SameFile(old, info) || old.Mode() != info.Mode() || old.Size() != info.Size() || !old.ModTime().Equal(info.ModTime()) {
						t.Fatalf("destination input/review changed filesystem metadata at %s", path)
					}
					index++
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if index != len(paths) {
					t.Fatal("destination input/review removed filesystem entries")
				}
			})
		}
	}
}
