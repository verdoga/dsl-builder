package manifest_test

import (
	"testing"

	"github.com/verdoga/dsl-builder/manifest"
)

func TestErrorsContractStableCodes(t *testing.T) {
	for _, code := range errorsContractCodes(t) {
		t.Run(code.name, func(t *testing.T) {
			if code.got != code.want {
				t.Errorf("%s = %q, требуется стабильный код %q", code.name, code.got, code.want)
			}
		})
	}
}

func TestErrorsContractUniqueCodes(t *testing.T) {
	seen := make(map[string]string)
	for _, code := range errorsContractCodes(t) {
		if previous, exists := seen[code.got]; exists {
			t.Errorf("%s и %s имеют одинаковый код %q", previous, code.name, code.got)
		}
		seen[code.got] = code.name
	}
}

func errorsContractCodes(t *testing.T) []struct{ name, got, want string } {
	t.Helper()
	// Константный контекст также проверяет, что API остаётся константами.
	const (
		shape       = manifest.CodeShape
		version     = manifest.CodeVersion
		title       = manifest.CodeTitle
		createdAt   = manifest.CodeCreatedAt
		integer     = manifest.CodeInteger
		contents    = manifest.CodeContents
		nullOrder   = manifest.CodeNullOrder
		uniqueItems = manifest.CodeUniqueItems
	)
	return []struct{ name, got, want string }{
		{"CodeShape", shape, "manifest.shape"},
		{"CodeVersion", version, "manifest.format_version"},
		{"CodeTitle", title, "manifest.title"},
		{"CodeCreatedAt", createdAt, "manifest.created_at"},
		{"CodeInteger", integer, "manifest.integer"},
		{"CodeContents", contents, "manifest.contents"},
		{"CodeNullOrder", nullOrder, "manifest.null_order"},
		{"CodeUniqueItems", uniqueItems, "manifest.unique_items"},
	}
}
