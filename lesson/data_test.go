package lesson

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestLessonDataEnums(t *testing.T) {
	for _, tc := range []struct {
		got  DataType
		want string
	}{{DataText, "text"}, {DataMedia, "media"}, {DataCollection, "collection"}} {
		if string(tc.got) != tc.want {
			t.Errorf("DataType = %q, want %q", tc.got, tc.want)
		}
	}
	for _, tc := range []struct {
		got  DataRole
		want string
	}{
		{RoleTitle, "title"}, {RoleBody, "body"}, {RoleInstruction, "instruction"},
		{RoleBefore, "before"}, {RoleAfter, "after"}, {RoleItems, "items"},
		{RoleItem, "item"}, {RoleOptions, "options"}, {RoleOption, "option"},
		{RolePositions, "positions"}, {RolePosition, "position"}, {RoleHint, "hint"},
		{RoleScript, "script"}, {RoleSubtitles, "subtitles"},
	} {
		if string(tc.got) != tc.want {
			t.Errorf("DataRole = %q, want %q", tc.got, tc.want)
		}
	}
}

func TestLessonDataUnionRoundTrip(t *testing.T) {
	text := "Кириллица 🦊\n\n<table>_____{ANSWER}</table>"
	cases := []struct {
		name  string
		value Data
		json  string
	}{
		{"text", Data{Type: DataText, Role: dataRole(RoleBody), Value: text, Answers: []string{"Б", "А", "Б"}, Hint: "Подсказка"},
			`{"type":"text","role":"body","value":"Кириллица 🦊\n\n<table>_____{ANSWER}</table>","answers":["Б","А","Б"],"hint":"Подсказка"}`},
		{"media", Data{Type: DataMedia, Role: dataRole(RoleScript), Value: EmbeddedFile{FileName: "текст.txt", MIMEType: "", Content: "AA=="}, Answers: []string{}, Hint: ""},
			`{"type":"media","role":"script","value":{"fileName":"текст.txt","mimeType":"","content":"AA=="},"answers":[],"hint":""}`},
		{"empty collection", Data{Type: DataCollection, Value: []Data{}, Answers: []string{}, Hint: ""},
			`{"type":"collection","value":[],"answers":[],"hint":""}`},
		{"nested collection", Data{Type: DataCollection, Role: dataRole(RoleBody), Value: []Data{
			{Type: DataCollection, Value: []Data{{Type: DataText, Value: "first", Answers: []string{}, Hint: ""}}, Answers: []string{}, Hint: ""},
			{Type: DataMedia, Value: EmbeddedFile{"b.bin", "application/octet-stream", "AQI="}, Answers: []string{}, Hint: ""},
		}, Answers: []string{}, Hint: ""},
			`{"type":"collection","role":"body","value":[{"type":"collection","value":[{"type":"text","value":"first","answers":[],"hint":""}],"answers":[],"hint":""},{"type":"media","value":{"fileName":"b.bin","mimeType":"application/octet-stream","content":"AQI="},"answers":[],"hint":""}],"answers":[],"hint":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			dataEqualJSON(t, raw, []byte(tc.json))
			var got Data
			if err := json.Unmarshal([]byte(tc.json), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.value) {
				t.Fatalf("decoded = %#v, want %#v", got, tc.value)
			}
			if err := got.UnmarshalJSON(raw); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.value) {
				t.Fatalf("round trip changed concrete union: %#v", got)
			}
		})
	}
}

func TestLessonDataMarshalKeepsNilAndEmptyShapes(t *testing.T) {
	for _, tc := range []struct {
		value Data
		want  string
	}{
		{Data{Type: DataText, Value: ""}, `{"type":"text","value":"","answers":null,"hint":""}`},
		{Data{Type: DataCollection, Value: []Data(nil), Answers: []string{}}, `{"type":"collection","value":null,"answers":[],"hint":""}`},
		{Data{Type: DataCollection, Value: []Data{}, Answers: []string{}}, `{"type":"collection","value":[],"answers":[],"hint":""}`},
	} {
		raw, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatal(err)
		}
		dataEqualJSON(t, raw, []byte(tc.want))
	}
}

func TestLessonDataExactKeysAndAdditionalProperties(t *testing.T) {
	cases := []struct {
		raw  string
		want Data
	}{
		{`{"TYPE":"media","VALUE":{},"ROLE":"hint","ANSWERS":null,"HINT":false,"future":{"a":[null]},"type":"text","value":"kept","answers":[],"hint":""}`,
			Data{Type: DataText, Value: "kept", Answers: []string{}, Hint: ""}},
		{`{"type":"text","role":"body","value":"kept","answers":["a"],"hint":"h","TYPE":null,"VALUE":17,"ROLE":"title","ANSWERS":[],"HINT":"shadow"}`,
			Data{Type: DataText, Role: dataRole(RoleBody), Value: "kept", Answers: []string{"a"}, Hint: "h"}},
		{`{"type":"media","value":{"FILENAME":null,"MIMETYPE":false,"CONTENT":[],"fileName":"x","mimeType":"m","content":"","FileName":"shadow","MimeType":"shadow","Content":"shadow","future":{}},"answers":[],"hint":""}`,
			Data{Type: DataMedia, Value: EmbeddedFile{"x", "m", ""}, Answers: []string{}, Hint: ""}},
	}
	for i, tc := range cases {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			got := dataSeed()
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("decoded = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestLessonDataRejectsInvalidShapeAtomically(t *testing.T) {
	for _, field := range []string{"type", "value", "answers", "hint", "role"} {
		for _, variant := range []string{"missing", "null", "wrong type", "uppercase only"} {
			if field == "role" && (variant == "missing" || variant == "uppercase only") {
				continue // role is optional; an unknown uppercase property is allowed.
			}
			t.Run(field+"/"+variant, func(t *testing.T) {
				obj := map[string]any{"type": "text", "value": "", "answers": []string{}, "hint": "", "role": "body"}
				switch variant {
				case "missing":
					delete(obj, field)
				case "null":
					obj[field] = nil
				case "wrong type":
					obj[field] = true
				case "uppercase only":
					upper := map[string]string{"type": "TYPE", "value": "VALUE", "answers": "ANSWERS", "hint": "HINT"}
					obj[upper[field]] = obj[field]
					delete(obj, field)
				}
				raw, err := json.Marshal(obj)
				if err != nil {
					t.Fatal(err)
				}
				dataReject(t, raw, CodeJSON)
			})
		}
	}
	for _, field := range []string{"fileName", "mimeType", "content"} {
		for _, variant := range []string{"missing", "null", "wrong type", "wrong case"} {
			t.Run("embedded/"+field+"/"+variant, func(t *testing.T) {
				file := map[string]any{"fileName": "x", "mimeType": "", "content": ""}
				switch variant {
				case "missing":
					delete(file, field)
				case "null":
					file[field] = nil
				case "wrong type":
					file[field] = 3
				case "wrong case":
					upper := map[string]string{"fileName": "FILENAME", "mimeType": "MIMETYPE", "content": "CONTENT"}
					file[upper[field]] = file[field]
					delete(file, field)
				}
				raw, err := json.Marshal(map[string]any{"type": "media", "value": file, "answers": []string{}, "hint": ""})
				if err != nil {
					t.Fatal(err)
				}
				dataReject(t, raw, CodeJSON)
			})
		}
	}
}

func TestLessonDataRejectsMalformedAndNestedValuesAtomically(t *testing.T) {
	for _, raw := range []string{
		``, `{`, `null`, `[]`, `"text"`, `1`, `true`,
		`{"type":"text","value":"","answers":[],"hint":""} true`,
		`{"type":"text","value":"","answers":[null],"hint":""}`,
		`{"type":"text","value":"","answers":[1],"hint":""}`,
		`{"type":"text","value":{},"answers":[],"hint":""}`,
		`{"type":"media","value":[],"answers":[],"hint":""}`,
		`{"type":"media","value":null,"answers":[],"hint":""}`,
		`{"type":"media","value":"file","answers":[],"hint":""}`,
		`{"type":"collection","value":"list","answers":[],"hint":""}`,
		`{"type":"collection","value":{},"answers":[],"hint":""}`,
		`{"type":"collection","value":null,"answers":[],"hint":""}`,
		`{"type":"collection","value":[null],"answers":[],"hint":""}`,
		`{"type":"collection","value":[{}],"answers":[],"hint":""}`,
		`{"type":"collection","value":[{"type":"media","value":{"fileName":"x","mimeType":"m"},"answers":[],"hint":""}],"answers":[],"hint":""}`,
		`{"type":"collection","value":[{"type":"text","value":"good","answers":[],"hint":""},{"type":"collection","value":[{"type":"text","value":"bad","answers":[]}],"answers":[],"hint":""}],"answers":[],"hint":""}`,
	} {
		t.Run(raw, func(t *testing.T) { dataReject(t, []byte(raw), CodeJSON) })
	}
}

func TestLessonDataErrorPointers(t *testing.T) {
	for _, tc := range []struct {
		name, raw, pointer, code string
	}{
		{"unknown root type", `{"type":"TEXT","value":"","answers":[],"hint":""}`, "/type", CodeContract},
		{"unknown child type", `{"type":"collection","value":[{"type":"TEXT","value":"","answers":[],"hint":""}],"answers":[],"hint":""}`, "/value/0/type", CodeContract},
		{"unknown grandchild type", `{"type":"collection","value":[{"type":"collection","value":[{"type":"TEXT","value":"","answers":[],"hint":""}],"answers":[],"hint":""}],"answers":[],"hint":""}`, "/value/0/value/0/type", CodeContract},
		{"text value", `{"type":"text","value":{},"answers":[],"hint":""}`, "/value", CodeJSON},
		{"media value", `{"type":"media","value":[],"answers":[],"hint":""}`, "/value", CodeJSON},
		{"collection value", `{"type":"collection","value":"list","answers":[],"hint":""}`, "/value", CodeJSON},
		{"child value", `{"type":"collection","value":[{"type":"text","value":{},"answers":[],"hint":""}],"answers":[],"hint":""}`, "/value/0/value", CodeJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problem := dataReject(t, []byte(tc.raw), tc.code)
			var pointers []string
			for _, detail := range problem.Details {
				if detail.Label == "Поле JSON" {
					pointers = append(pointers, detail.Value)
				}
			}
			if !reflect.DeepEqual(pointers, []string{tc.pointer}) {
				t.Errorf("JSON pointers = %q, want [%q]", pointers, tc.pointer)
			}
		})
	}
}

func TestLessonDataUnmarshalDefersSemanticRules(t *testing.T) {
	for _, raw := range []string{
		`{"type":"text","role":"unknown","value":"","answers":[],"hint":""}`,
		`{"type":"text","role":"","value":"","answers":[],"hint":""}`,
		`{"type":"collection","role":"options","value":[],"answers":[],"hint":""}`,
		`{"type":"collection","role":"instruction","value":[],"answers":[],"hint":""}`,
		`{"type":"collection","value":[{"type":"text","role":"item","value":"","answers":[],"hint":""}],"answers":[],"hint":""}`,
		`{"type":"media","value":{"fileName":"../x","mimeType":"custom","content":"not base64"},"answers":[],"hint":""}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var got Data
			if err := got.UnmarshalJSON([]byte(raw)); err != nil {
				t.Fatalf("local shape rejected semantic-only input: %v", err)
			}
		})
	}
}

