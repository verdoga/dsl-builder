package structure

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
)

// builder — изменяемое состояние одного вызова Build, не отдельная модель урока.
type builder struct {
	// input — проверенный источник для известных пути, ID и координат ошибок.
	input *parsed.Input

	// pages — строящиеся страницы без ID и собственных данных нод.
	pages []lesson.Page

	// bindings — привязки созданных нод в порядке прямого обхода дерева.
	bindings []Binding

	// page — индекс текущей страницы; -1 до первого отображаемого материала
	// либо явной границы. Предварительная пустая страница не создаётся заранее.
	page int

	// frames — активные семантические контейнеры от внешнего к внутреннему.
	// Порядок совпадает с вложенностью NodePath: step принадлежит конкретному
	// task, variant — конкретному variants; тип не даёт приоритета над глубиной.
	frames []frame
}

// frame хранит активную область контейнера; parentLine сам по себе не выражает
// время жизни step и variant, поэтому стек дополняет исходные родительские связи.
type frame struct {
	// binding — индекс привязки task, step, variants или variant в builder.
	// Её первое Entry — ключ контекста; это исходная строка и конкретная вставка.
	binding int

	// boundary — исходный контейнер, выход из которого завершает эту область.
	// nil означает документ; для включения хранится конкретный Entry include.
	boundary *fragments.Entry
}

// receivingPath выбирает ближайшего семантического получателя entry после
// trimContexts: последний подходящий frame по порядку стека, а не по типу.
// Принадлежность учитывает цепочку Entry.Parent и действующие области
// step/variant с общим исходным родителем. nil означает корень страницы.
// Step принимает содержимое только своего task; более близкие variant или
// вложенный task имеют преимущество перед внешним step/task. Так, при стеке
// task A → step A → variants → variant B получатель — B, а после открытия
// task B — task B либо его собственный step; внешние области A не мешают.
// Для ближайшего содержащего variants обязательна активная variant именно
// этого набора: её NodePath является непосредственным дочерним путём набора.
// Пока её нет, возвращает CodeContext с местом entry, даже если внешняя
// variant/task/step активна. Сам variants не принимает обычное содержимое.
// Объявление новой variant адресует openVariant; оно не требует прежней ветви.
func receivingPath(state *builder, entry *fragments.Entry) ([]int, *diagnostic.Error) {
	for i := len(state.frames) - 1; i >= 0; i-- {
		owner := state.bindings[state.frames[i].binding]
		if owner.Node(state.pages).Type != lesson.NodeVariants {
			continue
		}
		active := false
		for _, child := range state.frames[i+1:] {
			branch := state.bindings[child.binding]
			if branch.Node(state.pages).Type != lesson.NodeVariant || branch.Page != owner.Page || len(branch.NodePath) != len(owner.NodePath)+1 {
				continue
			}
			active = true
			for at, index := range owner.NodePath {
				if branch.NodePath[at] != index {
					active = false
					break
				}
			}
			if active {
				break
			}
		}
		if !active {
			return nil, structureError(CodeContext, "У набора variants нет собственной активной ветви", entry, owner.Entries, state)
		}
		break
	}
	if len(state.frames) == 0 {
		return nil, nil
	}
	return state.bindings[state.frames[len(state.frames)-1].binding].NodePath, nil
}

// trimContexts закрывает только области, которым entry больше не принадлежит.
// Анализирует цепочку Entry.Parent и активные семантические границы, а не
// Source.NestingLevel в одиночку. include прозрачен для внешней принадлежности.
// Сохраняет единую цепочку семантических предков: при выходе закрывает
// покинутую область и весь её внутренний хвост, но не внешние предки.
// Общий Parent с объявлением variant сохраняет ветвь своего variants;
// Parent, равный объявлению task, сохраняет действующий step этого task.
// Эти правила действуют внутри конкретных владельцев, не переносят строку
// из внутреннего variants/task во внешнюю ветвь/этап и не скрывают отсутствие
// ветви у внутреннего variants. Новые одноуровневые task/step/variant
// завершают предшественника через соответствующий open*, а не по типу
// произвольного внешнего frame. Закрытые области не восстанавливаются.
// Закрытие обычного контентного блока не закрывает внешнее задание.
func trimContexts(state *builder, entry *fragments.Entry) {
	contains := func(ancestor *fragments.Entry) bool {
		for parent := entry.Parent; parent != nil; parent = parent.Parent {
			if parent == ancestor {
				return true
			}
		}
		return false
	}
	parentOf := func(source *fragments.Entry) *fragments.Entry {
		parent := source.Parent
		for parent != nil && parent.Included != nil {
			parent = parent.Parent
		}
		return parent
	}
	peerTask := false
	for _, element := range entry.Source.Elements {
		if element.ElementType == "tag" && element.Value != nil {
			peerTask = *element.Value == "task" || *element.Value == "endtask"
			break
		}
	}
	for i, context := range state.frames {
		binding := state.bindings[context.binding]
		declaration := binding.Entries[0]
		if context.boundary != nil && !contains(context.boundary) {
			closeFrom(state, i)
			return
		}
		if contains(declaration) {
			continue
		}
		belongs := false
		switch binding.Node(state.pages).Type {
		case lesson.NodeTask:
			belongs = peerTask && parentOf(entry) == parentOf(declaration)
		case lesson.NodeStep, lesson.NodeVariant:
			ownerKind := lesson.NodeTask
			if binding.Node(state.pages).Type == lesson.NodeVariant {
				ownerKind = lesson.NodeVariants
			}
			// Владелец определяется прямым путём ноды, а не ближайшим типом.
			for j := i - 1; j >= 0; j-- {
				owner := state.bindings[state.frames[j].binding]
				if owner.Page != binding.Page || len(owner.NodePath)+1 != len(binding.NodePath) || owner.Node(state.pages).Type != ownerKind {
					continue
				}
				belongs = true
				for at, index := range owner.NodePath {
					if binding.NodePath[at] != index {
						belongs = false
						break
					}
				}
				if belongs {
					belongs = contains(owner.Entries[0])
					break
				}
			}
		}
		if !belongs {
			closeFrom(state, i)
			return
		}
	}
}

// closeFrom закрывает указанную область стека и все вложенные области,
// не удаляя уже построенные ноды и не меняя их собственного содержимого.
func closeFrom(state *builder, frameIndex int) {
	state.frames = state.frames[:frameIndex]
}
