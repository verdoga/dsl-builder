package manifest

import "encoding/json"

// Entry — запись одного урока; все восемь полей обязательны.
// Это собственный объект схемы манифеста, а не копия модели всего урока.
// json.Number сохраняет целые без ограничения машинным диапазоном и float64.
type Entry struct {
	// ID — стабильный ID урока в исходном регистре; синтаксис задаёт identity v2.
	ID string `json:"id"`

	// FileName — техническое имя урока в корне ZIP, проверяемое identity.
	FileName string `json:"fileName"`

	// Order — положительный вес строкой с исходными нулями; nil означает null.
	// Схема допускает nil только при единственной записи в содержащем lessons.
	Order *string `json:"order"`

	// Title — название урока с хотя бы одним непробельным символом.
	Title string `json:"title"`

	// Subtitle — любой подзаголовок, включая пустой; nil записывается как null.
	Subtitle *string `json:"subtitle"`

	// LineCount — целое неотрицательное число строк; пустой json.Number ошибочен.
	LineCount json.Number `json:"lineCount"`

	// CreatedAt — UTC-дата формирования версии урока, без изменения пакетом.
	CreatedAt string `json:"createdAt"`

	// Version — целая положительная ревизия; пустой json.Number ошибочен.
	Version json.Number `json:"version"`
}
