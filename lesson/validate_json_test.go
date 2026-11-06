package lesson

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

const (
	jsonShapeDocument = `{"id":"Lesson","version":1,"title":"","lineCount":0,"createdAt":"2026-01-02T03:04:05Z","pages":[]}`
	jsonShapePage     = `{"id":"lesson-p001","title":null,"nodes":[]}`
	jsonShapeNode     = `{"id":"lesson-n001","type":"text","flags":{},"data":[],"nodes":[]}`
	jsonShapeData     = `{"type":"text","value":"","answers":[],"hint":""}`
	jsonShapeEmbedded = `{"fileName":"a.bin","mimeType":"","content":""}`
)

func jsonShapeAssert(t *testing.T, errs []*diagnostic.Error, code, pointer string, context diagnostic.Context) {
	t.Helper()
	if len(errs) != 1 || errs[0] == nil {
		t.Fatalf("want one %s at %q, got %v", code, pointer, errs)
	}
	err := errs[0]
	if err.Code != code || err.Reason == "" {
		t.Errorf("code/reason = %q/%q", err.Code, err.Reason)
	}
	if err.Context.Path != context.Path || err.Context.FileName != context.FileName || err.Context.Line != context.Line || err.Context.Column != context.Column {
		t.Errorf("external context changed: %+v", err.Context)
	}
	for _, detail := range err.Details {
		if detail.Label == "Поле JSON" && detail.Value == pointer {
			return
		}
	}
	t.Errorf("missing pointer %q in %+v", pointer, err.Details)
}

func jsonShapeReplace(t *testing.T, raw, field, replacement string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, field)
	if replacement != "" {
		fields[field] = json.RawMessage(replacement)
	}
	result, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestLessonJSONShapeFields(t *testing.T) {
	type field struct {
		name, kind         string
		optional, nullable bool
	}
	tests := []struct {
		name, raw string
		check     func(json.RawMessage, string, diagnostic.Context) []*diagnostic.Error
		fields    []field
	}{
		{"lesson", `{"formatVersion":"1.0","document":` + jsonShapeDocument + `}`, func(r json.RawMessage, _ string, c diagnostic.Context) []*diagnostic.Error {
			return checkLessonJSON(r, c)
		}, []field{{"formatVersion", "string", false, false}, {"document", "object", false, false}}},
		{"document", jsonShapeDocument, checkDocumentJSON, []field{{"id", "string", false, false}, {"version", "number", false, false}, {"title", "string", false, false}, {"subtitle", "string", true, true}, {"lineCount", "number", false, false}, {"createdAt", "string", false, false}, {"pages", "array", false, false}}},
		{"page", jsonShapePage, checkPageJSON, []field{{"id", "string", false, false}, {"title", "string", false, true}, {"nodes", "array", false, false}}},
		{"node", jsonShapeNode, checkNodeJSON, []field{{"id", "string", false, false}, {"type", "string", false, false}, {"flags", "object", false, false}, {"data", "array", false, false}, {"nodes", "array", false, false}}},
		{"data", jsonShapeData, checkDataJSON, []field{{"type", "string", false, false}, {"role", "string", true, false}, {"value", "string", false, false}, {"answers", "array", false, false}, {"hint", "string", false, false}}},
		{"embedded", jsonShapeEmbedded, checkEmbeddedJSON, []field{{"fileName", "string", false, false}, {"mimeType", "string", false, false}, {"content", "string", false, false}}},
	}
	context := diagnostic.Context{Path: `C:\уроки.zip`, FileName: "l001.json", Line: 7, Column: 2}
	kinds := []struct{ kind, raw string }{{"string", `""`}, {"number", "2.5"}, {"boolean", "false"}, {"object", "{}"}, {"array", "[]"}, {"null", "null"}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prefix := "/x"
			if tc.name == "lesson" {
				prefix = ""
			}
			if errs := tc.check([]byte(tc.raw), prefix, context); len(errs) != 0 {
				t.Fatalf("valid shape: %v", errs)
			}
			for _, f := range tc.fields {
				t.Run(f.name, func(t *testing.T) {
					shadow := jsonShapeReplace(t, tc.raw, strings.ToUpper(f.name), `false`)
					if errs := tc.check(shadow, prefix, context); len(errs) != 0 {
						t.Fatalf("unknown case shadow: %v", errs)
					}
					for _, candidate := range append(kinds, struct{ kind, raw string }{"missing", ""}) {
						if candidate.kind == f.kind {
							continue
						}
						t.Run(candidate.kind, func(t *testing.T) {
							raw := jsonShapeReplace(t, tc.raw, f.name, candidate.raw)
							before := bytes.Clone(raw)
							errs := tc.check(raw, prefix, context)
							if (candidate.kind == "null" && f.nullable) || (candidate.kind == "missing" && f.optional) {
								if len(errs) != 0 {
									t.Fatalf("allowed optional/null: %v", errs)
								}
							} else {
								jsonShapeAssert(t, errs, CodeJSON, prefix+"/"+f.name, context)
							}
							if !bytes.Equal(raw, before) {
								t.Fatal("mutated raw")
							}
						})
					}
					upper := jsonShapeReplace(t, tc.raw, f.name, "")
					upper = jsonShapeReplace(t, string(upper), strings.ToUpper(f.name), `"shadow"`)
					if !f.optional {
						jsonShapeAssert(t, tc.check(upper, prefix, context), CodeJSON, prefix+"/"+f.name, context)
					}
				})
			}
			for _, wrong := range []string{"null", "[]", `"x"`, "1", "false"} {
				jsonShapeAssert(t, tc.check([]byte(wrong), prefix, context), CodeJSON, prefix, context)
			}
			unknown := jsonShapeReplace(t, tc.raw, "extra~/", `{"any":[null,false]}`)
			if errs := tc.check(unknown, prefix, context); len(errs) != 0 {
				t.Fatalf("additional property: %v", errs)
			}
		})
	}
}

