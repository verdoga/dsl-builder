package lesson

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func TestLessonIDsNil(t *testing.T) {
	ctx := idContext("Owner")
	errs := ValidateIDs(nil, ctx)
	if len(errs) != 1 || errs[0] == nil {
		t.Fatalf("nil: got %v, want one diagnostic", errs)
	}
	if errs[0].Code != CodeContract || errs[0].Context != ctx {
		t.Fatalf("nil: got %#v", errs[0])
	}
}

func TestLessonIDsAcceptValidWithoutMutation(t *testing.T) {
	for _, name := range []string{"A:B", "Lesson+1", "УРОК", "É", "A-p001-n002-v03", "x\u0085"} {
		t.Run(name, func(t *testing.T) {
			key := identity.LessonKey(name)
			huge := strings.Repeat("9", 90)
			value := idLesson(name)
			value.Document.Version = "2"
			value.Document.Pages = []Page{
				{ID: key + "-p999", Nodes: []Node{
					idNode(key+"-n001-v77", idNode(key+"-n001-v01")),
					idNode(key + "-n001"), idNode(key + "-n" + huge + "-v" + huge),
				}},
				{ID: key + "-p002", Nodes: []Node{idNode(key + "-n003")}},
				{ID: key + "-p" + huge, Nodes: []Node{}},
			}
			before := idSnapshot(t, value)
			for range 2 {
				if errs := ValidateIDs(value, idContext(name)); len(errs) != 0 {
					t.Fatalf("valid IDs: %v", errs)
				}
			}
			if !bytes.Equal(before, idSnapshot(t, value)) {
				t.Fatal("ValidateIDs changed the lesson")
			}
		})
	}
	if errs := ValidateIDs(idLesson("Empty"), idContext("Empty")); len(errs) != 0 {
		t.Fatalf("empty pages: %v", errs)
	}
}

func TestLessonIDsPreserveIdentityDiagnostics(t *testing.T) {
	groups := []struct {
		kind   string
		values []string
	}{
		{"lesson", []string{"", "bad id", "bad\u00a0id", "bad\ufeffid", "a\x00b", string([]byte{0xff})}},
		{"page", []string{"", "owner-p1", "owner-p000", "owner-p0001", "Owner-p001", "owner-p001-v01", "owner-p001\n"}},
		{"node", []string{"", "owner-n1", "owner-n000", "owner-n0001", "Owner-n001", "owner-n001-v1", "owner-n001-v00", "owner-n001-v001", "owner-n001\n"}},
	}
	for _, group := range groups {
		for _, input := range group.values {
			t.Run(group.kind+"/"+input, func(t *testing.T) {
				value := idLesson("Owner")
				ctx := idContext("Owner")
				pointer := "/document/id"
				var original *diagnostic.Error
				switch group.kind {
				case "lesson":
					value.Document.ID = input
					ctx.LessonID = input
					original = identity.ValidateLessonID(input)
				case "page":
					value.Document.Pages = []Page{{ID: input, Nodes: []Node{}}}
					ctx.PageID = input
					pointer = "/document/pages/0/id"
					_, original = identity.ParsePageID(input)
				case "node":
					value.Document.Pages = []Page{{ID: "owner-p001", Nodes: []Node{idNode("owner-n001", idNode(input))}}}
					ctx.PageID = "owner-p001"
					pointer = "/document/pages/0/nodes/0/nodes/0/id"
					_, original = identity.ParseNodeID(input)
				}
				if original == nil {
					t.Fatal("test input unexpectedly accepted by identity")
				}
				errs := ValidateIDs(value, ctx)
				if len(errs) != 1 {
					t.Fatalf("got %v, want only identity error", errs)
				}
				idCheckError(t, errs[0], original.Code, pointer, ctx)
				for _, detail := range original.Details {
					found := false
					for _, got := range errs[0].Details {
						found = found || got == detail
					}
					if !found {
						t.Errorf("lost identity detail %#v: %#v", detail, errs[0].Details)
					}
				}
			})
		}
	}
}

func TestLessonIDsPrefixOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, lesson, page, node, pointer string
	}{
		{"page", "Owner", "owner-p001-p002", "owner-n001", "/document/pages/0/id"},
		{"node", "Owner", "owner-p001", "owner-n001-n002", "/document/pages/0/nodes/0/nodes/0/id"},
		{"unicode", "É", "e\u0301-p001", "é-n001", "/document/pages/0/id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := idLesson(tc.lesson)
			value.Document.Pages = []Page{{ID: tc.page, Nodes: []Node{
				idNode(identity.LessonKey(tc.lesson)+"-n900", idNode(tc.node)),
			}}}
			ctx := idContext(tc.lesson)
			ctx.PageID = tc.page
			errs := ValidateIDs(value, ctx)
			if len(errs) != 1 {
				t.Fatalf("got %v, want one ownership error", errs)
			}
			idCheckError(t, errs[0], CodeIDPrefix, tc.pointer, ctx)
		})
	}
}

