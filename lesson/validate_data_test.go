package lesson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func dataValidationData(kind DataType, role DataRole, value any) Data {
	d := Data{Type: kind, Value: value, Answers: []string{}}
	if role != "" {
		d.Role = &role
	}
	return d
}

func dataValidationCheckErrors(t *testing.T, errs []*diagnostic.Error, context diagnostic.Context, pointers ...string) {
	t.Helper()
	if len(errs) != len(pointers) {
		t.Fatalf("errors = %v, want pointers %v", errs, pointers)
	}
	for i, e := range errs {
		if e == nil || e.Code != CodeContract || e.Context != context {
			t.Fatalf("error %d = %#v; want contract code and preserved context", i, e)
		}
		if !strings.ContainsFunc(e.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
			t.Errorf("error %d lacks a Russian reason: %q", i, e.Reason)
		}
		found := false
		for _, detail := range e.Details {
			found = found || detail.Label == "Поле JSON" && detail.Value == pointers[i]
		}
		if !found {
			t.Errorf("error %d details = %v, want JSON pointer %q", i, e.Details, pointers[i])
		}
	}
}

func TestLessonDataValidationValueUnion(t *testing.T) {
	type namedString string
	type namedFile EmbeddedFile
	type namedSlice []Data
	file := EmbeddedFile{FileName: "файл.bin", MIMEType: "anything", Content: "AA=="}
	text := "text"
	children := []Data{}
	for _, tc := range []struct {
		name  string
		kind  DataType
		value any
		field string
	}{
		{"text", DataText, "", ""}, {"media", DataMedia, file, ""}, {"collection", DataCollection, children, ""},
		{"unknown", "Text", "", "type"}, {"empty discriminator", "", "", "type"},
		{"nil", DataText, nil, "value"}, {"typed nil", DataCollection, []Data(nil), "value"},
		{"nil pointer", DataMedia, (*EmbeddedFile)(nil), "value"},
		{"text pointer", DataText, &text, "value"}, {"file pointer", DataMedia, &file, "value"},
		{"slice pointer", DataCollection, &children, "value"},
		{"map", DataMedia, map[string]any{"fileName": "file"}, "value"},
		{"named text", DataText, namedString(""), "value"}, {"named file", DataMedia, namedFile(file), "value"},
		{"named slice", DataCollection, namedSlice{}, "value"},
		{"text from media", DataText, file, "value"}, {"text from collection", DataText, children, "value"},
		{"media from text", DataMedia, "", "value"}, {"media from collection", DataMedia, children, "value"},
		{"collection from text", DataCollection, "", "value"}, {"collection from media", DataCollection, file, "value"},
		{"number", DataText, 1, "value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := dataValidationData(tc.kind, "", tc.value)
			e := validateDataValueType(d, "/data/0", diagnostic.Context{})
			errs := validateData(d, "/data/0", diagnostic.Context{})
			if tc.field == "" {
				if e != nil || len(errs) != 0 {
					t.Fatalf("valid union rejected: %v, %v", e, errs)
				}
				dataValidationCheckErrors(t, validateDataRole(d, "/data/0", diagnostic.Context{}), diagnostic.Context{})
				return
			}
			dataValidationCheckErrors(t, []*diagnostic.Error{e}, diagnostic.Context{}, "/data/0/"+tc.field)
			dataValidationCheckErrors(t, errs, diagnostic.Context{}, "/data/0/"+tc.field)
		})
	}
}

