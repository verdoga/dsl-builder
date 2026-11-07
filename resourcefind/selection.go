package resourcefind

import (
	"sort"

	"github.com/verdoga/dsl-builder/filetree"
)

// File связывает выбранный объект файловой системы с фиксированным MIME.
// Не повторяет поля Entry и не представляет встроенные данные урока.
type File struct {
	// Entry — исходный адрес, сведения и стабильный IdentityPath из filetree.
	// Имя для встраивания берётся из последнего компонента Entry.Path,
	// а не Info.Name или IdentityPath; расширение проверено по этому же имени.
	Entry filetree.Entry

	// MIMEType — значение таблицы ТЗ 9.3 без обращения к реестру Windows.
	MIMEType string
}

// Selection — полный выбор для одной медианоды; содержимого файлов здесь нет.
// Порядок передачи resourceembed: Body, Script при наличии, затем Subtitles.
// Роли body, script и subtitles назначает resourceembed по соответствующим
// полям; исходный script урока не участвует в поиске файлов.
type Selection struct {
	// Body — единственный основной файл объявленного типа.
	Body File

	// Script — необязательная одноимённая расшифровка; nil при отсутствии.
	Script *File

	// Subtitles — все найденные форматы в порядке vtt, srt, ttml, dfxp, ass, ssa.
	// Для изображений Script равен nil, Subtitles пуст.
	Subtitles []File

	// AccountedPaths — уникальные IdentityPath основного и сопутствующих файлов.
	// Дополняют область Prepare, в том числе явными файлами вне resourceDirs.
	// Не содержат остальных соседей внешнего файла и не означают использование.
	AccountedPaths []string
}

// selection собирает выбранные файлы и их пути учёта без файлового ввода-вывода.
// Устраняет повторы только в AccountedPaths; поля выбора сохраняют свои роли.
func selection(body File, script *File, subtitles []File) Selection {
	result := Selection{Body: body}
	entries := make([]filetree.Entry, 0, len(subtitles)+2)
	entries = append(entries, body.Entry)
	if script != nil {
		scriptCopy := *script
		result.Script = &scriptCopy
		entries = append(entries, script.Entry)
	}
	if subtitles != nil {
		result.Subtitles = make([]File, len(subtitles))
		copy(result.Subtitles, subtitles)
	}
	for _, subtitle := range subtitles {
		entries = append(entries, subtitle.Entry)
	}
	result.AccountedPaths = accountedPaths(entries)
	return result
}

// accountedPaths возвращает независимый срез точных Entry.IdentityPath после
// filetree.Unique, упорядоченный filetree.ComparePaths по полным путям.
// Включает файлы любого расширения. Строки непустые, полные, не меняют регистр
// и подходят непосредственно Registry.Register и Registry.MarkUsed владельца
// учёта; этот пакет их не вызывает. MarkUsed допустим лишь после встраивания.
func accountedPaths(entries []filetree.Entry) []string {
	unique := filetree.Unique(entries)
	paths := make([]string, len(unique))
	for i, entry := range unique {
		paths[i] = entry.IdentityPath
	}
	sort.Slice(paths, func(i, j int) bool {
		return filetree.ComparePaths(paths[i], paths[j]) < 0
	})
	return paths
}