func TestLessonJSONShapeRecursiveAndFlags(t *testing.T) {
	context := diagnostic.Context{Path: "урок.json", FileName: "l001.json"}
	tests := []struct {
		name, raw, pointer, code string
		check                    func(json.RawMessage, string, diagnostic.Context) []*diagnostic.Error
	}{
		{"page item", `{"pages":[null],"id":"x","version":1,"title":"","lineCount":0,"createdAt":"x"}`, "/x/pages/0", CodeJSON, checkDocumentJSON},
		{"node item", `{"id":"x","title":null,"nodes":[null]}`, "/x/nodes/0", CodeJSON, checkPageJSON},
		{"child node", string(jsonShapeReplace(t, jsonShapeNode, "nodes", `[null]`)), "/x/nodes/0", CodeJSON, checkNodeJSON},
		{"data item", string(jsonShapeReplace(t, jsonShapeNode, "data", `[false]`)), "/x/data/0", CodeJSON, checkNodeJSON},
		{"answer null", string(jsonShapeReplace(t, jsonShapeData, "answers", `["ok",null]`)), "/x/answers/1", CodeJSON, checkDataJSON},
		{"answer number", string(jsonShapeReplace(t, jsonShapeData, "answers", `[3]`)), "/x/answers/0", CodeJSON, checkDataJSON},
		{"media null", `{"type":"media","value":null,"answers":[],"hint":""}`, "/x/value", CodeJSON, checkDataJSON},
		{"media array", `{"type":"media","value":[],"answers":[],"hint":""}`, "/x/value", CodeJSON, checkDataJSON},
		{"media field", `{"type":"media","value":{"fileName":"a","mimeType":""},"answers":[],"hint":""}`, "/x/value/content", CodeJSON, checkDataJSON},
		{"collection item", `{"type":"collection","value":[{"type":"collection","value":[null],"answers":[],"hint":""}],"answers":[],"hint":""}`, "/x/value/0/value/0", CodeJSON, checkDataJSON},
		{"collection null", `{"type":"collection","value":null,"answers":[],"hint":""}`, "/x/value", CodeJSON, checkDataJSON},
		{"collection object", `{"type":"collection","value":{},"answers":[],"hint":""}`, "/x/value", CodeJSON, checkDataJSON},
		{"flags null", `null`, "/x", CodeJSON, checkFlagsJSON},
		{"flags array", `[]`, "/x", CodeJSON, checkFlagsJSON},
		{"flags scalar", `false`, "/x", CodeJSON, checkFlagsJSON},
		{"unknown flag", `{"un~known/":false}`, "/x/un~0known~1", CodeContract, checkFlagsJSON},
		{"wrong case flag", `{"Speaking":false}`, "/x/Speaking", CodeContract, checkFlagsJSON},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jsonShapeAssert(t, tc.check([]byte(tc.raw), "/x", context), tc.code, tc.pointer, context)
		})
	}
	for _, flag := range []string{"speaking", "stepped", "checkable"} {
		for _, raw := range []string{"true", "false", "null", "0", `"false"`, "[]", "{}"} {
			errs := checkFlagsJSON([]byte(`{"`+flag+`":`+raw+`}`), "/x", context)
			if raw == "true" || raw == "false" {
				if len(errs) != 0 {
					t.Fatalf("boolean flag: %v", errs)
				}
			} else {
				jsonShapeAssert(t, errs, CodeJSON, "/x/"+flag, context)
			}
		}
	}
	if errs := checkDataJSON([]byte(`{"type":"collection","role":"options","value":[],"answers":[],"hint":""}`), "/x", context); len(errs) != 0 {
		t.Fatalf("shape imposed semantic cardinality: %v", errs)
	}
}

