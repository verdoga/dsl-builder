package fragments

import (
	"sort"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// definition — адрес исходного определения и его тела для повторного копирования;
// это внутренний индекс, не вторая модель строк или метаданных документа.
type definition struct {
	// Header — физический номер строки fragment в Input.Result.Lines.
	Header int

	// Identifier — исходный элемент identifier для имени и точного диапазона.
	Identifier model.Element

	// Body — номера строк тела в исходном порядке без внешнего объявления,
	// его закрывающей строки и вложенных определений с их телами.
	Body []int
}

// collectDefinitions просматривает все исходные строки, независимо от ветвей
// и будущего использования. Для fragment вызывает readIdentifier и bodyLines.
// Ключ — strings.ToLower семантического identifier.Value; без обрезания,
// Unicode-нормализации и проверки синтаксиса ID урока через identity.
// Сохраняет всех кандидатов и их исходный порядок, не перезаписывает дубль.
// Имена task/document-id не входят в пространство имён фрагментов.
func collectDefinitions(input *parsed.Input) (map[string][]*definition, []*diagnostic.Error) {
	definitions := make(map[string][]*definition)
	var problems []*diagnostic.Error
	for _, line := range input.Result.Lines {
		fragment := false
		for _, element := range line.Elements {
			if element.ElementType == model.ElementTypeTag && element.Value != nil && *element.Value == "fragment" {
				fragment = true
				break
			}
		}
		if !fragment {
			continue
		}
		identifier, problem := readIdentifier(line, sourceContext(input, line.Number, 0))
		if problem != nil {
			problems = append(problems, problem)
		}
		body, bodyProblems := bodyLines(input, line.Number)
		problems = append(problems, bodyProblems...)
		if problem == nil {
			key := strings.ToLower(*identifier.Value)
			definitions[key] = append(definitions[key], &definition{
				Header: line.Number, Identifier: identifier, Body: body,
			})
		}
	}
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Context.Line != problems[j].Context.Line {
			return problems[i].Context.Line < problems[j].Context.Line
		}
		return problems[i].Context.Column < problems[j].Context.Column
	})
	return definitions, problems
}

// readIdentifier извлекает единственный непустой identifier.Value у fragment
// или include и его исходные координаты. Проверяет только необходимую форму:
// fragment — block-start, include — tag. Отсутствие, null, пустота или несколько
// идентификаторов дают CodeShape. Имя тега читается из tag.Value, не из Raw;
// экранирование не снимается повторно, нового белого списка ID не вводится.
func readIdentifier(line model.Line, context diagnostic.Context) (model.Element, *diagnostic.Error) {
	var identifier model.Element
	identifiers := 0
	tagColumn := 0
	foundTag, validShape := false, true
	for _, element := range line.Elements {
		switch element.ElementType {
		case model.ElementTypeTag:
			if element.Value == nil || *element.Value != "fragment" && *element.Value != "include" {
				continue
			}
			if !foundTag {
				tagColumn = element.Start
			}
			foundTag = true
			if *element.Value == "fragment" && line.LineType != model.LineTypeBlockStart ||
				*element.Value == "include" && line.LineType != model.LineTypeTag {
				validShape = false
			}
		case model.ElementTypeIdentifier:
			identifiers++
			if identifiers == 1 {
				identifier = element
			}
		}
	}
	context.Line = line.Number
	name := ""
	if identifier.Value != nil {
		name = *identifier.Value
	}
	if !foundTag || !validShape {
		context.Column = tagColumn
		return model.Element{}, fragmentError(CodeShape,
			"Определение fragment должно открывать блок, а include должен быть однострочным тегом.", name, context, nil)
	}
	if identifiers != 1 || identifier.Value == nil || name == "" {
		if identifiers != 0 {
			context.Column = identifier.Start
		}
		return model.Element{}, fragmentError(CodeShape,
			"Для fragment или include требуется единственный непустой идентификатор.", name, context, nil)
	}
	return identifier, nil
}

