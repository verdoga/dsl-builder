package fragments

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
)

// Expand принимает ненулевой Input, полученный parsed.Load без ошибок.
// Координирует collectDefinitions, checkDefinitionNames, resolveIncludes,
// checkIncludeCycles, documentLines и expandSequence; правила этих шагов
// не дублирует. Сбор определений предшествует любому включению: доступны
// определения после include и во всех ветвях текущего документа.
// Возвращает последовательность исходного порядка с отдельными экземплярами
// каждой вставки. Определения, их тела в месте объявления и собственные
// закрывающие строки отсутствуют. Остальные строки, в том числе blank,
// блоковые границы, метатеги, script и multifill, передаются без потерь.
// Строки include остаются только служебными границами Entry.Included;
// structure не создаёт для них ноды и обходит содержимое в месте вставки.
// Источник ресурсов остаётся input.Path; Document.LineCount не пересчитывается.
// Успех: непустой либо пустой ненулевой срез и nil ошибок; любая ошибка:
// nil результата и непустой список diagnostic.Error, без частичного успеха.
// Лимиты количества включений и глубины не вводятся. Повторные вызовы
// независимы, глобального изменяемого состояния и файлового ввода-вывода нет.
func Expand(input *parsed.Input) ([]*Entry, []*diagnostic.Error) {
	definitions, problems := collectDefinitions(input)
	if len(problems) != 0 {
		return nil, problems
	}
	if problems := checkDefinitionNames(input, definitions); len(problems) != 0 {
		return nil, problems
	}
	references, problems := resolveIncludes(input, definitions)
	if len(problems) != 0 {
		return nil, problems
	}
	if problems := checkIncludeCycles(input, definitions, references); len(problems) != 0 {
		return nil, problems
	}
	return expandSequence(input, documentLines(input), 0, nil, references)
}

// expandSequence координирует copyEntries, bindParents и expandInclusions
// для одного исходного набора номеров. sourceRoot=0 означает документ,
// иначе это номер определения, заменённого boundary. Для документа boundary
// равен nil. Предварительно ссылки разрешены и циклы исключены.
func expandSequence(input *parsed.Input, numbers []int, sourceRoot int, boundary *Entry, references map[int]*definition) ([]*Entry, []*diagnostic.Error) {
	entries := copyEntries(input, numbers)
	if problems := bindParents(input, entries, sourceRoot, boundary); len(problems) != 0 {
		return nil, problems
	}
	if problems := expandInclusions(input, entries, references); len(problems) != 0 {
		return nil, problems
	}
	return entries, nil
}

// expandInclusions заполняет только Included у Entry include, последовательно
// вызывая expandInclusion. Не превращает соседние строки в детей вставки.
func expandInclusions(input *parsed.Input, entries []*Entry, references map[int]*definition) []*diagnostic.Error {
	var problems []*diagnostic.Error
	for _, entry := range entries {
		target, include := references[entry.Source.Number]
		if !include {
			continue
		}
		included, inclusionProblems := expandInclusion(input, entry, target, references)
		if len(inclusionProblems) != 0 {
			problems = append(problems, inclusionProblems...)
			continue
		}
		entry.Included = included
	}
	return problems
}

// expandInclusion разворачивает новое тело через expandSequence с собственным
// boundary=site. Не переиспользует готовые Entry другого включения и не
// кэширует изменяемое содержимое. При ошибке добавляет место include в Related.
func expandInclusion(input *parsed.Input, site *Entry, target *definition, references map[int]*definition) ([]*Entry, []*diagnostic.Error) {
	entries, problems := expandSequence(input, target.Body, target.Header, site, references)
	if len(problems) != 0 {
		column := 0
		for _, element := range site.Source.Elements {
			if element.ElementType == "identifier" {
				column = element.Start
				break
			}
		}
		context := sourceContext(input, site.Source.Number, column)
		for _, problem := range problems {
			problem.Related = append(problem.Related, context)
		}
		return nil, problems
	}
	return entries, nil
}
