package lesson_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

func integrationRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/integration/combined.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationDecode(t *testing.T, raw []byte) lesson.Lesson {
	t.Helper()
	if errs := lesson.ValidateJSON(raw, diagnostic.Context{}); len(errs) != 0 {
		t.Fatalf("ValidateJSON: %v", errs)
	}
	var value lesson.Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func integrationDetail(err *diagnostic.Error, value string) bool {
	for _, detail := range err.Details {
		if detail.Value == value {
			return true
		}
	}
	return false
}

func TestIntegrationPublicRoundTrip(t *testing.T) {
	raw := integrationRaw(t)
	original := bytes.Clone(raw)
	value := integrationDecode(t, raw)
	if !bytes.Equal(raw, original) {
		t.Fatal("input bytes changed")
	}
	if value.Document.ID != "A:B" || value.Document.Title != "" || value.Document.Subtitle != nil || value.Document.Pages[0].Title != nil || value.Document.Pages[1].Title == nil || *value.Document.Pages[1].Title != "" {
		t.Fatal("metadata or exact field names lost")
	}
	if value.Document.Version.String() != "123456789012345678901234567890.000e+2" || value.Document.LineCount.String() != "0.0e+2" {
		t.Fatal("number lexemes lost")
	}
	nodes := value.Document.Pages[0].Nodes
	if nodes[0].Flags.Speaking == nil || *nodes[0].Flags.Speaking || nodes[0].Flags.Stepped == nil || nodes[0].Flags.Checkable != nil {
		t.Fatal("task false/absent flags lost")
	}
	media, ok := nodes[12].Data[0].Value.(lesson.EmbeddedFile)
	if !ok || media.FileName != "файл 0" || media.MIMEType != "" || media.Content != "AAE=" {
		t.Fatalf("media union: %#v", nodes[12].Data[0].Value)
	}
	children, ok := nodes[2].Data[0].Value.([]lesson.Data)
	if !ok || children[0].Role != nil || children[0].Value != "<b>Юникод</b>\n\n_____{ANSWER}" {
		t.Fatal("recursive collection/text union lost")
	}
	if nodes[14].Data[0].Value != "<b>Юникод</b>\n\n_____{ANSWER}" || nodes[14].Flags.Checkable == nil || *nodes[14].Flags.Checkable {
		t.Fatal("opaque multifill or false flag lost")
	}
	before := integrationMarshal(t, value)
	if errs := lesson.Validate(&value, diagnostic.Context{Path: "модуль.zip", FileName: "l001.json"}); len(errs) != 0 {
		t.Fatal(errs)
	}
	if !bytes.Equal(before, integrationMarshal(t, value)) {
		t.Fatal("Validate changed model")
	}
	again := integrationDecode(t, before)
	if !bytes.Equal(before, integrationMarshal(t, again)) {
		t.Fatal("round-trip changed typed model")
	}
	value.Document.Pages = []lesson.Page{}
	empty := integrationDecode(t, integrationMarshal(t, value))
	if empty.Document.Pages == nil || len(empty.Document.Pages) != 0 {
		t.Fatal("empty pages became null")
	}
}

func TestIntegrationShapeBeforeSemanticsAndAtomicDecode(t *testing.T) {
	base := string(integrationRaw(t))
	for _, tc := range []struct{ name, old, replacement, pointer string }{
		{"title case", `"title": ""`, `"TiTlE": ""`, "/document/title"},
		{"page null required", `"title": null,`, "", "/document/pages/0/title"},
		{"nested answers null", `"answers": []`, `"answers": null`, "/document/pages/0/nodes/2/data/0/value/0/answers"},
		{"unknown flag escaped", `"speaking": false`, `"speaking": false, "x~/": false`, "/document/pages/0/nodes/0/flags/x~0~1"},
		{"nested media null", `"content": "AAE="`, `"content": null`, "/document/pages/0/nodes/12/data/0/value/content"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(strings.Replace(base, tc.old, tc.replacement, 1))
			raw = bytes.Replace(raw, []byte(`"id": "A:B"`), []byte(`"id": "bad id"`), 1)
			original := bytes.Clone(raw)
			context := diagnostic.Context{Path: "архив.zip", FileName: "l007.json", Line: 8, Column: 3}
			errs := lesson.ValidateJSON(raw, context)
			if len(errs) == 0 {
				t.Fatal("accepted invalid shape")
			}
			found := false
			for _, err := range errs {
				if err.Code != lesson.CodeJSON && !(tc.name == "unknown flag escaped" && err.Code == lesson.CodeContract) {
					t.Fatalf("dependent semantics before shape: %v", errs)
				}
				if slices.Contains(err.Details, diagnostic.Detail{Label: "Поле JSON", Value: tc.pointer}) {
					found = true
				}
				if err.Context.Path != context.Path || err.Context.FileName != context.FileName || err.Context.Line != 8 || err.Context.Column != 3 {
					t.Fatalf("context lost: %+v", err.Context)
				}
			}
			if !found {
				t.Fatalf("missing pointer %s: %+v", tc.pointer, errs)
			}
			value := integrationDecode(t, []byte(base))
			before := integrationMarshal(t, value)
			var detail *diagnostic.Error
			if err := json.Unmarshal(raw, &value); err == nil || !errors.As(err, &detail) {
				t.Fatalf("decode error is not diagnostic: %v", err)
			}
			if !bytes.Equal(before, integrationMarshal(t, value)) || !bytes.Equal(original, raw) {
				t.Fatal("failed decode mutated receiver/input")
			}
		})
	}
}

