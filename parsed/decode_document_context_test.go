package parsed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
)

const decodeDocumentContextTestMetadata = `{"documentId":"MiXeD:A+1","title":null,"subtitle":null,"section":null,"order":null,"resourceDirs":null}`

func TestDecodeDocumentContextMetadataErrorsKeepReadID(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Вход\Технический.json`, FileName: "l017.json", PageID: "page", Line: 27, Column: 4}
	for _, test := range []struct {
		name, old, replacement, field string
	}{
		{"title", `"title":null`, `"title":17`, "document.metadata.title"},
		{"resourceDirs-element", `"resourceDirs":null`, `"resourceDirs":[null]`, "document.metadata.resourceDirs[0]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := json.RawMessage(strings.Replace(decodeDocumentContextTestMetadata, test.old, test.replacement, 1))
			want := context
			want.LessonID = "MiXeD:A+1"
			t.Run("metadata", func(t *testing.T) {
				_, problem := decodeMetadata(raw, "document.metadata", context)
				decodeDocumentContextTestProblem(t, problem, CodeContract, test.field, want)
			})
			t.Run("document", func(t *testing.T) {
				_, problem := decodeDocument(decodeDocumentContextTestDocument(t, raw, json.RawMessage("false")), "document", context)
				decodeDocumentContextTestProblem(t, problem, CodeContract, test.field, want)
			})
		})
	}
}

func TestDecodeDocumentContextHasErrorsKeepsCompleteMetadataID(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Вход\Технический.json`, FileName: "l017.json", PageID: "page", Line: 27, Column: 4}
	raw := decodeDocumentContextTestDocument(t, json.RawMessage(decodeDocumentContextTestMetadata), json.RawMessage(`"false"`))
	_, problem := decodeDocument(raw, "document", context)
	want := context
	want.LessonID = "MiXeD:A+1"
	decodeDocumentContextTestProblem(t, problem, CodeContract, "document.hasErrors", want)
}

func TestDecodeDocumentContextLoadEarlyErrorsKeepID(t *testing.T) {
	for _, test := range []struct {
		name, old, replacement, hasErrors, field string
	}{
		{"title", `"title":null`, `"title":17`, "false", "document.metadata.title"},
		{"resourceDirs-element", `"resourceDirs":null`, `"resourceDirs":[null]`, "false", "document.metadata.resourceDirs[0]"},
		{"hasErrors", "", "", `"false"`, "document.hasErrors"},
	} {
		t.Run(test.name, func(t *testing.T) {
			metadata := decodeDocumentContextTestMetadata
			if test.old != "" {
				metadata = strings.Replace(metadata, test.old, test.replacement, 1)
			}
			document := decodeDocumentContextTestDocument(t, json.RawMessage(metadata), json.RawMessage(test.hasErrors))
			path := decodeDocumentContextTestWrite(t, decodeDocumentContextTestResult(t, document, "1.0"))
			input, problems := Load(path)
			if input != nil {
				t.Error("ошибка формы вернула пригодный Input")
			}
			if len(problems) == 0 {
				t.Fatal("ошибка формы не возвращена")
			}
			for _, problem := range problems {
				decodeDocumentContextTestProblem(t, problem, CodeContract, test.field, diagnostic.Context{Path: path, LessonID: "MiXeD:A+1"})
			}
		})
	}
}

func TestDecodeDocumentContextLoadDoesNotInventID(t *testing.T) {
	for _, test := range []struct {
		name, metadata, version, code, field string
	}{
		{"null", strings.Replace(decodeDocumentContextTestMetadata, `"MiXeD:A+1"`, "null", 1), "1.0", CodeMissingID, "document.metadata.documentId"},
		{"missing", strings.Replace(decodeDocumentContextTestMetadata, `"documentId":"MiXeD:A+1",`, "", 1), "1.0", CodeMissingID, "document.metadata.documentId"},
		{"unread-wrong-type", strings.Replace(decodeDocumentContextTestMetadata, `"MiXeD:A+1"`, "17", 1), "1.0", CodeContract, "document.metadata.documentId"},
		{"unread-unsupported-version", decodeDocumentContextTestMetadata, "9.0", CodeVersion, "formatVersion"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := decodeDocumentContextTestDocument(t, json.RawMessage(test.metadata), json.RawMessage("false"))
			path := decodeDocumentContextTestWrite(t, decodeDocumentContextTestResult(t, document, test.version))
			input, problems := Load(path)
			if input != nil || len(problems) == 0 {
				t.Fatalf("ожидался nil Input и ошибки: Input=%v, errors=%v", input, problems)
			}
			for _, problem := range problems {
				decodeDocumentContextTestProblem(t, problem, test.code, test.field, diagnostic.Context{Path: path})
			}
		})
	}
}

