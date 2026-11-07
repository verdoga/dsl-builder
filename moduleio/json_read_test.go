package moduleio

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

func TestJsonreadEncodingExactBytes(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip", FileName: "l001.json", Line: 9, Column: 3}
	for _, raw := range [][]byte{nil, {}, []byte(`"Кириллица 😀 �"`), []byte(`"\ufeff"`)} {
		if problem := checkJSONEncoding(raw, context); problem != nil {
			t.Fatalf("valid UTF-8 rejected: %v", problem)
		}
	}
	for _, raw := range [][]byte{{0xef, 0xbb, 0xbf, '{', '}'}, {'"', 0xff, '"'}, {'"', 0xc0, 0xaf, '"'}, {0xed, 0xa0, 0x80}, {0xe2, 0x82}} {
		before := bytes.Clone(raw)
		problem := checkJSONEncoding(raw, context)
		JsonreadProblem(t, problem, CodeEncoding, context)
		if !bytes.Equal(raw, before) {
			t.Fatal("encoding check changed raw")
		}
		if value, problems := decodeLesson(raw, context); value != nil || len(problems) == 0 || problems[0].Code != CodeEncoding {
			t.Fatalf("lesson encoding gate: value=%v errors=%v", value, problems)
		}
		if value, problems := decodeManifest(raw, context); value != nil || len(problems) == 0 || problems[0].Code != CodeEncoding {
			t.Fatalf("manifest encoding gate: value=%v errors=%v", value, problems)
		}
		if !bytes.Equal(raw, before) {
			t.Fatal("decode changed invalid raw")
		}
	}
}

func TestJsonreadStandardTreeAndEOF(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip", FileName: "manifest.json"}
	raw := []byte(" {\"n\":123456789012345678901234567890123456789,\"a\":[1e9999,-0,1.00e+2]} \r\n\t")
	before := bytes.Clone(raw)
	value, problem := decodeJSONValue(raw, context)
	if problem != nil {
		t.Fatal(problem)
	}
	want := map[string]any{"n": json.Number("123456789012345678901234567890123456789"), "a": []any{json.Number("1e9999"), json.Number("-0"), json.Number("1.00e+2")}}
	if !reflect.DeepEqual(value, want) || !bytes.Equal(raw, before) {
		t.Fatalf("standard UseNumber tree or raw changed: %#v", value)
	}
	for _, raw := range []string{`null`, `true`, `"строка"`, `[]`, `-1.25e-9999`} {
		if _, problem := decodeJSONValue([]byte(raw), context); problem != nil {
			t.Errorf("schema-free JSON %s: %v", raw, problem)
		}
	}
	for _, raw := range []string{"", " \t", `{`, `{} {}`, `null true`, `[]x`, `1 2`, `01`, `{} /*comment*/`, "{}\x00"} {
		t.Run(raw, func(t *testing.T) {
			value, problem := decodeJSONValue([]byte(raw), context)
			if value != nil {
				t.Fatalf("partial JSON result: %#v", value)
			}
			JsonreadProblem(t, problem, CodeJSON, context)
			if problem.Cause == nil {
				t.Fatal("lost technical decoder cause")
			}
		})
	}
}

func TestJsonreadLessonTypedAndUnchanged(t *testing.T) {
	raw := JsonreadFixture(t)
	before := bytes.Clone(raw)
	context := diagnostic.Context{Path: "исходный.zip", FileName: "l001.json"}
	value, problems := decodeLesson(raw, context)
	if value == nil || len(problems) != 0 {
		t.Fatalf("valid typed lesson: %v", problems)
	}
	d := value.Document
	if d.ID != "EFX7" || d.Title != "  Урок  " || d.Subtitle == nil || *d.Subtitle != "" || d.Version != "123456789012345678901234567890123456789" || d.LineCount != "10e2" || d.CreatedAt != "2026-11-04T09:00:00Z" {
		t.Fatalf("metadata repaired or case-shadowed: %#v", d)
	}
	nodes := d.Pages[0].Nodes
	if d.Pages[0].Title != nil || nodes[0].Type != lesson.NodeMultifill || nodes[0].Data[0].Value != "I _____{am}.\n\n <b>текст</b>" {
		t.Fatal("text, null title, type or markup changed")
	}
	media, ok := nodes[1].Data[0].Value.(lesson.EmbeddedFile)
	if !ok || media.Content != "AAEC/w==" || media.FileName != "звук.bin" || media.MIMEType != "application/octet-stream" {
		t.Fatalf("typed EmbeddedFile lost: %#v", nodes[1].Data[0].Value)
	}
	collection, ok := nodes[2].Data[0].Value.([]lesson.Data)
	if !ok || len(collection) != 1 || collection[0].Value != "вложенный" {
		t.Fatalf("typed recursive collection lost: %#v", nodes[2].Data[0].Value)
	}
	if !bytes.Equal(raw, before) || !bytes.Contains(raw, []byte("UNKNOWN_RAW_SENTINEL")) {
		t.Fatal("raw properties changed")
	}
	second, problems := decodeLesson(raw, context)
	if second == nil || len(problems) != 0 {
		t.Fatal(problems)
	}
	value.Document.Pages[0].Nodes[0].Data[0].Answers = []string{"changed"}
	if len(second.Document.Pages[0].Nodes[0].Data[0].Answers) != 0 {
		t.Fatal("separate decodes share mutable model")
	}
}

