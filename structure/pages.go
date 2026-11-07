package structure

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// pageBoundary распознаёт только heading с heading-level.Value="3" и newpage.
// Название первой формы берёт из title.Value без нормализации, у второй nil.
// Отсутствующее/неоднозначное необходимое значение даёт CodeSource, а не
// подстановку document.title. header здесь не считается границей страницы.
func pageBoundary(entry *fragments.Entry, state *builder) (boundary bool, title *string, errors []*diagnostic.Error) {
	requiredValue := func(kind model.ElementType, label string) (*string, *diagnostic.Error) {
		var selected *model.Element
		count := 0
		for i := range entry.Source.Elements {
			element := &entry.Source.Elements[i]
			if element.ElementType == kind {
				count++
				if selected == nil {
					selected = element
				}
			}
		}
		if count == 1 && selected.Value != nil {
			return selected.Value, nil
		}
		problem := structureError(CodeSource, "Не удалось однозначно определить "+label, entry, nil, state)
		if selected != nil {
			problem.Context = sourceContext(state.input, entry, selected.Start)
		}
		return nil, problem
	}
	if entry.Source.LineType == model.LineTypeHeading {
		level, problem := requiredValue(model.ElementTypeHeadingLevel, "уровень заголовка")
		if problem != nil {
			return false, nil, []*diagnostic.Error{problem}
		}
		if *level != "3" {
			return false, nil, nil
		}
		title, problem := requiredValue(model.ElementTypeTitle, "название страницы")
		if problem != nil {
			return false, nil, []*diagnostic.Error{problem}
		}
		return true, title, nil
	}
	if entry.Source.LineType != model.LineTypeTag && entry.Source.LineType != model.LineTypeBlockStart {
		return false, nil, nil
	}
	tag, problem := requiredValue(model.ElementTypeTag, "имя тега")
	if problem != nil {
		return false, nil, []*diagnostic.Error{problem}
	}
	return *tag == "newpage", nil, nil
}

// openPage завершает все активные task/step/variant и открывает страницу.
// Перед первым heading 3 начальную страницу создаёт только ранее размещённый
// материал. Для первого newpage сохраняет начальную страницу и открывает
// следующую: newpage — граница, завершающая предшествующую страницу.
// Последующие явные границы всегда сохраняются; пустые явно созданные страницы
// не склеиваются и не запрещаются повторным применением старой DSL-валидации.
// Title равен переданному названию либо nil; ID пуст, Nodes — ненулевой срез,
// допускающий ноль элементов. Само событие не становится нодой.
func openPage(state *builder, entry *fragments.Entry, title *string) {
	closeFrom(state, 0)
	if entry.Source.LineType != model.LineTypeHeading {
		ensurePage(state)
	}
	if title != nil {
		value := *title
		title = &value
	}
	state.pages = append(state.pages, lesson.Page{Title: title, Nodes: make([]lesson.Node, 0)})
	state.page = len(state.pages) - 1
}

// ensurePage создаёт начальную страницу с Title=nil только при отсутствии
// текущей страницы. Вызывается перед первой отображаемой нодой.
func ensurePage(state *builder) {
	if state.page >= 0 {
		return
	}
	state.pages = append(state.pages, lesson.Page{Nodes: make([]lesson.Node, 0)})
	state.page = len(state.pages) - 1
}

// finishPages обеспечивает одну страницу с Title=nil, если границ и материала
// не было вообще; уже сформированные страницы и их названия не меняет.
// Ни название документа, ни заголовки 1/2 не используются вместо page.title.
func finishPages(state *builder) {
	ensurePage(state)
}
