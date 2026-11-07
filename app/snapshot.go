package app

import (
	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/compare"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/resourceusage"
	"github.com/verdoga/dsl-builder/safewrite"
)

// ReviewToken связывает решение с конкретным снимком без открытых счётчиков.
// Нулевой, чужой и прежний токены отклоняются; это не разрешение записи.
type ReviewToken struct {
	// key — закрытая идентичность показанного состояния.
	key *reviewKey
}

// reviewKey отличает состояние операции по идентичности адреса.
type reviewKey struct {
	// owner — операция, которой принадлежит состояние.
	owner *Operation
}

// Snapshot — независимые подготовленные данные для console и preview.
// Изменение полей снимка не меняет op; Snapshot не принимается назад как команда.
type Snapshot struct {
	// Token — адрес именно этого принятого состояния для последующего решения.
	Token ReviewToken
	// Mode — сценарий для выбора применимого оформления.
	Mode Mode
	// ArchivePath — полный исходный путь ZIP либо пусто при создании.
	ArchivePath string
	// Catalog — независимый catalog.Clone, включая незавершённое размещение.
	Catalog *catalog.State
	// Module — независимый контейнер, полный Files даже при невозможном Export.
	// Manifest значим как итоговый только при ManifestReady; иначе это прежний
	// либо нулевой манифест. Для Composition используются Catalog и Module.Files.
	Module *module.Module
	// ManifestReady — Export и module.Validate успешно подготовили манифест.
	ManifestReady bool
	// NewFileNames — добавленные имена; при Create пусто для preview.Composition.
	NewFileNames []string
	// Conflicts — все готовые catalog.Conflicts текущего каталога.
	Conflicts []catalog.Conflict
	// Resources — Registry.Snapshot без суммирования счётчиков уроков.
	Resources resourceusage.Report
	// ManifestChanges — полный catalog.Changes относительно beforeCatalog;
	// при Create пусто: nil не передаётся API Changes, создание видно из состава.
	ManifestChanges []catalog.Change
	// Before — старый неизменяемый File для FieldProposal и UpdateHeading.
	Before *module.File
	// After — принятый итоговый File; готовность определяет RevisionReady.
	After *module.File
	// Plan — глубокая копия полного compare.Plan, без удаления Fields/Pages.
	Plan *compare.Plan
	// FieldAnswered — отдельный факт решения по каждому Plan.Fields.
	FieldAnswered []bool
	// FieldAccepted — выбранное согласие по тем же индексам; ожидание отдельно.
	FieldAccepted []bool
	// AcceptedFields — только принятые элементы Plan.Fields для preview.
	AcceptedFields []compare.FieldChange
	// RevisionReady — можно показывать итоговые ID и Replacements.
	RevisionReady bool
	// PlacementProposals — отдельные исходные section/order «было → станет».
	PlacementProposals []catalog.Change
	// PlacementAnswered — факт отдельного решения по каждому предложению.
	PlacementAnswered []bool
	// Target — полный путь и наблюдаемое существование цели до согласия.
	Target *Destination
	// NoChanges — после всех решений нет содержательной правки урока и
	// catalog.Changes; время исключено. До завершения решений и при Create false.
	NoChanges bool
	// Actions — применимые действия, без текста и нумерации меню.
	Actions Actions
	// Blockers — полный актуальный список структурированных блокировок записи.
	Blockers []*diagnostic.Error
	// WriteResult — фактический safewrite.Result; OriginalPath/TemporaryPath видны
	// и при Installed=true с ошибкой очистки. Успех требует также пустых ошибок.
	WriteResult *safewrite.Result
}

// Actions задаёт доступность конкретных операций для последовательного меню.
type Actions struct {
	// EditCatalog — доступны предложения названий и весов.
	EditCatalog bool
	// EditSections — доступны создание и переносы при Update/Add.
	EditSections bool
	// SortSections — есть секции для полного ввода весов.
	SortSections bool
	// DecideFields — остаются поля без решения.
	DecideFields bool
	// PrepareRevision — все поля решены, нужна подготовка итога.
	PrepareRevision bool
	// DecidePlacement — остаются предложения размещения без решения.
	DecidePlacement bool
	// ChooseDestination — при Create можно ввести либо изменить каталог и имя ZIP.
	ChooseDestination bool
	// MainMenu — возврат доступен всегда; незаписанное отменяется через Cancel.
	MainMenu bool
	// ReviewSave — готов состав с изменениями, нет ожидающих решений и блокировок.
	ReviewSave bool
	// Save — получено актуальное окончательное согласие и разрешение перезаписи.
	Save bool
	// Retry — можно повторить полный анализ после исправления исходников.
	Retry bool
	// Cancel — можно отменить незаписанную операцию и вернуться в главное меню.
	Cancel bool
	// Exit — выход доступен всегда; console предварительно вызывает Cancel.
	Exit bool
	// UnusedResources — доступен отчёт текущего анализа, включая пустой список.
	UnusedResources bool
}

