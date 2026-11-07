package parsed

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestParserErrorsIndependentBlockingMarkers(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Курс\Технический JSON.json`, LessonID: "Ab:7"}
	for _, tc := range []struct {
		name   string
		result model.Result
		line   int
		column int
	}{
		{"document flag without diagnostics", model.Result{Document: model.Document{HasErrors: true}}, 0, 0},
		{"error despite false flags", model.Result{Diagnostics: []model.Diagnostic{parserErrorsTestDiagnostic("only", model.SeverityError, 12, 4)}}, 12, 4},
		{"invalid despite false flags", model.Result{Lines: []model.Line{{Number: 17, LineType: model.LineTypeInvalid}}}, 17, 0},
		{"unparsed in valid line without diagnostics", model.Result{Lines: []model.Line{{Number: 23, LineType: model.LineTypeContent, Elements: []model.Element{{ElementType: model.ElementTypeUnparsed, Start: 6, End: 9}}}}}, 23, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parserErrors(&tc.result, context)
			if len(got) != 1 {
				t.Fatalf("получено %d ошибок, нужна одна самостоятельная блокировка", len(got))
			}
			want := context
			want.Line, want.Column = tc.line, tc.column
			parserErrorsTestIssue(t, got[0], want)
		})
	}
	quiet := model.Result{Lines: []model.Line{{Number: 1, HasErrors: true}}, Diagnostics: []model.Diagnostic{
		parserErrorsTestDiagnostic("warning-secret", model.SeverityWarning, 1, 2),
		parserErrorsTestDiagnostic("recommendation-secret", model.SeverityRecommendation, 1, 3),
	}}
	if got := parserErrors(&quiet, context); len(got) != 0 {
		t.Fatalf("warning, recommendation и line.hasErrors не самостоятельные признаки: %#v", got)
	}
}

func TestParserErrorsOrderAndMarkerAssociations(t *testing.T) {
	context := diagnostic.Context{Path: `D:\Вход\Урок.json`, LessonID: "MiXeD"}
	remote := parserErrorsTestDiagnostic("remote", model.SeverityError, 40, 4)
	local := parserErrorsTestDiagnostic("local", model.SeverityError, 91, 6)
	unrelated := parserErrorsTestDiagnostic("unrelated", model.SeverityError, 5, 8)
	span := parserErrorsTestDiagnostic("spanning", model.SeverityError, 80, 3)
	span.Location.End = model.Position{Line: 99, Column: 2}
	span.RelatedLocations = []model.Location{{Start: model.Position{Line: 91, Column: 2}, End: model.Position{Line: 91, Column: 3}}}
	warning := parserErrorsTestDiagnostic("warning-secret", model.SeverityWarning, 91, 10)
	recommendation := parserErrorsTestDiagnostic("recommendation-secret", model.SeverityRecommendation, 91, 11)
	dsl := `C:\Старый DSL\Не вход.txt`
	result := model.Result{
		Document:    model.Document{HasErrors: true, FilePath: &dsl},
		Diagnostics: []model.Diagnostic{remote, warning, local, recommendation, unrelated, span},
		Lines: []model.Line{
			{Number: 91, LineType: model.LineTypeInvalid, Elements: []model.Element{
				{ElementType: model.ElementTypeContent, Start: 1, End: 4, ErrorIDs: []string{"unrelated-id"}},
				{ElementType: model.ElementTypeUnparsed, Start: 7, End: 8, ErrorIDs: []string{"remote-id", "warning-secret-id", "missing-id"}},
				{ElementType: model.ElementTypeUnparsed, Start: 12, End: 13, ErrorIDs: []string{"recommendation-secret-id"}},
			}},
			{Number: 8, LineType: model.LineTypeInvalid},
		},
	}
	for range 5 {
		got := parserErrors(&result, context)
		wantPositions := [][2]int{{0, 0}, {40, 4}, {91, 6}, {5, 8}, {80, 3}, {91, 0}, {91, 7}, {91, 12}, {8, 0}}
		if len(got) != len(wantPositions) {
			t.Fatalf("число признаков = %d, нужно %d", len(got), len(wantPositions))
		}
		for i, position := range wantPositions {
			want := context
			want.Line, want.Column = position[0], position[1]
			parserErrorsTestIssue(t, got[i], want)
			parserErrorsTestAbsent(t, got[i], "warning-secret", "recommendation-secret", dsl)
		}
		for i, item := range []model.Diagnostic{remote, local, unrelated, span} {
			parserErrorsTestValues(t, got[i+1], string(item.DiagnosticCode), item.Message)
		}
		parserErrorsTestValues(t, got[0], "remote-code", "local-code", "unrelated-code", "spanning-code")
		parserErrorsTestValues(t, got[5], "remote-code", "remote-message", "local-code", "local-message", "unrelated-code", "unrelated-message")
		parserErrorsTestAbsent(t, got[5], "spanning-code", "missing-id")
		parserErrorsTestValues(t, got[6], "remote-code", "remote-message")
		parserErrorsTestAbsent(t, got[6], "local-code", "unrelated-code", "spanning-code", "missing-id")
		parserErrorsTestAbsent(t, got[7], "remote-code", "local-code", "unrelated-code", "spanning-code")
		if len(got[7].Related) != 0 || len(got[8].Related) != 0 {
			t.Error("признак без доступной error-диагностики получил выдуманные связанные места")
		}
	}
}

func TestParserDiagnosticPreservesMetadataAndRanges(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Курс\Технический.json`, FileName: "l001.json", LessonID: "Ab:7", PageID: "ab:7-p001"}
	item := parserErrorsTestDiagnostic("  код ё 🧩\t", model.SeverityError, 101, 203)
	item.Message = "  Unexpected token: ё 🧩\n\tсохранить точно  "
	item.Location.End = model.Position{Line: 607, Column: 809}
	item.RelatedLocations = []model.Location{
		{Start: model.Position{Line: 307, Column: 409}, End: model.Position{Line: 911, Column: 1009}},
		{Start: model.Position{Line: 419, Column: 521}, End: model.Position{Line: 1117, Column: 1223}},
	}
	for _, fatal := range []bool{false, true} {
		t.Run(fmt.Sprint(fatal), func(t *testing.T) {
			item.Fatal = fatal
			got := parserDiagnostic(item, context)
			want := context
			want.Line, want.Column = 101, 203
			parserErrorsTestIssue(t, got, want)
			parserErrorsTestValues(t, got, item.ID, item.Source, string(item.DiagnosticCode), item.Message, fmt.Sprint(fatal))
			if len(got.Related) != 2 {
				t.Fatalf("Related содержит %d мест, нужно 2", len(got.Related))
			}
			for i, location := range item.RelatedLocations {
				want.Line, want.Column = location.Start.Line, location.Start.Column
				if got.Related[i] != want {
					t.Errorf("Related[%d] = %#v, нужно %#v", i, got.Related[i], want)
				}
			}
			numbers := strings.FieldsFunc(parserErrorsTestDetails(got), func(r rune) bool { return r < '0' || r > '9' })
			for _, coordinate := range []string{"607", "809", "911", "1009", "1117", "1223"} {
				if !slices.Contains(numbers, coordinate) {
					t.Errorf("конец диапазона %s потерян в Details: %#v", coordinate, got.Details)
				}
			}
		})
	}
}

