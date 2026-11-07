// Package resourceembed встраивает выбранные файлы в данные одной медианоды.
// Проект версии 1; правила ТЗ 6.2, 9.4–9.6, 10.6 и 17.1 имеют приоритет
// над старыми DSL и golden. Вход — результат resourcefind, а не текст DSL.
// Пакет не ищет файлы, не определяет MIME, не преобразует медиаформаты,
// не строит ноды и не меняет урок, реестр ресурсов, исходники или архив.
package resourceembed

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourcefind"
)

// Embed формирует собственные данные одной медианоды из успешного Find.
// selection содержит проверенный Body, необязательный Script и Subtitles
// в порядке resourcefind; нулевой или самостоятельно собранный выбор
// не является входом этого API. context сохраняет известные путь JSON,
// техническое имя, ID урока, страницу, исходную строку и колонку.
// Координатор вызывает embedFile для Body с lesson.RoleBody, Script при
// наличии с lesson.RoleScript, затем каждого Subtitles с lesson.RoleSubtitles.
// Порядок vtt, srt, ttml, dfxp, ass, ssa сохраняется без повторной сортировки.
// Отсутствующие сопутствующие файлы не создают элементов; у изображения
// выбор содержит только Body. Все выбранные файлы обязательны к чтению.
// Каждый файл даёт отдельный элемент; одинаковые байты или IdentityPath
// не служат основанием удалять элементы либо объединять их роли.
// Исходная текстовая нода script не передаётся сюда и не сравнивается с TXT.
// Успех возвращает независимый непустой срез и nil; любая ошибка чтения,
// включая найденный сопутствующий файл, возвращает nil и диагностику.
// Частично сформированные данные не публикуются; selection не изменяется.
// Вызывающий convert включает данные в медианоду и только после успешного
// включения в урок передаёт точный Selection.AccountedPaths в MarkUsed.
// Сам выбор, чтение или получение результата без включения в урок не дают
// отметки использования. Register области учёта остаётся у вызывающего кода.
func Embed(selection resourcefind.Selection, context diagnostic.Context) ([]lesson.Data, *diagnostic.Error) {
	body, err := embedFile(selection.Body, lesson.RoleBody, context)
	if err != nil {
		return nil, err
	}
	result := make([]lesson.Data, 0, len(selection.Subtitles)+2)
	result = append(result, body)
	if selection.Script != nil {
		script, err := embedFile(*selection.Script, lesson.RoleScript, context)
		if err != nil {
			return nil, err
		}
		result = append(result, script)
	}
	for _, file := range selection.Subtitles {
		subtitle, err := embedFile(file, lesson.RoleSubtitles, context)
		if err != nil {
			return nil, err
		}
		result = append(result, subtitle)
	}
	return result, nil
}

// embedFile координирует чтение одного файла через readBytes, формирование
// EmbeddedFile через embeddedFile и элемента данных через mediaData.
// role задаётся Embed только как body, script или subtitles; расширение
// и имя не используются для повторного определения роли или MIME.
// При ошибке возвращает нулевой Data, который вызывающий код не использует.
func embedFile(file resourcefind.File, role lesson.DataRole, context diagnostic.Context) (lesson.Data, *diagnostic.Error) {
	bytes, err := readBytes(file, context)
	if err != nil {
		return lesson.Data{}, err
	}
	return mediaData(embeddedFile(file, bytes), role), nil
}
