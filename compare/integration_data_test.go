package compare_test

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/lesson"
)

func TestTask23EmbeddedBytesNameAndMIMEAreContent(t *testing.T) {
	bytes := []byte{0, 255, 13, 10, 128, 1}
	for _, tc := range []struct {
		name    string
		file    lesson.EmbeddedFile
		changed bool
	}{
		{"same bytes", lesson.EmbeddedFile{FileName: "binary.bin", MIMEType: "application/octet-stream", Content: base64.StdEncoding.EncodeToString(append([]byte{}, bytes...))}, false},
		{"name", lesson.EmbeddedFile{FileName: "renamed.bin", MIMEType: "application/octet-stream", Content: base64.StdEncoding.EncodeToString(bytes)}, true},
		{"mime", lesson.EmbeddedFile{FileName: "binary.bin", MIMEType: "application/custom", Content: base64.StdEncoding.EncodeToString(bytes)}, true},
		{"byte", lesson.EmbeddedFile{FileName: "binary.bin", MIMEType: "application/octet-stream", Content: base64.StdEncoding.EncodeToString([]byte{0, 255, 13, 10, 128, 2})}, true},
		{"empty file", lesson.EmbeddedFile{FileName: "binary.bin", MIMEType: "application/octet-stream", Content: ""}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, candidate := task23Lesson("media"), task23Lesson("media")
			for _, value := range []*lesson.Lesson{old, candidate} {
				node := &value.Document.Pages[0].Nodes[0]
				node.Type = lesson.NodeMedia
				node.Data[0].Type = lesson.DataMedia
			}
			old.Document.Pages[0].Nodes[0].Data[0].Value = lesson.EmbeddedFile{FileName: "binary.bin", MIMEType: "application/octet-stream", Content: base64.StdEncoding.EncodeToString(bytes)}
			candidate.Document.Pages[0].Nodes[0].Data[0].Value = tc.file
			plan := task23Build(t, old, candidate)
			if plan.HasContentChanges() != tc.changed || plan.ReplaceLesson || len(plan.Fields)+len(plan.Reasons) != 0 {
				t.Fatalf("media plan: %+v", plan)
			}
			if tc.changed {
				if len(plan.Pages) != 1 || plan.Pages[0].ReplaceNodes || len(plan.Pages[0].Nodes) != 1 || !reflect.DeepEqual(plan.Pages[0].Nodes[0].Fields, []compare.NodeField{compare.NodeDataChanged}) {
					t.Fatalf("media own fields: %+v", plan)
				}
			} else if len(plan.Pages) != 0 {
				t.Fatalf("equal media: %+v", plan)
			}
		})
	}
}

func TestTask23DataChangesRemainOwnNodeChanges(t *testing.T) {
	for _, name := range []string{"data count", "data type", "text whitespace", "HTML", "multifill", "answers order", "hint", "nested order", "nested role presence"} {
		t.Run(name, func(t *testing.T) {
			old, candidate := task23Lesson("original"), task23Lesson("original")
			left, right := &old.Document.Pages[0].Nodes[0], &candidate.Document.Pages[0].Nodes[0]
			switch name {
			case "data count":
				right.Data = append(right.Data, task23Data("second"))
			case "data type":
				right.Data[0].Type, right.Data[0].Value = lesson.DataCollection, []lesson.Data{task23Data("original")}
			case "text whitespace":
				right.Data[0].Value = "original "
			case "HTML":
				left.Data[0].Value, right.Data[0].Value = "<b>same</b>", "same"
			case "multifill":
				left.Type, right.Type = lesson.NodeMultifill, lesson.NodeMultifill
				left.Data[0].Value, right.Data[0].Value = "I _____{am}.\n\nYou _____{are}.", "I _____{am}.\nYou _____{are}."
			case "answers order":
				left.Type, right.Type = lesson.NodeQuestion, lesson.NodeQuestion
				left.Data[0].Answers, right.Data[0].Answers = []string{"one", "two"}, []string{"two", "one"}
			case "hint":
				right.Data[0].Hint = "new hint"
			case "nested order", "nested role presence":
				left.Data[0].Type, right.Data[0].Type = lesson.DataCollection, lesson.DataCollection
				left.Data[0].Value = []lesson.Data{{Type: lesson.DataCollection, Value: []lesson.Data{task23Data("one"), task23Data("two")}, Answers: []string{}}}
				right.Data[0].Value = []lesson.Data{{Type: lesson.DataCollection, Value: []lesson.Data{task23Data("one"), task23Data("two")}, Answers: []string{}}}
				children := right.Data[0].Value.([]lesson.Data)[0].Value.([]lesson.Data)
				if name == "nested order" {
					children[0], children[1] = children[1], children[0]
				} else {
					children[0].Role = nil
				}
			}
			plan := task23Build(t, old, candidate)
			if plan.ReplaceLesson || !plan.HasContentChanges() || len(plan.Reasons)+len(plan.Fields) != 0 || len(plan.Pages) != 1 {
				t.Fatalf("data plan: %+v", plan)
			}
			page := plan.Pages[0]
			if page.ReplaceNodes || len(page.Reasons) != 0 || len(page.Nodes) != 1 || !reflect.DeepEqual(page.Nodes[0].Path, []int{0}) || !reflect.DeepEqual(page.Nodes[0].Fields, []compare.NodeField{compare.NodeDataChanged}) {
				t.Fatalf("data changes must not change node shape: %+v", page)
			}
		})
	}
}

func TestTask23JSONFormattingAndObjectOrderDoNotCreateChanges(t *testing.T) {
	source := task23Lesson("<b>Русский текст</b>\nsecond line")
	source.Document.Subtitle = task23Text("")
	source.Document.Version = "123456789012345678901234567890123456789"
	compact := task23JSON(t, source)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(compact), &fields); err != nil {
		t.Fatal(err)
	}
	formatted, err := json.MarshalIndent(fields, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if compact == string(formatted) {
		t.Fatal("fixture serializations should differ")
	}
	var old, candidate lesson.Lesson
	if err := json.Unmarshal([]byte(compact), &old); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(formatted, &candidate); err != nil {
		t.Fatal(err)
	}
	plan := task23Build(t, &old, &candidate)
	if plan.HasContentChanges() || plan.ReplaceLesson || len(plan.Fields)+len(plan.Pages)+len(plan.Reasons) != 0 {
		t.Fatalf("serialization was treated as content: %+v", plan)
	}
}