func TestParserDiagnosticFiltersSeverityAndKeepsUnknownCoordinates(t *testing.T) {
	context := diagnostic.Context{Path: "technical.json", LessonID: "Id"}
	for _, severity := range []model.Severity{model.SeverityWarning, model.SeverityRecommendation} {
		if got := parserDiagnostic(parserErrorsTestDiagnostic("secret", severity, 7, 9), context); got != nil {
			t.Errorf("%s преобразована в блокирующую ошибку: %#v", severity, got)
		}
	}
	item := parserErrorsTestDiagnostic("no-location", model.SeverityError, 1, 1)
	item.Location = nil
	got := parserDiagnostic(item, context)
	parserErrorsTestIssue(t, got, context)
	if len(got.Related) != 0 {
		t.Error("неизвестная location породила связанные координаты")
	}
	item.Message = ""
	parserErrorsTestValues(t, parserDiagnostic(item, context), "")
}

func TestParserMarkerFiltersNonErrorsAndPreservesKnownContext(t *testing.T) {
	for _, context := range []diagnostic.Context{{Path: "technical.json"}, {Path: "technical.json", Line: 12}, {Path: "technical.json", Line: 12, Column: 9}} {
		got := parserMarker("самостоятельный признак", context, []model.Diagnostic{
			parserErrorsTestDiagnostic("warning-secret", model.SeverityWarning, 1, 2),
			parserErrorsTestDiagnostic("recommendation-secret", model.SeverityRecommendation, 3, 4),
		})
		parserErrorsTestIssue(t, got, context)
		parserErrorsTestValues(t, got, "самостоятельный признак")
		parserErrorsTestAbsent(t, got, "warning-secret", "recommendation-secret")
		if len(got.Related) != 0 {
			t.Error("не-error диагностики попали в Related")
		}
	}
	item := parserErrorsTestDiagnostic("available", model.SeverityError, 0, 0)
	item.Location = nil
	got := parserMarker("признак без координат", diagnostic.Context{Path: "technical.json"}, []model.Diagnostic{item})
	parserErrorsTestIssue(t, got, diagnostic.Context{Path: "technical.json"})
	parserErrorsTestValues(t, got, "available-code", "available-message")
	for _, related := range got.Related {
		if related.Line != 0 || related.Column != 0 || related.Path != "technical.json" {
			t.Errorf("выдуманные координаты или путь: %#v", related)
		}
	}
}

