package parsed_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/order"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func inputTestPtr[T any](value T) *T { return &value }

func inputTestResult() model.Result {
	return model.Result{Format: model.FormatVersion1,
		Document:   model.Document{Metadata: model.DocumentMetadata{DocumentID: inputTestPtr("MiXeD:A+1")}},
		Processing: []model.Processing{}, Lines: []model.Line{}, Diagnostics: []model.Diagnostic{}}
}

func inputTestJSON(t *testing.T, result model.Result) []byte {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func inputTestLoad(t *testing.T, content []byte) (*parsed.Input, []*diagnostic.Error, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Технический JSON с пробелом.json")
	if err := os.WriteFile(path, content, 0400); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, diagnostics := parsed.Load(path)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, content) || info.Mode() != before.Mode() || !info.ModTime().Equal(before.ModTime()) || len(entries) != 1 {
		t.Fatal("Load изменил исходник или состав каталога")
	}
	return got, diagnostics, path
}

func inputTestErrors(t *testing.T, got *parsed.Input, diagnostics []*diagnostic.Error, path, id, code string) {
	t.Helper()
	if got != nil || len(diagnostics) == 0 {
		t.Fatalf("ошибка должна вернуть nil Input и диагностику: Input=%v, diagnostics=%v", got, diagnostics)
	}
	found := false
	for _, item := range diagnostics {
		if item == nil {
			t.Fatal("nil внутри диагностик")
		}
		if item.Code == code {
			found = true
		}
		if item.Context.Path != path || item.Context.LessonID != id {
			t.Errorf("контекст: %+v, нужен path=%q ID=%q", item.Context, path, id)
		}
		if item.Code == "" || !strings.ContainsFunc(item.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
			t.Errorf("код/русская причина: %+v", item)
		}
	}
	if !found {
		t.Errorf("нет кода %q: %v", code, diagnostics)
	}
}

func TestInputLoadReadErrors(t *testing.T) {
	for _, name := range []string{"missing", "directory"} {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir()
			if name == "missing" {
				path = filepath.Join(path, "нет.json")
			}
			got, diagnostics := parsed.Load(path)
			inputTestErrors(t, got, diagnostics, path, "", parsed.CodeRead)
			if diagnostics[0].Cause == nil {
				t.Error("не сохранена причина чтения")
			}
			if name == "missing" && !errors.Is(diagnostics[0], os.ErrNotExist) {
				t.Error("утрачена цепочка os.ErrNotExist")
			}
		})
	}
}

func TestInputLoadRejectsBeforeModelValidation(t *testing.T) {
	base := string(inputTestJSON(t, inputTestResult()))
	for _, tc := range []struct{ name, content, code string }{
		{"invalid UTF8 before JSON", string([]byte{0xff, '{'}), parsed.CodeUTF8},
		{"invalid UTF8 inside unknown field", strings.Replace(base, `"formatVersion"`, `"unknown":"`+string([]byte{0xff})+`","formatVersion"`, 1), parsed.CodeUTF8},
		{"empty", "", parsed.CodeJSON}, {"syntax", "{", parsed.CodeJSON},
		{"second value", base + " {}", parsed.CodeJSON}, {"JSONC", "/* comment */" + base, parsed.CodeJSON},
		{"second BOM", "\ufeff\ufeff" + base, parsed.CodeJSON}, {"BOM after whitespace", " \ufeff" + base, parsed.CodeJSON},
		{"root null", "null", parsed.CodeContract}, {"root array", "[]", parsed.CodeContract},
		{"unsupported version", strings.Replace(base, `"1.0"`, `"1.1"`, 1), parsed.CodeVersion},
		{"numeric version", strings.Replace(base, `"1.0"`, `1`, 1), parsed.CodeContract},
		{"missing version", strings.Replace(base, `"formatVersion":"1.0",`, "", 1), parsed.CodeContract},
		{"exact field spelling", strings.Replace(base, `"document"`, `"Document"`, 1), parsed.CodeContract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, diagnostics, path := inputTestLoad(t, []byte(tc.content))
			inputTestErrors(t, got, diagnostics, path, "", tc.code)
			for _, item := range diagnostics {
				if item.Context.Line != 0 || item.Context.Column != 0 {
					t.Errorf("JSON-координаты выданы за DSL: %+v", item.Context)
				}
			}
		})
	}
}

