package fragments

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
)

const (
	// CodeShape — форма определения или ссылки не позволяет развёртывание.
	CodeShape = "fragments.shape"

	// CodeMissing — определение запрошенного фрагмента отсутствует.
	CodeMissing = "fragments.missing"

	// CodeAmbiguous — имя соответствует нескольким определениям.
	CodeAmbiguous = "fragments.ambiguous"

	// CodeCycle — обнаружен цикл зависимостей включаемых фрагментов.
	CodeCycle = "fragments.cycle"

	// CodeParent — исходная связь не разрешается в конкретном экземпляре.
	CodeParent = "fragments.parent"
)

// sourceContext заполняет существующий diagnostic.Context: Path=input.Path,
// LessonID из проверенных метаданных, Line/Column из исходных координат.
// Column=0 означает отсутствие известной колонки. PageID не выдумывается;
// document.filePath не используется вместо пути технического JSON.
func sourceContext(input *parsed.Input, line int, column int) diagnostic.Context {
	context := diagnostic.Context{Path: input.Path, Line: line, Column: column}
	if input.Result.Document.Metadata.DocumentID != nil {
		context.LessonID = *input.Result.Document.Metadata.DocumentID
	}
	return context
}

// fragmentError создаёт diagnostic.Error с кодом пакета, русской причиной,
// исходным именем фрагмента в Details и известными связанными местами.
// Координаты остаются физическими даже при повторном включении. Ошибки
// не печатаются, предупреждения и рекомендации парсера не копируются.
func fragmentError(code string, reason string, identifier string, context diagnostic.Context, related []diagnostic.Context) *diagnostic.Error {
	return &diagnostic.Error{
		Code:    code,
		Reason:  reason,
		Context: context,
		Related: related,
		Details: []diagnostic.Detail{{Label: "Имя фрагмента", Value: identifier}},
	}
}
