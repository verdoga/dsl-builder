package app

import (
	"strings"
	"testing"
)

func TestDestinationRegressionNameEdgesAreRejectedWithoutRepair(t *testing.T) {
	for _, name := range []string{
		" unit", ".unit", "unit ", "unit.",
		" unit.ZIP", ".unit.ZIP", "unit.ZIP ", "unit.ZIP.",
		" .unit ", ".unit.zip.",
	} {
		t.Run(name, func(t *testing.T) {
			got, issue := archiveName(name)
			if got != "" || issue == nil || issue.Code != CodeArchiveName || issue.Reason == "" {
				t.Fatalf("archiveName(%q) = %q, %#v; invalid edge must fail without repair", name, got, issue)
			}
		})
	}
}

func TestDestinationRegressionNameExtensionAndUnicodePreserveSpelling(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"existing.ZIP", "existing.ZIP"},
		{"existing.zIp", "existing.zIp"},
		{"Я. 课", "Я. 课.zip"},
		{"é.я", "é.я.zip"},
		{"unit .part", "unit .part.zip"},
		{"unit. part", "unit. part.zip"},
		{"Курс 日本語.ZIP", "Курс 日本語.ZIP"},
		{"résumé e\u0301", "résumé e\u0301.zip"},
		{"unit.tar", "unit.tar.zip"},
		{strings.Repeat("Я", 1000), strings.Repeat("Я", 1000) + ".zip"},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, issue := archiveName(test.input)
			if issue != nil || got != test.want {
				t.Fatalf("archiveName(%q) = %q, %v; want exact %q", test.input, got, issue, test.want)
			}
		})
	}
}
