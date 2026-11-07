package revision

import "github.com/verdoga/dsl-builder/lesson"

// cloneLesson создаёт независимый снимок через clonePages и копирование
// указателя Subtitle. Скаляры сохраняются точно, включая json.Number.
// nil входа обрабатывается вызывающим API до копирования.
func cloneLesson(value *lesson.Lesson) *lesson.Lesson {
	result := *value
	if value.Document.Subtitle != nil {
		subtitle := *value.Document.Subtitle
		result.Document.Subtitle = &subtitle
	}
	result.Document.Pages = clonePages(value.Document.Pages)
	return &result
}

// clonePages копирует страницы, указатели Title и деревья через cloneNodes.
// Пустые ненулевые массивы остаются ненулевыми; nil также сохраняется.
func clonePages(values []lesson.Page) []lesson.Page {
	if values == nil {
		return nil
	}
	result := make([]lesson.Page, len(values))
	for i, value := range values {
		result[i] = value
		if value.Title != nil {
			title := *value.Title
			result[i].Title = &title
		}
		result[i].Nodes = cloneNodes(value.Nodes)
	}
	return result
}

// cloneNodes копирует каждый уровень дерева, все указатели Flags и Data
// через cloneData. Не нормализует текст, роли, флаги и порядок массивов.
func cloneNodes(values []lesson.Node) []lesson.Node {
	if values == nil {
		return nil
	}
	result := make([]lesson.Node, len(values))
	for i, value := range values {
		result[i] = value
		if value.Flags.Speaking != nil {
			speaking := *value.Flags.Speaking
			result[i].Flags.Speaking = &speaking
		}
		if value.Flags.Stepped != nil {
			stepped := *value.Flags.Stepped
			result[i].Flags.Stepped = &stepped
		}
		if value.Flags.Checkable != nil {
			checkable := *value.Flags.Checkable
			result[i].Flags.Checkable = &checkable
		}
		result[i].Data = cloneData(value.Data)
		result[i].Nodes = cloneNodes(value.Nodes)
	}
	return result
}

// cloneData копирует Answers, Role и вложенные []lesson.Data в Value.
// string и EmbeddedFile не содержат изменяемых ссылок и копируются по значению.
// Неизвестный вариант Value остаётся для диагностики lesson.Validate;
// до её успеха исходный Value не изменяется и результат не публикуется.
// Base64 не декодируется, не перекодируется и сохраняется точно.
func cloneData(values []lesson.Data) []lesson.Data {
	if values == nil {
		return nil
	}
	result := make([]lesson.Data, len(values))
	for i, value := range values {
		result[i] = value
		if value.Role != nil {
			role := *value.Role
			result[i].Role = &role
		}
		if value.Answers != nil {
			result[i].Answers = make([]string, len(value.Answers))
			copy(result[i].Answers, value.Answers)
		}
		if nested, ok := value.Value.([]lesson.Data); ok {
			result[i].Value = cloneData(nested)
		}
	}
	return result
}
