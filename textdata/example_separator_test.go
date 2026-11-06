package textdata

import (
	"bytes"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func exampleQALegacy(number int, prefix, suffix string) model.Line {
	line := exampleQALine(number, "---", "---")
	line.Raw = prefix + "---" + suffix
	line.Elements[0].Start = utf8.RuneCountInString(prefix) + 1
	line.Elements[0].End = line.Elements[0].Start + 3
	return line
}

func exampleQANormative(number int) model.Line {
	line := exampleQALegacy(number, " \t", "\t ")
	line.LineType = model.LineTypeSeparator
	return line
}

func TestExampleQASeparatorRecognition(t *testing.T) {
	for _, tc := range []struct {
		name string
		line model.Line
		want bool
	}{
		{"normative", exampleQANormative(11), true},
		{"legacy", exampleQALegacy(11, "", ""), true},
		{"legacy-space-tab", exampleQALegacy(11, "\t  ", " \t"), true},
		{"unicode-prefix", exampleQALegacy(11, "🙂", ""), false},
		{"unicode-suffix", exampleQALegacy(11, "", "ж"), false},
		{"nbsp-prefix", exampleQALegacy(11, "\u00a0", ""), false},
		{"nbsp-suffix", exampleQALegacy(11, "", "\u00a0"), false},
		{"escaped-raw", exampleQALine(11, `\---`, "---"), false},
		{"arrow", exampleQALine(11, "A -> B → C", "A -> B → C"), false},
		{"within-line", exampleQALine(11, "A --- B", "A --- B"), false},
		{"value-only", exampleQALine(11, `\-\-\-`, "---"), false},
		{"inline-declaration", exampleQADeclaration(false, "---"), false},
		{"blank", exampleQABlank(11), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := exampleQASnapshot(t, tc.line)
			if got := isExampleSeparator(tc.line); got != tc.want {
				t.Errorf("isExampleSeparator(%#v) = %v, want %v", tc.line, got, tc.want)
			}
			if !bytes.Equal(before, exampleQASnapshot(t, tc.line)) {
				t.Fatal("predicate changed input")
			}
		})
	}
}

func TestExampleQASeparatorRequiresWholeLegacyShape(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.Line)
	}{
		{"nil-value", func(line *model.Line) { line.Elements[0].Value = nil }},
		{"different-value", func(line *model.Line) { value := "другое"; line.Elements[0].Value = &value }},
		{"no-content", func(line *model.Line) { line.Elements = []model.Element{} }},
		{"two-content-elements", func(line *model.Line) {
			line.Raw = "--- x"
			line.Elements = append(line.Elements, exampleQAElement(model.ElementTypeContent, "x", "x", 5))
		}},
		// Некорректные диапазоны проверяют безопасную границу самого predicate.
		{"zero-start", func(line *model.Line) { line.Elements[0].Start = 0 }},
		{"end-outside", func(line *model.Line) { line.Elements[0].End = 5 }},
		{"reversed-range", func(line *model.Line) { line.Elements[0].Start, line.Elements[0].End = 4, 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := exampleQALegacy(11, "", "")
			tc.change(&line)
			if isExampleSeparator(line) {
				t.Fatalf("recognized incomplete legacy shape: %#v", line)
			}
		})
	}
}

func TestExampleQASplitKeepsExactOwnLines(t *testing.T) {
	left := []model.Line{exampleQABlank(11), exampleQALine(20, "До", "До"), exampleQABlank(21)}
	right := []model.Line{exampleQABlank(40), exampleQALine(50, "После", "После")}
	for _, separator := range []model.Line{exampleQANormative(30), exampleQALegacy(30, "\t", " ")} {
		lines := append(append(append([]model.Line{}, left...), separator), right...)
		original := exampleQASnapshot(t, lines)
		before, after, separated, err := splitExample(lines, diagnostic.Context{})
		if err != nil || !separated {
			t.Fatalf("split = %v, %v", separated, err)
		}
		if !bytes.Equal(exampleQASnapshot(t, before), exampleQASnapshot(t, left)) || !bytes.Equal(exampleQASnapshot(t, after), exampleQASnapshot(t, right)) {
			t.Fatalf("split lost or changed own lines: before=%#v, after=%#v", before, after)
		}
		if !bytes.Equal(original, exampleQASnapshot(t, lines)) {
			t.Fatal("split changed input")
		}
	}
	for _, lines := range [][]model.Line{
		nil,
		{exampleQABlank(11)},
		{exampleQALine(11, `\---`, "---"), exampleQALine(15, "A --- B → C", "A --- B → C")},
	} {
		before, after, separated, err := splitExample(lines, diagnostic.Context{})
		if err != nil || separated || len(after) != 0 || len(before) != len(lines) {
			t.Fatalf("unseparated split: before=%#v, after=%#v, separated=%v, err=%v", before, after, separated, err)
		}
		if len(lines) != 0 && !bytes.Equal(exampleQASnapshot(t, before), exampleQASnapshot(t, lines)) {
			t.Fatal("unseparated split changed own lines")
		}
	}
}

