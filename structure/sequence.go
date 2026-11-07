package structure

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// walkSequence обходит одну последовательность в заданном порядке, вызывая
// visitEntry для очередной ещё не потреблённой конструкции. Индекс следующей
// записи возвращает visitEntry; повторно обходить тело потреблённого блока нельзя.
// boundary равен nil у документа либо Entry include у вставки.
// Конец документа завершает все оставшиеся семантические области; конец
// последовательности include обрабатывает walkInclude, не закрывая внешний task.
func walkSequence(state *builder, entries []*fragments.Entry, boundary *fragments.Entry) []*diagnostic.Error {
	for at := 0; at < len(entries); {
		next, problems := visitEntry(state, entries, at, boundary)
		if len(problems) != 0 {
			return problems
		}
		at = next
	}
	if boundary == nil {
		closeFrom(state, 0)
	}
	return nil
}

// visitEntry — только диспетчер: skipEditor, walkInclude, pageBoundary/openPage,
// trimContexts, обработчики контейнеров и appendContent. blank вне тела ноды,
// заголовки 1/2 и метатеги dsl-version/document-id/section/order/resource-dir
// не создают нод; endtask и speaking передаются своим обработчикам.
// block-end завершает соответствующий контейнер, но не создаёт ноды.
// fragment после Expand означает нарушение предусловия и CodeSource.
// Значимый непредставимый вход не пропускается молча: возвращается ошибка.
func visitEntry(state *builder, entries []*fragments.Entry, at int, boundary *fragments.Entry) (next int, errors []*diagnostic.Error) {
	entry := entries[at]
	next = at + 1
	if entry.Source.LineType == model.LineTypeBlank {
		return next, nil
	}
	if entry.Source.LineType == model.LineTypeBlockEnd {
		return next, closeBlock(state, entry)
	}
	page, title, problems := pageBoundary(entry, state)
	if len(problems) != 0 {
		return next, problems
	}
	if page {
		openPage(state, entry, title)
		return next, nil
	}
	if entry.Source.LineType == model.LineTypeHeading {
		return next, nil
	}
	tag := ""
	if entry.Source.LineType == model.LineTypeTag || entry.Source.LineType == model.LineTypeBlockStart {
		for _, element := range entry.Source.Elements {
			if element.ElementType == model.ElementTypeTag && element.Value != nil {
				tag = *element.Value
				break
			}
		}
	}
	switch tag {
	case "editor":
		return skipEditor(state, entries, at)
	case "dsl-version", "document-id", "section", "order", "resource-dir":
		return next, nil
	case "fragment":
		return next, []*diagnostic.Error{structureError(CodeSource, "Определение fragment осталось после развёртывания вставок", entry, nil, state)}
	}
	trimContexts(state, entry)
	switch tag {
	case "include":
		return next, walkInclude(state, entry)
	case "task", "step", "variants", "variant":
		if entry.Source.LineType == model.LineTypeBlockStart {
			if _, _, problems := blockEntries(entries, at, state); len(problems) != 0 {
				return next, problems
			}
		}
		switch tag {
		case "task":
			return next, openTask(state, entry, boundary)
		case "step":
			return next, openStep(state, entry, boundary)
		case "variants":
			return next, openVariants(state, entry, boundary)
		default:
			return next, openVariant(state, entry, boundary)
		}
	case "endtask":
		return next, endTask(state, entry)
	case "speaking":
		return next, markSpeaking(state, entry)
	case "answer":
		return next, []*diagnostic.Error{structureError(CodeContext, "Ответ не принадлежит связанной конструкции упражнения", entry, nil, state)}
	default:
		return appendContent(state, entries, at)
	}
}

