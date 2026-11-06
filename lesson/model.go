// Package lesson определяет один итоговый урок по фиксированной Lesson.schema 1.0.
// Проверяет схему, явные ограничения итогового контракта ТЗ и внутренние ID.
// Не преобразует parsed, не назначает ID и даты, не сравнивает версии,
// не читает файлы и не проверяет модуль, ресурсы на диске или синтаксис DSL.
// Модель сериализуется encoding/json без служебных полей.
// Перед записью вызывают Validate; перед декодированием внешнего JSON — ValidateJSON.
package lesson

import "encoding/json"

// FormatVersion — поддерживаемая версия контракта итогового урока.
const FormatVersion = "1.0"

// Lesson — корень итогового JSON; все поля обязательны.
type Lesson struct {
	// FormatVersion — ровно 1.0; это не версия контракта parsed/model.
	FormatVersion string `json:"formatVersion"`

	// Document — содержимое и метаданные одного урока.
	Document Document `json:"document"`
}

// Document — итоговый документ без метаданных сборщика и парсера.
type Document struct {
	// ID — стабильный ID с исходным регистром, проверяемый identity v2.
	ID string `json:"id"`

	// Version — положительное целое JSON-число без машинного предела.
	// Арифметика ревизий и выбор новой версии принадлежат revision.
	Version json.Number `json:"version"`

	// Title — обязательная строка названия; схема допускает пустую строку.
	Title string `json:"title"`

	// Subtitle — подзаголовок; nil записывается как null.
	// При чтении отсутствующее поле и null имеют одно значение nil.
	Subtitle *string `json:"subtitle"`

	// LineCount — неотрицательное целое JSON-число, перенесённое из входа.
	// Пакет не пересчитывает строки и не сверяет значение с исходным DSL.
	LineCount json.Number `json:"lineCount"`

	// CreatedAt — дата версии в точном UTC-формате YYYY-MM-DDTHH:MM:SSZ.
	CreatedAt string `json:"createdAt"`

	// Pages — страницы в порядке отображения; nil недопустим, пустой срез допустим.
	Pages []Page `json:"pages"`
}

// Page — страница; все поля обязательны, минимальное число нод не задано.
type Page struct {
	// ID — канонический ID страницы с префиксом содержащего урока.
	ID string `json:"id"`

	// Title — название либо null; nil не приводит к пропуску поля.
	Title *string `json:"title"`

	// Nodes — корневые ноды в порядке отображения; nil недопустим.
	Nodes []Node `json:"nodes"`
}
