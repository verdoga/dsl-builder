package parsed

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

const decodeDocumentTestMetadata = `{"documentId":"Ab:+Я","title":"  Заголовок\\n\t😀  ","subtitle":"Подзаголовок","section":" Свободная Секция ","order":"0001234567890123456789012345678901234567890","resourceDirs":["Z/../путь","C:\\Audio","Z/../путь",""]}`
const decodeDocumentTestDocument = `{"dslVersion":"future-version","fileName":" не JSON.txt ","filePath":"relative/../исходник.txt","encoding":"unknown-encoding","hasBom":false,"lineCount":0,"byteLength":0,"sha256":"Unvalidated SHA","metadata":` + decodeDocumentTestMetadata + `,"hasErrors":false}`
const decodeDocumentTestProcessing = `{"id":"run-Я","tool":"custom.future.tool","version":"  v-next\\literal  ","startedAt":"2026-11-03T05:58:58.1234567+03:00","durationMs":0}`

type decodeDocumentTestDecoder func(json.RawMessage) (json.RawMessage, *diagnostic.Error)
type decodeDocumentTestField struct {
	name     string
	kind     string
	nullable bool
}

// decodeDocumentTestObject создаёт независимый набор исходных JSON-полей.
func decodeDocumentTestObject(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

// decodeDocumentTestJSON сериализует поля без преобразования чисел во float64.
func decodeDocumentTestJSON(t *testing.T, fields map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// decodeDocumentTestEqual проверяет сохранность известных полей, включая null и пустые массивы.
func decodeDocumentTestEqual(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	gotFields := decodeDocumentTestObject(t, string(got))
	wantFields := decodeDocumentTestObject(t, string(want))
	for name, expected := range wantFields {
		actual, ok := gotFields[name]
		if !ok {
			t.Errorf("поле %s потеряно", name)
			continue
		}
		if len(expected) > 0 && expected[0] == '{' {
			decodeDocumentTestEqual(t, actual, expected)
			continue
		}
		var a, b bytes.Buffer
		if err := json.Compact(&a, actual); err != nil {
			t.Fatal(err)
		}
		if err := json.Compact(&b, expected); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Errorf("%s = %s, требуется %s", name, actual, expected)
		}
	}
}

// decodeDocumentTestMatrix проверяет обязательность, nullable и точный тип каждого поля.
func decodeDocumentTestMatrix(t *testing.T, base string, path string, fields []decodeDocumentTestField, decode decodeDocumentTestDecoder, context diagnostic.Context) {
	t.Helper()
	checkError := func(t *testing.T, raw json.RawMessage, code, field string, wantContext diagnostic.Context) {
		t.Helper()
		_, err := decode(raw)
		if err == nil {
			t.Fatalf("принят неверный JSON: %s", raw)
		}
		if err.Code != code {
			t.Errorf("Code=%q, требуется %q", err.Code, code)
		}
		if err.Context != wantContext {
			t.Errorf("Context=%+v, требуется %+v", err.Context, wantContext)
		}
		if strings.TrimSpace(err.Reason) == "" {
			t.Error("причина ошибки пуста")
		}
		found := false
		for _, detail := range err.Details {
			if detail.Value == field || strings.HasPrefix(detail.Value, field+"[") {
				found = true
			}
		}
		if !found {
			t.Errorf("Details=%+v не содержит путь %q", err.Details, field)
		}
	}
	for _, bad := range []string{"null", "[]", `"text"`, "0", "false"} {
		t.Run("object/"+bad, func(t *testing.T) { checkError(t, json.RawMessage(bad), CodeContract, path, context) })
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			wantContext := context
			if path == "document.metadata" && field.name != "documentId" {
				// Исходный ID уже прочитан до проверки последующих полей metadata.
				wantContext.LessonID = "Ab:+Я"
			}
			t.Run("missing", func(t *testing.T) {
				obj := decodeDocumentTestObject(t, base)
				delete(obj, field.name)
				code := CodeContract
				if field.name == "documentId" {
					code = CodeMissingID
				}
				checkError(t, decodeDocumentTestJSON(t, obj), code, path+"."+field.name, wantContext)
			})
			t.Run("wrong-case", func(t *testing.T) {
				obj := decodeDocumentTestObject(t, base)
				value := obj[field.name]
				delete(obj, field.name)
				obj[strings.ToUpper(field.name)] = value
				code := CodeContract
				if field.name == "documentId" {
					code = CodeMissingID
				}
				checkError(t, decodeDocumentTestJSON(t, obj), code, path+"."+field.name, wantContext)
			})
			values := []string{"null", `""`, "false", "true", "0", "1", "1.0", "1e0", "[]", "{}", `["x",null]`, `[1]`}
			for _, value := range values {
				t.Run(value, func(t *testing.T) {
					obj := decodeDocumentTestObject(t, base)
					obj[field.name] = json.RawMessage(value)
					valid := value == "null" && field.nullable || field.kind == "string" && value == `""` || field.kind == "bool" && (value == "false" || value == "true") || field.kind == "int" && (value == "0" || value == "1") || field.kind == "strings" && value == "[]"
					raw := decodeDocumentTestJSON(t, obj)
					if !valid {
						code, errorField := CodeContract, path+"."+field.name
						if field.kind == "object" && value == "{}" {
							code, errorField = CodeMissingID, errorField+".documentId"
						}
						checkError(t, raw, code, errorField, wantContext)
						return
					}
					got, err := decode(raw)
					if err != nil {
						t.Fatalf("допустимая форма отклонена: %v", err)
					}
					decodeDocumentTestEqual(t, got, raw)
				})
			}
		})
	}
	t.Run("preservation-and-unknown", func(t *testing.T) {
		obj := decodeDocumentTestObject(t, base)
		obj["future"] = json.RawMessage(`{"arbitrary":[null,false,9007199254740993]}`)
		got, err := decode(decodeDocumentTestJSON(t, obj))
		if err != nil {
			t.Fatal(err)
		}
		decodeDocumentTestEqual(t, got, json.RawMessage(base))
	})
}

