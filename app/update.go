package app

import (
	"encoding/json"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/revision"
)

// compareUpdate сохраняет newFile из converted, вызывает compare.Build
// для oldFile.Lesson()/newFile.Lesson() и catalog.ProposePlacement
// с input.Result.Document.Metadata.Section/Order. План, оба файла и предложения
// остаются неизменными до Retry. current первоначально копирует baseline,
// revised=oldFile; отсутствие order не порождает предложение удаления веса.
// Каждое Fields и каждое предложение размещения получает отдельное ожидание.
func (op *Operation) compareUpdate() []*diagnostic.Error {
	if op == nil || op.mode != Update || op.baseline == nil || op.beforeCatalog == nil || op.oldFile == nil || len(op.converted) != 1 || op.converted[0] == nil || len(op.inputs) != 1 || op.inputs[0] == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Для сравнения нужны прежний урок и один подготовленный источник."}}
	}
	candidate := op.converted[0]
	plan, issues := compare.Build(op.oldFile.Lesson(), candidate.Lesson(), op.oldFile.Context(), candidate.Context())
	if len(issues) != 0 {
		return cloneDiagnostics(issues)
	}
	metadata := op.inputs[0].Result.Document.Metadata
	proposals, issues := catalog.ProposePlacement(op.beforeCatalog, op.oldFile.FileName(), metadata.Section, metadata.Order)
	if len(issues) != 0 {
		return cloneDiagnostics(issues)
	}
	op.newFile, op.plan = candidate, plan
	op.fields = make([]revision.Decision, len(plan.Fields))
	op.proposals = cloneChanges(proposals)
	op.placementDecisions = make([]revision.Decision, len(proposals))
	op.current, op.placement = cloneModule(op.baseline), catalog.Clone(op.beforeCatalog)
	op.revised, op.lessonChanged = op.oldFile, false
	op.revisionReady, op.manifestReady = false, false
	return nil
}

// DecideField принимает одно решение по индексу полного plan.Fields.
// token должен адресовать текущий Snapshot; index проверяется, включая
// PageTitle с OldPage=-1 и After=nil. Нет решения «принять все поля».
// Сохраняет revision.Accept либо Reject; урок пока не пересобирает.
// После изменения уже принятого решения revisionReady сбрасывается,
// итоговые данные до PrepareRevision недоступны для записи; согласие отзывается.
// Reject не подменяет поля принятого урока значениями кандидата.
func (op *Operation) DecideField(token ReviewToken, index int, accept bool) []*diagnostic.Error {
	if problem := op.checkReview(token); problem != nil {
		return []*diagnostic.Error{problem}
	}
	if op.mode != Update || !op.analyzed || op.plan == nil || op.edit != nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Решение поля доступно только для подготовленного обновления без активной правки."}}
	}
	if index < 0 || index >= len(op.plan.Fields) || len(op.fields) != len(op.plan.Fields) {
		return []*diagnostic.Error{{Code: CodeDecision, Reason: "Индекс не выбирает отдельное изменение поля.", Context: op.oldFile.Context()}}
	}
	decision := revision.Reject
	if accept {
		decision = revision.Accept
	}
	if op.fields[index] == decision {
		return nil
	}
	op.fields[index] = decision
	op.revisionReady, op.manifestReady = false, false
	op.invalidateReview()
	return nil
}

// PrepareRevision требует явного решения по каждому plan.Fields.
// Вызывает applyRevision и syncRevision, затем publishRevision и refreshPlan.
// До успешной подготовки обоих результатов op не меняется; публикация атомарна.
// Повторное применение использует только oldFile/newFile, исходный plan
// и стабильное op.at, никогда не предыдущий revised. Одна операция — +1 версия.
func (op *Operation) PrepareRevision(token ReviewToken) []*diagnostic.Error {
	if problem := op.checkReview(token); problem != nil {
		return []*diagnostic.Error{problem}
	}
	if op.mode != Update || !op.analyzed || op.plan == nil || op.oldFile == nil || op.newFile == nil || op.current == nil || op.placement == nil || op.edit != nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Нет подготовленного обновления для расчёта ревизии."}}
	}
	if len(op.fields) != len(op.plan.Fields) {
		return []*diagnostic.Error{{Code: CodeDecision, Reason: "Требуется отдельное решение по каждому изменению поля.", Context: op.oldFile.Context()}}
	}
	for _, decision := range op.fields {
		if decision != revision.Accept && decision != revision.Reject {
			return []*diagnostic.Error{{Code: CodeDecision, Reason: "Требуется отдельное решение по каждому изменению поля.", Context: op.oldFile.Context()}}
		}
	}
	file, changed, issues := op.applyRevision()
	if len(issues) != 0 {
		return cloneDiagnostics(issues)
	}
	value, issues := op.syncRevision(file)
	if value == nil {
		return cloneDiagnostics(issues)
	}
	op.publishRevision(file, changed, value)
	return op.refreshPlan()
}

