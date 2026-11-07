package fragments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestTask6DiagnosticCodes(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{CodeShape, "fragments.shape"},
		{CodeMissing, "fragments.missing"},
		{CodeAmbiguous, "fragments.ambiguous"},
		{CodeCycle, "fragments.cycle"},
		{CodeParent, "fragments.parent"},
	} {
		if tc.got != tc.want {
			t.Errorf("код = %q, нужен %q", tc.got, tc.want)
		}
	}
}

func TestTask6SourceContextUsesPhysicalSource(t *testing.T) {
	input := task6LoadInput(t)
	line := input.Result.Lines[0]
	column := line.Elements[1].Start
	// Перед identifier находятся кириллица и символ вне BMP: колонка —
	// кодовые точки исходной строки, а не байтовая или UTF-16 позиция.
	if column != 14 || strings.Index(line.Raw, "MiXeD")+1 == column {
		t.Fatal("fixture не различает Unicode- и байтовые колонки")
	}
	for _, tc := range []struct {
		name   string
		column int
	}{
		{"известная Unicode-колонка", column},
		{"неизвестная колонка", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := diagnostic.Context{
				Path: input.Path, LessonID: "EfX7", Line: 41, Column: tc.column,
			}
			for occurrence := 0; occurrence < 2; occurrence++ {
				if got := sourceContext(input, line.Number, tc.column); got != want {
					t.Errorf("вхождение %d: Context = %#v, нужен %#v", occurrence, got, want)
				}
			}
		})
	}
}

func TestTask6FragmentErrorPreservesDiagnosticData(t *testing.T) {
	input := task6LoadInput(t)
	context := sourceContext(input, 41, 14)
	// Порядок намеренно отличается от порядка физических номеров.
	related := []diagnostic.Context{
		{Path: input.Path, LessonID: "EfX7", Line: 95, Column: 0},
		{Path: input.Path, LessonID: "EfX7", Line: 12, Column: 7},
		{Path: input.Path, LessonID: "EfX7", Line: 41, Column: 14},
	}
	for _, tc := range []struct {
		code   string
		reason string
	}{
		{CodeShape, "Форма определения не позволяет развёртывание."},
		{CodeMissing, "Определение фрагмента отсутствует."},
		{CodeAmbiguous, "Имя соответствует нескольким определениям."},
		{CodeCycle, "Обнаружен цикл включений фрагментов."},
		{CodeParent, "Исходная связь родителя не разрешается."},
	} {
		t.Run(tc.code, func(t *testing.T) {
			for _, identifier := range []string{"MiXeD", " Ёe\u0301\\Имя ", "0007"} {
				issue := fragmentError(tc.code, tc.reason, identifier, context, related)
				if issue == nil {
					t.Fatal("ошибка не создана")
				}
				if issue.Code != tc.code || issue.Reason != tc.reason {
					t.Errorf("Code/Reason = %q/%q, нужны %q/%q", issue.Code, issue.Reason, tc.code, tc.reason)
				}
				if issue.Context != context || !slices.Equal(issue.Related, related) {
					t.Errorf("исходные места или порядок Related изменены: %#v / %#v", issue.Context, issue.Related)
				}
				found := false
				for _, detail := range issue.Details {
					if detail.Value == identifier {
						found = true
						if !strings.ContainsFunc(detail.Label, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
							t.Errorf("подробность имени должна иметь русское название: %q", detail.Label)
						}
					}
					for _, item := range input.Result.Diagnostics {
						if strings.Contains(detail.Value, item.Message) || strings.Contains(detail.Value, string(item.DiagnosticCode)) {
							t.Errorf("скопирована неблокирующая диагностика: %#v", detail)
						}
					}
				}
				if !found {
					t.Errorf("Details не содержит точное исходное имя %q: %#v", identifier, issue.Details)
				}
				if issue.Cause != nil {
					t.Errorf("выдумана техническая причина: %v", issue.Cause)
				}
			}
		})
	}
}

func TestTask6FragmentErrorWithoutRelatedLocations(t *testing.T) {
	context := diagnostic.Context{Path: "/вход/lesson.json", LessonID: "EfX7", Line: 41}
	issue := fragmentError(CodeShape, "Не удалось определить имя фрагмента.", "", context, nil)
	if issue == nil {
		t.Fatal("ошибка не создана")
	}
	if issue.Context != context || len(issue.Related) != 0 {
		t.Errorf("неизвестные места выдуманы: %#v / %#v", issue.Context, issue.Related)
	}
}

func task6LoadInput(t *testing.T) *parsed.Input {
	t.Helper()
	id, fileName, sourcePath := "EfX7", "lesson.txt", `Z:\Исходный DSL\lesson.txt`
	tag, name := "include", "MiXeD"
	result := model.Result{
		Format: model.FormatVersion1,
		Document: model.Document{
			FileName: &fileName, FilePath: &sourcePath,
			Metadata: model.DocumentMetadata{DocumentID: &id, ResourceDirs: []string{}},
		},
		Processing: []model.Processing{{ID: "parser-run", Tool: "dsl-parser"}},
		Lines: []model.Line{{
			Number: 41, LineType: model.LineTypeTag, Raw: "\tЖ😀 @include MiXeD",
			Elements: []model.Element{
				{ElementType: model.ElementTypeTag, Raw: "@include", Value: &tag, Start: 5, End: 13, ErrorIDs: []string{}},
				{ElementType: model.ElementTypeIdentifier, Raw: name, Value: &name, Start: 14, End: 19, ErrorIDs: []string{}},
			},
		}},
		Diagnostics: []model.Diagnostic{
			{ID: "w1", Source: "parser-run", DiagnosticCode: "parser.warning.task6", SeverityLevel: model.SeverityWarning,
				Message: "Предупреждение парсера task6", DiagnosticScope: model.ScopeDocument, RelatedLocations: []model.Location{}},
			{ID: "r1", Source: "parser-run", DiagnosticCode: "parser.recommendation.task6", SeverityLevel: model.SeverityRecommendation,
				Message: "Рекомендация парсера task6", DiagnosticScope: model.ScopeDocument, RelatedLocations: []model.Location{}},
		},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Технический JSON.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	input, issues := parsed.Load(path)
	if input == nil || len(issues) != 0 {
		t.Fatalf("реальный parsed.Load отклонил fixture: %v", issues)
	}
	return input
}
