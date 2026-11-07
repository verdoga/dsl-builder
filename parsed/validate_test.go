package parsed

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/order"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func validateTestString(value string) *string { return &value }
func validateTestInt(value int) *int          { return &value }
func validateTestInt64(value int64) *int64    { return &value }

func validateTestSnapshot(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func validateTestErrors(t *testing.T, items []*diagnostic.Error, codes []string, context diagnostic.Context) {
	t.Helper()
	if len(items) != len(codes) {
		t.Fatalf("ошибки: %+v; нужны коды %v", items, codes)
	}
	for i, item := range items {
		if item == nil || item.Code != codes[i] || item.Reason == "" {
			t.Fatalf("ошибка %d = %+v; нужен %s", i, item, codes[i])
		}
		if item.Context.Path != context.Path || item.Context.LessonID != context.LessonID {
			t.Errorf("контекст ошибки %d = %+v", i, item.Context)
		}
	}
}

func validateTestField(t *testing.T, item *diagnostic.Error, field string) {
	t.Helper()
	for _, detail := range item.Details {
		if detail.Value == field {
			return
		}
	}
	t.Fatalf("нет поля %s: %+v", field, item)
}

func validateTestDependency(t *testing.T, items []*diagnostic.Error, want *diagnostic.Error, context diagnostic.Context) {
	t.Helper()
	if want == nil {
		if items != nil {
			t.Fatal(items)
		}
		return
	}
	validateTestErrors(t, items, []string{want.Code}, context)
	if items[0].Context != context || items[0].Reason != want.Reason {
		t.Fatalf("ошибка зависимости изменена: %+v", items[0])
	}
	for _, detail := range want.Details {
		if !slices.Contains(items[0].Details, detail) {
			t.Fatalf("потеряна подробность зависимости: %+v", detail)
		}
	}
}

func validateTestResult() model.Result {
	return model.Result{Format: model.FormatVersion1,
		Document:   model.Document{LineCount: validateTestInt(3), Metadata: model.DocumentMetadata{DocumentID: validateTestString("Урок+A:B")}},
		Processing: []model.Processing{{ID: "run", Tool: "новый-инструмент"}},
		Lines: []model.Line{
			{Number: 1, LineType: model.LineTypeBlank, LineEnding: model.LineEndingLF, Elements: []model.Element{}},
			{Number: 2, LineType: model.LineTypeContent, ParentLine: validateTestInt(1), NestingLevel: 1, Raw: "Я", LineEnding: model.LineEndingLF,
				Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: "Я", Value: validateTestString("Я"), Start: 1, End: 2, ErrorIDs: []string{}}}},
			{Number: 3, LineType: model.LineTypeBlank, Elements: []model.Element{}},
		}, Diagnostics: []model.Diagnostic{}}
}

func TestValidateContractIndicesAndPreservation(t *testing.T) {
	result := validateTestResult()
	before := validateTestSnapshot(t, result)
	index, errs := validateContract(&result, diagnostic.Context{Path: `C:\JSON\Вход.json`})
	if errs != nil {
		t.Fatalf("валидный вход: %v", errs)
	}
	if !maps.Equal(index.ByNumber, map[int]int{1: 0, 2: 1, 3: 2}) || len(index.Children) != 2 ||
		!slices.Equal(index.Children[0], []int{1, 3}) || !slices.Equal(index.Children[1], []int{2}) {
		t.Fatalf("индекс = %+v", index)
	}
	if after := validateTestSnapshot(t, result); after != before {
		t.Fatal("валидация изменила вход")
	}
	result = model.Result{Format: model.FormatVersion1, Document: model.Document{LineCount: validateTestInt(0), Metadata: model.DocumentMetadata{DocumentID: validateTestString("Empty")}}, Lines: []model.Line{}, Processing: []model.Processing{}, Diagnostics: []model.Diagnostic{}}
	index, errs = validateContract(&result, diagnostic.Context{})
	if errs != nil || len(index.ByNumber) != 0 || len(index.Children) != 0 {
		t.Fatalf("пустой документ: %+v, %v", index, errs)
	}
}

