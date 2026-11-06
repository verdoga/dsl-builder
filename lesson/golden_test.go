package lesson

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func TestLessonGoldenHistoricalModels(t *testing.T) {
	cases := []struct {
		pages, nodes, media int
		createdAt           string
	}{
		{1, 7, 1, "2026-11-03T08:34:59Z"},
		{1, 7, 0, "2026-11-03T17:32:07Z"},
		{2, 18, 3, "2026-11-03T17:36:18Z"},
		{3, 3, 0, "2026-11-03T17:54:17Z"},
		{3, 8, 0, "2026-11-03T18:05:25Z"},
		{1, 14, 0, "2026-11-03T18:07:54Z"},
		{1, 15, 0, "2026-11-03T18:11:31Z"},
	}
	for i, want := range cases {
		t.Run(fmt.Sprint(i+1), func(t *testing.T) {
			raw, value, context := goldenRead(t, i+1)
			untouched := bytes.Clone(raw)
			if errs := ValidateJSON(raw, context); len(errs) != 0 {
				t.Fatalf("ValidateJSON: %v", errs)
			}
			if !bytes.Equal(raw, untouched) {
				t.Fatal("ValidateJSON changed its input")
			}
			before := goldenMarshal(t, value)
			if errs := Validate(&value, context); len(errs) != 0 {
				t.Fatalf("Validate: %v", errs)
			}
			if errs := ValidateIDs(&value, context); len(errs) != 0 {
				t.Fatalf("ValidateIDs: %v", errs)
			}
			if !bytes.Equal(before, goldenMarshal(t, value)) {
				t.Fatal("validators changed the model")
			}
			goldenEqualJSON(t, raw, before)
			var again Lesson
			if err := json.Unmarshal(before, &again); err != nil {
				t.Fatal(err)
			}
			goldenEqualJSON(t, before, goldenMarshal(t, again))
			if value.FormatVersion != "1.0" || value.Document.Version.String() != "1" || value.Document.CreatedAt != want.createdAt || len(value.Document.Pages) != want.pages {
				t.Fatal("historical root or document metadata changed")
			}
			goldenProvenance(t, i+1, value.Document)
			nodes, media := 0, 0
			seen := make(map[string]bool)
			for _, page := range value.Document.Pages {
				parts, err := identity.ParsePageID(page.ID)
				if err != nil || parts.Prefix != identity.LessonKey(value.Document.ID) || seen[page.ID] || page.Nodes == nil {
					t.Fatalf("invalid historical page %s", page.ID)
				}
				seen[page.ID] = true
				goldenWalk(t, page.Nodes, identity.LessonKey(value.Document.ID), seen, &nodes, &media)
			}
			if nodes != want.nodes || media != want.media {
				t.Fatalf("visited nodes/media = %d/%d, want %d/%d", nodes, media, want.nodes, want.media)
			}
		})
	}
}

func TestLessonGoldenRejectsInvalidCopies(t *testing.T) {
	cases := []struct {
		name, code, pointer string
		fixture             int
		change              func(*Lesson)
	}{
		{"duplicate", CodeDuplicateID, "/document/pages/0/nodes/1/id", 6, func(v *Lesson) { v.Document.Pages[0].Nodes[1].ID = v.Document.Pages[0].Nodes[0].ID }},
		{"foreign descendant", CodeIDPrefix, "/document/pages/0/nodes/0/nodes/0/id", 6, func(v *Lesson) { v.Document.Pages[0].Nodes[0].Nodes[0].ID = "other-n002" }},
		{"flag even false", CodeContract, "/document/pages/0/nodes/0/flags/checkable", 6, func(v *Lesson) { f := false; v.Document.Pages[0].Nodes[0].Flags.Checkable = &f }},
		{"one option", CodeContract, "/document/pages/0/nodes/0/nodes/1/data/0/value", 6, func(v *Lesson) {
			d := &v.Document.Pages[0].Nodes[0].Nodes[1].Data[0]
			d.Value = d.Value.([]Data)[:1]
		}},
		{"foreign collection role", CodeContract, "/document/pages/0/nodes/0/nodes/1/data/0/value/0/role", 6, func(v *Lesson) {
			r := RoleItem
			v.Document.Pages[0].Nodes[0].Nodes[1].Data[0].Value.([]Data)[0].Role = &r
		}},
		{"zero version", CodeContract, "/document/version", 6, func(v *Lesson) { v.Document.Version = "0" }},
		{"calendar", CodeContract, "/document/createdAt", 6, func(v *Lesson) { v.Document.CreatedAt = "2026-02-30T18:07:54Z" }},
		{"base64", CodeContract, "/document/pages/0/nodes/1/nodes/0/data/0/value/content", 1, func(v *Lesson) {
			d := &v.Document.Pages[0].Nodes[1].Nodes[0].Data[0]
			f := d.Value.(EmbeddedFile)
			f.Content = "AA"
			d.Value = f
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, value, context := goldenRead(t, tc.fixture)
			tc.change(&value)
			before := goldenMarshal(t, value)
			goldenError(t, Validate(&value, context), tc.code, tc.pointer, context)
			goldenError(t, ValidateJSON(before, context), tc.code, tc.pointer, context)
			if !bytes.Equal(before, goldenMarshal(t, value)) {
				t.Fatal("rejected model was changed")
			}
		})
	}
}

