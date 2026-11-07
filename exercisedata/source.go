package exercisedata

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// checkSource проверяет непустую проекцию и соответствие первого тега kind:
// question — однострочный; choice/multichoice/matching/ordering — блочные.
// Полный входной контракт, номера и отсутствие циклов уже проверены parsed.
// Проверяет завершённость локального блока через blockEnd; не достраивает
// отсутствующие строки, не принимает посторонний хвост после его закрытия.
func checkSource(kind lesson.NodeType, lines []model.Line, context diagnostic.Context) *diagnostic.Error {
	wantTag := ""
	wantType := model.LineTypeBlockStart
	switch kind {
	case lesson.NodeQuestion:
		wantTag, wantType = "question", model.LineTypeTag
	case lesson.NodeSingleChoice:
		wantTag = "choice"
	case lesson.NodeMultipleChoice:
		wantTag = "multichoice"
	case lesson.NodeMatching:
		wantTag = "matching"
	case lesson.NodeOrdering:
		wantTag = "ordering"
	default:
		return dataError(CodeKind, "Неподдерживаемый тип упражнения", context, nil, nil)
	}
	if len(lines) == 0 {
		return dataError(CodeSource, "Не задана проекция упражнения", context, nil, nil)
	}
	tag, err := tagValue(lines[0], context)
	if err != nil {
		return err
	}
	if tag != wantTag || lines[0].LineType != wantType {
		return dataError(CodeSource, "Объявление не соответствует типу упражнения", sourceContext(context, lines[0], nil), nil, nil)
	}
	if _, _, _, err := contentValue(lines[0], context); err != nil {
		return err
	}
	if wantType == model.LineTypeBlockStart {
		end, err := blockEnd(lines, 0, context)
		if err != nil {
			return err
		}
		if end != len(lines)-1 {
			return dataError(CodeSource, "После границы упражнения переданы посторонние строки", sourceContext(context, lines[end+1], nil), nil, nil)
		}
	}
	return nil
}

// blockEnd находит закрывающую строку переданного block-start по типу строки
// и ParentLine; возвращает её индекс в lines. Вложенные блоки не закрывают
// внешний. Отсутствующая граница — CodeSource, искусственного лимита нет.
func blockEnd(lines []model.Line, at int, context diagnostic.Context) (int, *diagnostic.Error) {
	if at < 0 || at >= len(lines) {
		return 0, dataError(CodeSource, "Не задано объявление блока", context, nil, nil)
	}
	context = sourceContext(context, lines[at], nil)
	if lines[at].LineType != model.LineTypeBlockStart {
		return 0, dataError(CodeSource, "Ожидалось объявление блочной конструкции", context, nil, nil)
	}
	for i := at + 1; i < len(lines); i++ {
		line := lines[i]
		if line.LineType == model.LineTypeBlockEnd && line.ParentLine != nil && *line.ParentLine == lines[at].Number {
			return i, nil
		}
	}
	return 0, dataError(CodeSource, "Не найдена закрывающая граница блока", context, nil, nil)
}

// tagValue возвращает каноническое ElementTypeTag.Value либо пустую строку
// при отсутствии тега. Nil у найденного тега — CodeSource, не пустой тег.
func tagValue(line model.Line, context diagnostic.Context) (string, *diagnostic.Error) {
	var tag *model.Element
	var sources []diagnostic.Context
	for i := range line.Elements {
		if line.Elements[i].ElementType == model.ElementTypeTag {
			tag = &line.Elements[i]
			sources = append(sources, sourceContext(context, line, tag))
		}
	}
	if len(sources) > 1 {
		return "", dataError(CodeSource, "Неоднозначное объявление тега", sources[0], sources, nil)
	}
	if tag == nil {
		return "", nil
	}
	if tag.Value == nil {
		return "", dataError(CodeSource, "Неизвестно семантическое значение тега", sources[0], nil, nil)
	}
	return *tag.Value, nil
}

// contentValue извлекает единственный ElementTypeContent.Value и его место;
// необязательное отсутствие даёт found=false, а nil и неоднозначность — ошибку.
// Непустой текст не обрезается. Значение "---" само по себе не разделитель.
func contentValue(line model.Line, context diagnostic.Context) (value string, found bool, source diagnostic.Context, err *diagnostic.Error) {
	source = sourceContext(context, line, nil)
	var content *model.Element
	var sources []diagnostic.Context
	for i := range line.Elements {
		if line.Elements[i].ElementType == model.ElementTypeContent {
			content = &line.Elements[i]
			sources = append(sources, sourceContext(context, line, content))
		}
	}
	if len(sources) > 1 {
		return "", false, sources[0], dataError(CodeSource, "Неоднозначное текстовое содержимое строки", sources[0], sources, nil)
	}
	if content == nil {
		return "", false, source, nil
	}
	source = sources[0]
	if content.Value == nil {
		return "", false, source, dataError(CodeSource, "Неизвестно семантическое значение текста", source, nil, nil)
	}
	return *content.Value, true, source, nil
}