func dataRole(role DataRole) *DataRole { return &role }

func dataSeed() Data {
	return Data{Type: DataCollection, Role: dataRole(RoleBody), Value: []Data{
		{Type: DataText, Role: dataRole(RoleTitle), Value: "исходное", Answers: []string{"ответ"}, Hint: "подсказка"},
	}, Answers: []string{"сохранить"}, Hint: "получатель"}
}

func dataReject(t *testing.T, raw []byte, code string) *diagnostic.Error {
	t.Helper()
	beforeRaw := append([]byte(nil), raw...)
	got := dataSeed()
	err := got.UnmarshalJSON(raw)
	if err == nil {
		t.Fatal("expected diagnostic error")
	}
	var detail *diagnostic.Error
	if !errors.As(err, &detail) || detail == nil {
		t.Fatalf("error %T does not expose *diagnostic.Error: %v", err, err)
	}
	if detail.Code != code || detail.Reason == "" {
		t.Errorf("diagnostic = %#v, want code %q and nonempty reason", detail, code)
	}
	hasRussian := false
	for _, r := range detail.Reason {
		hasRussian = hasRussian || unicode.In(r, unicode.Cyrillic)
	}
	if !hasRussian || detail.Context != (diagnostic.Context{}) {
		t.Errorf("diagnostic must retain Russian reason and empty unknown context: %#v", detail)
	}
	if !reflect.DeepEqual(got, dataSeed()) || string(raw) != string(beforeRaw) {
		t.Fatalf("receiver or raw changed on failure: %#v", got)
	}
	return detail
}

func dataEqualJSON(t *testing.T, got, want []byte) {
	t.Helper()
	var actual, expected any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}