func TestLessonIDsDuplicateTreeOrderAndFirstLocations(t *testing.T) {
	value := idLesson("Owner")
	value.Document.Pages = []Page{
		{ID: "owner-p009", Nodes: []Node{
			idNode("owner-n020", idNode("owner-n020", idNode("owner-n099-v03"))),
			idNode("owner-n099-v03"),
		}},
		{ID: "owner-p009", Nodes: []Node{idNode("owner-n099-v03"), idNode("owner-n020")}},
	}
	wants := []struct{ id, first, current string }{
		{"owner-n020", "/document/pages/0/nodes/0/id", "/document/pages/0/nodes/0/nodes/0/id"},
		{"owner-n099-v03", "/document/pages/0/nodes/0/nodes/0/nodes/0/id", "/document/pages/0/nodes/1/id"},
		{"owner-p009", "/document/pages/0/id", "/document/pages/1/id"},
		{"owner-n099-v03", "/document/pages/0/nodes/0/nodes/0/nodes/0/id", "/document/pages/1/nodes/0/id"},
		{"owner-n020", "/document/pages/0/nodes/0/id", "/document/pages/1/nodes/1/id"},
	}
	before := idSnapshot(t, value)
	ctx := idContext("Owner")
	ctx.PageID = "owner-p009"
	var previous []byte
	for range 3 {
		errs := ValidateIDs(value, ctx)
		if len(errs) != len(wants) {
			t.Fatalf("got %v, want %d duplicates", errs, len(wants))
		}
		for i, want := range wants {
			idCheckError(t, errs[i], CodeDuplicateID, want.current, ctx)
			for _, required := range []string{want.id, want.first, want.current} {
				idCheckDetailValue(t, errs[i], required)
			}
		}
		current := idSnapshot(t, errs)
		if previous != nil && !bytes.Equal(previous, current) {
			t.Fatal("diagnostics changed between identical calls")
		}
		previous = current
	}
	if !bytes.Equal(before, idSnapshot(t, value)) {
		t.Fatal("duplicate validation changed the lesson")
	}
}

func TestLessonIDsTraverseAfterInvalidParent(t *testing.T) {
	value := idLesson("Owner")
	value.Document.Pages = []Page{
		{ID: "owner-p001", Nodes: []Node{idNode("bad-parent", idNode("bad-child")), idNode("bad-sibling")}},
		{ID: "bad-page", Nodes: []Node{idNode("bad-next-page-node")}},
	}
	wants := []struct{ code, pointer, page string }{
		{identity.CodeInvalidNodeID, "/document/pages/0/nodes/0/id", "owner-p001"},
		{identity.CodeInvalidNodeID, "/document/pages/0/nodes/0/nodes/0/id", "owner-p001"},
		{identity.CodeInvalidNodeID, "/document/pages/0/nodes/1/id", "owner-p001"},
		{identity.CodeInvalidPageID, "/document/pages/1/id", "bad-page"},
		{identity.CodeInvalidNodeID, "/document/pages/1/nodes/0/id", "bad-page"},
	}
	ctx := idContext("Owner")
	errs := ValidateIDs(value, ctx)
	if len(errs) != len(wants) {
		t.Fatalf("got %v, want %d diagnostics", errs, len(wants))
	}
	for i, want := range wants {
		ctx.PageID = want.page
		idCheckError(t, errs[i], want.code, want.pointer, ctx)
	}
}

func idLesson(name string) *Lesson {
	return &Lesson{FormatVersion: FormatVersion, Document: Document{
		ID: name, Version: "1", Title: "Урок", LineCount: "7", CreatedAt: "2026-11-06T12:00:00Z", Pages: []Page{},
	}}
}

func idNode(name string, children ...Node) Node {
	if children == nil {
		children = []Node{}
	}
	return Node{ID: name, Type: NodeTask, Data: []Data{}, Nodes: children}
}

func idContext(lessonID string) diagnostic.Context {
	return diagnostic.Context{Path: "C:/Уроки/module.zip", FileName: "l007.json", LessonID: lessonID, Line: 12, Column: 3}
}

func idCheckError(t *testing.T, err *diagnostic.Error, code, pointer string, ctx diagnostic.Context) {
	t.Helper()
	if err == nil {
		t.Fatal("nil diagnostic")
	}
	if err.Code != code || err.Context != ctx || err.Reason == "" {
		t.Errorf("got %#v, want code=%q context=%#v and nonempty reason", err, code, ctx)
	}
	for _, detail := range err.Details {
		if detail.Label == "Поле JSON" && detail.Value == pointer {
			return
		}
	}
	t.Errorf("missing JSON pointer %q: %#v", pointer, err.Details)
}

func idCheckDetailValue(t *testing.T, err *diagnostic.Error, value string) {
	t.Helper()
	for _, detail := range err.Details {
		if detail.Value == value {
			return
		}
	}
	t.Errorf("missing detail value %q: %#v", value, err.Details)
}

func idSnapshot(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
