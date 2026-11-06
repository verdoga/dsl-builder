package lesson

import (
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// validateData проверяет дискриминатор и конкретный тип Value, ненулевой
// массив Answers, известную роль при её наличии и локальные ограничения роли.
// Для EmbeddedFile вызывает validateEmbeddedFile, для коллекции проверяет
// роли непосредственных детей и рекурсивно каждый элемент. Глубина не
// ограничивается произвольным пределом. Текст и ответы не разбираются как DSL.
func validateData(value Data, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	valueProblem := validateDataValueType(value, pointer, context)
	knownType := value.Type == DataText || value.Type == DataMedia || value.Type == DataCollection
	if !knownType {
		problems = append(problems, valueProblem)
	}
	problems = append(problems, validateDataRole(value, pointer, context)...)
	if knownType && valueProblem != nil {
		problems = append(problems, valueProblem)
	}
	if valueProblem == nil {
		switch content := value.Value.(type) {
		case EmbeddedFile:
			problems = append(problems, validateEmbeddedFile(content, pointer+"/value", context)...)
		case []Data:
			roleProblems := validateCollectionRoles(value.Role, content, pointer+"/value", context)
			roleIndex := 0
			for index, child := range content {
				childPointer := pointer + "/value/" + strconv.Itoa(index)
				childProblems := validateData(child, childPointer, context)
				// Ограничение непосредственного родителя относится к role:
				// оно следует за type ребёнка, но перед его value и потомками.
				typeCount := 0
				for typeCount < len(childProblems) && childProblems[typeCount].Details[0].Value == childPointer+"/type" {
					typeCount++
				}
				problems = append(problems, childProblems[:typeCount]...)
				if roleIndex < len(roleProblems) && roleProblems[roleIndex].Details[0].Value == childPointer+"/role" {
					problems = append(problems, roleProblems[roleIndex])
					roleIndex++
				}
				problems = append(problems, childProblems[typeCount:]...)
			}
		}
	}
	if value.Answers == nil {
		problems = append(problems, contractError(CodeContract, "Массив ответов не должен быть null", pointer+"/answers", context, nil))
	}
	return problems
}

// validateDataValueType требует string, EmbeddedFile или ненулевой []Data
// строго по Type. Неизвестный Type, nil и другие конкретные Go-типы ошибочны.
// Типы элементов Answers и Hint обеспечены Go-моделью и checkDataJSON.
func validateDataValueType(value Data, pointer string, context diagnostic.Context) *diagnostic.Error {
	switch value.Type {
	case DataText:
		if _, ok := value.Value.(string); !ok {
			return contractError(CodeContract, "Текстовые данные должны содержать строку", pointer+"/value", context, nil)
		}
	case DataMedia:
		if _, ok := value.Value.(EmbeddedFile); !ok {
			return contractError(CodeContract, "Медиаданные должны содержать встроенный файл", pointer+"/value", context, nil)
		}
	case DataCollection:
		if children, ok := value.Value.([]Data); !ok || children == nil {
			return contractError(CodeContract, "Коллекция должна содержать ненулевой массив данных", pointer+"/value", context, nil)
		}
	default:
		return contractError(CodeContract, "Неизвестный тип данных", pointer+"/type", context, nil)
	}
	return nil
}

// validateDataRole проверяет enum роли; nil сам по себе допустим.
// items, options, positions требуют DataCollection; instruction, item,
// option, position требуют DataText. options и positions содержат минимум
// два элемента. Прочие роли не ограничивают тип сверх правил схемы.
func validateDataRole(value Data, pointer string, context diagnostic.Context) []*diagnostic.Error {
	if value.Role == nil {
		return nil
	}
	var requiredType DataType
	switch *value.Role {
	case RoleItems, RoleOptions, RolePositions:
		requiredType = DataCollection
	case RoleInstruction, RoleItem, RoleOption, RolePosition:
		requiredType = DataText
	case RoleTitle, RoleBody, RoleBefore, RoleAfter, RoleHint, RoleScript, RoleSubtitles:
		return nil
	default:
		return []*diagnostic.Error{contractError(CodeContract, "Неизвестная роль данных", pointer+"/role", context, nil)}
	}
	// При неизвестном дискриминаторе его собственная ошибка достаточна;
	// конкретное содержимое Value не используется для вывода о роли.
	if value.Type != DataText && value.Type != DataMedia && value.Type != DataCollection {
		return nil
	}
	if value.Type != requiredType {
		return []*diagnostic.Error{contractError(CodeContract, "Тип данных не соответствует роли", pointer+"/type", context, nil)}
	}
	if *value.Role == RoleOptions || *value.Role == RolePositions {
		if children, ok := value.Value.([]Data); ok && children != nil && len(children) < 2 {
			return []*diagnostic.Error{contractError(CodeContract, "Коллекция вариантов или позиций должна содержать не менее двух элементов", pointer+"/value", context, nil)}
		}
	}
	return nil
}

// validateCollectionRoles проверяет непосредственных детей коллекции:
// items требует роль item у каждого, options — option, positions — position.
// У других коллекций роль необязательна, но item, option и position запрещены.
// В любой коллекции запрещены instruction, options и positions.
// Роль items внутри других коллекций сама по себе допустима по схеме.
func validateCollectionRoles(parentRole *DataRole, children []Data, pointer string, context diagnostic.Context) []*diagnostic.Error {
	var expectedRole DataRole
	if parentRole != nil {
		switch *parentRole {
		case RoleItems:
			expectedRole = RoleItem
		case RoleOptions:
			expectedRole = RoleOption
		case RolePositions:
			expectedRole = RolePosition
		}
	}
	var problems []*diagnostic.Error
	for index, child := range children {
		childPointer := pointer + "/" + strconv.Itoa(index) + "/role"
		if child.Role != nil && (*child.Role == RoleInstruction || *child.Role == RoleOptions || *child.Role == RolePositions) {
			problems = append(problems, contractError(CodeContract, "Эта роль запрещена внутри коллекции", childPointer, context, nil))
			continue
		}
		if expectedRole != "" {
			if child.Role == nil || *child.Role != expectedRole {
				problems = append(problems, contractError(CodeContract, "Роль элемента не соответствует непосредственной коллекции", childPointer, context, nil))
			}
			continue
		}
		if child.Role != nil && (*child.Role == RoleItem || *child.Role == RoleOption || *child.Role == RolePosition) {
			problems = append(problems, contractError(CodeContract, "Роль элемента разрешена только в соответствующей непосредственной коллекции", childPointer, context, nil))
		}
	}
	return problems
}
