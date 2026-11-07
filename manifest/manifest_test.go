package manifest

import (
	"encoding/json"
	"testing"
)

func TestManifestModelFormatVersion(t *testing.T) {
	const supported = FormatVersion
	if supported != "1.0" {
		t.Fatalf("FormatVersion = %q, требуется строковая константа 1.0", supported)
	}
}

func TestManifestModelRequiredRootFieldsRemainPresentAtZeroValues(t *testing.T) {
	encoded, err := json.Marshal(Manifest{})
	if err != nil {
		t.Fatal(err)
	}
	fields := manifestModelRootFields(t, encoded)
	for _, name := range []string{"formatVersion", "createdAt"} {
		if string(fields[name]) != `""` {
			t.Errorf("%s = %s, требуется пустая строка без автоматического заполнения", name, fields[name])
		}
	}
	var course map[string]json.RawMessage
	if err := json.Unmarshal(fields["course"], &course); err != nil {
		t.Fatalf("course должен оставаться объектом: %v", err)
	}
	if course == nil {
		t.Fatal("обязательный course сериализован как null")
	}
}

func TestManifestModelRootJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		course string
	}{
		{"без юнита", `{"title":"Курс А","lessons":[],"sections":[]}`},
		{"с юнитом", `{"title":"Курс А","unit":{"title":"Юнит Б","lessons":[],"sections":[]}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Дата намеренно фиксирована и не связана с текущим временем.
			input := `{"formatVersion":"1.0","createdAt":"2024-02-29T23:59:58Z","course":` + test.course + `}`
			var model Manifest
			if err := json.Unmarshal([]byte(input), &model); err != nil {
				t.Fatal(err)
			}
			if model.FormatVersion != "1.0" || model.CreatedAt != "2024-02-29T23:59:58Z" {
				t.Fatalf("не прочитаны корневые метаданные: %#v", model)
			}
			if model.Course.Title != "Курс А" {
				t.Fatalf("course не прочитан в корневую модель: %#v", model.Course)
			}
			encoded, err := json.Marshal(model)
			if err != nil {
				t.Fatal(err)
			}
			fields := manifestModelRootFields(t, encoded)
			if string(fields["formatVersion"]) != `"1.0"` {
				t.Errorf("formatVersion изменён: %s", fields["formatVersion"])
			}
			if string(fields["createdAt"]) != `"2024-02-29T23:59:58Z"` {
				t.Errorf("createdAt изменён: %s", fields["createdAt"])
			}
			var course map[string]json.RawMessage
			if err := json.Unmarshal(fields["course"], &course); err != nil {
				t.Fatal(err)
			}
			if string(course["title"]) != `"Курс А"` {
				t.Errorf("название курса изменено: %s", course["title"])
			}
			_, hasUnit := course["unit"]
			if hasUnit != (test.name == "с юнитом") {
				t.Errorf("форма course изменилась: %s", fields["course"])
			}
		})
	}
}

func TestManifestModelRootMetadataAreStoredWithoutReplacement(t *testing.T) {
	model := Manifest{FormatVersion: "future", CreatedAt: "назначено вызывающим кодом", Course: Course{Title: "Курс"}}
	encoded, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	fields := manifestModelRootFields(t, encoded)
	if string(fields["formatVersion"]) != `"future"` || string(fields["createdAt"]) != `"назначено вызывающим кодом"` {
		t.Fatalf("модель заменила переданные значения: %s", encoded)
	}
}

func manifestModelRootFields(t *testing.T, encoded []byte) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 {
		t.Errorf("корневых полей %d, требуется ровно 3: %s", len(fields), encoded)
	}
	for _, name := range []string{"formatVersion", "createdAt", "course"} {
		if _, exists := fields[name]; !exists {
			t.Errorf("обязательное поле %q отсутствует: %s", name, encoded)
		}
	}
	return fields
}
