package exercisedata

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// answerGroup сохраняет один исходный answer, его альтернативы и общую
// подсказку до привязки к вопросу, item или группе правильных вариантов.
// Группа нужна для ограничения одного answer и копирования группового hint.
type answerGroup struct {
	// values — непустой список точных семантических значений ответа.
	values []string

	// sources — исходное место каждого values в том же порядке.
	sources []diagnostic.Context

	// source — исходное место тега answer, включая блочную форму.
	source diagnostic.Context

	// hint — связанная подсказка, пустая строка при её отсутствии.
	hint string
}

// readAnswer читает ровно один answer на at: однострочный content либо
// непустые строки его блока через contentRows, затем validateAnswerValues
// и readLinkedHint. Возвращает первый индекс после группы, включая hint.
// allowEditor относится только к промежутку между answer и связанным hint.
// Не поглощает следующий answer; допустимость повторов решает владелец.
func readAnswer(lines []model.Line, at int, allowEditor bool, context diagnostic.Context) (answerGroup, int, *diagnostic.Error) {
	if at < 0 || at >= len(lines) {
		return answerGroup{}, at, dataError(CodeSource, "Не задано объявление ответа", context, nil, nil)
	}
	line := lines[at]
	tag, err := tagValue(line, context)
	if err != nil {
		return answerGroup{}, at, err
	}
	source := sourceContext(context, line, nil)
	for i := range line.Elements {
		if line.Elements[i].ElementType == model.ElementTypeTag {
			source = sourceContext(context, line, &line.Elements[i])
			break
		}
	}
	if tag != "answer" {
		return answerGroup{}, at, dataError(CodeSource, "Ожидалось объявление answer", source, nil, nil)
	}
	group := answerGroup{source: source}
	next := at + 1
	switch line.LineType {
	case model.LineTypeTag:
		value, found, valueSource, err := contentValue(line, context)
		if err != nil {
			return answerGroup{}, at, err
		}
		if found {
			group.values = []string{value}
			group.sources = []diagnostic.Context{valueSource}
		}
	case model.LineTypeBlockStart:
		end, err := blockEnd(lines, at, context)
		if err != nil {
			return answerGroup{}, at, err
		}
		group.values, group.sources, err = contentRows(lines, at+1, end, context)
		if err != nil {
			return answerGroup{}, at, err
		}
		next = end + 1
	default:
		return answerGroup{}, at, dataError(CodeSource, "Неверная форма объявления answer", source, nil, nil)
	}
	if err := validateAnswerValues(group.values, group.sources, source); err != nil {
		return answerGroup{}, at, err
	}
	group.hint, next, err = readLinkedHint(lines, next, line.ParentLine, allowEditor, context)
	if err != nil {
		return answerGroup{}, at, err
	}
	return group, next, nil
}

// validateAnswerValues требует хотя бы одно значение и хотя бы один
// непробельный Unicode-символ в каждом. Не нормализует сохраняемые ответы,
// не удаляет пунктуацию и не сравнивает их с ответами пользователя.
// Пустой явный answer — CodeAnswer, а не отключение checkable.
func validateAnswerValues(values []string, sources []diagnostic.Context, context diagnostic.Context) *diagnostic.Error {
	if len(values) == 0 {
		return dataError(CodeAnswer, "Явный ответ должен содержать хотя бы одно значение", context, nil, nil)
	}
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			source := context
			if i < len(sources) {
				source = sources[i]
			}
			return dataError(CodeAnswer, "Значение ответа должно содержать непробельный символ", source, nil, nil)
		}
	}
	return nil
}

// readLinkedHint читает максимум один hint следующей значимой конструкцией
// на том же ParentLine, что answer; после блочного answer — после block-end.
// Содержимое однострочной формы берётся из Value, блочной — из contentRows
// и соединяется через \n без пустых исходных строк. Несвязанную конструкцию
// не поглощает. Граница проекции исключает переход через include/контейнер.
// У найденной подсказки content обязан быть известен; nil не заменяется
// пустой строкой. Повторный hint остаётся владельцу для CodeBinding.
func readLinkedHint(lines []model.Line, at int, parentLine *int, allowEditor bool, context diagnostic.Context) (hint string, next int, err *diagnostic.Error) {
	candidate, err := nextSignificant(lines, at, allowEditor, context)
	if err != nil {
		return "", at, err
	}
	if candidate == len(lines) {
		return "", at, nil
	}
	line := lines[candidate]
	if (parentLine == nil) != (line.ParentLine == nil) ||
		(parentLine != nil && *parentLine != *line.ParentLine) {
		return "", at, nil
	}
	tag, err := tagValue(line, context)
	if err != nil {
		return "", at, err
	}
	if tag != "hint" {
		return "", at, nil
	}
	switch line.LineType {
	case model.LineTypeTag:
		value, found, source, err := contentValue(line, context)
		if err != nil {
			return "", at, err
		}
		if !found {
			return "", at, dataError(CodeSource, "Не задано содержимое связанной подсказки", source, nil, nil)
		}
		return value, candidate + 1, nil
	case model.LineTypeBlockStart:
		end, err := blockEnd(lines, candidate, context)
		if err != nil {
			return "", at, err
		}
		values, _, err := contentRows(lines, candidate+1, end, context)
		if err != nil {
			return "", at, err
		}
		return strings.Join(values, "\n"), end + 1, nil
	default:
		return "", at, dataError(CodeSource, "Неверная форма объявления hint", sourceContext(context, line, nil), nil, nil)
	}
}

// attachAnswer возвращает независимый entry с альтернативами и hint группы;
// применим к body question или item. Повторный answer даёт CodeBinding.
// Источники значений сохраняются для последующей проверки ссылок.
func attachAnswer(item entry, group answerGroup) (entry, *diagnostic.Error) {
	if item.answerSource != nil || len(item.data.Answers) != 0 {
		related := []diagnostic.Context{}
		if item.answerSource != nil {
			related = append(related, *item.answerSource)
		} else {
			related = append(related, item.source)
		}
		related = append(related, group.source)
		return entry{}, dataError(CodeBinding, "Для элемента повторно задан answer", group.source, related, nil)
	}
	if item.data.Role != nil {
		role := *item.data.Role
		item.data.Role = &role
	}
	item.data.Answers = make([]string, len(group.values))
	copy(item.data.Answers, group.values)
	item.data.Hint = group.hint
	source := group.source
	item.answerSource = &source
	item.answerSources = make([]diagnostic.Context, len(group.sources))
	copy(item.answerSources, group.sources)
	return item, nil
}

// answerCoverage определяет отсутствие ответов у всех entries (false) либо
// наличие у всех (true). Частичное наличие даёт CodeIncompleteAnswers:
// Context у объявления, Related у всех элементов с ответами и без них.
// Пустой явный answer уже отклонён readAnswer; false не скрывает ошибку.
func answerCoverage(entries []entry, context diagnostic.Context) (bool, *diagnostic.Error) {
	answered := 0
	for _, item := range entries {
		if item.answerSource != nil || len(item.data.Answers) != 0 {
			answered++
		}
	}
	if answered == 0 {
		return false, nil
	}
	if answered == len(entries) {
		return true, nil
	}
	related := make([]diagnostic.Context, len(entries))
	for i, item := range entries {
		related[i] = item.source
	}
	return false, dataError(CodeIncompleteAnswers, "Ответы заданы только для части элементов", context, related, nil)
}
