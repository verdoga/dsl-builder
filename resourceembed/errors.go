package resourceembed

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/resourcefind"
)

// CodeRead — выбранный файл не удалось полностью прочитать для встраивания.
const CodeRead = "resourceembed.read"

// readError создаёт diagnostic.Error с CodeRead, русской причиной
// «Не удалось прочитать выбранный ресурс» и исходной ошибкой cause в Cause.
// Context.Path указывает на точный file.Entry.Path ошибочного ресурса;
// Related содержит исходный context с путём JSON и известными координатами,
// техническим именем, ID урока и страницей. Неизвестные сведения не выдумываются.
// В Details допустимы имя и MIME, но не исходные байты и не Base64.
// Ничего не печатает, не управляет меню и не отмечает ресурс использованным.
func readError(file resourcefind.File, context diagnostic.Context, cause error) *diagnostic.Error {
	return &diagnostic.Error{
		Code:    CodeRead,
		Reason:  "Не удалось прочитать выбранный ресурс",
		Context: diagnostic.Context{Path: file.Entry.Path},
		Related: []diagnostic.Context{context},
		Cause:   cause,
	}
}
