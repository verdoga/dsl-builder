package console

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/catalog"
)

func TestActionsValuesAndRussianLabels(t *testing.T) {
	values := []struct {
		value action
		want  string
	}{
		{actionCourse, "course"}, {actionUnit, "unit"}, {actionLessonWeight, "lesson_weight"},
		{actionGroupWeights, "group_weights"}, {actionSectionWeights, "section_weights"},
		{actionNewSection, "new_section"}, {actionMove, "move"}, {actionFlat, "flat"},
		{actionField, "field"}, {actionPlacement, "placement"}, {actionRevision, "revision"},
		{actionDestination, "destination"}, {actionSave, "save"}, {actionUnused, "unused"},
		{actionRetry, "retry"}, {actionRestart, "restart"}, {actionMain, "main"}, {actionExit, "exit"},
	}
	seen := make(map[string]bool)
	for _, tc := range values {
		t.Run(tc.want, func(t *testing.T) {
			if string(tc.value) != tc.want {
				t.Fatalf("value=%q, want %q", tc.value, tc.want)
			}
			label := actionLabel(tc.value)
			if label == "" || !utf8.ValidString(label) || strings.ContainsAny(label, "\r\n") {
				t.Fatalf("invalid label %q", label)
			}
			russian := false
			for _, r := range label {
				if unicode.In(r, unicode.Cyrillic) {
					russian = true
				}
			}
			if !russian || seen[label] {
				t.Fatalf("label must be Russian and unique: %q", label)
			}
			seen[label] = true
		})
	}
	if got := actionLabel(actionRevision); got != "Перейти к итоговому просмотру изменений" {
		t.Fatalf("revision label=%q", got)
	}
}

func actionsTestSet(t *testing.T, snapshot app.Snapshot, want ...action) {
	t.Helper()
	got := availableActions(snapshot)
	counts := make(map[action]int)
	for _, a := range got {
		counts[a]++
	}
	if len(got) != len(want) {
		t.Errorf("actions=%v, want set %v", got, want)
	}
	for _, a := range want {
		if counts[a] != 1 {
			t.Errorf("%q count=%d in %v", a, counts[a], got)
		}
	}
	for a, n := range counts {
		found := false
		for _, w := range want {
			if a == w {
				found = true
			}
		}
		if !found || n != 1 {
			t.Errorf("unexpected/duplicate %q in %v", a, got)
		}
	}
}

func TestActionsProjectionUsesOnlyPermissions(t *testing.T) {
	// Эти DTO проверяют чистую проекцию, а не достижимость меню у app.
	populated := &catalog.State{Sectioned: true, Items: []catalog.Item{{}}, Sections: []catalog.SectionOrder{{Title: "Раздел"}}}
	cases := []struct {
		name  string
		flags app.Actions
		want  []action
	}{
		{"none", app.Actions{}, nil},
		{"catalog", app.Actions{EditCatalog: true}, []action{actionCourse, actionUnit, actionLessonWeight, actionGroupWeights}},
		{"sections", app.Actions{EditSections: true}, []action{actionNewSection, actionMove, actionFlat}},
		{"sort", app.Actions{SortSections: true}, []action{actionSectionWeights}},
		{"field", app.Actions{DecideFields: true}, []action{actionField}},
		{"placement", app.Actions{DecidePlacement: true}, []action{actionPlacement}},
		{"revision", app.Actions{PrepareRevision: true}, []action{actionRevision}},
		{"destination", app.Actions{ChooseDestination: true}, []action{actionDestination}},
		{"review", app.Actions{ReviewSave: true}, []action{actionSave}},
		{"unused", app.Actions{UnusedResources: true}, []action{actionUnused}},
		{"retry", app.Actions{Retry: true}, []action{actionRetry}},
		{"main", app.Actions{MainMenu: true}, []action{actionRestart, actionMain}},
		{"exit", app.Actions{Exit: true}, []action{actionExit}},
		{"save is not menu", app.Actions{Save: true}, nil},
		{"cancel covered by main", app.Actions{Cancel: true}, []action{actionMain}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actionsTestSet(t, app.Snapshot{Catalog: populated, Actions: tc.flags}, tc.want...)
		})
	}
	// Ни отсутствие блокировок, ни Mode/готовая модель не создают полномочий.
	actionsTestSet(t, app.Snapshot{Mode: app.Create, Catalog: populated, ManifestReady: true, RevisionReady: true})
	actionsTestSet(t, app.Snapshot{Actions: app.Actions{MainMenu: true, Cancel: true}}, actionRestart, actionMain)
}

