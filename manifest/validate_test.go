package manifest

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/identity"
	"github.com/verdoga/dsl-builder/order"
)

func validateContractString(value string) *string { return &value }

func validateContractEntry(id, weight string) Entry {
	entry := Entry{ID: id, FileName: "l001.json", Title: "Урок", LineCount: "25", CreatedAt: "2026-11-04T09:00:00Z", Version: "1"}
	if weight != "" {
		entry.Order = validateContractString(weight)
	}
	return entry
}

func validateContractFlat(lessons []Entry, sections []Section) Manifest {
	return Manifest{FormatVersion: "1.0", CreatedAt: "2026-11-04T09:00:00Z", Course: Course{Title: "Курс", Lessons: &lessons, Sections: &sections}}
}

func validateContractHasPointer(err *diagnostic.Error, pointer string) bool {
	for _, detail := range err.Details {
		if detail.Value == pointer {
			return true
		}
	}
	return false
}

func validateContractCheckErrors(t *testing.T, errors []*diagnostic.Error, prefix string) {
	t.Helper()
	for i, err := range errors {
		if err == nil {
			t.Fatalf("ошибка %d равна nil", i)
		}
		if err.Code == "" || !strings.ContainsFunc(err.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
			t.Errorf("неполная русская диагностика: %+v", err)
		}
		found := false
		for _, detail := range err.Details {
			found = found || strings.HasPrefix(detail.Value, prefix)
		}
		if !found {
			t.Errorf("нет JSON Pointer %q в %+v", prefix, err.Details)
		}
	}
}