func TestLessonGoldenRequiresJSONFields(t *testing.T) {
	raw, _, context := goldenRead(t, 6)
	for _, tc := range []struct{ name, old, replacement, pointer string }{
		{"missing hint", `"hint": ""`, `"HINT": ""`, "/document/pages/0/nodes/0/data/0/hint"},
		{"null answers", `"answers": []`, `"answers": null`, "/document/pages/0/nodes/0/data/0/answers"},
		{"missing page title", `"title": null`, `"TITLE": null`, "/document/pages/0/title"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := bytes.Replace(raw, []byte(tc.old), []byte(tc.replacement), 1)
			if bytes.Equal(raw, bad) {
				t.Fatal("fixture mutation did not apply")
			}
			goldenError(t, ValidateJSON(bad, context), CodeJSON, tc.pointer, context)
		})
	}
}

func goldenRead(t *testing.T, number int) ([]byte, Lesson, diagnostic.Context) {
	t.Helper()
	path := filepath.Join("..", "golden", fmt.Sprintf("%d. lesson.json", number))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value Lesson
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return raw, value, diagnostic.Context{Path: path, FileName: "l001.json"}
}

func goldenProvenance(t *testing.T, number int, document Document) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", number)))
	if err != nil {
		t.Fatal(err)
	}
	// Только происхождение метаданных; строки parsed не служат входом lesson.
	var source struct {
		Document struct {
			LineCount json.Number `json:"lineCount"`
			Metadata  struct {
				ID       string  `json:"documentId"`
				Title    string  `json:"title"`
				Subtitle *string `json:"subtitle"`
			} `json:"metadata"`
		} `json:"document"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	m := source.Document.Metadata
	if document.ID != m.ID || document.Title != m.Title || document.LineCount != source.Document.LineCount || !bytes.Equal(goldenMarshal(t, document.Subtitle), goldenMarshal(t, m.Subtitle)) {
		t.Fatal("historical metadata differs from parsed provenance")
	}
}

func goldenWalk(t *testing.T, nodes []Node, prefix string, seen map[string]bool, count, media *int) {
	t.Helper()
	for _, node := range nodes {
		*count += 1
		parts, err := identity.ParseNodeID(node.ID)
		if err != nil || parts.Prefix != prefix || seen[node.ID] || node.Nodes == nil || node.Data == nil {
			t.Fatalf("invalid historical node %s", node.ID)
		}
		seen[node.ID] = true
		goldenData(t, node.Data, media)
		goldenWalk(t, node.Nodes, prefix, seen, count, media)
	}
}

func goldenData(t *testing.T, data []Data, media *int) {
	t.Helper()
	for _, d := range data {
		if d.Answers == nil {
			t.Fatal("answers lost its empty array")
		}
		switch d.Type {
		case DataText:
			if _, ok := d.Value.(string); !ok {
				t.Fatalf("text value has type %T", d.Value)
			}
		case DataMedia:
			f, ok := d.Value.(EmbeddedFile)
			if !ok {
				t.Fatalf("media value has type %T", d.Value)
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(f.Content)
			// AA== в исторических примерах — один нулевой байт, не медиафайл.
			if err != nil || !bytes.Equal(decoded, []byte{0}) {
				t.Fatal("historical Base64 placeholder changed")
			}
			*media += 1
		case DataCollection:
			children, ok := d.Value.([]Data)
			if !ok || children == nil {
				t.Fatalf("collection value has type %T or is nil", d.Value)
			}
			goldenData(t, children, media)
		default:
			t.Fatalf("unexpected historical data type %s", d.Type)
		}
	}
}

func goldenMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func goldenEqualJSON(t *testing.T, want, got []byte) {
	t.Helper()
	// Стандартное JSON-дерево нужно лишь для сравнения всех полей без порядка ключей.
	canonical := func(raw []byte) []byte {
		var tree any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&tree); err != nil {
			t.Fatal(err)
		}
		return goldenMarshal(t, tree)
	}
	if !bytes.Equal(canonical(want), canonical(got)) {
		t.Fatal("JSON round-trip changed fields, values, numbers, flags, or array order")
	}
}

func goldenError(t *testing.T, errs []*diagnostic.Error, code, pointer string, context diagnostic.Context) {
	t.Helper()
	for _, err := range errs {
		if err == nil || err.Code != code {
			continue
		}
		if code == CodeDuplicateID {
			for _, required := range []string{"/document/pages/0/nodes/0/id", "/document/pages/0/nodes/1/id", "ba86-n001"} {
				found := false
				for _, detail := range err.Details {
					found = found || detail.Value == required
				}
				if !found {
					t.Fatalf("duplicate lacks location or full ID %s: %+v", required, err.Details)
				}
			}
		}
		for _, detail := range err.Details {
			if detail.Label == "Поле JSON" && detail.Value == pointer && err.Reason != "" && err.Context.Path == context.Path && err.Context.FileName == context.FileName {
				return
			}
		}
	}
	t.Fatalf("no %s at %s with preserved context: %v", code, pointer, errs)
}