func TestExampleQASeparatorErrors(t *testing.T) {
	context := diagnostic.Context{Path: `C:\уроки\вход.parsed.json`, FileName: "l014.json", LessonID: "EX14", PageID: "ex14-p001", Line: 999, Column: 777}
	for _, tc := range []struct {
		name       string
		lines      []model.Line
		separators []int
	}{
		{"repeated", []model.Line{exampleQALine(11, "До", "До"), exampleQANormative(12), exampleQALine(13, "Между", "Между"), exampleQALegacy(14, " \t", ""), exampleQALine(15, "После", "После")}, []int{12, 14}},
		{"leading", []model.Line{exampleQANormative(11), exampleQALine(12, "После", "После")}, []int{11}},
		{"trailing", []model.Line{exampleQALine(11, "До", "До"), exampleQANormative(12)}, []int{12}},
		{"blank-before", []model.Line{exampleQABlank(11), exampleQALegacy(12, " \t", ""), exampleQALine(13, "После", "После")}, []int{12}},
		{"empty-value-after", []model.Line{exampleQALine(11, "До", "До"), exampleQANormative(12), exampleQALine(13, "", ""), exampleQABlank(14)}, []int{12}},
		{"only-separator", []model.Line{exampleQANormative(11)}, []int{11}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := exampleQASnapshot(t, tc.lines)
			_, _, _, err := splitExample(tc.lines, context)
			exampleQASeparatorError(t, err, context, tc.separators)
			got, buildErr := buildExample(exampleQADeclaration(true, "Название"), tc.lines, context)
			if got != nil {
				t.Fatalf("returned partial example data on separator error: %#v", got)
			}
			exampleQASeparatorError(t, buildErr, context, tc.separators)
			if !bytes.Equal(original, exampleQASnapshot(t, tc.lines)) {
				t.Fatal("error changed input")
			}
		})
	}
}

func exampleQASeparatorError(t *testing.T, err *diagnostic.Error, context diagnostic.Context, separators []int) {
	t.Helper()
	if err == nil || err.Code != CodeExampleSeparator {
		t.Fatalf("error = %v, want CodeExampleSeparator", err)
	}
	if err.Context.Path != context.Path || err.Context.FileName != context.FileName || err.Context.LessonID != context.LessonID || err.Context.PageID != context.PageID {
		t.Errorf("lost outer context: %#v", err.Context)
	}
	hasRussian := false
	for _, r := range err.Reason {
		hasRussian = hasRussian || unicode.In(r, unicode.Cyrillic)
	}
	if !hasRussian {
		t.Errorf("missing Russian reason: %q", err.Reason)
	}
	foundPrimary := false
	for _, number := range separators {
		foundPrimary = foundPrimary || err.Context.Line == number
	}
	// Колонка 0 допустима для диагностики всей строки; элемент начинается с 3.
	if !foundPrimary || (err.Context.Column != 0 && err.Context.Column != 3) {
		t.Errorf("wrong separator coordinates: %#v", err.Context)
	}
	if len(separators) > 1 {
		for _, number := range separators {
			found := false
			for _, related := range err.Related {
				found = found || (related.Path == context.Path && related.Line == number)
			}
			if !found {
				t.Errorf("separator %d missing from Related: %#v", number, err.Related)
			}
		}
	}
}
