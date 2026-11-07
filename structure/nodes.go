package structure

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// contentType сопоставляет семантический tag.Value с lesson.NodeType:
// header→heading, instr→instruction, key→answerKey, wordlist→wordList,
// choice→singleChoice, multichoice→multipleChoice; text/note/alt/script/hint/
// example/table/media/question/multifill/matching/ordering — одноимённые типы.
// Самостоятельная содержательная строка content соответствует NodeText.
// Теги контейнеров и служебные теги обрабатывает visitEntry, а не эта функция.
// Непредставимая форма даёт CodeSource; raw не используется как новое DSL.
func contentType(line model.Line, state *builder) (lesson.NodeType, *diagnostic.Error) {
	fail := func(reason string) (lesson.NodeType, *diagnostic.Error) {
		return "", structureError(CodeSource, reason, &fragments.Entry{Source: line}, nil, state)
	}
	if line.LineType == model.LineTypeContent {
		found := false
		for _, element := range line.Elements {
			switch element.ElementType {
			case model.ElementTypeContent:
				if element.Value == nil {
					return fail("Неизвестно семантическое значение самостоятельного текста")
				}
				found = true
			case model.ElementTypePlaceholder:
				found = true
			default:
				return fail("Непредставимая форма самостоятельного текста")
			}
		}
		if found {
			return lesson.NodeText, nil
		}
		return fail("В самостоятельной строке отсутствует текстовое содержимое")
	}
	if line.LineType != model.LineTypeTag && line.LineType != model.LineTypeBlockStart {
		return fail("Тип исходной строки не представляет содержательную ноду")
	}
	tag, count := "", 0
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			count++
			if element.Value != nil {
				tag = *element.Value
			}
		}
	}
	if count != 1 || tag == "" {
		return fail("Неоднозначное либо неизвестное семантическое имя тега")
	}
	switch tag {
	case "header":
		return lesson.NodeHeading, nil
	case "instr":
		return lesson.NodeInstruction, nil
	case "key":
		return lesson.NodeAnswerKey, nil
	case "wordlist":
		return lesson.NodeWordList, nil
	case "choice":
		return lesson.NodeSingleChoice, nil
	case "multichoice":
		return lesson.NodeMultipleChoice, nil
	case "text", "note", "alt", "script", "hint", "example", "table", "media", "question", "multifill", "matching", "ordering":
		return lesson.NodeType(tag), nil
	}
	return fail("Исходный тег не представляет содержательную ноду")
}

// appendContent — координатор blockEntries, questionTail, splitInstruction
// и appendNode для одной отображаемой конструкции. Блочное содержимое text,
// table, multifill и остальных листовых нод не создаёт дочерних текстовых нод.
// Каждая самостоятельная непустая строка content вне блоков даёт одну text.
// script всегда отдельная нода на своём месте, даже сразу после media.
// hint, не вошедший в собственные данные инструкции/упражнения или questionTail,
// создаёт отдельную ноду; совпадения текста не вызывают удаления или слияния.
// Родителя определяет receivingPath для исходного entry после trimContexts.
// Для пары выделенная instruction/упражнение использует один полученный путь
// без смены контекста между ними; обе ноды принадлежат ближайшей области.
func appendContent(state *builder, entries []*fragments.Entry, at int) (next int, errors []*diagnostic.Error) {
	entry := entries[at]
	kind, problem := contentType(entry.Source, state)
	if problem != nil {
		return at + 1, []*diagnostic.Error{structureError(problem.Code, problem.Reason, entry, nil, state)}
	}
	body, next, errors := blockEntries(entries, at, state)
	if len(errors) != 0 {
		return next, errors
	}
	if entry.Source.LineType == model.LineTypeContent {
		present := false
		for _, element := range entry.Source.Elements {
			if element.ElementType == model.ElementTypePlaceholder || element.Value != nil && *element.Value != "" {
				present = true
			}
		}
		if !present {
			return next, nil
		}
	}
	trimContexts(state, entry)
	parentPath, problem := receivingPath(state, entry)
	if problem != nil {
		return next, []*diagnostic.Error{problem}
	}
	if kind == lesson.NodeQuestion {
		var tail []*fragments.Entry
		tail, next, errors = questionTail(state, entries, at, next)
		if len(errors) != 0 {
			return next, errors
		}
		body = append(append([]*fragments.Entry(nil), body...), tail...)
	}
	separate, problem := splitInstruction(entry, kind, state)
	if problem != nil {
		return next, []*diagnostic.Error{problem}
	}
	if separate {
		appendNode(state, parentPath, lesson.NodeInstruction, []*fragments.Entry{entry})
	}
	appendNode(state, parentPath, kind, body)
	return next, nil
}

