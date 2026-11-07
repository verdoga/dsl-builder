package app_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/module"
)

func TestIntegrationAddFileAndDirectoryRequirePlacementThenPreserveOldBytes(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("%d_new", count), func(t *testing.T) {
			base := t.TempDir()
			archive, oldRaw := integrationArchive(t, base)
			before := integrationRead(t, archive)
			source := filepath.Join(base, "new")
			for index := 0; index < count; index++ {
				integrationWriteInput(t, filepath.Join(source, fmt.Sprintf("deep/%d/lesson.JSON", index)), integrationInput(fmt.Sprintf("NEW%d", index), []string{"new body"}))
			}
			if count == 1 {
				source = filepath.Join(source, "deep/0/lesson.JSON")
			}
			op, issues := app.NewAdd(base, archive, source)
			integrationOK(t, issues)
			issues = op.Analyze(integrationAt())
			integrationCode(t, issues, catalog.CodeMissingWeight)
			snapshot := op.Snapshot()
			if snapshot.Catalog == nil || snapshot.Module == nil || len(snapshot.Module.Files) != 2+count || len(snapshot.NewFileNames) != count || snapshot.Actions.ReviewSave {
				t.Fatalf("bad add preview: %+v", snapshot)
			}
			for index, name := range snapshot.NewFileNames {
				if name != fmt.Sprintf("l%03d.json", index+4) {
					t.Fatalf("filled numbering gap: %v", snapshot.NewFileNames)
				}
			}
			edit, issues := op.ProposeLessonWeight(snapshot.Token, "l004.json", "010")
			integrationCode(t, issues, catalog.CodeWeightConflict)
			if edit == nil || len(edit.Blockers) == 0 {
				t.Fatal("conflicted candidate unavailable")
			}
			integrationCode(t, op.AcceptEdit(edit.Token), catalog.CodeWeightConflict)
			weights := map[string]string{"l001.json": "10", "l003.json": "20"}
			for index, name := range op.Snapshot().NewFileNames {
				weights[name] = fmt.Sprint(30 + index*10)
			}
			edit, issues = op.ProposeLessonWeights(op.Snapshot().Token, nil, weights)
			integrationOK(t, issues)
			integrationOK(t, op.AcceptEdit(edit.Token))
			edit, issues = op.ProposeSection(op.Snapshot().Token, "  New\t Section  ")
			integrationCode(t, issues, catalog.CodeUnassigned)
			if edit == nil || len(edit.Catalog.Sections) != 1 || edit.Catalog.Sections[0].Title != "New Section" {
				t.Fatal("section normalization not delegated")
			}
			integrationCode(t, op.AcceptEdit(edit.Token), catalog.CodeUnassigned)
			if op.Snapshot().Actions.ReviewSave {
				t.Fatal("empty section allowed save")
			}
			names := []string{"l001.json", "l003.json"}
			names = append(names, op.Snapshot().NewFileNames...)
			section := "New Section"
			for index, name := range names {
				edit, issues = op.ProposeMove(op.Snapshot().Token, name, &section)
				if edit == nil {
					t.Fatalf("move %s has nil candidate: %v", name, issues)
				}
				issues = op.AcceptEdit(edit.Token)
				if index == len(names)-1 {
					integrationOK(t, issues)
				} else {
					integrationCode(t, issues, catalog.CodeUnassigned)
				}
			}
			snapshot = op.Snapshot()
			if len(snapshot.Blockers) != 0 || !snapshot.ManifestReady || !snapshot.Actions.ReviewSave {
				t.Fatalf("completed distribution not ready: %+v", snapshot)
			}
			integrationSameBytes(t, archive, before)
			value := integrationSave(t, op, true)
			for name, raw := range oldRaw {
				if !bytes.Equal(integrationFile(t, value, name).OriginalBytes(), raw) {
					t.Fatalf("old %s reserialized", name)
				}
			}
			for _, name := range snapshot.NewFileNames {
				if integrationFile(t, value, name).Lesson().Document.Version != "1" {
					t.Fatal("new lesson not revision 1")
				}
			}
			if len(*value.Manifest.Course.Lessons) != 0 || len(*value.Manifest.Course.Sections) != 1 {
				t.Fatal("final section shape incorrect")
			}
		})
	}
}

func TestIntegrationAddDuplicateIDAndBadMemberAreAllOrNothing(t *testing.T) {
	for _, scenario := range []string{"existing_id", "duplicate_new_ids", "invalid_json"} {
		t.Run(scenario, func(t *testing.T) {
			base := t.TempDir()
			archive, _ := integrationArchive(t, base)
			before := integrationRead(t, archive)
			source := filepath.Join(base, "incoming")
			integrationWriteInput(t, filepath.Join(source, "a.json"), integrationInput("NEW1", []string{"body"}))
			id := "old1"
			if scenario == "duplicate_new_ids" {
				id = "new1"
			}
			integrationWriteInput(t, filepath.Join(source, "b.json"), integrationInput(id, []string{"body"}))
			if scenario == "invalid_json" {
				integrationWrite(t, filepath.Join(source, "b.json"), []byte("{"))
			}
			op, issues := app.NewAdd(base, archive, source)
			integrationOK(t, issues)
			issues = op.Analyze(integrationAt())
			switch scenario {
			case "existing_id":
				integrationCode(t, issues, module.CodeExistingID)
			case "duplicate_new_ids":
				integrationCode(t, issues, module.CodeDuplicateID)
			default:
				if len(issues) == 0 {
					t.Fatal("bad member ignored")
				}
			}
			if op.Snapshot().Actions.ReviewSave || op.Snapshot().Actions.Save {
				t.Fatal("partial addition can be saved")
			}
			_, issues = op.Save(integrationAt())
			if len(issues) == 0 {
				t.Fatal("invalid add saved")
			}
			integrationSameBytes(t, archive, before)
		})
	}
}
