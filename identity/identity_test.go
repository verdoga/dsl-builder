package identity_test

import (
	"testing"

	"github.com/verdoga/dsl-builder/identity"
)

func TestIdentityCodesStableValues(t *testing.T) {
	const (
		lessonIDCode = identity.CodeInvalidLessonID
		pageIDCode   = identity.CodeInvalidPageID
		nodeIDCode   = identity.CodeInvalidNodeID
		fileNameCode = identity.CodeInvalidFileName
		numberCode   = identity.CodeInvalidNumber
	)

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"CodeInvalidLessonID", lessonIDCode, "identity.invalid_lesson_id"},
		{"CodeInvalidPageID", pageIDCode, "identity.invalid_page_id"},
		{"CodeInvalidNodeID", nodeIDCode, "identity.invalid_node_id"},
		{"CodeInvalidFileName", fileNameCode, "identity.invalid_file_name"},
		{"CodeInvalidNumber", numberCode, "identity.invalid_number"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}
