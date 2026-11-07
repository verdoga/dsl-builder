package resourcefind

import (
	"sort"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

// resolveDirectories разрешает каждое значение resourceDirs через
// filetree.Resolve от baseDir; абсолютные Windows-пути использует напрямую.
// Не снимает экранирование, не меняет пробелы и не ищет альтернативных баз.
// Возвращает все полные адреса в исходном порядке либо nil и ошибку.
func resolveDirectories(baseDir string, resourceDirs []string, context diagnostic.Context) ([]string, *diagnostic.Error) {
	directories := make([]string, 0, len(resourceDirs))
	for _, directory := range resourceDirs {
		resolved, err := filetree.Resolve(baseDir, directory)
		if err != nil {
			return nil, contextualize(err, context)
		}
		directories = append(directories, resolved)
	}
	return directories, nil
}

// inventory вызывает tree.Walk с nil-фильтром для каждого указанного каталога.
// Проверка существования, типа и доступности, рекурсия без лимита глубины,
// обход ссылок и исключение циклов принадлежат filetree.
// Все обычные файлы, включая неподдерживаемые расширения, входят в учёт.
// Объединяет результаты через accountedPaths; пустая область допустима.
// Не пропускает очередной Walk из-за совпадения или пересечения каталогов.
// Ошибка одного каталога запрещает частичный результат.
func (f *Finder) inventory(context diagnostic.Context) ([]string, *diagnostic.Error) {
	var entries []filetree.Entry
	for _, directory := range f.directories {
		files, err := f.tree.Walk(directory, nil)
		if err != nil {
			return nil, contextualize(err, context)
		}
		entries = append(entries, files...)
	}
	return accountedPaths(entries), nil
}

// candidates ищет только в directories через tree.Walk с фильтром matchesMedia.
// Отбор каждого адреса по имени и расширению предшествует filetree.Unique,
// чтобы неподходящее имя ссылки не скрыло подходящее имя того же файла.
// Объединяет кандидатов разных каталогов по IdentityPath; один физический
// файл не создаёт неоднозначности из-за пересекающихся областей или ссылок.
// Итог упорядочивает filetree.ComparePaths по Entry.Path; при ошибке — nil.
// При отсутствии directories возвращает пустой список без обхода соседей.
func (f *Finder) candidates(mediaType string, value string, context diagnostic.Context) ([]filetree.Entry, *diagnostic.Error) {
	var entries []filetree.Entry
	for _, directory := range f.directories {
		files, err := f.tree.Walk(directory, func(entry filetree.Entry) bool {
			return matchesMedia(entry, mediaType, value)
		})
		if err != nil {
			return nil, contextualize(err, context)
		}
		entries = append(entries, files...)
	}
	entries = filetree.Unique(entries)
	sort.Slice(entries, func(i, j int) bool {
		return filetree.ComparePaths(entries[i].Path, entries[j].Path) < 0
	})
	return entries, nil
}
