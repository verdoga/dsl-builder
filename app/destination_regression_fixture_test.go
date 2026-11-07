package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/convert"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func destinationRegressionAt() time.Time {
	return time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
}

func destinationRegressionOK(t *testing.T, issues []*diagnostic.Error) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("unexpected diagnostics: %v", issues)
	}
}

func destinationRegressionCode(t *testing.T, issues []*diagnostic.Error, code string) {
	t.Helper()
	if len(issues) != 1 || issues[0] == nil || issues[0].Code != code || issues[0].Reason == "" {
		t.Fatalf("diagnostics = %v; want one %s with reason", issues, code)
	}
}

func destinationRegressionRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func destinationRegressionWrite(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func destinationRegressionBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	if !bytes.Equal(destinationRegressionRead(t, path), want) {
		t.Fatalf("unexpected mutation of %s", path)
	}
}

// destinationRegressionSource изменяет JSON до Load; проверенный Input неизменен.
func destinationRegressionSource(t *testing.T, path, id, title string) {
	t.Helper()
	raw := destinationRegressionRead(t, "testdata/destination_regression/source.json")
	var result model.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	result.Document.Metadata.DocumentID = &id
	result.Document.Metadata.Title = &title
	result.Document.ByteLength, result.Document.SHA256 = nil, nil
	result.Lines[1].Raw = "@document-id " + id
	result.Lines[1].Elements[1].Raw = id
	result.Lines[1].Elements[1].Value = &id
	result.Lines[1].Elements[1].End = 14 + len([]rune(id))
	result.Lines[2].Raw = "# " + title
	result.Lines[2].Elements[1].Raw = title
	result.Lines[2].Elements[1].Value = &title
	result.Lines[2].Elements[1].End = 3 + len([]rune(title))
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	destinationRegressionWrite(t, path, raw)
}

// destinationRegressionArchive строит ZIP через владельцев, вне app lifecycle.
func destinationRegressionArchive(t *testing.T, path, source string) []byte {
	t.Helper()
	input, issues := parsed.Load(source)
	destinationRegressionOK(t, issues)
	value, _, issues := convert.Build(input, &filetree.Resolver{}, destinationRegressionAt(), diagnostic.Context{})
	destinationRegressionOK(t, issues)
	file, issues := module.NewFile("l001.json", value, diagnostic.Context{Path: source})
	destinationRegressionOK(t, issues)
	state, issues := catalog.Create("Исходный курс", "", []catalog.Item{{
		Entry: catalog.EntryFromLesson(value.Document, file.FileName(), nil),
	}})
	destinationRegressionOK(t, issues)
	manifest, issues := catalog.Export(state, value.Document.CreatedAt, diagnostic.Context{})
	destinationRegressionOK(t, issues)
	var zip bytes.Buffer
	destinationRegressionOK(t, moduleio.Write(&zip, &module.Module{Manifest: *manifest, Files: []*module.File{file}}, diagnostic.Context{Path: path}))
	destinationRegressionWrite(t, path, zip.Bytes())
	read, issues := moduleio.Read(path)
	destinationRegressionOK(t, issues)
	if read == nil || len(read.Files) != 1 || len(read.Files[0].OriginalBytes()) == 0 {
		t.Fatal("fixture did not complete a real ZIP round trip")
	}
	return zip.Bytes()
}

// destinationRegressionReady даёт готовый публичный Update/Add с изменениями.
func destinationRegressionReady(t *testing.T, mode Mode) (*Operation, string, string) {
	t.Helper()
	base := t.TempDir()
	oldSource := filepath.Join(base, "old.json")
	destinationRegressionSource(t, oldSource, "BA82", "TITLE")
	archive := filepath.Join(base, "existing.ZIP")
	destinationRegressionArchive(t, archive, oldSource)
	source := filepath.Join(base, "incoming.json")
	var op *Operation
	var issues []*diagnostic.Error
	if mode == Update {
		destinationRegressionSource(t, source, "BA82", "UPDATED")
		op, issues = NewUpdate(base, archive, source)
	} else {
		destinationRegressionSource(t, source, "DEST41", "NEW LESSON")
		op, issues = NewAdd(base, archive, source)
	}
	destinationRegressionOK(t, issues)
	issues = op.Analyze(destinationRegressionAt())
	if mode == Update {
		destinationRegressionOK(t, issues)
	} else {
		destinationRegressionCode(t, issues, catalog.CodeMissingWeight)
	}
	destinationRegressionFinish(t, op, mode)
	return op, archive, source
}

func destinationRegressionFinish(t *testing.T, op *Operation, mode Mode) {
	t.Helper()
	if mode == Update {
		for index := range op.Snapshot().FieldAnswered {
			destinationRegressionOK(t, op.DecideField(op.Snapshot().Token, index, true))
		}
		for index := range op.Snapshot().PlacementAnswered {
			destinationRegressionOK(t, op.DecidePlacement(op.Snapshot().Token, index, false))
		}
		destinationRegressionOK(t, op.PrepareRevision(op.Snapshot().Token))
	} else {
		edit, issues := op.ProposeLessonWeight(op.Snapshot().Token, "l002.json", "2")
		destinationRegressionOK(t, issues)
		if edit == nil {
			t.Fatal("no weight proposal")
		}
		destinationRegressionOK(t, op.AcceptEdit(edit.Token))
	}
	snapshot := op.Snapshot()
	archive := snapshot.ArchivePath
	if !snapshot.ManifestReady || !snapshot.Actions.ReviewSave || snapshot.NoChanges || len(snapshot.Blockers) != 0 ||
		snapshot.Target == nil || !snapshot.Target.Exists || snapshot.Target.Path != archive {
		t.Fatalf("not a ready %s operation: ready=%v actions=%+v target=%+v blockers=%v", mode, snapshot.ManifestReady, snapshot.Actions, snapshot.Target, snapshot.Blockers)
	}
}

func destinationRegressionModel(t *testing.T, snapshot Snapshot) []byte {
	t.Helper()
	raw, err := json.Marshal(snapshot.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(snapshot.Module.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, manifest...)
	for _, file := range snapshot.Module.Files {
		lesson, err := json.Marshal(file.Lesson())
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, file.FileName()...)
		raw = append(raw, lesson...)
		raw = append(raw, file.OriginalBytes()...)
	}
	return raw
}
