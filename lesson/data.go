package lesson

import (
	"encoding/json"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// Data — рекурсивный элемент данных фиксированной схемы.
type Data struct {
	// Type — дискриминатор конкретного Go-типа Value.
	Type DataType `json:"type"`

	// Role — роль; nil допустим только у элементов обычных коллекций.
	// Пустая строка не заменяет отсутствие и является неизвестной ролью.
	Role *DataRole `json:"role,omitempty"`

	// Value — ровно string при DataText, EmbeddedFile при DataMedia,
	// []Data при DataCollection; указатели, nil, map и прочие типы недопустимы.
	// Ограниченный union нужен для трёх разных JSON-форм одного поля.
	Value any `json:"value"`

	// Answers — обязательный массив строк; nil недопустим, отсутствие ответов — пустой срез.
	Answers []string `json:"answers"`

	// Hint — обязательная строка подсказки, в том числе пустая.
	Hint string `json:"hint"`
}

// EmbeddedFile — встроенный файл без служебных путей и состояния использования.
type EmbeddedFile struct {
	// FileName — имя исходного файла без пути.
	FileName string `json:"fileName"`

	// MIMEType — строка MIME; таблица выбора MIME принадлежит resourcefind.
	MIMEType string `json:"mimeType"`

	// Content — стандартный Base64 с padding и без переводов строк по ТЗ 6.2.
	Content string `json:"content"`
}

// DataType — тип значения данных, ограниченный тремя вариантами схемы.
type DataType string

const (
	// DataText — строковое значение.
	DataText DataType = "text"
	// DataMedia — объект встроенного файла.
	DataMedia DataType = "media"
	// DataCollection — массив элементов Data.
	DataCollection DataType = "collection"
)

// DataRole — роль элемента; допустимость зависит от его расположения.
type DataRole string

const (
	// RoleTitle — название.
	RoleTitle DataRole = "title"
	// RoleBody — основное содержимое.
	RoleBody DataRole = "body"
	// RoleInstruction — локальная инструкция трёх типов упражнений.
	RoleInstruction DataRole = "instruction"
	// RoleBefore — часть примера до разделителя.
	RoleBefore DataRole = "before"
	// RoleAfter — часть примера после разделителя.
	RoleAfter DataRole = "after"
	// RoleItems — коллекция элементов сопоставления или порядка.
	RoleItems DataRole = "items"
	// RoleItem — непосредственный текстовый элемент items.
	RoleItem DataRole = "item"
	// RoleOptions — коллекция вариантов.
	RoleOptions DataRole = "options"
	// RoleOption — непосредственный текстовый вариант options.
	RoleOption DataRole = "option"
	// RolePositions — коллекция явно заданных позиций.
	RolePositions DataRole = "positions"
	// RolePosition — непосредственная текстовая позиция positions.
	RolePosition DataRole = "position"
	// RoleHint — данные подсказки.
	RoleHint DataRole = "hint"
	// RoleScript — данные расшифровки.
	RoleScript DataRole = "script"
	// RoleSubtitles — данные субтитров.
	RoleSubtitles DataRole = "subtitles"
)

// UnmarshalJSON восстанавливает конкретный тип Value через decodeDataValue.
// Проверяет локальную форму Data через checkDataJSON; null и пропуски
// обязательных полей не превращаются в нулевые значения. Не проверяет
// расположение роли, зависящее от контейнера. Получатель меняется только
// при успехе; ошибка — *diagnostic.Error через стандартный интерфейс error.
// Восстанавливает только известные свойства с точным регистром имён;
// дополнительные свойства, включая TYPE и VALUE, не затеняют type и value.
// Файлового ввода нет; полную проверку урока выполняет ValidateJSON.
func (d *Data) UnmarshalJSON(raw []byte) error {
	context := diagnostic.Context{}
	if problems := checkDataJSON(raw, "", context); len(problems) != 0 {
		return problems[0]
	}
	if d == nil {
		return contractError(CodeJSON, "Не задан получатель данных", "", context, nil)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return contractError(CodeJSON, "Не удалось прочитать объект данных", "", context, err)
	}
	var decoded Data
	if err := json.Unmarshal(fields["type"], &decoded.Type); err != nil {
		return contractError(CodeJSON, "Не удалось прочитать тип данных", "/type", context, err)
	}
	if role, exists := fields["role"]; exists {
		if err := json.Unmarshal(role, &decoded.Role); err != nil {
			return contractError(CodeJSON, "Не удалось прочитать роль данных", "/role", context, err)
		}
	}
	value, problem := decodeDataValue(decoded.Type, fields["value"])
	if problem != nil {
		pointer := "/value"
		if decoded.Type != DataText && decoded.Type != DataMedia && decoded.Type != DataCollection {
			pointer = "/type"
		}
		prefixed := *problem
		prefixed.Details = append([]diagnostic.Detail(nil), problem.Details...)
		hasPointer := false
		for i := range prefixed.Details {
			if prefixed.Details[i].Label == "Поле JSON" {
				prefixed.Details[i].Value = pointer + prefixed.Details[i].Value
				hasPointer = true
			}
		}
		if !hasPointer {
			prefixed.Details = append(prefixed.Details, diagnostic.Detail{
				Label: "Поле JSON",
				Value: pointer,
			})
		}
		return &prefixed
	}
	decoded.Value = value
	if err := json.Unmarshal(fields["answers"], &decoded.Answers); err != nil {
		return contractError(CodeJSON, "Не удалось прочитать ответы", "/answers", context, err)
	}
	if err := json.Unmarshal(fields["hint"], &decoded.Hint); err != nil {
		return contractError(CodeJSON, "Не удалось прочитать подсказку", "/hint", context, err)
	}
	*d = decoded
	return nil
}
