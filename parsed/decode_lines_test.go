package parsed

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

const (
	decodeLinesTestPosition   = `{"line":3,"column":7}`
	decodeLinesTestLocation   = `{"start":{"line":3,"column":7},"end":{"line":4,"column":9}}`
	decodeLinesTestElement    = `{"type":"content","raw":" \\@ёж😀 ","value":" \\n\\@ёж😀\n ","start":2,"end":9,"errorIds":["second","first"]}`
	decodeLinesTestLine       = `{"line":3,"type":"content","nestingLevel":2,"parentLine":1,"raw":" \t\\@ёж😀 ","eol":"\r\n","hasErrors":true,"elements":[` + decodeLinesTestElement + `]}`
	decodeLinesTestDiagnostic = `{"id":"d2","source":"p1","code":"custom.code","severity":"warning","message":" Текст\n\\n ","scope":"block","fatal":false,"location":` + decodeLinesTestLocation + `,"relatedLocations":[{"start":{"line":8,"column":2},"end":{"line":8,"column":3}},` + decodeLinesTestLocation + `]}`
)

type decodeLinesTestDecoder struct {
	name   string
	base   string
	fields string
	kinds  string
	want   any
	decode func(json.RawMessage, string, diagnostic.Context) (any, *diagnostic.Error)
}

func decodeLinesTestDecoders() []decodeLinesTestDecoder {
	return []decodeLinesTestDecoder{
		{"line", decodeLinesTestLine, "line type nestingLevel parentLine raw eol hasErrors elements", "int string int nullableInt string string bool array", new(model.Line), func(r json.RawMessage, f string, c diagnostic.Context) (any, *diagnostic.Error) {
			return decodeLine(r, f, c)
		}},
		{"element", decodeLinesTestElement, "type raw value start end errorIds", "string string nullableString int int array", new(model.Element), func(r json.RawMessage, f string, c diagnostic.Context) (any, *diagnostic.Error) {
			return decodeElement(r, f, c)
		}},
		{"diagnostic", decodeLinesTestDiagnostic, "id source code severity message scope fatal location relatedLocations", "string string string string string string bool nullableObject array", new(model.Diagnostic), func(r json.RawMessage, f string, c diagnostic.Context) (any, *diagnostic.Error) {
			return decodeDiagnostic(r, f, c)
		}},
		{"location", decodeLinesTestLocation, "start end", "object object", new(model.Location), func(r json.RawMessage, f string, c diagnostic.Context) (any, *diagnostic.Error) {
			return decodeLocation(r, f, c)
		}},
		{"position", decodeLinesTestPosition, "line column", "int int", new(model.Position), func(r json.RawMessage, f string, c diagnostic.Context) (any, *diagnostic.Error) {
			return decodePosition(r, f, c)
		}},
	}
}

func decodeLinesTestObject(t *testing.T, base string, key string, replacement string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(base), &fields); err != nil {
		t.Fatal(err)
	}
	if replacement == "" {
		delete(fields, key)
	} else {
		fields[key] = json.RawMessage(replacement)
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func decodeLinesTestContext() diagnostic.Context {
	return diagnostic.Context{Path: `C:\уроки\вход.json`, FileName: "l003.json", LessonID: "Ab:C+7", PageID: "ab:c+7-p003", Line: 41, Column: 12}
}

func decodeLinesTestError(t *testing.T, err *diagnostic.Error) {
	t.Helper()
	if err == nil {
		t.Fatal("ожидалась ошибка формы Contract")
	}
	wantContext := decodeLinesTestContext()
	if err.Code != CodeContract || err.Context.Path != wantContext.Path || err.Context.FileName != wantContext.FileName || err.Context.LessonID != wantContext.LessonID || err.Context.PageID != wantContext.PageID || err.Reason == "" {
		t.Fatalf("ошибка потеряла код, контекст или причину: %#v", err)
	}
	for _, detail := range err.Details {
		if strings.HasPrefix(detail.Value, "root[2]") {
			return
		}
	}
	t.Fatalf("нет пути ошибочного JSON-поля в Details: %#v", err)
}

func TestDecodeLinesRequiredExactKeysAndUnknownFields(t *testing.T) {
	for _, decoder := range decodeLinesTestDecoders() {
		t.Run(decoder.name, func(t *testing.T) {
			want, err := decoder.decode(json.RawMessage(decoder.base), "root[2]", decodeLinesTestContext())
			if err != nil {
				t.Fatalf("полная форма: %v", err)
			}
			if err := json.Unmarshal([]byte(decoder.base), decoder.want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, reflect.ValueOf(decoder.want).Elem().Interface()) {
				t.Fatalf("полная форма потеряла значения: %#v, нужно %#v", want, decoder.want)
			}
			for _, key := range strings.Fields(decoder.fields) {
				t.Run("missing/"+key, func(t *testing.T) {
					_, err := decoder.decode(decodeLinesTestObject(t, decoder.base, key, ""), "root[2]", decodeLinesTestContext())
					decodeLinesTestError(t, err)
				})
				t.Run("case/"+key, func(t *testing.T) {
					raw := decodeLinesTestObject(t, decoder.base, key, "")
					var fields map[string]json.RawMessage
					if err := json.Unmarshal([]byte(decoder.base), &fields); err != nil {
						t.Fatal(err)
					}
					raw = decodeLinesTestObject(t, string(raw), strings.ToUpper(key), string(fields[key]))
					_, err := decoder.decode(raw, "root[2]", decodeLinesTestContext())
					decodeLinesTestError(t, err)
				})
			}
			for _, key := range append(strings.Fields(strings.ToUpper(decoder.fields)), "futureField") {
				raw := decodeLinesTestObject(t, decoder.base, key, `{"deep":[null,{},false]}`)
				got, err := decoder.decode(raw, "root[2]", decodeLinesTestContext())
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("неизвестное поле %s изменило результат: %#v, %v", key, got, err)
				}
			}
		})
	}
}

