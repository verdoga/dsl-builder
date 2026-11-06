package lesson

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func validateModelLesson() Lesson {
	return Lesson{FormatVersion: FormatVersion, Document: Document{
		ID: "Efx7", Version: json.Number("1"), LineCount: json.Number("0"),
		CreatedAt: "2024-02-29T23:59:59Z",
		Pages:     []Page{{ID: "efx7-p009", Nodes: []Node{validateModelNode("efx7-n042-v99")}}},
	}}
}

func validateModelNode(id string) Node {
	return Node{ID: id, Type: NodeText, Data: []Data{}, Nodes: []Node{}}
}

func validateModelContext() diagnostic.Context {
	return diagnostic.Context{Path: "архив.zip", FileName: "l001.json", Line: 17, Column: 4}
}

func validateModelCheck(t *testing.T, got []*diagnostic.Error, codes, pointers []string) {
	t.Helper()
	if len(got) != len(pointers) {
		t.Fatalf("errors = %+v, want pointers %v", got, pointers)
	}
	for i, err := range got {
		if err == nil || err.Code != codes[i] {
			t.Fatalf("error[%d] = %+v, want code %q", i, err, codes[i])
		}
		found := false
		for _, detail := range err.Details {
			found = found || detail.Label == "Поле JSON" && detail.Value == pointers[i]
		}
		if !found || !strings.ContainsAny(err.Reason, "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯабвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
			t.Errorf("error[%d] lacks pointer %q or Russian reason: %+v", i, pointers[i], err)
		}
	}
}

func TestLessonValidateNil(t *testing.T) {
	context := validateModelContext()
	errs := Validate(nil, context)
	validateModelCheck(t, errs, []string{CodeContract}, []string{""})
	if errs[0].Context != context {
		t.Fatalf("nil context = %+v, want %+v", errs[0].Context, context)
	}
}

func TestLessonValidateModelFields(t *testing.T) {
	for _, tc := range []struct {
		name, pointer string
		change        func(*Lesson)
	}{
		{"format", "/formatVersion", func(v *Lesson) { v.FormatVersion = "2.0" }},
		{"empty format", "/formatVersion", func(v *Lesson) { v.FormatVersion = "" }},
		{"version zero", "/document/version", func(v *Lesson) { v.Document.Version = "0" }},
		{"version fraction", "/document/version", func(v *Lesson) { v.Document.Version = "1.5" }},
		{"version empty", "/document/version", func(v *Lesson) { v.Document.Version = "" }},
		{"version syntax", "/document/version", func(v *Lesson) { v.Document.Version = "01" }},
		{"negative lines", "/document/lineCount", func(v *Lesson) { v.Document.LineCount = "-1" }},
		{"fraction lines", "/document/lineCount", func(v *Lesson) { v.Document.LineCount = "0.5" }},
		{"empty lines", "/document/lineCount", func(v *Lesson) { v.Document.LineCount = "" }},
		{"calendar", "/document/createdAt", func(v *Lesson) { v.Document.CreatedAt = "2023-02-29T00:00:00Z" }},
		{"UTC spelling", "/document/createdAt", func(v *Lesson) { v.Document.CreatedAt = "2024-02-29T00:00:00+00:00" }},
		{"nil pages", "/document/pages", func(v *Lesson) { v.Document.Pages = nil }},
		{"nil nodes", "/document/pages/0/nodes", func(v *Lesson) { v.Document.Pages[0].Nodes = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := validateModelLesson()
			tc.change(&value)
			errs := Validate(&value, validateModelContext())
			validateModelCheck(t, errs, []string{CodeContract}, []string{tc.pointer})
			want := validateModelContext()
			want.LessonID = value.Document.ID
			if strings.HasPrefix(tc.pointer, "/document/pages/0/") {
				want.PageID = value.Document.Pages[0].ID
			}
			if errs[0].Context != want {
				t.Errorf("context = %+v, want %+v", errs[0].Context, want)
			}
			if tc.pointer != "/formatVersion" {
				context := validateModelContext()
				context.LessonID = value.Document.ID
				validateModelCheck(t, validateDocument(value.Document, context), []string{CodeContract}, []string{tc.pointer})
			}
		})
	}
}

func TestLessonValidateAllowsSchemaValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Lesson)
	}{
		{"empty pages", func(v *Lesson) { v.Document.Pages = []Page{} }},
		{"empty nodes", func(v *Lesson) { v.Document.Pages[0].Nodes = []Node{} }},
		{"empty titles", func(v *Lesson) { s := ""; v.Document.Subtitle = &s; v.Document.Pages[0].Title = &s }},
		{"whitespace titles", func(v *Lesson) { v.Document.Title = " \t\n" }},
		{"decimal integers", func(v *Lesson) { v.Document.Version = "10e-1"; v.Document.LineCount = "0.0" }},
		{"huge integers", func(v *Lesson) { v.Document.Version = "123456789012345678901234567890"; v.Document.LineCount = "1e100" }},
		{"negative zero", func(v *Lesson) { v.Document.LineCount = "-0" }},
		{"same base distinct suffix", func(v *Lesson) {
			v.Document.Pages[0].Nodes = append(v.Document.Pages[0].Nodes, validateModelNode("efx7-n042-v01"))
		}},
		{"arbitrary nesting", func(v *Lesson) {
			v.Document.Pages[0].Nodes[0].Nodes = []Node{validateModelNode("efx7-n003")}
			v.Document.Pages[0].Nodes[0].Nodes[0].Type = NodeTask
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := validateModelLesson()
			tc.change(&value)
			if errs := Validate(&value, validateModelContext()); len(errs) != 0 {
				t.Fatalf("schema-permitted value rejected: %+v", errs)
			}
		})
	}
}

func TestLessonValidatePrivateModelBoundary(t *testing.T) {
	value := validateModelLesson()
	value.Document.ID = "invalid id"
	value.Document.Pages[0].ID = "invalid page"
	value.Document.Pages[0].Nodes[0].ID = "invalid node"
	if errs := validateDocument(value.Document, validateModelContext()); len(errs) != 0 {
		t.Fatalf("validateDocument independently checked IDs: %+v", errs)
	}
	if errs := validatePage(value.Document.Pages[0], "/custom/4", validateModelContext()); len(errs) != 0 {
		t.Fatalf("validatePage independently checked IDs: %+v", errs)
	}
	value.Document.Pages[0].Nodes = nil
	errs := validatePage(value.Document.Pages[0], "/custom/4", validateModelContext())
	validateModelCheck(t, errs, []string{CodeContract}, []string{"/custom/4/nodes"})
	want := validateModelContext()
	want.PageID = "invalid page"
	if errs[0].Context != want {
		t.Errorf("private page context = %+v, want %+v", errs[0].Context, want)
	}
	value.Document.Pages[0].Nodes = []Node{validateModelNode("invalid node")}
	value.Document.Pages[0].Nodes[0].Data = nil
	validateModelCheck(t, validatePage(value.Document.Pages[0], "/custom/4", validateModelContext()),
		[]string{CodeContract}, []string{"/custom/4/nodes/0/data"})
}

func TestLessonValidateModelBeforeIDsAndArrayOrder(t *testing.T) {
	value := validateModelLesson()
	value.FormatVersion, value.Document.Version, value.Document.LineCount = "bad", "0", "-1"
	value.Document.CreatedAt = "2024-04-31T00:00:00Z"
	parent := validateModelNode("bad")
	parent.Data = nil
	child := validateModelNode("efx7-n003")
	child.Data = nil
	parent.Nodes = []Node{child}
	sibling := validateModelNode("efx7-n004")
	sibling.Data = nil
	value.Document.Pages[0].Nodes = []Node{parent, sibling}
	value.Document.Pages = append(value.Document.Pages, Page{ID: "efx7-p002"})
	pointers := []string{"/formatVersion", "/document/version", "/document/lineCount", "/document/createdAt",
		"/document/pages/0/nodes/0/data", "/document/pages/0/nodes/0/nodes/0/data",
		"/document/pages/0/nodes/1/data", "/document/pages/1/nodes", "/document/pages/0/nodes/0/id"}
	codes := []string{CodeContract, CodeContract, CodeContract, CodeContract, CodeContract, CodeContract, CodeContract, CodeContract, identity.CodeInvalidNodeID}
	for repeat := 0; repeat < 3; repeat++ {
		validateModelCheck(t, Validate(&value, validateModelContext()), codes, pointers)
	}
}

