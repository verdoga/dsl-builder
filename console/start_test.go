package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func startTestSource(t *testing.T, root, name, id string, change func(*model.Result)) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "golden", "2. parsed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value model.Result
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	value.Document.Metadata.DocumentID = &id
	if change != nil {
		change(&value)
	}
	raw, err = json.MarshalIndent(value, "", "  ")
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
	if input, issues := parsed.Load(path); input == nil || len(issues) != 0 {
		t.Fatalf("fixture parsed.Load: %v", issues)
	}
	return path
}

func startTestArchive(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "golden", "2. lesson.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	file, issues := module.NewFile("l001.json", &value, diagnostic.Context{})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	state, issues := catalog.Create("Исходный курс", "", []catalog.Item{{
		Entry: catalog.EntryFromLesson(value.Document, file.FileName(), nil),
	}})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	manifest, issues := catalog.Export(state, "2026-11-03T17:32:07Z", diagnostic.Context{})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	var zip bytes.Buffer
	if issues := moduleio.Write(&zip, &module.Module{Manifest: *manifest, Files: []*module.File{file}}, diagnostic.Context{}); len(issues) != 0 {
		t.Fatal(issues)
	}
	path := filepath.Join(root, "исходный модуль.zip")
	if err := os.WriteFile(path, zip.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if issues := moduleio.Verify(path); len(issues) != 0 {
		t.Fatal(issues)
	}
	return path
}

func startTestAnswers(values ...string) string {
	var input strings.Builder
	for _, value := range values {
		input.WriteString("1\n")
		input.WriteString(value)
		input.WriteByte('\n')
	}
	return input.String()
}

func startTestUnchanged(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(raw, after) {
				t.Errorf("исходник изменён: %s (%v)", path, err)
			}
		})
	}
}

type startTestWriteFunc func([]byte) (int, error)

func (write startTestWriteFunc) Write(value []byte) (int, error) { return write(value) }

type startTestReadFunc func([]byte) (int, error)

func (read startTestReadFunc) Read(value []byte) (int, error) { return read(value) }

func TestStartInitialGroupsUseRealOwnersAndExplicitBase(t *testing.T) {
	for _, mode := range []string{"create", "update", "add-file", "add-directory"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			archive := startTestArchive(t, base)
			id := "NEW2"
			if mode == "update" {
				id = "BA82"
			}
			source := startTestSource(t, base, "входы/урок.JSON", id, func(value *model.Result) {
				order := "2"
				value.Document.Metadata.Order = &order
			})
			startTestUnchanged(t, source, archive)
			values := []string{`"исходный модуль.zip"`, `"входы/урок.JSON"`}
			if mode == "create" {
				values = []string{`"входы"`, "Курс Ё", "Юнит Ж"}
			} else if mode == "add-directory" {
				values[1] = `"входы"`
			}
			var output bytes.Buffer
			d := dialog{input: bufio.NewReader(strings.NewReader(startTestAnswers(values...))), output: &output, baseDir: base}
			// Выбранные методы возвращают настоящий app.Operation; анализ выполняется отдельно.
			start := d.startCreate
			if mode == "update" {
				start = d.startUpdate
			} else if strings.HasPrefix(mode, "add") {
				start = d.startAdd
			}
			operation, err := start()
			if err != nil || operation == nil {
				t.Fatalf("start: operation=%v err=%v output=%s", operation, err, &output)
			}
			before := operation.Snapshot()
			if before.Catalog != nil || before.Module != nil || before.Actions.Retry {
				t.Fatalf("первичный ввод запустил анализ: %+v", before)
			}
			d.operation = operation
			if err := d.analyze(); err != nil {
				t.Fatal(err)
			}
			snapshot := operation.Snapshot()
			if len(snapshot.Blockers) != 0 || snapshot.Catalog == nil || snapshot.Module == nil {
				t.Fatalf("Analyze: %+v output=%s", snapshot.Blockers, &output)
			}
			if snapshot.Actions.Save || snapshot.WriteResult != nil {
				t.Fatal("анализ разрешил запись без согласия")
			}
			if mode == "create" {
				if snapshot.Catalog.CourseTitle != "Курс Ё" || snapshot.Catalog.UnitTitle == nil || *snapshot.Catalog.UnitTitle != "Юнит Ж" {
					t.Fatalf("потеряны начальные названия: %+v", snapshot.Catalog)
				}
			} else if snapshot.ArchivePath != archive {
				t.Fatalf("путь ZIP=%q, нужен %q", snapshot.ArchivePath, archive)
			}
			operation.Cancel()
		})
	}
}
