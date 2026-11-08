package exercisedata

import (
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// entry хранит готовые данные листа и исходные места для предметных ошибок.
// Это временное соответствие вывода источнику, а не новая модель lesson.Data;
// оно необходимо для повторов подписей, ссылок и неполной конфигурации ответов.
type entry struct {
	// data — текстовые данные с ролью body, item, option или position.
	data lesson.Data

	// source — место исходной формулировки, элемента или варианта.
	source diagnostic.Context

	// answerSource — место породившего ответы answer; nil означает его отсутствие.
	answerSource *diagnostic.Context

	// answerSources — места отдельных значений, в порядке data.Answers.
	answerSources []diagnostic.Context
}

// textData создаёт DataText заданной роли с точным value и переданными ответами
// и подсказкой. Нулевой answers заменяет только отсутствующий массив на пустой;
// строки ответов не изменяются. Срез ответов копируется для независимости.
func textData(role lesson.DataRole, value string, answers []string, hint string) lesson.Data {
	copiedAnswers := make([]string, len(answers))
	copy(copiedAnswers, answers)
	return lesson.Data{
		Type:    lesson.DataText,
		Role:    &role,
		Value:   value,
		Answers: copiedAnswers,
		Hint:    hint,
	}
}

// collectionData создаёт DataCollection с ролью items/options/positions,
// ненулевым []lesson.Data, пустыми Answers и Hint. Самих листовых ролей
// не меняет; коллекции никогда не вкладывает в другие коллекции.
func collectionData(role lesson.DataRole, entries []entry) lesson.Data {
	leaves := make([]lesson.Data, len(entries))
	for i, item := range entries {
		leaves[i] = item.data
		if item.data.Role != nil {
			leafRole := *item.data.Role
			leaves[i].Role = &leafRole
		}
		leaves[i].Answers = make([]string, len(item.data.Answers))
		copy(leaves[i].Answers, item.data.Answers)
	}
	return lesson.Data{
		Type:    lesson.DataCollection,
		Role:    &role,
		Value:   leaves,
		Answers: []string{},
		Hint:    "",
	}
}

// localInstruction извлекает необязательный content объявления
// choice/multichoice в DataText/RoleInstruction с пустыми ответами
// и подсказкой. Отсутствие даёт nil; чужие теги не принимаются.
// matching и ordering обслуживаются внешней самостоятельной instruction по контракту Build.
func localInstruction(line model.Line, context diagnostic.Context) (*lesson.Data, *diagnostic.Error) {
	tag, err := tagValue(line, context)
	if err != nil {
		return nil, err
	}
	switch tag {
	case "choice", "multichoice":
	default:
		return nil, dataError(CodeSource, "Локальная инструкция допустима только у choice и multichoice", sourceContext(context, line, nil), nil, nil)
	}
	value, found, _, err := contentValue(line, context)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	instruction := textData(lesson.RoleInstruction, value, nil, "")
	return &instruction, nil
}

// requireCount проверяет предметный минимум готового списка перед его
// помещением в обязательную коллекцию; сообщает CodeCollection с ролью,
// фактическим и требуемым количеством. Максимум не вводится.
func requireCount(role lesson.DataRole, entries []entry, minimum int, context diagnostic.Context) *diagnostic.Error {
	if len(entries) >= minimum {
		return nil
	}
	return dataError(CodeCollection, "Недостаточно элементов в обязательной коллекции", context, nil, []diagnostic.Detail{
		{Label: "Роль коллекции", Value: string(role)},
		{Label: "Фактическое количество", Value: strconv.Itoa(len(entries))},
		{Label: "Минимальное количество", Value: strconv.Itoa(minimum)},
	})
}
