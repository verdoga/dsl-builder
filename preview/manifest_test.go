package preview

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/manifest"
)

func TestManifestChangeShowsEveryFieldAndSelector(t *testing.T) {
	section := "Секция 中文\t\n\r\x1b[2J\u0085"
	fileName := "l900.json\t\r\x1b[31m"
	for _, field := range []string{
		"course.title", "course.unit", "sections", "section", "lessons",
		"id", "fileName", "order", "title", "subtitle", "lineCount", "createdAt", "version",
	} {
		t.Run(field, func(t *testing.T) {
			change := catalog.Change{Field: field, Before: []byte(`"before 中文"`), After: []byte(`"after Кириллица"`)}
			if field == "lessons" {
				change.Section = &section
				change.Before, change.After = []byte(`["l900.json","l001.json"]`), []byte(`["l002.json","l001.json"]`)
			} else if field == "sections" {
				change.Before, change.After = []byte(`["Я","А"]`), []byte(`["Б","А"]`)
			} else if field == "lineCount" || field == "version" {
				change.Before, change.After = []byte(`9007199254740993`), []byte(`18446744073709551616000001`)
			} else if field == "order" {
				change.Before, change.After = []byte(`"000000000000001"`), []byte(`"000000000000002"`)
			}
			if !strings.HasPrefix(field, "course.") && field != "sections" && field != "lessons" {
				change.FileName = fileName
			}
			before, _ := json.Marshal(change)
			got, err := manifestChange(change)
			if err != nil {
				t.Fatal(err)
			}
			manifestTestSafe(t, got, true)
			for _, label := range []string{field, "Было", "Станет"} {
				if !strings.Contains(got, label) {
					t.Errorf("missing %q in %q", label, got)
				}
			}
			quoted := manifestTestQuotedValues(t, got)
			for _, selector := range []string{change.FileName, manifestTestSection(change.Section)} {
				if selector != "" && !slices.Contains(quoted, selector) {
					t.Errorf("selector %q missing from %#v", selector, quoted)
				}
			}
			for _, raw := range []json.RawMessage{change.Before, change.After} {
				if raw[0] == '"' {
					var want string
					if err := json.Unmarshal(raw, &want); err != nil || !slices.Contains(quoted, want) {
						t.Errorf("missing string value %s", raw)
					}
				} else if raw[0] == '[' {
					var want []string
					if err := json.Unmarshal(raw, &want); err != nil {
						t.Fatal(err)
					}
					position := 0
					for _, value := range want {
						index := slices.Index(quoted[position:], value)
						if index < 0 {
							t.Fatalf("array lost ordered value %q: %#v", value, quoted)
						}
						position += index + 1
					}
				} else if !strings.Contains(got, string(raw)) {
					t.Errorf("missing ordered array or precise number %s in %q", raw, got)
				}
			}
			after, _ := json.Marshal(change)
			if !bytes.Equal(before, after) || section != "Секция 中文\t\n\r\x1b[2J\u0085" {
				t.Fatal("input change or Section pointer changed")
			}
		})
	}
}

func TestManifestChangesPreservesSuppliedOrderAndAllSides(t *testing.T) {
	changes := []catalog.Change{
		{FileName: "l900.json", Field: "title", Before: []byte(`"old-first"`), After: []byte(`"new-first"`)},
		{Field: "course.unit", Before: []byte(`"old-second"`), After: []byte(`"new-second"`)},
		{FileName: "l001.json", Field: "subtitle", Before: []byte(`"old-third"`), After: []byte(`"new-third"`)},
	}
	before, _ := json.Marshal(changes)
	got, err := ManifestChanges(changes)
	if err != nil {
		t.Fatal(err)
	}
	quoted := manifestTestQuotedValues(t, got)
	position := 0
	for _, want := range []string{"old-first", "new-first", "old-second", "new-second", "old-third", "new-third"} {
		index := slices.Index(quoted[position:], want)
		if index < 0 {
			t.Fatalf("missing ordered value %q in %#v", want, quoted)
		}
		position += index + 1
	}
	after, _ := json.Marshal(changes)
	if !bytes.Equal(before, after) {
		t.Fatal("Changes input changed")
	}
	if strings.Contains(got, "createdAt") {
		t.Fatalf("unsupplied creation time was added: %q", got)
	}
	manifestTestNoDecision(t, got)
}