func TestInputLoadRejectsShapeMetadataContractAndParents(t *testing.T) {
	for _, tc := range []struct {
		name, code, id string
		change         func(*model.Result)
	}{
		{"null ID", parsed.CodeMissingID, "", func(r *model.Result) { r.Document.Metadata.DocumentID = nil }},
		{"empty ID", identity.CodeInvalidLessonID, "", func(r *model.Result) { r.Document.Metadata.DocumentID = inputTestPtr("") }},
		{"invalid ID", identity.CodeInvalidLessonID, "Bad ID", func(r *model.Result) { r.Document.Metadata.DocumentID = inputTestPtr("Bad ID") }},
		{"invalid weight", order.CodeInvalidWeight, "MiXeD:A+1", func(r *model.Result) { r.Document.Metadata.Order = inputTestPtr("000") }},
		{"null lines", parsed.CodeContract, "MiXeD:A+1", func(r *model.Result) { r.Lines = nil }},
		{"negative source size", parsed.CodeContract, "MiXeD:A+1", func(r *model.Result) { r.Document.ByteLength = inputTestPtr(int64(-1)) }},
		{"line count mismatch", parsed.CodeContract, "MiXeD:A+1", func(r *model.Result) { r.Document.LineCount = inputTestPtr(1) }},
		{"missing parent", parsed.CodeParent, "MiXeD:A+1", func(r *model.Result) {
			r.Lines = []model.Line{{Number: 1, LineType: model.LineTypeBlank, ParentLine: inputTestPtr(9), NestingLevel: 1, Elements: []model.Element{}}}
		}},
		{"parent cycle", parsed.CodeParentCycle, "MiXeD:A+1", func(r *model.Result) {
			r.Lines = []model.Line{{Number: 1, LineType: model.LineTypeBlank, ParentLine: inputTestPtr(1), NestingLevel: 1, Elements: []model.Element{}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := inputTestResult()
			tc.change(&result)
			got, diagnostics, path := inputTestLoad(t, inputTestJSON(t, result))
			inputTestErrors(t, got, diagnostics, path, tc.id, tc.code)
		})
	}
}

func TestInputLoadParserMarkersPrecedeConsistency(t *testing.T) {
	for _, name := range []string{"document.hasErrors", "diagnostic severity", "invalid line", "unparsed element"} {
		t.Run(name, func(t *testing.T) {
			result := inputTestResult()
			result.Document.Metadata.Order = inputTestPtr("0")
			result.Lines = []model.Line{{Number: 1, LineType: model.LineTypeContent, Raw: "x", ParentLine: inputTestPtr(99), NestingLevel: 1, Elements: []model.Element{}}}
			switch name {
			case "document.hasErrors":
				result.Document.HasErrors = true
			case "diagnostic severity":
				result.Diagnostics = []model.Diagnostic{{ID: "err", Source: "missing-run", DiagnosticCode: "E_original", Message: "Точный текст ошибки", SeverityLevel: model.SeverityError, DiagnosticScope: model.ScopeLine, Location: &model.Location{Start: model.Position{Line: 7, Column: 3}, End: model.Position{Line: 7, Column: 5}}, RelatedLocations: []model.Location{}}}
			case "invalid line":
				result.Lines[0].LineType = model.LineTypeInvalid
			case "unparsed element":
				result.Lines[0].Elements = []model.Element{{ElementType: model.ElementTypeUnparsed, Raw: "x", Start: 1, End: 2, ErrorIDs: []string{"missing-error"}}}
			}
			got, diagnostics, path := inputTestLoad(t, inputTestJSON(t, result))
			inputTestErrors(t, got, diagnostics, path, "MiXeD:A+1", parsed.CodeParser)
			for _, item := range diagnostics {
				if item.Code != parsed.CodeParser {
					t.Errorf("проверка согласованности выполнена до маркеров: %v", item)
				}
			}
			if name == "diagnostic severity" {
				if diagnostics[0].Context.Line != 7 || diagnostics[0].Context.Column != 3 {
					t.Error("утрачены исходные координаты ошибки парсера")
				}
				for _, want := range []string{"E_original", "Точный текст ошибки"} {
					found := false
					for _, detail := range diagnostics[0].Details {
						if detail.Value == want {
							found = true
						}
					}
					if !found {
						t.Errorf("не сохранена подробность %q", want)
					}
				}
			}
		})
	}
}

func TestInputLoadPreservesModelAndBuildsIndex(t *testing.T) {
	for _, bom := range []bool{false, true} {
		t.Run(map[bool]string{false: "UTF8", true: "BOM"}[bom], func(t *testing.T) {
			result := inputTestResult()
			result.Document.FileName = inputTestPtr("старый.txt")
			result.Document.FilePath = inputTestPtr(`C:\несуществующий\старый.txt`)
			result.Document.Metadata.Title = inputTestPtr("\ufeffНазвание  ")
			result.Document.Metadata.Order = inputTestPtr("0001234567890123456789012345678901234567890")
			result.Document.Metadata.ResourceDirs = []string{`..\нет каталога`, `C:\нет ресурсов`}
			result.Document.LineCount = inputTestPtr(3)
			result.Lines = []model.Line{
				{Number: 1, LineType: model.LineTypeContent, Raw: " \tЯ😀\\n ", LineEnding: model.LineEndingCRLF, Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: "Я😀\\n", Value: inputTestPtr("Семантика\\n\ufeff"), Start: 3, End: 7, ErrorIDs: []string{}}}},
				{Number: 2, LineType: model.LineTypeContent, ParentLine: inputTestPtr(1), NestingLevel: 1, Raw: "_____", LineEnding: model.LineEndingLF, Elements: []model.Element{{ElementType: model.ElementTypePlaceholder, Raw: "_____", Start: 1, End: 6, ErrorIDs: []string{}}}},
				{Number: 3, LineType: model.LineTypeBlank, ParentLine: inputTestPtr(1), NestingLevel: 1, Raw: "\t ", Elements: []model.Element{}},
			}
			result.Processing = []model.Processing{{ID: "run", Tool: "other-tool"}}
			for _, severity := range []model.Severity{model.SeverityWarning, model.SeverityRecommendation} {
				result.Diagnostics = append(result.Diagnostics, model.Diagnostic{ID: string(severity), Source: "run", DiagnosticCode: "open-code", SeverityLevel: severity, Message: "Скрытый текст " + string(severity), DiagnosticScope: model.ScopeDocument, RelatedLocations: []model.Location{}})
			}
			content := inputTestJSON(t, result)
			if bom {
				content = append([]byte{0xef, 0xbb, 0xbf}, content...)
			}
			content = append(content, '\n', '\t', ' ')
			got, diagnostics, path := inputTestLoad(t, content)
			if got == nil || diagnostics != nil {
				t.Fatalf("успех: Input=%v diagnostics=%v", got, diagnostics)
			}
			if got.Path != path {
				t.Errorf("Path=%q, нужен %q", got.Path, path)
			}
			if !bytes.Equal(inputTestJSON(t, got.Result), inputTestJSON(t, result)) {
				t.Error("изменена исходная модель: null, raw, value, EOL или порядок")
			}
			if len(got.Index.ByNumber) != 3 || got.Index.ByNumber[1] != 0 || got.Index.ByNumber[2] != 1 || got.Index.ByNumber[3] != 2 {
				t.Errorf("ByNumber=%v", got.Index.ByNumber)
			}
			if len(got.Index.Children) != 2 || len(got.Index.Children[0]) != 1 || got.Index.Children[0][0] != 1 || len(got.Index.Children[1]) != 2 || got.Index.Children[1][0] != 2 || got.Index.Children[1][1] != 3 {
				t.Errorf("Children=%v", got.Index.Children)
			}
		})
	}
}

