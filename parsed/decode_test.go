package parsed

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func decodeTestError(t *testing.T, err *diagnostic.Error, code string, context diagnostic.Context) {
	t.Helper()
	if code == "" {
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if err.Code != code || err.Reason == "" || err.Context != context {
		t.Fatalf("ошибка = %+v; нужны код %q, причина и контекст %+v", err, code, context)
	}
}

func decodeTestInput(t *testing.T) string {
	t.Helper()
	id := "Урок:A+1"
	result := model.Result{Format: model.FormatVersion1, Processing: []model.Processing{}, Lines: []model.Line{}, Diagnostics: []model.Diagnostic{}}
	result.Document.Metadata.DocumentID = &id
	result.Document.Metadata.ResourceDirs = []string{}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDecodeResultEnvelope(t *testing.T) {
	base := decodeTestInput(t)
	context := diagnostic.Context{Path: "Каталог/Вход.json"}
	cases := []struct{ name, raw, code string }{
		{"единственный объект", base, ""},
		{"хвостовые пробелы", base + " \r\n\t", ""},
		{"неизвестные поля", strings.ReplaceAll(base, "{", `{"future":{"n":[null,9007199254740993]},`), ""},
		{"последняя версия побеждает", strings.Replace(base, `"formatVersion":"1.0"`, `"formatVersion":"2.0","formatVersion":"1.0"`, 1), ""},
		{"последняя версия неподдерживаемая", strings.Replace(base, `"formatVersion":"1.0"`, `"formatVersion":"1.0","formatVersion":"2.0"`, 1), CodeVersion},
		{"пустой ввод", "", CodeJSON}, {"повреждённый JSON", base[:len(base)-1], CodeJSON},
		{"комментарий JSONC", "// комментарий\n" + base, CodeJSON},
		{"встроенный комментарий", strings.Replace(base, "{", "{/* комментарий */", 1), CodeJSON},
		{"второй объект", base + " {}", CodeJSON}, {"второе null", base + " null", CodeJSON},
		{"мусор после объекта", base + " x", CodeJSON}, {"массив", "[]", CodeContract},
		{"null", "null", CodeContract}, {"строка", `"text"`, CodeContract},
		{"неподдерживаемая версия", strings.Replace(base, `"1.0"`, `"1.1"`, 1), CodeVersion},
		{"версия число", strings.Replace(base, `"1.0"`, `1.0`, 1), CodeContract},
		{"версия null", strings.Replace(base, `"1.0"`, `null`, 1), CodeContract},
		{"документ null", strings.Replace(base, `"document":{`, `"document":null,"ignored":{`, 1), CodeContract},
		{"документ массив", strings.Replace(base, `"document":{`, `"document":[],"ignored":{`, 1), CodeContract},
		{"точный ключ имеет приоритет над похожим", strings.Replace(base, `"formatVersion":"1.0"`, `"formatVersion":"1.0","FORMATVERSION":"2.0"`, 1), ""},
	}
	for _, field := range []string{"formatVersion", "document", "processing", "lines", "diagnostics"} {
		cases = append(cases, struct{ name, raw, code string }{"нет " + field, strings.Replace(base, `"`+field+`":`, `"unknown":`, 1), CodeContract})
		cases = append(cases, struct{ name, raw, code string }{"регистр " + field, strings.Replace(base, `"`+field+`":`, `"`+strings.ToUpper(field)+`":`, 1), CodeContract})
	}
	knownIDCaseStart := len(cases)
	for _, field := range []string{"processing", "lines", "diagnostics"} {
		for _, value := range []string{"null", "{}", "[null]", "[1]", `["x"]`} {
			cases = append(cases, struct{ name, raw, code string }{field + "=" + value, strings.Replace(base, `"`+field+`":[]`, `"`+field+`":`+value, 1), CodeContract})
		}
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := decodeResult([]byte(tc.raw), context)
			wantContext := context
			if i >= knownIDCaseStart {
				wantContext.LessonID = "Урок:A+1"
			}
			decodeTestError(t, err, tc.code, wantContext)
			if tc.code == "" && (result.Format != model.FormatVersion1 || result.Document.Metadata.DocumentID == nil || *result.Document.Metadata.DocumentID != "Урок:A+1" || result.Processing == nil || result.Lines == nil || result.Diagnostics == nil) {
				t.Fatalf("утрачена форма или значения: %+v", result)
			}
		})
	}
}