// bodyLines выбирает по Index.Children потомков определения и сохраняет
// порядок Result.Lines. Собственная block-end определяется по parentLine,
// а не поиском текста скобки; требуется ровно одна такая строка после тела.
// Отсутствующая/лишняя граница или потомок за границей дают CodeShape.
// Внутренние границы контентных блоков и пустые строки сохраняются.
// Вложенные определения исключаются со всем поддеревом, но регистрируются
// отдельно collectDefinitions. Глубина обхода не ограничивается константой.
func bodyLines(input *parsed.Input, header int) ([]int, []*diagnostic.Error) {
	descendants := make(map[int]bool)
	excluded := make(map[int]bool)
	pending := []int{header}
	for i := 0; i < len(pending); i++ {
		parent := pending[i]
		for _, number := range input.Index.Children[parent] {
			line := input.Result.Lines[input.Index.ByNumber[number]]
			descendants[number] = true
			excluded[number] = excluded[parent]
			for _, element := range line.Elements {
				if element.ElementType == model.ElementTypeTag && element.Value != nil && *element.Value == "fragment" {
					excluded[number] = true
					break
				}
			}
			pending = append(pending, number)
		}
	}
	root := input.Result.Lines[input.Index.ByNumber[header]]
	name := ""
	for _, element := range root.Elements {
		if element.ElementType == model.ElementTypeIdentifier && element.Value != nil {
			name = *element.Value
			break
		}
	}
	body := make([]int, 0)
	var problems []*diagnostic.Error
	boundary := 0
	for _, line := range input.Result.Lines {
		if !descendants[line.Number] {
			continue
		}
		ownEnd := line.LineType == model.LineTypeBlockEnd && line.ParentLine != nil && *line.ParentLine == header
		reason := ""
		if ownEnd {
			if boundary == 0 {
				boundary = line.Number
			} else {
				reason = "Определение fragment имеет лишнюю собственную закрывающую границу."
			}
		} else if boundary != 0 {
			reason = "Потомок определения fragment расположен после его закрывающей границы."
		} else if !excluded[line.Number] {
			body = append(body, line.Number)
		}
		if reason != "" {
			problems = append(problems, fragmentError(CodeShape, reason, name,
				sourceContext(input, line.Number, 0), []diagnostic.Context{sourceContext(input, header, 0)}))
		}
	}
	if boundary == 0 {
		problems = append([]*diagnostic.Error{fragmentError(CodeShape,
			"У определения fragment отсутствует собственная закрывающая граница.", name,
			sourceContext(input, header, 0), nil)}, problems...)
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return body, nil
}

// checkDefinitionNames отклоняет несколько определений одного ключа даже
// без включений: DSL 3.12.5 требует уникальность в текущем документе.
// Выдаёт одну CodeAmbiguous на группу со всеми объявлениями в Related;
// порядок групп определяется первой физической строкой, а не обходом map.
func checkDefinitionNames(input *parsed.Input, definitions map[string][]*definition) []*diagnostic.Error {
	checked := make(map[string]bool)
	var problems []*diagnostic.Error
	for _, line := range input.Result.Lines {
		for _, element := range line.Elements {
			if element.ElementType != model.ElementTypeIdentifier || element.Value == nil {
				continue
			}
			key := strings.ToLower(*element.Value)
			candidates := definitions[key]
			if len(candidates) < 2 || checked[key] || candidates[0].Header != line.Number {
				continue
			}
			checked[key] = true
			related := make([]diagnostic.Context, 0, len(candidates))
			for _, candidate := range candidates {
				related = append(related, sourceContext(input, candidate.Header, candidate.Identifier.Start))
			}
			problems = append(problems, fragmentError(CodeAmbiguous,
				"Имя фрагмента объявлено несколько раз в текущем документе.",
				*candidates[0].Identifier.Value, related[0], related))
		}
	}
	return problems
}

// documentLines возвращает исходные номера вне всех определений и их
// поддеревьев. Определение не создаёт границу task/step или отображаемый
// материал. Не фильтрует другие строки и не меняет порядок входа.
func documentLines(input *parsed.Input) []int {
	excluded := make(map[int]bool)
	numbers := make([]int, 0, len(input.Result.Lines))
	for _, line := range input.Result.Lines {
		if line.ParentLine != nil {
			excluded[line.Number] = excluded[*line.ParentLine]
		}
		for _, element := range line.Elements {
			if element.ElementType == model.ElementTypeTag && element.Value != nil && *element.Value == "fragment" {
				excluded[line.Number] = true
				break
			}
		}
		if !excluded[line.Number] {
			numbers = append(numbers, line.Number)
		}
	}
	return numbers
}
