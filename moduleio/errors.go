package moduleio

const (
	// CodeOpen — не удалось открыть файл архива для чтения.
	CodeOpen = "moduleio.open"
	// CodeArchive — повреждена структура ZIP или архив не поддерживается.
	CodeArchive = "moduleio.archive"
	// CodeEntryType — запись ZIP является каталогом или не обычным файлом.
	CodeEntryType = "moduleio.entry_type"
	// CodeEntryName — имя ZIP-записи не входит в плоский формат модуля.
	CodeEntryName = "moduleio.entry_name"
	// CodeDuplicateEntry — физическое имя ZIP-записи встречается повторно.
	CodeDuplicateEntry = "moduleio.duplicate_entry"
	// CodeManifest — отсутствует обязательная запись manifest.json.
	CodeManifest = "moduleio.manifest"
	// CodeRead — сбой чтения, распаковки, размера или CRC записи.
	CodeRead = "moduleio.read"
	// CodeClose — ошибка закрытия записи, архива или ZIP writer.
	CodeClose = "moduleio.close"
	// CodeEncoding — JSON не является UTF-8 без BOM.
	CodeEncoding = "moduleio.encoding"
	// CodeJSON — нарушен синтаксис или полное декодирование JSON.
	CodeJSON = "moduleio.json"
	// CodeWriter — отсутствует поток назначения.
	CodeWriter = "moduleio.writer"
	// CodeSerialize — модель не удалось сериализовать стандартным JSON.
	CodeSerialize = "moduleio.serialize"
	// CodeWrite — запись или завершение очередной ZIP-записи не удалось.
	CodeWrite = "moduleio.write"
)

// Ошибки представлены существующим diagnostic.Error, отдельного типа нет.
// Reason — понятная русская причина, Context — известный путь и имя записи,
// Cause — исходная техническая ошибка. Ошибки зависимостей сохраняют коды,
// поля и причины; дополнительный контекст не уничтожает известный источник.
// Ошибки закрытия идут после основных; пустой список означает полный успех.
// Полные JSON, содержимое уроков и Base64 в диагностику не копируются.
