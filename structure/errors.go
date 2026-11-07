package structure

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/parsed"
)

const (
	// CodeSource — исходная конструкция не позволяет однозначно выбрать структуру.
	CodeSource = "structure.source"

	// CodeBoundary — отсутствует либо противоречива необходимая граница блока.
	CodeBoundary = "structure.boundary"

	// CodeContext — step/variant/speaking/endtask не имеет нужного контейнера
	// в своей области, у ближайшего variants нет собственной активной ветви
	// либо служебный answer не принадлежит потреблённой конструкции данных.
	CodeContext = "structure.context"
)

// sourceContext заполняет путь технического JSON из Input.Path, исходный ID
// из метаданных и физическую строку Entry.Source.Number; column известна
// из элемента либо равна 0. PageID не выдумывается до назначения ID.
func sourceContext(input *parsed.Input, entry *fragments.Entry, column int) diagnostic.Context {
	context := diagnostic.Context{Path: input.Path}
	if id := input.Result.Document.Metadata.DocumentID; id != nil {
		context.LessonID = *id
	}
	if entry != nil {
		context.Line = entry.Source.Number
		context.Column = column
	}
	return context
}

// structureError создаёт diagnostic.Error с кодом пакета и русской причиной.
// Context содержит место исходной проблемы, Related — известные объявления,
// границы и всю цепочку мест include для конкретного повторного вхождения.
// Диагностики идут в порядке развёрнутого входа; предупреждения и рекомендации
// парсера не копируются. Ошибка блокирует весь Build, наружу ничего не печатается.
func structureError(code string, reason string, entry *fragments.Entry, related []*fragments.Entry, state *builder) *diagnostic.Error {
	context := func(source *fragments.Entry) diagnostic.Context {
		column := 0
		if source != nil && len(source.Source.Elements) != 0 {
			column = source.Source.Elements[0].Start
		}
		return sourceContext(state.input, source, column)
	}
	problem := &diagnostic.Error{Code: code, Reason: reason, Context: context(entry)}
	seen := map[*fragments.Entry]bool{entry: true}
	addRelated := func(source *fragments.Entry) {
		if source != nil && !seen[source] {
			seen[source] = true
			problem.Related = append(problem.Related, context(source))
		}
	}
	for _, source := range related {
		addRelated(source)
	}
	// Parent ведёт через границы именно этого вхождения, даже когда физические
	// строки другой вставки совпадают. Included ненулевой только у include.
	for _, source := range append([]*fragments.Entry{entry}, related...) {
		if source == nil {
			continue
		}
		for parent := source.Parent; parent != nil; parent = parent.Parent {
			if parent.Included != nil {
				addRelated(parent)
			}
		}
	}
	return problem
}