func TestValidateContractIndependentChecksAndNoPartialIndex(t *testing.T) {
	context := diagnostic.Context{Path: `C:\JSON\Вход.json`, LessonID: "Урок+A:B"}
	cases := []struct {
		name   string
		change func(*model.Result)
		code   string
	}{
		{"document", func(r *model.Result) { r.Document.Encoding = validateTestString("UTF-16") }, CodeContract},
		{"metadata", func(r *model.Result) { r.Document.Metadata.DocumentID = nil }, CodeMissingID},
		{"processing", func(r *model.Result) { r.Processing[0].DurationMs = validateTestInt(-1) }, CodeContract},
		{"line", func(r *model.Result) { r.Lines[0].LineType = "unknown" }, CodeContract},
		{"number", func(r *model.Result) { r.Lines[1].Number = 1 }, CodeContract},
		{"diagnostic", func(r *model.Result) {
			r.Diagnostics = []model.Diagnostic{{ID: "d", Source: "missing", SeverityLevel: model.SeverityWarning, DiagnosticScope: model.ScopeDocument}}
		}, CodeContract},
		{"reference", func(r *model.Result) { r.Lines[1].Elements[0].ErrorIDs = []string{"missing"} }, CodeContract},
		{"flags", func(r *model.Result) { r.Document.HasErrors = true }, CodeContract},
		{"parent", func(r *model.Result) { r.Lines[1].ParentLine = validateTestInt(99) }, CodeParent},
		{"cycle", func(r *model.Result) { r.Lines[0].ParentLine = validateTestInt(2); r.Lines[0].NestingLevel = 2 }, CodeParentCycle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validateTestResult()
			tc.change(&r)
			before := validateTestSnapshot(t, r)
			index, errs := validateContract(&r, context)
			if index.ByNumber != nil || index.Children != nil {
				t.Fatalf("частичный индекс при ошибке: %+v", index)
			}
			found := false
			for _, err := range errs {
				if err != nil && err.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("нет %s: %v", tc.code, errs)
			}
			if validateTestSnapshot(t, r) != before {
				t.Fatal("ошибочная проверка изменила вход")
			}
		})
	}
}

func TestValidateContractErrorOrder(t *testing.T) {
	r := validateTestResult()
	r.Document.Encoding = validateTestString("wrong")
	r.Document.Metadata.Order = validateTestString("00")
	r.Processing[0].DurationMs = validateTestInt(-1)
	r.Lines[0].LineType = "unknown"
	r.Diagnostics = []model.Diagnostic{{ID: "d", Source: "missing", SeverityLevel: model.SeverityWarning, DiagnosticScope: model.ScopeDocument}}
	context := diagnostic.Context{Path: "input.json", LessonID: "Урок+A:B"}
	_, errs := validateContract(&r, context)
	validateTestErrors(t, errs, []string{CodeContract, order.CodeInvalidWeight, CodeContract, CodeContract, CodeContract}, context)
	validateTestField(t, errs[0], "document.encoding")
	validateTestField(t, errs[2], "processing[0].durationMs")
	if errs[3].Context.Line != 1 || errs[4].Context.Line != 0 {
		t.Fatalf("порядок строк и диагностик: %+v", errs)
	}
	before := validateTestSnapshot(t, errs)
	for range 20 {
		_, again := validateContract(&r, context)
		if validateTestSnapshot(t, again) != before {
			t.Fatal("нестабильный порядок ошибок")
		}
	}
}

