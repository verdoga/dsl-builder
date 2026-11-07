package resourcefind

import (
	"path/filepath"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

// explicitSource отличает ограничивающую пару двойных кавычек в source.raw.
// raw — уже распознанный элемент Contract 1.0, а не строка DSL для разбора.
// Не проверяет его грамматику заново и не получает значение пути из raw.
func explicitSource(raw string) bool {
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

// mainFile выбирает explicitFile при кавычках в raw, иначе searchFile.
// value передаётся без обрезания пробелов, преобразования в число,
// повторного снятия экранирования и добавления либо удаления кавычек.
func (f *Finder) mainFile(mediaType string, raw string, value string, context diagnostic.Context) (File, *diagnostic.Error) {
	if explicitSource(raw) {
		return f.explicitFile(mediaType, value, context)
	}
	return f.searchFile(mediaType, value, context)
}

// explicitFile разрешает value через filetree.Resolve от baseDir и Inspect.
// Принимает только обычный файл с расширением объявленного mediaType.
// Абсолютный путь допускается вне resourceDirs; относительный не подставляется
// внутрь этих каталогов. При ошибке перехода к поиску по подстроке нет.
// Ошибки пути, доступа, вида объекта и расширения блокируют выбор.
func (f *Finder) explicitFile(mediaType string, value string, context diagnostic.Context) (File, *diagnostic.Error) {
	path, err := filetree.Resolve(f.baseDir, value)
	if err != nil {
		return File{}, contextualize(err, context)
	}
	entry, err := f.tree.Inspect(path)
	if err != nil {
		return File{}, contextualize(err, context)
	}
	if !entry.Info.Mode().IsRegular() {
		return File{}, selectionError(CodeKind, "Ресурс не является обычным файлом", context, mediaType, value, f.directories, []string{entry.Path})
	}
	kind, mimeType, ok := formatFor(filepath.Ext(filepath.Base(entry.Path)))
	if !ok || kind != mediaType {
		return File{}, selectionError(CodeExtension, "Расширение ресурса не соответствует объявленному типу", context, mediaType, value, f.directories, []string{entry.Path})
	}
	return File{Entry: entry, MIMEType: mimeType}, nil
}

// searchFile получает candidates и требует ровно один различный файл.
// Ноль, включая отсутствие resourceDirs, — CodeNotFound с русской причиной
// «Ресурс не найден»; больше одного — CodeAmbiguous со всеми путями.
// Первый кандидат не выбирается из неоднозначного набора.
func (f *Finder) searchFile(mediaType string, value string, context diagnostic.Context) (File, *diagnostic.Error) {
	entries, err := f.candidates(mediaType, value, context)
	if err != nil {
		return File{}, err
	}
	if len(entries) == 0 {
		return File{}, selectionError(CodeNotFound, "Ресурс не найден", context, mediaType, value, f.directories, nil)
	}
	if len(entries) > 1 {
		paths := make([]string, len(entries))
		for i, entry := range entries {
			paths[i] = entry.Path
		}
		return File{}, selectionError(CodeAmbiguous, "Найдено несколько подходящих ресурсов", context, mediaType, value, f.directories, paths)
	}
	_, mimeType, _ := formatFor(filepath.Ext(filepath.Base(entries[0].Path)))
	return File{Entry: entries[0], MIMEType: mimeType}, nil
}

// matchesMedia проверяет разрешённое для mediaType расширение и вхождение
// всего value в любую часть имени с расширением, взятого из Entry.Path.
// Сопоставление без учёта регистра средствами стандартной библиотеки Go.
// Имена каталогов не участвуют; точки, пробелы и ведущие нули значимы.
// Числовых, длиновых и иных дополнительных ограничений DSL нет.
func matchesMedia(entry filetree.Entry, mediaType string, value string) bool {
	name := filepath.Base(entry.Path)
	kind, _, ok := formatFor(filepath.Ext(name))
	return ok && kind == mediaType && strings.Contains(strings.ToLower(name), strings.ToLower(value))
}
