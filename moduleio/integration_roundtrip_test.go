package moduleio_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
)

func IntegrationWrite(t *testing.T, value *module.Module) []byte {
	t.Helper()
	var dst bytes.Buffer
	if problems := moduleio.Write(&dst, value, diagnostic.Context{Path: "candidate.zip"}); len(problems) != 0 {
		t.Fatal(problems)
	}
	return dst.Bytes()
}

func IntegrationRead(t *testing.T, path string) *module.Module {
	t.Helper()
	value, problems := moduleio.Read(path)
	if len(problems) != 0 || value == nil {
		t.Fatalf("Read: model=%v errors=%v", value, problems)
	}
	if problems := moduleio.Verify(path); len(problems) != 0 {
		t.Fatalf("Verify: %v", problems)
	}
	return value
}

func TestIntegrationCreateReadWriteVerify(t *testing.T) {
	file := IntegrationNew(t, "l1000.json", "New+ID")
	value := IntegrationModel(t, file)
	raw := IntegrationWrite(t, value)
	names, contents := IntegrationContents(t, raw)
	if !slices.Equal(names, []string{"manifest.json", "l1000.json"}) {
		t.Fatal(names)
	}
	if !bytes.Equal(contents["manifest.json"], IntegrationJSON(t, value.Manifest)) || !bytes.Equal(contents[file.FileName()], IntegrationJSON(t, file.Lesson())) {
		t.Fatal("new JSON differs from standard indent2 serialization")
	}
	if bytes.Contains(raw, []byte("private-source.json")) {
		t.Fatal("source context leaked")
	}
	loaded := IntegrationRead(t, IntegrationPath(t, raw))
	if !bytes.Equal(loaded.Files[0].OriginalBytes(), contents[file.FileName()]) {
		t.Fatal("Read did not attach exact bytes")
	}
	_, again := IntegrationContents(t, IntegrationWrite(t, loaded))
	if !bytes.Equal(again[file.FileName()], contents[file.FileName()]) {
		t.Fatal("loaded bytes changed")
	}
}