func TestDecodeDocumentTestShape(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Вход\lesson.json`, FileName: "l019.json", LessonID: "Ab:+Я", PageID: "page", Line: 17, Column: 4}
	decode := func(raw json.RawMessage) (json.RawMessage, *diagnostic.Error) {
		got, err := decodeDocument(raw, "document", context)
		if err != nil {
			return nil, err
		}
		encoded, e := json.Marshal(got)
		if e != nil {
			t.Fatal(e)
		}
		return encoded, nil
	}
	decodeDocumentTestMatrix(t, decodeDocumentTestDocument, "document", []decodeDocumentTestField{
		{"dslVersion", "string", true}, {"fileName", "string", true}, {"filePath", "string", true}, {"encoding", "string", true},
		{"hasBom", "bool", true}, {"lineCount", "int", true}, {"byteLength", "int", true}, {"sha256", "string", true},
		{"metadata", "object", false}, {"hasErrors", "bool", false},
	}, decode, context)
	t.Run("nested-missing-documentId", func(t *testing.T) {
		metadata := decodeDocumentTestObject(t, decodeDocumentTestMetadata)
		delete(metadata, "documentId")
		obj := decodeDocumentTestObject(t, decodeDocumentTestDocument)
		obj["metadata"] = decodeDocumentTestJSON(t, metadata)
		_, err := decode(decodeDocumentTestJSON(t, obj))
		if err == nil || err.Code != CodeMissingID {
			t.Fatalf("требуется CodeMissingID, получено %v", err)
		}
	})
}

func TestDecodeDocumentTestMetadataShape(t *testing.T) {
	context := diagnostic.Context{Path: "вход.json", LessonID: "ИсходныйID", Line: 8, Column: 2}
	decode := func(raw json.RawMessage) (json.RawMessage, *diagnostic.Error) {
		got, err := decodeMetadata(raw, "document.metadata", context)
		if err != nil {
			return nil, err
		}
		encoded, e := json.Marshal(got)
		if e != nil {
			t.Fatal(e)
		}
		return encoded, nil
	}
	decodeDocumentTestMatrix(t, decodeDocumentTestMetadata, "document.metadata", []decodeDocumentTestField{
		{"documentId", "string", true}, {"title", "string", true}, {"subtitle", "string", true},
		{"section", "string", true}, {"order", "string", true}, {"resourceDirs", "strings", true},
	}, decode, context)
}

func TestDecodeDocumentTestProcessingShape(t *testing.T) {
	context := diagnostic.Context{Path: "история.json", LessonID: "ID", Line: 3, Column: 7}
	decode := func(raw json.RawMessage) (json.RawMessage, *diagnostic.Error) {
		got, err := decodeProcessing(raw, "processing[2]", context)
		if err != nil {
			return nil, err
		}
		encoded, e := json.Marshal(got)
		if e != nil {
			t.Fatal(e)
		}
		return encoded, nil
	}
	decodeDocumentTestMatrix(t, decodeDocumentTestProcessing, "processing[2]", []decodeDocumentTestField{
		{"id", "string", false}, {"tool", "string", false}, {"version", "string", true}, {"startedAt", "string", true}, {"durationMs", "int", true},
	}, decode, context)
}

func TestDecodeDocumentTestIntegerPrecision(t *testing.T) {
	if strconv.IntSize == 64 {
		obj := decodeDocumentTestObject(t, decodeDocumentTestDocument)
		obj["lineCount"] = json.RawMessage("9007199254740993")
		got, err := decodeDocument(decodeDocumentTestJSON(t, obj), "document", diagnostic.Context{})
		if err != nil || got.LineCount == nil || int64(*got.LineCount) != int64(9007199254740993) {
			t.Fatalf("потеря точности lineCount: %+v, %v", got.LineCount, err)
		}
		processing := decodeDocumentTestObject(t, decodeDocumentTestProcessing)
		processing["durationMs"] = json.RawMessage("9007199254740993")
		run, err := decodeProcessing(decodeDocumentTestJSON(t, processing), "processing[0]", diagnostic.Context{})
		if err != nil || run.DurationMs == nil || int64(*run.DurationMs) != int64(9007199254740993) {
			t.Fatalf("потеря точности durationMs: %+v, %v", run.DurationMs, err)
		}
	}
	for _, value := range []string{"9007199254740993", "9223372036854775807"} {
		t.Run("byteLength/"+value, func(t *testing.T) {
			obj := decodeDocumentTestObject(t, decodeDocumentTestDocument)
			obj["byteLength"] = json.RawMessage(value)
			got, err := decodeDocument(decodeDocumentTestJSON(t, obj), "document", diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			encoded, e := json.Marshal(got)
			if e != nil {
				t.Fatal(e)
			}
			decodeDocumentTestEqual(t, encoded, decodeDocumentTestJSON(t, obj))
		})
	}
	for _, value := range []string{`"7"`, "0.5", "-0.5", "1.0", "1e3", "9223372036854775808", "-9223372036854775809"} {
		for _, field := range []string{"lineCount", "byteLength", "durationMs"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				var err *diagnostic.Error
				if field == "durationMs" {
					obj := decodeDocumentTestObject(t, decodeDocumentTestProcessing)
					obj[field] = json.RawMessage(value)
					_, err = decodeProcessing(decodeDocumentTestJSON(t, obj), "processing[0]", diagnostic.Context{})
				} else {
					obj := decodeDocumentTestObject(t, decodeDocumentTestDocument)
					obj[field] = json.RawMessage(value)
					_, err = decodeDocument(decodeDocumentTestJSON(t, obj), "document", diagnostic.Context{})
				}
				if err == nil || err.Code != CodeContract {
					t.Fatalf("неверное целое принято: %s, ошибка %v", value, err)
				}
			})
		}
	}
}
