// Package convert координирует преобразование одного проверенного входа.
// Проект v1; ТЗ 1.0, разделы 5–10, 12–13, 15 и 19; Contract 1.0.
// Один и тот же Build применяется при создании, добавлении и обновлении.
// Входной DSL повторно не разбирается; предметные правила принадлежат
// fragments, structure, textdata, exercisedata, resourcefind, resourceembed,
// resourceusage, revision и lesson. Используется только стандартная библиотека
// и разрешённые зависимости с префиксом smoothlingua.
// Искусственных пределов глубины, числа страниц, нод и ресурсов нет.
// Пакет не загружает JSON, не выбирает режим, не сравнивает уроки, не ведёт
// диалог, не назначает технические имена и не формирует манифест или ZIP.
// Нет новых моделей строк, нод, метаданных, ресурса и результата-обёртки.
package convert

import (
	"time"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/filetree"
	"github.com/verdoga/dsl-builder/fragments"
	"github.com/verdoga/dsl-builder/lesson"
	"github.com/verdoga/dsl-builder/parsed"
	"github.com/verdoga/dsl-builder/resourceusage"
	"github.com/verdoga/dsl-builder/revision"
	"github.com/verdoga/dsl-builder/structure"
)

// Build принимает input из успешного parsed.Load, неизменённый после загрузки.
// tree — тот же ненулевой Resolver, который app использует для всей операции;
// новый экземпляр на урок не создаётся. Вызовы с общим tree последовательны.
// at задаёт время первичной версии; системные часы здесь не читаются.
// context передаёт известное техническое имя; Path и LessonID берутся из input
// через inputContext. Старые PageID и координаты не переносятся в новый вход.
// nil input/tree дают CodeInput до обращения к их полям; проверку parsed
// повторно не выполняет. Вход, его индекс и вложенные данные не изменяются.
//
// Координатор вызывает только небольшие шаги: newLesson, fragments.Expand,
// structure.Build, prepareResources, fillData и revision.Initial.
// Успешные pages из structure.Build передаются в Document.Pages без изменения
// структуры; bindings принадлежат именно этим pages. Каждый следующий этап
// запускается только после успеха предыдущего. Для учёта создаётся локальный
// resourceusage.Registry с готовым нулевым значением, без копий по значению.
// prepareResources обязателен даже при отсутствии медианод.
//
// После заполнения данных revision.Initial(candidate, at, context) назначает
// первичные ID, версию и дату и вызывает lesson.Validate, включая ValidateIDs.
// Повторная нумерация, собственная проверка схемы и ещё один Validate не нужны.
// Возвращается именно независимый проверенный результат Initial; bindings
// после этого не применяются. Даже кандидат обновления проходит этот путь.
// Сравнение с прежним уроком и revision.Apply выполняет app за пределами Build;
// первичные ID не обещают соответствия прежним нодам и страницам.
//
// Успех: ненулевые value/usage и nil ошибок. Любая ошибка, в том числе поздняя
// проверка урока, даёт nil, nil и непустые errors: частичный урок и его локальный
// реестр не публикуются. Состояние общего Resolver остаётся допустимым кешем
// идентичности, а не признаком успеха. После исправления файлов app начинает
// новый анализ с новым общим Resolver. Ошибка запрещает запись всей операции.
// app объединяет успешные usage через Registry.Merge; Snapshot строится после
// объединения, а не суммированием счётчиков. Невостребованные файлы допустимы.
// Метаданные для размещения уже доступны app как input.Result.Document.Metadata:
// Section и Order сохраняются с null, регистром и ведущими нулями; новый тип
// метаданных не требуется. Они не добавляются в lesson.Document и не изменяются.
func Build(input *parsed.Input, tree *filetree.Resolver, at time.Time, context diagnostic.Context) (value *lesson.Lesson, usage *resourceusage.Registry, errors []*diagnostic.Error) {
	if input == nil {
		return nil, nil, []*diagnostic.Error{{
			Code:    CodeInput,
			Reason:  "Не передан обязательный проверенный вход",
			Context: context,
		}}
	}
	if tree == nil {
		return nil, nil, []*diagnostic.Error{{
			Code:    CodeInput,
			Reason:  "Не передан общий Resolver операции",
			Context: context,
		}}
	}
	context = inputContext(input, context)
	candidate, err := newLesson(input.Result.Document, context)
	if err != nil {
		return nil, nil, []*diagnostic.Error{err}
	}
	entries, errors := fragments.Expand(input)
	if len(errors) != 0 {
		return nil, nil, errors
	}
	pages, bindings, errors := structure.Build(input, entries)
	if len(errors) != 0 {
		return nil, nil, errors
	}
	candidate.Document.Pages = pages
	var registry resourceusage.Registry
	finder, err := prepareResources(input, tree, &registry, context)
	if err != nil {
		return nil, nil, []*diagnostic.Error{err}
	}
	if errors := fillData(pages, bindings, finder, &registry, context); len(errors) != 0 {
		return nil, nil, errors
	}
	value, errors = revision.Initial(candidate, at, context)
	if len(errors) != 0 {
		return nil, nil, errors
	}
	return value, &registry, nil
}
