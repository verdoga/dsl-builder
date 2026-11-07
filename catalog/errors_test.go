package catalog

import "testing"

func TestErrorsStableCodes(t *testing.T) {
	// Константные выражения проверяют, что машинные коды остаются константами API.
	const (
		state            = CodeState
		empty            = CodeEmpty
		mixedSections    = CodeMixedSections
		unassigned       = CodeUnassigned
		duplicateSection = CodeDuplicateSection
		partialWeight    = CodePartialWeight
		missingWeight    = CodeMissingWeight
		weightConflict   = CodeWeightConflict
		lessonOrder      = CodeLessonOrder
		sectionWeights   = CodeSectionWeights
		selection        = CodeSelection
		identity         = CodeIdentity
	)
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"CodeState", state, "catalog.state"},
		{"CodeEmpty", empty, "catalog.empty"},
		{"CodeMixedSections", mixedSections, "catalog.mixed_sections"},
		{"CodeUnassigned", unassigned, "catalog.unassigned"},
		{"CodeDuplicateSection", duplicateSection, "catalog.duplicate_section"},
		{"CodePartialWeight", partialWeight, "catalog.partial_weight"},
		{"CodeMissingWeight", missingWeight, "catalog.missing_weight"},
		{"CodeWeightConflict", weightConflict, "catalog.weight_conflict"},
		{"CodeLessonOrder", lessonOrder, "catalog.lesson_order"},
		{"CodeSectionWeights", sectionWeights, "catalog.section_weights"},
		{"CodeSelection", selection, "catalog.selection"},
		{"CodeIdentity", identity, "catalog.identity"},
	}

	seen := make(map[string]string, len(tests))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
		if previous, exists := seen[tt.got]; exists {
			t.Errorf("%s and %s share diagnostic code %q", previous, tt.name, tt.got)
		}
		seen[tt.got] = tt.name
	}
}
