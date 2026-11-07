package parsed

import (
	"encoding/json"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func validateDiagnosticsTestLocation(sl, sc, el, ec int) *model.Location {
	return &model.Location{Start: model.Position{Line: sl, Column: sc}, End: model.Position{Line: el, Column: ec}}
}

func validateDiagnosticsTestItem() model.Diagnostic {
	return model.Diagnostic{ID: "d1", Source: "run", DiagnosticCode: "future.code+1", SeverityLevel: model.SeverityWarning, DiagnosticScope: model.ScopeDocument, Message: "Свободное сообщение"}
}

func validateDiagnosticsTestSnapshot(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func validateDiagnosticsTestCheck(t *testing.T, input any, bad bool, run func(diagnostic.Context) []*diagnostic.Error) {
	t.Helper()
	context := diagnostic.Context{Path: `C:\Входные данные\lesson.json`, FileName: "l001.json", LessonID: "EfX7", PageID: "efx7-p001"}
	before := validateDiagnosticsTestSnapshot(t, input)
	var first string
	for i := 0; i < 8; i++ {
		errs := run(context)
		if (len(errs) > 0) != bad {
			t.Fatalf("errors = %v, want invalid=%v", errs, bad)
		}
		for _, err := range errs {
			if err == nil {
				t.Fatal("nil inside error slice")
			}
			if err.Code != CodeContract {
				t.Errorf("Code = %q, want %q", err.Code, CodeContract)
			}
			if err.Context.Path != context.Path || err.Context.FileName != context.FileName || err.Context.LessonID != context.LessonID || err.Context.PageID != context.PageID {
				t.Errorf("lost context: %+v", err.Context)
			}
			russian := false
			for _, r := range err.Reason {
				russian = russian || unicode.In(r, unicode.Cyrillic)
			}
			if !russian {
				t.Errorf("Reason must explain violation in Russian: %q", err.Reason)
			}
		}
		now := validateDiagnosticsTestSnapshot(t, errs)
		if i == 0 {
			first = now
		} else if now != first {
			t.Fatal("nondeterministic diagnostics")
		}
		if validateDiagnosticsTestSnapshot(t, input) != before {
			t.Fatal("input was modified")
		}
	}
}

func TestValidateDiagnosticsIndexIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []string
		bad  bool
	}{
		{"empty", nil, false}, {"preserve exact IDs", []string{"Z", "z", "ид", " "}, false},
		{"empty ID", []string{"x", ""}, true}, {"nonadjacent duplicate", []string{"x", "y", "x"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]model.Diagnostic, len(tc.ids))
			for i, id := range tc.ids {
				items[i].ID = id
			}
			validateDiagnosticsTestCheck(t, &items, tc.bad, func(c diagnostic.Context) []*diagnostic.Error {
				index, errs := indexDiagnostics(items, c)
				if !tc.bad {
					if len(index) != len(items) {
						t.Fatalf("index = %v", index)
					}
					for i, id := range tc.ids {
						if got, ok := index[id]; !ok || got != i {
							t.Errorf("index[%q] = %d, %v; want %d", id, got, ok, i)
						}
					}
				}
				return errs
			})
		})
	}
}