func TestInputLoadNullableMetadataAndUnknownFields(t *testing.T) {
	for _, emptyDirs := range []bool{false, true} {
		result := inputTestResult()
		if emptyDirs {
			result.Document.Metadata.ResourceDirs = []string{}
		}
		encoded := inputTestJSON(t, result)
		encoded = []byte(strings.ReplaceAll(string(encoded), "{", `{"future":{"ignored":true},`))
		got, diagnostics, _ := inputTestLoad(t, encoded)
		if got == nil || diagnostics != nil {
			t.Fatalf("не принят корректный вход: %v", diagnostics)
		}
		if !bytes.Equal(inputTestJSON(t, got.Result), inputTestJSON(t, result)) {
			t.Error("null заменён выдуманными значениями или []")
		}
		if len(got.Index.ByNumber) != 0 || len(got.Index.Children) != 0 {
			t.Error("создана фиктивная строка пустого документа")
		}
	}
}

func TestInputLoadDoesNotPrint(t *testing.T) {
	stdout, stderr := os.Stdout, os.Stderr
	capture, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	os.Stdout, os.Stderr = capture, capture
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	valid := inputTestJSON(t, inputTestResult())
	for _, content := range [][]byte{valid, []byte("{"), []byte(strings.Replace(string(valid), `"hasErrors":false`, `"hasErrors":true`, 1))} {
		inputTestLoad(t, content)
	}
	if _, err := capture.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	info, err := capture.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("Load записал %d байтов в stdout/stderr", info.Size())
	}
}
