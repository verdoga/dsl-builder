package identity_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func TestIdentityIntegrationRoundTrip(t *testing.T) {
	huge := strings.Repeat("1234567890", 80)
	for _, tc := range []struct{ lesson, prefix, input, number, padded, revision, suffix string }{
		{"EFX7", "efx7", "0001", "1", "001", "", ""},
		{"A:B", "a:b", "999", "999", "999", "0001", "-v01"},
		{"Lesson+1", "lesson+1", "001000", "1000", "1000", "0099", "-v99"},
		{"УРОК-p001-n003-v06", "урок-p001-n003-v06", "1001", "1001", "1001", "00100", "-v100"},
		{"É", "é", "1", "1", "001", "6", "-v06"},
		{"E\u0301", "e\u0301", "1", "1", "001", "", ""},
		{"A\u0085B", "a\u0085b", "1", "1", "001", "", ""},
		{"A\u200bB", "a\u200bb", "000" + huge, huge, huge, "000" + huge, "-v" + huge},
	} {
		t.Run(tc.lesson, func(t *testing.T) {
			if err := identity.ValidateLessonID(tc.lesson); err != nil {
				t.Fatal(err)
			}
			if got := identity.LessonKey(tc.lesson); got != tc.prefix {
				t.Fatalf("LessonKey = %q, want %q", got, tc.prefix)
			}
			page, err := identity.FormatPageID(tc.lesson, tc.input)
			if err != nil || page != tc.prefix+"-p"+tc.padded {
				t.Fatalf("FormatPageID = %q, %v", page, err)
			}
			pp, err := identity.ParsePageID(page)
			if err != nil || pp != (identity.PageIDParts{Prefix: tc.prefix, Number: tc.number}) {
				t.Fatalf("ParsePageID = %+v, %v", pp, err)
			}
			pageAgain, err := identity.FormatPageID(pp.Prefix, pp.Number)
			if err != nil || pageAgain != page {
				t.Fatalf("page round trip = %q, %v", pageAgain, err)
			}
			node, err := identity.FormatNodeID(tc.lesson, tc.input, tc.revision)
			if err != nil || node != tc.prefix+"-n"+tc.padded+tc.suffix {
				t.Fatalf("FormatNodeID = %q, %v", node, err)
			}
			np, err := identity.ParseNodeID(node)
			wantRevision := strings.TrimLeft(tc.revision, "0")
			if err != nil || np != (identity.NodeIDParts{Prefix: tc.prefix, Number: tc.number, Revision: wantRevision}) {
				t.Fatalf("ParseNodeID = %+v, %v", np, err)
			}
			nodeAgain, err := identity.FormatNodeID(np.Prefix, np.Number, np.Revision)
			if err != nil || nodeAgain != node {
				t.Fatalf("node round trip = %q, %v", nodeAgain, err)
			}
			file, err := identity.FormatFileName(tc.input)
			if err != nil || file != "l"+tc.padded+".json" {
				t.Fatalf("FormatFileName = %q, %v", file, err)
			}
			number, err := identity.ParseFileName(file)
			if err != nil || number != tc.number {
				t.Fatalf("ParseFileName = %q, %v", number, err)
			}
			fileAgain, err := identity.FormatFileName(number)
			if err != nil || fileAgain != file {
				t.Fatalf("file round trip = %q, %v", fileAgain, err)
			}
		})
	}
}