// Snapshot создаёт независимые копии всех изменяемых данных через cloneModule,
// catalog.Clone, clonePlan и cloneDiagnostics; Report получен Registry.Snapshot.
// До создания Registry ресурсы представлены нулевым Report. Конфликты
// вычисляются на независимом Clone каталога, без ссылок на внутренние Items.
// Nil Operation даёт снимок с CodeState и доступными MainMenu/Exit.
// Не меняет дату, решения или готовность и не выполняет новый анализ.
func (op *Operation) Snapshot() Snapshot {
	result := Snapshot{Actions: Actions{MainMenu: true, Exit: true}}
	if op == nil {
		result.Blockers = []*diagnostic.Error{{Code: CodeState, Reason: "Операция не задана."}}
		return result
	}
	result.Token, result.Mode, result.ArchivePath = ReviewToken{key: op.key}, op.mode, op.archivePath
	result.Catalog, result.Module = catalog.Clone(op.placement), cloneModule(op.current)
	result.ManifestReady, result.RevisionReady = op.manifestReady, op.revisionReady
	result.Before, result.After, result.Plan = op.oldFile, op.revised, clonePlan(op.plan)
	result.PlacementProposals, result.Blockers = cloneChanges(op.proposals), cloneDiagnostics(op.issues)
	result.Conflicts = catalog.Conflicts(catalog.Clone(op.placement))
	if op.usage != nil {
		result.Resources = op.usage.Snapshot()
	}
	if op.mode == Add {
		result.NewFileNames = append([]string(nil), op.names...)
	}
	if op.beforeCatalog != nil && op.placement != nil {
		result.ManifestChanges = cloneChanges(catalog.Changes(op.beforeCatalog, op.placement))
	}
	fieldsReady, placementReady := true, true
	if result.Plan != nil {
		result.FieldAnswered, result.FieldAccepted = make([]bool, len(result.Plan.Fields)), make([]bool, len(result.Plan.Fields))
		for index, field := range result.Plan.Fields {
			answered := index < len(op.fields) && (op.fields[index] == "accept" || op.fields[index] == "reject")
			result.FieldAnswered[index] = answered
			fieldsReady = fieldsReady && answered
			if answered && op.fields[index] == "accept" {
				result.FieldAccepted[index] = true
				result.AcceptedFields = append(result.AcceptedFields, field)
			}
		}
	}
	result.PlacementAnswered = make([]bool, len(op.proposals))
	for index := range op.proposals {
		answered := index < len(op.placementDecisions) && (op.placementDecisions[index] == "accept" || op.placementDecisions[index] == "reject")
		result.PlacementAnswered[index] = answered
		placementReady = placementReady && answered
	}
	revisionReady := op.mode != Update || (op.plan != nil && fieldsReady && op.revisionReady)
	result.NoChanges = op.analyzed && op.mode != Create && revisionReady && placementReady && !op.lessonChanged && len(result.ManifestChanges) == 0
	if op.target != nil {
		target := op.target.destination
		result.Target = &target
	}
	recovery, installed := false, false
	if op.result != nil {
		written := *op.result
		result.WriteResult = &written
		installed = written.Installed
		recovery = written.TemporaryPath != "" || (written.OriginalPath != "" && written.OriginalPath != written.TargetPath)
	}
	owned := op.key != nil && op.key.owner == op
	if !owned || op.finished || installed || recovery {
		if !owned && !op.finished {
			result.Blockers = append(result.Blockers, &diagnostic.Error{Code: CodeState, Reason: "Операция не инициализирована либо скопирована."})
		}
		return result
	}
	result.Actions.Cancel, result.Actions.UnusedResources = true, op.usage != nil
	result.Actions.Retry = (op.tree != nil || op.analyzed) && len(op.issues) != 0
	if !op.analyzed || op.edit != nil {
		return result
	}
	editable := op.placement != nil && (op.mode != Update || placementReady)
	result.Actions.EditCatalog = editable
	result.Actions.EditSections = editable && (op.mode == Update || op.mode == Add)
	result.Actions.SortSections = editable && len(op.placement.Sections) != 0
	result.Actions.DecideFields = op.mode == Update && op.plan != nil && !fieldsReady
	result.Actions.PrepareRevision = op.mode == Update && op.plan != nil && fieldsReady && !op.revisionReady
	result.Actions.DecidePlacement = op.mode == Update && !placementReady
	result.Actions.ChooseDestination = op.mode == Create
	ready := op.current != nil && op.placement != nil && op.manifestReady && revisionReady && placementReady && len(op.issues) == 0 && !result.NoChanges && op.target != nil
	result.Actions.ReviewSave = ready
	saving := op.saving
	result.Actions.Save = ready && saving != nil && saving.key != nil && saving.key.owner == op && saving.base == op.key && saving.target != nil && saving.confirmed && (!saving.target.destination.Exists || saving.replaceExisting)
	return result
}

