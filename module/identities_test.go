package module

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
)

func idsContexts(t *testing.T, ids ...string) []diagnostic.Context {
	t.Helper()
	values := make([]diagnostic.Context, len(ids))
	for i, id := range ids {
		values[i] = diagnostic.Context{Path: "источники/" + id + ".json", LessonID: id, Line: i + 2, Column: i + 1}
	}
	return values
}

func idsRequireCodes(t *testing.T, problems []*diagnostic.Error, codes ...string) {
	t.Helper()
	if len(problems) != len(codes) {
		t.Fatalf("errors = %+v, want codes %v", problems, codes)
	}
	for i, code := range codes {
		if problems[i] == nil || problems[i].Code != code || problems[i].Reason == "" {
			t.Fatalf("error[%d] = %+v, want %s with reason", i, problems[i], code)
		}
	}
}

func idsRequireRelated(t *testing.T, problem *diagnostic.Error, want []diagnostic.Context) {
	t.Helper()
	if !slices.Equal(problem.Related, want) {
		t.Fatalf("Related = %+v, want %+v", problem.Related, want)
	}
}

func idsDiagnosticJSON(t *testing.T, problems []*diagnostic.Error) string {
	t.Helper()
	raw, err := json.Marshal(problems)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestIDsCheckNewEmptyAndDistinct(t *testing.T) {
	for _, incoming := range [][]diagnostic.Context{nil, {}} {
		idsRequireCodes(t, CheckNewIDs(incoming, nil), CodeEmpty)
	}
	if problems := validateIDs(nil); len(problems) != 0 {
		t.Fatalf("empty private validation = %+v", problems)
	}
	values := idsContexts(t, "УРОК+1", "A:B", "é", "e\u0301", "Σ", "ς", "A\u0085B", "A\u200bB")
	before := slices.Clone(values)
	idsRequireCodes(t, CheckNewIDs(values, nil))
	idsRequireCodes(t, validateIDs(values))
	if !slices.Equal(values, before) {
		t.Fatalf("incoming changed: %+v", values)
	}
}

func TestIDsIncomingInvalidPreservesIdentityDiagnostics(t *testing.T) {
	for _, id := range []string{"", "A B", " A", "A\tB", "A\nB", "A\x00B", "A\x7fB", "A\u00a0B", "A\ufeffB", string([]byte{0xff})} {
		t.Run("invalid_"+id, func(t *testing.T) {
			context := diagnostic.Context{Path: "источник.json", FileName: "l017.json", LessonID: id, Line: 8, Column: 3}
			want := identity.ValidateLessonID(id)
			if want == nil {
				t.Fatal("invalid fixture passed identity")
			}
			for _, problems := range [][]*diagnostic.Error{validateIDs([]diagnostic.Context{context}), CheckNewIDs([]diagnostic.Context{context}, nil)} {
				idsRequireCodes(t, problems, identity.CodeInvalidLessonID)
				if problems[0].Reason != want.Reason || problems[0].Context != context {
					t.Fatalf("dependency reason/context lost: %+v", problems[0])
				}
				for _, detail := range want.Details {
					if !slices.Contains(problems[0].Details, detail) {
						t.Fatalf("identity detail lost: %+v", detail)
					}
				}
			}
		})
	}
}

func TestIDsDuplicatesReportEveryRepeatAndWholeGroup(t *testing.T) {
	for _, ids := range [][]string{{"УРОК", "урок"}, {"K", "K", "k"}} {
		t.Run(ids[0], func(t *testing.T) {
			values := idsContexts(t, ids...)
			before := slices.Clone(values)
			for _, problems := range [][]*diagnostic.Error{validateIDs(values), CheckNewIDs(values, nil)} {
				codes := make([]string, len(ids)-1)
				for i := range codes {
					codes[i] = CodeDuplicateID
				}
				idsRequireCodes(t, problems, codes...)
				for i, problem := range problems {
					if problem.Context != values[i+1] {
						t.Fatalf("repeat context = %+v, want %+v", problem.Context, values[i+1])
					}
					idsRequireRelated(t, problem, values)
				}
			}
			if !slices.Equal(values, before) {
				t.Fatal("duplicate validation changed input")
			}
		})
	}
}

func TestIDsIndependentInvalidsAndInterleavedGroupsKeepInputOrder(t *testing.T) {
	values := idsContexts(t, "", "Beta", "ALPHA", "bad id", "alpha", "BETA", "Alpha", "bad id")
	before := slices.Clone(values)
	wantContexts := []diagnostic.Context{values[0], values[3], values[4], values[5], values[6], values[7]}
	wantCodes := []string{identity.CodeInvalidLessonID, identity.CodeInvalidLessonID, CodeDuplicateID, CodeDuplicateID, CodeDuplicateID, identity.CodeInvalidLessonID}
	var first string
	for iteration := 0; iteration < 20; iteration++ {
		problems := validateIDs(values)
		idsRequireCodes(t, problems, wantCodes...)
		for i, problem := range problems {
			if problem.Context != wantContexts[i] {
				t.Fatalf("error[%d] context = %+v, want %+v", i, problem.Context, wantContexts[i])
			}
		}
		idsRequireRelated(t, problems[2], []diagnostic.Context{values[2], values[4], values[6]})
		idsRequireRelated(t, problems[3], []diagnostic.Context{values[1], values[5]})
		idsRequireRelated(t, problems[4], []diagnostic.Context{values[2], values[4], values[6]})
		encoded := idsDiagnosticJSON(t, problems)
		if iteration == 0 {
			first = encoded
		} else if encoded != first {
			t.Fatal("diagnostic order/content is unstable")
		}
	}
	if !slices.Equal(values, before) {
		t.Fatal("mixed validation changed input")
	}
}