func TestValidateDiagnosticsIndividual(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.Diagnostic)
		bad    bool
	}{
		{"open code", func(d *model.Diagnostic) {}, false},
		{"error fatal", func(d *model.Diagnostic) { d.SeverityLevel = model.SeverityError; d.Fatal = true }, false},
		{"recommendation", func(d *model.Diagnostic) { d.SeverityLevel = model.SeverityRecommendation }, false},
		{"unknown severity", func(d *model.Diagnostic) { d.SeverityLevel = "Error" }, true},
		{"empty severity", func(d *model.Diagnostic) { d.SeverityLevel = "" }, true},
		{"fatal warning", func(d *model.Diagnostic) { d.Fatal = true }, true},
		{"fatal recommendation", func(d *model.Diagnostic) { d.SeverityLevel = model.SeverityRecommendation; d.Fatal = true }, true},
		{"unknown scope", func(d *model.Diagnostic) { d.DiagnosticScope = "other" }, true},
		{"empty scope", func(d *model.Diagnostic) { d.DiagnosticScope = "" }, true},
		{"missing source", func(d *model.Diagnostic) { d.Source = "other" }, true},
		{"empty source", func(d *model.Diagnostic) { d.Source = "" }, true},
		{"element needs location", func(d *model.Diagnostic) { d.DiagnosticScope = model.ScopeElement }, true},
		{"element empty range", func(d *model.Diagnostic) {
			d.DiagnosticScope = model.ScopeElement
			d.Location = validateDiagnosticsTestLocation(2, 3, 2, 3)
		}, false},
		{"line null", func(d *model.Diagnostic) { d.DiagnosticScope = model.ScopeLine }, false},
		{"line range", func(d *model.Diagnostic) {
			d.DiagnosticScope = model.ScopeLine
			d.Location = validateDiagnosticsTestLocation(2, 1, 2, 5)
		}, false},
		{"line spans lines", func(d *model.Diagnostic) {
			d.DiagnosticScope = model.ScopeLine
			d.Location = validateDiagnosticsTestLocation(2, 1, 3, 1)
		}, true},
		{"block null", func(d *model.Diagnostic) { d.DiagnosticScope = model.ScopeBlock }, false},
		{"block spans lines", func(d *model.Diagnostic) {
			d.DiagnosticScope = model.ScopeBlock
			d.Location = validateDiagnosticsTestLocation(2, 9, 5, 1)
		}, false},
		{"invalid main range", func(d *model.Diagnostic) { d.Location = validateDiagnosticsTestLocation(0, 1, 2, 1) }, true},
		{"invalid related range", func(d *model.Diagnostic) {
			d.RelatedLocations = []model.Location{*validateDiagnosticsTestLocation(1, 2, 1, 1)}
		}, true},
		{"related duplicates main", func(d *model.Diagnostic) {
			d.Location = validateDiagnosticsTestLocation(1, 1, 1, 2)
			d.RelatedLocations = []model.Location{*d.Location}
		}, true},
		{"related nonadjacent duplicate", func(d *model.Diagnostic) {
			a := *validateDiagnosticsTestLocation(1, 1, 1, 2)
			b := *validateDiagnosticsTestLocation(3, 1, 3, 2)
			d.RelatedLocations = []model.Location{a, b, a}
		}, true},
		{"overlap is not equality", func(d *model.Diagnostic) {
			d.Location = validateDiagnosticsTestLocation(1, 1, 1, 3)
			d.RelatedLocations = []model.Location{*validateDiagnosticsTestLocation(1, 1, 1, 2), *validateDiagnosticsTestLocation(1, 2, 1, 4)}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validateDiagnosticsTestItem()
			tc.change(&d)
			processing := map[string]int{"run": 0}
			input := struct {
				Item       *model.Diagnostic
				Processing map[string]int
			}{&d, processing}
			validateDiagnosticsTestCheck(t, input, tc.bad, func(c diagnostic.Context) []*diagnostic.Error { return validateDiagnostic(d, processing, c) })
		})
	}
}

func TestValidateDiagnosticsRanges(t *testing.T) {
	for _, tc := range []struct {
		name           string
		sl, sc, el, ec int
		bad            bool
	}{
		{"empty", 1, 1, 1, 1, false}, {"exclusive end", 2, 3, 2, 4, false},
		{"multiline smaller column", 2, 99, 3, 1, false}, {"start line zero", 0, 1, 1, 1, true},
		{"start line negative", -1, 1, 1, 1, true}, {"start column zero", 1, 0, 1, 1, true},
		{"start column negative", 1, -1, 1, 1, true}, {"end line zero", 1, 1, 0, 1, true},
		{"end column zero", 1, 1, 2, 0, true}, {"end column negative", 1, 1, 2, -1, true},
		{"backward lines", 3, 1, 2, 99, true}, {"backward columns", 2, 4, 2, 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			location := validateDiagnosticsTestLocation(tc.sl, tc.sc, tc.el, tc.ec)
			validateDiagnosticsTestCheck(t, location, tc.bad, func(c diagnostic.Context) []*diagnostic.Error {
				err := validateLocation(*location, c)
				if err == nil {
					return nil
				}
				return []*diagnostic.Error{err}
			})
		})
	}
}

