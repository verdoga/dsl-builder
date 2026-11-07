package module_test

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/module"
)

func integrationFlowOK(t *testing.T, problems []*diagnostic.Error) {
	t.Helper()
	if len(problems) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", problems)
	}
}

func integrationFlowLesson(t *testing.T, id string) *lesson.Lesson {
	t.Helper()
	value := &lesson.Lesson{FormatVersion: lesson.FormatVersion, Document: lesson.Document{
		ID: id, Version: "1", Title: "Урок " + id, LineCount: "0",
		CreatedAt: "2026-11-01T10:20:30Z", Pages: []lesson.Page{},
	}}
	integrationFlowOK(t, lesson.Validate(value, diagnostic.Context{}))
	return value
}

func integrationFlowJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationFlowNew(t *testing.T, name, id string) *module.File {
	t.Helper()
	file, problems := module.NewFile(name, integrationFlowLesson(t, id), diagnostic.Context{Path: "вход/" + id + ".json"})
	integrationFlowOK(t, problems)
	if file == nil {
		t.Fatal("NewFile returned nil on success")
	}
	return file
}

func TestIntegrationFlowNewModuleUsesAssignedNamesAndCatalogPlacement(t *testing.T) {
	inputs := []diagnostic.Context{{Path: "A/Урок.json", LessonID: "FLOW+A"}, {Path: "B/Урок.json", LessonID: "FLOW:B"}}
	beforeInputs := slices.Clone(inputs)
	names, problems := module.AssignFileNames(nil, len(inputs), diagnostic.Context{})
	integrationFlowOK(t, problems)
	if !slices.Equal(names, []string{"l001.json", "l002.json"}) || module.ManifestFileName != "manifest.json" {
		t.Fatalf("assigned names: %v, manifest: %q", names, module.ManifestFileName)
	}
	integrationFlowOK(t, module.CheckNewIDs(inputs, nil))
	files := make([]*module.File, len(inputs))
	items := make([]catalog.Item, len(inputs))
	weights := []string{"020", "010"}
	for index, input := range inputs {
		value := integrationFlowLesson(t, input.LessonID)
		before := integrationFlowJSON(t, value)
		files[index], problems = module.NewFile(names[index], value, input)
		integrationFlowOK(t, problems)
		if files[index] == nil || !bytes.Equal(before, integrationFlowJSON(t, value)) || files[index].OriginalBytes() != nil {
			t.Fatal("NewFile must preserve input and create a snapshot without original bytes")
		}
		items[index] = catalog.Item{Entry: catalog.EntryFromLesson(files[index].Lesson().Document, names[index], &weights[index]), Context: input}
	}
	state := &catalog.State{CourseTitle: "Курс", Items: []catalog.Item{items[1], items[0]}}
	stateBefore := integrationFlowJSON(t, state)
	result, problems := catalog.Export(state, "2026-11-07T12:00:00Z", diagnostic.Context{})
	integrationFlowOK(t, problems)
	if result == nil {
		t.Fatal("catalog.Export returned nil")
	}
	candidate := &module.Module{Manifest: *result, Files: files}
	before := integrationFlowJSON(t, candidate.Manifest)
	integrationFlowOK(t, module.Validate(candidate, diagnostic.Context{Path: "результат.zip"}))
	if !bytes.Equal(before, integrationFlowJSON(t, candidate.Manifest)) || !bytes.Equal(stateBefore, integrationFlowJSON(t, state)) || !slices.Equal(inputs, beforeInputs) {
		t.Fatal("creation/validation changed inputs or placement")
	}
	if candidate.Files[0].FileName() != "l001.json" || (*result.Course.Lessons)[0].FileName != "l002.json" || *(*result.Course.Lessons)[0].Order != "010" {
		t.Fatal("technical names, leading zeros or display order changed")
	}
}

