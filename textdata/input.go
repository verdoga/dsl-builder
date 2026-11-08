package textdata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// checkNodeKind проверяет только принадлежность kind области Build;
// остальные известные lesson.NodeType и неизвестные значения дают
// CodeUnsupportedType. Это граница пакета, а не повтор проверки enum схемы.
func checkNodeKind(kind lesson.NodeType, context diagnostic.Context) *diagnostic.Error {
	switch kind {
	case lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants, lesson.NodeVariant,
		lesson.NodeHeading, lesson.NodeText, lesson.NodeInstruction, lesson.NodeNote,
		lesson.NodeAlt, lesson.NodeScript, lesson.NodeHint, lesson.NodeAnswerKey,
		lesson.NodeExample, lesson.NodeWordList, lesson.NodeTable, lesson.NodeMultifill:
		return nil
	default:
		return dataError(CodeUnsupportedType, "Тип ноды не поддерживается textdata: "+string(kind), nil, nil, context)
	}
}

// checkDeclaration проверяет наличие декларации и её соответствие kind
// по единственному tag.Value, без разбора Raw. Допускает nil только для text;
// для instruction ожидает instr либо объявление multifill, ordering или matching.
// Для heading ожидает header, для answerKey — key, для wordList — wordlist;
// остальные поддерживаемые имена соответствуют исходным тегам.
// Не выбирает выходной тип, не проверяет существование исходного родителя.
func checkDeclaration(kind lesson.NodeType, declaration *model.Line, context diagnostic.Context) *diagnostic.Error {
	if declaration == nil {
		if kind == lesson.NodeText {
			return nil
		}
		return dataError(CodeInput, "Для ноды требуется исходная декларация", nil, nil, context)
	}
	if declaration.LineType != model.LineTypeTag && declaration.LineType != model.LineTypeBlockStart {
		return dataError(CodeInput, "Декларация должна быть однострочным объявлением или началом блока", declaration, nil, context)
	}

	var tags []*model.Element
	for i := range declaration.Elements {
		if declaration.Elements[i].ElementType == model.ElementTypeTag {
			tags = append(tags, &declaration.Elements[i])
		}
	}
	if len(tags) == 0 {
		return dataError(CodeInput, "В декларации отсутствует тег", declaration, nil, context)
	}
	if len(tags) != 1 {
		problem := dataError(CodeInput, "Декларация содержит несколько тегов", declaration, tags[0], context)
		for _, tag := range tags {
			problem.Related = append(problem.Related, dataError(CodeInput, problem.Reason, declaration, tag, context).Context)
		}
		return problem
	}
	tag := tags[0]
	if tag.Value == nil {
		return dataError(CodeInput, "В декларации неизвестно семантическое имя тега", declaration, tag, context)
	}
	expected := string(kind)
	switch kind {
	case lesson.NodeHeading:
		expected = "header"
	case lesson.NodeAnswerKey:
		expected = "key"
	case lesson.NodeWordList:
		expected = "wordlist"
	case lesson.NodeInstruction:
		if *tag.Value == "instr" || *tag.Value == "multifill" || *tag.Value == "ordering" || *tag.Value == "matching" {
			return nil
		}
		expected = "instr, multifill, ordering или matching"
	}
	if *tag.Value != expected {
		return dataError(CodeInput, "Тег декларации не соответствует типу ноды; ожидается "+expected, declaration, tag, context)
	}
	return nil
}

// checkOwnLines проверяет только разрешённую форму собственного среза:
// пустой у title-only нод и выделенной инструкции multifill/ordering/matching;
// content/blank у текстовых тел, separator дополнительно у example/table.
// У блочного instr допускает строки вложенного hint и его границы;
// точное отделение единственной подсказки делегировано splitInstructionHint.
// Однострочные объявления не получают дополнительные строки тела.
// Не вычисляет принадлежность нод в дереве по NestingLevel или ParentLine;
// собственный срез заранее выбран вызывающим конвейером.
func checkOwnLines(kind lesson.NodeType, declaration *model.Line, lines []model.Line, context diagnostic.Context) *diagnostic.Error {
	if len(lines) == 0 {
		return nil
	}
	switch kind {
	case lesson.NodeTask, lesson.NodeStep, lesson.NodeVariant, lesson.NodeVariants, lesson.NodeHeading:
		return dataError(CodeInput, "Нода с собственным названием не принимает строки тела", &lines[0], nil, context)
	}

	blockInstruction := false
	if declaration != nil {
		if declaration.LineType == model.LineTypeTag {
			return dataError(CodeInput, "Однострочная декларация не принимает дополнительные строки тела", &lines[0], nil, context)
		}
		if kind == lesson.NodeInstruction {
			for i := range declaration.Elements {
				tag := &declaration.Elements[i]
				if tag.ElementType != model.ElementTypeTag || tag.Value == nil {
					continue
				}
				if *tag.Value == "multifill" || *tag.Value == "ordering" || *tag.Value == "matching" {
					return dataError(CodeInput, "Выделенная инструкция не принимает строки упражнения", &lines[0], nil, context)
				}
				blockInstruction = declaration.LineType == model.LineTypeBlockStart && *tag.Value == "instr"
			}
		}
	}

	for i := range lines {
		line := &lines[i]
		switch line.LineType {
		case model.LineTypeContent, model.LineTypeBlank:
			continue
		case model.LineTypeSeparator:
			if kind == lesson.NodeExample || kind == lesson.NodeTable {
				continue
			}
		case model.LineTypeBlockEnd:
			if blockInstruction {
				continue
			}
		case model.LineTypeTag, model.LineTypeBlockStart:
			if blockInstruction {
				hasHint := false
				for j := range line.Elements {
					tag := &line.Elements[j]
					if tag.ElementType != model.ElementTypeTag {
						continue
					}
					if tag.Value == nil || *tag.Value != "hint" {
						return dataError(CodeInput, "В собственных строках инструкции допустим только вложенный hint", line, tag, context)
					}
					hasHint = true
				}
				if hasHint {
					continue
				}
			}
		}
		return dataError(CodeInput, "Тип собственной строки не допускается для этой ноды", line, nil, context)
	}
	return nil
}
