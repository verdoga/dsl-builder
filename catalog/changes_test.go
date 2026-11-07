package catalog

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func changesTestPtr(s string) *string { return &s }

func changesTestItem(file, weight, section string) Item {
	item := Item{Entry: manifest.Entry{ID: file, FileName: file, Order: changesTestPtr(weight), Title: "Урок " + file, LineCount: "1", CreatedAt: "2026-11-01T00:00:00Z", Version: "1"}}
	if section != "" {
		item.Section = changesTestPtr(section)
	}
	return item
}

func changesTestChange(file, section, field, before, after string) Change {
	c := Change{FileName: file, Field: field, Before: json.RawMessage(before), After: json.RawMessage(after)}
	if section != "" {
		c.Section = changesTestPtr(section)
	}
	if before == "" {
		c.Before = nil
	}
	if after == "" {
		c.After = nil
	}
	return c
}

func changesTestJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func changesTestCheck(t *testing.T, got, want []Change) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("changes = %s, want %s", changesTestJSON(t, got), changesTestJSON(t, want))
	}
	for i, expected := range want {
		actual := got[i]
		if actual.FileName != expected.FileName || actual.Field != expected.Field || (actual.Section == nil) != (expected.Section == nil) || actual.Section != nil && *actual.Section != *expected.Section {
			t.Errorf("change[%d] selector = %+v, want %+v", i, actual, expected)
		}
		for j, pair := range [][2]json.RawMessage{{actual.Before, expected.Before}, {actual.After, expected.After}} {
			if (pair[0] == nil) != (pair[1] == nil) {
				t.Errorf("change[%d] side %d absence: got %s, want %s", i, j, pair[0], pair[1])
				continue
			}
			if pair[0] == nil {
				continue
			}
			var compact [2]bytes.Buffer
			for k, raw := range pair {
				if expected.Field == "entry" {
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(raw, &fields); err != nil || len(fields) != 8 {
						t.Fatalf("entry must contain exactly eight fields: %s (%v)", raw, err)
					}
					for _, name := range []string{"id", "fileName", "order", "title", "subtitle", "lineCount", "createdAt", "version"} {
						if fields[name] == nil {
							t.Fatalf("entry lacks %s: %s", name, raw)
						}
					}
					var entry manifest.Entry
					if err := json.Unmarshal(raw, &entry); err != nil {
						t.Fatal(err)
					}
					raw = json.RawMessage(changesTestJSON(t, entry))
				}
				if err := json.Compact(&compact[k], raw); err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(compact[0].Bytes(), compact[1].Bytes()) {
				t.Errorf("change[%d] side %d = %s, want %s", i, j, compact[0].Bytes(), compact[1].Bytes())
			}
		}
	}
}