func TestLessonValidateCoordinatesRealIDs(t *testing.T) {
	value := validateModelLesson()
	value.Document.Pages[0].ID = "other-p001"
	value.Document.Pages[0].Nodes[0].ID = "other-n002"
	value.Document.Pages = append(value.Document.Pages, Page{ID: "other-p001", Nodes: []Node{validateModelNode("other-n002")}})
	context := validateModelContext()
	errs := Validate(&value, context)
	validateModelCheck(t, errs, []string{CodeIDPrefix, CodeIDPrefix, CodeIDPrefix, CodeDuplicateID, CodeIDPrefix, CodeDuplicateID},
		[]string{"/document/pages/0/id", "/document/pages/0/nodes/0/id", "/document/pages/1/id", "/document/pages/1/id", "/document/pages/1/nodes/0/id", "/document/pages/1/nodes/0/id"})
	if want := ValidateIDs(&value, context); !reflect.DeepEqual(errs, want) {
		t.Fatalf("Validate changed or duplicated real ValidateIDs diagnostics:\ngot %+v\nwant %+v", errs, want)
	}
	for _, err := range errs {
		want := context
		want.LessonID, want.PageID = value.Document.ID, "other-p001"
		if err.Context != want {
			t.Errorf("ID context = %+v, want %+v", err.Context, want)
		}
	}
	for _, duplicate := range []struct {
		index     int
		id, first string
	}{
		{3, "other-p001", "/document/pages/0/id"}, {5, "other-n002", "/document/pages/0/nodes/0/id"},
	} {
		idFound, firstFound := false, false
		for _, detail := range errs[duplicate.index].Details {
			idFound = idFound || detail.Value == duplicate.id
			firstFound = firstFound || detail.Value == duplicate.first
		}
		if !idFound || !firstFound {
			t.Errorf("duplicate lost ID or first location: %+v", errs[duplicate.index])
		}
	}
	value = validateModelLesson()
	value.Document.Pages[0].Nodes[0].ID = "invalid node"
	errs = Validate(&value, context)
	validateModelCheck(t, errs, []string{identity.CodeInvalidNodeID}, []string{"/document/pages/0/nodes/0/id"})
	_, source := identity.ParseNodeID("invalid node")
	for _, detail := range source.Details {
		found := false
		for _, got := range errs[0].Details {
			found = found || got == detail
		}
		if !found {
			t.Errorf("identity detail lost: %+v", detail)
		}
	}
	value = validateModelLesson()
	value.Document.ID = ""
	validateModelCheck(t, Validate(&value, context), []string{identity.CodeInvalidLessonID}, []string{"/document/id"})
}

func validateModelRichLesson() Lesson {
	value := validateModelLesson()
	title, role, flag := " название \n", RoleBody, true
	value.Document.Subtitle, value.Document.Pages[0].Title = &title, &title
	node := &value.Document.Pages[0].Nodes[0]
	node.Type, node.Flags.Checkable = NodeQuestion, &flag
	node.Data = []Data{{Type: DataCollection, Role: &role, Answers: []string{}, Hint: "подсказка", Value: []Data{
		{Type: DataText, Value: "\n_____{ANSWER}\n<b>текст</b>", Answers: []string{"иной ответ", ""}, Hint: "?"},
		{Type: DataMedia, Value: EmbeddedFile{FileName: "нет-на-диске.xyz", MIMEType: "", Content: "AA=="}, Answers: []string{}},
	}}}
	node.Nodes = []Node{validateModelNode("efx7-n1000")}
	return value
}

func TestLessonValidateDoesNotMutateWholeModel(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		value, before := validateModelRichLesson(), validateModelRichLesson()
		if invalid {
			value.FormatVersion, before.FormatVersion = "bad", "bad"
			value.Document.Pages[0].Nodes[0].Nodes[0].ID = value.Document.Pages[0].Nodes[0].ID
			before.Document.Pages[0].Nodes[0].Nodes[0].ID = before.Document.Pages[0].Nodes[0].ID
		}
		errs := Validate(&value, validateModelContext())
		if (len(errs) > 0) != invalid {
			t.Fatalf("invalid=%v, errors=%+v", invalid, errs)
		}
		if !reflect.DeepEqual(value, before) {
			t.Fatalf("Validate mutated model: got %#v, want %#v", value, before)
		}
		validateDocument(value.Document, validateModelContext())
		validatePage(value.Document.Pages[0], "/document/pages/0", validateModelContext())
		if !reflect.DeepEqual(value, before) {
			t.Fatal("private validation mutated model, slices, flags or Data union")
		}
	}
}
