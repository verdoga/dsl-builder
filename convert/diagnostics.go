package convert

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/parsed/model"
	"github.com/verdoga/dsl-builder/structure"
)

const (
	// CodeInput — не передан обязательный проверенный вход либо общий Resolver.
	CodeInput = "convert.input"
	// CodeMetadata — обязательное выходное значение неизвестно во входе.
	CodeMetadata = "convert.metadata"
	// CodeMediaSource — объявление медиа не даёт однозначных параметров Find.
	CodeMediaSource = "convert.media_source"
)

// inputContext сохраняет FileName из base, устанавливает Path=input.Path
// и LessonID из проверенного DocumentID; PageID, Line и Column сбрасывает.
// Предусловие: input ненулевой и получен успешным parsed.Load.
// document.filePath никогда не используется как путь JSON или база ресурсов.
func inputContext(input *parsed.Input, base diagnostic.Context) diagnostic.Context {
	return diagnostic.Context{
		Path:     input.Path,
		FileName: base.FileName,
		LessonID: *input.Result.Document.Metadata.DocumentID,
	}
}

// bindingContext дополняет контекст исходным Entries[0].Source.Number,
// оставляя Column=0 до известного элемента. Предусловие — Binding от Build.
// PageID до revision.Initial неизвестен и остаётся пустым; индекс страницы
// или номер строки не выдаётся за processing ID. Физические номера повторных
// включений не перенумеровываются и не служат ключом идентичности ноды.
func bindingContext(base diagnostic.Context, binding structure.Binding) diagnostic.Context {
	base.PageID = ""
	base.Line = binding.Entries[0].Source.Number
	base.Column = 0
	return base
}

// bindingError возвращает независимую диагностику для ошибки шага данных.
// Сохраняет Code, Reason, Context, Cause, Related и Details владельца, включая
// ресурсный путь при ошибке чтения; не подменяет его путём технического JSON.
// Добавляет известный контекст привязки и исходные места include из цепочки
// Entries[0].Parent в Related, от внутренней вставки к внешней, без повтора
// уже имеющегося места. Вставка определяется ненулевым Entry.Included.
// В Details добавляет индекс страницы и NodePath как служебный адрес, не ID.
// Не изменяет исходную ошибку, не печатает сообщения и не переносит warning/
// recommendation парсера, полные тексты нод или Base64 в диагностику.
func bindingError(err *diagnostic.Error, binding structure.Binding, context diagnostic.Context) *diagnostic.Error {
	if err == nil {
		return nil
	}
	result := *err
	result.Related = slices.Clone(err.Related)
	result.Details = slices.Clone(err.Details)
	seen := make(map[diagnostic.Context]bool, len(result.Related)+1)
	seen[result.Context] = true
	for _, place := range result.Related {
		seen[place] = true
	}
	add := func(place diagnostic.Context) {
		if !seen[place] {
			result.Related = append(result.Related, place)
			seen[place] = true
		}
	}
	context = bindingContext(context, binding)
	add(context)
	for entry := binding.Entries[0].Parent; entry != nil; entry = entry.Parent {
		if entry.Included == nil {
			continue
		}
		place := context
		place.Line, place.Column = entry.Source.Number, 0
		for _, element := range entry.Source.Elements {
			if element.ElementType == model.ElementTypeIdentifier {
				place.Column = element.Start
				break
			}
		}
		add(place)
	}
	result.Details = append(result.Details,
		diagnostic.Detail{Label: "Индекс страницы", Value: strconv.Itoa(binding.Page)},
		diagnostic.Detail{Label: "Путь ноды", Value: fmt.Sprint(binding.NodePath)})
	return &result
}