func TestValidateDiagnosticsOrder(t *testing.T) {
	item := func(source string, line, column int) model.Diagnostic {
		d := validateDiagnosticsTestItem()
		d.Source = source
		if line > 0 {
			d.Location = validateDiagnosticsTestLocation(line, column, line, column+1)
		}
		return d
	}
	a, b := "z-first", "a-next"
	for _, tc := range []struct {
		name  string
		items []model.Diagnostic
		bad   bool
	}{
		{"empty", nil, false}, {"processing beats coordinates", []model.Diagnostic{item(a, 9, 9), item(b, 1, 1)}, false},
		{"processing inversion", []model.Diagnostic{item(b, 1, 1), item(a, 9, 9)}, true},
		{"line order", []model.Diagnostic{item(a, 1, 9), item(a, 2, 1)}, false},
		{"line inversion", []model.Diagnostic{item(a, 2, 1), item(a, 1, 9)}, true},
		{"column inversion", []model.Diagnostic{item(a, 2, 3), item(a, 2, 2)}, true},
		{"equal starts", []model.Diagnostic{item(a, 2, 3), item(a, 2, 3)}, false},
		{"column order", []model.Diagnostic{item(a, 2, 2), item(a, 2, 3)}, false},
		{"null before", []model.Diagnostic{item(a, 0, 0), item(a, 1, 1)}, false},
		{"null after", []model.Diagnostic{item(a, 1, 1), item(a, 0, 0)}, false},
		{"null between", []model.Diagnostic{item(a, 1, 9), item(a, 0, 0), item(a, 2, 1)}, false},
		{"inversion across null", []model.Diagnostic{item(a, 2, 1), item(a, 0, 0), item(a, 1, 9)}, true},
		{"known sources with null", []model.Diagnostic{item(b, 0, 0), item(a, 0, 0)}, true},
		{"unknown source alone", []model.Diagnostic{item("unknown", 1, 1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.items {
				tc.items[i].ID = string(rune('z' - i))
			}
			processing := map[string]int{a: 0, b: 1}
			input := struct {
				Items      *[]model.Diagnostic
				Processing map[string]int
			}{&tc.items, processing}
			validateDiagnosticsTestCheck(t, input, tc.bad, func(c diagnostic.Context) []*diagnostic.Error {
				if err := validateDiagnosticOrder(tc.items, processing, c); err != nil {
					return []*diagnostic.Error{err}
				}
				return nil
			})
		})
	}
}

func validateDiagnosticsTestReferences() *model.Result {
	d := validateDiagnosticsTestItem()
	d.DiagnosticScope = model.ScopeElement
	d.Location = validateDiagnosticsTestLocation(2, 3, 2, 5)
	e := d
	e.ID = "d2"
	e.SeverityLevel = model.SeverityRecommendation
	return &model.Result{Diagnostics: []model.Diagnostic{d, e}, Lines: []model.Line{{Number: 2, Elements: []model.Element{{Start: 3, End: 5, ErrorIDs: []string{"d1", "d2"}}}}}}
}

func TestValidateDiagnosticsReferences(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.Result)
		bad    bool
	}{
		{"warning recommendation references", func(r *model.Result) {}, false},
		{"empty", func(r *model.Result) { r.Diagnostics = nil; r.Lines = nil }, false},
		{"duplicate reference", func(r *model.Result) { r.Lines[0].Elements[0].ErrorIDs = []string{"d1", "d2", "d1"} }, true},
		{"missing diagnostic", func(r *model.Result) {
			r.Lines[0].Elements[0].ErrorIDs = append(r.Lines[0].Elements[0].ErrorIDs, "absent")
		}, true},
		{"reference order", func(r *model.Result) { r.Lines[0].Elements[0].ErrorIDs = []string{"d2", "d1"} }, true},
		{"missing reverse link", func(r *model.Result) { r.Lines[0].Elements[0].ErrorIDs = []string{"d1"} }, true},
		{"reverse wrong line", func(r *model.Result) { r.Lines[0].Number = 1 }, true},
		{"reverse wrong start", func(r *model.Result) { r.Lines[0].Elements[0].Start = 2 }, true},
		{"reverse wrong exclusive end", func(r *model.Result) { r.Lines[0].Elements[0].End = 4 }, true},
		{"reverse missing location", func(r *model.Result) { r.Diagnostics[0].Location = nil }, true},
		{"reverse multiline", func(r *model.Result) { r.Diagnostics[0].Location.End.Line = 3 }, true},
		{"one exact reverse link suffices", func(r *model.Result) {
			r.Lines = append(r.Lines, model.Line{Number: 4, Elements: []model.Element{{Start: 1, End: 2, ErrorIDs: []string{"d1"}}}})
		}, false},
		{"empty insertion range", func(r *model.Result) {
			for i := range r.Diagnostics {
				r.Diagnostics[i].Location.End.Column = 3
			}
			r.Lines[0].Elements[0].End = 3
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validateDiagnosticsTestReferences()
			tc.change(r)
			index := make(map[string]int)
			for i, d := range r.Diagnostics {
				index[d.ID] = i
			}
			input := struct {
				Result *model.Result
				Index  map[string]int
			}{r, index}
			validateDiagnosticsTestCheck(t, input, tc.bad, func(c diagnostic.Context) []*diagnostic.Error { return validateErrorReferences(r, index, c) })
		})
	}
	for _, scope := range []model.Scope{model.ScopeLine, model.ScopeBlock, model.ScopeDocument} {
		t.Run(string(scope), func(t *testing.T) {
			r := validateDiagnosticsTestReferences()
			r.Diagnostics[0].DiagnosticScope = scope
			validateDiagnosticsTestCheck(t, r, true, func(c diagnostic.Context) []*diagnostic.Error {
				return validateErrorReferences(r, map[string]int{"d1": 0, "d2": 1}, c)
			})
			r.Lines[0].Elements[0].ErrorIDs = []string{"d2"}
			validateDiagnosticsTestCheck(t, r, false, func(c diagnostic.Context) []*diagnostic.Error {
				return validateErrorReferences(r, map[string]int{"d1": 0, "d2": 1}, c)
			})
		})
	}
}