func TestManifestChangesPreservesWholeEntryOnAdditionAndRemoval(t *testing.T) {
	order, subtitle := "00000000000000000000000000001", "Подзаголовок\t\n\r\x1b[2J\u0085"
	want := manifest.Entry{
		ID: "AbЯ-99", FileName: "l1234.json", Order: &order, Title: "  中文 ⛵ \" \\ \x00  ",
		Subtitle: &subtitle, LineCount: "9007199254740993", CreatedAt: "2026-11-03T10:00:00Z",
		Version: "1844674407370955161600000000001",
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	for _, addition := range []bool{true, false} {
		change := catalog.Change{FileName: want.FileName, Field: "entry"}
		if addition {
			change.After = raw
		} else {
			change.Before = raw
		}
		before := bytes.Clone(raw)
		got, err := ManifestChanges([]catalog.Change{change})
		if err != nil {
			t.Fatal(err)
		}
		manifestTestSafe(t, got, true)
		start, end := strings.IndexByte(got, '{'), strings.LastIndexByte(got, '}')
		if start < 0 || end < start {
			t.Fatalf("whole entry is missing: %q", got)
		}
		var entry manifest.Entry
		var fields map[string]json.RawMessage
		object := []byte(got[start : end+1])
		if err := json.Unmarshal(object, &entry); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(object, &fields); err != nil || len(fields) != 8 {
			t.Fatalf("entry lost fields: %s, %v", object, err)
		}
		encoded, err := json.Marshal(entry)
		if err != nil || !bytes.Equal(encoded, raw) {
			t.Fatalf("entry changed: %s; want %s; error %v", encoded, raw, err)
		}
		if !strings.Contains(got, "отсутствует") || !strings.Contains(got, "Было") || !strings.Contains(got, "Станет") {
			t.Fatalf("addition/removal lost absent side: %q", got)
		}
		if !bytes.Equal(before, raw) || order != *want.Order || subtitle != *want.Subtitle {
			t.Fatal("entry input changed")
		}
		manifestTestNoDecision(t, got)
	}
}

func TestManifestChangesEmptyRefersOnlyToManifest(t *testing.T) {
	for _, changes := range [][]catalog.Change{nil, {}} {
		got, err := ManifestChanges(changes)
		if err != nil || !strings.Contains(strings.ToLower(got), "манифест") {
			t.Fatalf("empty list should explicitly describe manifest: %q, %v", got, err)
		}
		manifestTestNoDecision(t, got)
	}
}

func TestManifestChangesInvalidJSONDiscardsTheWholeBlock(t *testing.T) {
	for _, raw := range []json.RawMessage{{}, []byte(" \n\t"), []byte(`{"CONFIDENTIAL_MANIFEST_PAYLOAD":`), []byte(`"CONFIDENTIAL_MANIFEST_PAYLOAD" false`)} {
		for _, badBefore := range []bool{true, false} {
			bad := catalog.Change{FileName: "l002.json", Field: "subtitle", Before: []byte(`null`), After: []byte(`""`)}
			if badBefore {
				bad.Before = raw
			} else {
				bad.After = raw
			}
			changes := []catalog.Change{{Field: "course.title", Before: []byte(`"old"`), After: []byte(`"new"`)}, bad}
			before, after := bytes.Clone(bad.Before), bytes.Clone(bad.After)
			got, err := ManifestChanges(changes)
			manifestTestError(t, got, err)
			got, err = manifestChange(bad)
			manifestTestError(t, got, err)
			if !bytes.Equal(before, bad.Before) || !bytes.Equal(after, bad.After) {
				t.Fatal("invalid input changed")
			}
		}
	}
}

func TestManifestChangeDistinguishesAbsentNullAndEmptyString(t *testing.T) {
	for _, change := range []catalog.Change{
		{FileName: "l001.json", Field: "subtitle", Before: nil, After: []byte(`null`)},
		{FileName: "l001.json", Field: "subtitle", Before: []byte(`null`), After: []byte(`""`)},
		{Field: "lessons", Before: []byte(`[]`), After: []byte(`["l900.json","l001.json"]`)},
	} {
		got, err := manifestChange(change)
		if err != nil {
			t.Fatal(err)
		}
		if change.Before == nil && !strings.Contains(got, "отсутствует") {
			t.Fatalf("absent side is missing: %q", got)
		}
		if change.Field == "subtitle" && !strings.Contains(got, "null") {
			t.Fatalf("literal null is missing: %q", got)
		}
		quoted := manifestTestQuotedValues(t, got)
		if string(change.After) == `""` && !slices.Contains(quoted, "") {
			t.Fatalf("empty string is missing: %q", got)
		}
		if change.Field == "lessons" && (!strings.Contains(got, "[]") || !slices.Contains(quoted, "l900.json") || !slices.Contains(quoted, "l001.json")) {
			t.Fatalf("general group array is missing: %q", got)
		}
		manifestTestNoDecision(t, got)
	}
}

func manifestTestSection(section *string) string {
	if section == nil {
		return ""
	}
	return *section
}

func manifestTestNoDecision(t *testing.T, text string) {
	t.Helper()
	manifestTestSafe(t, text, true)
	for _, forbidden := range []string{
		"root.createdat", "дата манифеста будет обновлена", "подтверждён", "подтвержден",
		"успешно", "записать изменения?", "1. да", "содержательных изменений не обнаружено",
		"урок, его версия и архив оставлены без изменений",
	} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Errorf("preview makes operation decision %q: %q", forbidden, text)
		}
	}
}
