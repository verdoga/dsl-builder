package parsed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// processingTimeRegressionTestString сохраняет исходное написание времени.
func processingTimeRegressionTestString(value string) *string { return &value }

// processingTimeRegressionTestSnapshot фиксирует все поля без нормализации времени.
func processingTimeRegressionTestSnapshot(t *testing.T, value []model.Processing) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// processingTimeRegressionTestLoad создаёт настоящий полный Contract 1.0.
func processingTimeRegressionTestLoad(t *testing.T, processing []model.Processing) (*Input, []*diagnostic.Error, string) {
	t.Helper()
	id := "MiXeD:Урок+43"
	count := 0
	result := model.Result{
		Format: model.FormatVersion1,
		Document: model.Document{
			LineCount: &count,
			Metadata:  model.DocumentMetadata{DocumentID: &id, ResourceDirs: []string{}},
		},
		Processing:  processing,
		Lines:       []model.Line{},
		Diagnostics: []model.Diagnostic{},
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "время обработки.json")
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	input, errs := Load(path)
	return input, errs, path
}

// processingTimeRegressionTestError проверяет машинный код, полное место и поле.
func processingTimeRegressionTestError(t *testing.T, errs []*diagnostic.Error, context diagnostic.Context, field string) {
	t.Helper()
	if len(errs) != 1 || errs[0] == nil {
		t.Fatalf("нужна одна ошибка времени, получено: %+v", errs)
	}
	item := errs[0]
	if item.Code != CodeContract || item.Context != context {
		t.Errorf("ошибка = %+v; нужны Code=%q и Context=%+v", item, CodeContract, context)
	}
	for _, detail := range item.Details {
		if detail.Value == field {
			return
		}
	}
	t.Errorf("поле %q отсутствует в Details: %+v", field, item.Details)
}

func TestProcessingTimeRegressionRFC3339AcceptedAndPreserved(t *testing.T) {
	// RFC 3339 §§5.6–5.8, Contract.startedAt и S36-1 задают ожидания.
	// time.Parse не служит эталоном полноты стандарта (time/format.go:41–43).
	cases := []struct {
		name  string
		stamp *string
	}{
		{"S36_1_lower_t_and_z", processingTimeRegressionTestString("2026-11-06t12:34:56z")},
		{"S36_1_lower_t", processingTimeRegressionTestString("2026-11-06t12:34:56+00:00")},
		{"S36_1_lower_z", processingTimeRegressionTestString("2026-11-06T12:34:56z")},
		{"S36_1_leap_1990", processingTimeRegressionTestString("1990-12-31T23:59:60Z")},
		{"S36_1_leap_2016", processingTimeRegressionTestString("2016-12-31T23:59:60Z")},
		{"ordinary_UTC", processingTimeRegressionTestString("2026-11-06T12:34:56Z")},
		{"null", nil},
		{"positive_offset", processingTimeRegressionTestString("2026-09-16T14:25:30.123+03:00")},
		{"negative_offset", processingTimeRegressionTestString("2026-11-06T12:34:56-08:30")},
		{"zero_offset", processingTimeRegressionTestString("2026-11-06T12:34:56+00:00")},
		{"unknown_local_offset", processingTimeRegressionTestString("2026-11-06T12:34:56-00:00")},
		{"fraction_leading_trailing_zeros", processingTimeRegressionTestString("2026-11-06T12:34:56.0012300Z")},
		{"fraction_beyond_nanoseconds", processingTimeRegressionTestString("2026-11-06T12:34:56.12345678901234567890Z")},
		{"ordinary_leap_day", processingTimeRegressionTestString("2024-02-29T00:00:00Z")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Времена намеренно идут не по хронологии и не по ID.
			items := []model.Processing{
				{ID: "z-last-by-name", Tool: "future/tool", StartedAt: processingTimeRegressionTestString("2030-01-01T00:00:00Z")},
				{ID: "middle", Tool: "свободный-инструмент", StartedAt: tc.stamp},
				{ID: "a-first-by-name", Tool: "another/tool", StartedAt: processingTimeRegressionTestString("1980-01-01T00:00:00Z")},
			}
			before := processingTimeRegressionTestSnapshot(t, items)
			context := diagnostic.Context{Path: `C:\Вход\технический.json`, FileName: "l043.json", LessonID: "MiXeD:Урок+43", PageID: "page", Line: 7, Column: 3}
			index, errs := validateProcessing(items, context)
			if errs != nil {
				t.Errorf("RFC3339 значение отвергнуто direct API: %v", errs)
			} else {
				if len(index) != len(items) {
					t.Errorf("индекс = %v", index)
				}
				for i, item := range items {
					if position, ok := index[item.ID]; !ok || position != i {
						t.Errorf("индекс %q = %d, %v; нужен %d", item.ID, position, ok, i)
					}
				}
			}
			if after := processingTimeRegressionTestSnapshot(t, items); after != before {
				t.Fatal("direct API изменил processing или исходные строки")
			}
			input, loadErrors, path := processingTimeRegressionTestLoad(t, items)
			if input == nil || loadErrors != nil {
				t.Fatalf("Load: Input=%+v, ошибки=%v", input, loadErrors)
			}
			if input.Path != path || input.Result.Document.Metadata.DocumentID == nil || *input.Result.Document.Metadata.DocumentID != context.LessonID {
				t.Errorf("Load потерял исходные путь/ID: %+v", input)
			}
			if after := processingTimeRegressionTestSnapshot(t, input.Result.Processing); after != before {
				t.Errorf("Load изменил строки/null/порядок processing:\n%s\nнужно:\n%s", after, before)
			}
		})
	}
}

