// Package revision назначает жизненный цикл одного урока по ТЗ 1.0.
// Версия проекта: 1. Зависимости: lesson, identity, compare и diagnostic.
// Пакет применяет готовый план compare v2 и переданные решения app;
// не обнаруживает отличия, не выбирает уровень замены, не ведёт диалог,
// не меняет манифест, не назначает технические имена и не пишет файлы.
// Результат нужен до записи для превью. Повторное применение использует
// прежний old, а не предыдущий результат: одна операция даёт один прирост.
// app передаёт итоговый Document в catalog.SyncLesson и сохраняет прежнее
// техническое имя через module. preview использует исходный compare.Plan
// и старый/итоговый урок: отдельного дублирующего плана ID здесь нет.
package revision

import (
	"time"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

// Initial завершает первичную модель из convert для создания, добавления
// либо кандидата обновления. ID урока и LineCount уже взяты из входа;
// названия, данные, флаги и структура уже сформированы. Только Version,
// CreatedAt, Page.ID и Node.ID могут быть незаполненными и заменяются.
// FormatVersion должен быть lesson.FormatVersion, он не исправляется.
// Координирует cloneLesson, stampInitial, assignInitialIDs и lesson.Validate.
// Назначает версию 1, дату at в UTC с точностью до секунды, страницы с 1,
// ноды с 1 сквозь все страницы в порядке родитель → дети, без суффикса.
// Сохраняет исходное написание Document.ID и точное значение LineCount.
// nil даёт CodeInput. При ошибке результат nil, исходник не изменяется.
// Результат не разделяет изменяемую память с исходником; массивы nil
// не исправляются на пустые и выявляются итоговой lesson.Validate.
func Initial(candidate *lesson.Lesson, at time.Time, context diagnostic.Context) (*lesson.Lesson, []*diagnostic.Error) {
	if candidate == nil {
		return nil, []*diagnostic.Error{revisionError(CodeInput, "Первичная модель урока не задана", "", context)}
	}
	result := cloneLesson(candidate)
	stampInitial(&result.Document, at)
	if problems := assignInitialIDs(result, context); len(problems) != 0 {
		return nil, problems
	}
	if problems := lesson.Validate(result, context); len(problems) != 0 {
		return nil, problems
	}
	return result, nil
}

// Apply применяет план, построенный compare.Build именно для old/candidate.
// app сохраняет оба снимка и plan неизменными с момента сравнения;
// изменение снимков требует нового Build и новых решений. План нельзя
// редактировать вручную, удалять изменения или подменять уровни замены.
// decisions содержит ровно одно явное решение для каждого plan.Fields,
// по тем же индексам, включая поля без установленного прежнего значения.
// Координирует validateUpdateInputs, validatePlan, hasAcceptedChanges,
// cloneLesson, nextRevision, applyContent, applyFields, stampUpdate,
// lesson.Validate и ValidateHistory; не пересчитывает отличия.
// При отсутствии принятых изменений возвращает независимую копию old,
// changed=false: сохраняются ID, версия, дата и прежний LineCount.
// При изменении возвращает changed=true и версию old.Version+1 ровно один
// раз, at как дату версии и candidate.LineCount без пересчёта. Написание
// Document.ID всегда старое; первичные ID/дата/версия кандидата не переносятся.
// Изменения только манифеста не входят в этот API. changed=false позволяет
// moduleio сохранить исходные байты урока, даже если весь модуль записывается.
// На любой ошибке возвращает nil, false и ошибки; частичного результата нет.
// Снимки, план, решения и переданное время не меняются. Новые данные имеют
// собственные срезы и указатели, включая вложенные коллекции Data.Value.
func Apply(old *lesson.Lesson, candidate *lesson.Lesson, plan *compare.Plan, decisions []Decision, at time.Time, oldContext diagnostic.Context, candidateContext diagnostic.Context) (result *lesson.Lesson, changed bool, errors []*diagnostic.Error) {
	if problems := validateUpdateInputs(old, candidate, oldContext, candidateContext); len(problems) != 0 {
		return nil, false, problems
	}
	oldContext.LessonID = old.Document.ID
	if problems := validatePlan(old, candidate, plan, decisions, oldContext); len(problems) != 0 {
		return nil, false, problems
	}
	result = cloneLesson(old)
	changed = hasAcceptedChanges(plan, decisions)
	if changed {
		revision := nextRevision(old.Document.Version)
		if problems := applyContent(result, old, candidate, plan, revision.String(), oldContext); len(problems) != 0 {
			return nil, false, problems
		}
		applyFields(result, plan.Fields, decisions)
		stampUpdate(&result.Document, revision, candidate.Document.LineCount, at)
	}
	if problems := lesson.Validate(result, oldContext); len(problems) != 0 {
		return nil, false, problems
	}
	if problems := ValidateHistory(result, oldContext); len(problems) != 0 {
		return nil, false, problems
	}
	return result, changed, nil
}