// nextSignificant возвращает индекс следующей значимой строки либо len(lines).
// Пропускает blank; allowEditor дополнительно разрешает пропуск уже выделенного
// editor вместе с его блоком. Он true только при соседстве question/answer/hint
// на внешнем уровне; в упражнениях и телах answer/hint editor не разрешён.
// Границы блоков и любые другие теги не пропускаются; исходный порядок сохранён.
func nextSignificant(lines []model.Line, at int, allowEditor bool, context diagnostic.Context) (int, *diagnostic.Error) {
	if at < 0 || at > len(lines) {
		return at, dataError(CodeSource, "Неверная граница поиска строки", context, nil, nil)
	}
	for at < len(lines) {
		if lines[at].LineType == model.LineTypeBlank {
			at++
			continue
		}
		if !allowEditor {
			break
		}
		tag, err := tagValue(lines[at], context)
		if err != nil {
			return at, err
		}
		if tag != "editor" {
			break
		}
		switch lines[at].LineType {
		case model.LineTypeTag:
			at++
		case model.LineTypeBlockStart:
			end, err := blockEnd(lines, at, context)
			if err != nil {
				return at, err
			}
			at = end + 1
		default:
			return at, dataError(CodeSource, "Неверная форма редакторского комментария", sourceContext(context, lines[at], nil), nil, nil)
		}
	}
	return at, nil
}

// contentRows извлекает непустые content-значения тела answer или hint;
// возвращает значения и их контексты в одинаковом порядке и количестве.
// Только blank и пустые строки content отбрасываются. Иные вложенные теги
// дают CodeSource; отображаемая разметка сохраняется. Value не разбирается.
// Все содержательные строки должны иметь ParentLine объявления этого тела;
// start — первая строка после объявления, end — индекс его block-end.
func contentRows(lines []model.Line, start int, end int, context diagnostic.Context) ([]string, []diagnostic.Context, *diagnostic.Error) {
	if start < 1 || start > end || end >= len(lines) {
		return nil, nil, dataError(CodeSource, "Неверные границы текстового тела", context, nil, nil)
	}
	parent := lines[start-1]
	if parent.LineType != model.LineTypeBlockStart || lines[end].LineType != model.LineTypeBlockEnd || lines[end].ParentLine == nil || *lines[end].ParentLine != parent.Number {
		return nil, nil, dataError(CodeSource, "Границы текстового тела не соответствуют объявлению", sourceContext(context, parent, nil), nil, nil)
	}
	values := []string{}
	sources := []diagnostic.Context{}
	for i := start; i < end; i++ {
		line := lines[i]
		if line.LineType == model.LineTypeBlank {
			continue
		}
		if line.LineType != model.LineTypeContent || line.ParentLine == nil || *line.ParentLine != parent.Number {
			return nil, nil, dataError(CodeSource, "В текстовом теле допустимы только прямые строки содержимого", sourceContext(context, line, nil), nil, nil)
		}
		value, found, source, err := contentValue(line, context)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			return nil, nil, dataError(CodeSource, "Не задано содержимое текстовой строки", source, nil, nil)
		}
		if value != "" {
			values = append(values, value)
			sources = append(sources, source)
		}
	}
	return values, sources, nil
}

// isSeparator распознаёт LineTypeSeparator только среди прямых строк списков.
// Для golden 6/7 допускает прежний LineTypeContent с единственным content:
// Value="---", Raw элемента="---", Raw строки после удаления только краевых
// U+0020/TAB равен "---". Экранированный Raw="\\---" остаётся текстом.
// Не ищет разделители внутри answer/hint и не делит текст по подстрокам.
func isSeparator(line model.Line) bool {
	if line.LineType == model.LineTypeSeparator {
		return true
	}
	if line.LineType != model.LineTypeContent || len(line.Elements) != 1 {
		return false
	}
	element := line.Elements[0]
	return element.ElementType == model.ElementTypeContent && element.Value != nil && *element.Value == "---" && element.Raw == "---" && strings.Trim(line.Raw, " \t") == "---"
}
