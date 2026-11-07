package module

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func idsArchiveFiles(t *testing.T, ids ...string) []*File {
	t.Helper()
	files := make([]*File, len(ids))
	state := &catalog.State{CourseTitle: "Курс", Items: make([]catalog.Item, len(ids))}
	for i, id := range ids {
		name, problem := identity.FormatFileName(strconv.Itoa(i + 1))
		if problem != nil {
			t.Fatal(problem)
		}
		files[i] = idsFile(t, name, id)
		weight := strconv.Itoa(i + 1)
		state.Items[i] = catalog.Item{Entry: catalog.EntryFromLesson(files[i].Lesson().Document, name, &weight), Context: files[i].Context()}
	}
	context := diagnostic.Context{Path: "архив с пробелом.zip"}
	manifest, problems := catalog.Export(state, "2026-11-04T09:00:00Z", context)
	if len(problems) != 0 || manifest == nil {
		t.Fatalf("archive fixture catalog invalid: %+v", problems)
	}
	if problems := Validate(&Module{Manifest: *manifest, Files: files}, context); len(problems) != 0 {
		t.Fatalf("archive fixture module invalid: %+v", problems)
	}
	return files
}

func idsRequireParticipants(t *testing.T, problem *diagnostic.Error, want []diagnostic.Context) {
	t.Helper()
	if len(problem.Related) != len(want) {
		t.Fatalf("Related = %+v, want all participants %+v", problem.Related, want)
	}
	for _, context := range want {
		if !slices.Contains(problem.Related, context) {
			t.Fatalf("Related missing %+v: %+v", context, problem.Related)
		}
	}
}

func TestIDsExistingConflictsRequireUpdateAndPreserveWholeArchive(t *testing.T) {
	files := idsArchiveFiles(t, "УРОК", "Second")
	beforeFiles := slices.Clone(files)
	beforeRaw := [][]byte{files[0].OriginalBytes(), files[1].OriginalBytes()}
	beforeLessons := make([][]byte, len(files))
	for i, file := range files {
		var err error
		beforeLessons[i], err = json.Marshal(file.Lesson())
		if err != nil {
			t.Fatal(err)
		}
	}
	incoming := idsContexts(t, "New", "second", "урок")
	beforeIncoming := slices.Clone(incoming)
	for _, problems := range [][]*diagnostic.Error{checkExistingIDs(incoming, files), CheckNewIDs(incoming, files)} {
		idsRequireCodes(t, problems, CodeExistingID, CodeExistingID)
		for i, problem := range problems {
			if problem.Context != incoming[i+1] {
				t.Fatalf("conflict order/context = %+v, want %+v", problem.Context, incoming[i+1])
			}
			idsRequireParticipants(t, problem, []diagnostic.Context{incoming[i+1], files[1-i].Context()})
			if !strings.Contains(strings.ToLower(problem.Reason), "обнов") {
				t.Fatalf("existing ID must explain update mode: %q", problem.Reason)
			}
		}
	}
	if !slices.Equal(incoming, beforeIncoming) || !slices.Equal(files, beforeFiles) {
		t.Fatal("rejected addition changed input or archive slice")
	}
	for i, file := range files {
		after, err := json.Marshal(file.Lesson())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, beforeLessons[i]) || !bytes.Equal(file.OriginalBytes(), beforeRaw[i]) {
			t.Fatalf("existing file %d was replaced or changed", i)
		}
	}
	idsRequireCodes(t, CheckNewIDs(idsContexts(t, "New"), files))
}

func TestIDsExistingConflictingGroupListsEveryIncomingAndStoredParticipant(t *testing.T) {
	files := idsArchiveFiles(t, "УРОК")
	incoming := idsContexts(t, "урок", "Урок", "УРОК")
	want := append(slices.Clone(incoming), files[0].Context())
	first := ""
	for iteration := 0; iteration < 20; iteration++ {
		problems := checkExistingIDs(incoming, files)
		if len(problems) == 0 {
			t.Fatal("existing conflict was accepted")
		}
		for _, problem := range problems {
			if problem == nil || problem.Code != CodeExistingID {
				t.Fatalf("existing conflict code = %+v", problem)
			}
			idsRequireParticipants(t, problem, want)
		}
		encoded := idsDiagnosticJSON(t, problems)
		if iteration == 0 {
			first = encoded
		} else if encoded != first {
			t.Fatal("existing conflict diagnostics are unstable")
		}
	}
	problems := CheckNewIDs(incoming, files)
	duplicates, existing := 0, 0
	for _, problem := range problems {
		switch problem.Code {
		case CodeDuplicateID:
			duplicates++
			idsRequireRelated(t, problem, incoming)
		case CodeExistingID:
			existing++
			idsRequireParticipants(t, problem, want)
		default:
			t.Fatalf("unexpected conflict code: %+v", problem)
		}
	}
	if duplicates != 2 || existing == 0 {
		t.Fatalf("lost independent conflicts: duplicates=%d existing=%d", duplicates, existing)
	}
}

func TestIDsExistingMatchingUsesLowercaseWithoutNormalization(t *testing.T) {
	files := idsArchiveFiles(t, "É", "Σ", "A:B", "lesson+1")
	incoming := idsContexts(t, "e\u0301", "ς", "Other")
	idsRequireCodes(t, checkExistingIDs(incoming, files))
	idsRequireCodes(t, CheckNewIDs(incoming, files))
	for i, id := range []string{"é", "σ", "a:b", "LESSON+1"} {
		input := idsContexts(t, id)
		problems := checkExistingIDs(input, files)
		idsRequireCodes(t, problems, CodeExistingID)
		idsRequireParticipants(t, problems[0], []diagnostic.Context{input[0], files[i].Context()})
	}
	idsRequireCodes(t, checkExistingIDs(nil, files))
	idsRequireCodes(t, checkExistingIDs(idsContexts(t, "", "bad id", string([]byte{0xff})), files))
	idsRequireCodes(t, checkExistingIDs(idsContexts(t, "Fresh"), nil))
}