func validateContractSnapshot(t *testing.T, value Manifest) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestValidateContractAcceptedSnapshots(t *testing.T) {
	a, b := validateContractEntry("A:B", "000999999999999999999999999999999999"), validateContractEntry("lesson+1", "1")
	b.Subtitle = validateContractString("")
	b.LineCount, b.Version = "25.0", "1e0"
	for name, value := range map[string]Manifest{
		"flat single null":    validateContractFlat([]Entry{validateContractEntry("MiXeD", "")}, []Section{}),
		"flat unsorted mixed": validateContractFlat([]Entry{a, b}, []Section{{Title: "Zulu", Lessons: []Entry{validateContractEntry("C", "")}}, {Title: "Alpha", Lessons: []Entry{validateContractEntry("D", "")}}}),
		"unit mixed":          {FormatVersion: "1.0", CreatedAt: "2026-11-04T09:00:00Z", Course: Course{Title: "Курс", Unit: &Unit{Title: "Юнит", Lessons: []Entry{a}, Sections: []Section{{Title: "Секция", Lessons: []Entry{b}}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			before := validateContractSnapshot(t, value)
			if errs := Validate(value, diagnostic.Context{}); len(errs) != 0 {
				t.Fatalf("допустимый снимок отклонён: %+v", errs)
			}
			if after := validateContractSnapshot(t, value); after != before {
				t.Fatalf("модель изменена:\n%s\n%s", before, after)
			}
		})
	}
}

func TestValidateContractCourseFormsAndNullCollections(t *testing.T) {
	valid := validateContractFlat([]Entry{validateContractEntry("A", "")}, []Section{})
	for name, course := range map[string]Course{
		"no branch":         {Title: "Курс"},
		"lessons missing":   {Title: "Курс", Sections: valid.Course.Sections},
		"sections missing":  {Title: "Курс", Lessons: valid.Course.Lessons},
		"unit and lessons":  {Title: "Курс", Unit: &Unit{}, Lessons: valid.Course.Lessons},
		"unit and sections": {Title: "Курс", Unit: &Unit{}, Sections: valid.Course.Sections},
		"flat nil lessons":  validateContractFlat(nil, []Section{}).Course,
		"flat nil sections": validateContractFlat([]Entry{validateContractEntry("A", "")}, nil).Course,
		"flat empty":        validateContractFlat([]Entry{}, []Section{}).Course,
		"unit nil lessons":  {Title: "Курс", Unit: &Unit{Title: "Юнит", Sections: []Section{}}},
		"unit nil sections": {Title: "Курс", Unit: &Unit{Title: "Юнит", Lessons: []Entry{validateContractEntry("A", "")}}},
		"unit empty":        {Title: "Курс", Unit: &Unit{Title: "Юнит", Lessons: []Entry{}, Sections: []Section{}}},
	} {
		t.Run(name, func(t *testing.T) {
			valid.Course = course
			before := validateContractSnapshot(t, valid)
			errs := Validate(valid, diagnostic.Context{})
			if len(errs) == 0 {
				t.Fatal("недопустимая форма принята")
			}
			validateContractCheckErrors(t, errs, "/course")
			if validateContractSnapshot(t, valid) != before {
				t.Fatal("проверка исправила входную форму")
			}
		})
	}
}

func TestValidateContractNestedCoordinators(t *testing.T) {
	a := validateContractEntry("A", "")
	for _, test := range []struct {
		name string
		call func() []*diagnostic.Error
		bad  bool
	}{
		{"course", func() []*diagnostic.Error {
			return validateCourse(validateContractFlat([]Entry{a}, []Section{}).Course, "/x", diagnostic.Context{})
		}, false},
		{"unit", func() []*diagnostic.Error {
			return validateUnit(Unit{Title: "Юнит", Lessons: []Entry{a}, Sections: []Section{}}, "/x", diagnostic.Context{})
		}, false},
		{"mixed contents", func() []*diagnostic.Error {
			return validateContents([]Entry{a}, []Section{{Title: "Секция", Lessons: []Entry{a}}}, "/x", diagnostic.Context{})
		}, false},
		{"empty contents", func() []*diagnostic.Error {
			return validateContents([]Entry{}, []Section{}, "/x", diagnostic.Context{})
		}, true},
		{"nil contents", func() []*diagnostic.Error { return validateContents(nil, nil, "/x", diagnostic.Context{}) }, true},
		{"empty sections", func() []*diagnostic.Error { return validateSections([]Section{}, "/x", diagnostic.Context{}) }, false},
		{"nil sections", func() []*diagnostic.Error { return validateSections(nil, "/x", diagnostic.Context{}) }, true},
		{"nil section lessons", func() []*diagnostic.Error {
			return validateSection(Section{Title: "Секция"}, "/x", diagnostic.Context{})
		}, true},
		{"empty section lessons", func() []*diagnostic.Error {
			return validateSection(Section{Title: "Секция", Lessons: []Entry{}}, "/x", diagnostic.Context{})
		}, true},
		{"optional entries", func() []*diagnostic.Error { return validateEntries([]Entry{}, 0, "/x", diagnostic.Context{}) }, false},
		{"required entries", func() []*diagnostic.Error { return validateEntries([]Entry{}, 1, "/x", diagnostic.Context{}) }, true},
		{"nil entries", func() []*diagnostic.Error { return validateEntries(nil, 0, "/x", diagnostic.Context{}) }, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			errs := test.call()
			if (len(errs) != 0) != test.bad {
				t.Fatalf("ошибки: %+v; ожидалось отклонение: %v", errs, test.bad)
			}
			validateContractCheckErrors(t, errs, "/x")
		})
	}
}

func TestValidateContractNullOrderIsLocalToArray(t *testing.T) {
	a, b := validateContractEntry("A", ""), validateContractEntry("B", "")
	for _, entries := range [][]Entry{{a, b}, {validateContractEntry("A", "2"), b}} {
		errs := validateEntries(entries, 0, "/x", diagnostic.Context{})
		want := len(entries)
		if entries[0].Order != nil {
			want--
		}
		if len(errs) != want {
			t.Fatalf("получены %d ошибок вместо %d: %+v", len(errs), want, errs)
		}
		for i, err := range errs {
			index := i
			if entries[0].Order != nil {
				index++
			}
			pointer := []string{"/x/0/order", "/x/1/order"}[index]
			if err.Code != CodeNullOrder || !validateContractHasPointer(err, pointer) {
				t.Errorf("неверная ошибка null: %+v", err)
			}
		}
	}
}

func TestValidateContractIndependentErrorsInModelOrder(t *testing.T) {
	entry := Entry{ID: "bad id", FileName: "l000.json", Order: validateContractString("0"), Title: "", LineCount: "-1", CreatedAt: "bad", Version: "0"}
	value := Manifest{FormatVersion: "2.0", CreatedAt: "bad", Course: Course{Title: "", Unit: &Unit{Title: "", Lessons: []Entry{entry}, Sections: []Section{{Title: " bad ", Lessons: []Entry{}}}}}}
	context := diagnostic.Context{Path: `C:\Курс\module.zip`, FileName: "manifest.json"}
	before := validateContractSnapshot(t, value)
	errs := Validate(value, context)
	wantCodes := []string{CodeVersion, CodeCreatedAt, CodeTitle, CodeTitle, identity.CodeInvalidLessonID, identity.CodeInvalidFileName, order.CodeInvalidWeight, CodeTitle, CodeInteger, CodeCreatedAt, CodeInteger, CodeTitle, CodeContents}
	wantPointers := []string{"/formatVersion", "/createdAt", "/course/title", "/course/unit/title", "/course/unit/lessons/0/id", "/course/unit/lessons/0/fileName", "/course/unit/lessons/0/order", "/course/unit/lessons/0/title", "/course/unit/lessons/0/lineCount", "/course/unit/lessons/0/createdAt", "/course/unit/lessons/0/version", "/course/unit/sections/0/title", "/course/unit/sections/0/lessons"}
	if len(errs) != len(wantCodes) {
		t.Fatalf("получены %d ошибок вместо %d: %+v", len(errs), len(wantCodes), errs)
	}
	validateContractCheckErrors(t, errs, "/")
	for i, err := range errs {
		if err.Code != wantCodes[i] || !validateContractHasPointer(err, wantPointers[i]) {
			t.Errorf("ошибка %d: %+v; ожидаются %s %s", i, err, wantCodes[i], wantPointers[i])
		}
		want := context
		if i >= 4 && i <= 10 {
			want.FileName, want.LessonID = entry.FileName, entry.ID
		}
		if err.Context != want {
			t.Errorf("контекст %d: %+v; нужен %+v", i, err.Context, want)
		}
	}
	_, fileError := identity.ParseFileName(entry.FileName)
	for i, original := range []*diagnostic.Error{identity.ValidateLessonID(entry.ID), fileError, order.Validate(*entry.Order, diagnostic.Context{})} {
		actual := errs[i+4]
		if actual.Reason != original.Reason {
			t.Errorf("причина зависимости изменена: %+v", actual)
		}
		for _, detail := range original.Details {
			found := false
			for _, preserved := range actual.Details {
				found = found || preserved == detail
			}
			if !found {
				t.Errorf("деталь зависимости потеряна: %+v", detail)
			}
		}
	}
	if validateContractSnapshot(t, value) != before {
		t.Fatal("ошибочный снимок изменён")
	}
}

func TestValidateContractUniqueItemsAndResponsibilityBoundary(t *testing.T) {
	a := validateContractEntry("Same", "1")
	b := a
	b.Title = "Другой урок"
	c := a
	c.Order = validateContractString("01")
	for name, value := range map[string]Manifest{
		"duplicate scalars":                    validateContractFlat([]Entry{a, b, c}, []Section{}),
		"duplicate section titles":             validateContractFlat([]Entry{}, []Section{{Title: "Same", Lessons: []Entry{a}}, {Title: "Same", Lessons: []Entry{b}}}),
		"identical entries in distinct arrays": validateContractFlat([]Entry{a}, []Section{{Title: "Same", Lessons: []Entry{a}}}),
	} {
		t.Run(name, func(t *testing.T) {
			if errs := Validate(value, diagnostic.Context{}); len(errs) != 0 {
				t.Fatalf("проверено правило другого пакета: %+v", errs)
			}
		})
	}
	b = a
	b.LineCount, b.Version = "25e0", "1.0"
	for name, value := range map[string]Manifest{
		"entries":  validateContractFlat([]Entry{a, b}, []Section{}),
		"sections": validateContractFlat([]Entry{}, []Section{{Title: "Same", Lessons: []Entry{a}}, {Title: "Same", Lessons: []Entry{b}}}),
	} {
		t.Run(name, func(t *testing.T) {
			errs := Validate(value, diagnostic.Context{})
			if len(errs) == 0 {
				t.Fatal("полностью одинаковые JSON-объекты приняты")
			}
			prefix := "/course/lessons/"
			if name == "sections" {
				prefix = "/course/sections/"
			}
			for _, pointer := range []string{prefix + "0", prefix + "1"} {
				found := false
				for _, err := range errs {
					found = found || err.Code == CodeUniqueItems && validateContractHasPointer(err, pointer)
				}
				if !found {
					t.Errorf("не указан участник uniqueItems %s: %+v", pointer, errs)
				}
			}
		})
	}
}

func TestValidateContractInvalidScalarsDoNotReachUncheckedEquality(t *testing.T) {
	a := validateContractEntry("A", "1")
	a.LineCount, a.Version = json.Number("invalid"), json.Number("1e+")
	value := validateContractFlat([]Entry{a, a}, []Section{{Title: "Секция", Lessons: []Entry{a}}, {Title: "Секция", Lessons: []Entry{a}}})
	errs := Validate(value, diagnostic.Context{})
	count := 0
	for _, err := range errs {
		if err != nil && err.Code == CodeInteger {
			count++
		}
	}
	if count != 8 {
		t.Fatalf("не собраны независимые ошибки чисел: %d; %+v", count, errs)
	}
	for _, entries := range [][]Entry{*value.Course.Lessons, (*value.Course.Sections)[0].Lessons, (*value.Course.Sections)[1].Lessons} {
		for _, entry := range entries {
			if entry.LineCount != "invalid" || entry.Version != "1e+" {
				t.Fatal("числа исправлены")
			}
		}
	}
}
