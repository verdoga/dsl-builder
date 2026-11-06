package lesson

// Node — нода любой вложенности; все пять полей обязательны.
type Node struct {
	// ID — полный канонический ID ноды, включая необязательный суффикс.
	ID string `json:"id"`

	// Type — один из типов NodeType фиксированной схемы.
	Type NodeType `json:"type"`

	// Flags — обязательный объект; указатели отличают отсутствие флага от false.
	Flags NodeFlags `json:"flags"`

	// Data — собственные данные; nil недопустим, отсутствие данных означает пустой срез.
	Data []Data `json:"data"`

	// Nodes — дочерние ноды; nil недопустим, лист содержит пустой срез.
	Nodes []Node `json:"nodes"`
}

// NodeFlags — единственные допустимые флаги; применимость определяется типом ноды.
type NodeFlags struct {
	// Speaking — необязательный признак устного задания, только task.
	Speaking *bool `json:"speaking,omitempty"`

	// Stepped — необязательный признак этапов задания, только task.
	Stepped *bool `json:"stepped,omitempty"`

	// Checkable — необязательный признак проверки для шести интерактивных типов.
	Checkable *bool `json:"checkable,omitempty"`
}

// NodeType — тип итоговой ноды; не дублирует тип входной строки или элемента.
type NodeType string

const (
	// NodeTask — задание.
	NodeTask NodeType = "task"
	// NodeStep — этап задания.
	NodeStep NodeType = "step"
	// NodeVariants — набор ветвей.
	NodeVariants NodeType = "variants"
	// NodeVariant — отдельная ветвь.
	NodeVariant NodeType = "variant"
	// NodeHeading — заголовок внутри страницы.
	NodeHeading NodeType = "heading"
	// NodeText — самостоятельный текст.
	NodeText NodeType = "text"
	// NodeInstruction — самостоятельная инструкция.
	NodeInstruction NodeType = "instruction"
	// NodeNote — примечание.
	NodeNote NodeType = "note"
	// NodeAlt — альтернативный текст.
	NodeAlt NodeType = "alt"
	// NodeScript — расшифровка из исходного урока.
	NodeScript NodeType = "script"
	// NodeHint — самостоятельная подсказка.
	NodeHint NodeType = "hint"
	// NodeAnswerKey — ключ ответов.
	NodeAnswerKey NodeType = "answerKey"
	// NodeExample — пример.
	NodeExample NodeType = "example"
	// NodeWordList — список слов.
	NodeWordList NodeType = "wordList"
	// NodeTable — таблица в текстовой форме.
	NodeTable NodeType = "table"
	// NodeMedia — медианода.
	NodeMedia NodeType = "media"
	// NodeQuestion — вопрос.
	NodeQuestion NodeType = "question"
	// NodeMultifill — непрозрачный для сборщика текст с пропусками.
	NodeMultifill NodeType = "multifill"
	// NodeSingleChoice — одиночный выбор.
	NodeSingleChoice NodeType = "singleChoice"
	// NodeMultipleChoice — множественный выбор.
	NodeMultipleChoice NodeType = "multipleChoice"
	// NodeMatching — сопоставление.
	NodeMatching NodeType = "matching"
	// NodeOrdering — упорядочивание или распределение по позициям.
	NodeOrdering NodeType = "ordering"
)
