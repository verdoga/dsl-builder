package parsed

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/order"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func validateLinesTestElement(kind model.ElementType, raw string, value *string, start, end int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: value, Start: start, End: end, ErrorIDs: []string{}}
}

func validateLinesTestString(value string) *string { return &value }

func validateLinesTestContext() diagnostic.Context {
	return diagnostic.Context{Path: `C:\JSON\Урок.json`, FileName: "l007.json", LessonID: "Урок+7", PageID: "урок-p001", Line: 23, Column: 4}
}

func validateLinesTestError(t *testing.T, got *diagnostic.Error, want bool) {
	t.Helper()
	if (got != nil) != want {
		t.Fatalf("ошибка = %v; ожидалась ошибка: %v", got, want)
	}
	if got != nil {
		context := validateLinesTestContext()
		if got.Code == "" || strings.TrimSpace(got.Reason) == "" {
			t.Errorf("нет кода или причины: %#v", got)
		}
		if got.Context.Path != context.Path || got.Context.LessonID != context.LessonID || got.Context.FileName != context.FileName || got.Context.PageID != context.PageID {
			t.Errorf("утрачен известный контекст: %#v", got.Context)
		}
	}
}

func validateLinesTestErrors(t *testing.T, got []*diagnostic.Error, want bool) {
	t.Helper()
	if (len(got) > 0) != want {
		t.Fatalf("ошибки = %v; ожидалась ошибка: %v", got, want)
	}
	for _, err := range got {
		validateLinesTestError(t, err, true)
	}
}

func TestValidateLinesContractLine(t *testing.T) {
	for _, kind := range []model.LineType{model.LineTypeTag, model.LineTypeHeading, model.LineTypeContent, model.LineTypeBlockStart, model.LineTypeBlockEnd, model.LineTypeSeparator, model.LineTypeBlank, model.LineTypeInvalid} {
		t.Run(string(kind), func(t *testing.T) {
			line := model.Line{Number: 23, LineType: kind, Raw: "  \tпроизвольное содержимое  ", LineEnding: model.LineEndingCRLF, Elements: []model.Element{}}
			switch kind {
			case model.LineTypeBlockStart:
				line.Raw = "  { "
				line.Elements = []model.Element{validateLinesTestElement(model.ElementTypeBlockOpen, "{", validateLinesTestString("{"), 3, 4)}
			case model.LineTypeBlockEnd:
				line.Raw = " } "
				line.Elements = []model.Element{validateLinesTestElement(model.ElementTypeBlockClose, "}", validateLinesTestString("}"), 2, 3)}
			case model.LineTypeInvalid:
				line.Raw = "?"
				line.Elements = []model.Element{validateLinesTestElement(model.ElementTypeUnparsed, "?", nil, 1, 2)}
				line.Elements[0].ErrorIDs = []string{"parser-error"}
			}
			before, _ := json.Marshal(line)
			validateLinesTestErrors(t, validateLine(line, false, validateLinesTestContext()), false)
			after, _ := json.Marshal(line)
			if string(after) != string(before) {
				t.Fatal("строка нормализована")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		ending model.LineEnding
		last   bool
		raw    string
		level  int
		kind   model.LineType
		bad    bool
	}{
		{"LF", "\n", false, "  \t ", 0, "content", false}, {"CRLF-last", "\r\n", true, "", 0, "blank", false},
		{"EOF-last", "", true, "", 0, "content", false}, {"EOF-middle", "", false, "", 0, "content", true},
		{"CR", "\r", true, "", 0, "content", true}, {"unknown-EOL", "\n\n", true, "", 0, "content", true},
		{"raw-LF", "\n", true, "a\nb", 0, "content", true}, {"raw-CR", "\n", true, "a\rb", 0, "content", true},
		{"negative-level", "\n", true, "", -1, "content", true}, {"unknown-type", "\n", true, "", 0, "text", true},
		{"empty-type", "\n", true, "", 0, "", true}, {"case-type", "\n", true, "", 0, "Content", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validateLinesTestErrors(t, validateLine(model.Line{Number: 23, LineType: tc.kind, Raw: tc.raw, NestingLevel: tc.level, LineEnding: tc.ending}, tc.last, validateLinesTestContext()), tc.bad)
		})
	}
	line := model.Line{Number: 23, LineType: model.LineTypeContent, Raw: "ab", LineEnding: "\n", Elements: []model.Element{validateLinesTestElement("content", "b", validateLinesTestString("b"), 1, 2)}}
	validateLinesTestErrors(t, validateLine(line, true, validateLinesTestContext()), true)
	line.Elements = []model.Element{validateLinesTestElement("content", "b", validateLinesTestString("b"), 2, 3), validateLinesTestElement("content", "a", validateLinesTestString("a"), 1, 2)}
	validateLinesTestErrors(t, validateLine(line, true, validateLinesTestContext()), true)
}

func TestValidateLinesContractUnicodeRanges(t *testing.T) {
	line := model.Line{Number: 23, Raw: "\tЯ😀e\u0301 Z "}
	for _, tc := range []struct {
		name, raw  string
		start, end int
		bad        bool
	}{
		{"Cyrillic", "Я", 2, 3, false}, {"non-BMP", "😀", 3, 4, false}, {"combining", "e\u0301", 4, 6, false},
		{"whole", line.Raw, 1, 9, false}, {"empty-begin", "", 1, 1, false}, {"empty-end", "", 9, 9, false},
		{"empty-line", "", 1, 1, false}, {"zero-start", "", 0, 1, true}, {"reverse", "", 4, 3, true},
		{"beyond-end", "", 9, 10, true}, {"beyond-insertion", "", 10, 10, true}, {"bytes-not-runes", "😀", 3, 7, true},
		{"exclusive-end", "Я😀", 2, 3, true}, {"empty-raw-required", "Я", 2, 2, true}, {"no-normalization", "é", 4, 6, true},
		{"no-trimming", "Z", 7, 9, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := line
			if tc.name == "empty-line" {
				current.Raw = ""
			}
			e := validateLinesTestElement(model.ElementTypeContent, tc.raw, validateLinesTestString("семантика отличается от raw"), tc.start, tc.end)
			validateLinesTestErrors(t, validateElement(e, current, validateLinesTestContext()), tc.bad)
		})
	}
	for _, kind := range []model.ElementType{"", "unknown", "Content"} {
		validateLinesTestErrors(t, validateElement(validateLinesTestElement(kind, "Я", validateLinesTestString("Я"), 2, 3), line, validateLinesTestContext()), true)
	}
}

func TestValidateLinesContractElementOrder(t *testing.T) {
	a := validateLinesTestElement("content", "a", validateLinesTestString("a"), 2, 3)
	b := validateLinesTestElement("content", "b", validateLinesTestString("b"), 3, 5)
	empty := validateLinesTestElement("title", "", validateLinesTestString(""), 2, 2)
	for _, tc := range []struct {
		name     string
		elements []model.Element
		bad      bool
	}{
		{"nil", nil, false}, {"empty", []model.Element{}, false}, {"one", []model.Element{a}, false},
		{"adjacent", []model.Element{a, b}, false}, {"gaps", []model.Element{empty, b}, false},
		{"same-start-end-order", []model.Element{empty, a}, false}, {"reverse-start", []model.Element{b, a}, true},
		{"reverse-end", []model.Element{a, empty}, true}, {"overlap", []model.Element{a, validateLinesTestElement("title", "ab", validateLinesTestString("ab"), 2, 4)}, true},
		{"duplicate", []model.Element{a, a}, true}, {"empty-duplicate", []model.Element{empty, empty}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal(tc.elements)
			validateLinesTestError(t, validateElementOrder(tc.elements, validateLinesTestContext()), tc.bad)
			after, _ := json.Marshal(tc.elements)
			if string(after) != string(before) {
				t.Fatal("элементы изменены или пересортированы")
			}
		})
	}
}

