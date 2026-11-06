// Package model хранит полную объектную модель JSON-результата парсинга DSL
// по Contract.jsonc версии 1.0.
//
// Пакет не выполняет разбор, проверку инвариантов, JSON-маршалинг или
// обработку файлов. Он предоставляет структуру данных, JSON-теги,
// перечисления.
package model

// FormatVersion — закрытое множество поддерживаемых версий JSON-контракта.
type FormatVersion string

const (
	// FormatVersion1 — версия JSON-контракта 1.0.
	FormatVersion1 FormatVersion = "1.0"
)

// LineType — закрытое множество типов физических строк DSL.
type LineType string

const (
	// LineTypeTag — однострочное объявление DSL-тега.
	LineTypeTag LineType = "tag"

	// LineTypeHeading — заголовок первого, второго или третьего уровня.
	LineTypeHeading LineType = "heading"

	// LineTypeContent — строка содержимого.
	LineTypeContent LineType = "content"

	// LineTypeBlockStart — объявление с открывающей границу блока.
	LineTypeBlockStart LineType = "block-start"

	// LineTypeBlockEnd — отдельная строка, закрывающая блок.
	LineTypeBlockEnd LineType = "block-end"

	// LineTypeSeparator — допустимая строка-разделитель ---.
	LineTypeSeparator LineType = "separator"

	// LineTypeBlank — строка, пустая после удаления краевых пробелов и табуляций.
	LineTypeBlank LineType = "blank"

	// LineTypeInvalid — строка, не классифицируемая как допустимая конструкция.
	LineTypeInvalid LineType = "invalid"
)

// LineEnding — закрытое множество допустимых окончаний физических строк.
type LineEnding string

const (
	// LineEndingLF — окончание строки LF.
	LineEndingLF LineEnding = "\n"

	// LineEndingCRLF — окончание строки CRLF.
	LineEndingCRLF LineEnding = "\r\n"

	// LineEndingNone — отсутствие окончания у последней строки файла.
	LineEndingNone LineEnding = ""
)

// ElementType — закрытое множество типов выделенных элементов строки.
type ElementType string

const (
	// ElementTypeTag — имя DSL-тега.
	ElementTypeTag ElementType = "tag"

	// ElementTypeHeadingLevel — синтаксический уровень заголовка.
	ElementTypeHeadingLevel ElementType = "heading-level"

	// ElementTypeTitle — свободный текст заголовка или названия.
	ElementTypeTitle ElementType = "title"

	// ElementTypeContent — текстовое содержимое или инструкция.
	ElementTypeContent ElementType = "content"

	// ElementTypeIdentifier — идентификатор документа, задачи, фрагмента или включения.
	ElementTypeIdentifier ElementType = "identifier"

	// ElementTypeName — свободное имя секции или варианта.
	ElementTypeName ElementType = "name"

	// ElementTypeVersion — значение версии DSL.
	ElementTypeVersion ElementType = "version"

	// ElementTypeNumber — исходная строковая запись номера.
	ElementTypeNumber ElementType = "number"

	// ElementTypeMediaType — тип медиаресурса.
	ElementTypeMediaType ElementType = "media-type"

	// ElementTypeSource — источник медиаресурса.
	ElementTypeSource ElementType = "source"

	// ElementTypeResourcePath — путь из объявления @resource-dir.
	ElementTypeResourcePath ElementType = "resource-path"

	// ElementTypePlaceholder — плейсхолдер контракта.
	ElementTypePlaceholder ElementType = "placeholder"

	// ElementTypeUnparsed — ошибочный неразобранный фрагмент.
	ElementTypeUnparsed ElementType = "unparsed"

	// ElementTypeBlockOpen — открывающая фигурная скобка блока.
	ElementTypeBlockOpen ElementType = "block-open"

	// ElementTypeBlockClose — закрывающая фигурная скобка блока.
	ElementTypeBlockClose ElementType = "block-close"
)
