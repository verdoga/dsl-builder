package module

import "github.com/verdoga/dsl-builder/lesson"

// cloneLesson копирует FormatVersion и Document, делегируя массивы страниц
// clonePage. Копирует Subtitle; сохраняет json.Number без преобразования.
// Предусловие: lesson.Validate успешна; nil возвращает nil.
// Отдельные функции нужны для изоляции модели от сохранённых исходных байтов;
// сериализация/декодирование для копирования и новые типы модели не нужны.
func cloneLesson(value *lesson.Lesson) *lesson.Lesson {
	if value == nil {
		return nil
	}
	result := *value
	if value.Document.Subtitle != nil {
		subtitle := *value.Document.Subtitle
		result.Document.Subtitle = &subtitle
	}
	if value.Document.Pages != nil {
		result.Document.Pages = make([]lesson.Page, len(value.Document.Pages))
		for index, page := range value.Document.Pages {
			result.Document.Pages[index] = clonePage(page)
		}
	}
	return &result
}

// clonePage копирует Title и каждый Node через cloneNode, сохраняя порядок
// и различие nil/пустого среза. Текст названия не нормализует.
func clonePage(value lesson.Page) lesson.Page {
	result := value
	if value.Title != nil {
		title := *value.Title
		result.Title = &title
	}
	if value.Nodes != nil {
		result.Nodes = make([]lesson.Node, len(value.Nodes))
		for index, node := range value.Nodes {
			result.Nodes[index] = cloneNode(node)
		}
	}
	return result
}

// cloneNode копирует указатели Flags, Data через cloneData и дочерние Nodes
// через тот же шаг. Сохраняет ID и весь порядок, без ограничения глубины.
func cloneNode(value lesson.Node) lesson.Node {
	result := value
	if value.Flags.Speaking != nil {
		speaking := *value.Flags.Speaking
		result.Flags.Speaking = &speaking
	}
	if value.Flags.Stepped != nil {
		stepped := *value.Flags.Stepped
		result.Flags.Stepped = &stepped
	}
	if value.Flags.Checkable != nil {
		checkable := *value.Flags.Checkable
		result.Flags.Checkable = &checkable
	}
	if value.Data != nil {
		result.Data = make([]lesson.Data, len(value.Data))
		for index, data := range value.Data {
			result.Data[index] = cloneData(data)
		}
	}
	if value.Nodes != nil {
		result.Nodes = make([]lesson.Node, len(value.Nodes))
		for index, node := range value.Nodes {
			result.Nodes[index] = cloneNode(node)
		}
	}
	return result
}

// cloneData копирует Role, Answers и Value проверенной модели: string,
// lesson.EmbeddedFile либо рекурсивный срез lesson.Data. Сохраняет Hint,
// текст и Base64 буквально, не разбирает DSL и не вычисляет флаги.
func cloneData(value lesson.Data) lesson.Data {
	result := value
	if value.Role != nil {
		role := *value.Role
		result.Role = &role
	}
	if value.Answers != nil {
		result.Answers = make([]string, len(value.Answers))
		copy(result.Answers, value.Answers)
	}
	switch data := value.Value.(type) {
	case string:
		result.Value = data
	case lesson.EmbeddedFile:
		result.Value = data
	case []lesson.Data:
		var elements []lesson.Data
		if data != nil {
			elements = make([]lesson.Data, len(data))
			for index, element := range data {
				elements[index] = cloneData(element)
			}
		}
		result.Value = elements
	}
	return result
}
