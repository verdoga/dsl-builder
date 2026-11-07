package revision

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/lesson"
)

func TestValidateTestHistoryVersions(t *testing.T) {
	for _, tc := range []struct {
		name, version, id string
		bad               bool
	}{
		{"initial", "1", "efx7-n001", false},
		{"initial suffix one", "1", "efx7-n001-v01", true},
		{"initial decimal", "1.00", "efx7-n001-v01", true},
		{"initial exponent", "10e-1", "efx7-n001-v01", true},
		{"later unsuffixed", "100", "efx7-n001", false},
		{"past suffix", "100", "efx7-n001-v02", false},
		{"same suffix", "100", "efx7-n001-v100", false},
		{"future suffix", "100", "efx7-n001-v101", true},
		{"exact above float", "9007199254740992", "efx7-n001-v9007199254740993", true},
		{"above uint64", "18446744073709551616", "efx7-n001-v18446744073709551616", false},
		{"large exponent", "1e100", "efx7-n001-v" + "1" + strings.Repeat("0", 100), false},
		{"large future", "1e100", "efx7-n001-v" + strings.Repeat("9", 101), true},
		{"large fractional exponent", "100.00e98", "efx7-n001-v" + "1" + strings.Repeat("0", 100), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := validateTestLesson(t)
			value.Document.Version = json.Number(tc.version)
			value.Document.Pages[0].Nodes[0].ID = tc.id
			validateTestValid(t, value)
			before := validateTestSnapshot(t, value)
			context := diagnostic.Context{Path: "архив.zip", FileName: "l003.json"}
			problems := ValidateHistory(value, context)
			code := ""
			if tc.bad {
				code = CodeHistory
			}
			validateTestErrors(t, problems, code)
			parts, problem := identity.ParseNodeID(tc.id)
			if problem != nil {
				t.Fatal(problem)
			}
			pointer := "/document/pages/0/nodes/0"
			direct := validateNodeRevision(parts, &value.Document, pointer, context)
			if (direct != nil) != tc.bad {
				t.Fatalf("validateNodeRevision = %v, bad=%v", direct, tc.bad)
			}
			if tc.bad {
				validateTestErrors(t, []*diagnostic.Error{direct}, CodeHistory)
				for _, issue := range append(problems, direct) {
					if issue.Context.Path != context.Path || issue.Context.FileName != context.FileName {
						t.Errorf("потерян контекст: %#v", issue)
					}
					details := validateTestDetails([]*diagnostic.Error{issue})
					if !strings.Contains(details, pointer) || !strings.Contains(details, tc.id) {
						t.Errorf("нет ID и адреса в подробностях: %q", details)
					}
				}
			}
			if validateTestSnapshot(t, value) != before {
				t.Fatal("проверка изменила урок")
			}
		})
	}
}

func TestValidateTestHistoryGlobalNumbers(t *testing.T) {
	for _, tc := range []struct {
		name      string
		duplicate bool
		target    string
	}{
		{"gaps and differing suffixes", false, ""},
		{"duplicate nested", true, "nested"},
		{"duplicate across pages", true, "page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := validateTestLesson(t)
			value.Document.Pages[0].Nodes[0].ID = "efx7-n001-v02"
			value.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n003-v05"
			second := "/document/pages/1/nodes/0"
			if tc.duplicate {
				if tc.target == "nested" {
					value.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n001-v03"
					second = "/document/pages/0/nodes/0/nodes/0"
				} else {
					value.Document.Pages[1].Nodes[0].ID = "efx7-n001"
				}
			}
			validateTestValid(t, value)
			context := diagnostic.Context{Path: "old.zip", FileName: "l001.json"}
			before := validateTestSnapshot(t, value)
			problems := ValidateHistory(value, context)
			code := ""
			if tc.duplicate {
				code = CodeHistory
			}
			validateTestErrors(t, problems, code)
			if tc.duplicate {
				if len(problems) != 1 {
					t.Fatalf("один конфликт, получено: %v", problems)
				}
				details := validateTestDetails(problems)
				for _, pointer := range []string{"/document/pages/0/nodes/0", second} {
					if !strings.Contains(details, pointer) {
						t.Errorf("нет адреса %s: %q", pointer, details)
					}
				}
				if len(problems[0].Related) < 2 {
					t.Errorf("конфликт не содержит обеих связанных позиций: %#v", problems[0])
				}
			}
			seen := make(map[string]string)
			var direct []*diagnostic.Error
			for i, page := range value.Document.Pages {
				for j, node := range page.Nodes {
					pointer := fmt.Sprintf("/document/pages/%d/nodes/%d", i, j)
					direct = append(direct, validateHistoryNode(node, &value.Document, seen, pointer, context)...)
				}
			}
			validateTestErrors(t, direct, code)
			if strings.TrimSuffix(seen["1"], "/id") != "/document/pages/0/nodes/0" {
				t.Errorf("не сохранён адрес первого номера: %v", seen)
			}
			if validateTestSnapshot(t, value) != before {
				t.Fatal("история изменила снимок")
			}
		})
	}
}

func TestValidateTestHistoryReportsPreorder(t *testing.T) {
	value := validateTestLesson(t)
	value.Document.Version = "1"
	value.Document.Pages[0].Nodes[0].ID = "efx7-n001-v02"
	value.Document.Pages[0].Nodes[0].Nodes[0].ID = "efx7-n003-v03"
	value.Document.Pages[1].Nodes[0].ID = "efx7-n1000-v04"
	validateTestValid(t, value)
	problems := ValidateHistory(value, diagnostic.Context{})
	validateTestErrors(t, problems, CodeHistory)
	last := -1
	for _, id := range []string{"efx7-n001-v02", "efx7-n003-v03", "efx7-n1000-v04"} {
		found := -1
		for i, issue := range problems {
			if strings.Contains(validateTestDetails([]*diagnostic.Error{issue}), id) {
				found = i
				break
			}
		}
		if found <= last {
			t.Fatalf("ошибки идут не в порядке дерева, ID=%s, ошибки=%v", id, problems)
		}
		last = found
	}
	value.Document.Pages = []lesson.Page{}
	validateTestValid(t, value)
	validateTestErrors(t, ValidateHistory(value, diagnostic.Context{}), "")
}
