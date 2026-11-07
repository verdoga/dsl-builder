package console

import (
	"errors"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/preview"
)

// decideField выбирает первый ещё не отвеченный индекс Plan.Fields по
// FieldAnswered. Печатает preview.FieldProposal(snapshot.Before,
// snapshot.Plan.Fields[index]), включая OldPage=-1 и After=nil без исключений.
// После отдельного «Применить это изменение? 1. Да / 2. Нет» передаёт ответ
// Operation.DecideField(snapshot.Token, index, accept). Одного ответа на все
// поля нет. Следующее поле спрашивается только из нового Snapshot и меню.
// Отмена всей операции доступна между полями через главное меню/выход.
func (d *dialog) decideField(snapshot app.Snapshot) error {
	if !snapshot.Actions.DecideFields {
		return nil
	}
	if snapshot.Plan == nil {
		return presentationError(errors.New("Отсутствует план предложений отдельных полей."))
	}
	for index, change := range snapshot.Plan.Fields {
		if index < len(snapshot.FieldAnswered) && snapshot.FieldAnswered[index] {
			continue
		}
		text, err := preview.FieldProposal(snapshot.Before, change)
		if err != nil {
			return presentationError(err)
		}
		if err := d.writeText(text); err != nil {
			return err
		}
		accept, err := d.confirm("Применить это изменение?")
		if err != nil {
			return err
		}
		return d.showIssues(d.operation.DecideField(snapshot.Token, index, accept))
	}
	return nil
}

// decidePlacement выбирает первый неотвеченный индекс PlacementAnswered,
// печатает preview.ManifestChanges с единственным элементом среза
// snapshot.PlacementProposals[index:index+1] и получает отдельное 1/2.
// Передаёт Operation.DecidePlacement(snapshot.Token, index, accept).
// Решение по section не подтверждает order; ручные правки ждут всех решений.
func (d *dialog) decidePlacement(snapshot app.Snapshot) error {
	if !snapshot.Actions.DecidePlacement {
		return nil
	}
	for index := range snapshot.PlacementProposals {
		if index < len(snapshot.PlacementAnswered) && snapshot.PlacementAnswered[index] {
			continue
		}
		text, err := preview.ManifestChanges(snapshot.PlacementProposals[index : index+1])
		if err != nil {
			return presentationError(err)
		}
		if err := d.writeText(text); err != nil {
			return err
		}
		accept, err := d.confirm("Применить это изменение размещения?")
		if err != nil {
			return err
		}
		return d.showIssues(d.operation.DecidePlacement(snapshot.Token, index, accept))
	}
	return nil
}

// prepareRevision по выбору actionRevision вызывает
// Operation.PrepareRevision(snapshot.Token) ровно один раз, только при
// snapshot.Actions.PrepareRevision. Token принадлежит актуальному показанному
// снимку. Все возвращённые ошибки показывает через showIssues; предметная
// ошибка после успешного показа возвращает управление operationLoop, а сбой
// потока прекращает диалог. Ни успех, ни ошибка не запускают повтор сами.
// Следующий шаг operationLoop — новый Operation.Snapshot и его показ;
// прежние NoChanges, Blockers и RevisionReady повторно не оцениваются.
// Только новый RevisionReady=true допускает итоговые ID и проверку условий
// noChangesChoice. При false доступны дальнейшие действия из нового Actions,
// включая явную повторную подготовку, если она разрешена. Значения готовности
// и отсутствия изменений принадлежат app; console их не вычисляет и не меняет.
func (d *dialog) prepareRevision(snapshot app.Snapshot) error {
	if !snapshot.Actions.PrepareRevision {
		return nil
	}
	return d.showIssues(d.operation.PrepareRevision(snapshot.Token))
}