func TestIntegrationUpdateAddAndManifestOnly(t *testing.T) {
	original := IntegrationRaw(t)
	lessonValue := IntegrationLesson(t, original)
	loadedFile, problems := module.LoadedFile("l009.json", lessonValue, original, diagnostic.Context{})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	other := IntegrationNew(t, "l002.json", "AB88")
	initial := IntegrationModel(t, loadedFile, other)
	entries := *initial.Manifest.Course.Lessons
	entries[0], entries[1] = entries[1], entries[0]
	first, second := "001", "9007199254740993123456789"
	entries[0].Order, entries[1].Order = &first, &second
	archive := IntegrationZIP(t, IntegrationEntry{Name: "l009.json", Raw: original}, IntegrationEntry{Name: "manifest.json", Raw: IntegrationJSON(t, initial.Manifest)}, IntegrationEntry{Name: "l002.json", Raw: IntegrationJSON(t, other.Lesson())})
	path := IntegrationPath(t, archive)
	source := IntegrationRead(t, path)
	if source.Files[0].FileName() != "l009.json" || source.Files[1].FileName() != "l002.json" || (*source.Manifest.Course.Lessons)[0].FileName != "l002.json" {
		t.Fatal("ZIP order or manifest order changed")
	}
	nodes := source.Files[0].Lesson().Document.Pages[0].Nodes
	media, ok := nodes[0].Data[0].Value.(lesson.EmbeddedFile)
	if !ok {
		t.Fatalf("media type: %T", nodes[0].Data[0].Value)
	}
	decoded, err := base64.StdEncoding.DecodeString(media.Content)
	if err != nil || !bytes.Equal(decoded, []byte{0, 1, 2, 127, 128, 254, 255, 10, 13}) {
		t.Fatalf("resource bytes: %x %v", decoded, err)
	}
	for _, item := range nodes[0].Data {
		if _, ok := item.Value.(lesson.EmbeddedFile); !ok {
			t.Fatalf("media type %T", item.Value)
		}
	}
	if nodes[1].Flags.Checkable == nil || *nodes[1].Flags.Checkable || nodes[1].Data[0].Value != "I _____{am} Camila.\n\nМы _____{are} <b>日本語</b>." {
		t.Fatal("flags or opaque text changed")
	}
	if _, ok := nodes[2].Data[0].Value.([]lesson.Data); !ok {
		t.Fatalf("collection type: %T", nodes[2].Data[0].Value)
	}
	nested, ok := nodes[3].Data[0].Value.([]lesson.Data)
	if !ok {
		t.Fatal("outer collection lost")
	}
	if _, ok := nested[0].Value.([]lesson.Data); !ok {
		t.Fatal("recursive collection lost")
	}
	if source.Files[0].Lesson().Document.Version != "1e0" || source.Files[0].Lesson().Document.LineCount != "900719925474099300000.0" {
		t.Fatal("numeric lexemes changed")
	}
	for _, mode := range []string{"manifest-only", "update", "add"} {
		t.Run(mode, func(t *testing.T) {
			candidate := IntegrationRead(t, path)
			candidate.Manifest.CreatedAt = "2026-11-04T05:06:07Z"
			candidate.Manifest.Course.Title = "Изменённый курс"
			expected := map[string][]byte{"l009.json": original, "l002.json": IntegrationJSON(t, other.Lesson())}
			switch mode {
			case "update":
				changed := candidate.Files[0].Lesson()
				changed.Document.Title = "Изменённый урок"
				changed.Document.Version = "2"
				changed.Document.CreatedAt = "2026-11-03T01:02:03Z"
				replacement, errs := module.ReplaceFile(candidate.Files[0], changed, diagnostic.Context{Path: "new-source.json"})
				if len(errs) != 0 {
					t.Fatal(errs)
				}
				if replacement.OriginalBytes() != nil {
					t.Fatal("replacement retained old bytes")
				}
				candidate.Files[0] = replacement
				entry := &(*candidate.Manifest.Course.Lessons)[1]
				entry.Title, entry.Version, entry.CreatedAt = changed.Document.Title, changed.Document.Version, changed.Document.CreatedAt
				expected["l009.json"] = IntegrationJSON(t, changed)
			case "add":
				added := IntegrationNew(t, "l010.json", "Added")
				candidate.Files = append(candidate.Files, added)
				prepared := IntegrationModel(t, added)
				entry := (*prepared.Manifest.Course.Lessons)[0]
				order := "9007199254740993123456790"
				entry.Order = &order
				*candidate.Manifest.Course.Lessons = append(*candidate.Manifest.Course.Lessons, entry)
				expected["l010.json"] = IntegrationJSON(t, added.Lesson())
			}
			output := IntegrationWrite(t, candidate)
			names, actual := IntegrationContents(t, output)
			wantNames := []string{"manifest.json"}
			for _, file := range candidate.Files {
				wantNames = append(wantNames, file.FileName())
			}
			if !slices.Equal(names, wantNames) {
				t.Fatal(names)
			}
			if !bytes.Equal(actual["manifest.json"], IntegrationJSON(t, candidate.Manifest)) {
				t.Fatal("manifest date, numbers or arrays changed")
			}
			for name, want := range expected {
				if !bytes.Equal(actual[name], want) {
					t.Fatalf("%s bytes changed", name)
				}
			}
			roundtrip := IntegrationRead(t, IntegrationPath(t, output))
			if roundtrip.Files[0].Lesson().Document.Subtitle != nil || roundtrip.Files[0].Lesson().Document.Pages[0].Title != nil {
				t.Fatal("null fields changed")
			}
		})
	}
	exposed := source.Files[0].OriginalBytes()
	exposed[0] ^= 1
	copyLesson := source.Files[0].Lesson()
	copyLesson.Document.Title = "external mutation"
	if !bytes.Equal(source.Files[0].OriginalBytes(), original) || source.Files[0].Lesson().Document.Title != lessonValue.Document.Title {
		t.Fatal("snapshot ownership violated")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, archive) {
		t.Fatal("source archive changed", err)
	}
	if err := os.Rename(path, path+".closed"); err != nil {
		t.Fatal("archive descriptor remains open", err)
	}
}
