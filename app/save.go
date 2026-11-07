package app

import (
	"os"
	"time"

	"github.com/verdoga/dsl-builder/catalog"
	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/revision"
	"github.com/verdoga/dsl-builder/safewrite"
)

// saveReview связывает итоговое согласие с составом, целью и её существованием.
type saveReview struct {
	// key — отдельная идентичность окончательного показа записи.
	key *reviewKey
	// base — принятый снимок, для которого получено согласие.
	base *reviewKey
	// target — независимое наблюдение показанной цели.
	target *targetState
	// confirmed — явное окончательное согласие на запись всего плана.
	confirmed bool
	// replaceExisting — отдельное явное разрешение перезаписи показанной цели.
	replaceExisting bool
}

// SavePreview представляет конкретный результат для окончательного решения.
type SavePreview struct {
	// Token — адрес этого предложения записи, не токен каталога или поля.
	Token ReviewToken
	// Snapshot — полный актуальный состав и различия, подлежащие показу.
	Snapshot Snapshot
	// Destination — полный путь и существование цели; никогда не nil.
	Destination Destination
	// ManifestTimeWillChange — корневая дата обновится при фактической попытке
	// записи; это единственное служебное изменение после согласия, до callback.
	ManifestTimeWillChange bool
}

// ReviewSave требует актуальный token, готовый итог, отсутствие блокировок,
// решений в ожидании и активной правки. Проверяет содержательные изменения:
// без них не создаёт предложение и не пишет. Проверяет цель и сохраняет её
// наблюдение для ConfirmSave/Save, возвращая независимое SavePreview.
// Для Update/Add сначала checkDestination сравнивает цель с наблюдением
// loadBaseline: нельзя принять изменённый с анализа архив как новую базу.
// Полный исходный путь всегда показывается как перезапись.
func (op *Operation) ReviewSave(token ReviewToken) (*SavePreview, []*diagnostic.Error) {
	if op == nil || op.key == nil || op.key.owner != op || op.finished {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Операция недоступна для показа записи."}}
	}
	if problem := op.checkReview(token); problem != nil {
		return nil, []*diagnostic.Error{problem}
	}
	if op.result != nil && (op.result.TemporaryPath != "" || (op.result.OriginalPath != "" && op.result.OriginalPath != op.result.TargetPath)) {
		return nil, []*diagnostic.Error{{Code: CodeRecovery, Reason: "Оставшиеся файлы требуют рассмотрения результата записи."}}
	}
	if !op.analyzed || op.current == nil || op.placement == nil || op.edit != nil {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Итоговый состав ещё не готов к подтверждению записи."}}
	}
	if op.result != nil && !op.result.Installed {
		op.refreshPlan()
	}
	if len(op.issues) != 0 {
		return nil, cloneDiagnostics(op.issues)
	}
	if !op.manifestReady {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Итоговый манифест ещё не готов к записи."}}
	}
	if op.mode == Update {
		if op.plan == nil || !op.revisionReady || len(op.fields) != len(op.plan.Fields) || len(op.placementDecisions) != len(op.proposals) {
			return nil, []*diagnostic.Error{{Code: CodeDecision, Reason: "Итог обновления или отдельные решения ещё не подготовлены."}}
		}
		for _, decisions := range [][]revision.Decision{op.fields, op.placementDecisions} {
			for _, decision := range decisions {
				if decision != revision.Accept && decision != revision.Reject {
					return nil, []*diagnostic.Error{{Code: CodeDecision, Reason: "Не получены все отдельные решения обновления."}}
				}
			}
		}
	}
	changed := op.lessonChanged
	if op.mode == Create {
		changed = len(op.current.Files) != 0
	} else {
		if op.baseline == nil || op.beforeCatalog == nil {
			return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует исходная модель архива."}}
		}
		changed = changed || len(catalog.Changes(op.beforeCatalog, op.placement)) != 0
	}
	if !changed {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Содержательных изменений для записи нет."}}
	}
	if op.target == nil {
		return nil, []*diagnostic.Error{{Code: CodeDestination, Reason: "Не выбрана цель записи."}}
	}
	if op.mode != Create {
		if problem := op.checkDestination(); problem != nil {
			return nil, []*diagnostic.Error{problem}
		}
	} else {
		observed, problem := inspectDestination(op.target.destination.Path)
		if problem != nil {
			return nil, []*diagnostic.Error{problem}
		}
		op.target = observed
	}
	target := *op.target
	op.saving = &saveReview{key: &reviewKey{owner: op}, base: op.key, target: &target}
	return &SavePreview{
		Token: ReviewToken{key: op.saving.key}, Snapshot: op.Snapshot(),
		Destination: target.destination, ManifestTimeWillChange: true,
	}, nil
}

