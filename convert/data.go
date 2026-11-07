package convert

import (
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/exercisedata"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/resourcefind"
	"github.com/verdoga/dsl-builder/resourceusage"
	"github.com/verdoga/dsl-builder/structure"
	"github.com/verdoga/dsl-builder/textdata"
)

// fillData обходит готовые bindings в порядке structure.Build и для каждой
// вызывает fillNode с bindingContext. Само дерево повторно не строит и не
// ищет исходники по физическому номеру: разные include имеют разные Binding.
// Количество, порядок и вложенность pages/Nodes остаются неизменными.
// На первой ошибке прекращает заполнение; локальная модель и реестр будут
// отброшены Build. Ошибки одного вызова сохраняются в порядке владельца.
func fillData(pages []lesson.Page, bindings []structure.Binding, finder *resourcefind.Finder, usage *resourceusage.Registry, context diagnostic.Context) []*diagnostic.Error {
	for _, binding := range bindings {
		if err := fillNode(pages, binding, finder, usage, bindingContext(context, binding)); err != nil {
			return []*diagnostic.Error{err}
		}
	}
	return nil
}

// fillNode получает адрес только через binding.Node(pages) и выбирает шаг
// по готовому Node.Type: NodeMedia — fillMedia; NodeQuestion, NodeSingleChoice,
// NodeMultipleChoice, NodeMatching, NodeOrdering — fillExercise; остальные
// поддерживаемые textdata типы — fillText. Не распознаёт исходные теги заново.
// Ошибка обогащается bindingError; нового типа или ноды взамен ошибочной нет.
// Вложенные ноды имеют собственные Binding и заполняются независимо.
func fillNode(pages []lesson.Page, binding structure.Binding, finder *resourcefind.Finder, usage *resourceusage.Registry, context diagnostic.Context) *diagnostic.Error {
	node := binding.Node(pages)
	var err *diagnostic.Error
	switch node.Type {
	case lesson.NodeMedia:
		err = fillMedia(node, binding, finder, usage, context)
	case lesson.NodeQuestion, lesson.NodeSingleChoice, lesson.NodeMultipleChoice, lesson.NodeMatching, lesson.NodeOrdering:
		err = fillExercise(node, binding, context)
	default:
		err = fillText(node, binding, context)
	}
	if err != nil {
		return bindingError(err, binding, context)
	}
	return nil
}

// fillText передаёт binding.TextSource() в textdata.Build(node.Type,
// declaration, lines, context) и только при успехе присваивает node.Data.
// Флаги не меняет: task сохраняет Speaking/Stepped от structure, multifill
// сохраняет пустые Flags с Checkable=nil. Внутренние ответы не извлекает.
// Отдельные instruction уже размещены structure и получают свои декларации;
// script обрабатывается как самостоятельная текстовая нода, отдельно от TXT.
// Правила текста, коллекций, переносов и подсказок остаются у textdata.
func fillText(node *lesson.Node, binding structure.Binding, context diagnostic.Context) *diagnostic.Error {
	declaration, lines := binding.TextSource()
	data, err := textdata.Build(node.Type, declaration, lines, context)
	if err != nil {
		return err
	}
	node.Data = data
	return nil
}

// fillExercise передаёт полную binding.Lines() в exercisedata.Build.
// Только при успехе устанавливает node.Data и отдельный указатель Checkable
// на возвращённый bool, включая false; Speaking/Stepped не добавляет.
// Не отбирает answer/hint повторно, не создаёт коллекции и не вычисляет
// проверяемость самостоятельно. NodeMultifill сюда никогда не передаётся.
func fillExercise(node *lesson.Node, binding structure.Binding, context diagnostic.Context) *diagnostic.Error {
	data, checkable, err := exercisedata.Build(node.Type, binding.Lines(), context)
	if err != nil {
		return err
	}
	node.Data = data
	node.Flags.Checkable = &checkable
	return nil
}