func TestDecodeResultPreservesModelValues(t *testing.T) {
	raw := decodeTestInput(t)
	for _, replacement := range [][2]string{
		{`"hasBom":null`, `"hasBom":false`},
		{`"lineCount":null`, `"lineCount":1`},
		{`"byteLength":null`, `"byteLength":9007199254740993`},
		{`"resourceDirs":[]`, `"resourceDirs":["Б","","А"]`},
		{`"processing":[]`, `"processing":[{"id":"second","tool":"custom","version":null,"startedAt":null,"durationMs":0},{"id":"first","tool":"parser","version":"x","startedAt":null,"durationMs":null}]`},
		{`"lines":[]`, `"lines":[{"line":1,"type":"content","nestingLevel":0,"parentLine":null,"raw":"Я","eol":"","hasErrors":false,"elements":[{"type":"content","raw":"Я","value":"\\n","start":1,"end":2,"errorIds":[]}]}]`},
		{`"diagnostics":[]`, `"diagnostics":[{"id":"d","source":"second","code":"custom.code","severity":"warning","message":"Текст","scope":"document","fatal":false,"location":null,"relatedLocations":[]}]`},
	} {
		raw = strings.Replace(raw, replacement[0], replacement[1], 1)
	}
	result, err := decodeResult([]byte(raw), diagnostic.Context{})
	decodeTestError(t, err, "", diagnostic.Context{})
	got, marshalErr := json.Marshal(result)
	if marshalErr != nil || string(got) != raw {
		t.Fatalf("значения или порядок изменены: %s; ошибка=%v; нужно %s", got, marshalErr, raw)
	}
}

func TestDecodeObjectFields(t *testing.T) {
	context := diagnostic.Context{Path: "input.json", LessonID: "AbC", Line: 9, Column: 3}
	for _, tc := range []struct{ name, raw, want, code string }{
		{"точные ключи", `{"value":false,"unknown":null}`, "false", ""},
		{"точность числа", `{"value":9007199254740993}`, "9007199254740993", ""},
		{"null присутствует", `{"value":null}`, "null", ""},
		{"последний дубль", `{"value":1,"value":0}`, "0", ""},
		{"разный регистр независим", `{"value":1,"VALUE":2}`, "1", ""},
		{"нет поля", `{}`, "", CodeContract}, {"неверный регистр", `{"Value":0}`, "", CodeContract},
		{"нет объекта", "", "", CodeContract}, {"null", "null", "", CodeContract},
		{"массив", "[]", "", CodeContract}, {"число", "0", "", CodeContract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := decodeObject(json.RawMessage(tc.raw), []string{"value"}, "document", context)
			decodeTestError(t, err, tc.code, context)
			if tc.code == "" && string(fields["value"]) != tc.want {
				t.Fatalf("value=%s, нужно %s", fields["value"], tc.want)
			}
		})
	}
	if fields, err := decodeObject(json.RawMessage(`{}`), nil, "document", context); err != nil || fields == nil {
		t.Fatalf("пустой объект без обязательных ключей: %v, %v", fields, err)
	}
}

func TestDecodeArrayShapeAndOrder(t *testing.T) {
	context := diagnostic.Context{Path: "input.json"}
	for _, tc := range []struct {
		raw                        string
		nullable, valid, nilResult bool
	}{
		{"", true, false, false}, {"", false, false, false},
		{"null", true, true, true}, {"null", false, false, false},
		{"[]", false, true, false}, {"[]", true, true, false},
		{`[null,9007199254740993,"я",{}]`, false, true, false},
		{"{}", true, false, false}, {"false", false, false, false}, {`"[]"`, true, false, false},
	} {
		t.Run(tc.raw+"/nullable="+map[bool]string{true: "true", false: "false"}[tc.nullable], func(t *testing.T) {
			items, err := decodeArray(json.RawMessage(tc.raw), tc.nullable, "items", context)
			code := CodeContract
			if tc.valid {
				code = ""
			}
			decodeTestError(t, err, code, context)
			if !tc.valid {
				return
			}
			if (items == nil) != tc.nilResult {
				t.Fatalf("null и [] смешаны: %#v", items)
			}
			if strings.HasPrefix(tc.raw, "[null,") {
				want := []string{"null", "9007199254740993", `"я"`, "{}"}
				if len(items) != len(want) {
					t.Fatalf("элементы: %v", items)
				}
				for i := range want {
					if string(items[i]) != want[i] {
						t.Fatalf("элемент %d=%s", i, items[i])
					}
				}
			}
		})
	}
}

