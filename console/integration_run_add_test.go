package console

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
)

func TestConsoleIntegrationAddPublicFileAndDirectoryPreservesOldBytes(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			base := t.TempDir()
			old := filepath.Join(base, "old")
			integrationTestSource(t, old, "OLD41", "Старый", "Old source", nil, integrationTestPointer("1"))
			archive := integrationTestSeed(t, base, old, "add.zip")
			archiveBytes := integrationTestBytes(t, archive)
			oldBytes := integrationTestRead(t, archive).Files[0].OriginalBytes()
			dir := filepath.Join(base, "new")
			source := integrationTestSource(t, dir, "NEW41", "Новый", "New source", nil, integrationTestPointer("2"))
			sourceBytes := integrationTestBytes(t, source)
			count := 2
			var nextSource string
			var nextBytes []byte
			if directory {
				nextSource = integrationTestSource(t, dir, "NEXT41", "Следующий", "Next", nil, integrationTestPointer("3"))
				nextBytes = integrationTestBytes(t, nextSource)
				source = dir
				count = 3
			}
			owner, issues := app.NewAdd(base, archive, source)
			integrationTestIssues(t, issues)
			integrationTestIssues(t, owner.Analyze(time.Now()))
			expected := integrationTestCompositionBlocks(t, owner.Snapshot())
			steps := []integrationTestStep{integrationTestNumber(3, "Добав")}
			steps = integrationTestAppendValue(steps, archive, source)
			steps = append(steps, integrationTestMenu("(запис|сохран).*"), integrationTestStep{literal: "1", check: func(s string) {
				integrationTestContains(t, s, expected...)
				integrationTestContains(t, s, archive, `"l001.json" | "Старый" | order: "1"`, `"l002.json" | "Новый" | order: "2"`, `НОВЫЙ | ID: "NEW41"`)
				if directory {
					integrationTestContains(t, s, `"l003.json" | "Следующий" | order: "3"`, `НОВЫЙ | ID: "NEXT41"`)
				}
				if strings.Count(s, "НОВЫЙ") != count-1 || strings.Contains(s, `ID: "OLD41"`) {
					t.Fatal("addition markers or stable IDs were assigned to the wrong rows")
				}
				if !bytes.Equal(archiveBytes, integrationTestBytes(t, archive)) {
					t.Fatal("archive changed before save consent")
				}
			}}, integrationTestStep{literal: "1", check: func(s string) {
				integrationTestContains(t, s, archive, "Перезапис")
				if !bytes.Equal(archiveBytes, integrationTestBytes(t, archive)) {
					t.Fatal("archive changed before overwrite consent")
				}
			}}, integrationTestMenu("Выход|Выйти"))
			integrationTestRun(t, base, steps)
			result := integrationTestRead(t, archive)
			if len(result.Files) != count {
				t.Fatalf("files: %d", len(result.Files))
			}
			found := false
			wantIDs := map[string]string{"l001.json": "OLD41", "l002.json": "NEW41"}
			if directory {
				wantIDs["l003.json"] = "NEXT41"
			}
			for _, file := range result.Files {
				if want, exists := wantIDs[file.FileName()]; !exists || file.Lesson().Document.ID != want {
					t.Fatalf("saved technical name/ID: %s/%s", file.FileName(), file.Lesson().Document.ID)
				}
				delete(wantIDs, file.FileName())
				if file.Lesson().Document.ID == "OLD41" {
					found = true
					if !bytes.Equal(oldBytes, file.OriginalBytes()) {
						t.Fatal("old lesson bytes changed")
					}
				}
			}
			if !found || len(wantIDs) != 0 {
				t.Fatal("old lesson disappeared")
			}
			if !bytes.Equal(sourceBytes, integrationTestBytes(t, filepath.Join(dir, "NEW41.json"))) {
				t.Fatal("new source changed")
			}
			if directory && !bytes.Equal(nextBytes, integrationTestBytes(t, nextSource)) {
				t.Fatal("next source changed")
			}
		})
	}
}
