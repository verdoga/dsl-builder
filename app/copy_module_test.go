package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/manifest"
	"github.com/verdoga/dsl-builder/module"
)

func copyModuleFixture(t *testing.T, unit, sectioned bool) *module.Module {
	t.Helper()
	order, subtitle := "000123456789012345678901234567890", ""
	entry := manifest.Entry{ID: "COPY", FileName: "l001.json", Order: &order,
		Title: "Урок", Subtitle: &subtitle, LineCount: json.Number("12345678901234567890"),
		CreatedAt: "2026-11-07T10:00:00Z", Version: json.Number("12345678901234567890")}
	lessons, sections := []manifest.Entry{entry}, []manifest.Section{}
	if sectioned {
		lessons = []manifest.Entry{}
		sections = []manifest.Section{{Title: "Раздел", Lessons: []manifest.Entry{entry}}}
	}
	value := &module.Module{Manifest: manifest.Manifest{FormatVersion: "1.0",
		CreatedAt: "2026-11-07T11:00:00Z", Course: manifest.Course{Title: "Курс"}}}
	if unit {
		value.Manifest.Course.Unit = &manifest.Unit{Title: "Юнит", Lessons: lessons, Sections: sections}
	} else {
		value.Manifest.Course.Lessons, value.Manifest.Course.Sections = &lessons, &sections
	}
	file, issues := module.NewFile("l001.json", &lesson.Lesson{FormatVersion: "1.0",
		Document: lesson.Document{ID: "COPY", Version: "1", Title: "Урок", LineCount: "0",
			CreatedAt: "2026-11-07T10:00:00Z", Pages: []lesson.Page{}}}, diagnostic.Context{Path: "исходник.json"})
	if len(issues) != 0 || file == nil {
		t.Fatalf("module.NewFile: %v", issues)
	}
	value.Files = []*module.File{file, file, nil}
	return value
}

func copyModuleMutate(value *module.Module, unit, sectioned bool) {
	value.Manifest.FormatVersion = "изменено"
	value.Manifest.CreatedAt = "изменено"
	value.Manifest.Course.Title = "изменено"
	var entries []manifest.Entry
	if unit {
		value.Manifest.Course.Unit.Title = "изменено"
		entries = value.Manifest.Course.Unit.Lessons
		if sectioned {
			value.Manifest.Course.Unit.Sections[0].Title = "изменено"
			entries = value.Manifest.Course.Unit.Sections[0].Lessons
		}
	} else {
		entries = *value.Manifest.Course.Lessons
		if sectioned {
			(*value.Manifest.Course.Sections)[0].Title = "изменено"
			entries = (*value.Manifest.Course.Sections)[0].Lessons
		}
	}
	*entries[0].Order = "77"
	*entries[0].Subtitle = "изменено"
	entries[0].Title = "изменено"
	entries[0].LineCount = "9"
	entries[0].Version = "9"
	entries[0].ID = "OTHER"
	entries[0].FileName = "l002.json"
	entries[0].CreatedAt = "изменено"
	value.Files[0] = nil
}

func TestCopyModuleIndependentEveryManifestForm(t *testing.T) {
	for _, unit := range []bool{false, true} {
		for _, sectioned := range []bool{false, true} {
			t.Run(map[bool]string{false: "course", true: "unit"}[unit]+map[bool]string{false: "/lessons", true: "/sections"}[sectioned], func(t *testing.T) {
				source := copyModuleFixture(t, unit, sectioned)
				first, second := cloneModule(source), cloneModule(source)
				if !reflect.DeepEqual(first, source) || !reflect.DeepEqual(second, source) {
					t.Fatal("копия потеряла значения или форму Manifest/Files")
				}
				copyModuleMutate(first, unit, sectioned)
				if !reflect.DeepEqual(source, copyModuleFixture(t, unit, sectioned)) || !reflect.DeepEqual(second, source) {
					t.Fatal("правка первой копии изменила источник или вторую копию")
				}
				copyModuleMutate(source, unit, sectioned)
				if !reflect.DeepEqual(second, copyModuleFixture(t, unit, sectioned)) {
					t.Fatal("правка источника изменила ранее полученную копию")
				}
				model := second.Files[0].Lesson()
				model.Document.Title = "внешняя правка"
				if second.Files[0].Lesson().Document.Title != "Урок" {
					t.Fatal("File перестал быть независимым immutable снимком")
				}
			})
		}
	}
}

func TestCopyModulePreservesNilAndEmpty(t *testing.T) {
	if cloneModule(nil) != nil {
		t.Fatal("nil Module должен остаться nil")
	}
	for _, empty := range []bool{false, true} {
		var entries []manifest.Entry
		var sections []manifest.Section
		var files []*module.File
		if empty {
			entries, sections, files = []manifest.Entry{}, []manifest.Section{}, []*module.File{}
		}
		for _, value := range []*module.Module{
			{Files: files},
			{Files: files, Manifest: manifest.Manifest{Course: manifest.Course{Lessons: &entries, Sections: &sections}}},
			{Files: files, Manifest: manifest.Manifest{Course: manifest.Course{Unit: &manifest.Unit{Lessons: entries, Sections: sections}}}},
			{Manifest: manifest.Manifest{Course: manifest.Course{Sections: &[]manifest.Section{{Lessons: entries}}}}},
		} {
			if got := cloneModule(value); !reflect.DeepEqual(got, value) {
				t.Fatalf("изменена форма nil/empty: got=%#v want=%#v", got, value)
			}
		}
	}
}

func TestCopyModuleSlicePointersAndNullableEntryValues(t *testing.T) {
	for _, unit := range []bool{false, true} {
		value := copyModuleFixture(t, unit, false)
		var entry *manifest.Entry
		if unit {
			entry = &value.Manifest.Course.Unit.Lessons[0]
		} else {
			entry = &(*value.Manifest.Course.Lessons)[0]
		}
		entry.Order, entry.Subtitle = nil, nil
		first, second := cloneModule(value), cloneModule(value)
		if !reflect.DeepEqual(first, value) {
			t.Fatal("null Order/Subtitle не сохранены")
		}
		if unit {
			first.Manifest.Course.Unit.Lessons = []manifest.Entry{}
			first.Manifest.Course.Unit.Sections = append(first.Manifest.Course.Unit.Sections, manifest.Section{Title: "Новая"})
			if len(value.Manifest.Course.Unit.Lessons) != 1 || len(second.Manifest.Course.Unit.Sections) != 0 {
				t.Fatal("срезы Unit разделяются")
			}
		} else {
			*first.Manifest.Course.Lessons = []manifest.Entry{}
			*first.Manifest.Course.Sections = append(*first.Manifest.Course.Sections, manifest.Section{Title: "Новая"})
			if len(*value.Manifest.Course.Lessons) != 1 || len(*second.Manifest.Course.Sections) != 0 {
				t.Fatal("указатели массивов Course разделяются")
			}
			*value.Manifest.Course.Sections = []manifest.Section{{Title: "Правка источника"}}
			if len(*second.Manifest.Course.Sections) != 0 {
				t.Fatal("прежний снимок пустого массива зависит от источника")
			}
		}
	}
}
