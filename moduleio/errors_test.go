package moduleio

import "testing"

func TestErrorsCodesMatchStorageContract(t *testing.T) {
	// Присваивания константам дополнительно проверяют константность и тип string.
	const (
		_ string = CodeOpen
		_ string = CodeArchive
		_ string = CodeEntryType
		_ string = CodeEntryName
		_ string = CodeDuplicateEntry
		_ string = CodeManifest
		_ string = CodeRead
		_ string = CodeClose
		_ string = CodeEncoding
		_ string = CodeJSON
		_ string = CodeWriter
		_ string = CodeSerialize
		_ string = CodeWrite
	)

	for _, test := range []struct {
		name string
		got  string
		want string
	}{
		{"CodeOpen", CodeOpen, "moduleio.open"},
		{"CodeArchive", CodeArchive, "moduleio.archive"},
		{"CodeEntryType", CodeEntryType, "moduleio.entry_type"},
		{"CodeEntryName", CodeEntryName, "moduleio.entry_name"},
		{"CodeDuplicateEntry", CodeDuplicateEntry, "moduleio.duplicate_entry"},
		{"CodeManifest", CodeManifest, "moduleio.manifest"},
		{"CodeRead", CodeRead, "moduleio.read"},
		{"CodeClose", CodeClose, "moduleio.close"},
		{"CodeEncoding", CodeEncoding, "moduleio.encoding"},
		{"CodeJSON", CodeJSON, "moduleio.json"},
		{"CodeWriter", CodeWriter, "moduleio.writer"},
		{"CodeSerialize", CodeSerialize, "moduleio.serialize"},
		{"CodeWrite", CodeWrite, "moduleio.write"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Errorf("%s = %q, want %q", test.name, test.got, test.want)
			}
		})
	}
}