func TestValidateLinesContractValues(t *testing.T) {
	for _, tag := range strings.Fields("dsl-version document-id section order resource-dir header task endtask step speaking newpage editor media example wordlist table script text key instr note alt hint fragment include answer question multifill choice multichoice matching ordering variants variant") {
		t.Run("tag/"+tag, func(t *testing.T) {
			validateLinesTestError(t, validateElementValue(validateLinesTestElement("tag", "@"+strings.ToUpper(tag), &tag, 1, 2), validateLinesTestContext()), false)
		})
	}
	for _, tc := range []struct {
		kind       model.ElementType
		raw, value string
		bad        bool
	}{
		{"tag", "@future", "future", true}, {"tag", "@TASK", "TASK", true}, {"tag", "@task", "@task", true}, {"tag", "@task", "", true},
		{"heading-level", "#", "1", false}, {"heading-level", "##", "2", false}, {"heading-level", "###", "3", false},
		{"heading-level", "##", "1", true}, {"heading-level", "####", "4", true}, {"heading-level", " #", "1", true},
		{"media-type", "AUDIO", "audio", false}, {"media-type", "video", "video", false}, {"media-type", "image", "image", false},
		{"media-type", "Audio", "Audio", true}, {"media-type", "other", "other", true},
		{"block-open", "{", "{", false}, {"block-close", "}", "}", false}, {"block-open", "}", "{", true}, {"block-close", "}", "{", true}, {"block-open", " {", "{", true},
		{"title", "\\@raw", "  Свободный\tтекст\n🙂  ", false}, {"content", "_____{answer} <b>|</b>", "_____{answer} <b>|</b>", false},
		{"content", "", "", false}, {"title", "", "", false}, {"identifier", "A:B+1", "A:B+1", false}, {"name", " section  name ", " section  name ", false},
		{"version", "9.future", "9.future", false}, {"source", "12.01 track A", "12.01 track A", false}, {"source", `"C:\Медиа\a,b.mp3"`, `C:\Медиа\a,b.mp3`, false},
		{"resource-path", `"..\a,b"`, `..\a,b`, false}, {"placeholder", "_____{ответ}", "ответ", false}, {"placeholder", "upstream-form", "не разбирать заново", false},
	} {
		t.Run(string(tc.kind)+"/"+tc.raw+"/"+tc.value, func(t *testing.T) {
			e := validateLinesTestElement(tc.kind, tc.raw, &tc.value, 1, len([]rune(tc.raw))+1)
			before, _ := json.Marshal(e)
			validateLinesTestError(t, validateElementValue(e, validateLinesTestContext()), tc.bad)
			if !tc.bad {
				validateLinesTestErrors(t, validateElement(e, model.Line{Number: 23, Raw: tc.raw}, validateLinesTestContext()), false)
			}
			after, _ := json.Marshal(e)
			if string(after) != string(before) {
				t.Fatal("raw/value изменены")
			}
		})
	}
}