func TestValidateDocumentKnownAndUnknownValues(t *testing.T) {
	context := diagnostic.Context{Path: "input.json", LessonID: "MiXeD"}
	valid := []model.Document{{}, {DSLVersion: validateTestString("future-DSL"), LineCount: validateTestInt(0), ByteLength: validateTestInt64(0), Encoding: validateTestString("UTF-8"), SHA256: validateTestString(strings.Repeat("0123456789abcdef", 4))}}
	for i, d := range valid {
		if errs := validateDocument(d, context); errs != nil {
			t.Fatalf("допустимый %d: %v", i, errs)
		}
	}
	cases := []struct {
		name     string
		document model.Document
		field    string
	}{
		{"lineCount", model.Document{LineCount: validateTestInt(-1)}, "document.lineCount"},
		{"byteLength", model.Document{ByteLength: validateTestInt64(-1)}, "document.byteLength"},
		{"encoding", model.Document{Encoding: validateTestString("utf-8")}, "document.encoding"},
		{"emptySHA", model.Document{SHA256: validateTestString("")}, "document.sha256"},
		{"shortSHA", model.Document{SHA256: validateTestString(strings.Repeat("a", 63))}, "document.sha256"},
		{"longSHA", model.Document{SHA256: validateTestString(strings.Repeat("a", 65))}, "document.sha256"},
		{"upperSHA", model.Document{SHA256: validateTestString(strings.Repeat("A", 64))}, "document.sha256"},
		{"nonhexSHA", model.Document{SHA256: validateTestString(strings.Repeat("g", 64))}, "document.sha256"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateDocument(tc.document, context)
			validateTestErrors(t, errs, []string{CodeContract}, context)
			validateTestField(t, errs[0], tc.field)
		})
	}
	d := model.Document{Encoding: validateTestString("bad"), LineCount: validateTestInt(-1), ByteLength: validateTestInt64(-1), SHA256: validateTestString("bad")}
	errs := validateDocument(d, context)
	validateTestErrors(t, errs, []string{CodeContract, CodeContract, CodeContract, CodeContract}, context)
	for i, field := range []string{"document.encoding", "document.lineCount", "document.byteLength", "document.sha256"} {
		validateTestField(t, errs[i], field)
	}
}

