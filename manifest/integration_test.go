package manifest_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/order"
)

func TestIntegrationContractRepositoryExamples(t *testing.T) {
	for _, name := range []string{"Manifest flat example.json", "Manifest unit example.json"} {
		t.Run(name, func(t *testing.T) {
			input, err := os.ReadFile("../" + name)
			if err != nil {
				t.Fatal(err)
			}
			value := integrationContractDecode(t, input)
			integrationContractRoundTrip(t, value)
			if name == "Manifest flat example.json" {
				if value.Course.Unit != nil || value.Course.Lessons == nil || len(*value.Course.Lessons) != 1 || (*value.Course.Lessons)[0].ID != "DEMO-INTRO" {
					t.Fatalf("потеряна плоская форма примера: %+v", value.Course)
				}
			} else if value.Course.Unit == nil || value.Course.Lessons != nil || value.Course.Sections != nil || len(value.Course.Unit.Sections) != 3 || value.Course.Unit.Sections[2].Lessons[1].ID != "NRUO" {
				t.Fatalf("потеряны форма или порядок unit-примера: %+v", value.Course)
			}
		})
	}
}

func TestIntegrationContractOperationSnapshots(t *testing.T) {
	initial := integrationContractEntry("MiXeD:A+1", "l001.json", "0001")
	updated := initial
	updated.Version, updated.LineCount, updated.CreatedAt = "2", "90071992547409931234567890", "2026-11-05T12:34:56Z"
	added := integrationContractEntry("Новый+урок", "l003.json", "0002")
	subtitle := "Подзаголовок\nс переносом"
	added.Subtitle = &subtitle
	// Снимки уже подготовлены владельцами операций; manifest их только проверяет.
	for _, tc := range []struct {
		name     string
		lessons  []manifest.Entry
		sections []manifest.Section
	}{
		{"creation", []manifest.Entry{initial}, []manifest.Section{}},
		{"update_text_page_or_pages_with_retained_title", []manifest.Entry{updated}, []manifest.Section{}},
		{"addition_and_sections", []manifest.Entry{}, []manifest.Section{{Title: "Основное", Lessons: []manifest.Entry{initial, added}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			integrationContractRoundTrip(t, integrationContractModel(tc.lessons, tc.sections, false))
		})
	}
}

func TestIntegrationContractCatalogAndModuleBoundaries(t *testing.T) {
	first := integrationContractEntry("ID", "l001.json", "20")
	for _, tc := range []struct {
		name     string
		lessons  []manifest.Entry
		sections []manifest.Section
	}{
		{"unsorted", []manifest.Entry{first, integrationContractEntry("Other", "l002.json", "1")}, []manifest.Section{}},
		{"same_id", []manifest.Entry{first, integrationContractEntry("ID", "l002.json", "1")}, []manifest.Section{}},
		{"case_insensitive_id_collision", []manifest.Entry{first, integrationContractEntry("id", "l002.json", "1")}, []manifest.Section{}},
		{"same_filename", []manifest.Entry{first, integrationContractEntry("Other", "l001.json", "1")}, []manifest.Section{}},
		{"same_numeric_weight", []manifest.Entry{integrationContractEntry("A", "l001.json", "1"), integrationContractEntry("B", "l002.json", "001")}, []manifest.Section{}},
		{"same_section_title", []manifest.Entry{}, []manifest.Section{{Title: "Тема", Lessons: []manifest.Entry{first}}, {Title: "Тема", Lessons: []manifest.Entry{integrationContractEntry("Other", "l002.json", "1")}}}},
		{"mixed_and_cross_array_duplicate", []manifest.Entry{first}, []manifest.Section{{Title: "Я", Lessons: []manifest.Entry{first}}, {Title: "А", Lessons: []manifest.Entry{integrationContractEntry("Other", "l002.json", "1")}}}},
	} {
		for _, unit := range []bool{false, true} {
			name := tc.name + "/flat"
			if unit {
				name = tc.name + "/unit"
			}
			t.Run(name, func(t *testing.T) {
				integrationContractRoundTrip(t, integrationContractModel(tc.lessons, tc.sections, unit))
			})
		}
	}
	first.Order = nil
	other := integrationContractEntry("Other", "l002.json", "1")
	other.Order = nil
	// null зависит от длины конкретного массива, даже если всего уроков два.
	integrationContractRoundTrip(t, integrationContractModel([]manifest.Entry{first}, []manifest.Section{{Title: "Тема", Lessons: []manifest.Entry{other}}}, true))
}

func TestIntegrationContractShapeSurvivesInformationLosingDecode(t *testing.T) {
	valid := integrationContractModel([]manifest.Entry{integrationContractEntry("ID", "l001.json", "1")}, []manifest.Section{}, false)
	for _, field := range []string{"order", "subtitle", "extra"} {
		t.Run(field, func(t *testing.T) {
			raw := integrationContractTree(t, integrationContractJSON(t, valid))
			entry := raw.(map[string]any)["course"].(map[string]any)["lessons"].([]any)[0].(map[string]any)
			if field == "extra" {
				entry[field] = true
			} else {
				delete(entry, field)
			}
			before := integrationContractJSON(t, raw)
			errs := manifest.ValidateShape(raw, diagnostic.Context{Path: "модуль.zip", FileName: "manifest.json"})
			if len(errs) != 1 || errs[0].Code != manifest.CodeShape || !integrationContractHasPointer(errs[0], "/course/lessons/0/"+field) {
				t.Fatalf("ошибка формы потеряна: %+v", errs)
			}
			if !bytes.Equal(before, integrationContractJSON(t, raw)) {
				t.Fatal("ValidateShape изменил дерево JSON")
			}
			var decoded manifest.Manifest
			if err := json.Unmarshal(before, &decoded); err != nil {
				t.Fatal(err)
			}
			// Успех типизированной проверки не отменяет уже обнаруженную Shape-ошибку.
			if got := manifest.Validate(decoded, diagnostic.Context{}); len(got) != 0 {
				t.Fatalf("ожидалась потеря сведений при обычном Decode: %+v", got)
			}
		})
	}
}

func TestIntegrationContractNestedDiagnosticsAndNonmutation(t *testing.T) {
	bad := integrationContractEntry("bad id", "bad.json", "0")
	bad.Version = "0"
	other := integrationContractEntry("Other", "l002.json", "1")
	other.Title = ""
	value := integrationContractModel([]manifest.Entry{}, []manifest.Section{{Title: "Тема", Lessons: []manifest.Entry{bad, other}}}, true)
	before := integrationContractJSON(t, value)
	value = integrationContractDecode(t, before)
	ctx := diagnostic.Context{Path: `C:\Курсы\модуль.zip`, FileName: "manifest.json"}
	errs := manifest.Validate(value, ctx)
	wantCodes := []string{identity.CodeInvalidLessonID, identity.CodeInvalidFileName, order.CodeInvalidWeight, manifest.CodeInteger, manifest.CodeTitle}
	wantFields := []string{"0/id", "0/fileName", "0/order", "0/version", "1/title"}
	if len(errs) != len(wantCodes) {
		t.Fatalf("получено %d ошибок, ожидалось %d: %+v", len(errs), len(wantCodes), errs)
	}
	for i, err := range errs {
		entry := bad
		if i == len(errs)-1 {
			entry = other
		}
		if err.Code != wantCodes[i] || err.Reason == "" || !integrationContractHasPointer(err, "/course/unit/sections/0/lessons/"+wantFields[i]) || err.Context != (diagnostic.Context{Path: ctx.Path, FileName: entry.FileName, LessonID: entry.ID}) {
			t.Errorf("неполная или неверно упорядоченная диагностика %d: %+v", i, err)
		}
	}
	_, filenameError := identity.ParseFileName(bad.FileName)
	for i, source := range []*diagnostic.Error{identity.ValidateLessonID(bad.ID), filenameError, order.Validate(*bad.Order, diagnostic.Context{})} {
		if errs[i].Reason != source.Reason {
			t.Errorf("изменена причина зависимости: %+v", errs[i])
		}
		for _, expected := range source.Details {
			found := false
			for _, got := range errs[i].Details {
				found = found || got == expected
			}
			if !found {
				t.Errorf("потеряна подробность зависимости %+v: %+v", expected, errs[i])
			}
		}
	}
	if !bytes.Equal(before, integrationContractJSON(t, value)) {
		t.Fatal("ошибочная модель изменена проверкой")
	}
}

func integrationContractEntry(id, filename, weight string) manifest.Entry {
	return manifest.Entry{ID: id, FileName: filename, Order: &weight, Title: "Исходное название", LineCount: "25", CreatedAt: "2026-11-04T09:00:00Z", Version: "1"}
}

func integrationContractModel(lessons []manifest.Entry, sections []manifest.Section, unit bool) manifest.Manifest {
	course := manifest.Course{Title: "Курс", Lessons: &lessons, Sections: &sections}
	if unit {
		course = manifest.Course{Title: "Курс", Unit: &manifest.Unit{Title: "Юнит", Lessons: lessons, Sections: sections}}
	}
	return manifest.Manifest{FormatVersion: manifest.FormatVersion, CreatedAt: "2026-11-06T00:00:00Z", Course: course}
}

func integrationContractJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func integrationContractTree(t *testing.T, encoded []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func integrationContractDecode(t *testing.T, encoded []byte) manifest.Manifest {
	t.Helper()
	raw := integrationContractTree(t, encoded)
	before := integrationContractJSON(t, raw)
	if errs := manifest.ValidateShape(raw, diagnostic.Context{}); len(errs) != 0 {
		t.Fatalf("форма JSON: %+v", errs)
	}
	if !bytes.Equal(before, integrationContractJSON(t, raw)) {
		t.Fatal("проверка формы изменила дерево")
	}
	var value manifest.Manifest
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func integrationContractRoundTrip(t *testing.T, value manifest.Manifest) {
	t.Helper()
	before := integrationContractJSON(t, value)
	for range 2 {
		if errs := manifest.Validate(value, diagnostic.Context{}); len(errs) != 0 {
			t.Fatalf("значения модели: %+v", errs)
		}
		if !bytes.Equal(before, integrationContractJSON(t, value)) {
			t.Fatal("проверка изменила модель или порядок массивов")
		}
		decoded := integrationContractDecode(t, integrationContractJSON(t, value))
		if !reflect.DeepEqual(value, decoded) {
			t.Fatalf("повторное декодирование изменило модель: %+v", decoded)
		}
		value = decoded
	}
}

func integrationContractHasPointer(err *diagnostic.Error, pointer string) bool {
	for _, detail := range err.Details {
		if detail.Value == pointer {
			return true
		}
	}
	return false
}
