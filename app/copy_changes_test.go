package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
)

func copyChangesFixture() []catalog.Change {
	section := "Раздел Ё"
	return []catalog.Change{
		{FileName: "l001.json", Section: &section, Field: "order", Before: json.RawMessage(`"00000000000000000000000000000001"`), After: json.RawMessage(`"02"`)},
		{Field: "entry", Before: nil, After: json.RawMessage("null")},
		{Field: "version", Before: json.RawMessage("123456789012345678901234567890"), After: json.RawMessage("1e+999")},
		{Field: "course.title", Before: json.RawMessage(`"\u041aурс  "`), After: json.RawMessage(`""`)},
		{Field: "lessons", Before: json.RawMessage{}, After: nil},
	}
}

func TestCopyChangesPreservesExactJSONAndIndependence(t *testing.T) {
	source := copyChangesFixture()
	first, second := cloneChanges(source), cloneChanges(source)
	if !reflect.DeepEqual(first, source) || !reflect.DeepEqual(second, source) {
		t.Fatal("изменены Section, точные JSON-байты, nil либо пустые байты")
	}
	*first[0].Section = "правка"
	first[0].Before[1], first[0].After[1] = '7', '8'
	first[1].After[0] = 'N'
	first[2].After[0] = '2'
	first[3].Before[2] = 'X'
	first[4].Field = "правка"
	if !reflect.DeepEqual(source, copyChangesFixture()) || !reflect.DeepEqual(second, source) {
		t.Fatal("правка первой копии изменила исходный отчёт или вторую копию")
	}
	*source[0].Section = "правка источника"
	source[2].Before[0] = '9'
	source[3].After[0] = 'X'
	source[1] = catalog.Change{}
	if !reflect.DeepEqual(second, copyChangesFixture()) {
		t.Fatal("ранее полученная копия зависит от источника")
	}
}

func TestCopyChangesPreservesNilEmpty(t *testing.T) {
	if cloneChanges(nil) != nil {
		t.Fatal("nil Changes не должен превращаться в пустой срез")
	}
	if got := cloneChanges([]catalog.Change{}); got == nil || len(got) != 0 {
		t.Fatal("пустой ненулевой Changes должен остаться ненулевым")
	}
	section := ""
	for _, source := range [][]catalog.Change{
		{{}},
		{{Section: &section, Before: json.RawMessage{}, After: json.RawMessage{}}},
		{{Before: json.RawMessage("null"), After: json.RawMessage(`""`)}},
	} {
		if got := cloneChanges(source); !reflect.DeepEqual(got, source) {
			t.Fatalf("неточное копирование отсутствия/null/empty: %#v", got)
		}
	}
}