func TestValidateSourceNameWindowsInvariantOnly(t *testing.T) {
	context := diagnostic.Context{Path: `C:\JSON\технический.json`, LessonID: "ID"}
	cases := []struct {
		name, path, file string
		valid            bool
	}{
		{"drive", `Z:\Нет такого каталога\Урок ①.txt`, "Урок ①.txt", true},
		{"UNC", `\\server\share\Нет каталога\Урок.txt`, "Урок.txt", true},
		{"slash", `C:/Нет каталога/Урок.txt`, "Урок.txt", true},
		{"reservedName", `C:\Нет каталога\CON.txt`, "CON.txt", true},
		{"notOutputName", `C:\Нет каталога\lesson?.txt`, "lesson?.txt", true},
		{"driveRelative", `C:Урок.txt`, "Урок.txt", false},
		{"relative", `dir\Урок.txt`, "Урок.txt", false},
		{"rootRelative", `\dir\Урок.txt`, "Урок.txt", false},
		{"wrongBase", `C:\dir\Другой.txt`, "Урок.txt", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := model.Document{FilePath: validateTestString(tc.path), FileName: validateTestString(tc.file)}
			before := validateTestSnapshot(t, d)
			err := validateSourceName(d, context)
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid {
				validateTestErrors(t, []*diagnostic.Error{err}, []string{CodeContract}, context)
			}
			if validateTestSnapshot(t, d) != before {
				t.Fatal("путь изменён")
			}
		})
	}
	for _, d := range []model.Document{{}, {FileName: validateTestString("arbitrary:name?.txt")}} {
		if err := validateSourceName(d, context); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidateMetadataDelegatesAndPreserves(t *testing.T) {
	context := diagnostic.Context{Path: `C:\JSON\Вход.json`, FileName: "l001.json", LessonID: "Known", PageID: "page", Line: 7, Column: 3}
	if errs := validateMetadata(model.DocumentMetadata{}, context); len(errs) != 1 || errs[0].Code != CodeMissingID {
		t.Fatalf("null ID: %v", errs)
	}
	for _, id := range []string{"A:B", "lesson+1", "СмеШаНный①", "A\u0085B", "", "bad id", "bad\u00a0id"} {
		t.Run("ID="+id, func(t *testing.T) {
			m := model.DocumentMetadata{DocumentID: validateTestString(id)}
			before := validateTestSnapshot(t, m)
			errs := validateMetadata(m, context)
			want := identity.ValidateLessonID(id)
			validateTestDependency(t, errs, want, context)
			if validateTestSnapshot(t, m) != before {
				t.Fatal("ID изменён")
			}
		})
	}
	for _, raw := range []string{"0001", strings.Repeat("9", 200), "", "000", "-1", "1.0", " 1", "١"} {
		t.Run("order="+raw, func(t *testing.T) {
			m := model.DocumentMetadata{DocumentID: validateTestString("ID"), Order: validateTestString(raw)}
			before := validateTestSnapshot(t, m)
			errs := validateMetadata(m, context)
			want := order.Validate(raw, context)
			validateTestDependency(t, errs, want, context)
			if validateTestSnapshot(t, m) != before {
				t.Fatal("вес изменён")
			}
		})
	}
	for _, dirs := range [][]string{nil, {}, {`missing\relative`, `Z:\not-existing`, "", `missing\relative`}} {
		m := model.DocumentMetadata{DocumentID: validateTestString("MiXeD"), Title: validateTestString("  Заголовок\t\n"), Subtitle: validateTestString(""), Section: validateTestString(" Свободная\nсекция "), ResourceDirs: dirs}
		before := validateTestSnapshot(t, m)
		if errs := validateMetadata(m, context); errs != nil {
			t.Fatal(errs)
		}
		if validateTestSnapshot(t, m) != before {
			t.Fatal("метаданные изменены")
		}
	}
}

func TestValidateProcessingRulesAndArrayOrder(t *testing.T) {
	context := diagnostic.Context{Path: "input.json", LessonID: "ID"}
	items := []model.Processing{
		{ID: "p", Tool: "future/tool", Version: validateTestString("version любую"), StartedAt: validateTestString("2026-11-06T12:00:00.123+03:00"), DurationMs: validateTestInt(0)},
		{ID: "P", Tool: "другой", Version: validateTestString(""), StartedAt: validateTestString("2020-01-01T00:00:00Z")},
		{ID: "nulls", Tool: "новый-инструмент"},
	}
	before := validateTestSnapshot(t, items)
	index, errs := validateProcessing(items, context)
	if errs != nil || !maps.Equal(index, map[string]int{"p": 0, "P": 1, "nulls": 2}) {
		t.Fatalf("processing: %v, %v", index, errs)
	}
	if validateTestSnapshot(t, items) != before {
		t.Fatal("processing пересортирован или изменён")
	}
	for _, items := range [][]model.Processing{nil, {}} {
		index, errs := validateProcessing(items, context)
		if errs != nil || len(index) != 0 {
			t.Fatalf("пустой processing: %v %v", index, errs)
		}
	}
	cases := []struct {
		name  string
		items []model.Processing
	}{
		{"emptyID", []model.Processing{{ID: ""}}},
		{"duplicateID", []model.Processing{{ID: "p"}, {ID: "p"}}},
		{"negativeDuration", []model.Processing{{ID: "p", DurationMs: validateTestInt(-1)}}},
		{"missingZone", []model.Processing{{ID: "p", StartedAt: validateTestString("2026-11-06T12:00:00")}}},
		{"invalidDate", []model.Processing{{ID: "p", StartedAt: validateTestString("2026-02-30T12:00:00Z")}}},
		{"emptyDate", []model.Processing{{ID: "p", StartedAt: validateTestString("")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := validateProcessing(tc.items, context)
			validateTestErrors(t, errs, []string{CodeContract}, context)
		})
	}
	items = []model.Processing{{StartedAt: validateTestString("bad"), DurationMs: validateTestInt(-1)}, {StartedAt: validateTestString("bad"), DurationMs: validateTestInt(-1)}}
	_, errs = validateProcessing(items, context)
	fields := []string{"processing[0].id", "processing[0].startedAt", "processing[0].durationMs", "processing[1].id", "processing[1].startedAt", "processing[1].durationMs"}
	validateTestErrors(t, errs, []string{CodeContract, CodeContract, CodeContract, CodeContract, CodeContract, CodeContract}, context)
	for i, field := range fields {
		validateTestField(t, errs[i], field)
	}
}
