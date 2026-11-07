package convert

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/structure"
)

func TestDiagnosticsCodes(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{CodeInput, "convert.input"},
		{CodeMetadata, "convert.metadata"},
		{CodeMediaSource, "convert.media_source"},
	} {
		if tc.got != tc.want {
			t.Errorf("code = %q, want %q", tc.got, tc.want)
		}
	}
}

func TestDiagnosticsInputContextReplacesOnlyInputFields(t *testing.T) {
	input, _ := diagnosticsBindings(t)
	before := diagnosticsJSON(t, input)
	for _, name := range []string{"l017.json", ""} {
		t.Run(name, func(t *testing.T) {
			base := diagnostic.Context{Path: "old.zip", FileName: name, LessonID: "OLD", PageID: "old-p009", Line: 300, Column: 20}
			got := inputContext(input, base)
			want := diagnostic.Context{Path: input.Path, FileName: name, LessonID: "MiXeD-7"}
			if got != want {
				t.Fatalf("context = %+v, want %+v", got, want)
			}
			if base.Path != "old.zip" || base.PageID != "old-p009" || base.Line != 300 || base.Column != 20 {
				t.Fatal("base context changed")
			}
		})
	}
	if string(diagnosticsJSON(t, input)) != string(before) {
		t.Fatal("input or its index changed")
	}
}

func TestDiagnosticsBindingContextKeepsPhysicalSource(t *testing.T) {
	input, bindings := diagnosticsBindings(t)
	base := inputContext(input, diagnostic.Context{FileName: "l017.json", PageID: "old-p001", Line: 50, Column: 12})
	for _, binding := range bindings {
		got := bindingContext(base, binding)
		want := diagnostic.Context{Path: input.Path, FileName: "l017.json", LessonID: "MiXeD-7", Line: binding.Entries[0].Source.Number}
		if got != want {
			t.Errorf("page %d path %v: context = %+v, want %+v", binding.Page, binding.NodePath, got, want)
		}
	}
	if bindings[1].Entries[0] == bindings[3].Entries[0] || bindings[1].Entries[0].Source.Number != bindings[3].Entries[0].Source.Number {
		t.Fatal("fixture must have independent inclusions with repeated physical numbers")
	}
}

func TestDiagnosticsBindingErrorPreservesOwnerAndCopiesSlices(t *testing.T) {
	input, bindings := diagnosticsBindings(t)
	binding := bindings[3]
	base := bindingContext(inputContext(input, diagnostic.Context{FileName: "l017.json"}), binding)
	cause := &diagnostic.Error{Code: "disk.read", Reason: "Техническая причина", Related: []diagnostic.Context{{Path: "nested"}}, Details: []diagnostic.Detail{{Label: "Вложенная причина", Value: "точно"}}}
	owner := &diagnostic.Error{
		Code: "resourceembed.read", Reason: "Не удалось прочитать выбранный ресурс",
		Context: diagnostic.Context{Path: "C:\\Ресурсы\\файл.mp3", FileName: "l017.json", LessonID: "MiXeD-7"}, Cause: cause,
		Related: make([]diagnostic.Context, 1, 8), Details: make([]diagnostic.Detail, 1, 8),
	}
	owner.Related[0] = diagnostic.Context{Path: "C:\\Другой\\кандидат.mp3", Line: 2}
	owner.Details[0] = diagnostic.Detail{Label: "Исходное значение", Value: " 003 "}
	before := diagnosticsJSON(t, owner)
	got := bindingError(owner, binding, base)
	if got == nil || got == owner {
		t.Fatal("expected independent diagnostic")
	}
	if got.Code != owner.Code || got.Reason != owner.Reason || got.Context != owner.Context || got.Cause != cause || !errors.Is(got, cause) {
		t.Fatalf("owner fields or cause changed: %+v", got)
	}
	if len(got.Related) < 1 || len(got.Details) < 1 || !reflect.DeepEqual(got.Related[:1], owner.Related) || !reflect.DeepEqual(got.Details[:1], owner.Details) {
		t.Fatal("owner Related/Details prefix changed")
	}
	diagnosticsRelated(t, got.Related[1:], input.Path, []int{9, 6, 3})
	if string(diagnosticsJSON(t, owner)) != string(before) {
		t.Fatal("owner changed during enrichment")
	}
	second := bindingError(owner, binding, base)
	got.Related[0].Path = "changed result"
	got.Details[0].Value = "changed result"
	if string(diagnosticsJSON(t, owner)) != string(before) || second.Related[0].Path != owner.Related[0].Path || second.Details[0].Value != " 003 " {
		t.Fatal("diagnostic slices share mutable storage")
	}
	for _, slot := range owner.Related[1:cap(owner.Related)] {
		if slot != (diagnostic.Context{}) {
			t.Fatal("append wrote into owner Related capacity")
		}
	}
	for _, slot := range owner.Details[1:cap(owner.Details)] {
		if slot != (diagnostic.Detail{}) {
			t.Fatal("append wrote into owner Details capacity")
		}
	}
}

