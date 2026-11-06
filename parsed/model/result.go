package model

// Result — верхнеуровневая модель результата обработки одного DSL-документа.
type Result struct {
	// Format — версия JSON-контракта.
	Format FormatVersion `json:"formatVersion"`

	// Document — сведения об исходном DSL-документе.
	Document Document `json:"document"`

	// Processing — история фактически начатых запусков инструментов.
	Processing []Processing `json:"processing"`

	// Lines — физические строки входного файла в исходном порядке.
	Lines []Line `json:"lines"`

	// Diagnostics — все ошибки и проблемы этого документа.
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Document — сведения об исходном DSL-документе и извлечённых метаданных.
type Document struct {
	// DSLVersion — прочитанное значение версии DSL или nil, если оно неизвестно.
	DSLVersion *string `json:"dslVersion"`

	// FileName — имя входного файла или nil, если оно неизвестно.
	FileName *string `json:"fileName"`

	// FilePath — абсолютный очищенный путь входного файла или nil, если он неизвестен.
	FilePath *string `json:"filePath"`

	// Encoding — установленная кодировка или nil, если её нельзя определить.
	Encoding *string `json:"encoding"`

	// HasBOM — признак наличия UTF-8 BOM или nil, если байтовый вход недоступен.
	HasBOM *bool `json:"hasBom"`

	// LineCount — число физических строк или nil, если оно неизвестно.
	LineCount *int `json:"lineCount"`

	// ByteLength — размер точных исходных байтов или nil, если он неизвестен.
	ByteLength *int64 `json:"byteLength"`

	// SHA256 — SHA-256 точных исходных байтов или nil, если они недоступны.
	SHA256 *string `json:"sha256"`

	// Metadata — однозначно извлечённые метаданные документа.
	Metadata DocumentMetadata `json:"metadata"`

	// HasErrors — признак наличия хотя бы одной диагностики уровня error.
	HasErrors bool `json:"hasErrors"`
}

// DocumentMetadata — извлечённые метаданные DSL-документа.
type DocumentMetadata struct {
	// DocumentID — значение единственного однозначно извлечённого @document-id.
	DocumentID *string `json:"documentId"`

	// Title — текст единственного однозначно извлечённого заголовка первого уровня.
	Title *string `json:"title"`

	// Subtitle — текст единственного однозначно извлечённого заголовка второго уровня.
	Subtitle *string `json:"subtitle"`

	// Section — свободное имя единственного однозначно извлечённого @section.
	Section *string `json:"section"`

	// Order — исходная строковая запись значения @order.
	Order *string `json:"order"`

	// ResourceDirs — пути из @resource-dir в порядке объявлений.
	ResourceDirs []string `json:"resourceDirs"`
}

// Processing — сведения об одном фактически начатом запуске инструмента.
type Processing struct {
	// ID — уникальный в пределах Processing идентификатор запуска.
	ID string `json:"id"`

	// Tool — стабильное машинное имя инструмента.
	Tool string `json:"tool"`

	// Version — версия инструмента или nil, если она неизвестна.
	Version *string `json:"version"`

	// StartedAt — время начала в RFC 3339 с часовым поясом или nil, если оно неизвестно.
	StartedAt *string `json:"startedAt"`

	// DurationMs — длительность запуска в миллисекундах или nil, если она неизвестна.
	DurationMs *int `json:"durationMs"`
}

// Line — одна физическая строка исходного DSL-файла.
type Line struct {
	// Number — уникальный номер физической строки, начиная с 1.
	Number int `json:"line"`

	// LineType — классификация строки по контракту.
	LineType LineType `json:"type"`

	// NestingLevel — логическая глубина строки.
	NestingLevel int `json:"nestingLevel"`

	// ParentLine — номер логической родительской строки или nil для корневой строки.
	ParentLine *int `json:"parentLine"`

	// Raw — точная строка без BOM и перевода строки.
	Raw string `json:"raw"`

	// LineEnding — исходное окончание физической строки.
	LineEnding LineEnding `json:"eol"`

	// HasErrors — признак ошибки, относящейся к этой строке.
	HasErrors bool `json:"hasErrors"`

	// Elements — элементы строки, отсортированные по исходному положению.
	Elements []Element `json:"elements"`
}

// Element — выделенный фрагмент одной строки DSL.
type Element struct {
	// ElementType — классификация элемента по контракту.
	ElementType ElementType `json:"type"`

	// Raw — точный фрагмент исходной строки.
	Raw string `json:"raw"`

	// Value — семантическое значение элемента или nil, если его нет.
	Value *string `json:"value"`

	// Start — начальная колонка фрагмента в кодовых точках Unicode.
	Start int `json:"start"`

	// End — исключающая конечная колонка фрагмента в кодовых точках Unicode.
	End int `json:"end"`

	// ErrorIDs — идентификаторы диагностик области element без повторов.
	ErrorIDs []string `json:"errorIds"`
}

// Diagnostic — запись конкретной диагностики в результате обработки.
type Diagnostic struct {
	// ID — уникальный в пределах Diagnostics идентификатор диагностики.
	ID string `json:"id"`

	// Source — идентификатор записи Processing, создавшей диагностику.
	Source string `json:"source"`

	// DiagnosticCode — машинный код диагностики.
	DiagnosticCode Code `json:"code"`

	// SeverityLevel — уровень серьёзности диагностики.
	SeverityLevel Severity `json:"severity"`

	// Message — конкретное человекочитаемое описание проблемы.
	Message string `json:"message"`

	// DiagnosticScope — область диагностики по контракту.
	DiagnosticScope Scope `json:"scope"`

	// Fatal — признак невозможности штатно продолжить обработку.
	Fatal bool `json:"fatal"`

	// Location — основной диапазон исходника или nil при отсутствии привязки.
	Location *Location `json:"location"`

	// RelatedLocations — дополнительные связанные диапазоны без повторов.
	RelatedLocations []Location `json:"relatedLocations"`
}

// Location — непрерывный диапазон исходного текста.
type Location struct {
	// Start — включаемая начальная позиция диапазона.
	Start Position `json:"start"`

	// End — исключающая конечная позиция диапазона.
	End Position `json:"end"`
}

// Position — позиция в исходном тексте по физической строке и Unicode-колонке.
type Position struct {
	// Line — номер физической строки, начиная с 1.
	Line int `json:"line"`

	// Column — номер колонки в кодовых точках Unicode, начиная с 1.
	Column int `json:"column"`
}
