package model

import (
	"encoding/json"
	"testing"
)

func TestResultPreservesPopulatedJSON(t *testing.T) {
	dslVersion, fileName, filePath, encoding := "1.2", "урок.dsl", "/уроки/урок.dsl", "UTF-8"
	hasBOM, lineCount, byteLength := true, 3, int64(123)
	sha256 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	documentID, title, subtitle, section := "Lesson-A", "Урок 🌍", "Подзаголовок", "Свободная секция"
	order := "0009007199254740993"
	version, startedAt, duration := "2.3.4", "2026-10-04T12:00:00.123+03:00", 17
	parent, tag, open, close, content := 1, "text", "{", "}", "Привет 🌍"
	location := Location{Start: Position{Line: 2, Column: 2}, End: Position{Line: 2, Column: 10}}
	result := Result{
		Format: FormatVersion1,
		Document: Document{
			DSLVersion: &dslVersion, FileName: &fileName, FilePath: &filePath, Encoding: &encoding,
			HasBOM: &hasBOM, LineCount: &lineCount, ByteLength: &byteLength, SHA256: &sha256,
			Metadata: DocumentMetadata{
				DocumentID: &documentID, Title: &title, Subtitle: &subtitle, Section: &section,
				Order: &order, ResourceDirs: []string{"ресурсы/z", "ресурсы/a"},
			},
			HasErrors: true,
		},
		Processing: []Processing{
			{ID: "run-z", Tool: "dsl-parser", Version: &version, StartedAt: &startedAt, DurationMs: &duration},
			{ID: "run-a", Tool: "dsl-validator"},
		},
		Lines: []Line{
			{Number: 1, LineType: LineTypeBlockStart, Raw: "@text {", LineEnding: LineEndingLF,
				Elements: []Element{
					{ElementType: ElementTypeTag, Raw: "@text", Value: &tag, Start: 1, End: 6, ErrorIDs: []string{}},
					{ElementType: ElementTypeBlockOpen, Raw: "{", Value: &open, Start: 7, End: 8, ErrorIDs: []string{}},
				}},
			{Number: 2, LineType: LineTypeContent, NestingLevel: 1, ParentLine: &parent,
				Raw: "\tПривет 🌍", LineEnding: LineEndingCRLF, HasErrors: true,
				Elements: []Element{
					{ElementType: ElementTypeContent, Raw: "Привет 🌍", Value: &content, Start: 2, End: 10,
						ErrorIDs: []string{"diag-z", "diag-a"}},
				}},
			{Number: 3, LineType: LineTypeBlockEnd, NestingLevel: 1, ParentLine: &parent,
				Raw: "}", LineEnding: LineEndingNone,
				Elements: []Element{
					{ElementType: ElementTypeBlockClose, Raw: "}", Value: &close, Start: 1, End: 2, ErrorIDs: []string{}},
				}},
		},
		Diagnostics: []Diagnostic{
			{ID: "diag-z", Source: "run-a", DiagnosticCode: Code("CUSTOM001"),
				SeverityLevel: SeverityError, Message: "Проблема в тексте 🌍",
				DiagnosticScope: ScopeElement, Location: &location,
				RelatedLocations: []Location{
					{Start: Position{Line: 3, Column: 1}, End: Position{Line: 3, Column: 2}},
					{Start: Position{Line: 1, Column: 1}, End: Position{Line: 1, Column: 6}},
				}},
			{ID: "diag-a", Source: "run-a", DiagnosticCode: Code("CUSTOM002"),
				SeverityLevel: SeverityWarning, Message: "Предупреждение",
				DiagnosticScope: ScopeElement, Location: &location, RelatedLocations: []Location{}},
		},
	}
	assertModelJSON(t, result, `{
		"formatVersion":"1.0",
		"document":{
			"dslVersion":"1.2","fileName":"урок.dsl","filePath":"/уроки/урок.dsl","encoding":"UTF-8",
			"hasBom":true,"lineCount":3,"byteLength":123,
			"sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"metadata":{"documentId":"Lesson-A","title":"Урок 🌍","subtitle":"Подзаголовок",
				"section":"Свободная секция","order":"0009007199254740993","resourceDirs":["ресурсы/z","ресурсы/a"]},
			"hasErrors":true
		},
		"processing":[
			{"id":"run-z","tool":"dsl-parser","version":"2.3.4","startedAt":"2026-10-04T12:00:00.123+03:00","durationMs":17},
			{"id":"run-a","tool":"dsl-validator","version":null,"startedAt":null,"durationMs":null}
		],
		"lines":[
			{"line":1,"type":"block-start","nestingLevel":0,"parentLine":null,"raw":"@text {","eol":"\n","hasErrors":false,
				"elements":[{"type":"tag","raw":"@text","value":"text","start":1,"end":6,"errorIds":[]},
					{"type":"block-open","raw":"{","value":"{","start":7,"end":8,"errorIds":[]}]},
			{"line":2,"type":"content","nestingLevel":1,"parentLine":1,"raw":"\tПривет 🌍","eol":"\r\n","hasErrors":true,
				"elements":[{"type":"content","raw":"Привет 🌍","value":"Привет 🌍","start":2,"end":10,"errorIds":["diag-z","diag-a"]}]},
			{"line":3,"type":"block-end","nestingLevel":1,"parentLine":1,"raw":"}","eol":"","hasErrors":false,
				"elements":[{"type":"block-close","raw":"}","value":"}","start":1,"end":2,"errorIds":[]}]}
		],
		"diagnostics":[
			{"id":"diag-z","source":"run-a","code":"CUSTOM001","severity":"error","message":"Проблема в тексте 🌍",
				"scope":"element","fatal":false,"location":{"start":{"line":2,"column":2},"end":{"line":2,"column":10}},
				"relatedLocations":[{"start":{"line":3,"column":1},"end":{"line":3,"column":2}},
					{"start":{"line":1,"column":1},"end":{"line":1,"column":6}}]},
			{"id":"diag-a","source":"run-a","code":"CUSTOM002","severity":"warning","message":"Предупреждение",
				"scope":"element","fatal":false,"location":{"start":{"line":2,"column":2},"end":{"line":2,"column":10}},"relatedLocations":[]}
		]
	}`)
}

