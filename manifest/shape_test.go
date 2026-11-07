package manifest_test

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func shapeContractDecode(t *testing.T, raw string) any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func shapeContractFixture(t *testing.T, unit bool) any {
	t.Helper()
	entry := `{"id":"A:B","fileName":"l001.json","order":null,"title":"Урок","subtitle":null,"lineCount":9007199254740993,"createdAt":"2026-11-04T09:00:00Z","version":1}`
	contents := `"lessons":[` + entry + `],"sections":[{"title":"Раздел","lessons":[` + entry + `]}]`
	course := `{"title":"Курс",` + contents + `}`
	if unit {
		course = `{"title":"Курс","unit":{"title":"Юнит",` + contents + `}}`
	}
	return shapeContractDecode(t, `{"formatVersion":"1.0","createdAt":"2026-11-04T09:00:00Z","course":`+course+`}`)
}

func shapeContractAt(value any, pointer string) any {
	if pointer == "" {
		return value
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		switch current := value.(type) {
		case map[string]any:
			value = current[part]
		case []any:
			index, _ := strconv.Atoi(part)
			value = current[index]
		}
	}
	return value
}

func shapeContractSnapshot(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func shapeContractErrors(t *testing.T, value any, pointers ...string) {
	t.Helper()
	context := diagnostic.Context{Path: `C:\Курс\архив.zip`, FileName: "manifest.json", LessonID: "Known-ID", PageID: "known-page", Line: 7, Column: 3}
	before := shapeContractSnapshot(t, value)
	errors := manifest.ValidateShape(value, context)
	if len(errors) != len(pointers) {
		t.Fatalf("errors = %+v; want pointers %q", errors, pointers)
	}
	for i, err := range errors {
		if err == nil {
			t.Fatalf("error %d is nil", i)
		}
		if err.Code != manifest.CodeShape {
			t.Errorf("code = %q", err.Code)
		}
		if !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
			t.Errorf("reason is not Russian: %q", err.Reason)
		}
		expectedContext := context
		parts := strings.Split(pointers[i], "/")
		for j, part := range parts {
			if part == "lessons" && j+1 < len(parts) {
				entry, ok := shapeContractAt(value, strings.Join(parts[:j+2], "/")).(map[string]any)
				if ok {
					if id, ok := entry["id"].(string); ok {
						expectedContext.LessonID = id
					}
					if name, ok := entry["fileName"].(string); ok {
						expectedContext.FileName = name
					}
				}
				break
			}
		}
		if err.Context != expectedContext {
			t.Errorf("context = %+v; want %+v", err.Context, expectedContext)
		}
		found := false
		for _, detail := range err.Details {
			if detail.Value == pointers[i] {
				found = true
			}
		}
		if !found {
			t.Errorf("error %d lacks pointer %q: %+v", i, pointers[i], err.Details)
		}
	}
	if after := shapeContractSnapshot(t, value); after != before {
		t.Fatal("ValidateShape mutated input")
	}
}

func TestShapeContractExamplesAndForms(t *testing.T) {
	for _, name := range []string{"Manifest flat example.json", "Manifest unit example.json"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../" + name)
			if err != nil {
				t.Fatal(err)
			}
			shapeContractErrors(t, shapeContractDecode(t, string(raw)))
		})
	}
	for _, unit := range []bool{false, true} {
		value := shapeContractFixture(t, unit)
		shapeContractErrors(t, value)
		base := "/course"
		if unit {
			base += "/unit"
		}
		entry := shapeContractAt(value, base+"/lessons/0").(map[string]any)
		entry["subtitle"], entry["order"] = "", "0001"
		entry["lineCount"], entry["version"] = json.Number("1.00e2"), json.Number("123456789012345678901234567890")
		shapeContractErrors(t, value)
	}
	for _, raw := range []string{`null`, `false`, `1`, `"x"`, `[]`} {
		shapeContractErrors(t, shapeContractDecode(t, raw), "")
	}
}