func TestJsonreadLessonOwnerDiagnostics(t *testing.T) {
	base := string(JsonreadFixture(t))
	context := diagnostic.Context{Path: "архив.zip", FileName: "l001.json", Line: 7, Column: 4}
	for _, change := range [][2]string{
		{`"title": "  Урок  "`, `"title": null`},
		{`"title": null`, `"TITLE_ONLY": null`},
		{`"flags": {}`, `"flags": {"UNKNOWN": false}`},
		{`"hint": ""`, `"hint": null`},
		{`"version": 123456789012345678901234567890123456789`, `"version": 0`},
		{`"id": "efx7-n001"`, `"id": "other-n001"`},
		{`"content": "AAEC/w=="`, `"content": "PRIVATE_BASE64_SENTINEL"`},
		{`"type": "text"`, `"type": "unknown"`},
	} {
		raw := []byte(strings.Replace(base, change[0], change[1], 1))
		want := lesson.ValidateJSON(raw, context)
		if len(want) == 0 {
			t.Fatalf("invalid test fixture mutation %s", change[0])
		}
		value, got := decodeLesson(raw, context)
		if value != nil {
			t.Fatal("invalid lesson returned partial model")
		}
		JsonreadOwner(t, got, want)
		JsonreadNoBody(t, got, "PRIVATE_BASE64_SENTINEL", "UNKNOWN_RAW_SENTINEL", "I _____{am}")
	}
	for _, suffix := range []string{` {}`, ` trailing`, "\x00"} {
		raw := []byte(base + suffix)
		value, problems := decodeLesson(raw, context)
		if value != nil || len(problems) == 0 {
			t.Fatalf("lesson accepted trailing data %q", suffix)
		}
		JsonreadOwner(t, problems, lesson.ValidateJSON(raw, context))
	}
}

func JsonreadFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/json_read/typed.lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func JsonreadProblem(t *testing.T, problem *diagnostic.Error, code string, context diagnostic.Context) {
	t.Helper()
	if problem == nil || problem.Code != code || problem.Reason == "" || problem.Context != context {
		t.Fatalf("diagnostic: %#v; want code %s context %#v", problem, code, context)
	}
}

func JsonreadOwner(t *testing.T, got, want []*diagnostic.Error) {
	t.Helper()
	for _, expected := range want {
		found := false
		for _, actual := range got {
			if actual == nil || actual.Code != expected.Code || actual.Reason != expected.Reason || actual.Context != expected.Context || !reflect.DeepEqual(actual.Details, expected.Details) || !reflect.DeepEqual(actual.Related, expected.Related) {
				continue
			}
			if (actual.Cause == nil) != (expected.Cause == nil) {
				continue
			}
			if expected.Cause != nil && (reflect.TypeOf(actual.Cause) != reflect.TypeOf(expected.Cause) || actual.Cause.Error() != expected.Cause.Error()) {
				continue
			}
			if actual.Cause != nil && !errors.Is(actual, actual.Cause) {
				t.Fatal("diagnostic no longer unwraps cause")
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("owner diagnostic lost: %#v; actual=%#v", expected, got)
		}
	}
}

func JsonreadNoBody(t *testing.T, problems []*diagnostic.Error, secrets ...string) {
	t.Helper()
	for _, problem := range problems {
		if problem == nil {
			t.Fatal("nil diagnostic")
		}
		encoded := problem.Reason + problem.Error()
		for _, detail := range problem.Details {
			encoded += detail.Label + detail.Value
		}
		if problem.Cause != nil {
			encoded += problem.Cause.Error()
		}
		for _, secret := range secrets {
			if strings.Contains(encoded, secret) {
				t.Fatal("diagnostic exposes JSON body or Base64")
			}
		}
	}
}