func TestParserErrorsDoNotPrintParserWarnings(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = writer, writer
	defer func() { os.Stdout, os.Stderr = stdout, stderr; reader.Close(); writer.Close() }()
	warning := parserErrorsTestDiagnostic("warning-secret", model.SeverityWarning, 1, 2)
	recommendation := parserErrorsTestDiagnostic("recommendation-secret", model.SeverityRecommendation, 3, 4)
	result := model.Result{Document: model.Document{HasErrors: true}, Diagnostics: []model.Diagnostic{warning, recommendation}}
	parserErrors(&result, diagnostic.Context{Path: "technical.json"})
	parserDiagnostic(warning, diagnostic.Context{})
	parserMarker("признак", diagnostic.Context{}, result.Diagnostics)
	writer.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 0 {
		t.Errorf("parsed печатает пользовательские сообщения: %q", output)
	}
}

func parserErrorsTestDiagnostic(name string, severity model.Severity, line, column int) model.Diagnostic {
	return model.Diagnostic{ID: name + "-id", Source: name + "-source", DiagnosticCode: model.Code(name + "-code"), SeverityLevel: severity,
		Message: name + "-message", DiagnosticScope: model.ScopeElement,
		Location: &model.Location{Start: model.Position{Line: line, Column: column}, End: model.Position{Line: line, Column: column + 1}}}
}

func parserErrorsTestIssue(t *testing.T, issue *diagnostic.Error, want diagnostic.Context) {
	t.Helper()
	if issue == nil {
		t.Fatal("признак не вернул diagnostic.Error")
	}
	if issue.Code != CodeParser || issue.Context != want {
		t.Errorf("Code/Context = %q %#v, нужно %q %#v", issue.Code, issue.Context, CodeParser, want)
	}
	if !strings.ContainsFunc(issue.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Errorf("причина не объясняет блокировку по-русски: %q", issue.Reason)
	}
}

func parserErrorsTestDetails(issue *diagnostic.Error) string {
	var values []string
	for _, detail := range issue.Details {
		values = append(values, detail.Value)
	}
	return strings.Join(values, "\n")
}

func parserErrorsTestValues(t *testing.T, issue *diagnostic.Error, values ...string) {
	t.Helper()
	if issue == nil {
		t.Fatal("нет ошибки для проверки Details")
	}
	for _, value := range values {
		found := false
		for _, detail := range issue.Details {
			found = found || detail.Value == value
		}
		if !found {
			t.Errorf("точное значение %q отсутствует в Details: %#v", value, issue.Details)
		}
	}
}

func parserErrorsTestAbsent(t *testing.T, issue *diagnostic.Error, fragments ...string) {
	t.Helper()
	text := fmt.Sprintf("%s %s %#v %#v", issue.Code, issue.Reason, issue.Details, issue.Related)
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			t.Errorf("посторонняя диагностика/путь %q попала в ошибку: %s", fragment, text)
		}
	}
}
