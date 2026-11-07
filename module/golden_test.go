package module_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

// Исторические уроки не преобразуются и не актуализируются этим пакетом.
func TestGoldenHistoricalLessonPreconditionsAndSnapshots(t *testing.T) {
	var entries []manifest.Entry
	if err := json.Unmarshal(goldenRead(t, "testdata/golden/historical.entries.json"), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 7 {
		t.Fatalf("expected seven historical roots, got %d", len(entries))
	}
	files := make([]*module.File, 0, 7)
	for i, entry := range entries {
		t.Run(fmt.Sprint(i+1), func(t *testing.T) {
			path := fmt.Sprintf("../golden/%d. lesson.json", i+1)
			raw := goldenRead(t, path)
			context := diagnostic.Context{Path: path}
			value := goldenDecode(t, raw, context)
			actual := catalog.EntryFromLesson(value.Document, entry.FileName, entry.Order)
			goldenEqualJSON(t, actual, entry)
			file, problems := module.LoadedFile(entry.FileName, value, raw, context)
			goldenNoProblems(t, problems)
			if file == nil {
				t.Fatal("LoadedFile returned nil")
			}
			goldenEqualJSON(t, file.Lesson(), value)
			if !bytes.Equal(file.OriginalBytes(), raw) {
				t.Fatal("historical bytes changed")
			}
			value.Document.Title = "изменён вход"
			copy := file.Lesson()
			copy.Document.Pages[0].Nodes[0].ID = "изменена копия"
			goldenEqualJSON(t, file.Lesson(), goldenDecode(t, raw, context))
			files = append(files, file)
		})
	}
	if len(files) != 7 {
		t.Fatal("historical lesson precondition failed; no partial module")
	}
	// Files намеренно не совпадает с порядком отображения манифеста.
	for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
		files[i], files[j] = files[j], files[i]
	}
	sections := []manifest.Section{}
	value := &module.Module{Files: files, Manifest: manifest.Manifest{
		FormatVersion: "1.0", CreatedAt: "2026-11-07T01:02:03Z",
		Course: manifest.Course{Title: "Исторические снимки", Lessons: &entries, Sections: &sections},
	}}
	goldenNoProblems(t, module.Validate(value, diagnostic.Context{Path: "historical.zip"}))
	for _, file := range files {
		found, problems := module.FindByID(files, file.Lesson().Document.ID, diagnostic.Context{})
		goldenNoProblems(t, problems)
		if found != file {
			t.Fatal("lookup must return the immutable historical File")
		}
	}
}

func TestGoldenLoadedRawOwnershipAndAcceptedReplacement(t *testing.T) {
	raw := goldenRead(t, "testdata/golden/raw.lesson.json")
	expectedRaw := bytes.Clone(raw)
	context := diagnostic.Context{Path: `C:\Курс\модуль.zip`, FileName: "l007.json"}
	value := goldenDecode(t, raw, context)
	expected := goldenDecode(t, expectedRaw, context)
	file, problems := module.LoadedFile("l007.json", value, raw, context)
	goldenNoProblems(t, problems)
	if file == nil {
		t.Fatal("LoadedFile returned nil")
	}
	raw[0] = ' '
	*value.Document.Subtitle = "changed"
	*value.Document.Pages[0].Title = "changed"
	root := &value.Document.Pages[0].Nodes[0]
	*root.Flags.Speaking = true
	choice := &root.Nodes[0]
	*choice.Flags.Checkable = false
	options := choice.Data[0].Value.([]lesson.Data)
	*options[0].Role = lesson.RoleBody
	options[0].Answers[0] = "changed"
	options[0].Hint = "changed"
	options[0].Value = "changed"
	root.Nodes[1].Data[0].Value = lesson.EmbeddedFile{Content: "changed"}
	root.Nodes[2].Data[0].Value = "changed"
	goldenEqualJSON(t, file.Lesson(), expected)
	copy := file.Lesson()
	*copy.Document.Pages[0].Title = "changed copy"
	*copy.Document.Pages[0].Nodes[0].Flags.Stepped = true
	copyOptions := copy.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Value.([]lesson.Data)
	copyOptions[0].Answers[0] = "changed copy"
	*copyOptions[0].Role = lesson.RoleItem
	goldenEqualJSON(t, file.Lesson(), expected)
	returnedRaw := file.OriginalBytes()
	returnedRaw[0] = '!'
	if !bytes.Equal(file.OriginalBytes(), expectedRaw) {
		t.Fatal("raw ownership/formatting/unknown allowed fields were lost")
	}
	if file.Lesson().Document.Version != "1.0" || file.Lesson().Document.LineCount != "1e0" {
		t.Fatal("number spelling was normalized")
	}
	media := file.Lesson().Document.Pages[0].Nodes[0].Nodes[1].Data[0].Value.(lesson.EmbeddedFile)
	decoded, err := base64.StdEncoding.DecodeString(media.Content)
	if err != nil || !bytes.Equal(decoded, []byte{0, 1, 2, 255}) {
		t.Fatalf("embedded bytes: %v, %v", decoded, err)
	}
	gotContext := file.Context()
	if gotContext.Path != context.Path || gotContext.FileName != "l007.json" || gotContext.LessonID != expected.Document.ID {
		t.Fatalf("source context: %+v", gotContext)
	}
	revised := file.Lesson()
	revised.Document.Title = "Принятое изменение"
	revised.Document.Version = "2"
	revised.Document.CreatedAt = "2026-11-07T02:03:04Z"
	replacement, problems := module.ReplaceFile(file, revised, diagnostic.Context{Path: "new.parsed.json"})
	goldenNoProblems(t, problems)
	if replacement == nil || replacement.FileName() != "l007.json" || replacement.OriginalBytes() != nil {
		t.Fatal("accepted replacement must retain its name and require fresh serialization")
	}
	goldenEqualJSON(t, replacement.Lesson(), revised)
	revised.Document.Pages[0].Nodes[0].Data[0].Value = "changed after replacement"
	if replacement.Lesson().Document.Pages[0].Nodes[0].Data[0].Value != " a " {
		t.Fatal("replacement aliases its input")
	}
	goldenEqualJSON(t, file.Lesson(), expected)
	if !bytes.Equal(file.OriginalBytes(), expectedRaw) {
		t.Fatal("old snapshot changed after replacement")
	}
}

