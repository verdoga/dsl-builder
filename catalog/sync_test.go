package catalog

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

func syncTestString(value string) *string { return &value }

func syncTestDocument() lesson.Document {
	return lesson.Document{ID: "MiXeD+1", Title: "Старое название", Subtitle: syncTestString("Подзаголовок"), LineCount: "47", CreatedAt: "2026-11-03T10:00:00Z", Version: "5", Pages: []lesson.Page{}}
}

func syncTestState() *State {
	return &State{CourseTitle: "Курс", UnitTitle: syncTestString("Юнит"), Sectioned: true,
		Sections: []SectionOrder{{Title: "Z", Weight: syncTestString("20")}, {Title: "A", Weight: syncTestString("10")}},
		Items: []Item{
			{Entry: manifest.Entry{ID: "MiXeD+1", FileName: "l001.json", Order: syncTestString("01"), Title: "Старое название", Subtitle: syncTestString("Подзаголовок"), LineCount: "47", CreatedAt: "2026-11-03T10:00:00Z", Version: "5"}, Section: syncTestString("Z"), RelativePath: "вход/A.json", Context: diagnostic.Context{Path: "архив.zip", FileName: "l001.json", LessonID: "MiXeD+1", PageID: "mixed+1-p001", Line: 7, Column: 3}},
			{Entry: manifest.Entry{ID: "Other", FileName: "l002.json", Order: syncTestString("2"), Title: "Другой урок", Subtitle: syncTestString("Другой"), LineCount: "52", CreatedAt: "2026-11-02T10:00:00Z", Version: "9"}, Section: syncTestString("Z"), RelativePath: "вход/B.json", Context: diagnostic.Context{Path: "другой.json", FileName: "l002.json", LessonID: "Other"}},
		}}
}

func syncTestEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("получено %#v, ожидалось %#v", got, want)
	}
}

func syncTestCode(t *testing.T, problems []*diagnostic.Error, code string) *diagnostic.Error {
	t.Helper()
	for _, problem := range problems {
		if problem != nil && problem.Code == code {
			return problem
		}
	}
	t.Fatalf("нет диагностики %s: %v", code, problems)
	return nil
}

func TestSyncEntryFromLessonCopiesExactMetadata(t *testing.T) {
	for _, subtitle := range []*string{nil, syncTestString(""), syncTestString("Точный\nподзаголовок")} {
		for _, weight := range []*string{nil, syncTestString("000123456789012345678901234567890")} {
			document := syncTestDocument()
			document.Subtitle = subtitle
			document.LineCount, document.Version = "900719925474099312345678901234567890", "123456789012345678901234567890e100"
			original := document
			if subtitle != nil {
				original.Subtitle = syncTestString(*subtitle)
			}
			var originalWeight *string
			if weight != nil {
				originalWeight = syncTestString(*weight)
			}
			entry := EntryFromLesson(document, "l1000.json", weight)
			want := manifest.Entry{ID: original.ID, FileName: "l1000.json", Order: originalWeight, Title: original.Title, Subtitle: original.Subtitle, LineCount: original.LineCount, CreatedAt: original.CreatedAt, Version: original.Version}
			syncTestEqual(t, entry, want)
			syncTestEqual(t, document, original)
			syncTestEqual(t, weight, originalWeight)
			if subtitle != nil {
				if entry.Subtitle == subtitle {
					t.Fatal("Subtitle разделяет указатель с Document")
				}
				old := *subtitle
				*entry.Subtitle = "правка результата"
				syncTestEqual(t, *subtitle, old)
				*subtitle = "правка источника"
				syncTestEqual(t, *entry.Subtitle, "правка результата")
			}
			if weight != nil {
				if entry.Order == weight {
					t.Fatal("Order разделяет входной указатель")
				}
				old := *weight
				*entry.Order = "3"
				syncTestEqual(t, *weight, old)
			}
		}
	}
}