func TestLessonJSONShapeRequireField(t *testing.T) {
	context := diagnostic.Context{Path: "x", LessonID: "L", PageID: "P", Line: 4, Column: 9}
	for _, tc := range []struct{ kind, raw string }{{"string", `"строка"`}, {"number", "1e9999"}, {"boolean", "false"}, {"object", "{}"}, {"array", "[]"}} {
		fields := map[string]json.RawMessage{"a~/": []byte(tc.raw)}
		got, err := requireJSONField(fields, "a~/", tc.kind, false, "/root", context)
		if err != nil || !bytes.Equal(got, fields["a~/"]) {
			t.Fatalf("%s: %s, %v", tc.kind, got, err)
		}
		for _, nullable := range []bool{false, true} {
			for _, raw := range []string{"", "null"} {
				fields = map[string]json.RawMessage{}
				if raw != "" {
					fields["a~/"] = []byte(raw)
				}
				got, err = requireJSONField(fields, "a~/", tc.kind, nullable, "/root", context)
				if nullable && raw == "null" {
					if err != nil || string(got) != "null" {
						t.Fatal("nullable field lost")
					}
					continue
				}
				if got != nil {
					t.Errorf("invalid field returned %s", got)
				}
				jsonShapeAssert(t, []*diagnostic.Error{err}, CodeJSON, "/root/a~0~1", context)
				if err.Context != context {
					t.Errorf("context = %+v", err.Context)
				}
			}
		}
	}
}

func TestLessonJSONShapeDecodeUnion(t *testing.T) {
	tests := []struct {
		kind DataType
		raw  string
		want any
	}{
		{DataText, `"a\n_____{Ответ}<b>"`, "a\n_____{Ответ}<b>"},
		{DataMedia, `{"fileName":"a.bin","FILENAME":"shadow","mimeType":"x/a","MIMETYPE":null,"content":"AA==","CONTENT":false}`, EmbeddedFile{FileName: "a.bin", MIMEType: "x/a", Content: "AA=="}},
		{DataCollection, `[]`, []Data{}},
		{DataCollection, `[{"type":"collection","value":[{"type":"text","TYPE":"media","value":"first","VALUE":false,"answers":["a","b"],"hint":"h"},{"type":"media","value":` + jsonShapeEmbedded + `,"answers":[],"hint":""}],"answers":[],"hint":""}]`, []Data{{Type: DataCollection, Value: []Data{{Type: DataText, Value: "first", Answers: []string{"a", "b"}, Hint: "h"}, {Type: DataMedia, Value: EmbeddedFile{FileName: "a.bin"}, Answers: []string{}, Hint: ""}}, Answers: []string{}, Hint: ""}}},
	}
	for _, tc := range tests {
		got, err := decodeDataValue(tc.kind, []byte(tc.raw))
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %#v, %v; want %#v", tc.kind, got, err, tc.want)
		}
	}
	for _, tc := range []struct {
		kind DataType
		raw  string
	}{{"unknown", `"x"`}, {DataText, "{}"}, {DataMedia, "[]"}, {DataCollection, `"x"`}} {
		_, err := decodeDataValue(tc.kind, []byte(tc.raw))
		if err == nil || err.Code != CodeContract {
			t.Errorf("%s/%s: %v", tc.kind, tc.raw, err)
		}
	}
}

