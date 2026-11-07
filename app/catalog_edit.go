package app

import (
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
)

// EditPreview содержит кандидат одной отдельной правки для показа и решения.
// Даже ненулевые Blockers не запрещают принять редактируемый catalog:
// они запрещают запись, а последующие правки могут устранить конфликт.
type EditPreview struct {
	// Token — закрытый идентификатор этого предложения, не токен записи.
	Token ReviewToken
	// Catalog — независимая копия всего предлагаемого размещения.
	Catalog *catalog.State
	// Changes — catalog.Changes между принятым и предлагаемым снимками.
	Changes []catalog.Change
	// Blockers — актуальные ошибки кандидата, включая временные конфликты.
	Blockers []*diagnostic.Error
}

// catalogEdit сохраняет неподтверждённый кандидат отдельно от placement.
type catalogEdit struct {
	// key — идентичность конкретного предложения.
	key *reviewKey
	// base — принятый снимок, на котором построено предложение.
	base *reviewKey
	// candidate — ненулевой результат операции catalog, не доступный снаружи.
	candidate *catalog.State
	// issues — блокировки предлагаемого снимка.
	issues []*diagnostic.Error
}

// Все Propose-методы сначала вызывают checkCatalogEdit. Новое предложение
// при активном edit отклоняется до RejectEdit/AcceptEdit; незавершённый
// ввод группы не создаёт edit. Пока edit активен, ReviewSave/Save запрещены.

// checkCatalogEdit проверяет token, готовность каталога и режим. При Update
// сначала требуются все решения исходных section/order, чтобы ручная правка
// не сделала исходное «было → станет» устаревшим. Сохранение недоступно
// во время отдельного предложения; ранее данное согласие отзывается.
func (op *Operation) checkCatalogEdit(token ReviewToken) *diagnostic.Error {
	if problem := op.checkReview(token); problem != nil {
		return problem
	}
	if !op.analyzed || op.placement == nil || op.current == nil || (op.mode != Create && op.mode != Update && op.mode != Add) {
		return &diagnostic.Error{Code: CodeState, Reason: "Каталог ещё не готов к редактированию."}
	}
	if op.edit != nil {
		return &diagnostic.Error{Code: CodeState, Reason: "Сначала примите или отклоните отдельное предложение правки."}
	}
	if op.mode == Update {
		if len(op.placementDecisions) != len(op.proposals) {
			return &diagnostic.Error{Code: CodeDecision, Reason: "Сначала решите все исходные предложения секций и весов."}
		}
		for _, decision := range op.placementDecisions {
			if decision != "accept" && decision != "reject" {
				return &diagnostic.Error{Code: CodeDecision, Reason: "Сначала решите все исходные предложения секций и весов."}
			}
		}
	}
	return nil
}

