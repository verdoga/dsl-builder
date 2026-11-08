// Package structure строит страницы и семантическое дерево нод из проверенного
// Contract 1.0 после fragments.Expand. Проект v2, только объявления и контракты.
// Использует готовые lesson.Page, Node, NodeType и NodeFlags, не вводит второй
// модели урока. Не читает TXT/файлы, не разбирает DSL и не изменяет исходники.
// Собственные data, ответы, ресурсы, checkable, ID и ревизии формируют другие
// этапы. Пустые Data здесь — заготовки, а не готовые данные упражнений.
package structure

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// Binding связывает одну итоговую ноду с её входным материалом вне JSON.
// Нужен для передачи данных следующим этапам без повторного построения дерева.
// Адрес через индексы не зависит от перемещения срезов при добавлении нод.
type Binding struct {
	// Page — индекс страницы с нуля, не номер processing ID.
	Page int

	// NodePath — непустая цепочка индексов Nodes с нуля от страницы к ноде.
	// Не изменяется после Build; путь не ограничивается фиксированной глубиной.
	NodePath []int

	// Entries — исходные вхождения собственных данных в порядке входа.
	// Первое — объявление либо самостоятельная строка content. У контейнеров
	// task/step/variant/variants содержит только объявление, без дочерних нод.
	// У листового блока — объявление, полное тело и его block-end, включая
	// blank, separator, внутренние answer/hint и их границы. У question также
	// содержит внешний связанный answer и следующий связанный hint целиком.
	// У выделенной instruction содержит только объявление multifill/ordering/matching.
	// Записи доступны лишь для чтения; *Entry различает экземпляры включений.
	Entries []*fragments.Entry
}

// Build принимает ненулевой input из parsed.Load и полный результат успешного
// fragments.Expand именно этого input. Координирует walkSequence, finishPages
// и finishTaskFlags; преобразования данных и вызова lesson.Validate здесь нет.
// Возвращает страницы и привязки в порядке «родитель, затем дети», по страницам.
// У каждой ноды ровно одна привязка. Все ID пусты, Data/Nodes — ненулевые срезы;
// флаги заполнены только у task. Выход пока не является готовым уроком схемы.
// convert переносит страницы в lesson.Document, использует Binding для вызова
// владельцев данных/ресурсов, затем поручает ID и финальную проверку их владельцам.
// До окончания заполнения нельзя менять количество, порядок и вложенность нод:
// это нарушит адреса Binding. Изменение Data, Flags и ID адреса не нарушает.
// Успех: ненулевые pages/bindings и nil ошибок; ошибка: оба результата nil.
// Работа атомарна для вызывающего кода, повторные вызовы независимы; нет
// глобального состояния и искусственных лимитов числа нод, страниц и глубины.
func Build(input *parsed.Input, entries []*fragments.Entry) (pages []lesson.Page, bindings []Binding, errors []*diagnostic.Error) {
	state := builder{
		input:    input,
		pages:    make([]lesson.Page, 0),
		bindings: make([]Binding, 0),
		page:     -1,
	}
	if errors := walkSequence(&state, entries, nil); len(errors) != 0 {
		return nil, nil, errors
	}
	finishPages(&state)
	finishTaskFlags(&state)
	return state.pages, state.bindings, nil
}

// Node возвращает адрес ноды по привязке из того же успешного Build.
// Предусловие: структура pages с момента Build не изменена. Поиск по ID,
// исходному номеру строки либо отображаемому идентификатору не применяется.
func (binding Binding) Node(pages []lesson.Page) *lesson.Node {
	node := &pages[binding.Page].Nodes[binding.NodePath[0]]
	for _, index := range binding.NodePath[1:] {
		node = &node.Nodes[index]
	}
	return node
}

// Lines предоставляет Entries.Source в исходном порядке для API владельцев
// данных, использующих parsed/model. Все поля, nullable-значения, Raw, EOL,
// элементы и физические координаты сохраняются; повторного экранирования нет.
// Результат и его вложенные данные доступны только для чтения. Такая полная
// проекция совместима с lines принятого exercisedata.Build для пяти упражнений.
func (binding Binding) Lines() []model.Line {
	lines := make([]model.Line, len(binding.Entries))
	for i, entry := range binding.Entries {
		lines[i] = entry.Source
	}
	return lines
}

// TextSource отделяет декларацию от собственных строк для textdata.Build:
// у самостоятельного text возвращает nil и его строки; у объявленного блока
// исключает только собственную декларацию и её последний block-end, сохраняя
// вложенный hint инструкции целиком. Для однострочных и структурных нод тело пусто.
// Для выделенной instruction Entries содержит только исходное объявление
// multifill/ordering/matching: оно передаётся как declaration, а lines остаётся пустым.
// convert вызывает textdata.Build с фактическим NodeInstruction; извлечение
// текста content/placeholder выполняет textdata, не structure и не convert.
// Возвращаемые model.Line исходные и доступны только для чтения; title/body,
// ответы, переносы и экранирование здесь не вычисляются. Метод применяется
// только для типов, поддерживаемых textdata; упражнению передают полную Lines.
func (binding Binding) TextSource() (declaration *model.Line, lines []model.Line) {
	full := binding.Lines()
	if len(full) == 0 || full[0].LineType == model.LineTypeContent {
		return nil, full
	}
	declaration = &full[0]
	if declaration.LineType != model.LineTypeBlockStart {
		return declaration, full[1:1]
	}
	end := len(full)
	if end > 1 && full[end-1].LineType == model.LineTypeBlockEnd &&
		binding.Entries[end-1].Parent == binding.Entries[0] {
		end--
	}
	return declaration, full[1:end]
}
