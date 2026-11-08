package lesson

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func matchingInstructionLesson() Lesson {
	value := validateModelLesson()
	instruction := validateModelNode("efx7-n001")
	instruction.Type = NodeInstruction
	instruction.Data = []Data{nodeValidationData(RoleBody, 0)}
	matching := nodeValidationNode(NodeMatching)
	matching.ID = "efx7-n002"
	value.Document.Pages[0].Nodes = []Node{instruction, matching}
	return value
}

func TestLessonMatchingInstructionPublicValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(*Lesson)
		pointers []string
	}{
		{name: "separate instruction body before matching"},
		{
			name: "matching does not require a separate instruction",
			change: func(v *Lesson) {
				v.Document.Pages[0].Nodes = v.Document.Pages[0].Nodes[1:]
			},
		},
		{
			name: "separate instruction need not be adjacent",
			change: func(v *Lesson) {
				nodes := v.Document.Pages[0].Nodes
				v.Document.Pages[0].Nodes = []Node{nodes[0], validateModelNode("efx7-n003"), nodes[1]}
			},
		},
		{
			name: "matching rejects local instruction",
			change: func(v *Lesson) {
				node := &v.Document.Pages[0].Nodes[1]
				node.Data = append(node.Data, nodeValidationData(RoleInstruction, 0))
			},
			pointers: []string{"/document/pages/0/nodes/1/data/2/role"},
		},
		{
			name: "matching still requires options",
			change: func(v *Lesson) {
				node := &v.Document.Pages[0].Nodes[1]
				node.Data = node.Data[1:]
			},
			pointers: []string{"/document/pages/0/nodes/1/data"},
		},
		{
			name: "matching still requires items",
			change: func(v *Lesson) {
				node := &v.Document.Pages[0].Nodes[1]
				node.Data = node.Data[:1]
			},
			pointers: []string{"/document/pages/0/nodes/1/data"},
		},
		{
			name: "single choice permits one local instruction",
			change: func(v *Lesson) {
				node := &v.Document.Pages[0].Nodes[1]
				node.Type = NodeSingleChoice
				node.Data = []Data{node.Data[0], nodeValidationData(RoleInstruction, 0)}
			},
		},
		{
			name: "multiple choice permits one local instruction",
			change: func(v *Lesson) {
				node := &v.Document.Pages[0].Nodes[1]
				node.Type = NodeMultipleChoice
				node.Data = []Data{node.Data[0], nodeValidationData(RoleInstruction, 0)}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := matchingInstructionLesson()
			if tc.change != nil {
				tc.change(&value)
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var before Lesson
			if err := json.Unmarshal(raw, &before); err != nil {
				t.Fatal(err)
			}
			beforeRaw := bytes.Clone(raw)
			codes := make([]string, len(tc.pointers))
			for i := range codes {
				codes[i] = CodeContract
			}

			modelErrors := Validate(&value, validateModelContext())
			validateModelCheck(t, modelErrors, codes, tc.pointers)
			if !reflect.DeepEqual(value, before) {
				t.Fatal("Validate mutated the lesson")
			}
			jsonErrors := ValidateJSON(raw, validateModelContext())
			validateModelCheck(t, jsonErrors, codes, tc.pointers)
			if !bytes.Equal(raw, beforeRaw) {
				t.Fatal("ValidateJSON mutated its input")
			}
			if !reflect.DeepEqual(modelErrors, jsonErrors) {
				t.Fatalf("public validation results differ:\nValidate: %+v\nValidateJSON: %+v", modelErrors, jsonErrors)
			}
		})
	}
}
