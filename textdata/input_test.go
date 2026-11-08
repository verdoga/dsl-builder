package textdata

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

func inputQAKinds() []lesson.NodeType {
	return []lesson.NodeType{lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants,
		lesson.NodeVariant, lesson.NodeHeading, lesson.NodeText, lesson.NodeInstruction,
		lesson.NodeNote, lesson.NodeAlt, lesson.NodeScript, lesson.NodeHint,
		lesson.NodeAnswerKey, lesson.NodeExample, lesson.NodeWordList, lesson.NodeTable,
		lesson.NodeMultifill}
}

func inputQATag(kind lesson.NodeType) string {
	switch kind {
	case lesson.NodeHeading:
		return "header"
	case lesson.NodeInstruction:
		return "instr"
	case lesson.NodeAnswerKey:
		return "key"
	case lesson.NodeWordList:
		return "wordlist"
	default:
		return string(kind)
	}
}

func inputQAElement(kind model.ElementType, raw string, value *string, start int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: value,
		Start: start, End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}}
}

func inputQADeclaration(tag string, block bool) *model.Line {
	line := &model.Line{Number: 20, LineType: model.LineTypeTag, LineEnding: model.LineEndingLF,
		Raw: "\t@" + strings.ToUpper(tag), Elements: []model.Element{}}
	line.Elements = append(line.Elements, inputQAElement(model.ElementTypeTag, line.Raw[1:], &tag, 2))
	add := func(kind model.ElementType, raw string) {
		start := utf8.RuneCountInString(line.Raw) + 2
		line.Raw += " " + raw
		line.Elements = append(line.Elements, inputQAElement(kind, raw, &raw, start))
	}
	switch tag {
	case "task":
		add(model.ElementTypeIdentifier, "А1")
	case "step", "header":
		add(model.ElementTypeTitle, "Заголовок🙂")
	case "variant":
		add(model.ElementTypeName, "Ветка🙂")
	case "variants":
	default:
		if block {
			switch tag {
			case "text", "note", "alt", "script", "key", "wordlist", "table", "example":
				add(model.ElementTypeTitle, "Название🙂")
			case "multifill", "ordering", "matching":
				add(model.ElementTypeContent, "Заполни🙂")
			}
		} else {
			add(model.ElementTypeContent, "Текст🙂")
		}
	}
	if block {
		line.LineType = model.LineTypeBlockStart
		add(model.ElementTypeBlockOpen, "{")
	}
	return line
}

func inputQAContext() diagnostic.Context {
	return diagnostic.Context{Path: `C:\Учебные материалы\урок.parsed.json`,
		FileName: "урок.json", LessonID: "AB12", PageID: "ab12-p001", Line: 3, Column: 4}
}

func inputQAProblem(t *testing.T, got *diagnostic.Error, code string) {
	t.Helper()
	if got == nil {
		t.Fatal("ожидалась блокирующая диагностика")
	}
	if (code != "" && got.Code != code) || got.Code == "" {
		t.Errorf("код = %q, ожидается %q", got.Code, code)
	}
	if !strings.ContainsFunc(got.Reason, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) {
		t.Errorf("причина не содержит русского текста: %q", got.Reason)
	}
	context := inputQAContext()
	if got.Context.Path != context.Path || got.Context.LessonID != context.LessonID ||
		got.Context.PageID != context.PageID || got.Context.FileName != context.FileName {
		t.Errorf("потерян внешний контекст: %+v", got.Context)
	}
}

func inputQASnapshot(t *testing.T, declaration *model.Line, lines []model.Line) string {
	t.Helper()
	value, err := json.Marshal(struct {
		Declaration *model.Line
		Lines       []model.Line
	}{declaration, lines})
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}

func TestInputQANodeKindBoundary(t *testing.T) {
	for _, kind := range inputQAKinds() {
		t.Run("supported/"+string(kind), func(t *testing.T) {
			if err := checkNodeKind(kind, inputQAContext()); err != nil {
				t.Fatalf("поддерживаемый тип: %v", err)
			}
		})
	}
	unsupported := []lesson.NodeType{lesson.NodeMedia, lesson.NodeQuestion,
		lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching,
		lesson.NodeOrdering, "", "future", "TEXT", "wordlist", "instr"}
	for _, kind := range unsupported {
		t.Run("unsupported/"+string(kind), func(t *testing.T) {
			err := checkNodeKind(kind, inputQAContext())
			inputQAProblem(t, err, CodeUnsupportedType)
			if err.Context != inputQAContext() {
				t.Errorf("контекст без исходной строки изменился: %+v", err.Context)
			}
		})
	}
}

