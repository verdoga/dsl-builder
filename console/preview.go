package console

import (
	"errors"
	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/preview"
)

// showSnapshot — небольшой координатор showComposition, showUpdate,
// showManifest и showIssues(snapshot.Blockers). Отсутствующий Catalog/Module
// после провала анализа не передаёт обязательным параметрам preview; вместо
// обещания готового превью показывает диагностику и доступные действия.
// Для ресурсов вызывает preview.Resources(snapshot.Resources), только когда
// Actions.UnusedResources сообщает о доступности отчёта текущего анализа.
// Ошибки разных блоков сохраняет вместе, не выдавая неполный вывод за полный.
// Ошибки preview возвращает через presentationError, ошибки потока сохраняет
// отдельно и прекращает вывод сразу. Не печатает Node.Data и Base64.
func (d *dialog) showSnapshot(snapshot app.Snapshot) error {
	var failures error
	blocks := []func() error{
		func() error {
			if snapshot.Catalog != nil && snapshot.Module != nil {
				return d.showComposition(snapshot)
			}
			if err := d.writeText("Состав пока недоступен для предварительного просмотра. Дальнейшие действия доступны после диагностики.\n"); err != nil {
				return err
			}
			return d.writeText(preview.OrderConflicts(snapshot.Conflicts))
		},
		func() error { return d.showUpdate(snapshot) },
		func() error { return d.showManifest(snapshot) },
		func() error {
			if snapshot.Actions.UnusedResources {
				return d.writeText(preview.Resources(snapshot.Resources))
			}
			return nil
		},
		func() error { return d.showIssues(snapshot.Blockers) },
	}
	for _, block := range blocks {
		err := block()
		failures = errors.Join(failures, err)
		if isStreamFailure(err) {
			return failures
		}
	}
	return failures
}

// showComposition вызывает preview.Composition(snapshot.Catalog,
// snapshot.Module, snapshot.NewFileNames), затем отдельно всегда показывает
// preview.OrderConflicts(snapshot.Conflicts), даже если Composition ошибся.
// До ManifestReady не использует Module.Manifest как готовый результат.
func (d *dialog) showComposition(snapshot app.Snapshot) error {
	text, err := preview.Composition(snapshot.Catalog, snapshot.Module, snapshot.NewFileNames)
	var failure error
	if err != nil {
		failure = presentationError(err)
	} else if err := d.writeText(text); err != nil {
		return err
	}
	return errors.Join(failure, d.writeText(preview.OrderConflicts(snapshot.Conflicts)))
}

// showUpdate только при Mode=app.Update и RevisionReady вызывает
// preview.UpdateHeading(ArchivePath, Before, After),
// preview.AcceptedFields(Before, After, AcceptedFields) и
// preview.Replacements(Before, After, Plan), сохраняя каждый результат/ошибку.
// До RevisionReady сообщает, что итоговые версии и ID ещё не подготовлены,
// и не подменяет After первичным кандидатом или вычисленными console ID.
func (d *dialog) showUpdate(snapshot app.Snapshot) error {
	if snapshot.Mode != app.Update {
		return nil
	}
	if !snapshot.RevisionReady {
		return d.writeText("Итоговые версии и ID ещё не подготовлены. Перейдите к итоговому просмотру изменений отдельным пунктом меню после решений по полям.\n")
	}
	blocks := []func() (string, error){
		func() (string, error) {
			return preview.UpdateHeading(snapshot.ArchivePath, snapshot.Before, snapshot.After)
		},
		func() (string, error) {
			return preview.AcceptedFields(snapshot.Before, snapshot.After, snapshot.AcceptedFields)
		},
		func() (string, error) { return preview.Replacements(snapshot.Before, snapshot.After, snapshot.Plan) },
	}
	var failures error
	for _, block := range blocks {
		text, err := block()
		if err != nil {
			failures = errors.Join(failures, presentationError(err))
			continue
		}
		if err := d.writeText(text); err != nil {
			return errors.Join(failures, err)
		}
	}
	return failures
}

// showManifest передаёт snapshot.ManifestChanges в preview.ManifestChanges.
// Изменения курса/юнита/секций/записей не вычисляет и не фильтрует заново.
func (d *dialog) showManifest(snapshot app.Snapshot) error {
	text, err := preview.ManifestChanges(snapshot.ManifestChanges)
	if err != nil {
		return presentationError(err)
	}
	return d.writeText(text)
}

// showUnused печатает preview.UnusedResources(snapshot.Resources) и отдельный
// пункт «1. Вернуться к предварительному просмотру». Возврат не изменяет app.
func (d *dialog) showUnused(snapshot app.Snapshot) error {
	if err := d.writeText(preview.UnusedResources(snapshot.Resources)); err != nil {
		return err
	}
	_, err := d.choose("Продолжение", []string{"Вернуться к предварительному просмотру"})
	return err
}

// showEdit показывает preview.Composition(edit.Catalog, snapshot.Module,
// snapshot.NewFileNames), preview.ManifestChanges(edit.Changes) и все
// edit.Blockers. Здесь состав нужен, в частности, для временного порядка
// секций и новой пустой секции, не входящих в экспортированный манифест.
// Ошибка формирования любого блока запрещает подтверждение этого кандидата.
func (d *dialog) showEdit(snapshot app.Snapshot, edit *app.EditPreview) error {
	if edit == nil {
		return presentationError(errors.New("Не задан кандидат правки для предварительного просмотра."))
	}
	show := func(text string, err error) error {
		if err != nil {
			return presentationError(err)
		}
		return d.writeText(text)
	}
	blocks := []func() error{
		func() error { return show(preview.Composition(edit.Catalog, snapshot.Module, snapshot.NewFileNames)) },
		func() error { return show(preview.ManifestChanges(edit.Changes)) },
		func() error { return d.showIssues(edit.Blockers) },
	}
	var failures error
	for _, block := range blocks {
		err := block()
		failures = errors.Join(failures, err)
		if isStreamFailure(err) {
			return failures
		}
	}
	return failures
}
