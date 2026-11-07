package parsed_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/order"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func integrationTestRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func integrationTestWrite(t *testing.T, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "технический JSON.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func integrationTestJSON(t *testing.T, result model.Result) []byte {
	t.Helper()
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func integrationTestLoad(t *testing.T, path string) (*parsed.Input, []*diagnostic.Error) {
	t.Helper()
	before := integrationTestRead(t, path)
	capture, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	stdout, stderr := os.Stdout, os.Stderr
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	os.Stdout, os.Stderr = capture, capture
	input, problems := parsed.Load(path)
	os.Stdout, os.Stderr = stdout, stderr
	if output := integrationTestRead(t, capture.Name()); len(output) != 0 {
		t.Errorf("Load printed %q", output)
	}
	if after := integrationTestRead(t, path); !bytes.Equal(before, after) {
		t.Error("Load changed source bytes")
	}
	return input, problems
}

func integrationTestSuccess(t *testing.T, path string, expected model.Result) {
	t.Helper()
	input, problems := integrationTestLoad(t, path)
	if input == nil || problems != nil {
		t.Fatalf("Load = %v, %v", input, problems)
	}
	if input.Path != path {
		t.Errorf("Path = %q, want technical JSON %q", input.Path, path)
	}
	if !reflect.DeepEqual(input.Result, expected) {
		t.Error("Result changed original values, nulls, arrays, raw, EOL or coordinates")
	}
	numbers, children := make(map[int]int), make(map[int][]int)
	for i, line := range expected.Lines {
		numbers[line.Number] = i
		parent := 0
		if line.ParentLine != nil {
			parent = *line.ParentLine
		}
		children[parent] = append(children[parent], line.Number)
	}
	if !reflect.DeepEqual(input.Index.ByNumber, numbers) {
		t.Errorf("ByNumber = %v, want %v", input.Index.ByNumber, numbers)
	}
	if !reflect.DeepEqual(input.Index.Children, children) {
		t.Errorf("Children = %v, want %v", input.Index.Children, children)
	}
}

func integrationTestFailure(t *testing.T, b []byte, code, id string) []*diagnostic.Error {
	t.Helper()
	path := integrationTestWrite(t, b)
	input, problems := integrationTestLoad(t, path)
	if input != nil || len(problems) == 0 {
		t.Fatalf("expected no Input and diagnostics, got %v, %v", input, problems)
	}
	found := false
	for _, problem := range problems {
		if problem == nil {
			t.Fatal("nil diagnostic")
		}
		if problem.Context.Path != path {
			t.Errorf("diagnostic path = %q, want %q", problem.Context.Path, path)
		}
		if id != "" && problem.Context.LessonID != id {
			t.Errorf("diagnostic lesson ID = %q, want %q", problem.Context.LessonID, id)
		}
		if problem.Reason == "" {
			t.Error("empty reason")
		}
		found = found || problem.Code == code
	}
	if !found {
		t.Errorf("missing diagnostic code %q: %v", code, problems)
	}
	return problems
}

func integrationTestFixture(t *testing.T) model.Result {
	t.Helper()
	var result model.Result
	const fixture = `{"formatVersion":"1.0","document":{"dslVersion":"future","fileName":"несуществующий.txt","filePath":"Z:\\нет такого каталога\\несуществующий.txt","encoding":"UTF-8","hasBom":false,"lineCount":6,"byteLength":null,"sha256":null,"metadata":{"documentId":"MiXeD:урок+1","title":null,"subtitle":null,"section":null,"order":"0001234567890123456789012345678901234567890","resourceDirs":["missing/../ Audio","Z:\\missing","missing/../ Audio"]},"hasErrors":false},"processing":[{"id":"p","tool":"arbitrary-tool","version":null,"startedAt":null,"durationMs":null}],"lines":[{"line":1,"type":"block-start","nestingLevel":0,"parentLine":null,"raw":"{","eol":"\r\n","hasErrors":false,"elements":[{"type":"block-open","raw":"{","value":"{","start":1,"end":2,"errorIds":[]}]},{"line":2,"type":"content","nestingLevel":1,"parentLine":1,"raw":"  Я😀\\x  ","eol":"\n","hasErrors":false,"elements":[{"type":"content","raw":"Я😀\\x","value":"Я😀\\x","start":3,"end":7,"errorIds":[]}]},{"line":3,"type":"blank","nestingLevel":1,"parentLine":1,"raw":"\t  ","eol":"\r\n","hasErrors":false,"elements":[]},{"line":4,"type":"block-end","nestingLevel":1,"parentLine":1,"raw":"}","eol":"\n","hasErrors":false,"elements":[{"type":"block-close","raw":"}","value":"}","start":1,"end":2,"errorIds":[]}]},{"line":5,"type":"tag","nestingLevel":0,"parentLine":null,"raw":"\"café clip\"","eol":"\n","hasErrors":false,"elements":[{"type":"source","raw":"\"café clip\"","value":"café clip","start":1,"end":12,"errorIds":[]}]},{"line":6,"type":"content","nestingLevel":0,"parentLine":null,"raw":"_____","eol":"","hasErrors":false,"elements":[{"type":"placeholder","raw":"_____","value":null,"start":1,"end":6,"errorIds":[]}]}],"diagnostics":[{"id":"w","source":"p","code":"open.warning","severity":"warning","message":"HIDDEN_WARNING","scope":"document","fatal":false,"location":null,"relatedLocations":[]},{"id":"r","source":"p","code":"open.recommendation","severity":"recommendation","message":"HIDDEN_RECOMMENDATION","scope":"document","fatal":false,"location":null,"relatedLocations":[]}]}`
	if err := json.Unmarshal([]byte(fixture), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestIntegrationGoldenAndUnitJSON(t *testing.T) {
	for n := 1; n <= 7; n++ {
		path, err := filepath.Abs(filepath.Join("..", "golden", fmt.Sprintf("%d. parsed.json", n)))
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprintf("golden%d", n), func(t *testing.T) {
			var expected model.Result
			if err := json.Unmarshal(integrationTestRead(t, path), &expected); err != nil {
				t.Fatal(err)
			}
			integrationTestSuccess(t, path, expected)
		})
	}
	archive, err := zip.OpenReader(filepath.Join("..", "Unit example.ZIP"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	count := 0
	for _, file := range archive.File {
		if !strings.HasSuffix(file.Name, ".json") {
			continue
		}
		count++
		t.Run(file.Name, func(t *testing.T) {
			reader, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			var expected model.Result
			if err := json.Unmarshal(b, &expected); err != nil {
				t.Fatal(err)
			}
			integrationTestSuccess(t, integrationTestWrite(t, b), expected)
		})
	}
	if count != 11 {
		t.Errorf("Unit JSON count = %d, want 11", count)
	}
}

func TestIntegrationPreservesTechnicalInput(t *testing.T) {
	result := integrationTestFixture(t)
	semantic := "\ufeffЯ😀\\x"
	result.Lines[1].Elements[0].Value = &semantic
	b := integrationTestJSON(t, result)
	// Неизвестные ключи на каждом уровне не меняют известную модель.
	b = bytes.ReplaceAll(b, []byte(`:{`), []byte(`:{"future":true,`))
	b = bytes.ReplaceAll(b, []byte(`[{`), []byte(`[{"future":true,`))
	b = bytes.Replace(b, []byte(`{"formatVersion"`), []byte(`{"future":true,"formatVersion"`), 1)
	for _, prefix := range []string{"", "\xef\xbb\xbf"} {
		t.Run(fmt.Sprintf("BOM=%t", prefix != ""), func(t *testing.T) {
			integrationTestSuccess(t, integrationTestWrite(t, append([]byte(prefix), b...)), result)
		})
	}
	result.Document.LineCount = nil
	result.Document.Metadata.Order = nil
	result.Document.Metadata.ResourceDirs = nil
	integrationTestSuccess(t, integrationTestWrite(t, integrationTestJSON(t, result)), result)
	result.Document.Metadata.ResourceDirs = []string{}
	integrationTestSuccess(t, integrationTestWrite(t, integrationTestJSON(t, result)), result)
	// Повторный ID и вес в другом файле не относятся к проверкам одного Load.
	integrationTestSuccess(t, integrationTestWrite(t, integrationTestJSON(t, result)), result)
}

func TestIntegrationParserMarkersPrecedeInvariants(t *testing.T) {
	for _, marker := range []string{"document", "diagnostic", "invalid", "unparsed"} {
		t.Run(marker, func(t *testing.T) {
			result := integrationTestFixture(t)
			switch marker {
			case "document":
				result.Document.HasErrors = true
			case "diagnostic":
				result.Diagnostics[0].SeverityLevel = model.SeverityError
				result.Diagnostics[0].Message = "EXACT_PARSER_MESSAGE"
				result.Diagnostics[0].Location = &model.Location{Start: model.Position{Line: 2, Column: 3}, End: model.Position{Line: 2, Column: 7}}
			case "invalid":
				result.Lines[1].LineType = model.LineTypeInvalid
			case "unparsed":
				result.Lines[1].Elements[0].ElementType = model.ElementTypeUnparsed
				result.Lines[1].Elements[0].Value = nil
			}
			// Дополнительная сломанная ссылка не должна скрывать parser marker.
			missing := 999
			result.Lines[2].ParentLine = &missing
			problems := integrationTestFailure(t, integrationTestJSON(t, result), parsed.CodeParser, *result.Document.Metadata.DocumentID)
			if problems[0].Code != parsed.CodeParser {
				t.Errorf("parser marker not first: %v", problems)
			}
			for _, problem := range problems {
				if strings.Contains(problem.Reason+fmt.Sprint(problem.Details), "HIDDEN_") {
					t.Errorf("warning/recommendation leaked: %+v", problem)
				}
			}
			if marker == "diagnostic" {
				found := false
				for _, problem := range problems {
					if problem.Context.Line == 2 && problem.Context.Column == 3 {
						for _, detail := range problem.Details {
							found = found || detail.Value == "EXACT_PARSER_MESSAGE"
						}
					}
				}
				if !found {
					t.Error("parser message or original DSL coordinates lost")
				}
			}
		})
	}
}

func TestIntegrationFailuresAcrossStages(t *testing.T) {
	result := integrationTestFixture(t)
	b := integrationTestJSON(t, result)
	for _, test := range []struct {
		name string
		b    []byte
		code string
	}{
		{"UTF8-before-JSON", append([]byte{0xff}, b...), parsed.CodeUTF8},
		{"double-BOM", append([]byte("\xef\xbb\xbf\xef\xbb\xbf"), b...), parsed.CodeJSON},
		{"second-value", append(append([]byte{}, b...), []byte(" {}")...), parsed.CodeJSON},
		{"required-array-null", bytes.Replace(b, []byte(`"processing":[`), []byte(`"processing":null,"ignored":[`), 1), parsed.CodeContract},
		{"missing-documentId", bytes.Replace(b, []byte(`"documentId":`), []byte(`"DocumentId":`), 1), parsed.CodeMissingID},
	} {
		t.Run(test.name, func(t *testing.T) { integrationTestFailure(t, test.b, test.code, "") })
	}
	for _, mode := range []string{"identity", "order", "parent", "cycle"} {
		t.Run(mode, func(t *testing.T) {
			result := integrationTestFixture(t)
			code := parsed.CodeParent
			switch mode {
			case "identity":
				bad := "bad ID"
				result.Document.Metadata.DocumentID = &bad
				code = identity.CodeInvalidLessonID
			case "order":
				bad := "000"
				result.Document.Metadata.Order = &bad
				code = order.CodeInvalidWeight
			case "parent":
				missing := 999
				result.Lines[1].ParentLine = &missing
			case "cycle":
				a, b := 3, 2
				result.Lines[1].ParentLine = &a
				result.Lines[2].ParentLine = &b
				code = parsed.CodeParentCycle
			}
			problems := integrationTestFailure(t, integrationTestJSON(t, result), code, *result.Document.Metadata.DocumentID)
			if mode == "cycle" {
				found := map[int]bool{}
				for _, problem := range problems {
					if problem.Code == code {
						for _, related := range problem.Related {
							found[related.Line] = true
						}
					}
				}
				if !found[2] || !found[3] {
					t.Errorf("cycle participants missing: %v", found)
				}
			}
		})
	}
}
