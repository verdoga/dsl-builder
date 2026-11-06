package model

import (
	"encoding/json"
	"testing"
)

func TestSeveritiesHaveStableUniqueValues(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		want     string
	}{
		{name: "SeverityError", severity: SeverityError, want: "error"},
		{name: "SeverityWarning", severity: SeverityWarning, want: "warning"},
		{name: "SeverityRecommendation", severity: SeverityRecommendation, want: "recommendation"},
	}

	seen := make(map[Severity]string, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.severity) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.severity, test.want)
			}
			if previous, exists := seen[test.severity]; exists {
				t.Errorf("%s и %s имеют одинаковое значение %q", previous, test.name, test.severity)
			}
			seen[test.severity] = test.name
		})
	}
}

func TestScopesHaveStableUniqueValues(t *testing.T) {
	tests := []struct {
		name  string
		scope Scope
		want  string
	}{
		{name: "ScopeElement", scope: ScopeElement, want: "element"},
		{name: "ScopeLine", scope: ScopeLine, want: "line"},
		{name: "ScopeBlock", scope: ScopeBlock, want: "block"},
		{name: "ScopeDocument", scope: ScopeDocument, want: "document"},
	}

	seen := make(map[Scope]string, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.scope) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.scope, test.want)
			}
			if previous, exists := seen[test.scope]; exists {
				t.Errorf("%s и %s имеют одинаковое значение %q", previous, test.name, test.scope)
			}
			seen[test.scope] = test.name
		})
	}
}

func TestDiagnosticJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		code     Code
		severity Severity
		scope    Scope
	}{
		{
			name:   "known_code",
			source: `{"id":"d1","source":"run1","code":"P011","severity":"error","message":"Незакрытый блок","scope":"block","fatal":true,"location":{"start":{"line":1,"column":7},"end":{"line":3,"column":2}},"relatedLocations":[]}`,
			code:   Code("P011"), severity: SeverityError, scope: ScopeBlock,
		},
		{
			name:   "custom_code",
			source: `{"id":"d2","source":"validator","code":"CUSTOM001","severity":"recommendation","message":"Подсказка 🌍","scope":"document","fatal":false,"location":null,"relatedLocations":null}`,
			code:   Code("CUSTOM001"), severity: SeverityRecommendation, scope: ScopeDocument,
		},
		{
			name:   "zero_values",
			source: `{"id":"","source":"","code":"","severity":"","message":"","scope":"","fatal":false,"location":null,"relatedLocations":[]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var decoded Diagnostic
			if err := json.Unmarshal([]byte(test.source), &decoded); err != nil {
				t.Fatalf("Чтение диагностики из JSON: %v", err)
			}
			if decoded.DiagnosticCode != test.code || decoded.SeverityLevel != test.severity || decoded.DiagnosticScope != test.scope {
				t.Fatalf("Значения диагностики изменились: code=%q, severity=%q, scope=%q",
					decoded.DiagnosticCode, decoded.SeverityLevel, decoded.DiagnosticScope)
			}
			encoded, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("Запись диагностики в JSON: %v", err)
			}
			assertJSONEqual(t, encoded, json.RawMessage(test.source))
		})
	}
}