func TestDecodeLinesRejectWrongTypesAndNull(t *testing.T) {
	invalid := map[string][]string{
		"string":         {`null`, `false`, `1`, `{}`, `[]`},
		"nullableString": {`false`, `1`, `{}`, `[]`},
		"bool":           {`null`, `"false"`, `0`, `{}`, `[]`},
		"int":            {`null`, `true`, `"1"`, `1.5`, `1.0`, `1e0`, `9223372036854775808`, `{}`, `[]`},
		"nullableInt":    {`true`, `"1"`, `1.5`, `1.0`, `1e0`, `9223372036854775808`, `{}`, `[]`},
		"array":          {`null`, `false`, `1`, `"[]"`, `{}`},
		"object":         {`null`, `false`, `1`, `"{}"`, `[]`},
		"nullableObject": {`false`, `1`, `"{}"`, `[]`},
	}
	for _, decoder := range decodeLinesTestDecoders() {
		t.Run(decoder.name, func(t *testing.T) {
			for _, raw := range []string{"", `null`, `false`, `1`, `"object"`, `[]`, `[{}]`} {
				t.Run("object/"+raw, func(t *testing.T) {
					_, err := decoder.decode(json.RawMessage(raw), "root[2]", decodeLinesTestContext())
					decodeLinesTestError(t, err)
				})
			}
			kinds := strings.Fields(decoder.kinds)
			for i, key := range strings.Fields(decoder.fields) {
				for _, raw := range invalid[kinds[i]] {
					t.Run(key+"/"+raw, func(t *testing.T) {
						_, err := decoder.decode(decodeLinesTestObject(t, decoder.base, key, raw), "root[2]", decodeLinesTestContext())
						decodeLinesTestError(t, err)
					})
				}
			}
		})
	}
}

func TestDecodeLinesRejectMalformedNestedItems(t *testing.T) {
	for _, tc := range []struct {
		decoder  int
		key, raw string
	}{
		{0, "elements", `[null]`}, {0, "elements", `[1]`}, {0, "elements", `[{}]`},
		{0, "elements", `[` + decodeLinesTestElement + `,null]`},
		{0, "elements", `[{"type":"content","raw":"x","value":false,"start":1,"end":2,"errorIds":[]}]`},
		{1, "errorIds", `[null]`}, {1, "errorIds", `[1]`}, {1, "errorIds", `[false]`}, {1, "errorIds", `[{}]`}, {1, "errorIds", `[[]]`}, {1, "errorIds", `["valid",null]`},
		{2, "location", `{}`}, {2, "location", `{"start":null,"end":` + decodeLinesTestPosition + `}`},
		{2, "relatedLocations", `[null]`}, {2, "relatedLocations", `[{}]`}, {2, "relatedLocations", `[1]`}, {2, "relatedLocations", `[` + decodeLinesTestLocation + `,null]`},
		{2, "relatedLocations", `[{"start":{"line":1},"end":` + decodeLinesTestPosition + `}]`},
		{3, "start", `{}`}, {3, "end", `{"line":1}`}, {3, "start", `{"line":1,"column":null}`}, {3, "end", `{"line":1.5,"column":2}`},
	} {
		decoder := decodeLinesTestDecoders()[tc.decoder]
		t.Run(decoder.name+"/"+tc.key+"/"+tc.raw, func(t *testing.T) {
			_, err := decoder.decode(decodeLinesTestObject(t, decoder.base, tc.key, tc.raw), "root[2]", decodeLinesTestContext())
			decodeLinesTestError(t, err)
		})
	}
}