func TestLessonDataValidationLocalRoles(t *testing.T) {
	roles := []DataRole{RoleTitle, RoleBody, RoleInstruction, RoleBefore, RoleAfter, RoleItems, RoleItem, RoleOptions, RoleOption, RolePositions, RolePosition, RoleHint, RoleScript, RoleSubtitles}
	for _, role := range roles {
		for _, kind := range []DataType{DataText, DataMedia, DataCollection} {
			t.Run(string(role)+"/"+string(kind), func(t *testing.T) {
				var value any = ""
				if kind == DataMedia {
					value = EmbeddedFile{}
				}
				if kind == DataCollection {
					value = []Data{{}, {}}
				}
				errs := validateDataRole(dataValidationData(kind, role, value), "/d", diagnostic.Context{})
				wantKind := kind
				switch role {
				case RoleItems, RoleOptions, RolePositions:
					wantKind = DataCollection
				case RoleInstruction, RoleItem, RoleOption, RolePosition:
					wantKind = DataText
				}
				if kind == wantKind {
					dataValidationCheckErrors(t, errs, diagnostic.Context{})
					return
				}
				dataValidationCheckErrors(t, errs, diagnostic.Context{}, "/d/type")
			})
		}
	}
	for _, role := range []DataRole{"", "BODY", "unknown"} {
		d := dataValidationData(DataText, "", "")
		d.Role = &role
		dataValidationCheckErrors(t, validateDataRole(d, "/d", diagnostic.Context{}), diagnostic.Context{}, "/d/role")
	}
	for _, role := range []DataRole{RoleOptions, RolePositions, RoleItems} {
		for n := 0; n <= 2; n++ {
			d := dataValidationData(DataCollection, role, make([]Data, n))
			errs := validateDataRole(d, "/d", diagnostic.Context{})
			if role != RoleItems && n < 2 {
				dataValidationCheckErrors(t, errs, diagnostic.Context{}, "/d/value")
			} else {
				dataValidationCheckErrors(t, errs, diagnostic.Context{})
			}
		}
	}
}

func TestLessonDataValidationImmediateCollectionRoles(t *testing.T) {
	roles := []DataRole{"", RoleTitle, RoleBody, RoleInstruction, RoleBefore, RoleAfter, RoleItems, RoleItem, RoleOptions, RoleOption, RolePositions, RolePosition, RoleHint, RoleScript, RoleSubtitles}
	for _, parent := range []DataRole{"", RoleBody, RoleItems, RoleOptions, RolePositions} {
		for _, child := range roles {
			t.Run(string(parent)+"/"+string(child), func(t *testing.T) {
				var parentRole *DataRole
				if parent != "" {
					parentRole = &parent
				}
				children := []Data{dataValidationData(DataText, child, "")}
				errs := validateCollectionRoles(parentRole, children, "/d", diagnostic.Context{})
				valid := child != RoleItem && child != RoleOption && child != RolePosition && child != RoleInstruction && child != RoleOptions && child != RolePositions
				switch parent {
				case RoleItems:
					valid = child == RoleItem
				case RoleOptions:
					valid = child == RoleOption
				case RolePositions:
					valid = child == RolePosition
				}
				if valid && len(errs) != 0 || !valid && len(errs) == 0 {
					t.Fatalf("errors = %v, valid = %v", errs, valid)
				}
			})
		}
	}
}

func TestLessonDataValidationIntegratedConstraints(t *testing.T) {
	file := EmbeddedFile{FileName: "без расширения", Content: ""}
	for _, tc := range []struct {
		name  string
		data  Data
		field string
	}{
		{"unknown role", dataValidationData(DataText, "BODY", ""), "role"},
		{"instruction must be text", dataValidationData(DataMedia, RoleInstruction, file), "type"},
		{"items must be collection", dataValidationData(DataText, RoleItems, ""), "type"},
		{"option must be text", dataValidationData(DataMedia, RoleOption, file), "type"},
		{"one option", dataValidationData(DataCollection, RoleOptions, []Data{dataValidationData(DataText, RoleOption, "")}), "value"},
		{"one position", dataValidationData(DataCollection, RolePositions, []Data{dataValidationData(DataText, RolePosition, "")}), "value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataValidationCheckErrors(t, validateData(tc.data, "/d", diagnostic.Context{}), diagnostic.Context{}, "/d/"+tc.field)
		})
	}
	d := dataValidationData(DataText, "", "")
	d.Answers = nil
	dataValidationCheckErrors(t, validateData(d, "/d", diagnostic.Context{}), diagnostic.Context{}, "/d/answers")
	badRole := dataValidationData(DataText, "unknown", "")
	outer := dataValidationData(DataCollection, "", []Data{badRole, d})
	dataValidationCheckErrors(t, validateData(outer, "/d", diagnostic.Context{}), diagnostic.Context{}, "/d/value/0/role", "/d/value/1/answers")
}

