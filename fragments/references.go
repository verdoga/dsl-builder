package fragments

import (
	"sort"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// resolveIncludes проверяет каждую исходную строку include, включая строки
// в определениях, через readIdentifier и resolveInclude. Возвращает индекс
// физического номера include в единственное определение. Разрешение имён
// ограничено одним Input; соседние файлы, метаданные и task не участвуют.
// При ошибках не возвращает пригодный частичный индекс.
func resolveIncludes(input *parsed.Input, definitions map[string][]*definition) (map[int]*definition, []*diagnostic.Error) {
	references := make(map[int]*definition)
	var problems []*diagnostic.Error
	for _, line := range input.Result.Lines {
		include := false
		for _, element := range line.Elements {
			if element.ElementType == model.ElementTypeTag && element.Value != nil && *element.Value == "include" {
				include = true
				break
			}
		}
		if !include {
			continue
		}
		context := sourceContext(input, line.Number, 0)
		identifier, problem := readIdentifier(line, context)
		if problem != nil {
			problems = append(problems, problem)
			continue
		}
		context.Column = identifier.Start
		target, problem := resolveInclude(identifier, definitions, context)
		if problem != nil {
			problems = append(problems, problem)
			continue
		}
		references[line.Number] = target
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return references, nil
}

// resolveInclude ищет по strings.ToLower(identifier.Value), сохраняя исходное
// написание. Ноль кандидатов даёт CodeMissing, несколько — CodeAmbiguous
// с полным перечнем определений в Related; первый кандидат не выбирается.
func resolveInclude(identifier model.Element, definitions map[string][]*definition, context diagnostic.Context) (*definition, *diagnostic.Error) {
	name := *identifier.Value
	candidates := definitions[strings.ToLower(name)]
	if len(candidates) == 0 {
		return nil, fragmentError(CodeMissing, "Определение включаемого фрагмента не найдено.", name, context, nil)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	related := make([]diagnostic.Context, 0, len(candidates))
	for _, candidate := range candidates {
		location := context
		location.Line, location.Column = candidate.Header, candidate.Identifier.Start
		related = append(related, location)
	}
	sort.SliceStable(related, func(i, j int) bool {
		if related[i].Line != related[j].Line {
			return related[i].Line < related[j].Line
		}
		return related[i].Column < related[j].Column
	})
	return nil, fragmentError(CodeAmbiguous, "Имя включаемого фрагмента соответствует нескольким определениям.", name, context, related)
}

// checkIncludeCycles проверяет конечность зависимостей определений по include
// в их Body. Прямой или косвенный цикл даёт CodeCycle с цепочкой исходных
// include и определений в Related. Повтор независимого использования одного
// определения не является циклом. Проверка не ограничивает глубину и число
// включений; это защита конечности развёртывания, не повторный DSL-валидатор.
// В корректном DSL 1.2 вложенные include не встречаются; неожиданный вход,
// прошедший Contract, не должен приводить к бесконечному развёртыванию.
// Порядок проверок задаётся исходными строками, не порядком map.
func checkIncludeCycles(input *parsed.Input, definitions map[string][]*definition, references map[int]*definition) []*diagnostic.Error {
	ordered := make([]*definition, 0)
	for _, candidates := range definitions {
		ordered = append(ordered, candidates...)
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Header < ordered[j].Header })
	includeIdentifier := func(number int) model.Element {
		line := input.Result.Lines[input.Index.ByNumber[number]]
		for _, element := range line.Elements {
			if element.ElementType == model.ElementTypeIdentifier {
				return element
			}
		}
		return model.Element{}
	}
	includeContext := func(number int) diagnostic.Context {
		return sourceContext(input, number, includeIdentifier(number).Start)
	}
	state := make(map[*definition]uint8)
	position := make(map[*definition]int)
	var problems []*diagnostic.Error
	for _, root := range ordered {
		if state[root] != 0 {
			continue
		}
		// via — исходный include, по которому вошли в кадр; у корня его нет.
		stack := []struct {
			target *definition
			next   int
			via    int
		}{{target: root}}
		state[root], position[root] = 1, 0
		for len(stack) != 0 {
			current := &stack[len(stack)-1]
			if current.next == len(current.target.Body) {
				state[current.target] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			number := current.target.Body[current.next]
			current.next++
			target := references[number]
			if target == nil || state[target] == 2 {
				continue
			}
			if state[target] == 0 {
				state[target], position[target] = 1, len(stack)
				stack = append(stack, struct {
					target *definition
					next   int
					via    int
				}{target: target, via: number})
				continue
			}
			start := position[target]
			related := make([]diagnostic.Context, 0, 2*(len(stack)-start))
			for i := start; i < len(stack); i++ {
				if i > start {
					related = append(related, includeContext(stack[i].via))
				}
				member := stack[i].target
				related = append(related, sourceContext(input, member.Header, member.Identifier.Start))
			}
			related = append(related, includeContext(number))
			identifier := includeIdentifier(number)
			problems = append(problems, fragmentError(CodeCycle,
				"Обнаружен цикл включений фрагментов.", *identifier.Value, includeContext(number), related))
		}
	}
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Context.Line != problems[j].Context.Line {
			return problems[i].Context.Line < problems[j].Context.Line
		}
		return problems[i].Context.Column < problems[j].Context.Column
	})
	return problems
}
