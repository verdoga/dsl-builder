package module

const (
	// CodeModule — отсутствует модель модуля.
	CodeModule = "module.model"
	// CodeFile — отсутствует файл или его внутренний снимок урока.
	CodeFile = "module.file"
	// CodeEmpty — отсутствуют уроки или запрошен неположительный размер набора.
	CodeEmpty = "module.empty"
	// CodeOriginalBytes — отсутствуют обязательные байты загруженного урока.
	CodeOriginalBytes = "module.original_bytes"
	// CodeDuplicateFile — несколько файлов имеют одно техническое имя.
	CodeDuplicateFile = "module.duplicate_file"
	// CodeDuplicateID — повтор ключа ID во входах, файлах или записях манифеста.
	CodeDuplicateID = "module.duplicate_id"
	// CodeExistingID — добавляемый ID уже существует в архиве.
	CodeExistingID = "module.existing_id"
	// CodeMissingLesson — обновляемый ID не найден в составе файлов.
	CodeMissingLesson = "module.missing_lesson"
	// CodeAmbiguousLesson — обновляемый ID выбирает несколько файлов.
	CodeAmbiguousLesson = "module.ambiguous_lesson"
	// CodeIdentityChange — замена меняет стабильный ID либо его написание.
	CodeIdentityChange = "module.identity_change"
	// CodeDuplicateReference — несколько записей ссылаются на одно имя файла.
	CodeDuplicateReference = "module.duplicate_reference"
	// CodeMissingFile — запись манифеста ссылается на отсутствующий файл.
	CodeMissingFile = "module.missing_file"
	// CodeUnlistedFile — файл урока не имеет записи манифеста.
	CodeUnlistedFile = "module.unlisted_file"
	// CodeMetadata — поле записи манифеста не совпадает с метаданными урока.
	CodeMetadata = "module.metadata"
)

// Ошибки создаются существующим diagnostic.Error: русская Reason, известный
// Context, Related со всеми участниками и Details с полем и значениями.
// Код/причина ошибок зависимостей сохраняются. Порядок диагностики устойчив;
// содержимое нод и Base64 в сообщения не копируются. Пустой срез означает успех.
