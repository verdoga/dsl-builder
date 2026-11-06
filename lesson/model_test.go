package lesson

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLessonModelFormatVersion(t *testing.T) {
	if FormatVersion != "1.0" {
		t.Fatalf("FormatVersion = %q, want 1.0", FormatVersion)
	}
}

func TestLessonModelJSONFieldsAndOrder(t *testing.T) {
	subtitle, pageTitle := "Подзаголовок 雪", "Страница 2"
	value := Lesson{
		FormatVersion: FormatVersion,
		Document: Document{
			ID:        "EfX7",
			Version:   json.Number("23"),
			Title:     "Название урока",
			Subtitle:  &subtitle,
			LineCount: json.Number("4567"),
			CreatedAt: "2026-11-06T12:34:56Z",
			Pages: []Page{
				{ID: "efx7-p009", Title: nil, Nodes: []Node{}},
				{ID: "efx7-p002", Title: &pageTitle, Nodes: []Node{}},
			},
		},
	}

	root := testLessonModelObject(t, testLessonModelMarshal(t, value), "formatVersion", "document")
	testLessonModelRaw(t, root["formatVersion"], `"1.0"`)
	document := testLessonModelObject(t, root["document"],
		"id", "version", "title", "subtitle", "lineCount", "createdAt", "pages")
	for field, want := range map[string]string{
		"id": `"EfX7"`, "version": "23", "title": `"Название урока"`,
		"subtitle": `"Подзаголовок 雪"`, "lineCount": "4567", "createdAt": `"2026-11-06T12:34:56Z"`,
	} {
		t.Run(field, func(t *testing.T) { testLessonModelRaw(t, document[field], want) })
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(document["pages"], &pages); err != nil {
		t.Fatalf("decode pages: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("pages length = %d, want 2", len(pages))
	}
	for i, want := range []struct{ id, title string }{
		{`"efx7-p009"`, "null"}, {`"efx7-p002"`, `"Страница 2"`},
	} {
		page := testLessonModelObject(t, pages[i], "id", "title", "nodes")
		testLessonModelRaw(t, page["id"], want.id)
		testLessonModelRaw(t, page["title"], want.title)
		testLessonModelRaw(t, page["nodes"], "[]")
	}
}

func TestLessonModelNullableTitles(t *testing.T) {
	empty, text := "", "Имя 雪"
	for _, tc := range []struct {
		name string
		text *string
		want string
	}{
		{"nil", nil, "null"},
		{"empty", &empty, `""`},
		{"text", &text, `"Имя 雪"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := testLessonModelObject(t, testLessonModelMarshal(t, Document{
				ID: "AbC", Version: json.Number("1"), Title: "", Subtitle: tc.text,
				LineCount: json.Number("0"), CreatedAt: "2026-11-06T00:00:00Z", Pages: []Page{},
			}), "id", "version", "title", "subtitle", "lineCount", "createdAt", "pages")
			testLessonModelRaw(t, document["title"], `""`)
			testLessonModelRaw(t, document["subtitle"], tc.want)
			testLessonModelRaw(t, document["pages"], "[]")
			page := testLessonModelObject(t, testLessonModelMarshal(t, Page{
				ID: "abc-p001", Title: tc.text, Nodes: []Node{},
			}), "id", "title", "nodes")
			testLessonModelRaw(t, page["title"], tc.want)
			testLessonModelRaw(t, page["nodes"], "[]")
		})
	}
}

func TestLessonModelNumberLexemes(t *testing.T) {
	for _, number := range []string{
		"9007199254740993",
		"123456789012345678901234567890123456789012345678901234567890",
		"123456789012345678901234567890.000",
		"1E+1000",
		"1.2300e+42",
	} {
		t.Run(number, func(t *testing.T) {
			value := Document{Version: json.Number(number), LineCount: json.Number(number), Pages: []Page{}}
			document := testLessonModelObject(t, testLessonModelMarshal(t, value),
				"id", "version", "title", "subtitle", "lineCount", "createdAt", "pages")
			testLessonModelRaw(t, document["version"], number)
			testLessonModelRaw(t, document["lineCount"], number)
			if value.Version.String() != number || value.LineCount.String() != number {
				t.Fatal("marshaling changed number lexemes in the model")
			}
		})
	}
}

func TestLessonModelNilSliceRepresentation(t *testing.T) {
	// nil — недопустимое значение обязательного массива при валидации;
	// сама модель не заменяет его на пустой массив при сериализации.
	document := testLessonModelObject(t, testLessonModelMarshal(t, Document{}),
		"id", "version", "title", "subtitle", "lineCount", "createdAt", "pages")
	testLessonModelRaw(t, document["pages"], "null")
	page := testLessonModelObject(t, testLessonModelMarshal(t, Page{}), "id", "title", "nodes")
	testLessonModelRaw(t, page["nodes"], "null")
	testLessonModelRaw(t, page["title"], "null")
}

func testLessonModelMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal model: %v", err)
	}
	return raw
}

func testLessonModelObject(t *testing.T, raw []byte, names ...string) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("decode JSON object: %v", err)
	}
	if len(object) != len(names) {
		t.Fatalf("JSON object = %s; want exactly fields %v", raw, names)
	}
	for _, name := range names {
		if _, ok := object[name]; !ok {
			t.Fatalf("JSON object %s misses field %q", raw, name)
		}
	}
	return object
}

func testLessonModelRaw(t *testing.T, raw []byte, want string) {
	t.Helper()
	if !bytes.Equal(raw, []byte(want)) {
		t.Errorf("JSON value = %s, want %s", raw, want)
	}
}