func TestLessonJSONShapePublic(t *testing.T) {
	context := diagnostic.Context{Path: `C:\уроки.zip`, FileName: "l001.json", Line: 2, Column: 3}
	fixture, err := os.ReadFile("testdata/validate_json/case.json")
	if err != nil {
		t.Fatal(err)
	}
	beforeFixture := bytes.Clone(fixture)
	if errs := ValidateJSON(fixture, context); len(errs) != 0 {
		t.Fatalf("fixture: %v", errs)
	}
	var decoded Lesson
	if err := json.Unmarshal(fixture, &decoded); err != nil {
		t.Fatal(err)
	}
	page := decoded.Document.Pages[0]
	node := page.Nodes[0]
	if decoded.Document.ID != "Lesson" || decoded.Document.Subtitle != nil || page.ID != "lesson-p001" || page.Title != nil || node.ID != "lesson-n001" || node.Type != NodeText || node.Flags.Checkable != nil || node.Data[0].Value != "текст\n_____{Ответ}" {
		t.Fatalf("case-sensitive decoding: %+v", decoded)
	}
	if !bytes.Equal(fixture, beforeFixture) {
		t.Fatal("fixture mutated")
	}
	raw := []byte(`{"formatVersion":"1.0","FORMATVERSION":null,"document":` + jsonShapeDocument + `,"DOCUMENT":false}`)
	for _, number := range []string{"1.00", "1e1000", "1234567890123456789012345678901234567890"} {
		doc := jsonShapeReplace(t, jsonShapeDocument, "version", number)
		doc = jsonShapeReplace(t, string(doc), "lineCount", "0.00e999")
		doc = jsonShapeReplace(t, string(doc), "TITLE", "null")
		valid := jsonShapeReplace(t, string(raw), "document", string(doc))
		before := bytes.Clone(valid)
		if errs := ValidateJSON(valid, context); len(errs) != 0 {
			t.Fatalf("valid numbers/case: %v", errs)
		}
		var value Lesson
		if err := json.Unmarshal(valid, &value); err != nil {
			t.Fatal(err)
		}
		if string(value.Document.Version) != number || string(value.Document.LineCount) != "0.00e999" || value.Document.Title != "" || value.Document.Subtitle != nil {
			t.Fatalf("decoded document: %+v", value.Document)
		}
		if !bytes.Equal(valid, before) {
			t.Fatal("ValidateJSON mutated input")
		}
	}
	for _, invalid := range []string{"", "{", string(raw) + " {}", string(raw) + " true", `{"formatVersion":"1.0","document":` + jsonShapeDocument + `,"unknown":[}`} {
		errs := ValidateJSON([]byte(invalid), context)
		jsonShapeAssert(t, errs, CodeJSON, "", context)
		if errs[0].Cause == nil {
			t.Error("syntax error lost cause")
		}
	}
	bad := jsonShapeReplace(t, jsonShapeDocument, "title", "")
	bad = jsonShapeReplace(t, string(bad), "version", "-1")
	bad = jsonShapeReplace(t, string(bad), "id", `"bad id"`)
	jsonShapeAssert(t, ValidateJSON(jsonShapeReplace(t, string(raw), "document", string(bad)), context), CodeJSON, "/document/title", context)
	nested := strings.Replace(string(fixture), `"answers": []`, `"answers": [null]`, 1)
	jsonShapeAssert(t, ValidateJSON([]byte(nested), context), CodeJSON, "/document/pages/0/nodes/0/data/0/answers/0", context)
	unsupported := jsonShapeReplace(t, string(raw), "formatVersion", `"2.0"`)
	jsonShapeAssert(t, ValidateJSON(unsupported, context), CodeContract, "/formatVersion", context)
}
