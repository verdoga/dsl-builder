package app

import (
	"reflect"
	"testing"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/manifest"
)

func copyEditState() *catalog.State {
	unit, section, order, subtitle, weight := "Юнит", "Раздел", "001", "", "0002"
	return &catalog.State{CourseTitle: "Курс", UnitTitle: &unit, Sectioned: true,
		Items: []catalog.Item{{Entry: manifest.Entry{ID: "COPY", FileName: "l001.json", Order: &order,
			Title: "Название", Subtitle: &subtitle, LineCount: "7", Version: "1", CreatedAt: "2026-11-07T10:00:00Z"},
			Section: &section, RelativePath: "вход/урок.json", Context: diagnostic.Context{Path: "урок.json", Line: 3}}},
		Sections: []catalog.SectionOrder{{Title: "Раздел", Weight: &weight}}}
}

func TestCopyEditPreviewKeepsCandidateClosed(t *testing.T) {
	candidate := copyEditState()
	before := copyEditState()
	before.CourseTitle = "Прежний курс"
	issue := &diagnostic.Error{Code: "catalog.conflict", Reason: "Конфликт",
		Related: []diagnostic.Context{{Path: "связанный.json"}}, Details: []diagnostic.Detail{{Label: "Вес", Value: "001"}},
		Cause: &diagnostic.Error{Code: "catalog.inner", Reason: "Причина", Details: []diagnostic.Detail{{Label: "Исходное", Value: "001"}}}}
	op := &Operation{placement: before}
	key, base := &reviewKey{owner: op}, &reviewKey{owner: op}
	op.edit = &catalogEdit{key: key, base: base, candidate: candidate, issues: []*diagnostic.Error{issue}}
	first, second := op.copyEditPreview(), op.copyEditPreview()
	if first == nil || second == nil || first.Token.key != key || second.Token.key != key {
		t.Fatal("предложение отсутствует либо потеряло идентичность")
	}
	if !reflect.DeepEqual(first.Catalog, candidate) || !reflect.DeepEqual(first.Blockers, op.edit.issues) ||
		!reflect.DeepEqual(first.Changes, catalog.Changes(before, candidate)) {
		t.Fatal("неполное предложение каталога/изменений/блокировок")
	}
	first.Catalog.CourseTitle = "внешняя правка"
	*first.Catalog.UnitTitle = "внешняя правка"
	*first.Catalog.Items[0].Section = "внешняя правка"
	*first.Catalog.Items[0].Entry.Order = "99"
	*first.Catalog.Items[0].Entry.Subtitle = "внешняя правка"
	first.Catalog.Items[0].RelativePath = "внешняя правка"
	first.Catalog.Items[0].Context.Path = "внешняя правка"
	*first.Catalog.Sections[0].Weight = "99"
	first.Catalog.Sections[0].Title = "внешняя правка"
	first.Changes[0].After[1] = 'X'
	first.Blockers[0].Details[0].Value = "99"
	first.Blockers[0].Related[0].Path = "внешняя правка"
	first.Blockers[0].Cause.(*diagnostic.Error).Details[0].Value = "99"
	if !reflect.DeepEqual(candidate, copyEditState()) || before.CourseTitle != "Прежний курс" ||
		!reflect.DeepEqual(second.Catalog, candidate) || !reflect.DeepEqual(second.Changes, catalog.Changes(before, candidate)) ||
		issue.Details[0].Value != "001" || issue.Cause.(*diagnostic.Error).Details[0].Value != "001" {
		t.Fatal("превью раскрыло изменяемые данные кандидата или другой копии")
	}
	*candidate.Items[0].Entry.Order = "77"
	issue.Related[0].Path = "правка источника"
	if *second.Catalog.Items[0].Entry.Order != "001" || second.Blockers[0].Related[0].Path != "связанный.json" {
		t.Fatal("выданный снимок изменился вслед за источником")
	}
	if op.edit.key != key || op.edit.base != base || op.placement != before {
		t.Fatal("копирование изменило accepted/staged состояние")
	}
}

func TestCopyEditPreviewWithoutEdit(t *testing.T) {
	if got := (&Operation{}).copyEditPreview(); got != nil {
		t.Fatal("нет активного предложения, но выдано превью")
	}
}