// refreshPlan заменяет устаревшие блокировки проверками catalog.Validate,
// catalog.Conflicts, catalog.Export и module.Validate при успешном экспорте.
// При блокировках сохраняет полный Files и catalog для исправления.
// Для готового размещения экспорт использует прежний baseline.Manifest.CreatedAt,
// при Create — op.at в UTC; текущие часы не создают различий.
// Changes вызывается только для двух ненулевых catalog с подготовленными
// записями; конфликты размещения не скрывают различия. При блокировках
// экспорт не считается готовым, но Snapshot сохраняет ManifestChanges.
// Изменения создания определены наличием новых файлов. Отсутствие изменений
// определяется отдельно от готовности и не разрешает запись ошибочного плана.
func (op *Operation) refreshPlan() []*diagnostic.Error {
	if op == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Операция не задана."}}
	}
	op.manifestReady = false
	op.issues = catalog.Validate(op.placement)
	if len(catalog.Conflicts(op.placement)) != 0 || len(op.issues) != 0 {
		return cloneDiagnostics(op.issues)
	}
	createdAt := op.at.UTC().Format("2006-01-02T15:04:05Z")
	if op.mode != Create && op.baseline != nil {
		createdAt = op.baseline.Manifest.CreatedAt
	}
	context := diagnostic.Context{Path: op.archivePath}
	manifestContext := context
	manifestContext.FileName = module.ManifestFileName
	manifest, issues := catalog.Export(op.placement, createdAt, manifestContext)
	op.issues = issues
	if manifest == nil || len(issues) != 0 {
		return cloneDiagnostics(op.issues)
	}
	candidate := cloneModule(op.current)
	if candidate != nil {
		candidate.Manifest = *manifest
	}
	op.issues = module.Validate(candidate, context)
	if len(op.issues) == 0 {
		op.current, op.manifestReady = candidate, true
	}
	return cloneDiagnostics(op.issues)
}

// invalidateReview выдаёт новую закрытую идентичность после принятого изменения,
// отбрасывает кандидата правки и любое согласие записи, включая перезапись.
// Отказ от правки и ошибка её ввода не изменяют принятую модель.
func (op *Operation) invalidateReview() {
	if op != nil {
		op.key, op.edit, op.saving = &reviewKey{owner: op}, nil, nil
	}
}

// checkReview проверяет принадлежность, актуальность и допустимость состояния.
func (op *Operation) checkReview(token ReviewToken) *diagnostic.Error {
	if op == nil || op.key == nil {
		return &diagnostic.Error{Code: CodeState, Reason: "Операция не инициализирована либо завершена."}
	}
	if op.key.owner != op || token.key == nil || token.key.owner != op || token.key != op.key {
		return &diagnostic.Error{Code: CodeStaleReview, Reason: "Подтверждение не относится к текущему снимку этой операции."}
	}
	if op.finished || !op.analyzed || (op.result != nil && op.result.Installed) {
		return &diagnostic.Error{Code: CodeState, Reason: "Действие недоступно на текущем этапе операции."}
	}
	if op.result != nil && (op.result.TemporaryPath != "" || (op.result.OriginalPath != "" && op.result.OriginalPath != op.result.TargetPath)) {
		return &diagnostic.Error{Code: CodeRecovery, Reason: "Сначала необходимо рассмотреть оставшиеся файлы предыдущей записи."}
	}
	return nil
}