// applyRevision вызывает revision.Apply с неизменными oldFile.Lesson(),
// newFile.Lesson(), plan, fields и контекстами этих файлов. При changed=false
// итогом остаётся сам oldFile с OriginalBytes; возвращённая копия урока
// не превращается в NewFile. При true применяется module.ReplaceFile.
func (op *Operation) applyRevision() (*module.File, bool, []*diagnostic.Error) {
	result, changed, issues := revision.Apply(op.oldFile.Lesson(), op.newFile.Lesson(), op.plan, op.fields, op.at, op.oldFile.Context(), op.newFile.Context())
	if len(issues) != 0 {
		return nil, false, issues
	}
	if !changed {
		return op.oldFile, false, nil
	}
	file, issues := module.ReplaceFile(op.oldFile, result, op.newFile.Context())
	if len(issues) != 0 {
		return nil, false, issues
	}
	return file, true, nil
}

// syncRevision вызывает catalog.SyncLesson с окончательным file.Lesson().Document
// на копии текущего placement, сохраняющей принятые section/order.
// Ошибка nil State отклоняет результат целиком; ненулевой с блокировками
// остаётся редактируемым. op не меняется до publishRevision.
func (op *Operation) syncRevision(file *module.File) (*catalog.State, []*diagnostic.Error) {
	lesson := file.Lesson()
	if lesson == nil {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует окончательный файл обновляемого урока."}}
	}
	return catalog.SyncLesson(catalog.Clone(op.placement), file.FileName(), lesson.Document)
}

// publishRevision заменяет ровно один File в отдельном срезе current.Files,
// принимает подготовленный catalog и changed, устанавливает revisionReady
// и отзывает старое согласие. Входы здесь уже проверены предыдущими шагами.
func (op *Operation) publishRevision(file *module.File, changed bool, value *catalog.State) {
	current := cloneModule(op.current)
	for index, previous := range current.Files {
		if previous.FileName() == op.oldFile.FileName() {
			current.Files[index] = file
			break
		}
	}
	op.current, op.placement = current, value
	op.revised, op.lessonChanged = file, changed
	op.revisionReady, op.manifestReady = true, false
	op.invalidateReview()
}

// DecidePlacement отдельно решает одно неизменное предложение section/order.
// До вызова console показывает Snapshot.PlacementProposals[index].
// Accept применяет только указанное поле к текущему catalog через MoveLesson
// либо SetLessonWeight; Reject лишь отмечает отказ, не меняя принятый catalog.
// Ненулевой State с конфликтами может быть принят и исправлен из превью.
// nil State сохраняет ожидание и прежний снимок. Другие поля не подтверждаются.
// Уже решённый индекс повторно не применяется: следующие ручные изменения
// выполняются через отдельные Propose-методы. Accept отзывает прежний просмотр
// и вызывает refreshPlan; Reject сохраняет модель, но обновляет доступность действий.
func (op *Operation) DecidePlacement(token ReviewToken, index int, accept bool) []*diagnostic.Error {
	if problem := op.checkReview(token); problem != nil {
		return []*diagnostic.Error{problem}
	}
	if op.mode != Update || !op.analyzed || op.placement == nil || op.oldFile == nil || op.edit != nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Решение размещения доступно только для подготовленного обновления без активной правки."}}
	}
	if index < 0 || index >= len(op.proposals) || len(op.placementDecisions) != len(op.proposals) {
		return []*diagnostic.Error{{Code: CodeDecision, Reason: "Индекс не выбирает отдельное предложение размещения.", Context: op.oldFile.Context()}}
	}
	if op.placementDecisions[index] != "" {
		return []*diagnostic.Error{{Code: CodeDecision, Reason: "Предложение размещения уже решено; для новой правки создайте отдельное предложение.", Context: op.oldFile.Context()}}
	}
	if !accept {
		op.placementDecisions[index] = revision.Reject
		return nil
	}
	proposal := op.proposals[index]
	var after *string
	if err := json.Unmarshal(proposal.After, &after); err != nil {
		return []*diagnostic.Error{{Code: CodeDecision, Reason: "Не удалось прочитать значение предложения размещения.", Context: op.oldFile.Context(), Cause: err}}
	}
	var candidate *catalog.State
	var issues []*diagnostic.Error
	switch proposal.Field {
	case "section":
		candidate, issues = catalog.MoveLesson(catalog.Clone(op.placement), proposal.FileName, after)
	case "order":
		if after == nil {
			return []*diagnostic.Error{{Code: CodeDecision, Reason: "Предложение веса не содержит нового значения.", Context: op.oldFile.Context()}}
		}
		candidate, issues = catalog.SetLessonWeight(catalog.Clone(op.placement), proposal.FileName, *after)
	default:
		return []*diagnostic.Error{{Code: CodeDecision, Reason: "Предложение не является отдельным изменением секции или веса.", Context: op.oldFile.Context()}}
	}
	if candidate == nil {
		return cloneDiagnostics(issues)
	}
	op.placement, op.placementDecisions[index] = candidate, revision.Accept
	op.invalidateReview()
	return op.refreshPlan()
}