func TestIdentityIntegrationCallerRevision(t *testing.T) {
	for _, old := range []string{"a:b-n035", "a:b-n035-v03", "a:b-n035-v100"} {
		for _, tc := range []struct{ revision, want string }{{"6", "a:b-n035-v06"}, {"1", "a:b-n035-v01"}, {"100", "a:b-n035-v100"}, {"", "a:b-n035"}} {
			t.Run(old+"/"+tc.revision, func(t *testing.T) {
				parts, err := identity.ParseNodeID(old)
				if err != nil {
					t.Fatal(err)
				}
				got, err := identity.FormatNodeID(parts.Prefix, parts.Number, tc.revision)
				if err != nil || got != tc.want {
					t.Fatalf("caller-selected revision = %q, %v; want %q", got, err, tc.want)
				}
				parsed, err := identity.ParseNodeID(got)
				if err != nil || parsed.Prefix != parts.Prefix || parsed.Number != parts.Number || parsed.Revision != tc.revision {
					t.Fatalf("replacement parts = %+v, %v", parsed, err)
				}
			})
		}
	}
	for _, pair := range [][2]string{{"a-n001", "a-n001-v01"}, {"a-n001-v01", "a-n001-v02"}, {"a-n001", "b-n001"}} {
		formatted := [2]string{}
		for i, id := range pair {
			parts, err := identity.ParseNodeID(id)
			if err != nil {
				t.Fatal(err)
			}
			formatted[i], err = identity.FormatNodeID(parts.Prefix, parts.Number, parts.Revision)
			if err != nil || formatted[i] != id {
				t.Fatalf("full ID was changed: %q, %v", formatted[i], err)
			}
		}
		if formatted[0] == formatted[1] {
			t.Fatalf("distinct full IDs collapsed: %q", pair)
		}
	}
}

func TestIdentityIntegrationDiagnostics(t *testing.T) {
	for _, bad := range []string{"", "A B", "A\ufeffB", string([]byte{'a', 0xff})} {
		t.Run(fmt.Sprintf("lesson/%q", bad), func(t *testing.T) {
			identityIntegrationError(t, identity.ValidateLessonID(bad), identity.CodeInvalidLessonID, bad)
			page, err := identity.FormatPageID(bad, "1")
			if page != "" {
				t.Fatalf("nonzero page: %q", page)
			}
			identityIntegrationError(t, err, identity.CodeInvalidLessonID, bad)
			node, err := identity.FormatNodeID(bad, "1", "")
			if node != "" {
				t.Fatalf("nonzero node: %q", node)
			}
			identityIntegrationError(t, err, identity.CodeInvalidLessonID, bad)
		})
	}
	for _, bad := range []string{"", "000", "+1", "１２", "1\n"} {
		t.Run(fmt.Sprintf("number/%q", bad), func(t *testing.T) {
			file, err := identity.FormatFileName(bad)
			if file != "" {
				t.Fatalf("nonzero file: %q", file)
			}
			identityIntegrationError(t, err, identity.CodeInvalidNumber, bad)
			page, err := identity.FormatPageID("A:B", bad)
			if page != "" {
				t.Fatalf("nonzero page: %q", page)
			}
			identityIntegrationError(t, err, identity.CodeInvalidNumber, bad)
			node, err := identity.FormatNodeID("A:B", bad, "6")
			if node != "" {
				t.Fatalf("nonzero node: %q", node)
			}
			identityIntegrationError(t, err, identity.CodeInvalidNumber, bad)
			if bad == "" {
				return
			} // Пустая ревизия обозначает отсутствие суффикса.
			node, revisionErr := identity.FormatNodeID("A:B", "1", bad)
			if node != "" {
				t.Fatalf("nonzero node: %q", node)
			}
			identityIntegrationError(t, revisionErr, identity.CodeInvalidNumber, bad)
			if slices.Equal(err.Details, revisionErr.Details) {
				t.Error("number and revision diagnostics do not distinguish the invalid part")
			}
		})
	}
	for _, tc := range []struct{ page, node, file string }{
		{"A-p001", "A-n001", "L001.json"},
		{"a-p0001", "a-n0001-v01", "l0001.json"},
		{"a-p001\n", "a-n001-v01\n", "l001.json\n"},
		{"a-p001-v01", "a-n001-v00", "dir/l001.json"},
	} {
		t.Run(tc.page, func(t *testing.T) {
			page, err := identity.ParsePageID(tc.page)
			if page != (identity.PageIDParts{}) {
				t.Fatalf("nonzero parts: %+v", page)
			}
			identityIntegrationError(t, err, identity.CodeInvalidPageID, tc.page)
			node, err := identity.ParseNodeID(tc.node)
			if node != (identity.NodeIDParts{}) {
				t.Fatalf("nonzero parts: %+v", node)
			}
			identityIntegrationError(t, err, identity.CodeInvalidNodeID, tc.node)
			number, err := identity.ParseFileName(tc.file)
			if number != "" {
				t.Fatalf("nonzero number: %q", number)
			}
			identityIntegrationError(t, err, identity.CodeInvalidFileName, tc.file)
		})
	}
}

