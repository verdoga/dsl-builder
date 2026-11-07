package filetree

import (
	"path/filepath"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// JSONFiles находит файлы с расширением .json без учёта регистра в каталоге.
// Координирует Walk с отбором isJSON и проверку непустого результата.
// Сортировка относительно directory и устранение повторов принадлежат Walk.
// Пустой результат — CodeEmptyJSON; ошибка одного пути запрещает результат.
// Каждый возвращённый файл вызывающий пакет обязан передать parsed;
// неподходящее содержимое не является основанием молча исключать файл здесь.
func (r *Resolver) JSONFiles(directory string) ([]Entry, *diagnostic.Error) {
	entries, err := r.Walk(directory, isJSON)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, pathError(CodeEmptyJSON, "В выбранном каталоге не найдено JSON-файлов", filepath.Clean(directory), nil)
	}
	return entries, nil
}

// JSONFile проверяет один полный путь: обычный файл с расширением .json
// без учёта регистра. Каталог не разворачивает, содержимое не проверяет.
// Режим обновления вызывает только этот метод; при добавлении выбор между
// JSONFile и JSONFiles по типу входа делает app, при создании — JSONFiles.
func (r *Resolver) JSONFile(path string) (Entry, *diagnostic.Error) {
	entry, err := r.Inspect(path)
	if err != nil {
		return Entry{}, err
	}
	if !entry.Info.Mode().IsRegular() {
		return Entry{}, pathError(CodeKind, "Для входного JSON нужен обычный файл", entry.Path, nil)
	}
	if !isJSON(entry) {
		return Entry{}, pathError(CodeExtension, "Входной файл должен иметь расширение .json", entry.Path, nil)
	}
	return entry, nil
}

// isJSON проверяет только расширение адреса entry.Path без учёта регистра.
// Используется до устранения повторов, в том числе для ссылок с иным именем.
func isJSON(entry Entry) bool {
	return strings.EqualFold(filepath.Ext(entry.Path), ".json")
}