// blockEntries выбирает конструкцию до её собственного block-end включительно,
// сопоставляя закрывающую строку через Entry.Parent с конкретным объявлением.
// У однострочной конструкции возвращает одну запись. Сохраняет внутренние
// границы и пустые строки; raw не анализирует и текстовые данные не собирает.
// Незамкнутый/пересечённый блок даёт CodeBoundary с исходными координатами.
func blockEntries(entries []*fragments.Entry, at int, state *builder) (body []*fragments.Entry, next int, errors []*diagnostic.Error) {
	start := entries[at]
	if start.Source.LineType != model.LineTypeBlockStart {
		return entries[at : at+1], at + 1, nil
	}
	opened := []*fragments.Entry{start}
	for i := at + 1; i < len(entries); i++ {
		entry := entries[i]
		current := opened[len(opened)-1]
		if entry.Source.LineType == model.LineTypeBlockEnd {
			if entry.Parent != current {
				return nil, i + 1, []*diagnostic.Error{structureError(CodeBoundary, "Закрывающая граница пересекает другой блок", entry, opened, state)}
			}
			opened = opened[:len(opened)-1]
			if len(opened) == 0 {
				return entries[at : i+1], i + 1, nil
			}
			continue
		}
		if entry.Source.LineType == model.LineTypeBlockStart {
			opened = append(opened, entry)
		}
	}
	return nil, len(entries), []*diagnostic.Error{structureError(CodeBoundary, "Не найдена закрывающая граница блока", opened[len(opened)-1], opened, state)}
}

// skipEditor пропускает однострочный editor либо полное тело его блока,
// найденное blockEntries. Содержимое комментария не интерпретируется как теги.
func skipEditor(state *builder, entries []*fragments.Entry, at int) (next int, errors []*diagnostic.Error) {
	_, next, errors = blockEntries(entries, at, state)
	return next, errors
}

// walkInclude вызывает walkSequence для site.Included в точке вставки.
// Сохраняет действующие внешний task/step/variant и не создаёт ноды include.
// Соседство question/answer/hint снаружи и внутри разделено этой границей.
// После вставки закрывает только ещё активные области, открытые внутри неё;
// внешние области, уже завершённые реальной границей, не возобновляет.
// Исходные номера не перенумеровываются; разные *Entry не объединяются.
func walkInclude(state *builder, site *fragments.Entry) []*diagnostic.Error {
	if site.Included == nil {
		return []*diagnostic.Error{structureError(CodeSource, "Вставка include не развёрнута", site, nil, state)}
	}
	problems := walkSequence(state, site.Included, site)
	for i, context := range state.frames {
		if context.boundary == site {
			closeFrom(state, i)
			break
		}
	}
	return problems
}

// closeBlock завершает область конкретного блока, указанного Entry.Parent,
// и все её внутренние области через closeFrom. Закрытие внутреннего variants
// закрывает только его variant и её потомков; внешние variants/variant/task/step
// остаются активны. Сопоставление по типу или номеру другого include запрещено.
// Блоки собственных данных уже потреблены blockEntries и сюда не поступают.
func closeBlock(state *builder, end *fragments.Entry) []*diagnostic.Error {
	declaration := end.Parent
	if declaration == nil || declaration.Source.LineType != model.LineTypeBlockStart {
		return []*diagnostic.Error{structureError(CodeBoundary, "Закрывающая граница не имеет объявления блока", end, nil, state)}
	}
	for i := len(state.frames) - 1; i >= 0; i-- {
		if state.bindings[state.frames[i].binding].Entries[0] == declaration {
			closeFrom(state, i)
			return nil
		}
	}
	// Реальная семантическая граница могла завершить этот контейнер раньше.
	for _, binding := range state.bindings {
		if len(binding.Entries) == 1 && binding.Entries[0] == declaration {
			switch binding.Node(state.pages).Type {
			case "task", "step", "variants", "variant":
				return nil
			}
		}
	}
	return []*diagnostic.Error{structureError(CodeBoundary, "Граница не относится к активному структурному блоку", end, []*fragments.Entry{declaration}, state)}
}