func TestValidateDiagnosticsErrorFlags(t *testing.T) {
	for _, tc := range []struct {
		name                string
		severity            model.Severity
		doc, first, second  bool
		location, reference bool
		bad                 bool
	}{
		{"warning clear", model.SeverityWarning, false, false, false, true, true, false},
		{"recommendation clear", model.SeverityRecommendation, false, false, false, true, true, false},
		{"error own start only", model.SeverityError, true, true, false, true, false, false},
		{"reference marks other line", model.SeverityError, true, true, true, true, true, false},
		{"error null location", model.SeverityError, true, false, false, false, false, false},
		{"null with reference", model.SeverityError, true, false, true, false, true, false},
		{"missing document flag", model.SeverityError, false, true, false, true, false, true},
		{"spurious document flag", model.SeverityWarning, true, false, false, false, false, true},
		{"missing own line flag", model.SeverityError, true, false, false, true, false, true},
		{"missing reference flag", model.SeverityError, true, true, false, true, true, true},
		{"spurious line flag", model.SeverityRecommendation, false, true, false, true, false, true},
		{"range end not marked", model.SeverityError, true, true, true, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validateDiagnosticsTestItem()
			d.SeverityLevel = tc.severity
			if tc.location {
				d.Location = validateDiagnosticsTestLocation(2, 3, 3, 1)
			}
			parent := 1
			r := &model.Result{Document: model.Document{HasErrors: tc.doc}, Diagnostics: []model.Diagnostic{d}, Lines: []model.Line{{Number: 1}, {Number: 2, ParentLine: &parent, NestingLevel: 1, HasErrors: tc.first}, {Number: 3, HasErrors: tc.second}}}
			if tc.reference {
				r.Lines[2].Elements = []model.Element{{ErrorIDs: []string{"d1"}}}
			}
			validateDiagnosticsTestCheck(t, r, tc.bad, func(c diagnostic.Context) []*diagnostic.Error { return validateErrorFlags(r, c) })
		})
	}
	for _, tc := range []struct {
		name           string
		doc, line, bad bool
	}{
		{"no diagnostics", false, false, false}, {"document alone", true, false, true}, {"line alone", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &model.Result{Document: model.Document{HasErrors: tc.doc}, Lines: []model.Line{{Number: 1, HasErrors: tc.line, LineType: model.LineTypeInvalid, Elements: []model.Element{{ElementType: model.ElementTypeUnparsed}}}}}
			validateDiagnosticsTestCheck(t, r, tc.bad, func(c diagnostic.Context) []*diagnostic.Error { return validateErrorFlags(r, c) })
		})
	}
}