func TestIntegrationFlowAdditionKeepsLoadedFilesAndSkipsNumberingHoles(t *testing.T) {
	raw, err := os.ReadFile("testdata/integration/loaded.lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	context := diagnostic.Context{Path: "прежний.zip", FileName: "l009.json"}
	integrationFlowOK(t, lesson.ValidateJSON(raw, context))
	var decoded lesson.Lesson
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	loaded, problems := module.LoadedFile("l009.json", &decoded, raw, context)
	integrationFlowOK(t, problems)
	old := []*module.File{loaded, integrationFlowNew(t, "l001.json", "OLD")}
	oldBytes := integrationFlowJSON(t, loaded.Lesson())
	weights := []string{"10", "20", "30", "40"}
	state := &catalog.State{CourseTitle: "Курс"}
	for index, file := range old {
		state.Items = append(state.Items, catalog.Item{Entry: catalog.EntryFromLesson(file.Lesson().Document, file.FileName(), &weights[index])})
	}
	oldState := integrationFlowJSON(t, state)
	incoming := []diagnostic.Context{{Path: "Новые/A.json", LessonID: "NEW+A"}, {Path: "Новые/B.json", LessonID: "NEW:B"}}
	integrationFlowOK(t, module.CheckNewIDs(incoming, old))
	names, problems := module.AssignFileNames(old, len(incoming), context)
	integrationFlowOK(t, problems)
	if !slices.Equal(names, []string{"l010.json", "l011.json"}) {
		t.Fatalf("holes must remain unused: %v", names)
	}
	files := slices.Clone(old)
	var additions []catalog.Item
	for index, input := range incoming {
		file := integrationFlowNew(t, names[index], input.LessonID)
		files = append(files, file)
		additions = append(additions, catalog.Item{Entry: catalog.EntryFromLesson(file.Lesson().Document, names[index], &weights[index+2]), Context: input})
	}
	added, problems := catalog.Add(state, additions)
	integrationFlowOK(t, problems)
	exported, problems := catalog.Export(added, "2026-11-07T12:00:00Z", context)
	integrationFlowOK(t, problems)
	if exported == nil {
		t.Fatal("missing exported candidate")
	}
	integrationFlowOK(t, module.Validate(&module.Module{Manifest: *exported, Files: files}, context))
	if files[0] != old[0] || files[1] != old[1] || len(old) != 2 || !bytes.Equal(raw, loaded.OriginalBytes()) || !bytes.Equal(oldBytes, integrationFlowJSON(t, loaded.Lesson())) || !bytes.Equal(oldState, integrationFlowJSON(t, state)) {
		t.Fatal("addition changed old files, raw, metadata, slice or catalog snapshot")
	}
	for _, file := range files[2:] {
		if file.Lesson().Document.Version != "1" || file.OriginalBytes() != nil {
			t.Fatal("addition generated a revision or attached old bytes")
		}
	}
}

func TestIntegrationFlowReplacementUsesReadyRevisionAndSyncsCatalog(t *testing.T) {
	previous := integrationFlowNew(t, "l007.json", "Keep+ID")
	weight := "0007"
	state := &catalog.State{CourseTitle: "Курс", Items: []catalog.Item{{Entry: catalog.EntryFromLesson(previous.Lesson().Document, previous.FileName(), &weight)}}}
	oldState, oldLesson := integrationFlowJSON(t, state), integrationFlowJSON(t, previous.Lesson())
	found, problems := module.FindByID([]*module.File{previous}, "KEEP+id", diagnostic.Context{Path: "обновление.json"})
	integrationFlowOK(t, problems)
	if found != previous {
		t.Fatal("FindByID did not preserve the original file")
	}
	ready := previous.Lesson()
	ready.Document.Version, ready.Document.CreatedAt = "2.0", "2026-11-06T11:12:13Z"
	ready.Document.LineCount, ready.Document.Title = "123", "Принятое название"
	readyBefore := integrationFlowJSON(t, ready)
	replaced, problems := module.ReplaceFile(found, ready, diagnostic.Context{Path: "обновление.json"})
	integrationFlowOK(t, problems)
	if replaced == nil || replaced == previous || replaced.FileName() != "l007.json" || replaced.OriginalBytes() != nil || !bytes.Equal(readyBefore, integrationFlowJSON(t, replaced.Lesson())) {
		t.Fatal("replacement changed the ready revision, ID, date or file name")
	}
	synced, problems := catalog.SyncLesson(state, replaced.FileName(), replaced.Lesson().Document)
	integrationFlowOK(t, problems)
	exported, problems := catalog.Export(synced, "2026-11-07T12:00:00Z", diagnostic.Context{})
	integrationFlowOK(t, problems)
	if exported == nil {
		t.Fatal("missing replacement manifest")
	}
	integrationFlowOK(t, module.Validate(&module.Module{Manifest: *exported, Files: []*module.File{replaced}}, diagnostic.Context{}))
	if !bytes.Equal(oldState, integrationFlowJSON(t, state)) || !bytes.Equal(oldLesson, integrationFlowJSON(t, previous.Lesson())) || !bytes.Equal(readyBefore, integrationFlowJSON(t, ready)) || *synced.Items[0].Entry.Order != weight {
		t.Fatal("replacement or synchronization mutated an input")
	}
	for _, id := range []string{"keep+id", "Other"} {
		ready.Document.ID = id
		failed, problems := module.ReplaceFile(previous, ready, diagnostic.Context{})
		if failed != nil || len(problems) != 1 || problems[0].Code != module.CodeIdentityChange {
			t.Fatalf("replacement must reject changed spelling or ID atomically: %v, %v", failed, problems)
		}
	}
}

func TestIntegrationFlowRejectionIsAtomicAndIncludesEverySource(t *testing.T) {
	old := integrationFlowNew(t, "l999.json", "Old+ID")
	before := integrationFlowJSON(t, old.Lesson())
	inputs := []diagnostic.Context{{Path: "A.json", LessonID: "OLD+id"}, {Path: "B.json", LessonID: "old+ID"}, {Path: "C.json", LessonID: "Old+ID"}}
	problems := module.CheckNewIDs(inputs, []*module.File{old})
	var repeats int
	for _, problem := range problems {
		if problem.Code == module.CodeDuplicateID {
			repeats++
			if !slices.Equal(problem.Related, inputs) {
				t.Fatalf("duplicate must name all inputs in order: %+v", problem.Related)
			}
		}
		if problem.Code == module.CodeExistingID {
			for _, input := range inputs {
				if !slices.Contains(problem.Related, input) {
					t.Fatalf("archive conflict omitted input %+v", input)
				}
			}
			if !slices.Contains(problem.Related, old.Context()) {
				t.Fatal("archive conflict omitted the original file")
			}
		}
	}
	if repeats != 2 || len(problems) < 3 || !bytes.Equal(before, integrationFlowJSON(t, old.Lesson())) {
		t.Fatalf("expected each repeated input and existing-ID rejection without update: %v", problems)
	}
	names, problems := module.AssignFileNames([]*module.File{old, old}, 2, diagnostic.Context{})
	if names != nil || len(problems) == 0 || problems[0].Code != module.CodeDuplicateFile {
		t.Fatal("invalid existing set must not produce partially allocated names")
	}
	names, problems = module.AssignFileNames([]*module.File{integrationFlowNew(t, "l999999999999999999999999999999.json", "HUGE")}, 2, diagnostic.Context{})
	integrationFlowOK(t, problems)
	if !slices.Equal(names, []string{"l1000000000000000000000000000000.json", "l1000000000000000000000000000001.json"}) {
		t.Fatalf("technical numbering acquired a machine limit: %v", names)
	}
	invalid := integrationFlowLesson(t, "Valid")
	invalid.Document.Version = "1.5"
	file, problems := module.NewFile("l002.json", invalid, diagnostic.Context{})
	if file != nil || len(problems) == 0 || problems[0].Code != lesson.CodeContract {
		t.Fatal("invalid lesson must not return a partial file")
	}
	badID := identity.ValidateLessonID("bad id")
	problems = module.CheckNewIDs([]diagnostic.Context{{Path: "bad.json", LessonID: "bad id"}}, nil)
	if len(problems) != 1 || problems[0].Code != badID.Code || problems[0].Reason != badID.Reason {
		t.Fatal("identity diagnostics were replaced")
	}
}

func TestIntegrationFlowFindNeverChoosesPartialOrAmbiguousResult(t *testing.T) {
	first := integrationFlowNew(t, "l001.json", "Select+ID")
	second := integrationFlowNew(t, "l002.json", "SELECT+id")
	for _, scenario := range []struct {
		files    []*module.File
		id, code string
	}{{[]*module.File{first}, "missing", module.CodeMissingLesson}, {[]*module.File{first, second}, "select+id", module.CodeAmbiguousLesson}, {[]*module.File{first, nil}, "Select+ID", module.CodeFile}} {
		found, problems := module.FindByID(scenario.files, scenario.id, diagnostic.Context{Path: "обновление.json"})
		if found != nil || len(problems) == 0 || problems[0].Code != scenario.code {
			t.Fatalf("FindByID returned a partial match: %v, %v", found, problems)
		}
		if scenario.code == module.CodeAmbiguousLesson && !slices.Equal(problems[0].Related, []diagnostic.Context{first.Context(), second.Context()}) {
			t.Fatalf("ambiguous lookup omitted or reordered candidates: %+v", problems[0].Related)
		}
	}
}

func TestIntegrationFlowUnitSectionsKeepExactNamesAndAuthoritativeOrder(t *testing.T) {
	first, second := integrationFlowNew(t, "l008.json", "SECTION+A"), integrationFlowNew(t, "l003.json", "SECTION:B")
	unit, upper, lower, weight := "Юнит", "Studentbook", "studentbook", "0001"
	state := &catalog.State{CourseTitle: "Курс", UnitTitle: &unit, Sectioned: true,
		Sections: []catalog.SectionOrder{{Title: lower}, {Title: upper}},
		Items: []catalog.Item{
			{Entry: catalog.EntryFromLesson(second.Lesson().Document, second.FileName(), &weight), Section: &lower},
			{Entry: catalog.EntryFromLesson(first.Lesson().Document, first.FileName(), &weight), Section: &upper},
		},
	}
	exported, problems := catalog.Export(state, "2026-11-07T00:00:00Z", diagnostic.Context{})
	integrationFlowOK(t, problems)
	if exported == nil {
		t.Fatal("unit export returned nil")
	}
	candidate := &module.Module{Manifest: *exported, Files: []*module.File{first, second}}
	before := integrationFlowJSON(t, candidate.Manifest)
	integrationFlowOK(t, module.Validate(candidate, diagnostic.Context{}))
	if !bytes.Equal(before, integrationFlowJSON(t, candidate.Manifest)) || exported.Course.Unit.Sections[0].Title != lower || exported.Course.Unit.Sections[1].Title != upper {
		t.Fatal("module normalized section names, sorted sections or confused file and placement order")
	}
}