func TestDecodeLinesPreserveValuesAndLeaveSemanticsForValidation(t *testing.T) {
	element, err := decodeElement(json.RawMessage(decodeLinesTestElement), "element", decodeLinesTestContext())
	if err != nil {
		t.Fatal(err)
	}
	if element.ElementType != model.ElementTypeContent || element.Raw != ` \@ёж😀 ` || element.Value == nil || *element.Value != " \\n\\@ёж😀\n " || element.Start != 2 || element.End != 9 || !reflect.DeepEqual(element.ErrorIDs, []string{"second", "first"}) {
		t.Fatalf("изменены исходные строки/порядок/координаты: %#v", element)
	}
	for _, value := range []string{`null`, `""`, `"000000000000000000000000123"`, `"текст с пробелами"`} {
		got, err := decodeElement(decodeLinesTestObject(t, decodeLinesTestElement, "value", value), "element", decodeLinesTestContext())
		var want *string
		if unmarshalErr := json.Unmarshal([]byte(value), &want); unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}
		if err != nil || !reflect.DeepEqual(got.Value, want) {
			t.Fatalf("value=%s: %#v, %v", value, got, err)
		}
	}
	rawDiagnostic := decodeLinesTestObject(t, decodeLinesTestDiagnostic, "scope", `"element"`)
	rawDiagnostic = decodeLinesTestObject(t, string(rawDiagnostic), "location", `null`)
	item, diagnosticErr := decodeDiagnostic(rawDiagnostic, "diagnostic", decodeLinesTestContext())
	if diagnosticErr != nil || item.Location != nil || item.DiagnosticScope != model.ScopeElement {
		t.Fatalf("семантическая обязательность location проверена на слое формы: %#v, %v", item, diagnosticErr)
	}
	for _, tc := range []struct {
		decoder  int
		key, raw string
	}{
		{0, "line", `-2`}, {0, "nestingLevel", `-1`}, {0, "parentLine", `null`}, {0, "parentLine", `0`}, {0, "type", `"future"`}, {0, "type", `"invalid"`}, {0, "eol", `"\r"`}, {0, "elements", `[]`},
		{0, "elements", `[` + decodeLinesTestElement + `,{"type":"placeholder","raw":"_____","value":null,"start":1,"end":6,"errorIds":[]}]`},
		{1, "type", `"unparsed"`}, {1, "type", `"future"`}, {1, "start", `0`}, {1, "end", `-1`}, {1, "errorIds", `[]`}, {1, "errorIds", `["x","x"]`},
		{2, "id", `""`}, {2, "severity", `"future"`}, {2, "severity", `"error"`}, {2, "scope", `"future"`}, {2, "fatal", `true`}, {2, "location", `null`}, {2, "relatedLocations", `[]`},
		{3, "end", `{"line":-1,"column":0}`}, {4, "line", `0`}, {4, "column", `-1`}, {4, "column", `9007199254740993`},
		{0, "line", `9007199254740993`}, {0, "nestingLevel", `9007199254740993`}, {0, "parentLine", `9007199254740993`},
		{1, "start", `9007199254740993`}, {1, "end", `9007199254740993`}, {4, "line", `9007199254740993`},
	} {
		decoder := decodeLinesTestDecoders()[tc.decoder]
		t.Run(decoder.name+"/"+tc.key+"/"+tc.raw, func(t *testing.T) {
			raw := decodeLinesTestObject(t, decoder.base, tc.key, tc.raw)
			got, err := decoder.decode(raw, "root[2]", decodeLinesTestContext())
			if err != nil {
				t.Fatalf("декодер формы выполнил семантическую проверку: %v", err)
			}
			encoded, marshalErr := json.Marshal(got)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields[tc.key]) != tc.raw {
				t.Fatalf("%s: сохранено %s, нужно %s", tc.key, fields[tc.key], tc.raw)
			}
		})
	}
}