func TestResultKeepsZeroFieldsAndNilSlices(t *testing.T) {
	assertModelJSON(t, Result{}, `{
		"formatVersion":"","document":{
			"dslVersion":null,"fileName":null,"filePath":null,"encoding":null,"hasBom":null,
			"lineCount":null,"byteLength":null,"sha256":null,
			"metadata":{"documentId":null,"title":null,"subtitle":null,"section":null,"order":null,"resourceDirs":null},
			"hasErrors":false},"processing":null,"lines":null,"diagnostics":null
	}`)
	assertModelJSON(t, Processing{}, `{"id":"","tool":"","version":null,"startedAt":null,"durationMs":null}`)
	assertModelJSON(t, Line{}, `{"line":0,"type":"","nestingLevel":0,"parentLine":null,"raw":"","eol":"","hasErrors":false,"elements":null}`)
	assertModelJSON(t, Element{}, `{"type":"","raw":"","value":null,"start":0,"end":0,"errorIds":null}`)
	assertModelJSON(t, Diagnostic{}, `{
		"id":"","source":"","code":"","severity":"","message":"","scope":"","fatal":false,"location":null,"relatedLocations":null
	}`)
}

func TestModelDistinguishesKnownZeroValuesFromNull(t *testing.T) {
	empty, zero, zeroBytes, noBOM := "", 0, int64(0), false
	assertModelJSON(t, Document{
		DSLVersion: &empty, FileName: &empty, FilePath: &empty, Encoding: &empty,
		HasBOM: &noBOM, LineCount: &zero, ByteLength: &zeroBytes, SHA256: &empty,
		Metadata: DocumentMetadata{
			DocumentID: &empty, Title: &empty, Subtitle: &empty, Section: &empty, Order: &empty, ResourceDirs: []string{},
		},
	}, `{"dslVersion":"","fileName":"","filePath":"","encoding":"","hasBom":false,
		"lineCount":0,"byteLength":0,"sha256":"",
		"metadata":{"documentId":"","title":"","subtitle":"","section":"","order":"","resourceDirs":[]},"hasErrors":false}`)
	assertModelJSON(t, Processing{Version: &empty, StartedAt: &empty, DurationMs: &zero},
		`{"id":"","tool":"","version":"","startedAt":"","durationMs":0}`)
	assertModelJSON(t, Line{ParentLine: &zero, Elements: []Element{}},
		`{"line":0,"type":"","nestingLevel":0,"parentLine":0,"raw":"","eol":"","hasErrors":false,"elements":[]}`)
	assertModelJSON(t, Element{Value: &empty, Start: 5, End: 5, ErrorIDs: []string{}},
		`{"type":"","raw":"","value":"","start":5,"end":5,"errorIds":[]}`)
	assertModelJSON(t, Diagnostic{Location: &Location{}, RelatedLocations: []Location{}},
		`{"id":"","source":"","code":"","severity":"","message":"","scope":"","fatal":false,
		"location":{"start":{"line":0,"column":0},"end":{"line":0,"column":0}},"relatedLocations":[]}`)
}