func TestDecodeScalarTypesAndPresence(t *testing.T) {
	context := diagnostic.Context{Path: "input.json", Line: 4}
	for _, tc := range []struct {
		name, raw, kind, want string
		nullable, invalid     bool
	}{
		{"строка", `"Я\\n\n🙂"`, "string", `"Я\\n\n🙂"`, false, false},
		{"пустая строка", `""`, "string", `""`, false, false},
		{"false", "false", "bool", "false", false, false}, {"true", "true", "bool", "true", false, false},
		{"ноль", "0", "int", "0", false, false}, {"отрицательное целое", "-4", "int", "-4", false, false},
		{"точное int64", "9007199254740993", "int64", "9007199254740993", false, false},
		{"max int64", "9223372036854775807", "int64", "9223372036854775807", false, false},
		{"min int64", "-9223372036854775808", "int64", "-9223372036854775808", false, false},
		{"переполнение", "9223372036854775808", "int64", "", false, true},
		{"дробь", "1.5", "int", "", false, true}, {"дробная запись целого", "1.0", "int64", "", false, true},
		{"экспонента", "1e0", "int64", "", false, true}, {"целое строкой", `"1"`, "int", "", false, true},
		{"число вместо строки", "0", "string", "", false, true}, {"строка вместо bool", `"false"`, "bool", "", false, true},
		{"ноль вместо bool", "0", "bool", "", false, true}, {"объект", "{}", "string", "", false, true},
		{"массив", "[]", "int", "", false, true}, {"null строки", "null", "string", "", false, true},
		{"null bool", "null", "bool", "", false, true}, {"null числа", "null", "int", "", false, true},
		{"нет nullable", "", "ptr", "", true, true}, {"нет обязательного", "", "int", "", false, true},
		{"nullable null", "null", "ptr", "null", true, false}, {"nullable пустая", `""`, "ptr", `""`, true, false},
		{"nullable false", "false", "boolptr", "false", true, false}, {"nullable ноль", "0", "intptr", "0", true, false},
		{"nullable bool null", "null", "boolptr", "null", true, false}, {"nullable integer null", "null", "intptr", "null", true, false},
		{"nullable нет bool", "", "boolptr", "", true, true}, {"nullable нет integer", "", "intptr", "", true, true},
		{"версия model", `"1.0"`, "version", `"1.0"`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dst any
			switch tc.kind {
			case "string":
				dst = new(string)
			case "bool":
				dst = new(bool)
			case "int":
				dst = new(int)
			case "int64":
				dst = new(int64)
			case "ptr":
				dst = new(*string)
			case "boolptr":
				dst = new(*bool)
			case "intptr":
				dst = new(*int)
			case "version":
				dst = new(model.FormatVersion)
			}
			err := decodeScalar(json.RawMessage(tc.raw), dst, tc.nullable, "field", context)
			code := ""
			if tc.invalid {
				code = CodeContract
			}
			decodeTestError(t, err, code, context)
			if !tc.invalid {
				got, marshalErr := json.Marshal(dst)
				if marshalErr != nil || string(got) != tc.want {
					t.Fatalf("значение=%s, ошибка=%v; нужно %s", got, marshalErr, tc.want)
				}
			}
		})
	}
}

func TestDecodeStringsNullElementsAndOrder(t *testing.T) {
	context := diagnostic.Context{Path: "input.json"}
	for _, tc := range []struct {
		raw             string
		nullable, valid bool
		want            string
	}{
		{"", true, false, ""}, {"", false, false, ""}, {"null", true, true, "null"},
		{"null", false, false, ""}, {"[]", false, true, "[]"}, {"[]", true, true, "[]"},
		{`["б","","а","б"]`, false, true, `["б","","а","б"]`},
		{`["a",null]`, true, false, ""}, {`[null]`, false, false, ""},
		{`[1]`, true, false, ""}, {`[false]`, false, false, ""}, {`[{}]`, false, false, ""},
		{`[[]]`, true, false, ""}, {`{}`, false, false, ""}, {`"a"`, true, false, ""},
	} {
		t.Run(tc.raw+"/nullable="+map[bool]string{true: "true", false: "false"}[tc.nullable], func(t *testing.T) {
			got, err := decodeStrings(json.RawMessage(tc.raw), tc.nullable, "resourceDirs", context)
			code := CodeContract
			if tc.valid {
				code = ""
			}
			decodeTestError(t, err, code, context)
			if tc.valid {
				raw, marshalErr := json.Marshal(got)
				if marshalErr != nil || string(raw) != tc.want {
					t.Fatalf("строки=%s, ошибка=%v; нужно %s", raw, marshalErr, tc.want)
				}
			}
		})
	}
}
