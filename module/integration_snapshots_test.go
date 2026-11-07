package module_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
)

func integrationSnapshotInput(t *testing.T) (*lesson.Lesson, []byte, diagnostic.Context) {
	t.Helper()
	raw, err := os.ReadFile("testdata/integration/loaded.lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	context := diagnostic.Context{Path: "Каталог с пробелами/Модуль.zip", FileName: "l017.json", Line: 3, Column: 7}
	if problems := lesson.ValidateJSON(raw, context); len(problems) != 0 {
		t.Fatalf("fixture is not a valid real lesson: %v", problems)
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return &value, raw, context
}

func integrationSnapshotJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationSnapshotMutateData(t *testing.T, value *lesson.Data) {
	t.Helper()
	if value.Role != nil {
		*value.Role = lesson.RoleTitle
	}
	for index := range value.Answers {
		value.Answers[index] = "Изменённый ответ"
	}
	value.Hint = "Изменённая подсказка"
	switch content := value.Value.(type) {
	case string:
		value.Value = "Изменённый текст"
	case lesson.EmbeddedFile:
		content.FileName, content.MIMEType, content.Content = "новый.bin", "changed/type", "AAAA"
		value.Value = content
	case []lesson.Data:
		for index := range content {
			integrationSnapshotMutateData(t, &content[index])
		}
		value.Value = append(content, lesson.Data{})
	default:
		t.Fatalf("unexpected decoded Data.Value type %T", content)
	}
}

func integrationSnapshotMutateNode(t *testing.T, value *lesson.Node) {
	t.Helper()
	value.ID, value.Type = "изменённый-id", lesson.NodeNote
	for _, flag := range []*bool{value.Flags.Speaking, value.Flags.Stepped, value.Flags.Checkable} {
		if flag != nil {
			*flag = !*flag
		}
	}
	for index := range value.Data {
		integrationSnapshotMutateData(t, &value.Data[index])
	}
	for index := range value.Nodes {
		integrationSnapshotMutateNode(t, &value.Nodes[index])
	}
	value.Data = append(value.Data, lesson.Data{})
	value.Nodes = append(value.Nodes, lesson.Node{})
}

func integrationSnapshotMutate(t *testing.T, value *lesson.Lesson) {
	t.Helper()
	value.FormatVersion = "changed"
	value.Document.ID, value.Document.Title = "Changed", "Изменено"
	value.Document.Version, value.Document.LineCount, value.Document.CreatedAt = "13", "17", "2030-01-01T00:00:00Z"
	if value.Document.Subtitle != nil {
		*value.Document.Subtitle = "Изменённый подзаголовок"
	}
	for index := range value.Document.Pages {
		page := &value.Document.Pages[index]
		page.ID = "изменённая-страница"
		if page.Title != nil {
			*page.Title = "Изменённая страница"
		}
		for node := range page.Nodes {
			integrationSnapshotMutateNode(t, &page.Nodes[node])
		}
		page.Nodes = append(page.Nodes, lesson.Node{})
	}
	value.Document.Pages = append(value.Document.Pages, lesson.Page{})
}

func TestIntegrationSnapshotsOwnInputsAndEveryReturnedLayer(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		name := "new"
		if loaded {
			name = "loaded"
		}
		t.Run(name, func(t *testing.T) {
			input, raw, context := integrationSnapshotInput(t)
			wantRaw := bytes.Clone(raw)
			want := integrationSnapshotJSON(t, input)
			var file *module.File
			var problems []*diagnostic.Error
			if loaded {
				file, problems = module.LoadedFile("l017.json", input, raw, context)
			} else {
				file, problems = module.NewFile("l017.json", input, context)
			}
			if len(problems) != 0 || file == nil {
				t.Fatalf("snapshot creation: %v", problems)
			}
			if !bytes.Equal(want, integrationSnapshotJSON(t, input)) || !bytes.Equal(wantRaw, raw) {
				t.Fatal("constructor changed its inputs")
			}
			integrationSnapshotMutate(t, input)
			raw[0] = '!'
			for attempt := 0; attempt < 3; attempt++ {
				copy := file.Lesson()
				if !bytes.Equal(want, integrationSnapshotJSON(t, copy)) {
					t.Fatalf("input or prior returned copy leaked into snapshot at attempt %d", attempt)
				}
				if copy.Document.Version != "9007199254740993" || copy.Document.LineCount != "9007199254740995" || copy.Document.Pages[1].Title != nil || copy.Document.Pages[1].Nodes == nil {
					t.Fatal("exact numbers, nil title or empty array changed")
				}
				children := copy.Document.Pages[0].Nodes[0].Nodes
				collection := children[0].Data[0].Value.([]lesson.Data)
				if collection[1].Role != nil || collection[1].Value.([]lesson.Data) == nil || *children[2].Flags.Checkable {
					t.Fatal("nil role, empty recursive collection or explicit false changed")
				}
				media := children[1].Data[0].Value.(lesson.EmbeddedFile)
				binary, err := base64.StdEncoding.DecodeString(media.Content)
				if err != nil || !bytes.Equal(binary, []byte{0, 1, 2, 255, 254}) || children[2].Data[0].Value != "I _____{am} Camila.\n\nWe _____{are} from Spain." {
					t.Fatal("media bytes or opaque multifill text changed")
				}
				integrationSnapshotMutate(t, copy)
				returned := file.OriginalBytes()
				if loaded {
					if !bytes.Equal(returned, wantRaw) {
						t.Fatal("loaded bytes were serialized or aliased")
					}
					returned[len(returned)-1] = '!'
				} else if returned != nil {
					t.Fatal("new file unexpectedly has original bytes")
				}
				returnedContext := file.Context()
				if returnedContext.Path != context.Path || returnedContext.FileName != "l017.json" || returnedContext.LessonID != "A:Б+" || returnedContext.Line != 3 || returnedContext.Column != 7 {
					t.Fatalf("lost source context: %+v", returnedContext)
				}
				returnedContext.Path, returnedContext.LessonID = "changed", "changed"
			}
		})
	}
}

func TestIntegrationSnapshotsManifestOnlyKeepsSameLoadedFile(t *testing.T) {
	value, raw, context := integrationSnapshotInput(t)
	file, problems := module.LoadedFile("l017.json", value, raw, context)
	if len(problems) != 0 || file == nil {
		t.Fatalf("LoadedFile: %v", problems)
	}
	weight := "00042"
	state := &catalog.State{CourseTitle: "Прежний курс", Items: []catalog.Item{{Entry: catalog.EntryFromLesson(value.Document, file.FileName(), &weight)}}}
	before := integrationSnapshotJSON(t, state)
	candidate := catalog.Clone(state)
	unit, section := "Новый юнит", "studentbook"
	candidate.CourseTitle, candidate.UnitTitle, candidate.Sectioned = "Новый курс", &unit, true
	candidate.Sections = []catalog.SectionOrder{{Title: section}}
	candidate.Items[0].Section = &section
	newWeight := "007"
	candidate.Items[0].Entry.Order = &newWeight
	exported, problems := catalog.Export(candidate, "2026-11-07T00:00:00Z", context)
	if len(problems) != 0 || exported == nil {
		t.Fatalf("manifest-only Export: %v", problems)
	}
	moduleCandidate := &module.Module{Manifest: *exported, Files: []*module.File{file}}
	if problems := module.Validate(moduleCandidate, context); len(problems) != 0 {
		t.Fatalf("manifest-only Validate: %v", problems)
	}
	if moduleCandidate.Files[0] != file || !bytes.Equal(raw, file.OriginalBytes()) || !bytes.Equal(integrationSnapshotJSON(t, value), integrationSnapshotJSON(t, file.Lesson())) || !bytes.Equal(before, integrationSnapshotJSON(t, state)) {
		t.Fatal("manifest-only change modified a lesson, old placement or original bytes")
	}
	entry := exported.Course.Unit.Sections[0].Lessons[0]
	if entry.Version != value.Document.Version || entry.CreatedAt != value.Document.CreatedAt || *entry.Order != "007" || exported.CreatedAt == entry.CreatedAt {
		t.Fatal("root build date, revision or placement crossed ownership boundaries")
	}
}

func TestIntegrationSnapshotsReplacementDropsOnlyNewOriginal(t *testing.T) {
	value, raw, context := integrationSnapshotInput(t)
	old, problems := module.LoadedFile("l017.json", value, raw, context)
	if len(problems) != 0 || old == nil {
		t.Fatalf("LoadedFile: %v", problems)
	}
	ready := old.Lesson()
	ready.Document.Version, ready.Document.CreatedAt = "9007199254740994", "2026-11-07T01:02:03Z"
	ready.Document.Pages[0].Nodes[0].Nodes[0].Data[0].Hint = "Принятое изменение"
	before := integrationSnapshotJSON(t, ready)
	replaced, problems := module.ReplaceFile(old, ready, diagnostic.Context{Path: "Новый вход.json"})
	if len(problems) != 0 || replaced == nil || replaced.OriginalBytes() != nil || replaced.FileName() != old.FileName() || !bytes.Equal(before, integrationSnapshotJSON(t, replaced.Lesson())) {
		t.Fatalf("replacement must retain ready model and discard old serialization: %v", problems)
	}
	integrationSnapshotMutate(t, ready)
	returned := replaced.Lesson()
	integrationSnapshotMutate(t, returned)
	if !bytes.Equal(before, integrationSnapshotJSON(t, replaced.Lesson())) || !bytes.Equal(raw, old.OriginalBytes()) || !bytes.Equal(integrationSnapshotJSON(t, value), integrationSnapshotJSON(t, old.Lesson())) {
		t.Fatal("replacement aliased ready model, returned model or old file")
	}
	for _, raw := range [][]byte{nil, {}} {
		failed, problems := module.LoadedFile("l017.json", value, raw, context)
		if failed != nil || len(problems) != 1 || problems[0].Code != module.CodeOriginalBytes {
			t.Fatal("empty original bytes returned a partial loaded file")
		}
	}
	var absent *module.File
	if absent.FileName() != "" || absent.Lesson() != nil || absent.OriginalBytes() != nil || absent.Context() != (diagnostic.Context{}) {
		t.Fatal("nil file accessors must return zero values")
	}
}

func TestIntegrationSnapshotsFailedReplacementKeepsOldBytesAndInputs(t *testing.T) {
	value, raw, context := integrationSnapshotInput(t)
	old, problems := module.LoadedFile("l017.json", value, raw, context)
	if len(problems) != 0 || old == nil {
		t.Fatalf("LoadedFile: %v", problems)
	}
	invalid := old.Lesson()
	invalid.Document.Version = "0"
	before := integrationSnapshotJSON(t, invalid)
	for _, pair := range []struct {
		old   *module.File
		value *lesson.Lesson
		code  string
	}{{nil, value, module.CodeFile}, {old, nil, module.CodeFile}, {old, invalid, lesson.CodeContract}} {
		failed, problems := module.ReplaceFile(pair.old, pair.value, context)
		if failed != nil || len(problems) == 0 || problems[0].Code != pair.code {
			t.Fatalf("failed replacement returned a partial file or replaced dependency diagnostic: %v, %v", failed, problems)
		}
	}
	if !bytes.Equal(before, integrationSnapshotJSON(t, invalid)) || !bytes.Equal(raw, old.OriginalBytes()) || !bytes.Equal(integrationSnapshotJSON(t, value), integrationSnapshotJSON(t, old.Lesson())) {
		t.Fatal("failed replacement changed input or old snapshot")
	}
	empty := ""
	value.Document.Subtitle = &empty
	file, problems := module.NewFile("l017.json", value, context)
	if len(problems) != 0 || file == nil {
		t.Fatalf("empty subtitle is valid: %v", problems)
	}
	weight := "1"
	entry := catalog.EntryFromLesson(value.Document, file.FileName(), &weight)
	entry.Subtitle = nil
	problems = module.ValidateLinks([]*module.File{file}, []catalog.Item{{Entry: entry}}, context)
	if len(problems) != 1 || problems[0].Code != module.CodeMetadata {
		t.Fatal("empty lesson subtitle was conflated with null manifest subtitle")
	}
}
