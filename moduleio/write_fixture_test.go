package moduleio

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func WriteTestModel(t *testing.T) *module.Module {
	t.Helper()
	raw, err := os.ReadFile("testdata/write/loaded.lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	source := diagnostic.Context{Path: "исходный архив.zip", Line: 37, Column: 4}
	if !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("invalid fixture encoding")
	}
	// Полное чтение реального ZIP устанавливает предусловие LoadedFile.
	var original bytes.Buffer
	zw := zip.NewWriter(&original)
	w, err := zw.Create("l010.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	entries := WriteTestZIP(t, original.Bytes())
	raw = WriteTestRead(t, entries[0])
	if problems := lesson.ValidateJSON(raw, source); len(problems) != 0 {
		t.Fatal(problems)
	}
	var loaded lesson.Lesson
	if err = json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	first, problems := module.LoadedFile("l010.json", &loaded, raw, source)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	newLesson := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: "WRBB", Version: "1e2", Title: "Новый урок", Subtitle: new(string),
		LineCount: "900719925474099312345", CreatedAt: "2026-01-02T03:04:05Z",
		Pages: []lesson.Page{{ID: "wrbb-p002", Nodes: []lesson.Node{}}, {ID: "wrbb-p001", Nodes: []lesson.Node{}}},
	}}
	second, problems := module.NewFile("l002.json", newLesson, diagnostic.Context{Path: "новый урок.json", Line: 8})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	entriesManifest := []manifest.Entry{WriteTestEntry(t, second, "0001"), WriteTestEntry(t, first, "999999999999999999999999")}
	sections := []manifest.Section{}
	value := &module.Module{Files: []*module.File{first, second}, Manifest: manifest.Manifest{
		FormatVersion: manifest.FormatVersion, CreatedAt: "2026-11-04T09:00:00Z",
		Course: manifest.Course{Title: "Курс", Lessons: &entriesManifest, Sections: &sections},
	}}
	WriteTestValid(t, value)
	return value
}

func WriteTestEntry(t *testing.T, file *module.File, order string) manifest.Entry {
	t.Helper()
	document := file.Lesson().Document
	return manifest.Entry{ID: document.ID, FileName: file.FileName(), Order: &order,
		Title: document.Title, Subtitle: document.Subtitle, LineCount: document.LineCount,
		CreatedAt: document.CreatedAt, Version: document.Version}
}

func WriteTestValid(t *testing.T, value *module.Module) {
	t.Helper()
	if problems := module.Validate(value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatal(problems)
	}
}

func WriteTestZIP(t *testing.T, raw []byte) []*zip.File {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("unfinished ZIP: %v", err)
	}
	return r.File
}

func WriteTestRead(t *testing.T, file *zip.File) []byte {
	t.Helper()
	r, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}
	raw, readErr := io.ReadAll(r)
	closeErr := r.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("entry read=%v close=%v", readErr, closeErr)
	}
	return raw
}

func WriteTestJSON(t *testing.T, value any, indent bool) []byte {
	t.Helper()
	var raw []byte
	var err error
	if indent {
		raw, err = json.MarshalIndent(value, "", "  ")
	} else {
		raw, err = json.Marshal(value)
	}
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func WriteTestSnapshot(t *testing.T, value *module.Module) string {
	t.Helper()
	var result bytes.Buffer
	result.Write(WriteTestJSON(t, value.Manifest, false))
	for _, file := range value.Files {
		fmt.Fprintf(&result, "%p:%s:%+v:", file, file.FileName(), file.Context())
		result.Write(WriteTestJSON(t, file.Lesson(), false))
		result.Write(file.OriginalBytes())
	}
	return result.String()
}

func WriteTestLargeModel(t *testing.T) *module.Module {
	t.Helper()
	value := WriteTestModel(t)
	var body bytes.Buffer
	body.WriteString("SECRET_BODY_WRITE")
	for i := 0; i < 5000; i++ {
		digest := sha256.Sum256([]byte(fmt.Sprint(i)))
		body.WriteString(hex.EncodeToString(digest[:]))
	}
	model := value.Files[1].Lesson()
	role := lesson.RoleBody
	model.Document.Pages[0].Nodes = []lesson.Node{{ID: "wrbb-n001", Type: lesson.NodeText,
		Data: []lesson.Data{{Type: lesson.DataText, Role: &role, Value: body.String(), Answers: []string{}}}, Nodes: []lesson.Node{}}}
	file, problems := module.NewFile(value.Files[1].FileName(), model, value.Files[1].Context())
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	value.Files[1] = file
	WriteTestValid(t, value)
	return value
}
