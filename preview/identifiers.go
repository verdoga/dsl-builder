package preview

import (
	"errors"
	"strings"

	"github.com/verdoga/dsl-builder/module"
)

// pageIDs возвращает точные ID всех страниц в порядке переданного снимка.
// Пустой список страниц допустим; это извлечение для показа, не генерация ID.
func pageIDs(file *module.File) ([]string, error) {
	if file == nil {
		return nil, errors.New("Для показа ID страниц нужен снимок урока")
	}
	snapshot := file.Lesson()
	if snapshot == nil {
		return nil, errors.New("Для показа ID страниц нужен снимок урока")
	}
	ids := make([]string, len(snapshot.Document.Pages))
	for index, page := range snapshot.Document.Pages {
		ids[index] = page.ID
	}
	return ids, nil
}

// nodeIDs возвращает все полные ID нод выбранной страницы: родитель, затем
// дети в порядке массива, без ограничения глубины. Data не читаются.
// Пустая страница даёт пустой список; недоступный индекс даёт error.
func nodeIDs(file *module.File, pageIndex int) ([]string, error) {
	if file == nil {
		return nil, errors.New("Для показа ID нод нужен снимок урока")
	}
	snapshot := file.Lesson()
	if snapshot == nil {
		return nil, errors.New("Для показа ID нод нужен снимок урока")
	}
	if pageIndex < 0 || pageIndex >= len(snapshot.Document.Pages) {
		return nil, errors.New("Страница для показа ID нод недоступна")
	}
	nodes := snapshot.Document.Pages[pageIndex].Nodes
	pending := nodes[:0:0]
	for index := len(nodes) - 1; index >= 0; index-- {
		pending = append(pending, nodes[index])
	}
	ids := make([]string, 0)
	for len(pending) > 0 {
		last := len(pending) - 1
		node := pending[last]
		pending = pending[:last]
		ids = append(ids, node.ID)
		for index := len(node.Nodes) - 1; index >= 0; index-- {
			pending = append(pending, node.Nodes[index])
		}
	}
	return ids, nil
}

// nodeID возвращает точный ID по индексу страницы и пути ноды из compare.
// Пустой либо недоступный путь даёт error, без подбора ближайшей ноды.
func nodeID(file *module.File, pageIndex int, path []int) (string, error) {
	if file == nil {
		return "", errors.New("Для показа ID ноды нужен снимок урока")
	}
	snapshot := file.Lesson()
	if snapshot == nil {
		return "", errors.New("Для показа ID ноды нужен снимок урока")
	}
	if pageIndex < 0 || pageIndex >= len(snapshot.Document.Pages) {
		return "", errors.New("Страница для показа ID ноды недоступна")
	}
	if len(path) == 0 {
		return "", errors.New("Для показа ID ноды нужен непустой путь")
	}
	nodes := snapshot.Document.Pages[pageIndex].Nodes
	var id string
	for _, index := range path {
		if index < 0 || index >= len(nodes) {
			return "", errors.New("Путь ноды для показа ID недоступен")
		}
		node := nodes[index]
		id = node.ID
		nodes = node.Nodes
	}
	return id, nil
}

// idList представляет готовые ID полностью и в исходном порядке;
// пустой список отображает словом «нет», без искусственного диапазона.
func idList(values []string) string {
	if len(values) == 0 {
		return "нет"
	}
	quoted := make([]string, len(values))
	for index, id := range values {
		quoted[index] = quotedText(id)
	}
	return strings.Join(quoted, ", ")
}