func TestLessonDataValidationRecursionAndDiagnostics(t *testing.T) {
	context := diagnostic.Context{Path: "архив.zip", FileName: "урок.json", LessonID: "lesson", PageID: "page", Line: 7, Column: 3}
	root := "/a~1b~0c/0"
	leaf := dataValidationData(DataMedia, RoleScript, EmbeddedFile{FileName: "dir/file.bin", MIMEType: "", Content: "%%%"})
	inner := dataValidationData(DataCollection, "", []Data{leaf})
	d := dataValidationData(DataCollection, RoleBody, []Data{inner})
	before, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	errs := validateData(d, root, context)
	dataValidationCheckErrors(t, errs, context, root+"/value/0/value/0/value/fileName", root+"/value/0/value/0/value/content")
	for i := 0; i < 5; i++ {
		if again := validateData(d, root, context); !reflect.DeepEqual(errs, again) {
			t.Fatalf("nondeterministic errors: %v / %v", errs, again)
		}
	}
	after, err := json.Marshal(d)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("validation mutated input: %s / %s, %v", before, after, err)
	}
	for _, e := range errs {
		if strings.Contains(fmt.Sprint(e, e.Details), "%%%") {
			t.Error("diagnostic exposes complete Base64 content")
		}
	}

	t.Run("child roles are checked recursively", func(t *testing.T) {
		child := dataValidationData(DataText, RoleOption, "")
		bad := dataValidationData(DataCollection, "", []Data{child})
		outer := dataValidationData(DataCollection, RoleBody, []Data{bad})
		dataValidationCheckErrors(t, validateData(outer, root, context), context, root+"/value/0/value/0/role")
	})
	t.Run("answers are required at every depth", func(t *testing.T) {
		child := dataValidationData(DataText, "", "")
		child.Answers = nil
		outer := dataValidationData(DataCollection, RoleBody, []Data{child})
		dataValidationCheckErrors(t, validateData(outer, root, context), context, root+"/value/0/answers")
		child.Answers = []string{"", "не совпадает с текстом", "_____{ANSWER}"}
		child.Value, child.Hint = "<b>\n\n_____{ANSWER}", "непрозрачная подсказка"
		dataValidationCheckErrors(t, validateData(child, root, context), context)
	})
}

func TestLessonDataValidationValidNestedCollections(t *testing.T) {
	item := dataValidationData(DataText, RoleItem, "")
	items := dataValidationData(DataCollection, RoleItems, []Data{item})
	d := dataValidationData(DataCollection, RoleBody, []Data{items, dataValidationData(DataCollection, RoleItems, []Data{})})
	dataValidationCheckErrors(t, validateData(d, "/d", diagnostic.Context{}), diagnostic.Context{})
	for i := 0; i < 512; i++ {
		d = dataValidationData(DataCollection, "", []Data{d})
	}
	dataValidationCheckErrors(t, validateData(d, "/d", diagnostic.Context{}), diagnostic.Context{})
	for _, tc := range []struct{ parent, child DataRole }{{RoleOptions, RoleOption}, {RolePositions, RolePosition}} {
		child := dataValidationData(DataText, tc.child, "")
		d := dataValidationData(DataCollection, tc.parent, []Data{child, child})
		dataValidationCheckErrors(t, validateData(d, "/d", diagnostic.Context{}), diagnostic.Context{})
	}
}