func TestProcessingTimeRegressionRFC3339InvalidBlocked(t *testing.T) {
	cases := []struct{ name, stamp string }{
		{"missing_zone", "2026-11-06T12:34:56"},
		{"positive_offset_hour_24", "2026-11-06T12:34:56+24:00"},
		{"negative_offset_hour_24", "2026-11-06T12:34:56-24:00"},
		{"offset_minute_60", "2026-11-06T12:34:56+03:60"},
		{"non_leap_february_29", "2025-02-29T12:34:56Z"},
		{"april_31", "2026-04-31T12:34:56Z"},
		{"month_13", "2026-13-01T12:34:56Z"},
		{"day_zero", "2026-11-00T12:34:56Z"},
		{"seconds_61", "2016-12-31T23:59:61Z"},
		{"hour_24", "2026-11-06T24:00:00Z"},
		{"minute_60", "2026-11-06T12:60:00Z"},
		{"one_digit_hour", "2026-11-06T2:34:56Z"},
		{"missing_seconds", "2026-11-06T12:34Z"},
		{"comma_fraction", "2026-11-06T12:34:56,123Z"},
		{"empty_fraction", "2026-11-06T12:34:56.Z"},
		{"zone_without_colon", "2026-11-06T12:34:56+0300"},
		{"zone_without_minutes", "2026-11-06T12:34:56+03"},
		{"zone_name", "2026-11-06T12:34:56UTC"},
		{"leading_space", " 2026-11-06T12:34:56Z"},
		{"trailing_space", "2026-11-06T12:34:56Z "},
		{"trailing_text", "2026-11-06T12:34:56Zjunk"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := []model.Processing{
				{ID: "known-null", Tool: "parser"},
				{ID: "bad-time", Tool: "validator", StartedAt: &tc.stamp},
				{ID: "known-valid", Tool: "future/tool", StartedAt: processingTimeRegressionTestString("2026-11-06T12:34:56Z")},
			}
			before := processingTimeRegressionTestSnapshot(t, items)
			context := diagnostic.Context{Path: `C:\Вход\технический.json`, FileName: "l043.json", LessonID: "MiXeD:Урок+43", PageID: "known-page", Line: 9, Column: 4}
			_, errs := validateProcessing(items, context)
			processingTimeRegressionTestError(t, errs, context, "processing[1].startedAt")
			if after := processingTimeRegressionTestSnapshot(t, items); after != before {
				t.Fatal("невалидный processing был изменён")
			}
			input, loadErrors, path := processingTimeRegressionTestLoad(t, items)
			if input != nil {
				t.Errorf("Load вернул частичный Input: %+v", input)
			}
			processingTimeRegressionTestError(t, loadErrors, diagnostic.Context{Path: path, LessonID: context.LessonID}, "processing[1].startedAt")
		})
	}
}

func TestProcessingTimeRegressionInvalidHistoryErrorOrder(t *testing.T) {
	items := []model.Processing{
		{ID: "later", Tool: "parser", StartedAt: processingTimeRegressionTestString("2030-01-01T00:00:00")},
		{ID: "earlier", Tool: "validator", StartedAt: processingTimeRegressionTestString("1980-01-01T00:00:61Z")},
	}
	input, errs, path := processingTimeRegressionTestLoad(t, items)
	if input != nil || len(errs) != 2 {
		t.Fatalf("нужны nil Input и две ошибки: Input=%+v, ошибки=%v", input, errs)
	}
	for i, item := range errs {
		processingTimeRegressionTestError(t, []*diagnostic.Error{item}, diagnostic.Context{Path: path, LessonID: "MiXeD:Урок+43"}, fmt.Sprintf("processing[%d].startedAt", i))
	}
}