func TestGoldenManifestOnlyAndAdditionPreserveLoadedBytes(t *testing.T) {
	raw := goldenRead(t, "testdata/golden/raw.lesson.json")
	context := diagnostic.Context{Path: "старый.zip"}
	old, problems := module.LoadedFile("l007.json", goldenDecode(t, raw, context), raw, context)
	goldenNoProblems(t, problems)
	weight := "01"
	state := &catalog.State{CourseTitle: "Прежний курс", Items: []catalog.Item{{
		Entry: catalog.EntryFromLesson(old.Lesson().Document, old.FileName(), &weight),
	}}}
	changed := catalog.Clone(state)
	changed.CourseTitle = "Принятое название курса"
	unit := "Новый юнит"
	changed.UnitTitle = &unit
	ready, problems := catalog.Export(changed, "2026-11-07T01:02:03Z", context)
	goldenNoProblems(t, problems)
	goldenNoProblems(t, module.Validate(&module.Module{Manifest: *ready, Files: []*module.File{old}}, context))
	if !bytes.Equal(old.OriginalBytes(), raw) || old.Lesson().Document.CreatedAt != "2026-11-03T08:00:00Z" || old.Lesson().Document.Version != "1.0" {
		t.Fatal("manifest-only candidate changed the loaded lesson")
	}
	var additions []lesson.Lesson
	if err := json.Unmarshal(goldenRead(t, "testdata/golden/normative.flat.lessons.json"), &additions); err != nil {
		t.Fatal(err)
	}
	incoming := &additions[0]
	goldenNoProblems(t, module.CheckNewIDs([]diagnostic.Context{{Path: "new.json", LessonID: incoming.Document.ID}}, []*module.File{old}))
	names, problems := module.AssignFileNames([]*module.File{old}, 1, context)
	goldenNoProblems(t, problems)
	if len(names) != 1 || names[0] != "l008.json" {
		t.Fatalf("addition must continue after l007 without filling gaps: %v", names)
	}
	added, problems := module.NewFile(names[0], incoming, diagnostic.Context{Path: "new.json"})
	goldenNoProblems(t, problems)
	nextWeight := "02"
	withAddition, problems := catalog.Add(changed, []catalog.Item{{Entry: catalog.EntryFromLesson(incoming.Document, names[0], &nextWeight)}})
	goldenNoProblems(t, problems)
	ready, problems = catalog.Export(withAddition, "2026-11-07T02:03:04Z", context)
	goldenNoProblems(t, problems)
	goldenNoProblems(t, module.Validate(&module.Module{Manifest: *ready, Files: []*module.File{old, added}}, context))
	if !bytes.Equal(old.OriginalBytes(), raw) || added.OriginalBytes() != nil || old.FileName() != "l007.json" {
		t.Fatal("addition changed old raw/name or attached original bytes to a new lesson")
	}
	if state.CourseTitle != "Прежний курс" || len(state.Items) != 1 || len(changed.Items) != 1 {
		t.Fatal("candidate operations changed previous catalog snapshots")
	}
}

func goldenRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func goldenDecode(t *testing.T, raw []byte, context diagnostic.Context) *lesson.Lesson {
	t.Helper()
	goldenNoProblems(t, lesson.ValidateJSON(raw, context))
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	goldenNoProblems(t, lesson.Validate(&value, context))
	return &value
}

func goldenNoProblems(t *testing.T, problems []*diagnostic.Error) {
	t.Helper()
	for _, problem := range problems {
		t.Errorf("%s: %s; context=%+v; details=%+v", problem.Code, problem.Reason, problem.Context, problem.Details)
	}
	if len(problems) != 0 {
		t.FailNow()
	}
}

func goldenEqualJSON(t *testing.T, got, want any) {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotRaw, wantRaw) {
		t.Fatal("snapshot or exact metadata differs")
	}
}
