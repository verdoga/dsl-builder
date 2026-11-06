package lesson

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func jsonModelRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/json_model/recursive.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func jsonModelReplace(t *testing.T, raw []byte, old, replacement string) []byte {
	t.Helper()
	if bytes.Count(raw, []byte(old)) != 1 {
		t.Fatalf("fixture must contain exactly one %q", old)
	}
	return bytes.Replace(raw, []byte(old), []byte(replacement), 1)
}

func jsonModelExpected() Lesson {
	body, subtitle, pageTitle := RoleBody, "Подзаголовок", "Вторая"
	no, yes := false, true
	text := Data{Type: DataText, Value: "<b>Строка</b>\n\n{a|б}", Answers: []string{"β", "а"}, Hint: "подсказка"}
	media := Data{Type: DataMedia, Value: EmbeddedFile{FileName: "звук.bin", MIMEType: "application/custom", Content: "AAE="}, Answers: []string{}, Hint: ""}
	empty := Data{Type: DataCollection, Value: []Data{}, Answers: []string{}, Hint: ""}
	inner := Data{Type: DataCollection, Value: []Data{media, empty}, Answers: []string{}, Hint: ""}
	outer := Data{Type: DataCollection, Role: &body, Value: []Data{text, inner}, Answers: []string{}, Hint: ""}
	question := Node{ID: "abc-12-n002", Type: NodeQuestion, Flags: NodeFlags{Checkable: &no},
		Data: []Data{{Type: DataText, Role: &body, Value: "Вопрос?", Answers: []string{}, Hint: ""}}, Nodes: []Node{}}
	leaf := Node{ID: "abc-12-n003", Type: NodeText, Data: []Data{}, Nodes: []Node{}}
	task := Node{ID: "abc-12-n001", Type: NodeTask, Flags: NodeFlags{Speaking: &no, Stepped: &yes},
		Data: []Data{outer}, Nodes: []Node{question, leaf}}
	return Lesson{FormatVersion: "1.0", Document: Document{ID: "Abc-12",
		Version: json.Number("1234567890123456789012345678901234567890.0"), Title: "Урок 🌍", Subtitle: &subtitle,
		LineCount: json.Number("12e+3"), CreatedAt: "2026-11-06T12:34:56Z",
		Pages: []Page{{ID: "abc-12-p001", Title: nil, Nodes: []Node{task}}, {ID: "abc-12-p002", Title: &pageTitle, Nodes: []Node{}}}}}
}

func TestLessonJSONModelExactRecursiveDecode(t *testing.T) {
	for _, standard := range []bool{false, true} {
		t.Run(map[bool]string{false: "method", true: "encoding_json"}[standard], func(t *testing.T) {
			raw := jsonModelRaw(t)
			original := bytes.Clone(raw)
			got := jsonModelExpected()
			got.FormatVersion, got.Document.Title = "stale", "старое"
			got.Document.Pages[0].Nodes[0].Flags.Checkable = new(bool)
			got.Document.Pages[0].Nodes[0].Nodes[1].Flags.Speaking = new(bool)
			got.Document.Pages = append(got.Document.Pages, Page{ID: "stale"})
			var err error
			if standard {
				err = json.Unmarshal(raw, &got)
			} else {
				err = got.UnmarshalJSON(raw)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, jsonModelExpected()) {
				t.Fatalf("decoded model differs: %#v", got)
			}
			if !bytes.Equal(raw, original) {
				t.Fatal("decoder changed raw input")
			}
		})
	}
}

