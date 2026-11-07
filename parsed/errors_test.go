package parsed

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/order"
)

func TestErrorsStableCodes(t *testing.T) {
	tests := []struct {
		name, got, want string
	}{
		{"read", CodeRead, "parsed.read"},
		{"utf8", CodeUTF8, "parsed.utf8"},
		{"json", CodeJSON, "parsed.json"},
		{"version", CodeVersion, "parsed.version"},
		{"contract", CodeContract, "parsed.contract"},
		{"missing ID", CodeMissingID, "parsed.missing_id"},
		{"parser", CodeParser, "parsed.parser"},
		{"parent", CodeParent, "parsed.parent"},
		{"parent cycle", CodeParentCycle, "parsed.parent_cycle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("code = %q, want %q", tt.got, tt.want)
			}
			got := inputError(tt.got, "Нарушено условие входного контракта.", "lines[2].parentLine", diagnostic.Context{}, nil)
			if got == nil || got.Code != tt.want {
				t.Fatalf("inputError code = %v, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorsInputErrorPreservesContextReasonAndCause(t *testing.T) {
	sentinel := errors.New("technical cause: do not replace the Russian reason")
	wrapped := fmt.Errorf("read failed: %w", sentinel)
	tests := []struct {
		name    string
		context diagnostic.Context
		cause   error
	}{
		{"unknown location", diagnostic.Context{}, nil},
		{"path without coordinates", diagnostic.Context{Path: `C:\Учебные материалы\Урок 1.json`}, sentinel},
		{"known line without column", diagnostic.Context{Path: "inputs/a.json", LessonID: "Ab:Я+1", Line: 42}, wrapped},
		{"full known context", diagnostic.Context{
			Path: `C:\Материалы\..\Вход\A.json`, FileName: "l009.json",
			LessonID: "Ab:Я+1", PageID: "ab:я+1-p002", Line: 17, Column: 9,
		}, wrapped},
	}
	const reason = "Не удалось проверить технический JSON."
	const field = "lines[16].elements[0].value"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inputError(CodeContract, reason, field, tt.context, tt.cause)
			if got == nil {
				t.Fatal("inputError returned nil")
			}
			if got.Code != CodeContract || got.Reason != reason {
				t.Errorf("Code/Reason = %q/%q", got.Code, got.Reason)
			}
			if got.Context != tt.context {
				t.Errorf("Context = %+v, want %+v", got.Context, tt.context)
			}
			if got.Cause != tt.cause || errors.Unwrap(got) != tt.cause {
				t.Errorf("Cause/Unwrap did not preserve original error %v", tt.cause)
			}
			if tt.cause != nil && (!errors.Is(got, tt.cause) || !errors.Is(got, sentinel)) {
				t.Error("errors.Is cannot reach the original cause")
			}
			var structured *diagnostic.Error
			if !errors.As(fmt.Errorf("load: %w", got), &structured) || structured != got {
				t.Error("errors.As cannot recover the structured diagnostic")
			}
			errorsTestRequireDetailValue(t, got.Details, field)
			if len(got.Related) != 0 {
				t.Errorf("invented related locations: %+v", got.Related)
			}
		})
	}
}

func TestErrorsInputErrorWithoutField(t *testing.T) {
	got := inputError(CodeRead, "Не удалось прочитать технический JSON.", "", diagnostic.Context{Path: "inputs/a.json"}, nil)
	if got == nil {
		t.Fatal("inputError returned nil")
	}
	if got.Context.Line != 0 || got.Context.Column != 0 || got.Context.LessonID != "" {
		t.Errorf("invented source information: %+v", got.Context)
	}
	if len(got.Details) != 0 {
		t.Errorf("details invented without a field or technical cause: %+v", got.Details)
	}
	if got.Cause != nil || errors.Unwrap(got) != nil {
		t.Error("invented a technical cause")
	}
}

func TestErrorsJSONOffsetsAreDetailsNotDSLCoordinates(t *testing.T) {
	tests := []struct {
		name, offset string
		cause        error
	}{
		{"syntax", "97", &json.SyntaxError{Offset: 97}},
		{"field type", "123", &json.UnmarshalTypeError{Value: "number", Offset: 123, Field: "lineCount"}},
		{"wrapped syntax", "219", fmt.Errorf("decode: %w", &json.SyntaxError{Offset: 219})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, context := range []diagnostic.Context{
				{Path: `C:\Вход\Урок.json`},
				{Path: `C:\Вход\Урок.json`, LessonID: "Урок:1", Line: 41, Column: 3},
			} {
				got := inputError(CodeJSON, "Некорректный технический JSON.", "document.lineCount", context, tt.cause)
				if got == nil {
					t.Fatal("inputError returned nil")
				}
				if got.Context != context {
					t.Errorf("JSON position changed DSL context: got %+v, want %+v", got.Context, context)
				}
				if !errors.Is(got, tt.cause) {
					t.Error("JSON cause is absent from the error chain")
				}
				errorsTestRequireDetailValue(t, got.Details, "document.lineCount")
				found := false
				for _, detail := range got.Details {
					text := strings.ToLower(detail.Label + " " + detail.Value)
					if strings.Contains(text, "json") && strings.Contains(text, tt.offset) {
						found = true
					}
				}
				if !found {
					t.Errorf("JSON offset %s is not separately identified in Details: %+v", tt.offset, got.Details)
				}
			}
		})
	}
}

func TestErrorsDependencyCodesAndOriginalValuesSurvive(t *testing.T) {
	context := diagnostic.Context{Path: `C:\Курс\Урок.json`, LessonID: "Ab:Я+1", Line: 7}
	tests := []struct {
		name, field string
		cause       *diagnostic.Error
	}{
		{"identity", "document.metadata.documentId", identity.ValidateLessonID(" Ab:Я+1 ")},
		{"order", "document.metadata.order", order.Validate("0000", context)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cause == nil {
				t.Fatal("real dependency unexpectedly accepted invalid input")
			}
			got := inputError(tt.cause.Code, tt.cause.Reason, tt.field, context, tt.cause)
			if got == nil {
				t.Fatal("inputError returned nil")
			}
			if got.Code != tt.cause.Code || got.Reason != tt.cause.Reason || got.Context != context {
				t.Errorf("dependency diagnostic lost Code, Reason or actual Context: %+v", got)
			}
			if got.Cause != tt.cause || !errors.Is(got, tt.cause) {
				t.Fatal("dependency error was removed from the chain")
			}
			var original *diagnostic.Error
			if !errors.As(got.Cause, &original) || original != tt.cause {
				t.Fatal("dependency error is unavailable through errors.As")
			}
			for _, detail := range tt.cause.Details {
				errorsTestRequireDetailValue(t, original.Details, detail.Value)
			}
			errorsTestRequireDetailValue(t, got.Details, tt.field)
		})
	}
}

func errorsTestRequireDetailValue(t *testing.T, details []diagnostic.Detail, value string) {
	t.Helper()
	for _, detail := range details {
		if detail.Value == value && detail.Label != "" {
			return
		}
	}
	t.Errorf("missing named detail with exact value %q: %+v", value, details)
}