func TestSyncLessonUsesFinalDocumentAndPreservesPlacement(t *testing.T) {
	for _, scenario := range []string{"принято", "отказ от названия", "нет изменений"} {
		t.Run(scenario, func(t *testing.T) {
			before, want, document := syncTestState(), syncTestState(), syncTestDocument()
			if scenario != "нет изменений" {
				document.Version, document.LineCount, document.CreatedAt = "9007199254740993", "9007199254740993123456789", "2026-11-04T12:15:00Z"
				document.Subtitle = nil
				if scenario == "принято" {
					document.Title = "Подтверждённое название"
				}
			}
			want.Items[0].Entry.Title, want.Items[0].Entry.Subtitle = document.Title, document.Subtitle
			want.Items[0].Entry.Version, want.Items[0].Entry.LineCount, want.Items[0].Entry.CreatedAt = document.Version, document.LineCount, document.CreatedAt
			got, problems := SyncLesson(before, "l001.json", document)
			if len(problems) != 0 {
				t.Fatalf("SyncLesson: %v", problems)
			}
			syncTestEqual(t, got, want)
			syncTestEqual(t, before, syncTestState())
			if scenario == "нет изменений" && len(Changes(before, got)) != 0 {
				t.Fatal("прежний Document создаёт изменения")
			}
		})
	}
}

func TestSyncLessonRejectsSelectionAndIdentityWithoutMutation(t *testing.T) {
	for _, scenario := range []struct{ name, selector, id, code string }{
		{"missing", "l099.json", "MiXeD+1", CodeSelection}, {"ID is not selector", "MiXeD+1", "MiXeD+1", CodeSelection},
		{"case selector", "L001.json", "MiXeD+1", CodeSelection}, {"duplicate", "l001.json", "MiXeD+1", CodeSelection},
		{"case identity", "l001.json", "mixed+1", CodeIdentity}, {"other identity", "l001.json", "Other", CodeIdentity},
		{"nil state", "l001.json", "MiXeD+1", CodeState},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before, want, document := syncTestState(), syncTestState(), syncTestDocument()
			if scenario.name == "duplicate" {
				before.Items[1].Entry.FileName, want.Items[1].Entry.FileName = "l001.json", "l001.json"
			}
			if scenario.name == "nil state" {
				before, want = nil, nil
			}
			document.ID = scenario.id
			got, problems := SyncLesson(before, scenario.selector, document)
			if got != nil {
				t.Fatal("отклонённая операция вернула State")
			}
			syncTestCode(t, problems, scenario.code)
			syncTestEqual(t, before, want)
		})
	}
}

func TestSyncLessonReturnsIndependentSnapshot(t *testing.T) {
	before, document := syncTestState(), syncTestDocument()
	got, problems := SyncLesson(before, "l001.json", document)
	if got == nil || len(problems) != 0 {
		t.Fatalf("SyncLesson: %v", problems)
	}
	*got.UnitTitle, *got.Sections[0].Weight, got.Sections[0].Title = "новый юнит", "99", "новая секция"
	for i := range got.Items {
		*got.Items[i].Section, *got.Items[i].Entry.Order, *got.Items[i].Entry.Subtitle = "другая", "77", "другой"
		got.Items[i].Context.Path, got.Items[i].RelativePath = "правка", "правка"
	}
	syncTestEqual(t, before, syncTestState())
	syncTestEqual(t, document, syncTestDocument())
	*before.Items[0].Entry.Subtitle, *document.Subtitle = "источник", "документ"
	syncTestEqual(t, *got.Items[0].Entry.Subtitle, "другой")
}

func TestSyncLessonRefreshesWholeComposition(t *testing.T) {
	for _, scenario := range []struct{ name, code string }{
		{"missing", CodeMissingWeight}, {"target missing", CodeMissingWeight}, {"conflict", CodeWeightConflict}, {"unassigned", CodeUnassigned}, {"invalid", order.CodeInvalidWeight},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := syncTestState()
			switch scenario.name {
			case "target missing":
				before.Items[0].Entry.Order = nil
			case "missing":
				before.Items[1].Entry.Order = nil
			case "conflict":
				before.Items[1].Entry.Order = syncTestString("001")
			case "unassigned":
				before.Items[1].Section = nil
			case "invalid":
				before.Items[1].Entry.Order = syncTestString("0")
			}
			got, problems := SyncLesson(before, "l001.json", syncTestDocument())
			if got == nil {
				t.Fatal("блокировка всего состава должна сохранять редактируемый кандидат")
			}
			syncTestCode(t, problems, scenario.code)
		})
	}
	before := syncTestState()
	before.Items[0], before.Items[1] = before.Items[1], before.Items[0]
	got, problems := SyncLesson(before, "l001.json", syncTestDocument())
	if len(problems) != 0 {
		t.Fatalf("сортировка: %v", problems)
	}
	syncTestEqual(t, got, syncTestState())
	syncTestEqual(t, before.Items[0].Entry.FileName, "l002.json")
}

