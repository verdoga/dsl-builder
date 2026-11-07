package resourcefind

import (
	"path/filepath"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
)

// companions перечисляет tree.ReadDir только каталога Body.Entry.Path.
// Поиск основан на адресе выбранного имени, а не на адресе цели ссылки.
// Отбирает отдельными companion расшифровку .txt и все форматы субтитров.
// Базовое имя — имя Body.Entry.Path без последнего расширения; совпадение
// полное и без регистра, не подстрока. Подкаталоги не обходятся.
// Необязательные отсутствующие файлы допустимы независимо друг от друга.
// Ошибка перечисления или доступа к найденному объекту блокирует результат;
// проверка чтения байтов остаётся resourceembed и также обязательна.
func (f *Finder) companions(body File, context diagnostic.Context) (script *File, subtitles []File, err *diagnostic.Error) {
	entries, err := f.tree.ReadDir(filepath.Dir(body.Entry.Path))
	if err != nil {
		return nil, nil, contextualize(err, context)
	}
	name := filepath.Base(body.Entry.Path)
	baseName := strings.TrimSuffix(name, filepath.Ext(name))
	script, err = companion(entries, baseName, ".txt", context)
	if err != nil {
		return nil, nil, err
	}
	for _, extension := range subtitleExtensions() {
		subtitle, err := companion(entries, baseName, extension, context)
		if err != nil {
			return nil, nil, err
		}
		if subtitle != nil {
			subtitles = append(subtitles, *subtitle)
		}
	}
	return script, subtitles, nil
}

// companion отбирает из entries обычные файлы с заданным полным базовым
// именем и extension без учёта регистра, затем применяет filetree.Unique.
// Отсутствие даёт nil, nil; один файл даёт File с MIME таблицы;
// несколько разных файлов, в том числе отличающихся только регистром,
// дают CodeAmbiguous со всеми кандидатами. Псевдонимы одного файла не спорят.
// Остальные имена и расширения не подключаются и вне resourceDirs не учитываются.
// Файловую систему не читает; entries получены одним ReadDir.
func companion(entries []filetree.Entry, baseName string, extension string, context diagnostic.Context) (*File, *diagnostic.Error) {
	var matches []filetree.Entry
	for _, entry := range entries {
		if entry.Info == nil || !entry.Info.Mode().IsRegular() {
			continue
		}
		name := filepath.Base(entry.Path)
		ext := filepath.Ext(name)
		if strings.EqualFold(ext, extension) && strings.EqualFold(strings.TrimSuffix(name, ext), baseName) {
			matches = append(matches, entry)
		}
	}
	matches = filetree.Unique(matches)
	if len(matches) == 0 {
		return nil, nil
	}
	kind, mimeType, _ := formatFor(extension)
	if len(matches) > 1 {
		paths := make([]string, len(matches))
		for i, entry := range matches {
			paths[i] = entry.Path
		}
		return nil, selectionError(CodeAmbiguous, "Найдено несколько сопутствующих файлов", context,
			kind, baseName+extension, []string{filepath.Dir(matches[0].Path)}, paths)
	}
	return &File{Entry: matches[0], MIMEType: mimeType}, nil
}

// subtitleExtensions возвращает независимый срез фиксированного порядка:
// .vtt, .srt, .ttml, .dfxp, .ass, .ssa. Каждый формат выбирается отдельно;
// найденный формат не вытесняет другие. Таблица не настраивается пользователем.
func subtitleExtensions() []string {
	return []string{".vtt", ".srt", ".ttml", ".dfxp", ".ass", ".ssa"}
}
