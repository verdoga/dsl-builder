package resourceembed

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourcefind"
)

// readBytes читает полное содержимое выбранного обычного файла средствами
// стандартной библиотеки Go по полному file.Entry.Path, без новой базы пути.
// Доступность проверяется фактическим чтением, сведения Entry.Info не
// заменяют его. Удаление файла или отказ доступа после Find — ошибка чтения;
// поиск замены, пропуск файла и переход к другому адресу не выполняются.
// Любая ошибка возвращает nil и readError, частичные байты отбрасываются.
// Файловый дескриптор закрывается на всех путях после успешного открытия.
// Пустой файл допустим. Искусственных ограничений размера не вводится.
// Байты TXT и субтитров так же непрозрачны, как байты основного медиа:
// BOM, переводы строк, кодировка и любое содержимое сохраняются полностью.
// Не проверяет UTF-8, не разбирает контейнеры и не изменяет исходный файл.
func readBytes(file resourcefind.File, context diagnostic.Context) ([]byte, *diagnostic.Error) {
	bytes, err := os.ReadFile(file.Entry.Path)
	if err != nil {
		return nil, readError(file, context, err)
	}
	return bytes, nil
}

// embeddedFile формирует lesson.EmbeddedFile из полностью прочитанных bytes.
// FileName — последний компонент Entry.Path по правилам Windows filepath,
// с исходными регистром, пробелами и Unicode, без каталога; Info.Name и
// IdentityPath не подставляются, в том числе при обращении через ссылку.
// MIMEType точно копируется из File.MIMEType; таблица resourcefind не
// дублируется, содержимое файла и реестр Windows не используются для MIME.
// Content — base64.StdEncoding.EncodeToString(bytes): стандартный алфавит,
// padding, отсутствие переносов и префикса data URI; для пустых bytes — "".
// Декодирование Content возвращает все исходные байты без преобразования.
// Функция не изменяет bytes, не сохраняет их срез и не выполняет ввод-вывод.
func embeddedFile(file resourcefind.File, bytes []byte) lesson.EmbeddedFile {
	// Оба разделителя Windows учитываются и при проверках на другой ОС;
	// сам полный очищенный адрес выбранного файла не изменяется.
	name := filepath.Base(strings.ReplaceAll(file.Entry.Path, `\`, "/"))
	return lesson.EmbeddedFile{
		FileName: name,
		MIMEType: file.MIMEType,
		Content:  base64.StdEncoding.EncodeToString(bytes),
	}
}

// mediaData оформляет один элемент с Type=lesson.DataMedia, собственным
// ненулевым указателем Role на переданную роль, Value конкретного типа
// lesson.EmbeddedFile по значению, ненулевым пустым Answers и пустым Hint.
// Указатель на EmbeddedFile не используется: его запрещает контракт Data.
// Не добавляет пути, идентификаторы, флаги, счётчики и служебные поля.
func mediaData(value lesson.EmbeddedFile, role lesson.DataRole) lesson.Data {
	return lesson.Data{
		Type:    lesson.DataMedia,
		Role:    &role,
		Value:   value,
		Answers: []string{},
		Hint:    "",
	}
}
