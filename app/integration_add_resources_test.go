package app_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/app"
)

func TestIntegrationAddResourcesCoverOnlyNewInputsAndUniquePaths(t *testing.T) {
	base := t.TempDir()
	archive, oldRaw := integrationArchive(t, base)
	source := filepath.Join(base, "source")
	integrationWrite(t, filepath.Join(base, "media", "nested", "Clip.mp3"), []byte{0, 1, 2, 255})
	integrationWrite(t, filepath.Join(base, "media", "unused.bin"), []byte{9})
	for index, id := range []string{"NEW-MEDIA1", "NEW-MEDIA2"} {
		input := integrationMediaInput(id)
		input.Document.Metadata.Order = integrationText(fmt.Sprint(30 + index*10))
		integrationWriteInput(t, filepath.Join(source, fmt.Sprintf("%d.json", index)), input)
	}
	op, issues := app.NewAdd(base, archive, source)
	integrationOK(t, issues)
	integrationOK(t, op.Analyze(integrationAt()))
	snapshot := op.Snapshot()
	if len(snapshot.NewFileNames) != 2 || snapshot.Resources.Total != 2 || snapshot.Resources.Used != 1 || snapshot.Resources.Unused != 1 {
		t.Fatalf("incorrect Add resource scope/union: %+v", snapshot.Resources)
	}
	value := integrationSave(t, op, true)
	for name, raw := range oldRaw {
		if !bytes.Equal(integrationFile(t, value, name).OriginalBytes(), raw) {
			t.Fatalf("old lesson changed: %s", name)
		}
	}
	for _, name := range snapshot.NewFileNames {
		if integrationFile(t, value, name).Lesson().Document.Version != "1" {
			t.Fatal("Add changed initial revision")
		}
	}
}