func TestInputQADeclarationMapping(t *testing.T) {
	for _, kind := range inputQAKinds() {
		t.Run(string(kind), func(t *testing.T) {
			declaration := inputQADeclaration(inputQATag(kind), kind == lesson.NodeVariants)
			before := inputQASnapshot(t, declaration, nil)
			if err := checkDeclaration(kind, declaration, inputQAContext()); err != nil {
				t.Fatalf("каноническое Value при верхнем регистре Raw: %v", err)
			}
			if before != inputQASnapshot(t, declaration, nil) {
				t.Fatal("декларация изменена")
			}
			for _, sourceKind := range inputQAKinds() {
				if sourceKind == kind || (kind == lesson.NodeInstruction && sourceKind == lesson.NodeMultifill) {
					continue
				}
				wrong := inputQADeclaration(inputQATag(sourceKind), sourceKind == lesson.NodeVariants)
				inputQAProblem(t, checkDeclaration(kind, wrong, inputQAContext()), CodeInput)
			}
			if kind == lesson.NodeText {
				if err := checkDeclaration(kind, nil, inputQAContext()); err != nil {
					t.Fatalf("самостоятельный текст без декларации: %v", err)
				}
			} else {
				inputQAProblem(t, checkDeclaration(kind, nil, inputQAContext()), CodeInput)
			}
		})
	}
	for _, tag := range []string{"multifill", "ordering", "matching"} {
		for _, block := range []bool{false, true} {
			t.Run("instruction/"+tag+"/block="+strconv.FormatBool(block), func(t *testing.T) {
				declaration := inputQADeclaration(tag, block)
				before := inputQASnapshot(t, declaration, nil)
				if err := checkDeclaration(lesson.NodeInstruction, declaration, inputQAContext()); err != nil {
					t.Fatalf("выделенная инструкция: %v", err)
				}
				if before != inputQASnapshot(t, declaration, nil) {
					t.Fatal("декларация изменена")
				}
			})
		}
	}
	for _, tag := range []string{"choice", "multichoice", "instruction", "answerKey", "wordList", "heading"} {
		inputQAProblem(t, checkDeclaration(lesson.NodeInstruction, inputQADeclaration(tag, true), inputQAContext()), CodeInput)
	}
}

func TestInputQADeclarationNeedsExactlyOneSemanticTag(t *testing.T) {
	for _, tag := range []string{"text", "matching"} {
		for _, block := range []bool{false, true} {
			t.Run(tag+"/block="+strconv.FormatBool(block), func(t *testing.T) {
				kind := lesson.NodeText
				if tag == "matching" {
					kind = lesson.NodeInstruction
				}
				for _, name := range []string{"no-tag", "nil-value", "empty-value", "duplicate-same", "duplicate-other"} {
					t.Run(name, func(t *testing.T) {
						declaration := inputQADeclaration(tag, block)
						code := CodeInput
						switch name {
						case "no-tag":
							declaration.Elements = declaration.Elements[1:]
						case "nil-value":
							declaration.Elements[0].Value = nil
							code = "" // Контракт метода не назначает точный код отсутствующему tag.Value.
						case "empty-value":
							empty := ""
							declaration.Elements[0].Value = &empty
						case "duplicate-same", "duplicate-other":
							duplicateTag := tag
							if name == "duplicate-other" {
								duplicateTag = "note"
							}
							raw := "@" + duplicateTag
							start := utf8.RuneCountInString(declaration.Raw) + 2
							declaration.Raw += " " + raw
							declaration.Elements = append(declaration.Elements, inputQAElement(model.ElementTypeTag, raw, &duplicateTag, start))
						}
						before := inputQASnapshot(t, declaration, nil)
						inputQAProblem(t, checkDeclaration(kind, declaration, inputQAContext()), code)
						if before != inputQASnapshot(t, declaration, nil) {
							t.Fatal("ошибочная декларация изменена")
						}
					})
				}
			})
		}
	}
}

func TestInputQAMatchingInstructionNeverReceivesExerciseBody(t *testing.T) {
	for _, block := range []bool{false, true} {
		t.Run("block="+strconv.FormatBool(block), func(t *testing.T) {
			declaration := inputQADeclaration("matching", block)
			inputQACheckLines(t, lesson.NodeInstruction, declaration, nil, false)
			inputQACheckLines(t, lesson.NodeInstruction, declaration, []model.Line{}, false)
			for _, lineType := range []model.LineType{model.LineTypeContent, model.LineTypeBlank, model.LineTypeSeparator} {
				t.Run(string(lineType), func(t *testing.T) {
					inputQACheckLines(t, lesson.NodeInstruction, declaration, []model.Line{inputQABody(80, lineType)}, true)
				})
			}
		})
	}
}
