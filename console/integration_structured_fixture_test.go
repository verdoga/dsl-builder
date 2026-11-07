package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func integrationTestStructuredSource(t *testing.T, dir, id, title string, subtitle *string, pages [][]string, titles []*string) string {
	t.Helper()
	v := model.Result{Format: model.FormatVersion1, Document: model.Document{DSLVersion: integrationTestPointer("1.2"), Encoding: integrationTestPointer("UTF-8"), HasBOM: integrationTestPointer(false), Metadata: model.DocumentMetadata{DocumentID: &id, Title: &title, Subtitle: subtitle, Order: integrationTestPointer("1"), ResourceDirs: []string{}}}, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{}, Lines: []model.Line{}}
	element := func(kind model.ElementType, raw, value string, start int) model.Element {
		return model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: start + len([]rune(raw)), ErrorIDs: []string{}}
	}
	add := func(raw string, kind model.LineType, parent *int, elements ...model.Element) {
		nesting := 0
		if parent != nil {
			nesting = 1
		}
		v.Lines = append(v.Lines, model.Line{Number: len(v.Lines) + 1, LineType: kind, NestingLevel: nesting, ParentLine: parent, Raw: raw, LineEnding: model.LineEndingLF, Elements: elements})
	}
	for i, texts := range pages {
		if titles[i] != nil {
			s := *titles[i]
			add("### "+s, model.LineTypeHeading, nil, element(model.ElementTypeHeadingLevel, "###", "3", 1), element(model.ElementTypeTitle, s, s, 5))
		}
		add("@task a", model.LineTypeTag, nil, element(model.ElementTypeTag, "@task", "task", 1), element(model.ElementTypeIdentifier, "a", "a", 7))
		parent := len(v.Lines)
		for _, body := range texts {
			add("@instr "+body, model.LineTypeTag, &parent, element(model.ElementTypeTag, "@instr", "instr", 1), element(model.ElementTypeContent, body, body, 8))
		}
	}
	v.Document.LineCount = integrationTestPointer(len(v.Lines))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".json")
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, issues := parsed.Load(path)
	integrationTestIssues(t, issues)
	return path
}
