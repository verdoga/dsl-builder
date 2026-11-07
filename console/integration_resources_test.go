package console

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/preview"
)

func TestConsoleIntegrationResourcesRealFilesAndCurrentReportScope(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "json")
	media := filepath.Join(dir, "media")
	if err := os.MkdirAll(media, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"used.mp4": "small video payload", "unused.txt": "unused payload"} {
		if err := os.WriteFile(filepath.Join(media, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Construct final technical JSON before parsed.Load; this is not DSL parsing.
	value := model.Result{Format: model.FormatVersion1, Document: model.Document{DSLVersion: integrationTestPointer("1.2"), Encoding: integrationTestPointer("UTF-8"), HasBOM: integrationTestPointer(false), LineCount: integrationTestPointer(1), Metadata: model.DocumentMetadata{DocumentID: integrationTestPointer("MEDIA41"), Title: integrationTestPointer("Media"), Order: integrationTestPointer("1"), ResourceDirs: []string{"media"}}}, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{}, Lines: []model.Line{{Number: 1, LineType: model.LineTypeTag, Raw: "@media video used", LineEnding: model.LineEndingNone, Elements: []model.Element{
		{ElementType: model.ElementTypeTag, Raw: "@media", Value: integrationTestPointer("media"), Start: 1, End: 7, ErrorIDs: []string{}},
		{ElementType: model.ElementTypeMediaType, Raw: "video", Value: integrationTestPointer("video"), Start: 8, End: 13, ErrorIDs: []string{}},
		{ElementType: model.ElementTypeSource, Raw: "used", Value: integrationTestPointer("used"), Start: 14, End: 18, ErrorIDs: []string{}},
	}}}}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "media.json")
	if err = os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, issues := parsed.Load(source)
	integrationTestIssues(t, issues)
	op := integrationTestCreateOperation(t, base, dir, "Курс", "")
	snapshot := op.Snapshot()
	if snapshot.Resources.Used != 1 || snapshot.Resources.Unused != 1 {
		t.Fatalf("real resource report: %+v", snapshot.Resources)
	}
	driver := &integrationTestDriver{t: t, steps: []integrationTestStep{{literal: "1", check: func(s string) {
		integrationTestContains(t, s, preview.UnusedResources(snapshot.Resources), filepath.Join(media, "unused.txt"))
	}}}}
	d := dialog{input: bufio.NewReader(driver), output: &driver.output, baseDir: base, operation: op}
	if err = d.showSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	integrationTestContains(t, driver.output.String(), preview.Resources(snapshot.Resources))
	if err = d.showUnused(snapshot); err != nil {
		t.Fatal(err)
	}
	d.operation.Cancel()
	freshDir := filepath.Join(base, "fresh")
	integrationTestSource(t, freshDir, "FRESH41", "Fresh", "Text", nil, nil)
	fresh := integrationTestCreateOperation(t, base, freshDir, "Fresh", "")
	if fresh.Snapshot().Resources.Total != 0 {
		t.Fatal("resource report leaked from previous operation")
	}
}