func TestActionsProjectionListAndPlacementGuards(t *testing.T) {
	edit := app.Actions{EditCatalog: true, EditSections: true, SortSections: true}
	base := []action{actionCourse, actionUnit, actionNewSection}
	for _, state := range []*catalog.State{nil, {}} {
		actionsTestSet(t, app.Snapshot{Actions: edit, Catalog: state}, base...)
	}
	actionsTestSet(t, app.Snapshot{Actions: edit, Catalog: &catalog.State{Sectioned: true}}, actionCourse, actionUnit, actionNewSection, actionFlat)
	section := "Раздел"
	cases := []struct {
		name  string
		state catalog.State
		want  []action
	}{
		{"flat model", catalog.State{Items: []catalog.Item{{}}}, []action{actionCourse, actionUnit, actionLessonWeight, actionGroupWeights, actionNewSection, actionMove}},
		{"all unassigned", catalog.State{Sectioned: true, Items: []catalog.Item{{}, {}}}, []action{actionCourse, actionUnit, actionLessonWeight, actionGroupWeights, actionNewSection, actionMove, actionFlat}},
		{"one assigned", catalog.State{Sectioned: true, Items: []catalog.Item{{}, {Section: &section}}, Sections: []catalog.SectionOrder{{Title: section}}}, []action{actionCourse, actionUnit, actionLessonWeight, actionGroupWeights, actionNewSection, actionMove, actionSectionWeights}},
		{"sections without items", catalog.State{Sectioned: true, Sections: []catalog.SectionOrder{{Title: section}}}, []action{actionCourse, actionUnit, actionNewSection, actionSectionWeights, actionFlat}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { actionsTestSet(t, app.Snapshot{Actions: edit, Catalog: &tc.state}, tc.want...) })
	}
	flags := edit
	flags.DecidePlacement = true
	flags.DecideFields = true
	flags.PrepareRevision = true
	flags.ReviewSave = true
	flags.ChooseDestination = true
	flags.Retry = true
	flags.UnusedResources = true
	flags.MainMenu = true
	flags.Exit = true
	s := app.Snapshot{Actions: flags, Catalog: &catalog.State{Sectioned: true, Items: []catalog.Item{{}}, Sections: []catalog.SectionOrder{{Title: section}}}, PlacementAnswered: []bool{false}}
	actionsTestSet(t, s, actionField, actionPlacement, actionRevision, actionSave, actionDestination, actionRetry, actionUnused, actionRestart, actionMain, actionExit)
}

func TestActionsChoiceUsesGivenListAndRepeatsSameNumbers(t *testing.T) {
	for _, list := range [][]action{{actionExit}, {actionRevision, actionMain, actionCourse}, {actionCourse, actionExit}} {
		for index, want := range list {
			t.Run(fmt.Sprintf("%v/%d", list, index), func(t *testing.T) {
				var out bytes.Buffer
				d := dialog{input: bufio.NewReader(strings.NewReader(fmt.Sprintf("0\n999\nда\n\n%d\n", index+1))), output: &out}
				got, err := d.operationChoice(list)
				if err != nil || got != want {
					t.Fatalf("choice=(%q,%v), want %q", got, err, want)
				}
				text := out.String()
				for n, a := range list {
					label := fmt.Sprintf("%d. %s", n+1, actionLabel(a))
					if strings.Count(text, label) != 5 {
						t.Errorf("same list not repeated five times: %q\n%s", label, text)
					}
				}
				for _, a := range []action{actionRetry, actionSave, actionUnused} {
					if strings.Contains(text, actionLabel(a)) {
						t.Errorf("unprovided action %q shown", a)
					}
				}
			})
		}
	}
}
