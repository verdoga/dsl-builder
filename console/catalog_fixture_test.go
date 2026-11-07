package console

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func catalogTestTime() time.Time         { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }
func catalogTestString(s string) *string { return &s }
func catalogTestOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected owner diagnostics: %v", issues)
	}
}
func catalogTestRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// catalogTestSource меняет технический JSON до его загрузки настоящим app.
func catalogTestSource(t *testing.T, root, name, id, title string, section, weight *string) string {
	t.Helper()
	var value model.Result
	if err := json.Unmarshal(catalogTestRead(t, "testdata/catalog/source.json"), &value); err != nil {
		t.Fatal(err)
	}
	value.Document.Metadata.DocumentID = &id
	value.Document.Metadata.Title = &title
	value.Document.Metadata.Section = section
	value.Document.Metadata.Order = weight
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// catalogTestArchive создаёт реальный проверенный ZIP с непоследовательными именами.
func catalogTestArchive(t *testing.T, root string, sectioned bool) string {
	t.Helper()
	value := &module.Module{}
	entries := make([]manifest.Entry, 0, 2)
	for index, fixture := range []struct{ path, name, weight string }{
		{"testdata/catalog/old-one.json", "l002.json", "10"},
		{"testdata/catalog/old-two.json", "l007.json", "20"},
	} {
		var item lesson.Lesson
		if err := json.Unmarshal(catalogTestRead(t, fixture.path), &item); err != nil {
			t.Fatal(err)
		}
		file, issues := module.NewFile(fixture.name, &item, diagnostic.Context{})
		catalogTestOK(t, issues)
		value.Files = append(value.Files, file)
		entry := catalog.EntryFromLesson(item.Document, fixture.name, &fixture.weight)
		entries = append(entries, entry)
		if index == 1 && item.Document.Pages[0].Nodes[0].Data[0].Role == nil {
			t.Fatal("fixture lost options role")
		}
	}
	sections := []manifest.Section{}
	direct := entries
	if sectioned {
		direct = []manifest.Entry{}
		sections = []manifest.Section{{Title: "Зета", Lessons: entries[:1]}, {Title: "Альфа", Lessons: entries[1:]}}
	}
	value.Manifest = manifest.Manifest{FormatVersion: "1.0", CreatedAt: "2026-11-02T12:00:00Z",
		Course: manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит", Lessons: direct, Sections: sections}}}
	var zip bytes.Buffer
	catalogTestOK(t, moduleio.Write(&zip, value, diagnostic.Context{}))
	path := filepath.Join(root, "исходный.zip")
	if err := os.WriteFile(path, zip.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, issues := moduleio.Read(path)
	catalogTestOK(t, issues)
	if len(loaded.Files) != 2 || len(loaded.Files[0].OriginalBytes()) == 0 {
		t.Fatal("fixture is not a preserved module")
	}
	catalogTestOK(t, moduleio.Verify(path))
	return path
}

func catalogTestDisk(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[relative] = fmt.Sprintf("%x", sha256.Sum256(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func catalogTestUnwritten(t *testing.T, root string, disk map[string]string, op *app.Operation) {
	t.Helper()
	if !maps.Equal(disk, catalogTestDisk(t, root)) {
		t.Fatal("catalog dialog changed source/archive or created a file")
	}
	snap := op.Snapshot()
	if snap.Actions.Save || snap.WriteResult != nil {
		t.Fatal("catalog consent became write consent/result")
	}
}
func catalogTestCatalogEqual(t *testing.T, before, after *catalog.State) {
	t.Helper()
	left, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatalf("accepted catalog changed:\nbefore %s\nafter %s", left, right)
	}
}
func catalogTestFilesEqual(t *testing.T, before, after app.Snapshot) {
	t.Helper()
	if before.Module == nil || after.Module == nil || len(before.Module.Files) != len(after.Module.Files) {
		t.Fatal("file set changed")
	}
	for _, old := range before.Module.Files {
		var found *module.File
		for _, current := range after.Module.Files {
			if old.FileName() == current.FileName() {
				found = current
			}
		}
		if found == nil {
			t.Fatalf("lost %s", old.FileName())
		}
		left, err := json.Marshal(old.Lesson())
		if err != nil {
			t.Fatal(err)
		}
		right, err := json.Marshal(found.Lesson())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, right) || !bytes.Equal(old.OriginalBytes(), found.OriginalBytes()) {
			t.Fatalf("changed lesson data/roles/IDs/version/bytes %s", old.FileName())
		}
		catalogTestOK(t, lesson.Validate(found.Lesson(), found.Context()))
	}
}
func catalogTestItemSection(t *testing.T, snap app.Snapshot, name string) *string {
	t.Helper()
	for _, item := range snap.Catalog.Items {
		if item.Entry.FileName == name {
			return item.Section
		}
	}
	t.Fatalf("missing file %s", name)
	return nil
}
func catalogTestCode(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue != nil && issue.Code == code {
			return
		}
	}
	t.Fatalf("missing %s in %v", code, issues)
}

func TestConsoleCatalogFixtureOwners(t *testing.T) {
	for _, sectioned := range []bool{false, true} {
		root := t.TempDir()
		archive := catalogTestArchive(t, root, sectioned)
		source := catalogTestSource(t, root, "new/source.json", "CT03", "Новый урок", nil, catalogTestString("30"))
		op, issues := app.NewAdd(root, archive, source)
		catalogTestOK(t, issues)
		issues = op.Analyze(catalogTestTime())
		if !sectioned {
			catalogTestOK(t, issues)
		}
		snap := op.Snapshot()
		if !snap.Actions.EditSections || len(snap.Catalog.Items) != 3 || len(snap.NewFileNames) != 1 || snap.NewFileNames[0] != "l008.json" {
			t.Fatalf("invalid Add fixture: %+v", snap)
		}
		op.Cancel()
	}
}