func TestShapeContractRequiredTypesAndUnknownFields(t *testing.T) {
	for _, unit := range []bool{false, true} {
		base := "/course"
		courseFields := []string{"title:string", "lessons:array", "sections:array"}
		if unit {
			base += "/unit"
			courseFields = []string{"title:string", "unit:object"}
		}
		nodes := []struct {
			pointer string
			fields  []string
		}{
			{"", []string{"formatVersion:string", "createdAt:string", "course:object"}},
			{"/course", courseFields},
			{base + "/sections/0", []string{"title:string", "lessons:array"}},
		}
		if unit {
			nodes = append(nodes, struct {
				pointer string
				fields  []string
			}{base, []string{"title:string", "lessons:array", "sections:array"}})
		}
		for _, parent := range []string{base + "/lessons/0", base + "/sections/0/lessons/0"} {
			nodes = append(nodes, struct {
				pointer string
				fields  []string
			}{parent, []string{"id:string", "fileName:string", "order:nullable", "title:string", "subtitle:nullable", "lineCount:number", "createdAt:string", "version:number"}})
		}
		for _, node := range nodes {
			for _, field := range node.fields {
				parts := strings.Split(field, ":")
				name, kind := parts[0], parts[1]
				pointer := node.pointer + "/" + name
				// Наличие unit выбирает ветвь курса; отсутствие проверяется отдельно.
				if name != "unit" {
					t.Run(strconv.FormatBool(unit)+pointer+"/missing", func(t *testing.T) {
						value := shapeContractFixture(t, unit)
						delete(shapeContractAt(value, node.pointer).(map[string]any), name)
						shapeContractErrors(t, value, pointer)
					})
				}
				for i, invalid := range []any{nil, false, json.Number("2"), float64(2), "text", []any{}, map[string]any{}} {
					valid := kind == "nullable" && (invalid == nil || i == 4) || kind == "string" && i == 4 || kind == "number" && i == 2 || kind == "array" && i == 5 || kind == "object" && i == 6
					if valid {
						continue
					}
					t.Run(strconv.FormatBool(unit)+pointer+"/type"+strconv.Itoa(i), func(t *testing.T) {
						value := shapeContractFixture(t, unit)
						shapeContractAt(value, node.pointer).(map[string]any)[name] = invalid
						shapeContractErrors(t, value, pointer)
					})
				}
			}
			t.Run(strconv.FormatBool(unit)+node.pointer+"/unknown", func(t *testing.T) {
				value := shapeContractFixture(t, unit)
				object := shapeContractAt(value, node.pointer).(map[string]any)
				object["z/last"] = nil
				object["a~first"] = false
				shapeContractErrors(t, value, node.pointer+"/a~0first", node.pointer+"/z~1last")
			})
		}
		for _, array := range []string{base + "/lessons", base + "/sections", base + "/sections/0/lessons"} {
			for _, raw := range []string{`null`, `true`, `1`, `"x"`, `[]`} {
				value := shapeContractFixture(t, unit)
				items := shapeContractAt(value, array).([]any)
				items[0] = shapeContractDecode(t, raw)
				shapeContractErrors(t, value, array+"/0")
			}
		}
	}
}

func TestShapeContractUnitPresenceAndExclusiveCourseForms(t *testing.T) {
	for _, raw := range []string{
		`{"title":"Курс"}`,
		`{"title":"Курс","unit":{},"lessons":[],"sections":[]}`,
		`{"title":"Курс","unit":null,"lessons":[],"sections":[]}`,
	} {
		value := shapeContractFixture(t, false)
		value.(map[string]any)["course"] = shapeContractDecode(t, raw)
		before := shapeContractSnapshot(t, value)
		errors := manifest.ValidateShape(value, diagnostic.Context{})
		if len(errors) == 0 {
			t.Fatalf("accepted invalid course %s", raw)
		}
		for _, err := range errors {
			if err == nil || err.Code != manifest.CodeShape {
				t.Fatalf("unexpected error: %+v", err)
			}
		}
		if before != shapeContractSnapshot(t, value) {
			t.Fatal("mutated input")
		}
	}
	value := shapeContractFixture(t, true)
	course := value.(map[string]any)["course"].(map[string]any)
	delete(course, "unit")
	course["lessons"], course["sections"] = []any{}, []any{}
	shapeContractErrors(t, value)
}

func TestShapeContractFieldAndArrayDiagnosticOrder(t *testing.T) {
	value := shapeContractFixture(t, false)
	root := value.(map[string]any)
	root["formatVersion"], root["createdAt"] = false, nil
	course := root["course"].(map[string]any)
	course["title"] = false
	entries := course["lessons"].([]any)
	for i := 0; i < 12; i++ {
		entries = append(entries, map[string]any{"id": false, "fileName": false, "order": false, "title": false, "subtitle": false, "lineCount": false, "createdAt": false, "version": false})
	}
	course["lessons"] = entries
	pointers := []string{"/formatVersion", "/createdAt", "/course/title"}
	for i := 1; i < len(entries); i++ {
		for _, field := range []string{"id", "fileName", "order", "title", "subtitle", "lineCount", "createdAt", "version"} {
			pointers = append(pointers, "/course/lessons/"+strconv.Itoa(i)+"/"+field)
		}
	}
	section := course["sections"].([]any)[0].(map[string]any)
	section["title"], section["lessons"] = false, false
	pointers = append(pointers, "/course/sections/0/title", "/course/sections/0/lessons")
	for i := 0; i < 4; i++ {
		shapeContractErrors(t, value, pointers...)
	}
}

func TestShapeContractDefersSemanticConstraints(t *testing.T) {
	value := shapeContractFixture(t, true)
	root := value.(map[string]any)
	root["formatVersion"], root["createdAt"] = "unsupported", "not-a-date"
	course := root["course"].(map[string]any)
	course["title"] = ""
	unit := course["unit"].(map[string]any)
	unit["title"] = " "
	entry := unit["lessons"].([]any)[0].(map[string]any)
	entry["id"], entry["fileName"], entry["order"], entry["title"] = "", "bad", "0", ""
	entry["createdAt"], entry["lineCount"], entry["version"] = "bad", json.Number("-1.5"), json.Number("0")
	unit["lessons"] = []any{entry, entry}
	sections := unit["sections"].([]any)
	section := sections[0].(map[string]any)
	section["title"], section["lessons"] = " a  b\n", []any{}
	unit["sections"] = []any{section, section}
	shapeContractErrors(t, value)
	entry["order"] = nil
	shapeContractErrors(t, value)
	unit["lessons"], unit["sections"] = []any{}, []any{}
	shapeContractErrors(t, value)
}