func TestDecodeDocumentContextMetadataNullIDIsNotGenerated(t *testing.T) {
	metadata := strings.Replace(decodeDocumentContextTestMetadata, `"MiXeD:A+1"`, "null", 1)
	got, problem := decodeMetadata(json.RawMessage(metadata), "document.metadata", diagnostic.Context{Path: "вход.json"})
	if problem != nil || got.DocumentID != nil {
		t.Fatalf("null ID должен сохраняться на этапе формы: DocumentID=%v, error=%v", got.DocumentID, problem)
	}
	metadata = strings.Replace(metadata, `"title":null`, `"title":17`, 1)
	_, problem = decodeMetadata(json.RawMessage(metadata), "document.metadata", diagnostic.Context{Path: "вход.json"})
	decodeDocumentContextTestProblem(t, problem, CodeContract, "document.metadata.title", diagnostic.Context{Path: "вход.json"})
}

func TestDecodeDocumentContextLoadValidControl(t *testing.T) {
	document := decodeDocumentContextTestDocument(t, json.RawMessage(decodeDocumentContextTestMetadata), json.RawMessage("false"))
	path := decodeDocumentContextTestWrite(t, decodeDocumentContextTestResult(t, document, "1.0"))
	input, problems := Load(path)
	if input == nil || len(problems) != 0 {
		t.Fatalf("валидная база отклонена: Input=%v, errors=%v", input, problems)
	}
	id := input.Result.Document.Metadata.DocumentID
	if input.Path != path || id == nil || *id != "MiXeD:A+1" {
		t.Fatalf("исходные путь/ID потеряны: Path=%q, DocumentID=%v", input.Path, id)
	}
}

// decodeDocumentContextTestDocument создаёт полный Document с заданной формой двух полей.
func decodeDocumentContextTestDocument(t *testing.T, metadata, hasErrors json.RawMessage) json.RawMessage {
	t.Helper()
	return decodeDocumentContextTestJSON(t, map[string]json.RawMessage{
		"dslVersion": json.RawMessage("null"), "fileName": json.RawMessage("null"),
		"filePath": json.RawMessage("null"), "encoding": json.RawMessage("null"),
		"hasBom": json.RawMessage("null"), "lineCount": json.RawMessage("null"),
		"byteLength": json.RawMessage("null"), "sha256": json.RawMessage("null"),
		"metadata": metadata, "hasErrors": hasErrors,
	})
}

// decodeDocumentContextTestResult создаёт полный технический JSON без поздних ошибок.
func decodeDocumentContextTestResult(t *testing.T, document json.RawMessage, version string) json.RawMessage {
	t.Helper()
	encodedVersion, err := json.Marshal(version)
	if err != nil {
		t.Fatal(err)
	}
	return decodeDocumentContextTestJSON(t, map[string]json.RawMessage{
		"formatVersion": encodedVersion, "document": document,
		"processing": json.RawMessage("[]"), "lines": json.RawMessage("[]"), "diagnostics": json.RawMessage("[]"),
	})
}

// decodeDocumentContextTestJSON сериализует тестовые поля без изменения скалярных значений.
func decodeDocumentContextTestJSON(t *testing.T, fields map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// decodeDocumentContextTestWrite сохраняет независимый вход в каталоге текущего теста.
func decodeDocumentContextTestWrite(t *testing.T, raw []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Технический вход.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// decodeDocumentContextTestProblem проверяет машинный код и фактический контекст без привязки к тексту причины.
func decodeDocumentContextTestProblem(t *testing.T, problem *diagnostic.Error, code, field string, context diagnostic.Context) {
	t.Helper()
	if problem == nil {
		t.Fatal("ожидаемая ошибка отсутствует")
	}
	if problem.Code != code {
		t.Errorf("Code=%q, требуется %q", problem.Code, code)
	}
	if problem.Context != context {
		t.Errorf("Context=%+v, требуется %+v", problem.Context, context)
	}
	if strings.TrimSpace(problem.Reason) == "" {
		t.Error("причина ошибки пуста")
	}
	for _, detail := range problem.Details {
		if detail.Value == field {
			return
		}
	}
	t.Errorf("Details=%+v не содержит точный путь поля %q", problem.Details, field)
}
