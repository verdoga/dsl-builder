package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/resourceusage"
)

func snapshotFixtureText(value string) *string { return &value }

func snapshotFixtureTime() time.Time {
	return time.Date(2026, 11, 4, 12, 30, 0, 0, time.FixedZone("+03", 3*60*60))
}

func snapshotFixtureOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", issues)
	}
}

func snapshotFixtureLesson(id string) *lesson.Lesson {
	return &lesson.Lesson{FormatVersion: "1.0", Document: lesson.Document{
		ID: id, Title: "Урок " + id, Subtitle: snapshotFixtureText("Подзаголовок"),
		Version: "1", LineCount: "5", CreatedAt: "2026-11-01T10:00:00Z",
		Pages: []lesson.Page{{ID: id + "-p001", Title: snapshotFixtureText("Страница"),
			Nodes: []lesson.Node{{ID: id + "-n001", Type: lesson.NodeText,
				Data: []lesson.Data{{Type: lesson.DataText, Role: func() *lesson.DataRole {
					r := lesson.RoleBody
					return &r
				}(), Value: "Текст", Answers: []string{}, Hint: ""}}, Nodes: []lesson.Node{}}}}},
	}}
}

// snapshotFixtureOperation создаёт проверенный ZIP реальными владельцами.
func snapshotFixtureOperation(t *testing.T, mode Mode, unit, sectioned bool) *Operation {
	t.Helper()
	var files []*module.File
	var items []catalog.Item
	for i, id := range []string{"snap-a", "snap-b"} {
		name := fmt.Sprintf("l%03d.json", i+1)
		file, issues := module.NewFile(name, snapshotFixtureLesson(id), diagnostic.Context{FileName: name})
		snapshotFixtureOK(t, issues)
		files = append(files, file)
		item := catalog.Item{Entry: catalog.EntryFromLesson(file.Lesson().Document, name,
			snapshotFixtureText(fmt.Sprint(i+1))), RelativePath: name,
			Context: diagnostic.Context{Path: filepath.Join(t.TempDir(), name), FileName: name, LessonID: id}}
		if sectioned {
			item.Section = snapshotFixtureText("Секция")
		}
		items = append(items, item)
	}
	unitTitle := ""
	if unit {
		unitTitle = "Юнит"
	}
	state, issues := catalog.Create("Курс", unitTitle, items)
	snapshotFixtureOK(t, issues)
	if mode == Add {
		state, issues = catalog.Create("Курс", unitTitle, items[:1])
		snapshotFixtureOK(t, issues)
	}
	m, issues := catalog.Export(state, "2026-11-01T09:00:00Z", diagnostic.Context{})
	snapshotFixtureOK(t, issues)
	value := &module.Module{Manifest: *m, Files: files}
	if mode == Add {
		value.Files = files[:1]
	}
	var buf bytes.Buffer
	snapshotFixtureOK(t, moduleio.Write(&buf, value, diagnostic.Context{}))
	dir := t.TempDir()
	path := filepath.Join(dir, "исходный.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	baseline, issues := moduleio.Read(path)
	snapshotFixtureOK(t, issues)
	before, issues := catalog.Import(baseline.Manifest, diagnostic.Context{Path: path})
	snapshotFixtureOK(t, issues)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	op := &Operation{mode: mode, baseDir: dir, archivePath: path, at: snapshotFixtureTime(),
		baseline: baseline, beforeCatalog: before, placement: catalog.Clone(before),
		current:  &module.Module{Manifest: baseline.Manifest, Files: append([]*module.File(nil), baseline.Files...)},
		analyzed: true, revisionReady: true, usage: &resourceusage.Registry{},
		target: &targetState{destination: Destination{Directory: dir, FileName: "исходный.zip", Path: path, Exists: true}, info: info}}
	if mode == Create {
		op.archivePath, op.baseline, op.beforeCatalog = "", nil, nil
		op.lessonChanged = true
		op.current.Files = files
		op.converted = files
		op.target = &targetState{destination: Destination{Directory: dir, FileName: "новый.zip", Path: filepath.Join(dir, "новый.zip")}}
	}
	if mode == Add {
		op.placement, issues = catalog.Add(before, items[1:])
		snapshotFixtureOK(t, issues)
		op.current.Files = append(op.current.Files, files[1])
		op.converted = files[1:]
		for _, file := range op.converted {
			op.names = append(op.names, file.FileName())
		}
		op.lessonChanged = true
	}
	if mode == Update {
		op.oldFile, op.newFile, op.revised = baseline.Files[0], baseline.Files[0], baseline.Files[0]
		op.plan, issues = compare.Build(op.oldFile.Lesson(), op.newFile.Lesson(), diagnostic.Context{}, diagnostic.Context{})
		snapshotFixtureOK(t, issues)
	}
	op.key = &reviewKey{owner: op}
	snapshotFixtureOK(t, op.refreshPlan())
	return op
}

func snapshotFixtureCode(t *testing.T, issue *diagnostic.Error, code string) {
	t.Helper()
	if issue == nil || issue.Code != code {
		t.Fatalf("diagnostic = %+v, want %s", issue, code)
	}
}