func TestResultPreservesEmptyArrays(t *testing.T) {
	assertModelJSON(t, Result{
		Format: FormatVersion1, Document: Document{Metadata: DocumentMetadata{ResourceDirs: []string{}}},
		Processing: []Processing{}, Lines: []Line{}, Diagnostics: []Diagnostic{},
	}, `{"formatVersion":"1.0","document":{
		"dslVersion":null,"fileName":null,"filePath":null,"encoding":null,"hasBom":null,"lineCount":null,"byteLength":null,"sha256":null,
		"metadata":{"documentId":null,"title":null,"subtitle":null,"section":null,"order":null,"resourceDirs":[]},"hasErrors":false},
		"processing":[],"lines":[],"diagnostics":[]}`)
}

func TestDiagnosticPreservesLocationsAndFatalFlag(t *testing.T) {
	tests := []struct {
		name       string
		diagnostic Diagnostic
		want       string
	}{
		{name: "fatal_without_location", diagnostic: Diagnostic{
			ID: "read-error", Source: "run", DiagnosticCode: Code("IO001"), SeverityLevel: SeverityError,
			Message: "Не удалось прочитать файл", DiagnosticScope: ScopeDocument, Fatal: true, RelatedLocations: []Location{},
		}, want: `{"id":"read-error","source":"run","code":"IO001","severity":"error","message":"Не удалось прочитать файл",
			"scope":"document","fatal":true,"location":null,"relatedLocations":[]}`},
		{name: "multiline_range", diagnostic: Diagnostic{
			ID: "block-error", Source: "run", DiagnosticCode: Code("P011"), SeverityLevel: SeverityError,
			Message: "Незакрытый блок", DiagnosticScope: ScopeBlock,
			Location: &Location{Start: Position{Line: 1, Column: 7}, End: Position{Line: 3, Column: 2}}, RelatedLocations: []Location{},
		}, want: `{"id":"block-error","source":"run","code":"P011","severity":"error","message":"Незакрытый блок",
			"scope":"block","fatal":false,"location":{"start":{"line":1,"column":7},"end":{"line":3,"column":2}},"relatedLocations":[]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertModelJSON(t, test.diagnostic, test.want)
		})
	}
}

func assertModelJSON[T any](t *testing.T, model T, want string) {
	t.Helper()
	encoded, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("JSON-маршалинг: %v", err)
	}
	if !json.Valid([]byte(want)) {
		t.Fatal("Некорректный ожидаемый JSON")
	}
	assertJSONEqual(t, encoded, json.RawMessage(want))
}

// assertJSONEqual сравнивает JSON без требования к порядку ключей объектов.
func assertJSONEqual(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	var gotObject, wantObject map[string]json.RawMessage
	if json.Unmarshal(want, &wantObject) == nil && wantObject != nil {
		if err := json.Unmarshal(got, &gotObject); err != nil || len(gotObject) != len(wantObject) {
			t.Fatalf("JSON-объект = %s, требуется %s", got, want)
		}
		for key, expected := range wantObject {
			actual, exists := gotObject[key]
			if !exists {
				t.Fatalf("Нет JSON-поля %q в %s", key, got)
			}
			assertJSONEqual(t, actual, expected)
		}
		return
	}
	var gotArray, wantArray []json.RawMessage
	if json.Unmarshal(want, &wantArray) == nil && wantArray != nil {
		if err := json.Unmarshal(got, &gotArray); err != nil || gotArray == nil || len(gotArray) != len(wantArray) {
			t.Fatalf("JSON-массив = %s, требуется %s", got, want)
		}
		for i := range wantArray {
			assertJSONEqual(t, gotArray[i], wantArray[i])
		}
		return
	}
	if string(got) != string(want) {
		t.Errorf("JSON-значение = %s, требуется %s", got, want)
	}
}
