package textdata

import (
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed/model"
)

// buildTextBlock координирует declarationTitle, inlineText и joinBody
// для text, note, alt, script, hint, answerKey, wordList и table.
// Необязательный title предшествует body. Однострочный content берётся
// из декларации; у блочной формы body содержит только переданные lines.
// Для самостоятельного текста declaration=nil и body берётся из lines.
// Несколько строк дают один body с переводами \n; пустые исходные строки
// отбрасываются. При отсутствии названия и содержимого возвращается пустой
// ненулевой срез; пустое тело не синтезируется как исключение multifill.
// Таблица, HTML, текст script, ключ и список слов остаются строками;
// ячейки, реплики, ответы, разметка и точки с запятой не разбираются.
func buildTextBlock(kind lesson.NodeType, declaration *model.Line, lines []model.Line, context diagnostic.Context) ([]lesson.Data, *diagnostic.Error) {
	result := make([]lesson.Data, 0, 2)
	if declaration != nil {
		title, err := declarationTitle(kind, *declaration, context)
		if err != nil {
			return nil, err
		}
		if title != nil {
			result = append(result, textDatum(lesson.RoleTitle, *title, ""))
		}
	}
	var body string
	var err *diagnostic.Error
	if declaration != nil && declaration.LineType != model.LineTypeBlockStart {
		body, _, err = inlineText(*declaration, context)
	} else {
		body, _, err = joinBody(lines, false, context)
	}
	if err != nil {
		return nil, err
	}
	if body != "" {
		result = append(result, textDatum(lesson.RoleBody, body, ""))
	}
	return result, nil
}

// inlineText выбирает только content и placeholder декларации и вызывает
// composeElements. Возвращает текст и наличие содержательных элементов;
// отсутствие content не подменяется title или полным Raw объявления.
// tag, title, identifier, name и скобки в текст не включаются.
// Наличие элемента с пустым Value отличается от отсутствия элемента.
func inlineText(line model.Line, context diagnostic.Context) (string, bool, *diagnostic.Error) {
	elements := make([]model.Element, 0, len(line.Elements))
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeContent || element.ElementType == model.ElementTypePlaceholder {
			elements = append(elements, element)
		}
	}
	value, err := composeElements(line, elements, context)
	if err != nil {
		return "", false, err
	}
	return value, len(elements) != 0, nil
}

// joinBody соединяет значения bodyLine ровно одним \n между сохранёнными
// физическими строками, не использует EOL и не добавляет завершающий перенос.
// keepEmpty=true применяется только к multifill: сохраняются все пустые
// строки, включая начальные, конечные и последовательные; они естественно
// могут образовать начальный или конечный \n в результате соединения.
// При false пустые строки отбрасываются; непустая разметка, например <br>,
// сохраняется буквально. Номера строк не используются для заполнения дыр.
// bool результата обозначает наличие хотя бы одной сохранённой строки.
func joinBody(lines []model.Line, keepEmpty bool, context diagnostic.Context) (string, bool, *diagnostic.Error) {
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		value, err := bodyLine(line, context)
		if err != nil {
			return "", false, err
		}
		if keepEmpty || value != "" {
			values = append(values, value)
		}
	}
	return strings.Join(values, "\n"), len(values) != 0, nil
}

// bodyLine возвращает пустую строку для LineTypeBlank, строку --- для
// нормативного LineTypeSeparator и composeElements для LineTypeContent.
// Separator сохраняется внутри table; example удаляет его до joinBody.
// Теги, границы блоков и иные нетекстовые строки здесь дают CodeInput.
// Content с пустым Value является корректной пустой строкой; отсутствие
// нужных элементов не заменяется чтением полного Raw как обычного текста.
func bodyLine(line model.Line, context diagnostic.Context) (string, *diagnostic.Error) {
	switch line.LineType {
	case model.LineTypeBlank:
		return "", nil
	case model.LineTypeSeparator:
		return "---", nil
	case model.LineTypeContent:
		value, present, err := inlineText(line, context)
		if err != nil {
			return "", err
		}
		if !present {
			return "", dataError(CodeInput, "В строке содержимого отсутствуют текстовые элементы", &line, nil, context)
		}
		return value, nil
	default:
		return "", dataError(CodeInput, "Строка не является текстовым содержимым", &line, nil, context)
	}
}

// composeElements соединяет выбранные content и placeholder в исходном
// порядке. Для content берёт Value без повторного снятия экранирования;
// для placeholder берёт полный Raw, никогда не его Value с ответом.
// Пустое Value content допустимо, nil даёт CodeMissingValue.
// Промежутки между выбранными диапазонами сохраняет через elementGap;
// за пределами первого и последнего выбранного элемента Raw не добавляет.
// Не вставляет разделительные пробелы от себя и не склеивает через теги.
// Все специальные записи multifill остаются целыми; ответы не извлекаются.
func composeElements(line model.Line, elements []model.Element, context diagnostic.Context) (string, *diagnostic.Error) {
	var result strings.Builder
	for index, element := range elements {
		if index != 0 {
			gap, err := elementGap(line, elements[index-1], element, context)
			if err != nil {
				return "", err
			}
			result.WriteString(gap)
		}
		switch element.ElementType {
		case model.ElementTypeContent:
			if element.Value == nil {
				return "", dataError(CodeMissingValue, "Неизвестно значение текстового элемента", &line, &element, context)
			}
			result.WriteString(*element.Value)
		case model.ElementTypePlaceholder:
			result.WriteString(element.Raw)
		default:
			return "", dataError(CodeInput, "Выбранный элемент не является текстом или пропуском", &line, &element, context)
		}
	}
	return result.String(), nil
}

// elementGap возвращает непокрытый промежуток Line.Raw между End левого
// и Start правого элемента, считая колонки в кодовых точках Unicode с 1.
// Промежуток допустим только из U+0020/TAB; непокрытый содержательный текст,
// пересечение или выход диапазона за строку дают CodeInput без догадок.
// Никакой повторной обработки экранирования или нормализации пробелов нет.
func elementGap(line model.Line, left model.Element, right model.Element, context diagnostic.Context) (string, *diagnostic.Error) {
	raw := []rune(line.Raw)
	for _, element := range []model.Element{left, right} {
		if element.Start < 1 || element.End < element.Start || element.End > len(raw)+1 {
			return "", dataError(CodeInput, "Диапазон текстового элемента выходит за строку", &line, &element, context)
		}
	}
	if left.End > right.Start {
		err := dataError(CodeInput, "Диапазоны текстовых элементов пересекаются или нарушают исходный порядок", &line, &right, context)
		for _, element := range []model.Element{left, right} {
			place := context
			place.Line, place.Column = line.Number, element.Start
			err.Related = append(err.Related, place)
		}
		return "", err
	}
	gap := raw[left.End-1 : right.Start-1]
	for _, character := range gap {
		if character != ' ' && character != '\t' {
			return "", dataError(CodeInput, "Между текстовыми элементами находится непокрытое содержимое", &line, &right, context)
		}
	}
	return string(gap), nil
}

// textDatum создаёт текстовый Data с переданной ролью, значением и hint,
// пустым ненулевым Answers и новым указателем Role. Используется только
// для title, body, before и after; выходные типы и роли не переобъявляются.
func textDatum(role lesson.DataRole, value string, hint string) lesson.Data {
	return lesson.Data{
		Type:    lesson.DataText,
		Role:    &role,
		Value:   value,
		Answers: make([]string, 0),
		Hint:    hint,
	}
}