func TestIntegrationOrderedDiagnosticsAndFullIDs(t *testing.T) {
	value := integrationDecode(t, integrationRaw(t))
	value.FormatVersion = "2.0"
	value.Document.LineCount = "-1"
	value.Document.Pages[0].Nodes[0].Nodes[0].ID = "a:b-n000"
	value.Document.Pages[1].ID = "other-p001"
	value.Document.Pages[1].Nodes = []lesson.Node{value.Document.Pages[0].Nodes[1]}
	context := diagnostic.Context{Path: "архив.zip", FileName: "l001.json", Line: 9, Column: 2}
	before := integrationMarshal(t, value)
	wantCodes := []string{lesson.CodeContract, lesson.CodeContract, identity.CodeInvalidNodeID, lesson.CodeIDPrefix, lesson.CodeDuplicateID}
	wantPointers := []string{"/formatVersion", "/document/lineCount", "/document/pages/0/nodes/0/nodes/0/id", "/document/pages/1/id", "/document/pages/1/nodes/0/id"}
	var first []byte
	for repeat := 0; repeat < 4; repeat++ {
		errs := lesson.Validate(&value, context)
		if len(errs) != len(wantCodes) {
			t.Fatalf("errors: %+v", errs)
		}
		encoded := integrationMarshal(t, errs)
		if repeat == 0 {
			first = encoded
		} else if !bytes.Equal(first, encoded) {
			t.Fatal("diagnostics changed between calls")
		}
		for i, err := range errs {
			if err.Code != wantCodes[i] || !slices.Contains(err.Details, diagnostic.Detail{Label: "Поле JSON", Value: wantPointers[i]}) {
				t.Fatalf("error %d: %+v", i, err)
			}
			if err.Context.Path != context.Path || err.Context.FileName != context.FileName || err.Context.LessonID != "A:B" || err.Context.Line != 9 || err.Context.Column != 2 {
				t.Fatalf("context: %+v", err.Context)
			}
			pageID := "other-p001"
			if i == 2 {
				pageID = "a:b-p001"
			}
			if i >= 2 && err.Context.PageID != pageID {
				t.Fatalf("page context: %+v", err.Context)
			}
		}
		_, original := identity.ParseNodeID("a:b-n000")
		for _, detail := range original.Details {
			if !slices.Contains(errs[2].Details, detail) {
				t.Fatalf("identity detail lost: %+v", detail)
			}
		}
		if !integrationDetail(errs[4], "/document/pages/0/nodes/1/id") || !integrationDetail(errs[4], "a:b-n005") {
			t.Fatal("duplicate missing first pointer/full ID")
		}
	}
	if !bytes.Equal(before, integrationMarshal(t, value)) {
		t.Fatal("errors mutated model")
	}
	valid := integrationDecode(t, integrationRaw(t))
	valid.Document.Version = "1"
	node := valid.Document.Pages[0].Nodes[1]
	node.ID += "-v99"
	valid.Document.Pages[1].Nodes = []lesson.Node{node}
	if errs := lesson.Validate(&valid, context); len(errs) != 0 {
		t.Fatalf("full IDs or revision boundary: %v", errs)
	}
}

