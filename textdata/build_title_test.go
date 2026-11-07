package textdata

import (
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func TestBuildQATitleOnly(t *testing.T) {
	for _, tc := range []struct {
		kind  lesson.NodeType
		field model.ElementType
		value string
	}{
		{lesson.NodeTask, model.ElementTypeIdentifier, "AbC-00007"},
		{lesson.NodeStep, model.ElementTypeTitle, "  Шаг\t"},
		{lesson.NodeHeading, model.ElementTypeTitle, "Заголовок"},
		{lesson.NodeVariant, model.ElementTypeName, "Вариант"},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			dec := buildQALine(29, model.LineTypeTag,
				buildQAElement(model.ElementTypeContent, "Не название"),
				buildQAElement(tc.field, tc.value))
			got, err := buildTitleOnly(tc.kind, dec, diagnostic.Context{})
			if err != nil {
				t.Fatal(err)
			}
			buildQAData(t, got, []lesson.DataRole{lesson.RoleTitle}, []string{tc.value})
		})
	}
	dec := buildQALine(29, model.LineTypeTag, model.Element{ElementType: model.ElementTypeTitle})
	got, err := buildTitleOnly(lesson.NodeVariants, dec, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	buildQAData(t, got, nil, nil)
}

func TestBuildQADeclarationTitleSelection(t *testing.T) {
	cases := []struct {
		kind     lesson.NodeType
		field    model.ElementType
		required bool
	}{
		{lesson.NodeTask, model.ElementTypeIdentifier, true},
		{lesson.NodeVariant, model.ElementTypeName, true},
		{lesson.NodeHeading, model.ElementTypeTitle, true},
		{lesson.NodeStep, model.ElementTypeTitle, true},
		{lesson.NodeText, model.ElementTypeTitle, false},
		{lesson.NodeNote, model.ElementTypeTitle, false},
		{lesson.NodeAlt, model.ElementTypeTitle, false},
		{lesson.NodeScript, model.ElementTypeTitle, false},
		{lesson.NodeAnswerKey, model.ElementTypeTitle, false},
		{lesson.NodeWordList, model.ElementTypeTitle, false},
		{lesson.NodeTable, model.ElementTypeTitle, false},
		{lesson.NodeExample, model.ElementTypeTitle, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			ctx := diagnostic.Context{Path: "данные.json", FileName: "l001.json", LessonID: "ID", PageID: "id-p001"}
			for _, value := range []string{"  Ёж\t", "", "<br>\\n"} {
				dec := buildQALine(51, model.LineTypeTag, buildQAElement(tc.field, value), buildQAElement(model.ElementTypeContent, "другое"))
				title, err := declarationTitle(tc.kind, dec, ctx)
				if err != nil || title == nil || *title != value {
					t.Fatalf("value %q: title=%v error=%v", value, title, err)
				}
			}
			dec := buildQALine(51, model.LineTypeTag, buildQAElement(model.ElementTypeContent, "не title"))
			title, err := declarationTitle(tc.kind, dec, ctx)
			if tc.required {
				buildQAError(t, err, CodeInput)
			} else if err != nil {
				t.Fatal(err)
			}
			if title != nil {
				t.Fatalf("missing title=%q", *title)
			}
			dec = buildQALine(51, model.LineTypeTag, buildQAElement(tc.field, "Ёж"))
			dec.Elements[0].Value = nil
			title, err = declarationTitle(tc.kind, dec, ctx)
			buildQAError(t, err, CodeMissingValue)
			if title != nil || err.Context.Path != ctx.Path || err.Context.Line != 51 || err.Context.Column != 1 {
				t.Fatalf("title=%v error=%#v", title, err)
			}
			dec = buildQALine(51, model.LineTypeTag, buildQAElement(tc.field, "А"), buildQAElement(tc.field, "Б"), buildQAElement(tc.field, "В"))
			title, err = declarationTitle(tc.kind, dec, ctx)
			buildQAError(t, err, CodeInput)
			if title != nil || len(err.Related) != 3 {
				t.Fatalf("title=%v related=%#v", title, err.Related)
			}
			for i, related := range err.Related {
				want := ctx
				want.Line = 51
				want.Column = dec.Elements[i].Start
				if related != want {
					t.Errorf("related[%d]=%#v want %#v", i, related, want)
				}
			}
		})
	}
}

func TestBuildQATypesWithoutTitle(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeVariants, lesson.NodeHint, lesson.NodeInstruction, lesson.NodeMultifill} {
		t.Run(string(kind), func(t *testing.T) {
			dec := buildQALine(9, model.LineTypeTag,
				model.Element{ElementType: model.ElementTypeTitle},
				model.Element{ElementType: model.ElementTypeTitle},
				model.Element{ElementType: model.ElementTypeName},
				model.Element{ElementType: model.ElementTypeIdentifier})
			title, err := declarationTitle(kind, dec, diagnostic.Context{})
			if title != nil || err != nil {
				t.Fatalf("title=%v error=%v", title, err)
			}
		})
	}
}

func TestBuildQATitleOnlyErrorsAreAtomic(t *testing.T) {
	for _, kind := range []lesson.NodeType{lesson.NodeTask, lesson.NodeStep, lesson.NodeHeading, lesson.NodeVariant} {
		t.Run(string(kind), func(t *testing.T) {
			got, err := buildTitleOnly(kind, model.Line{Number: 7}, diagnostic.Context{Path: "технический.json"})
			buildQAError(t, err, CodeInput)
			if got != nil {
				t.Fatalf("partial=%#v", got)
			}
		})
	}
}

func TestBuildQAUsesSelectedSliceWithoutGlobalValidation(t *testing.T) {
	// Координаты и родитель — внешний контекст, а не повторный обход документа.
	parent := 17
	dec := buildQADeclaration("note", model.LineTypeBlockStart, buildQAElement(model.ElementTypeTitle, "Название"))
	dec.ParentLine = &parent
	dec.NestingLevel = 4
	a := buildQALine(48, model.LineTypeContent, buildQAElement(model.ElementTypeContent, "A"))
	b := buildQALine(97, model.LineTypeContent, buildQAElement(model.ElementTypeContent, "Б"))
	a.ParentLine = &parent
	b.ParentLine = &parent
	got, err := Build(lesson.NodeNote, &dec, []model.Line{a, b}, diagnostic.Context{})
	if err != nil {
		t.Fatal(err)
	}
	buildQAData(t, got, []lesson.DataRole{lesson.RoleTitle, lesson.RoleBody}, []string{"Название", "A\nБ"})
}

func TestBuildQATitleOnlyPropagatesMissingValue(t *testing.T) {
	for _, tc := range []struct {
		kind  lesson.NodeType
		field model.ElementType
	}{
		{lesson.NodeTask, model.ElementTypeIdentifier},
		{lesson.NodeStep, model.ElementTypeTitle},
		{lesson.NodeHeading, model.ElementTypeTitle},
		{lesson.NodeVariant, model.ElementTypeName},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			dec := buildQALine(11, model.LineTypeTag, buildQAElement(tc.field, "Название"))
			dec.Elements[0].Value = nil
			got, err := buildTitleOnly(tc.kind, dec, diagnostic.Context{})
			buildQAError(t, err, CodeMissingValue)
			if got != nil {
				t.Fatalf("partial=%#v", got)
			}
		})
	}
}
