package revision

import (
	"strconv"

	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/lesson"
)

// Decision — явное решение по одному compare.FieldChange; нулевое значение
// не означает отказ и позволяет обнаружить незавершённый сбор решений.
type Decision string

const (
	// Accept — применить предложенное After.
	Accept Decision = "accept"
	// Reject — оставить Before либо null при неизвестном прежнем page.title.
	Reject Decision = "reject"
)

// hasAcceptedChanges использует plan.HasContentChanges и решения Fields.
// Возвращает true для любого принятого поля или содержательной замены.
// У несопоставленного PageTitle даже After=nil не доказывает равенство
// прежнему полю; такая запись возможна только при ReplaceLesson=true.
// Версия, дата, LineCount и различия регистра ID не создают изменение.
func hasAcceptedChanges(plan *compare.Plan, decisions []Decision) bool {
	if plan.HasContentChanges() {
		return true
	}
	for index := range plan.Fields {
		if decisions[index] == Accept {
			return true
		}
	}
	return false
}

// applyFields вызывает applyField в порядке plan.Fields, не выбирая пары
// страниц заново. При полной замене документ исходно сохраняет старые
// Title/Subtitle, а страницы берутся из кандидата; у безопасной пары без
// записи FieldChange название кандидата уже равно старому по compare.
// При частичном обновлении исходные названия взяты из копии old.
func applyFields(result *lesson.Lesson, fields []compare.FieldChange, decisions []Decision) {
	for index, change := range fields {
		applyField(result, change, decisions[index])
	}
}

// applyField копирует выбранное значение в указанное поле результата.
// Accept выбирает After; Reject выбирает Before, включая известное null.
// Для PageTitle и OldPage=-1 Reject устанавливает title=null в NewPage:
// это отказ от нового названия, не восстановление неизвестного старого.
// Старое название по NewPage, сходству названий или первичному ID не ищется.
// OldPage служит только контракту поля; даже безопасная пара при полной
// замене не разрешает перенос ID. При DocumentTitle значение не nil.
func applyField(result *lesson.Lesson, change compare.FieldChange, decision Decision) {
	var selected *string
	switch decision {
	case Accept:
		selected = change.After
	case Reject:
		selected = change.Before
		if change.Field == compare.PageTitle && change.OldPage == -1 {
			selected = nil
		}
	default:
		return
	}
	if selected != nil {
		text := *selected
		selected = &text
	}
	switch change.Field {
	case compare.DocumentTitle:
		result.Document.Title = *selected
	case compare.DocumentSubtitle:
		result.Document.Subtitle = selected
	case compare.PageTitle:
		result.Document.Pages[change.NewPage].Title = selected
	}
}

// validateDecisions требует точную длину Fields и только Accept/Reject.
// Отсутствующее, пустое и неизвестное решение дают CodeDecision; решения
// по названиям не выводятся из согласия на замену урока или запись архива.
func validateDecisions(fields []compare.FieldChange, decisions []Decision, context diagnostic.Context) []*diagnostic.Error {
	var problems []*diagnostic.Error
	if len(fields) != len(decisions) {
		problems = append(problems, revisionError(CodeDecision,
			"Число решений должно совпадать с числом изменений полей", "/decisions", context))
	}
	for index, decision := range decisions {
		if decision != Accept && decision != Reject {
			problems = append(problems, revisionError(CodeDecision,
				"Для изменения поля требуется явное решение accept или reject", "/decisions/"+strconv.Itoa(index), context))
		}
	}
	return problems
}