func TestIntegrationIdentityAndPageDuplicates(t *testing.T) {
	for _, tc := range []struct{ name, code, value, pointer string }{
		{"lesson", identity.CodeInvalidLessonID, "bad id", "/document/id"},
		{"page", identity.CodeInvalidPageID, "a:b-p000", "/document/pages/1/id"},
		{"duplicate page", lesson.CodeDuplicateID, "a:b-p001", "/document/pages/1/id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := integrationDecode(t, integrationRaw(t))
			if tc.name == "lesson" {
				value.Document.ID = tc.value
				value.Document.Pages = []lesson.Page{}
			} else {
				value.Document.Pages[1].ID = tc.value
			}
			errs := lesson.Validate(&value, diagnostic.Context{})
			if len(errs) != 1 || errs[0].Code != tc.code || !integrationDetail(errs[0], tc.pointer) || !integrationDetail(errs[0], tc.value) {
				t.Fatalf("identity diagnostic: %+v", errs)
			}
			if tc.name == "duplicate page" && !integrationDetail(errs[0], "/document/pages/0/id") {
				t.Fatal("missing first page pointer")
			}
		})
	}
}

func TestIntegrationDataReceiverNestedFailure(t *testing.T) {
	value := integrationDecode(t, integrationRaw(t))
	data := value.Document.Pages[0].Nodes[2].Data[0]
	before := integrationMarshal(t, data)
	raw := bytes.Replace(before, []byte(`"answers":[]`), []byte(`"answers":null`), 1)
	original := bytes.Clone(raw)
	var detail *diagnostic.Error
	if err := json.Unmarshal(raw, &data); err == nil || !errors.As(err, &detail) {
		t.Fatalf("decode error is not diagnostic: %v", err)
	}
	if !bytes.Equal(before, integrationMarshal(t, data)) || !bytes.Equal(raw, original) {
		t.Fatal("nested failure mutated Data/input")
	}
}

func TestIntegrationSemanticFailuresDoNotMutate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*lesson.Lesson)
	}{
		{"wrong union", func(v *lesson.Lesson) { v.Document.Pages[0].Nodes[2].Data[0].Value = lesson.EmbeddedFile{} }},
		{"false inapplicable flag", func(v *lesson.Lesson) { flag := false; v.Document.Pages[0].Nodes[1].Flags.Checkable = &flag }},
		{"foreign collection child", func(v *lesson.Lesson) {
			role := lesson.RoleOption
			data := v.Document.Pages[0].Nodes[2].Data[0].Value.([]lesson.Data)
			data[0].Role = &role
		}},
		{"nil required array", func(v *lesson.Lesson) { v.Document.Pages[0].Nodes[2].Data[0].Answers = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := integrationDecode(t, integrationRaw(t))
			tc.change(&value)
			before := integrationMarshal(t, value)
			if errs := lesson.Validate(&value, diagnostic.Context{}); len(errs) == 0 {
				t.Fatal("accepted invalid model")
			}
			if !bytes.Equal(before, integrationMarshal(t, value)) {
				t.Fatal("validation mutated invalid model")
			}
		})
	}
}

func TestIntegrationRecursivePublicPipeline(t *testing.T) {
	value := integrationDecode(t, integrationRaw(t))
	role := lesson.RoleBody
	data := lesson.Data{Type: lesson.DataText, Value: "конец", Answers: []string{}, Hint: ""}
	node := lesson.Node{ID: "a:b-n999", Type: lesson.NodeText, Data: []lesson.Data{}, Nodes: []lesson.Node{}}
	for i := 0; i < 128; i++ {
		data = lesson.Data{Type: lesson.DataCollection, Value: []lesson.Data{data}, Answers: []string{}, Hint: ""}
		node = lesson.Node{ID: fmt.Sprintf("a:b-n%d", 1000+i), Type: lesson.NodeText, Data: []lesson.Data{}, Nodes: []lesson.Node{node}}
	}
	data.Role = &role
	node.Data = []lesson.Data{data}
	value.Document.Pages[0].Nodes = []lesson.Node{node}
	raw := integrationMarshal(t, value)
	again := integrationDecode(t, raw)
	if !bytes.Equal(raw, integrationMarshal(t, again)) {
		t.Fatal("recursive round-trip lost content/order")
	}
}
