package moduleio

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

const jsonreadManifestEntry = `{"id":"EFX7","fileName":"l001.json","order":null,"title":"Урок","subtitle":null,"lineCount":123456789012345678901234567890123456789,"createdAt":"2026-11-04T09:00:00Z","version":1.00e+2}`
const jsonreadManifestFlat = `{"formatVersion":"1.0","createdAt":"2026-11-04T09:00:00Z","course":{"title":"Курс","lessons":[` + jsonreadManifestEntry + `],"sections":[]}}`

func TestJsonreadManifestFormsAndNumbers(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip", FileName: "manifest.json"}
	unit := `{"formatVersion":"1.0","createdAt":"2026-11-04T09:00:00Z","course":{"title":"Курс","unit":{"title":"Юнит","lessons":[],"sections":[{"title":"Вторая","lessons":[` + jsonreadManifestEntry + `]},{"title":"Первая","lessons":[` + strings.Replace(jsonreadManifestEntry, "EFX7", "ABCD", 1) + `]}]}}}`
	for _, raw := range []string{jsonreadManifestFlat + " \r\n\t", unit} {
		before := []byte(raw)
		value, problems := decodeManifest(before, context)
		if value == nil || len(problems) != 0 {
			t.Fatalf("valid manifest decode: %v", problems)
		}
		if string(before) != raw || value.CreatedAt != "2026-11-04T09:00:00Z" {
			t.Fatal("raw/date changed")
		}
		var entry manifest.Entry
		if value.Course.Unit == nil {
			if value.Course.Lessons == nil || value.Course.Sections == nil || *value.Course.Sections == nil || len(*value.Course.Sections) != 0 {
				t.Fatal("empty arrays collapsed or absent")
			}
			entry = (*value.Course.Lessons)[0]
		} else {
			u := value.Course.Unit
			if value.Course.Lessons != nil || value.Course.Sections != nil || u.Lessons == nil || len(u.Lessons) != 0 || u.Sections[0].Title != "Вторая" || u.Sections[1].Title != "Первая" {
				t.Fatal("unit shape or section order changed")
			}
			entry = u.Sections[0].Lessons[0]
		}
		if entry.Order != nil || entry.Subtitle != nil || entry.LineCount != "123456789012345678901234567890123456789" || entry.Version != "1.00e+2" {
			t.Fatalf("null/number lexemes changed: %#v", entry)
		}
	}
}

func TestJsonreadManifestRealShapeDiagnostics(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip", FileName: "manifest.json", PageID: "known-page", Line: 8, Column: 2}
	cases := []string{`null`, `[]`, `{}`, strings.Replace(jsonreadManifestFlat, `"course":{`, `"course":{"unit":null,`, 1)}
	for _, field := range []string{"formatVersion", "createdAt", "course"} {
		cases = append(cases, strings.Replace(jsonreadManifestFlat, `"`+field+`":`, `"`+strings.ToUpper(field)+`":`, 1))
	}
	for _, change := range [][2]string{
		{`"order":null,`, ``}, {`"subtitle":null,`, ``},
		{`"lineCount":123456789012345678901234567890123456789`, `"lineCount":null`},
		{`"sections":[]`, `"sections":null`},
		{`"lessons":[`, `"Lessons":[`},
		{`"title":"Курс"`, `"title":"Курс","TITLE":"другое"`},
		{`"subtitle":null`, `"subtitle":null,"SUBTITLE":"другое"`},
		{`"course":{`, `"extra":true,"course":{`},
		{`"order":null`, `"order":3`},
	} {
		cases = append(cases, strings.Replace(jsonreadManifestFlat, change[0], change[1], 1))
	}
	unit := `{"formatVersion":"1.0","createdAt":"2026-11-04T09:00:00Z","course":{"title":"Курс","unit":{"title":"Юнит","lessons":[],"sections":[{"title":"Раздел","lessons":[` + jsonreadManifestEntry + `]}]}}}`
	cases = append(cases,
		strings.Replace(unit, `"title":"Юнит"`, `"title":"Юнит","extra":false`, 1),
		strings.Replace(unit, `"title":"Раздел"`, `"title":"Раздел","extra":false`, 1))
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			decoder := json.NewDecoder(strings.NewReader(raw))
			decoder.UseNumber()
			var tree any
			if err := decoder.Decode(&tree); err != nil {
				t.Fatal("invalid test JSON", err)
			}
			want := manifest.ValidateShape(tree, context)
			if len(want) == 0 {
				t.Fatal("invalid shape fixture accepted by real owner")
			}
			input := []byte(raw)
			value, got := decodeManifest(input, context)
			if value != nil || string(input) != raw {
				t.Fatal("partial result or raw changed")
			}
			JsonreadOwner(t, got, want)
		})
	}
}

func TestJsonreadManifestSemanticValidationDeferred(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip", FileName: "manifest.json"}
	for _, change := range [][2]string{
		{`"formatVersion":"1.0"`, `"formatVersion":"other"`},
		{`"title":"Курс"`, `"title":" "`},
		{`"version":1.00e+2`, `"version":0`},
		{`"createdAt":"2026-11-04T09:00:00Z"`, `"createdAt":"not-a-date"`},
		{`"fileName":"l001.json"`, `"fileName":"L001.JSON"`},
	} {
		raw := []byte(strings.Replace(jsonreadManifestFlat, change[0], change[1], 1))
		value, problems := decodeManifest(raw, context)
		if value == nil || len(problems) != 0 {
			t.Fatalf("decode performed deferred semantic validation: %v", problems)
		}
		if len(manifest.Validate(*value, context)) == 0 {
			t.Fatal("semantic fixture unexpectedly valid")
		}
	}
	for _, suffix := range []string{` {}`, ` true`, ` garbage`, "\x00"} {
		value, problems := decodeManifest([]byte(jsonreadManifestFlat+suffix), context)
		if value != nil || len(problems) == 0 {
			t.Fatal("manifest accepted trailing data")
		}
		JsonreadProblem(t, problems[0], CodeJSON, context)
	}
	// A shape-valid empty course is left for manifest.Validate as well.
	raw := bytes.Replace([]byte(jsonreadManifestFlat), []byte(jsonreadManifestEntry), nil, 1)
	value, problems := decodeManifest(raw, context)
	if value == nil || len(problems) != 0 || len(manifest.Validate(*value, context)) == 0 {
		t.Fatalf("empty shape/semantic gate: value=%v problems=%v", value, problems)
	}
}
