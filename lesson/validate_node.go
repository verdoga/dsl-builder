package lesson

import (
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// validateNode проверяет Type, Flags, обязательные ненулевые срезы Data
// и Nodes, делегирует собственные данные и затем дочерние ноды.
// Пустые массивы допустимы; ограничений DSL на вложенность не добавляет.
func validateNode(value Node, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if problem := validateNodeType(value.Type, pointer+"/type", context); problem != nil {
		problems = append(problems, problem)
	}
	problems = append(problems, validateFlags(value.Type, value.Flags, pointer+"/flags", context)...)
	if value.Data == nil {
		problems = append(problems, contractError(CodeContract, "Массив данных ноды не должен быть null", pointer+"/data", context, nil))
	} else {
		problems = append(problems, validateNodeData(value.Type, value.Data, pointer+"/data", context)...)
	}
	if value.Nodes == nil {
		problems = append(problems, contractError(CodeContract, "Массив дочерних нод не должен быть null", pointer+"/nodes", context, nil))
	} else {
		for index, child := range value.Nodes {
			problems = append(problems, validateNode(child, pointer+"/nodes/"+strconv.Itoa(index), context)...)
		}
	}
	return problems
}

// validateNodeType проверяет enum всех 22 типов фиксированной схемы.
func validateNodeType(value NodeType, pointer string, context diagnostic.Context) *diagnostic.Error {
	switch value {
	case NodeTask, NodeStep, NodeVariants, NodeVariant, NodeHeading, NodeText,
		NodeInstruction, NodeNote, NodeAlt, NodeScript, NodeHint, NodeAnswerKey,
		NodeExample, NodeWordList, NodeTable, NodeMedia, NodeQuestion, NodeMultifill,
		NodeSingleChoice, NodeMultipleChoice, NodeMatching, NodeOrdering:
		return nil
	default:
		return contractError(CodeContract, "Неизвестный тип ноды", pointer, context, nil)
	}
}

// validateFlags допускает Speaking и Stepped только у task, Checkable только
// у multifill, singleChoice, multipleChoice, ordering, matching и question.
// У остальных типов все указатели nil; неприменимый указатель ошибочен
// даже при false. Все применимые флаги необязательны. Соответствие флагов
// ответам, внутренней разметке и истории преобразования здесь не вычисляется.
func validateFlags(kind NodeType, flags NodeFlags, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if kind != NodeTask {
		if flags.Speaking != nil {
			problems = append(problems, contractError(CodeContract, "Флаг speaking допустим только у task", pointer+"/speaking", context, nil))
		}
		if flags.Stepped != nil {
			problems = append(problems, contractError(CodeContract, "Флаг stepped допустим только у task", pointer+"/stepped", context, nil))
		}
	}
	if flags.Checkable != nil {
		switch kind {
		case NodeMultifill, NodeSingleChoice, NodeMultipleChoice, NodeOrdering, NodeMatching, NodeQuestion:
		default:
			problems = append(problems, contractError(CodeContract, "Флаг checkable недопустим у этого типа ноды", pointer+"/checkable", context, nil))
		}
	}
	return problems
}

// validateNodeData проверяет каждый Data, непосредственные роли и число
// коллекций отдельными функциями; сохраняет исходный порядок элементов.
func validateNodeData(kind NodeType, data []Data, pointer string, context diagnostic.Context) []*diagnostic.Error {
	if data == nil {
		return []*diagnostic.Error{contractError(CodeContract, "Массив данных ноды не должен быть null", pointer, context, nil)}
	}
	var problems []*diagnostic.Error
	countProblems := validateCollectionCounts(kind, data, pointer, context)
	countIndex := 0
	// Ограничения всего массива предшествуют проверкам его элементов.
	for countIndex < len(countProblems) && countProblems[countIndex].Details[0].Value == pointer {
		problems = append(problems, countProblems[countIndex])
		countIndex++
	}
	for index, value := range data {
		itemPointer := pointer + "/" + strconv.Itoa(index)
		itemProblems := validateData(value, itemPointer, context)
		directProblem := validateDirectRole(kind, value.Role, itemPointer+"/role", context)
		fieldIndex := 0
		for fieldIndex < len(itemProblems) && itemProblems[fieldIndex].Details[0].Value == itemPointer+"/type" {
			problems = append(problems, itemProblems[fieldIndex])
			fieldIndex++
		}
		if directProblem != nil {
			problems = append(problems, directProblem)
		}
		for fieldIndex < len(itemProblems) && itemProblems[fieldIndex].Details[0].Value == itemPointer+"/role" {
			// Не повторяем одну неизвестную роль из двух профильных проверок.
			if directProblem == nil {
				problems = append(problems, itemProblems[fieldIndex])
			}
			fieldIndex++
		}
		if countIndex < len(countProblems) && countProblems[countIndex].Details[0].Value == itemPointer+"/value" {
			problems = append(problems, countProblems[countIndex])
			countIndex++
		}
		problems = append(problems, itemProblems[fieldIndex:]...)
	}
	return problems
}

// validateDirectRole требует известную непустую роль; item, option, position
// запрещены. instruction и options разрешены только для singleChoice,
// multipleChoice, matching; positions — только для ordering.
// Другие роли не ограничиваются по типу ноды сверх фиксированной схемы.
func validateDirectRole(kind NodeType, role *DataRole, pointer string, context diagnostic.Context) *diagnostic.Error {
	if role == nil {
		return contractError(CodeContract, "Роль непосредственных данных ноды обязательна", pointer, context, nil)
	}
	switch *role {
	case RoleTitle, RoleBody, RoleBefore, RoleAfter, RoleItems, RoleHint, RoleScript, RoleSubtitles:
		return nil
	case RoleInstruction, RoleOptions:
		if kind == NodeSingleChoice || kind == NodeMultipleChoice || kind == NodeMatching {
			return nil
		}
		return contractError(CodeContract, "Эта роль допустима только непосредственно у singleChoice, multipleChoice или matching", pointer, context, nil)
	case RolePositions:
		if kind == NodeOrdering {
			return nil
		}
		return contractError(CodeContract, "Роль positions допустима только непосредственно у ordering", pointer, context, nil)
	case RoleItem, RoleOption, RolePosition:
		return contractError(CodeContract, "Роль элемента разрешена только внутри соответствующей коллекции", pointer, context, nil)
	default:
		return contractError(CodeContract, "Неизвестная роль данных", pointer, context, nil)
	}
}

// validateCollectionCounts требует ровно одну options у singleChoice,
// multipleChoice и matching, ровно одну items у matching и ordering,
// не более одной positions у ordering и instruction у трёх разрешённых типов.
// Минимум элементов прямой items: matching — один, ordering — два.
// На items других типов нод дополнительных ограничений схема не накладывает.
// Общий минимум options и positions проверяет validateDataRole.
func validateCollectionCounts(kind NodeType, data []Data, pointer string, context diagnostic.Context) []*diagnostic.Error {
	if data == nil {
		return nil
	}
	var options, positions, items, instructions int
	for _, value := range data {
		if value.Role == nil {
			continue
		}
		switch *value.Role {
		case RoleOptions:
			options++
		case RolePositions:
			positions++
		case RoleItems:
			items++
		case RoleInstruction:
			instructions++
		}
	}
	var problems []*diagnostic.Error
	choice := kind == NodeSingleChoice || kind == NodeMultipleChoice || kind == NodeMatching
	if choice && options != 1 {
		problems = append(problems, contractError(CodeContract, "Нода должна содержать ровно одну коллекцию options", pointer, context, nil))
	}
	if kind == NodeOrdering && positions > 1 {
		problems = append(problems, contractError(CodeContract, "Нода ordering допускает не более одной коллекции positions", pointer, context, nil))
	}
	if (kind == NodeMatching || kind == NodeOrdering) && items != 1 {
		problems = append(problems, contractError(CodeContract, "Нода должна содержать ровно одну коллекцию items", pointer, context, nil))
	}
	if choice && instructions > 1 {
		problems = append(problems, contractError(CodeContract, "Нода допускает не более одной локальной инструкции", pointer, context, nil))
	}
	minimum := 0
	if kind == NodeMatching {
		minimum = 1
	} else if kind == NodeOrdering {
		minimum = 2
	}
	if minimum != 0 {
		for index, value := range data {
			if value.Role == nil || *value.Role != RoleItems || value.Type != DataCollection {
				continue
			}
			// Некорректный контейнер диагностирует validateData; его длину не проверяем.
			if children, ok := value.Value.([]Data); ok && children != nil && len(children) < minimum {
				problems = append(problems, contractError(CodeContract, "В коллекции items недостаточно элементов", pointer+"/"+strconv.Itoa(index)+"/value", context, nil))
			}
		}
	}
	return problems
}
