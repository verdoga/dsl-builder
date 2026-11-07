package fragments

import "github.com/verdoga/dsl-builder/parsed/model"

// Entry — пассивная модель без конструктора и алгоритмов. Преобразование
// фиксирует точный контракт полей без reflection и повторения присваиваний.
// Независимость Source и связывание графа проверяются у copy и Expand.
var _ = struct {
	Source   model.Line
	Parent   *Entry
	Included []*Entry
}(Entry{})
