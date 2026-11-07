package compare

import "github.com/verdoga/dsl-builder/lesson"

// Field — поле вне нод, требующее самостоятельного подтверждения.
type Field string

const (
	// DocumentTitle — название урока.
	DocumentTitle Field = "document.title"
	// DocumentSubtitle — подзаголовок урока.
	DocumentSubtitle Field = "document.subtitle"
	// PageTitle — название конкретной страницы.
	PageTitle Field = "page.title"
)

// FieldChange — предложение поля для отдельного подтверждения. При известном
// прежнем поле содержит точные значения и создаётся только при их различии.
// Для PageTitle без безопасной пары запись явно сообщает отсутствие прежнего
// поля, даже если After=nil. Подтверждение не заменяется полной заменой урока.
type FieldChange struct {
	// Field — название изменяемого поля.
	Field Field

	// OldPage — индекс безопасно установленной прежней страницы для PageTitle.
	// -1 при PageTitle означает, что прежнее поле не установлено; при полях
	// документа -1 означает неприменимость индекса, прежнее поле известно.
	// Правило выбора пары описано в comparePageTitles; ID отсюда не переносятся.
	OldPage int

	// NewPage — индекс страницы кандидата; -1 для полей документа.
	NewPage int

	// Before — прежняя строка или null при установленном прежнем поле;
	// для DocumentTitle всегда не nil. При PageTitle и OldPage=-1 всегда nil,
	// но означает отсутствие установленного поля, а не прежнее title=null.
	// Показ отличает этот случай от OldPage>=0 с прежним title=null.
	Before *string

	// After — предлагаемая строка или null; для DocumentTitle всегда не nil.
	// Согласие выбирает After. Отказ при установленном прежнем поле сохраняет
	// Before; для PageTitle результат относится к странице NewPage.
	// При PageTitle и OldPage=-1 отказ оставляет NewPage без названия, title=null:
	// это отказ от нового названия, а не восстановление неизвестного старого.
	// Этот результат показывается до решения; применение выполняет revision.
	After *string
}

// compareDocumentFields возвращает только различия Title и Subtitle.
// Пустая строка отличается от nil, регистр и пробелы значимы.
// Значения копируются для независимых подтверждений, снимки не меняются.
func compareDocumentFields(old lesson.Document, candidate lesson.Document) []FieldChange {
	var changes []FieldChange
	if old.Title != candidate.Title {
		before, after := old.Title, candidate.Title
		changes = append(changes, FieldChange{
			Field: DocumentTitle, OldPage: -1, NewPage: -1,
			Before: &before, After: &after,
		})
	}
	if !equalOptionalText(old.Subtitle, candidate.Subtitle) {
		change := FieldChange{Field: DocumentSubtitle, OldPage: -1, NewPage: -1}
		if old.Subtitle != nil {
			before := *old.Subtitle
			change.Before = &before
		}
		if candidate.Subtitle != nil {
			after := *candidate.Subtitle
			change.After = &after
		}
		changes = append(changes, change)
	}
	return changes
}

// comparePageTitles возвращает изменения названий в порядке новых страниц.
// При replaceLesson=false последовательность сохранена: OldPage=NewPage,
// Before берётся из этой старой страницы; равенство названий не даёт записи.
// При replaceLesson=true прежнее поле доступно только для взаимно однозначной
// точной пары: OldForNew содержит один старый индекс, а его NewForOld —
// только этот новый индекс. Before берётся только из установленной пары;
// равные названия такой пары не создают записи, при отказе сохраняется Before.
// Совпадение названий, первичных ID или индексов не заменяет безопасную пару.
// Новая либо несопоставленная страница, включая неоднозначные дубликаты,
// всегда даёт запись OldPage=-1, Before=nil, After=название кандидата.
// Позиционного запасного источника нет; равенство с чужим названием или
// After=nil не скрывает отсутствие установленного прежнего поля.
// Для такой записи показ сообщает «прежнее поле не установлено», новое
// значение и результат отказа title=null; app/console получает отдельное
// решение. Согласие на полную замену не подтверждает никакое название.
// При отказе чужое название не переносится и новая пара не подбирается.
// Удалённая страница не образует изменения поля: удаление входит в полную
// замену. Значения копируются; снимки и ID не изменяются. Даже безопасные
// пары при полной замене разрешают сохранить название, но не старые ID.
func comparePageTitles(old []lesson.Page, candidate []lesson.Page, matches pageMatches, replaceLesson bool) []FieldChange {
	var changes []FieldChange
	for newIndex, page := range candidate {
		oldIndex := newIndex
		if replaceLesson {
			oldIndex = -1
			if newIndex < len(matches.OldForNew) && len(matches.OldForNew[newIndex]) == 1 {
				matched := matches.OldForNew[newIndex][0]
				if matched >= 0 && matched < len(old) && matched < len(matches.NewForOld) &&
					len(matches.NewForOld[matched]) == 1 && matches.NewForOld[matched][0] == newIndex {
					oldIndex = matched
				}
			}
		}
		if oldIndex >= 0 && equalOptionalText(old[oldIndex].Title, page.Title) {
			continue
		}
		change := FieldChange{Field: PageTitle, OldPage: oldIndex, NewPage: newIndex}
		if oldIndex >= 0 && old[oldIndex].Title != nil {
			before := *old[oldIndex].Title
			change.Before = &before
		}
		if page.Title != nil {
			after := *page.Title
			change.After = &after
		}
		changes = append(changes, change)
	}
	return changes
}

// equalOptionalText сравнивает наличие и точные значения строк;
// адреса указателей не имеют значения, nil и указатель на пустую строку различны.
func equalOptionalText(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