func TestChangesTitlesAndUnit(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after *string
		old, new      string
	}{
		{"create", nil, changesTestPtr("Юнит"), "null", `"Юнит"`},
		{"remove", changesTestPtr("Юнит"), nil, `"Юнит"`, "null"},
		{"rename", changesTestPtr("Юнит"), changesTestPtr("юнит"), `"Юнит"`, `"юнит"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := &State{CourseTitle: "Курс", UnitTitle: tc.before, Items: []Item{changesTestItem("l001.json", "1", "")}}
			after := &State{CourseTitle: "курс ", UnitTitle: tc.after, Items: []Item{changesTestItem("l001.json", "1", "")}}
			changesTestCheck(t, Changes(before, after), []Change{changesTestChange("", "", "course.title", `"Курс"`, `"курс "`), changesTestChange("", "", "course.unit", tc.old, tc.new)})
		})
	}
}

func TestChangesEntryFieldsAndNull(t *testing.T) {
	before := changesTestItem("l001.json", "01", "")
	after := changesTestItem("l002.json", "1", "")
	after.Entry.ID, after.Entry.Title = "ДругойID", " новое название "
	after.Entry.Subtitle, after.Entry.LineCount = changesTestPtr(""), "2"
	after.Entry.CreatedAt, after.Entry.Version = "2026-11-02T00:00:00Z", "3"
	fields := []string{"id", "fileName", "order", "title", "subtitle", "lineCount", "createdAt", "version"}
	old := []string{`"l001.json"`, `"l001.json"`, `"01"`, `"Урок l001.json"`, "null", "1", `"2026-11-01T00:00:00Z"`, "1"}
	new := []string{`"ДругойID"`, `"l002.json"`, `"1"`, `" новое название "`, `""`, "2", `"2026-11-02T00:00:00Z"`, "3"}
	want := make([]Change, len(fields))
	for i, field := range fields {
		want[i] = changesTestChange("l002.json", "", field, old[i], new[i])
	}
	changesTestCheck(t, changesEntry(before, after), want)
	after.Entry.Subtitle = nil
	before.Entry = after.Entry
	before.Entry.Subtitle = changesTestPtr("")
	changesTestCheck(t, changesEntry(before, after), []Change{changesTestChange("l002.json", "", "subtitle", `""`, "null")})
}

func TestChangesNumbersAreExactMathematicalValues(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		changed       bool
	}{
		{"1", "1.0", false}, {"1.0", "1e0", false}, {"0", "-0.0e10", false},
		{"1e400", "10e399", false}, {"1" + strings.Repeat("0", 400), "1e400", false},
		{"9007199254740992", "9007199254740993", true},
		{strings.Repeat("9", 120), "1" + strings.Repeat("0", 120), true},
	} {
		t.Run(tc.before+"_"+tc.after, func(t *testing.T) {
			before, after := changesTestItem("l001.json", "1", ""), changesTestItem("l001.json", "1", "")
			before.Entry.LineCount, after.Entry.LineCount = json.Number(tc.before), json.Number(tc.after)
			var want []Change
			if tc.changed {
				want = []Change{changesTestChange("l001.json", "", "lineCount", tc.before, tc.after)}
			}
			changesTestCheck(t, Changes(&State{CourseTitle: "Курс", Items: []Item{before}}, &State{CourseTitle: "Курс", Items: []Item{after}}), want)
			if tc.before != "0" {
				before.Entry.LineCount, after.Entry.LineCount = "1", "1"
				before.Entry.Version, after.Entry.Version = json.Number(tc.before), json.Number(tc.after)
				if tc.changed {
					want[0].Field = "version"
				}
				changesTestCheck(t, changesEntry(before, after), want)
			}
		})
	}
}

func TestChangesSectionsAndLessonGroups(t *testing.T) {
	a, b, c := changesTestItem("l001.json", "1", "A"), changesTestItem("l002.json", "2", "A"), changesTestItem("l003.json", "3", "B")
	before := &State{CourseTitle: "Курс", Sectioned: true, Sections: []SectionOrder{{Title: "A"}, {Title: "B"}}, Items: []Item{a, b, c}}
	after := &State{CourseTitle: "Курс", Sectioned: true, Sections: []SectionOrder{{Title: "B"}, {Title: "A"}}, Items: []Item{b, c, a}}
	after.Items[0].Section = changesTestPtr("B")
	got := Changes(before, after)
	want := []Change{changesTestChange("", "", "sections", `["A","B"]`, `["B","A"]`), changesTestChange("l002.json", "", "section", `"A"`, `"B"`), changesTestChange("", "B", "lessons", `["l003.json"]`, `["l002.json","l003.json"]`), changesTestChange("", "A", "lessons", `["l001.json","l002.json"]`, `["l001.json"]`)}
	if len(got) != len(want) {
		t.Fatalf("changes = %s", changesTestJSON(t, got))
	}
	changesTestCheck(t, got[:1], want[:1])
	for _, expected := range want[1:] {
		found := false
		for _, actual := range got[1:] {
			if actual.Field == expected.Field && actual.FileName == expected.FileName && (actual.Section == nil && expected.Section == nil || actual.Section != nil && expected.Section != nil && *actual.Section == *expected.Section) {
				changesTestCheck(t, []Change{actual}, []Change{expected})
				found = true
			}
		}
		if !found {
			t.Errorf("missing change %+v", expected)
		}
	}
}

func TestChangesFlatLessonOrder(t *testing.T) {
	a, b := changesTestItem("l001.json", "1", ""), changesTestItem("l002.json", "2", "")
	afterA, afterB := a, b
	afterA.Entry.Order, afterB.Entry.Order = changesTestPtr("2"), changesTestPtr("1")
	changesTestCheck(t, Changes(&State{CourseTitle: "Курс", Items: []Item{a, b}}, &State{CourseTitle: "Курс", Items: []Item{afterB, afterA}}), []Change{
		changesTestChange("", "", "lessons", `["l001.json","l002.json"]`, `["l002.json","l001.json"]`),
		changesTestChange("l002.json", "", "order", `"2"`, `"1"`), changesTestChange("l001.json", "", "order", `"1"`, `"2"`),
	})
}

func TestChangesStableFileJoinWholeEntriesAndReportOrder(t *testing.T) {
	a, b, removed := changesTestItem("l001.json", "1", ""), changesTestItem("l002.json", "2", ""), changesTestItem("l003.json", "3", "")
	newB, newA, added := b, a, changesTestItem("l004.json", "2", "")
	newB.Entry.Title, newB.Entry.Order = "B", changesTestPtr("1")
	newA.Entry.Title, newA.Entry.Order = "A", changesTestPtr("3")
	added.Entry.ID = removed.Entry.ID
	before := &State{CourseTitle: "Курс", Items: []Item{a, b, removed}}
	after := &State{CourseTitle: "Новый", Items: []Item{newB, added, newA}}
	changesTestCheck(t, Changes(before, after), []Change{
		changesTestChange("", "", "course.title", `"Курс"`, `"Новый"`),
		changesTestChange("", "", "lessons", `["l001.json","l002.json","l003.json"]`, `["l002.json","l004.json","l001.json"]`),
		changesTestChange("l002.json", "", "order", `"2"`, `"1"`), changesTestChange("l002.json", "", "title", `"Урок l002.json"`, `"B"`),
		changesTestChange("l004.json", "", "entry", "", changesTestJSON(t, added.Entry)),
		changesTestChange("l001.json", "", "order", `"1"`, `"3"`), changesTestChange("l001.json", "", "title", `"Урок l001.json"`, `"A"`),
		changesTestChange("l003.json", "", "entry", changesTestJSON(t, removed.Entry), ""),
	})
}

func TestChangesIgnoreWorkingDetailsAndOwnResults(t *testing.T) {
	before := &State{CourseTitle: "Курс", Items: []Item{changesTestItem("l001.json", "1", "")}}
	after := &State{CourseTitle: "Курс", Sectioned: true, Sections: []SectionOrder{{Title: "Пустая", Weight: changesTestPtr("9")}}, Items: []Item{changesTestItem("l001.json", "1", "")}}
	after.Items[0].RelativePath, after.Items[0].Context = "путь.json", diagnostic.Context{Path: "archive.zip", FileName: "l001.json", LessonID: "Источник", Line: 9}
	changesTestCheck(t, Changes(before, after), nil)
	before.Sectioned, after.Sectioned = true, true
	before.Sections, after.Sections = []SectionOrder{{Title: "A", Weight: changesTestPtr("1")}}, []SectionOrder{{Title: "B", Weight: changesTestPtr("7")}}
	before.Items[0].Section, after.Items[0].Section = changesTestPtr("A"), changesTestPtr("A")
	after.Sections[0].Title = "A"
	changesTestCheck(t, Changes(before, after), nil)
	after.Sections[0].Title, after.Items[0].Section = "B", changesTestPtr("B")
	after.Items[0].Entry.Subtitle = changesTestPtr("новый")
	oldBefore, oldAfter := changesTestJSON(t, before), changesTestJSON(t, after)
	got := Changes(before, after)
	if changesTestJSON(t, before) != oldBefore || changesTestJSON(t, after) != oldAfter {
		t.Fatal("Changes mutated inputs")
	}
	original := changesTestJSON(t, got)
	for i := range got {
		if got[i].Section != nil {
			*got[i].Section = "чужая"
		}
		for _, raw := range []json.RawMessage{got[i].Before, got[i].After} {
			if len(raw) > 0 {
				raw[0] = '?'
			}
		}
	}
	if changesTestJSON(t, before) != oldBefore || changesTestJSON(t, after) != oldAfter {
		t.Fatal("result aliases inputs")
	}
	if changesTestJSON(t, Changes(before, after)) != original {
		t.Fatal("result aliases future reports")
	}
}

func TestChangesSectionNamesAreExactAndFlatMoveIsNull(t *testing.T) {
	for _, section := range []string{"", "A"} {
		before := &State{CourseTitle: "Курс", Items: []Item{changesTestItem("l001.json", "1", section)}}
		after := &State{CourseTitle: "Курс", Sectioned: true, Sections: []SectionOrder{{Title: "a"}}, Items: []Item{changesTestItem("l001.json", "1", "a")}}
		oldNames, oldSection := "[]", "null"
		if section != "" {
			before.Sectioned = true
			before.Sections = []SectionOrder{{Title: section}}
			oldNames, oldSection = `["A"]`, `"A"`
		}
		got := Changes(before, after)
		if len(got) < 2 {
			t.Fatalf("missing structure changes: %s", changesTestJSON(t, got))
		}
		changesTestCheck(t, got[:1], []Change{changesTestChange("", "", "sections", oldNames, `["a"]`)})
		found := false
		for _, change := range got {
			if change.Field == "section" {
				changesTestCheck(t, []Change{change}, []Change{changesTestChange("l001.json", "", "section", oldSection, `"a"`)})
				found = true
			}
		}
		if !found {
			t.Fatal("missing placement change")
		}
	}
}
