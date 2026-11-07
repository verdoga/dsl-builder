package convert

import (
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestResourcesMediaSourcePreservesSemanticValuesAndRaw(t *testing.T) {
	for _, value := range []string{"", "003", "12.01", "  русский фрагмент  ", "a/../b.mp3", `a\b.mp3`} {
		t.Run(value, func(t *testing.T) {
			line := resourcesMediaLine(t, 37, "audio", `"opaque raw"`, value)
			before := resourcesJSON(t, line)
			kind, source, problem := mediaSource(line, diagnostic.Context{Path: "input.json", Line: 99, Column: 88})
			if problem != nil {
				t.Fatal(problem)
			}
			if kind != "audio" || source.Value != line.Elements[2].Value || resourcesJSON(t, source) != resourcesJSON(t, line.Elements[2]) {
				t.Fatalf("parameters changed: kind=%q source=%+v", kind, source)
			}
			if resourcesJSON(t, line) != before {
				t.Fatal("mediaSource changed input")
			}
		})
	}
}

func TestResourcesMediaSourceRejectsMissingUnknownAndConflictingElements(t *testing.T) {
	base := diagnostic.Context{Path: "технический.json", FileName: "l003.json", LessonID: "RES1", Line: 91, Column: 72}
	for _, scenario := range []struct {
		name            string
		edit            func(*model.Line)
		conflictColumns []int
	}{
		{"missing-media-type", func(line *model.Line) { line.Elements = append(line.Elements[:1], line.Elements[2:]...) }, nil},
		{"missing-source", func(line *model.Line) { line.Elements = line.Elements[:2] }, nil},
		{"unknown-media-type-value", func(line *model.Line) { line.Elements[1].Value = nil }, nil},
		{"unknown-source-value", func(line *model.Line) { line.Elements[2].Value = nil }, nil},
		{"two-media-types", func(line *model.Line) {
			*line = resourcesLine(t, 37, model.LineTypeTag, line.Elements[0], line.Elements[1],
				resourcesElement(t, model.ElementTypeMediaType, "video", "video"), line.Elements[2])
		}, []int{8, 14}},
		{"two-sources", func(line *model.Line) {
			*line = resourcesLine(t, 37, model.LineTypeTag, line.Elements[0], line.Elements[1], line.Elements[2],
				resourcesElement(t, model.ElementTypeSource, "second", "second"))
		}, []int{14, 18}},
		{"both-conflicts", func(line *model.Line) {
			*line = resourcesLine(t, 37, model.LineTypeTag, line.Elements[0], line.Elements[1],
				resourcesElement(t, model.ElementTypeMediaType, "video", "video"), line.Elements[2],
				resourcesElement(t, model.ElementTypeSource, "second", "second"))
		}, []int{8, 14, 20, 24}},
		{"content-line", func(line *model.Line) { line.LineType = model.LineTypeContent }, nil},
		{"block-declaration", func(line *model.Line) { line.LineType = model.LineTypeBlockStart }, nil},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			line := resourcesMediaLine(t, 37, "audio", "003", "003")
			scenario.edit(&line)
			before := resourcesJSON(t, line)
			_, _, problem := mediaSource(line, base)
			if problem == nil || problem.Code != CodeMediaSource {
				t.Fatalf("expected CodeMediaSource, got %+v", problem)
			}
			if problem.Context.Path != base.Path || problem.Context.FileName != base.FileName || problem.Context.LessonID != base.LessonID || problem.Context.Line != 37 {
				t.Fatalf("lost original coordinates/context: %+v", problem.Context)
			}
			if problem.Context.Column == 72 {
				t.Fatal("stale column survived")
			}
			for _, column := range scenario.conflictColumns {
				found := false
				for _, related := range problem.Related {
					if related.Path == base.Path && related.Line == 37 && related.Column == column {
						found = true
					}
				}
				if !found {
					t.Errorf("conflict at column %d missing in %+v", column, problem.Related)
				}
			}
			if resourcesJSON(t, line) != before {
				t.Fatal("mediaSource changed rejected input")
			}
		})
	}
}
