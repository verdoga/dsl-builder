package module_test

import (
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
)

func TestErrorsCodeContract(t *testing.T) {
	// Объявления требуют именно константы, совместимые с обычной строкой.
	const (
		errorsModule             string = module.CodeModule
		errorsFile               string = module.CodeFile
		errorsEmpty              string = module.CodeEmpty
		errorsOriginalBytes      string = module.CodeOriginalBytes
		errorsDuplicateFile      string = module.CodeDuplicateFile
		errorsDuplicateID        string = module.CodeDuplicateID
		errorsExistingID         string = module.CodeExistingID
		errorsMissingLesson      string = module.CodeMissingLesson
		errorsAmbiguousLesson    string = module.CodeAmbiguousLesson
		errorsIdentityChange     string = module.CodeIdentityChange
		errorsDuplicateReference string = module.CodeDuplicateReference
		errorsMissingFile        string = module.CodeMissingFile
		errorsUnlistedFile       string = module.CodeUnlistedFile
		errorsMetadata           string = module.CodeMetadata
	)
	// Ожидаемые значения взяты из Builder/module/errors.go в arc.zip.
	cases := []struct {
		name string
		code string
		want string
	}{
		{"CodeModule", errorsModule, "module.model"},
		{"CodeFile", errorsFile, "module.file"},
		{"CodeEmpty", errorsEmpty, "module.empty"},
		{"CodeOriginalBytes", errorsOriginalBytes, "module.original_bytes"},
		{"CodeDuplicateFile", errorsDuplicateFile, "module.duplicate_file"},
		{"CodeDuplicateID", errorsDuplicateID, "module.duplicate_id"},
		{"CodeExistingID", errorsExistingID, "module.existing_id"},
		{"CodeMissingLesson", errorsMissingLesson, "module.missing_lesson"},
		{"CodeAmbiguousLesson", errorsAmbiguousLesson, "module.ambiguous_lesson"},
		{"CodeIdentityChange", errorsIdentityChange, "module.identity_change"},
		{"CodeDuplicateReference", errorsDuplicateReference, "module.duplicate_reference"},
		{"CodeMissingFile", errorsMissingFile, "module.missing_file"},
		{"CodeUnlistedFile", errorsUnlistedFile, "module.unlisted_file"},
		{"CodeMetadata", errorsMetadata, "module.metadata"},
	}
	seen := make(map[string]string, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Используется существующий публичный тип без нового типа ошибки.
			problem := diagnostic.Error{Code: tc.code}
			if problem.Code != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, problem.Code, tc.want)
			}
			if previous, exists := seen[problem.Code]; exists {
				t.Errorf("%s and %s share code %q", previous, tc.name, problem.Code)
			}
			seen[problem.Code] = tc.name
		})
	}
}