func TestValidateLinesContractNullableValues(t *testing.T) {
	for _, tc := range []struct {
		kind  model.ElementType
		raw   string
		value *string
		ids   []string
		bad   bool
	}{
		{"placeholder", "_____", nil, nil, false}, {"placeholder", "_____", nil, []string{"e1"}, true}, {"unparsed", "?", nil, []string{"e1"}, false},
		{"unparsed", "?", nil, nil, true}, {"unparsed", "?", validateLinesTestString("?"), []string{"e1"}, true},
		{"media-type", "unsupported", nil, []string{"e1"}, false}, {"media-type", "unsupported", validateLinesTestString("unsupported"), []string{"e1"}, true}, {"media-type", "unsupported", nil, nil, true},
		{"version", "9.future", validateLinesTestString("9.future"), []string{"e1"}, false},
	} {
		t.Run(string(tc.kind)+"/"+tc.raw+"/"+strings.Join(tc.ids, ","), func(t *testing.T) {
			e := validateLinesTestElement(tc.kind, tc.raw, tc.value, 1, len([]rune(tc.raw))+1)
			e.ErrorIDs = tc.ids
			validateLinesTestError(t, validateElementValue(e, validateLinesTestContext()), tc.bad)
		})
	}
	for _, kind := range []model.ElementType{"tag", "heading-level", "title", "content", "identifier", "name", "version", "number", "media-type", "source", "resource-path", "block-open", "block-close"} {
		t.Run("null/"+string(kind), func(t *testing.T) {
			validateLinesTestError(t, validateElementValue(validateLinesTestElement(kind, "", nil, 1, 1), validateLinesTestContext()), true)
		})
	}
}

func TestValidateLinesContractNumberUsesOrder(t *testing.T) {
	for i, raw := range []string{"1", "0007", strings.Repeat("0", 1000) + strings.Repeat("9", 5000), "", "0", "000", "+1", "-1", "1.0", "1e2", " 1", "1 ", "1\n", "１２", "١"} {
		t.Run(fmt.Sprintf("case-%02d", i), func(t *testing.T) {
			context := validateLinesTestContext()
			want := order.Validate(raw, context)
			got := validateElementValue(validateLinesTestElement("number", raw, &raw, 1, len([]rune(raw))+1), context)
			validateLinesTestError(t, got, want != nil)
			if want != nil && got != nil {
				if got.Code != want.Code {
					t.Errorf("код %q; нужен код зависимости %q", got.Code, want.Code)
				}
				if got.Context != context {
					t.Errorf("контекст order изменён: %#v", got.Context)
				}
				found := false
				for _, detail := range got.Details {
					if detail.Value == raw {
						found = true
					}
				}
				if !found {
					t.Errorf("исходный вес потерян: %#v", got.Details)
				}
			}
		})
	}
}

func TestValidateLinesContractBoundaries(t *testing.T) {
	for _, tc := range []struct {
		line     model.LineType
		required model.ElementType
		raw      string
	}{
		{"block-start", "block-open", "{"}, {"block-end", "block-close", "}"}, {"invalid", "unparsed", "?"},
	} {
		t.Run(string(tc.line), func(t *testing.T) {
			line := model.Line{Number: 23, LineType: tc.line, Raw: tc.raw}
			validateLinesTestError(t, validateBoundaryElements(line, validateLinesTestContext()), true)
			line.Elements = []model.Element{validateLinesTestElement("content", tc.raw, &tc.raw, 1, 2)}
			validateLinesTestError(t, validateBoundaryElements(line, validateLinesTestContext()), true)
			e := validateLinesTestElement(tc.required, tc.raw, &tc.raw, 1, 2)
			if tc.required == "unparsed" {
				e.Value = nil
				e.ErrorIDs = []string{"e1"}
			}
			line.Elements = append(line.Elements, e)
			validateLinesTestError(t, validateBoundaryElements(line, validateLinesTestContext()), false)
		})
	}
	for _, kind := range []model.LineType{"tag", "heading", "content", "separator", "blank"} {
		validateLinesTestError(t, validateBoundaryElements(model.Line{Number: 23, LineType: kind}, validateLinesTestContext()), false)
	}
}
