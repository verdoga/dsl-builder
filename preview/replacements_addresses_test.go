package preview

import (
	"bytes"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/module"
)

func TestPageReplacementMissingSnapshotsAndAddressesWithValidSelector(t *testing.T) {
	before := replacementsTestFile(t, true,
		replacementsTestPage("efx7-p001", replacementsTestNode("efx7-n001")))
	after := replacementsTestFile(t, false,
		replacementsTestPage("efx7-p001", replacementsTestNode("efx7-n002-v02"), replacementsTestNode("efx7-n003-v02")))
	beforeTwo := replacementsTestFile(t, true,
		replacementsTestPage("efx7-p001"),
		replacementsTestPage("efx7-p002", replacementsTestNode("efx7-n001")))
	afterTwo := replacementsTestFile(t, false,
		replacementsTestPage("efx7-p001"),
		replacementsTestPage("efx7-p002", replacementsTestNode("efx7-n002-v02"), replacementsTestNode("efx7-n003-v02")))
	emptyBefore := replacementsTestFile(t, true)
	emptyAfter := replacementsTestFile(t, false)
	zeroBefore, zeroAfter := &module.File{}, &module.File{}

	for _, tc := range []struct {
		name          string
		before, after *module.File
		index         int
	}{
		{"nil old snapshot", nil, after, 0},
		{"nil new snapshot", before, nil, 0},
		{"zero old file", zeroBefore, after, 0},
		{"zero new file", before, zeroAfter, 0},
		{"negative page index", before, after, -1},
		{"index beyond both snapshots", before, after, 1},
		{"index beyond old snapshot only", before, afterTwo, 1},
		{"index beyond new snapshot only", beforeTwo, after, 1},
		{"page absent from old snapshot", emptyBefore, after, 0},
		{"page absent from new snapshot", before, emptyAfter, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer replacementsTestUnchanged(t, tc.before)()
			defer replacementsTestUnchanged(t, tc.after)()
			// R1: согласованный селектор позволяет дойти до недоступного
			// снимка/адреса, а не завершиться на ReplaceNodes=false.
			change := compare.PageChange{
				Index:        tc.index,
				ReplaceNodes: true,
				Nodes:        []compare.NodeChange{},
				Reasons: []compare.Reason{{
					Code:        compare.NodeCountChanged,
					Description: "Число корневых нод изменилось с 1 до 2.",
				}},
			}
			original := replacementsTestJSON(t, change)
			text, err := pageReplacement(tc.before, tc.after, change)
			replacementsTestError(t, text, err)
			for _, dumpField := range []string{`"document":`, `"nodes":`, `"data":`} {
				if strings.Contains(err.Error(), dumpField) {
					t.Errorf("error contains a JSON dump field %q", dumpField)
				}
			}
			if !bytes.Equal(original, replacementsTestJSON(t, change)) {
				t.Error("page-address error changed PageChange")
			}
		})
	}
}