func TestDiagnosticsBindingErrorDeduplicatesKnownLocations(t *testing.T) {
	input, bindings := diagnosticsBindings(t)
	base := inputContext(input, diagnostic.Context{FileName: "l017.json"})
	firstContext := bindingContext(base, bindings[1])
	first := bindingError(&diagnostic.Error{Code: "data.failure", Reason: "Ошибка", Context: firstContext}, bindings[1], firstContext)
	diagnosticsRelated(t, first.Related, input.Path, []int{6, 1})
	owner := &diagnostic.Error{Code: "data.failure", Reason: "Ошибка", Context: firstContext, Related: append([]diagnostic.Context{}, first.Related...)}
	before := diagnosticsJSON(t, owner)
	got := bindingError(owner, bindings[3], bindingContext(base, bindings[3]))
	diagnosticsRelated(t, got.Related, input.Path, []int{6, 1, 3})
	if !reflect.DeepEqual(got.Related[:2], owner.Related) || string(diagnosticsJSON(t, owner)) != string(before) {
		t.Fatal("known places or their order changed")
	}
}

func TestDiagnosticsBindingErrorAddsServiceAddressWithoutNodeText(t *testing.T) {
	input, bindings := diagnosticsBindings(t)
	base := inputContext(input, diagnostic.Context{FileName: "l017.json"})
	owner := &diagnostic.Error{Code: "data.failure", Reason: "Ошибка", Context: base}
	var details [][]diagnostic.Detail
	for _, index := range []int{1, 3, 2} {
		binding := bindings[index]
		got := bindingError(owner, binding, bindingContext(base, binding))
		if got.Context != base || got.Context.PageID != "" {
			t.Fatalf("service address replaced owner context or became PageID: %+v", got.Context)
		}
		if len(got.Details) < 2 {
			t.Fatal("page index and NodePath are missing from Details")
		}
		for _, detail := range got.Details {
			if detail.Label == "" || detail.Value == "" {
				t.Fatalf("unlabelled or empty service address: %+v", detail)
			}
		}
		if strings.Contains(string(diagnosticsJSON(t, got)), "FULL_NODE_TEXT_не_включать_в_диагностику") {
			t.Fatal("full node text leaked into diagnostic")
		}
		for _, location := range got.Related {
			if location.PageID != "" {
				t.Fatalf("service address became PageID: %+v", location)
			}
		}
		details = append(details, got.Details)
	}
	if reflect.DeepEqual(details[0], details[1]) || reflect.DeepEqual(details[1], details[2]) {
		t.Fatal("Details must distinguish page indexes and nested NodePaths")
	}
}

func diagnosticsBindings(t *testing.T) (*parsed.Input, []structure.Binding) {
	t.Helper()
	path, err := filepath.Abs("testdata/diagnostics/nested.parsed.json")
	if err != nil {
		t.Fatal(err)
	}
	var tree filetree.Resolver
	entry, problem := tree.JSONFile(path)
	if problem != nil {
		t.Fatal(problem)
	}
	input, problems := parsed.Load(entry.Path)
	if len(problems) != 0 {
		t.Fatalf("Load: %v", problems)
	}
	entries, problems := fragments.Expand(input)
	if len(problems) != 0 {
		t.Fatalf("Expand: %v", problems)
	}
	_, bindings, problems := structure.Build(input, entries)
	if len(problems) != 0 {
		t.Fatalf("structure.Build: %v", problems)
	}
	if len(bindings) != 4 || bindings[1].Page != 0 || bindings[3].Page != 1 || !reflect.DeepEqual(bindings[3].NodePath, []int{0, 0}) {
		t.Fatalf("unexpected fixture bindings: %+v", bindings)
	}
	return input, bindings
}

func diagnosticsRelated(t *testing.T, got []diagnostic.Context, path string, lines []int) {
	t.Helper()
	if len(got) != len(lines) {
		t.Fatalf("Related = %+v, want lines %v", got, lines)
	}
	for i, line := range lines {
		if got[i].Path != path || got[i].FileName != "l017.json" || got[i].LessonID != "MiXeD-7" || got[i].PageID != "" || got[i].Line != line {
			t.Errorf("Related[%d] = %+v, want source line %d with original JSON/filename/ID", i, got[i], line)
		}
	}
}

func diagnosticsJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