// ProposeCourseTitle вызывает catalog.SetCourseTitle и stageCatalogEdit.
func (op *Operation) ProposeCourseTitle(token ReviewToken, title string) (*EditPreview, []*diagnostic.Error) {
	if problem := op.checkCatalogEdit(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate, issues := catalog.SetCourseTitle(op.placement, title)
	return op.stageCatalogEdit(candidate, issues)
}

// ProposeUnitTitle вызывает catalog.SetUnitTitle и stageCatalogEdit;
// пустое название предлагает убрать юнит, без изменения уроков.
func (op *Operation) ProposeUnitTitle(token ReviewToken, title string) (*EditPreview, []*diagnostic.Error) {
	if problem := op.checkCatalogEdit(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate, issues := catalog.SetUnitTitle(op.placement, title)
	return op.stageCatalogEdit(candidate, issues)
}

// ProposeSection нормализует пользовательское имя только через
// catalog.NormalizeSectionTitle, затем catalog.CreateSection на отдельной копии.
// Предложение показывает нормализованное имя; до AcceptEdit оно не принято.
// Доступно при Update/Add; пустая секция может требовать распределить все уроки.
func (op *Operation) ProposeSection(token ReviewToken, rawTitle string) (*EditPreview, []*diagnostic.Error) {
	if problem := op.checkCatalogEdit(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	if op.mode == Create {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Создание секций доступно при обновлении и добавлении."}}
	}
	title, problem := catalog.NormalizeSectionTitle(rawTitle, diagnostic.Context{})
	if problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate, issues := catalog.CreateSection(catalog.Clone(op.placement), title)
	return op.stageCatalogEdit(candidate, issues)
}

// ProposeMove вызывает catalog.MoveLesson для одного технического имени.
// nil означает отсутствие секции; введённое имя сначала нормализует catalog.
// Разрешён при Update/Add; не подтверждает другие переносы или новые веса.
func (op *Operation) ProposeMove(token ReviewToken, fileName string, section *string) (*EditPreview, []*diagnostic.Error) {
	if problem := op.checkCatalogEdit(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	if op.mode == Create {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Перенос уроков доступен при обновлении и добавлении."}}
	}
	if section != nil {
		title, problem := catalog.NormalizeSectionTitle(*section, diagnostic.Context{FileName: fileName})
		if problem != nil {
			return nil, []*diagnostic.Error{problem}
		}
		section = &title
	}
	candidate, issues := catalog.MoveLesson(op.placement, fileName, section)
	return op.stageCatalogEdit(candidate, issues)
}

// ProposeFlatPlacement вызывает catalog.UseFlatPlacement; не снимает секции
// со всех записей автоматически. Переносы пользователь подтверждает отдельно.
func (op *Operation) ProposeFlatPlacement(token ReviewToken) (*EditPreview, []*diagnostic.Error) {
	if problem := op.checkCatalogEdit(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	if op.mode == Create {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Изменение распределения по секциям доступно при обновлении и добавлении."}}
	}
	candidate, issues := catalog.UseFlatPlacement(op.placement)
	return op.stageCatalogEdit(candidate, issues)
}

// ProposeLessonWeight вызывает catalog.SetLessonWeight для одной записи,
// в частности для отсутствующего веса; не выполняет собственную арифметику.
func (op *Operation) ProposeLessonWeight(token ReviewToken, fileName string, raw string) (*EditPreview, []*diagnostic.Error) {
	if problem := op.checkCatalogEdit(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate, issues := catalog.SetLessonWeight(op.placement, fileName, raw)
	return op.stageCatalogEdit(candidate, issues)
}

// ProposeLessonWeights принимает только полный набор весов выбранной группы:
// keys — все её FileName ровно по одному, без чужих файлов. console собирает
// последовательный ввод локально и при отмене не вызывает этот метод.
// Координирует checkWeightMembers, applyGroupWeights и stageCatalogEdit.
// Изменения выполняются на catalog.Clone; ошибка любого ввода отклоняет
// весь набор. Временные конфликты промежуточных ненулевых кандидатов не мешают
// завершить набор; наружу идут только итоговые ошибки catalog.Refresh.
func (op *Operation) ProposeLessonWeights(token ReviewToken, section *string, weights map[string]string) (*EditPreview, []*diagnostic.Error) {
	if problem := op.checkCatalogEdit(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	if problem := checkWeightMembers(op.placement, section, weights); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	candidate, issues := applyGroupWeights(op.placement, weights)
	return op.stageCatalogEdit(candidate, issues)
}

// checkWeightMembers проверяет полноту набора по текущим catalog.Items;
// записи весов проверяет только catalog.SetLessonWeight, не app.
func checkWeightMembers(value *catalog.State, section *string, weights map[string]string) *diagnostic.Error {
	if value == nil {
		return &diagnostic.Error{Code: CodeState, Reason: "Отсутствует каталог для ввода весов."}
	}
	count := 0
	for _, item := range value.Items {
		if (item.Section == nil) != (section == nil) || (section != nil && *item.Section != *section) {
			continue
		}
		count++
		if _, exists := weights[item.Entry.FileName]; !exists {
			return &diagnostic.Error{Code: CodeIncompleteWeights, Reason: "Нужно задать веса всем урокам выбранной группы.", Context: item.Context}
		}
	}
	if count != len(weights) {
		return &diagnostic.Error{Code: CodeIncompleteWeights, Reason: "Набор весов должен содержать только все уроки выбранной группы."}
	}
	return nil
}

// applyGroupWeights последовательно применяет SetLessonWeight к закрытой копии
// в порядке Items, а не map; nil любого шага отбрасывает весь кандидат.
func applyGroupWeights(value *catalog.State, weights map[string]string) (*catalog.State, []*diagnostic.Error) {
	candidate := catalog.Clone(value)
	if candidate == nil {
		return catalog.Refresh(nil)
	}
	for _, item := range value.Items {
		raw, exists := weights[item.Entry.FileName]
		if !exists {
			continue
		}
		next, issues := catalog.SetLessonWeight(candidate, item.Entry.FileName, raw)
		if next == nil {
			return nil, issues
		}
		candidate = next
	}
	return catalog.Refresh(candidate)
}

// ProposeSectionWeights передаёт полный независимый срез в
// catalog.SetSectionWeights; полноту, числа и сортировку проверяет владелец.
// Отмена последовательного ввода не вызывает этот API, частичного набора нет.
func (op *Operation) ProposeSectionWeights(token ReviewToken, weights []catalog.SectionOrder) (*EditPreview, []*diagnostic.Error) {
	if problem := op.checkCatalogEdit(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	independent := catalog.Clone(&catalog.State{Sections: weights}).Sections
	candidate, issues := catalog.SetSectionWeights(op.placement, independent)
	return op.stageCatalogEdit(candidate, issues)
}

// stageCatalogEdit не принимает nil candidate: тогда возвращает ошибки без
// изменения placement. Ненулевой кандидат с ошибками сохраняется отдельно,
// получает собственный токен и независимый EditPreview. Новое предложение
// не заменяет другое активное предложение. Только AcceptEdit может
// заменить принятое размещение.
func (op *Operation) stageCatalogEdit(candidate *catalog.State, issues []*diagnostic.Error) (*EditPreview, []*diagnostic.Error) {
	if op == nil {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Операция не задана."}}
	}
	if op.edit != nil {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Сначала примите или отклоните отдельное предложение правки."}}
	}
	if candidate == nil {
		return nil, cloneDiagnostics(issues)
	}
	op.edit = &catalogEdit{key: &reviewKey{owner: op}, base: op.key, candidate: catalog.Clone(candidate), issues: cloneDiagnostics(issues)}
	op.saving = nil
	return op.copyEditPreview(), cloneDiagnostics(issues)
}

// AcceptEdit проверяет токен предложения и его base, принимает весь кандидат,
// отзывает прежнее согласие через invalidateReview и вызывает refreshPlan.
// Отдельное согласие не является окончательным согласием на запись архива.
func (op *Operation) AcceptEdit(token ReviewToken) []*diagnostic.Error {
	if op == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Операция не задана."}}
	}
	if problem := op.checkReview(ReviewToken{key: op.key}); problem != nil {
		return []*diagnostic.Error{problem}
	}
	if op.edit == nil || token.key == nil || token.key.owner != op || token.key != op.edit.key || op.edit.base != op.key {
		return []*diagnostic.Error{{Code: CodeStaleReview, Reason: "Предложение правки больше не актуально."}}
	}
	if op.edit.candidate == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует кандидат правки каталога."}}
	}
	op.placement = op.edit.candidate
	op.invalidateReview()
	return cloneDiagnostics(op.refreshPlan())
}

// RejectEdit отбрасывает только указанное актуальное предложение. Принятый
// catalog, файлы, решения полей и токен принятого снимка остаются прежними.
func (op *Operation) RejectEdit(token ReviewToken) []*diagnostic.Error {
	if op == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Операция не задана."}}
	}
	if problem := op.checkReview(ReviewToken{key: op.key}); problem != nil {
		return []*diagnostic.Error{problem}
	}
	if op.edit == nil || token.key == nil || token.key.owner != op || token.key != op.edit.key || op.edit.base != op.key {
		return []*diagnostic.Error{{Code: CodeStaleReview, Reason: "Предложение правки больше не актуально."}}
	}
	op.edit = nil
	return nil
}
