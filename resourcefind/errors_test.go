package resourcefind

import (
	"slices"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
)

func TestRFErrorsStableCodes(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{CodeNotFound, "resourcefind.not_found"},
		{CodeAmbiguous, "resourcefind.ambiguous"},
		{CodeExtension, "resourcefind.extension"},
		{CodeKind, "resourcefind.kind"},
	} {
		if tc.got != tc.want {
			t.Errorf("code = %q, want %q", tc.got, tc.want)
		}
	}
}

func rfErrorsRequirePreserved(t *testing.T, got, want *diagnostic.Error) {
	t.Helper()
	if got == nil {
		t.Fatal("diagnostic is nil")
	}
	if got.Code != want.Code || got.Reason != want.Reason || got.Context != want.Context || got.Cause != want.Cause {
		t.Errorf("diagnostic fields changed: got %#v, want %#v", got, want)
	}
	if !slices.Equal(got.Related, want.Related) {
		t.Errorf("Related = %#v, want %#v", got.Related, want.Related)
	}
	if !slices.Equal(got.Details, want.Details) {
		t.Errorf("Details = %#v, want %#v", got.Details, want.Details)
	}
}

func rfErrorsRequireDetails(t *testing.T, got []diagnostic.Detail, values []string) {
	t.Helper()
	if len(got) != len(values) {
		t.Fatalf("Details = %#v; want exactly %d known values", got, len(values))
	}
	remaining := slices.Clone(values)
	for _, detail := range got {
		if !rfErrorsHasCyrillic(detail.Label) {
			t.Errorf("detail label %q must be in Russian", detail.Label)
		}
		index := slices.Index(remaining, detail.Value)
		if index < 0 {
			t.Errorf("unexpected or changed detail value %q", detail.Value)
			continue
		}
		remaining = slices.Delete(remaining, index, index+1)
	}
	if len(remaining) != 0 {
		t.Errorf("missing exact detail values: %q", remaining)
	}
}

func rfErrorsHasCyrillic(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}