func TestLessonJSONModelCheckedHelpers(t *testing.T) {
	raw := jsonModelRaw(t)
	if errs := checkLessonJSON(raw, diagnostic.Context{}); len(errs) != 0 {
		t.Fatalf("fixture shape: %v", errs)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	doc, err := decodeDocumentJSON(root["document"])
	if err != nil || !reflect.DeepEqual(doc, jsonModelExpected().Document) {
		t.Fatalf("document: %#v; %v", doc, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(root["document"], &fields); err != nil {
		t.Fatal(err)
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(fields["pages"], &pages); err != nil {
		t.Fatal(err)
	}
	page, err := decodePageJSON(pages[0])
	if err != nil || !reflect.DeepEqual(page, doc.Pages[0]) {
		t.Fatalf("page: %#v; %v", page, err)
	}
	if err := json.Unmarshal(pages[0], &fields); err != nil {
		t.Fatal(err)
	}
	var nodes []json.RawMessage
	if err := json.Unmarshal(fields["nodes"], &nodes); err != nil {
		t.Fatal(err)
	}
	node, err := decodeNodeJSON(nodes[0])
	if err != nil || !reflect.DeepEqual(node, page.Nodes[0]) {
		t.Fatalf("node: %#v; %v", node, err)
	}
	for _, tc := range []struct {
		raw  string
		want NodeFlags
	}{{`{}`, NodeFlags{}}, {`{"speaking":false,"stepped":true}`, node.Flags}, {`{"checkable":false}`, node.Nodes[0].Flags}} {
		flags, err := decodeFlagsJSON(json.RawMessage(tc.raw))
		if err != nil || !reflect.DeepEqual(flags, tc.want) {
			t.Errorf("flags %s: %#v; %v", tc.raw, flags, err)
		}
	}
}

func TestLessonJSONModelOptionalSubtitleAndReplacement(t *testing.T) {
	for _, replacement := range []string{"", `"subtitle": null,`, `"subtitle": "",`} {
		t.Run(replacement, func(t *testing.T) {
			raw := jsonModelReplace(t, jsonModelRaw(t), `"subtitle": "Подзаголовок",`, replacement)
			got := jsonModelExpected()
			if err := got.UnmarshalJSON(raw); err != nil {
				t.Fatal(err)
			}
			want := jsonModelExpected()
			want.Document.Subtitle = nil
			if replacement == `"subtitle": "",` {
				empty := ""
				want.Document.Subtitle = &empty
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("optional subtitle retained stale state: %#v", got.Document)
			}
		})
	}
}

func TestLessonJSONModelNumberLexemesAndSemanticBoundary(t *testing.T) {
	for _, number := range []string{"9007199254740993", "1.000", "1e10000", "-1.25"} {
		t.Run(number, func(t *testing.T) {
			raw := jsonModelRaw(t)
			raw = jsonModelReplace(t, raw, "1234567890123456789012345678901234567890.0", number)
			raw = jsonModelReplace(t, raw, "12e+3", number)
			raw = jsonModelReplace(t, raw, `"formatVersion": "1.0"`, `"formatVersion": "future"`)
			raw = jsonModelReplace(t, raw, `"2026-11-06T12:34:56Z"`, `"not-a-date"`)
			var got Lesson
			if err := got.UnmarshalJSON(raw); err != nil {
				t.Fatal(err)
			}
			if got.Document.Version.String() != number || got.Document.LineCount.String() != number || got.FormatVersion != "future" || got.Document.CreatedAt != "not-a-date" {
				t.Fatalf("decoder changed lexemes or applied semantic validation: %#v", got.Document)
			}
		})
	}
}

func TestLessonJSONModelErrorsAreAtomicAndLocated(t *testing.T) {
	const node = "/document/pages/0/nodes/0"
	for _, tc := range []struct {
		name, old, replacement, pointer string
		wantCode                        string
		page                            bool
	}{
		{"root_type", `"formatVersion": "1.0"`, `"formatVersion": null`, "/formatVersion", CodeJSON, false},
		{"exact_root_key", `"formatVersion": "1.0",`, "", "/formatVersion", CodeJSON, false},
		{"exact_document_id", `"id": "Abc-12",`, "", "/document/id", CodeJSON, false},
		{"exact_document_title", `"title": "Урок 🌍",`, "", "/document/title", CodeJSON, false},
		{"page_title_required", `"title": null,`, "", "/document/pages/0/title", CodeJSON, true},
		{"page_nodes_null", "\"nodes\": [\n          {", "\"nodes\": null, \"extraNodes\": [\n          {", "/document/pages/0/nodes", CodeJSON, true},
		{"node_id_case", `"id": "abc-12-n001",`, "", node + "/id", CodeJSON, true},
		{"flags_null", `"flags": {"speaking": false, "stepped": true}`, `"flags": null`, node + "/flags", CodeJSON, true},
		{"flag_wrong_case", `"speaking": false`, `"SPEAKING": false`, node + "/flags/SPEAKING", CodeContract, true},
		{"flag_escaped_pointer", `"speaking": false`, `"a~/b": false`, node + "/flags/a~0~1b", CodeContract, true},
		{"child_flag_null", `"checkable": false`, `"checkable": null`, node + "/nodes/0/flags/checkable", CodeJSON, true},
		{"deep_answers_null", `"answers": ["β", "а"]`, `"answers": null`, node + "/data/0/value/0/answers", CodeJSON, true},
		{"deep_media_field_null", `"content": "AAE="`, `"content": null`, node + "/data/0/value/1/value/0/value/content", CodeJSON, true},
		{"syntax", `"formatVersion": "1.0"`, `"formatVersion":`, "", CodeJSON, false},
		{"trailing_value", "\n}\n", "\n}\n{}", "", CodeJSON, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := jsonModelReplace(t, jsonModelRaw(t), tc.old, tc.replacement)
			original := bytes.Clone(raw)
			got, before := jsonModelExpected(), jsonModelExpected()
			err := got.UnmarshalJSON(raw)
			var diagnosticError *diagnostic.Error
			if !errors.As(err, &diagnosticError) || diagnosticError == nil {
				t.Fatalf("want structured diagnostic, got %T: %v", err, err)
			}
			if diagnosticError.Code != tc.wantCode || !strings.ContainsFunc(diagnosticError.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
				t.Errorf("code/reason: %#v", diagnosticError)
			}
			located := false
			for _, detail := range diagnosticError.Details {
				located = located || detail.Label == "Поле JSON" && detail.Value == tc.pointer
			}
			if !located {
				t.Errorf("missing exact pointer %q: %#v", tc.pointer, diagnosticError.Details)
			}
			if tc.page && diagnosticError.Context.PageID != "abc-12-p001" {
				t.Errorf("lost available PageID: %#v", diagnosticError.Context)
			}
			if tc.name == "syntax" {
				var syntax *json.SyntaxError
				if !errors.As(err, &syntax) {
					t.Error("syntax error cause not preserved")
				}
			}
			if !reflect.DeepEqual(got, before) || !bytes.Equal(raw, original) {
				t.Error("failed decode changed receiver or raw input")
			}
		})
	}
}

func TestLessonJSONModelEmptyArraysReplacePreviousState(t *testing.T) {
	raw := []byte(`{"formatVersion":"1.0","document":{"id":"","version":0,"title":"","lineCount":0,"createdAt":"","pages":[]}}`)
	got := jsonModelExpected()
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := Lesson{FormatVersion: "1.0", Document: Document{Version: json.Number("0"), LineCount: json.Number("0"), Pages: []Page{}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stale receiver state: %#v", got)
	}
}
