package structure

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
)

// openTask закрывает предыдущее задание того же семантического уровня
// вместе с его внутренними областями; затем receivingPath определяет родителя
// нового NodeTask. Уровень определяется цепочкой Entry.Parent и ближайшей
// действующей ветвью/контейнером, а не одинаковой глубиной или типом frame.
// Если цепочка содержит объявление прежнего task либо между ним и новым
// task открыт содержащий новый task variants/variant, прежний task — предок,
// а не одноуровневый предшественник: он и его step остаются снаружи.
// Новый task в текущей ветви с общим Parent=variants закрывает только прежний
// task этой же ветви. Без ветви у ближайшего variants возвращает CodeContext.
// Открывает новый контекст после его родителя. Использует конкретное Entry,
// не identifier.Value: одинаковые отображаемые ID в ветвях/вставках независимы.
// Объявление сохраняет в Binding для последующего формирования data.title.
// Содержимое нового task принимает он сам либо его собственный step.
func openTask(state *builder, entry *fragments.Entry, boundary *fragments.Entry) []*diagnostic.Error {
	// Вставка прозрачна для уровня задания, но её Entry остаётся границей frame.
	parent := func(e *fragments.Entry) *fragments.Entry {
		p := e.Parent
		for p != nil && p.Included != nil {
			p = p.Parent
		}
		return p
	}
	for i := len(state.frames) - 1; i >= 0; i-- {
		binding := state.bindings[state.frames[i].binding]
		kind := binding.Node(state.pages).Type
		if kind == lesson.NodeVariants || kind == lesson.NodeVariant {
			break
		}
		if kind != lesson.NodeTask {
			continue
		}
		previous := binding.Entries[0]
		ancestor := false
		for p := entry.Parent; p != nil; p = p.Parent {
			if p == previous {
				ancestor = true
				break
			}
		}
		if !ancestor && parent(previous) == parent(entry) {
			closeFrom(state, i)
		}
		break
	}
	path, problem := receivingPath(state, entry)
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	binding := appendNode(state, path, lesson.NodeTask, []*fragments.Entry{entry})
	state.frames = append(state.frames, frame{binding: binding, boundary: boundary})
	return nil
}

// endTask закрывает ближайшее соответствующее Entry.Parent и семантической
// области задание со всеми его внутренними областями, не создавая ноды.
// Не подменяет отсутствующее задание внутренней ветви внешним task;
// закрытие вложенного task сохраняет его внешних предков.
// Последующее внешнее содержимое размещается вне закрытого задания.
// При отсутствии соответствующей области возвращает CodeContext.
func endTask(state *builder, entry *fragments.Entry) []*diagnostic.Error {
	for i := len(state.frames) - 1; i >= 0; i-- {
		binding := state.bindings[state.frames[i].binding]
		switch binding.Node(state.pages).Type {
		case lesson.NodeVariants, lesson.NodeVariant:
			return []*diagnostic.Error{structureError(CodeContext, "В текущей ветви нет задания для endtask.", entry, binding.Entries, state)}
		case lesson.NodeTask:
			closeFrom(state, i)
			return nil
		}
	}
	return []*diagnostic.Error{structureError(CodeContext, "Нет задания для endtask.", entry, nil, state)}
}

// openStep требует ближайшее задание текущей семантической области по
// Entry.Parent и frames; не перескакивает через внутреннюю ветвь/variants
// к внешнему task, если собственного задания здесь нет: это CodeContext.
// Завершает только предыдущий step выбранного задания и его внутренние области
// и добавляет NodeStep непосредственно в task.Nodes. Последующие ноды
// принадлежат этому step даже при Entry.Parent, указывающем на task.
// До первого step содержимое остаётся непосредственными детьми task.
func openStep(state *builder, entry *fragments.Entry, boundary *fragments.Entry) []*diagnostic.Error {
	for i := len(state.frames) - 1; i >= 0; i-- {
		owner := state.bindings[state.frames[i].binding]
		switch owner.Node(state.pages).Type {
		case lesson.NodeVariants, lesson.NodeVariant:
			return []*diagnostic.Error{structureError(CodeContext, "В текущей ветви нет задания для step.", entry, owner.Entries, state)}
		case lesson.NodeTask:
			// Этап всегда непосредственный ребёнок выбранного задания.
			closeFrom(state, i+1)
			binding := appendNode(state, owner.NodePath, lesson.NodeStep, []*fragments.Entry{entry})
			state.frames = append(state.frames, frame{binding: binding, boundary: boundary})
			return nil
		}
	}
	return []*diagnostic.Error{structureError(CodeContext, "Нет задания для step.", entry, nil, state)}
}

