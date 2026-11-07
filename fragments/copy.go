package fragments

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// copyEntries создаёт отдельный Entry для каждого номера через cloneLine,
// используя готовый Input.Index.ByNumber. Parent и Included пока не связаны.
// Номера получены из documentLines/bodyLines и существуют в проверенном Input.
// Порядок numbers сохраняется, исходные номера не перенумеровываются.
func copyEntries(input *parsed.Input, numbers []int) []*Entry {
	entries := make([]*Entry, len(numbers))
	for i, number := range numbers {
		entries[i] = &Entry{Source: cloneLine(input.Result.Lines[input.Index.ByNumber[number]])}
	}
	return entries
}

// cloneLine копирует model.Line, её ParentLine, Elements и изменяемые данные
// каждого элемента через cloneElement. Ничто изменяемое не разделяется
// с Input либо другим Entry. Строковые значения и null сохраняются точно.
func cloneLine(source model.Line) model.Line {
	cloned := source
	if source.ParentLine != nil {
		parent := *source.ParentLine
		cloned.ParentLine = &parent
	}
	if source.Elements != nil {
		cloned.Elements = make([]model.Element, len(source.Elements))
		for i, element := range source.Elements {
			cloned.Elements[i] = cloneElement(element)
		}
	}
	return cloned
}

// cloneElement копирует model.Element с отдельными Value и ErrorIDs;
// сохраняет различие nil и пустого среза, Raw и диапазон Unicode-колонок.
func cloneElement(source model.Element) model.Element {
	cloned := source
	if source.Value != nil {
		value := *source.Value
		cloned.Value = &value
	}
	if source.ErrorIDs != nil {
		cloned.ErrorIDs = make([]string, len(source.ErrorIDs))
		copy(cloned.ErrorIDs, source.ErrorIDs)
	}
	return cloned
}

// bindParents связывает Parent в пределах нового набора Entry по исходным
// parentLine. Ссылка на sourceRoot заменяется boundary; у корня документа
// parentLine=null даёт nil. Остальные ссылки обязаны разрешаться в этом наборе,
// включая block-end к своему block-start; иначе CodeParent с исходной строкой.
// Для корня тела sourceRoot — номер fragment, boundary — конкретный include.
// Нельзя оставлять ссылку на копию из другого включения или на исходный Input.
// Source.ParentLine/NestingLevel не переписываются: structure использует
// Entry.Parent для принадлежности и отдельно ведёт семантические контейнеры.
func bindParents(input *parsed.Input, entries []*Entry, sourceRoot int, boundary *Entry) []*diagnostic.Error {
	byNumber := make(map[int]*Entry, len(entries))
	for _, entry := range entries {
		byNumber[entry.Source.Number] = entry
	}
	identifier := ""
	if sourceRoot != 0 {
		root := input.Result.Lines[input.Index.ByNumber[sourceRoot]]
		for _, element := range root.Elements {
			if element.ElementType == model.ElementTypeIdentifier && element.Value != nil {
				identifier = *element.Value
				break
			}
		}
	}
	var problems []*diagnostic.Error
	for _, entry := range entries {
		entry.Parent = nil
		parent := entry.Source.ParentLine
		if parent == nil {
			continue
		}
		if *parent == sourceRoot {
			entry.Parent = boundary
			continue
		}
		if resolved, exists := byNumber[*parent]; exists {
			entry.Parent = resolved
			continue
		}
		problems = append(problems, fragmentError(CodeParent,
			"Родительская строка отсутствует в текущем экземпляре.", identifier,
			sourceContext(input, entry.Source.Number, 0), nil))
	}
	return problems
}