// ConfirmSave получает два независимых факта ответа: согласие на запись и
// явное согласие перезаписать существующий файл. При Exists=true без второго
// разрешения запись недоступна; его console получает отдельным вопросом.
// accept=false отбрасывает saving, возвращая к прежнему превью без изменения
// моделей. Успех связан только с Token актуального SavePreview и его base.
func (op *Operation) ConfirmSave(token ReviewToken, accept bool, replaceExisting bool) []*diagnostic.Error {
	if op == nil || op.key == nil || op.key.owner != op || op.finished {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Операция недоступна для подтверждения записи."}}
	}
	if op.saving == nil || token.key == nil || token.key.owner != op || token.key != op.saving.key || op.saving.base != op.key {
		return []*diagnostic.Error{{Code: CodeStaleReview, Reason: "Предложение записи отсутствует либо устарело."}}
	}
	if !accept {
		op.saving = nil
		return nil
	}
	if op.result != nil && (op.result.TemporaryPath != "" || (op.result.OriginalPath != "" && op.result.OriginalPath != op.result.TargetPath)) {
		return []*diagnostic.Error{{Code: CodeRecovery, Reason: "Оставшиеся файлы требуют рассмотрения результата записи."}}
	}
	if len(op.issues) != 0 {
		return cloneDiagnostics(op.issues)
	}
	if !op.analyzed || op.current == nil || op.placement == nil || !op.manifestReady || op.edit != nil || op.saving.target == nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "План не готов к окончательному подтверждению записи."}}
	}
	if problem := op.checkDestination(); problem != nil {
		return []*diagnostic.Error{problem}
	}
	if op.saving.target.destination.Exists && !replaceExisting {
		op.saving.confirmed = false
		op.saving.replaceExisting = false
		return []*diagnostic.Error{{Code: CodeOverwrite, Reason: "Требуется отдельное согласие на перезапись существующего архива.", Context: diagnostic.Context{Path: op.saving.target.destination.Path}}}
	}
	op.saving.confirmed = true
	op.saving.replaceExisting = replaceExisting
	return nil
}

// Save — короткая последовательность checkSave, freezeForWrite,
// writeFrozen и recordWriteResult. Согласие потребляется перед первой попыткой;
// повторный вызов не повторяет запись. Ошибки до установки позволяют вернуться
// к превью и получить новое согласие, если не осталось recovery-файлов.
// Installed с ошибками завершает возможность записи этой Operation, но не
// объявляет полный успех. Никаких автоматических повторов или удаления резервы.
func (op *Operation) Save(at time.Time) (safewrite.Result, []*diagnostic.Error) {
	if issues := op.checkSave(); len(issues) != 0 {
		return safewrite.Result{}, issues
	}
	op.saving.confirmed = false
	value, issues := op.freezeForWrite(at)
	if len(issues) != 0 {
		op.saving = nil
		return safewrite.Result{}, issues
	}
	result, issues := op.writeFrozen(value)
	op.recordWriteResult(result, issues)
	return result, issues
}

