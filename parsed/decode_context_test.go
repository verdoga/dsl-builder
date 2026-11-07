package parsed_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestDecodeContextLateTypedErrorsPreserveDocumentID(t *testing.T) {
	valid, err := json.Marshal(decodeContextTestResult())
	if err != nil {
		t.Fatal(err)
	}
	path := decodeContextTestWrite(t, valid)
	input, problems := parsed.Load(path)
	if input == nil || problems != nil {
		t.Fatalf("полный исходный пример должен загружаться: input=%v, errors=%v", input, problems)
	}
	if input.Result.Document.Metadata.DocumentID == nil || *input.Result.Document.Metadata.DocumentID != "MiXeD:A+1" {
		t.Fatal("исходный пример потерял точный documentId")
	}

	for _, tc := range []struct {
		name   string
		before string
		after  string
		field  string
		line   int
		column int
	}{
		{
			name:   "processing id number",
			before: `"id":"p"`, after: `"id":7`,
			field: "processing[0].id",
		},
		{
			name:   "line type number after line 27",
			before: `"line":27,"type":"content"`, after: `"line":27,"type":3`,
			field: "lines[26].type", line: 27,
		},
		{
			name:   "null related locations after location 9 3",
			before: `"relatedLocations":[]`, after: `"relatedLocations":null`,
			field: "diagnostics[0].relatedLocations", line: 9, column: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Count(string(valid), tc.before) != 1 {
				t.Fatalf("неоднозначная замена тестового поля %s", tc.field)
			}
			broken := []byte(strings.Replace(string(valid), tc.before, tc.after, 1))
			path := decodeContextTestWrite(t, broken)
			input, problems := parsed.Load(path)
			if input != nil {
				t.Error("ошибка формы вернула частично пригодный Input")
			}
			problem := decodeContextTestProblem(t, problems, tc.field)
			want := diagnostic.Context{Path: path, LessonID: "MiXeD:A+1", Line: tc.line, Column: tc.column}
			if problem.Context != want {
				t.Errorf("контекст поздней ошибки = %+v, нужен %+v", problem.Context, want)
			}
		})
	}
}

func TestDecodeContextNullLinesRejectsWithoutPartialInput(t *testing.T) {
	result := decodeContextTestResult()
	result.Lines = nil
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := decodeContextTestWrite(t, data)
	input, problems := parsed.Load(path)
	if input != nil {
		t.Error("lines=null вернул частично пригодный Input")
	}
	problem := decodeContextTestProblem(t, problems, "lines")
	if problem.Context.Path != path || problem.Context.Line != 0 || problem.Context.Column != 0 {
		t.Errorf("контекст lines=null = %+v, нужен путь %q без выдуманных координат", problem.Context, path)
	}
	// Ранняя проверка контейнеров допустима до чтения metadata. Сохранение
	// уже прочитанного ID независимо проверяют поздние ошибки записей выше.
	if id := problem.Context.LessonID; id != "" && id != "MiXeD:A+1" {
		t.Errorf("lines=null вернул выдуманный или нормализованный ID %q", id)
	}
}

// decodeContextTestResult создаёт полный корректный вход с известными ID и позициями.
func decodeContextTestResult() model.Result {
	id := "MiXeD:A+1"
	result := model.Result{
		Format:     model.FormatVersion1,
		Document:   model.Document{Metadata: model.DocumentMetadata{DocumentID: &id}},
		Processing: []model.Processing{{ID: "p", Tool: "context-regression"}},
		Lines:      make([]model.Line, 27),
		Diagnostics: []model.Diagnostic{{
			ID: "d", Source: "p", DiagnosticCode: "sample.warning",
			SeverityLevel: model.SeverityWarning, Message: "Скрытое предупреждение",
			DiagnosticScope: model.ScopeLine,
			Location: &model.Location{
				Start: model.Position{Line: 9, Column: 3},
				End:   model.Position{Line: 9, Column: 5},
			},
			RelatedLocations: []model.Location{},
		}},
	}
	for i := range result.Lines {
		result.Lines[i] = model.Line{
			Number: i + 1, LineType: model.LineTypeContent, Raw: "abcde",
			LineEnding: model.LineEndingLF, Elements: []model.Element{},
		}
	}
	result.Lines[len(result.Lines)-1].LineEnding = model.LineEndingNone
	return result
}

// decodeContextTestWrite сохраняет отдельный вход в принадлежащем тесту каталоге.
func decodeContextTestWrite(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Технический вход.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// decodeContextTestProblem находит ошибку формы назначенного JSON-поля.
func decodeContextTestProblem(t *testing.T, problems []*diagnostic.Error, field string) *diagnostic.Error {
	t.Helper()
	for _, problem := range problems {
		if problem == nil || problem.Code != parsed.CodeContract {
			continue
		}
		for _, detail := range problem.Details {
			if detail.Value == field {
				return problem
			}
		}
	}
	t.Fatalf("нет ошибки %s для JSON-поля %s: %v", parsed.CodeContract, field, problems)
	return nil
}
