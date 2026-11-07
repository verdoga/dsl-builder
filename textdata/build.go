package textdata

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// Build координирует только выбор специализированного формирования данных.
// kind уже выбран structure: task, step, variants, variant, heading,
// text, instruction, note, alt, script, hint, answerKey, example,
// wordList, table или multifill. Другой тип даёт CodeUnsupportedType.
// declaration — исходная строка объявления; nil допустим только для
// самостоятельного text. Назначение типа по исходному тегу здесь не выполняется.
// lines — собственные строки одного экземпляра ноды в порядке источника,
// без декларации, её закрывающей границы и содержимого дочерних нод.
// Срез может быть несмежным: номера строк не превращаются в пропуски текста.
// Для instruction из блочного instr в lines включён вложенный hint
// целиком, включая его декларацию и, при наличии, закрывающую границу.
// Для task, step, variant, variants и heading lines должен быть пуст.
// Для instruction из объявления multifill/ordering передаются эта декларация
// и пустой lines; содержимое упражнения в инструкцию не попадает.
// Для самого multifill декларационный content не является body.
// Обычные строки text передаются через lines при declaration=nil.
// Проверка пригодности локального среза — checkInput; далее вызовы
// buildTitleOnly, buildTextBlock, buildInstruction, buildExample или
// buildMultifill. При ошибке возвращается nil и одна блокирующая диагностика;
// частичный результат не возвращается. Успех обозначается nil ошибки.
func Build(kind lesson.NodeType, declaration *model.Line, lines []model.Line, context diagnostic.Context) ([]lesson.Data, *diagnostic.Error) {
	if err := checkInput(kind, declaration, lines, context); err != nil {
		return nil, err
	}
	var result []lesson.Data
	var err *diagnostic.Error
	switch kind {
	case lesson.NodeTask, lesson.NodeStep, lesson.NodeVariants, lesson.NodeVariant, lesson.NodeHeading:
		result, err = buildTitleOnly(kind, *declaration, context)
	case lesson.NodeInstruction:
		result, err = buildInstruction(*declaration, lines, context)
	case lesson.NodeExample:
		result, err = buildExample(*declaration, lines, context)
	case lesson.NodeMultifill:
		result, err = buildMultifill(lines, context)
	default:
		result, err = buildTextBlock(kind, declaration, lines, context)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// checkInput координирует checkNodeKind, checkDeclaration и checkOwnLines.
// Проверяет поддерживаемый kind, декларацию и локальную форму строк.
// Не переопределяет kind, не обходит
// документ, не проверяет дерево, parser diagnostics, ID и версию контракта.
// Неподходящая декларация, чужие теги в собственных строках либо передача
// строк тела для title-only ноды дают CodeInput, а не молчаливую потерю текста.
// При NodeInstruction разрешены декларации instr, multifill и ordering;
// у двух последних lines пуст. NodeText с nil принимает только свой текст.
func checkInput(kind lesson.NodeType, declaration *model.Line, lines []model.Line, context diagnostic.Context) *diagnostic.Error {
	if err := checkNodeKind(kind, context); err != nil {
		return err
	}
	if err := checkDeclaration(kind, declaration, context); err != nil {
		return err
	}
	return checkOwnLines(kind, declaration, lines, context)
}

// buildTitleOnly формирует title для task из identifier, step и heading
// из title, variant из name; использует declarationTitle и textDatum.
// У variants возвращает пустой ненулевой срез; его потомки не читаются.
// Исходный ID task сохраняется как текст, не используется как итоговый ID.
func buildTitleOnly(kind lesson.NodeType, declaration model.Line, context diagnostic.Context) ([]lesson.Data, *diagnostic.Error) {
	title, err := declarationTitle(kind, declaration, context)
	if err != nil {
		return nil, err
	}
	if title == nil {
		return make([]lesson.Data, 0), nil
	}
	return []lesson.Data{textDatum(lesson.RoleTitle, *title, "")}, nil
}

// declarationTitle извлекает единственное соответствующее Value объявления:
// identifier у task, name у variant, title у heading, step и именуемых
// блоков text, note, alt, script, answerKey, wordList, table, example.
// Возвращает nil при отсутствии необязательного названия; отсутствие
// обязательного элемента или несколько кандидатов дают CodeInput.
// Присутствующее nil Value даёт CodeMissingValue; значение не обрезается.
// В variants, hint, instruction и multifill собственного title нет.
func declarationTitle(kind lesson.NodeType, declaration model.Line, context diagnostic.Context) (*string, *diagnostic.Error) {
	var elementType model.ElementType
	required := false
	switch kind {
	case lesson.NodeTask:
		elementType, required = model.ElementTypeIdentifier, true
	case lesson.NodeVariant:
		elementType, required = model.ElementTypeName, true
	case lesson.NodeHeading, lesson.NodeStep:
		elementType, required = model.ElementTypeTitle, true
	case lesson.NodeText, lesson.NodeNote, lesson.NodeAlt, lesson.NodeScript,
		lesson.NodeAnswerKey, lesson.NodeWordList, lesson.NodeTable, lesson.NodeExample:
		elementType = model.ElementTypeTitle
	default:
		return nil, nil
	}
	var candidates []*model.Element
	for i := range declaration.Elements {
		if declaration.Elements[i].ElementType == elementType {
			candidates = append(candidates, &declaration.Elements[i])
		}
	}
	if len(candidates) == 0 {
		if required {
			return nil, dataError(CodeInput, "В декларации отсутствует обязательное название", &declaration, nil, context)
		}
		return nil, nil
	}
	if len(candidates) > 1 {
		err := dataError(CodeInput, "Название в декларации неоднозначно", &declaration, candidates[0], context)
		for _, candidate := range candidates {
			place := dataError(CodeInput, err.Reason, &declaration, candidate, context).Context
			err.Related = append(err.Related, place)
		}
		return nil, err
	}
	if candidates[0].Value == nil {
		return nil, dataError(CodeMissingValue, "Неизвестно значение названия", &declaration, candidates[0], context)
	}
	title := *candidates[0].Value
	return &title, nil
}