// checkSave проверяет сохранённое согласие, текущий base, отсутствие правки,
// блокировок и ожиданий; вызывает checkDestination до любых записей.
func (op *Operation) checkSave() []*diagnostic.Error {
	if op == nil || op.key == nil || op.key.owner != op || op.finished {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Операция недоступна для записи."}}
	}
	if op.result != nil && (op.result.TemporaryPath != "" || (op.result.OriginalPath != "" && op.result.OriginalPath != op.result.TargetPath)) {
		return []*diagnostic.Error{{Code: CodeRecovery, Reason: "Оставшиеся файлы требуют рассмотрения результата записи."}}
	}
	if op.saving == nil || !op.saving.confirmed {
		return []*diagnostic.Error{{Code: CodeUnconfirmed, Reason: "Нет актуального окончательного согласия на запись."}}
	}
	if op.saving.key == nil || op.saving.key.owner != op || op.saving.base != op.key {
		return []*diagnostic.Error{{Code: CodeStaleReview, Reason: "Подтверждённый план записи устарел."}}
	}
	if len(op.issues) != 0 {
		return cloneDiagnostics(op.issues)
	}
	if !op.analyzed || op.current == nil || op.placement == nil || !op.manifestReady || op.edit != nil {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Для записи нужен готовый план без ожидающих правок."}}
	}
	if op.mode == Update {
		if op.plan == nil || !op.revisionReady || len(op.fields) != len(op.plan.Fields) || len(op.placementDecisions) != len(op.proposals) {
			return []*diagnostic.Error{{Code: CodeDecision, Reason: "Итог обновления или отдельные решения ещё не подготовлены."}}
		}
		for _, decisions := range [][]revision.Decision{op.fields, op.placementDecisions} {
			for _, decision := range decisions {
				if decision != revision.Accept && decision != revision.Reject {
					return []*diagnostic.Error{{Code: CodeDecision, Reason: "Не получены все отдельные решения обновления."}}
				}
			}
		}
	}
	changed := op.lessonChanged
	if op.mode == Create {
		changed = len(op.current.Files) != 0
	} else {
		if op.baseline == nil || op.beforeCatalog == nil {
			return []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует исходная модель архива."}}
		}
		changed = changed || len(catalog.Changes(op.beforeCatalog, op.placement)) != 0
	}
	if !changed {
		return []*diagnostic.Error{{Code: CodeState, Reason: "Содержательных изменений для записи нет."}}
	}
	shown := op.saving.target
	if shown == nil || op.target == nil || shown.destination != op.target.destination {
		return []*diagnostic.Error{{Code: CodeStaleReview, Reason: "Подтверждение относится к другой цели записи."}}
	}
	if (shown.info == nil) != (op.target.info == nil) || (shown.info != nil && (!os.SameFile(shown.info, op.target.info) || shown.info.Size() != op.target.info.Size() || !shown.info.ModTime().Equal(op.target.info.ModTime()))) {
		return []*diagnostic.Error{{Code: CodeTargetChanged, Reason: "Наблюдение цели изменилось после показа.", Context: diagnostic.Context{Path: shown.destination.Path}}}
	}
	if shown.destination.Exists && !op.saving.replaceExisting {
		return []*diagnostic.Error{{Code: CodeOverwrite, Reason: "Перезапись существующего архива не подтверждена.", Context: diagnostic.Context{Path: shown.destination.Path}}}
	}
	if problem := op.checkDestination(); problem != nil {
		return []*diagnostic.Error{problem}
	}
	return nil
}

// freezeForWrite вызывает catalog.Export с at.UTC().Format("2006-01-02T15:04:05Z"),
// присоединяет отдельный срез current.Files и вызывает module.Validate.
// Выполняется только после согласия и определения изменений, до safewrite.
// Меняет лишь корневой createdAt; даты/версии/lineCount уроков уже получены
// convert/revision. Результат закрыт от внешней правки и не меняется в callbacks.
func (op *Operation) freezeForWrite(at time.Time) (*module.Module, []*diagnostic.Error) {
	if op == nil || op.key == nil || op.key.owner != op || op.finished || op.current == nil || op.placement == nil || op.saving == nil || op.saving.target == nil || op.saving.base != op.key {
		return nil, []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует подготовленный состав для записи."}}
	}
	context := diagnostic.Context{Path: op.saving.target.destination.Path}
	manifest, issues := catalog.Export(op.placement, at.UTC().Format("2006-01-02T15:04:05Z"), context)
	if len(issues) != 0 {
		return nil, issues
	}
	value := &module.Module{Manifest: *manifest, Files: append([]*module.File(nil), op.current.Files...)}
	if issues := module.Validate(value, context); len(issues) != 0 {
		return nil, issues
	}
	return value, nil
}

// writeFrozen вызывает safewrite.Write с показанным полным путём,
// подтверждённым replaceExisting, newWriteCallback и verifyCallback.
// Ни один callback не преобразует уроки, не обновляет дату и не меняет catalog.
func (op *Operation) writeFrozen(value *module.Module) (safewrite.Result, []*diagnostic.Error) {
	if op == nil || op.key == nil || op.key.owner != op || op.finished || op.saving == nil || op.saving.target == nil || op.saving.base != op.key || value == nil {
		return safewrite.Result{}, []*diagnostic.Error{{Code: CodeState, Reason: "Отсутствует закрытый кандидат записи."}}
	}
	path := op.saving.target.destination.Path
	return safewrite.Write(path, op.saving.replaceExisting, newWriteCallback(value, diagnostic.Context{Path: path}), verifyCallback)
}

// recordWriteResult сохраняет Result и структурированные ошибки, отзывает
// согласие. Полный успех — только Installed=true и отсутствие всех ошибок.
// Installed с ошибками очистки показывается как установленный результат с
// неполным завершением; OriginalPath/TemporaryPath остаются видимы.
// Непустой TemporaryPath либо OriginalPath вне TargetPath означает остаточные
// recovery-файлы и блокирует Save/Retry. OriginalPath=TargetPath после полного
// отката сам по себе не считается остаточной резервой.
func (op *Operation) recordWriteResult(result safewrite.Result, issues []*diagnostic.Error) {
	if op == nil || op.key == nil || op.key.owner != op || op.finished {
		return
	}
	op.result = &result
	op.issues = cloneDiagnostics(issues)
	op.saving = nil
	op.finished = result.Installed
}