func identityIntegrationError(t *testing.T, err *diagnostic.Error, code, bad string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected diagnostic error")
	}
	var found *diagnostic.Error
	if !errors.As(fmt.Errorf("caller: %w", err), &found) || found != err {
		t.Fatal("errors.As lost diagnostic")
	}
	if found.Code != code || !strings.ContainsFunc(found.Reason, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
		t.Fatalf("invalid code/reason: %+v", found)
	}
	if found.Context != (diagnostic.Context{}) || len(found.Related) != 0 {
		t.Fatalf("invented context: %+v", found)
	}
	for _, detail := range found.Details {
		if detail.Value == bad && strings.ContainsFunc(detail.Label, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) {
			return
		}
	}
	t.Fatalf("missing exact invalid value %q in Russian-labeled details: %+v", bad, found.Details)
}

type identityIntegrationNode struct {
	ID    string                    `json:"id"`
	Nodes []identityIntegrationNode `json:"nodes"`
}

func TestIdentityIntegrationGoldenIDs(t *testing.T) {
	for n := 1; n <= 7; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			var parsed struct {
				Document struct {
					Metadata struct {
						DocumentID string `json:"documentId"`
					} `json:"metadata"`
				} `json:"document"`
			}
			var lesson struct {
				Document struct {
					ID    string                    `json:"id"`
					Pages []identityIntegrationNode `json:"pages"`
				} `json:"document"`
			}
			identityIntegrationReadJSON(t, fmt.Sprintf("../golden/%d. parsed.json", n), &parsed)
			identityIntegrationReadJSON(t, fmt.Sprintf("../golden/%d. lesson.json", n), &lesson)
			original := parsed.Document.Metadata.DocumentID
			if err := identity.ValidateLessonID(original); err != nil {
				t.Fatal(err)
			}
			if err := identity.ValidateLessonID(lesson.Document.ID); err != nil {
				t.Fatal(err)
			}
			if lesson.Document.ID != original {
				t.Fatalf("original lesson ID changed: %q -> %q", original, lesson.Document.ID)
			}
			key := identity.LessonKey(original)
			if key != strings.ToLower(original) || key == original {
				t.Fatalf("golden lesson ID and lower-case prefix: %q / %q", original, key)
			}
			if len(lesson.Document.Pages) == 0 {
				t.Fatal("no golden page IDs extracted")
			}
			nodeCount := 0
			for _, page := range lesson.Document.Pages {
				parts, err := identity.ParsePageID(page.ID)
				if err != nil || parts.Prefix != key || parts.Number == "" {
					t.Fatalf("golden page %q: %+v, %v", page.ID, parts, err)
				}
				again, err := identity.FormatPageID(original, parts.Number)
				if err != nil || again != page.ID {
					t.Fatalf("golden page round trip: %q, %v", again, err)
				}
				nodeCount += identityIntegrationGoldenNodes(t, page.Nodes, key)
			}
			if nodeCount == 0 {
				t.Fatal("no golden node IDs extracted")
			}
			t.Logf("extracted documentId + document.id, %d page IDs, %d node IDs", len(lesson.Document.Pages), nodeCount)
		})
	}
}

func identityIntegrationGoldenNodes(t *testing.T, nodes []identityIntegrationNode, key string) int {
	t.Helper()
	count := 0
	for _, node := range nodes {
		parts, err := identity.ParseNodeID(node.ID)
		if err != nil || parts.Prefix != key || parts.Number == "" {
			t.Fatalf("golden node %q: %+v, %v", node.ID, parts, err)
		}
		again, err := identity.FormatNodeID(parts.Prefix, parts.Number, parts.Revision)
		if err != nil || again != node.ID {
			t.Fatalf("golden node round trip: %q, %v", again, err)
		}
		count += 1 + identityIntegrationGoldenNodes(t, node.Nodes, key)
	}
	return count
}

func identityIntegrationReadJSON(t *testing.T, path string, target any) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contents, target); err != nil {
		t.Fatal(err)
	}
}