// openVariants добавляет NodeVariants к receivingPath до открытия нового
// контекста и открывает его область до собственного block-end. Новой страницы нет.
// Внешние task/step/variant остаются предками, если entry им принадлежит.
// Новый набор первоначально не имеет своей ветви: внешняя variant её
// не заменяет. Содержимое до первой собственной variant даёт CodeContext.
// Не применяет старые ограничения DSL на максимальную глубину вложенности.
func openVariants(state *builder, entry *fragments.Entry, boundary *fragments.Entry) []*diagnostic.Error {
	path, problem := receivingPath(state, entry)
	if problem != nil {
		return []*diagnostic.Error{problem}
	}
	binding := appendNode(state, path, lesson.NodeVariants, []*fragments.Entry{entry})
	state.frames = append(state.frames, frame{binding: binding, boundary: boundary})
	return nil
}

// openVariant выбирает ближайший содержащий variants по Entry.Parent
// и действующим frames, без перехода к внешнему набору через внутренний.
// Требует этот набор, закрывает только его предыдущую variant со всеми её
// внутренними областями, добавляет NodeVariant прямо в Nodes этого variants.
// Внешние variant/task/step сохраняются; отсутствие набора даёт CodeContext.
// До следующего variant/закрытия variants сохраняет эту семантическую ветвь,
// в том числе для task с Entry.Parent, равным объявлению variants.
// Имя сохраняется только в исходной привязке для будущих собственных данных.
func openVariant(state *builder, entry *fragments.Entry, boundary *fragments.Entry) []*diagnostic.Error {
	for i := len(state.frames) - 1; i >= 0; i-- {
		owner := state.bindings[state.frames[i].binding]
		if owner.Node(state.pages).Type != lesson.NodeVariants {
			continue
		}
		closeFrom(state, i+1)
		binding := appendNode(state, owner.NodePath, lesson.NodeVariant, []*fragments.Entry{entry})
		state.frames = append(state.frames, frame{binding: binding, boundary: boundary})
		return nil
	}
	return []*diagnostic.Error{structureError(CodeContext, "Нет набора variants для ветви variant.", entry, nil, state)}
}

// markSpeaking выбирает task той же семантической области, что openStep,
// и устанавливает только его Speaking в true; внешнее задание не подставляет.
// Сам признак не создаёт ноды; отсутствие задания даёт CodeContext.
// Не проверяет заново редакторское правило «первой значимой конструкции».
func markSpeaking(state *builder, entry *fragments.Entry) []*diagnostic.Error {
	for i := len(state.frames) - 1; i >= 0; i-- {
		binding := state.bindings[state.frames[i].binding]
		node := binding.Node(state.pages)
		switch node.Type {
		case lesson.NodeVariants, lesson.NodeVariant:
			return []*diagnostic.Error{structureError(CodeContext, "В текущей ветви нет задания для speaking.", entry, binding.Entries, state)}
		case lesson.NodeTask:
			speaking := true
			node.Flags.Speaking = &speaking
			return nil
		}
	}
	return []*diagnostic.Error{structureError(CodeContext, "Нет задания для speaking.", entry, nil, state)}
}

// finishTaskFlags обходит созданные task и устанавливает оба применимых флага:
// speaking по отмеченному признаку, stepped по наличию относящихся к этому
// заданию внутренних step. Отсутствующий признак получает false, без checkable.
// Этап другого вложенного задания не делает внешнее задание stepped.
// Флаги всех остальных типов остаются пустыми; checkable вычисляет владелец
// данных упражнений. Для multifill он остаётся отсутствующим по ТЗ 10.7.
func finishTaskFlags(state *builder) {
	for _, binding := range state.bindings {
		node := binding.Node(state.pages)
		if node.Type != lesson.NodeTask {
			continue
		}
		speaking := node.Flags.Speaking != nil && *node.Flags.Speaking
		stepped := false
		for _, child := range node.Nodes {
			if child.Type == lesson.NodeStep {
				stepped = true
				break
			}
		}
		node.Flags = lesson.NodeFlags{Speaking: &speaking, Stepped: &stepped}
	}
}
