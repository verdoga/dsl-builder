package moduleio

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func TestWriteCompletesFlatZIPPreservingPayloadAndDisplayOrder(t *testing.T) {
	for _, layout := range []string{"flat", "unit-sections"} {
		t.Run(layout, func(t *testing.T) {
			value := WriteTestModel(t)
			if layout == "unit-sections" {
				entries := *value.Manifest.Course.Lessons
				value.Manifest.Course = manifest.Course{Title: "Курс", Unit: &manifest.Unit{
					Title: "Юнит", Lessons: []manifest.Entry{}, Sections: []manifest.Section{
						{Title: "Я", Lessons: []manifest.Entry{entries[0]}},
						{Title: "А", Lessons: []manifest.Entry{entries[1]}},
					},
				}}
			}
			WriteTestValid(t, value)
			before := WriteTestSnapshot(t, value)
			dst := &WriteTestSink{}
			// Несуществующий каталог не мешает записи в переданный stream.
			context := diagnostic.Context{Path: filepath.Join(t.TempDir(), "нет", "кандидат.zip")}
			if problems := Write(dst, value, context); len(problems) != 0 {
				t.Fatal(problems)
			}
			if dst.closed || dst.synced {
				t.Fatal("Write took ownership of caller stream")
			}
			if _, err := os.Stat(filepath.Dir(context.Path)); !os.IsNotExist(err) {
				t.Fatalf("operation path was used for filesystem writes: %v", err)
			}
			entries := WriteTestZIP(t, dst.Bytes())
			wantNames := []string{module.ManifestFileName, "l010.json", "l002.json"}
			if len(entries) != len(wantNames) {
				t.Fatalf("entries=%d want=%d", len(entries), len(wantNames))
			}
			for i, entry := range entries {
				if entry.Name != wantNames[i] || entry.Method != zip.Deflate || !entry.Mode().IsRegular() || strings.ContainsAny(entry.Name, `/\`) {
					t.Fatalf("entry %d: name=%q method=%d mode=%v", i, entry.Name, entry.Method, entry.Mode())
				}
				actual := WriteTestRead(t, entry)
				var expected []byte
				if i == 0 {
					expected = WriteTestJSON(t, value.Manifest, true)
				} else {
					file := value.Files[i-1]
					expected = file.OriginalBytes()
					if expected == nil {
						expected = WriteTestJSON(t, file.Lesson(), true)
					}
				}
				if !bytes.Equal(actual, expected) {
					t.Errorf("uncompressed payload changed for %s", entry.Name)
				}
			}
			if after := WriteTestSnapshot(t, value); after != before {
				t.Fatal("Write mutated source model, bytes, context or Files order")
			}
		})
	}
}

func TestWriteValidatesBeforeFirstByteAndPreservesOwnerDiagnostics(t *testing.T) {
	for _, name := range []string{"nil", "empty", "nil-file", "manifest-schema", "metadata", "duplicate-file", "missing-weight"} {
		t.Run(name, func(t *testing.T) {
			value := WriteTestModel(t)
			switch name {
			case "nil":
				value = nil
			case "empty":
				value.Files = nil
			case "nil-file":
				value.Files[0] = nil
			case "manifest-schema":
				value.Manifest.FormatVersion = "2.0"
			case "metadata":
				(*value.Manifest.Course.Lessons)[1].Title = "Другой заголовок"
			case "duplicate-file":
				value.Files = append(value.Files, value.Files[0])
			case "missing-weight":
				(*value.Manifest.Course.Lessons)[0].Order = nil
			}
			context := diagnostic.Context{Path: "кандидат.zip"}
			want := module.Validate(value, context)
			if len(want) == 0 {
				t.Fatal("test model must be invalid")
			}
			var before string
			if value != nil {
				before = WriteTestSnapshot(t, value)
			}
			dst := &WriteTestSink{}
			got := Write(dst, value, context)
			if dst.calls != 0 || dst.Len() != 0 || dst.closed || dst.synced {
				t.Fatal("invalid model touched destination")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("owner diagnostics changed:\ngot %#v\nwant %#v", got, want)
			}
			if value != nil && before != WriteTestSnapshot(t, value) {
				t.Fatal("invalid model mutated during Write")
			}
		})
	}
}

func TestWriteUsesExistingPathOnlyAsDiagnosticContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "исходный архив.zip")
	original := []byte("existing target must survive Write")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	dst := &WriteTestSink{}
	if problems := Write(dst, WriteTestModel(t), diagnostic.Context{Path: path}); len(problems) != 0 {
		t.Fatal(problems)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatalf("existing path changed: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected filesystem changes: %v, entries=%d", err, len(entries))
	}
	WriteTestZIP(t, dst.Bytes())
}

func TestWriteRejectsNilDestination(t *testing.T) {
	context := diagnostic.Context{Path: "временный.zip"}
	problems := Write(nil, WriteTestModel(t), context)
	if len(problems) == 0 || problems[0] == nil || problems[0].Code != CodeWriter || problems[0].Context != context || problems[0].Reason == "" {
		t.Fatalf("nil destination diagnostics: %#v", problems)
	}
}

func TestWriteContentsLeavesWriterOpenForCaller(t *testing.T) {
	value := WriteTestModel(t)
	var dst bytes.Buffer
	zw := zip.NewWriter(&dst)
	if problems := writeContents(zw, value, diagnostic.Context{}); len(problems) != 0 {
		t.Fatal(problems)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("writeContents closed writer owned by Write: %v", err)
	}
	if entries := WriteTestZIP(t, dst.Bytes()); len(entries) != 3 {
		t.Fatalf("entries=%d", len(entries))
	}
}

func TestWriteEntryWritesAllPreparedJSONWithDeflate(t *testing.T) {
	raw := WriteTestJSON(t, struct{ Body string }{strings.Repeat("Кириллица и точные байты\n", 2000)}, false)
	var dst bytes.Buffer
	zw := zip.NewWriter(&dst)
	context := diagnostic.Context{Path: "кандидат.zip", FileName: "l1000.json"}
	if problem := writeEntry(zw, "l1000.json", raw, context); problem != nil {
		t.Fatal(problem)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	entries := WriteTestZIP(t, dst.Bytes())
	if len(entries) != 1 || entries[0].Name != "l1000.json" || entries[0].Method != zip.Deflate || !entries[0].Mode().IsRegular() {
		t.Fatal("writeEntry did not create one regular Deflate entry")
	}
	if !bytes.Equal(WriteTestRead(t, entries[0]), raw) {
		t.Fatal("prepared JSON was truncated or changed")
	}
}