func TestSyncProposePlacementReturnsSeparateUnappliedFields(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		section, weight *string
		expected        map[string][2]string
	}{
		{"both", syncTestString("A"), syncTestString("1"), map[string][2]string{"section": {`"Z"`, `"A"`}, "order": {`"01"`, `"1"`}}},
		{"flat keeps weight", nil, nil, map[string][2]string{"section": {`"Z"`, `null`}}},
		{"same", syncTestString("Z"), syncTestString("01"), nil},
		{"case section", syncTestString("z"), nil, map[string][2]string{"section": {`"Z"`, `"z"`}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := syncTestState()
			changes, problems := ProposePlacement(before, "l001.json", scenario.section, scenario.weight)
			if len(problems) != 0 || len(changes) != len(scenario.expected) {
				t.Fatalf("changes=%v, errors=%v", changes, problems)
			}
			seen := map[string]bool{}
			for _, change := range changes {
				pair, ok := scenario.expected[change.Field]
				if !ok || seen[change.Field] {
					t.Fatalf("лишнее или повторное поле: %s", change.Field)
				}
				seen[change.Field] = true
				syncTestEqual(t, change.FileName, "l001.json")
				syncTestEqual(t, [2]string{string(change.Before), string(change.After)}, pair)
			}
			syncTestEqual(t, before, syncTestState())
		})
	}
}

func TestSyncProposePlacementDiagnosticsRetainContextAndRawWeight(t *testing.T) {
	for _, raw := range []string{"", "000", "+1", "1e2", " 01", "١"} {
		before := syncTestState()
		changes, problems := ProposePlacement(before, "l001.json", syncTestString("Z"), &raw)
		problem := syncTestCode(t, problems, order.CodeInvalidWeight)
		syncTestEqual(t, problem.Context, before.Items[0].Context)
		found := false
		for _, detail := range problem.Details {
			if detail.Label == "Исходный вес" && detail.Value == raw {
				found = true
			}
		}
		if !found || len(changes) != 0 {
			t.Fatalf("утерян raw или предложен неверный вес: %v %v", changes, problems)
		}
		syncTestEqual(t, before, syncTestState())
	}
	for _, raw := range []string{"", " A", "A  B", "A\tB", "A\nB", string([]byte{255})} {
		before := syncTestState()
		changes, problems := ProposePlacement(before, "l001.json", &raw, nil)
		problem := syncTestCode(t, problems, manifest.CodeTitle)
		syncTestEqual(t, problem.Context, before.Items[0].Context)
		if len(changes) != 0 {
			t.Fatal("неверная секция предложена к применению")
		}
		syncTestEqual(t, before, syncTestState())
	}
}

func TestSyncProposePlacementSelectionAndOwnership(t *testing.T) {
	for _, scenario := range []string{"missing", "duplicate", "nil"} {
		before, code := syncTestState(), CodeSelection
		selector := "l099.json"
		if scenario == "duplicate" {
			selector = "l001.json"
			before.Items[1].Entry.FileName = selector
		}
		if scenario == "nil" {
			before, code = nil, CodeState
		}
		changes, problems := ProposePlacement(before, selector, nil, nil)
		syncTestCode(t, problems, code)
		if len(changes) != 0 {
			t.Fatal("неоднозначный селектор вернул предложения")
		}
	}
	before, section, weight := syncTestState(), "A", "1"
	changes, problems := ProposePlacement(before, "l001.json", &section, &weight)
	if len(problems) != 0 || len(changes) != 2 {
		t.Fatalf("%v %v", changes, problems)
	}
	encoded, err := json.Marshal(changes)
	if err != nil {
		t.Fatal(err)
	}
	section, weight = "изменённый вход", "999"
	for i := range changes {
		changes[i].Before[0], changes[i].After[0] = '!', '!'
	}
	again, problems := ProposePlacement(before, "l001.json", syncTestString("A"), syncTestString("1"))
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	encodedAgain, err := json.Marshal(again)
	if err != nil {
		t.Fatal(err)
	}
	syncTestEqual(t, string(encodedAgain), string(encoded))
	syncTestEqual(t, before, syncTestState())
}