// appendNode добавляет одну lesson.Node указанного типа к parentPath либо
// в корень страницы и одновременно регистрирует Binding с новым путём.
// Инициализирует пустые ID, ненулевые Data/Nodes и пустые Flags; собственных
// данных не формирует. Возвращает индекс привязки для стека контейнеров.
// Инструкция и интерактивная нода добавляются разными вызовами с одним parentPath.
func appendNode(state *builder, parentPath []int, kind lesson.NodeType, entries []*fragments.Entry) int {
	ensurePage(state)
	nodes := &state.pages[state.page].Nodes
	for _, index := range parentPath {
		nodes = &(*nodes)[index].Nodes
	}
	path := make([]int, len(parentPath)+1)
	copy(path, parentPath)
	path[len(parentPath)] = len(*nodes)
	*nodes = append(*nodes, lesson.Node{Type: kind, Data: make([]lesson.Data, 0), Nodes: make([]lesson.Node, 0)})
	state.bindings = append(state.bindings, Binding{Page: state.page, NodePath: path, Entries: append([]*fragments.Entry(nil), entries...)})
	return len(state.bindings) - 1
}

// questionTail выбирает лишь структурно связанные внешние конструкции question:
// следующий значимый answer на том же уровне и следующий за ним один hint.
// Использует nextSignificant и blockEntries; возвращает исходные вхождения,
// не извлекает ответы/подсказки, не проверяет их количество или содержимое.
// Не пересекает страницы, task/step/variant, block-end и границы include.
// При отсутствии answer ничего не поглощает: следующий hint самостоятельный.
// Ответы/подсказки внутри других интерактивных блоков сохраняются в их Entries
// целиком; размещение по элементам данных принадлежит exercisedata.
func questionTail(state *builder, entries []*fragments.Entry, question int, next int) (tail []*fragments.Entry, after int, errors []*diagnostic.Error) {
	after = next
	for _, want := range []string{"answer", "hint"} {
		at, problems := nextSignificant(state, entries, after)
		if len(problems) != 0 {
			return nil, after, problems
		}
		if at == len(entries) {
			break
		}
		entry := entries[at]
		if entry.Included != nil || entry.Source.LineType != model.LineTypeTag && entry.Source.LineType != model.LineTypeBlockStart {
			break
		}
		tag, count := "", 0
		for _, element := range entry.Source.Elements {
			if element.ElementType == model.ElementTypeTag {
				count++
				if element.Value != nil {
					tag = *element.Value
				}
			}
		}
		if count != 1 || tag != want || entry.Parent != entries[question].Parent {
			break
		}
		// Проверяем область на отдельном стеке: просмотр хвоста не закрывает
		// действующие контексты обхода и не возобновляет завершённые области.
		probe := *state
		probe.frames = append([]frame(nil), state.frames...)
		trimContexts(&probe, entry)
		if len(probe.frames) != len(state.frames) {
			break
		}
		if _, problem := receivingPath(&probe, entry); problem != nil {
			break
		}
		body, end, problems := blockEntries(entries, at, state)
		if len(problems) != 0 {
			return nil, after, problems
		}
		tail = append(tail, body...)
		after = end
	}
	return tail, after, nil
}

// nextSignificant ищет следующую конструкцию только в данной последовательности,
// пропуская blank и полные editor. Границы вставок и блоков не пропускает.
// Возвращает len(entries), когда конструкций больше нет; ничего не изменяет.
func nextSignificant(state *builder, entries []*fragments.Entry, at int) (next int, errors []*diagnostic.Error) {
	for at < len(entries) {
		entry := entries[at]
		if entry.Included != nil || entry.Source.LineType == model.LineTypeBlockEnd {
			break
		}
		if entry.Source.LineType == model.LineTypeBlank {
			at++
			continue
		}
		count, editor := 0, false
		for _, element := range entry.Source.Elements {
			if element.ElementType == model.ElementTypeTag {
				count++
				editor = element.Value != nil && *element.Value == "editor"
			}
		}
		if count != 1 || !editor {
			break
		}
		var problems []*diagnostic.Error
		at, problems = skipEditor(state, entries, at)
		if len(problems) != 0 {
			return at, problems
		}
	}
	return at, nil
}
