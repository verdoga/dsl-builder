package console

import (
	"errors"
	"fmt"
	"time"

	"github.com/verdoga/dsl-builder/app"
)

// chooseDestination при Actions.ChooseDestination читает отдельно обязательный
// каталог и обязательное имя ZIP. Отмена второго вопроса отбрасывает оба ответа.
// Только полный ввод вызывает Operation.SetDestination(snapshot.Token,
// destinationDirectory(directory), name). Проверки каталога, имени Windows,
// расширения и существования выполняет app. После успеха обязателен новый
// Snapshot; прежний Token и прежнее согласие далее не используются.
func (d *dialog) chooseDestination(snapshot app.Snapshot) error {
	if !snapshot.Actions.ChooseDestination {
		return nil
	}
	directory, accepted, err := d.readValue("Каталог назначения:", false)
	if err != nil {
		d.operation.Cancel()
		return err
	}
	if !accepted {
		return nil
	}
	name, accepted, err := d.readValue("Имя архива:", false)
	if err != nil {
		d.operation.Cancel()
		return err
	}
	if !accepted {
		return nil
	}
	issues := d.operation.SetDestination(snapshot.Token, destinationDirectory(directory), name)
	if err := d.showIssues(issues); err != nil {
		d.operation.Cancel()
		return err
	}
	return nil
}

// reviewSaving при Actions.ReviewSave вызывает Operation.ReviewSave с Token
// показанного снимка. Ошибки показывает; nil SavePreview не допускает вопроса
// о согласии. Ненулевой результат передаёт showSavePreview и confirmSaving.
// Ошибка представления закрывает предложение ConfirmSave(token,false,false),
// сохраняя обе причины через errors.Join; ошибка потока отменяет Operation.
func (d *dialog) reviewSaving(snapshot app.Snapshot) (route, error) {
	if !snapshot.Actions.ReviewSave {
		return routePreview, nil
	}
	value, issues := d.operation.ReviewSave(snapshot.Token)
	if err := d.showIssues(issues); err != nil {
		d.operation.Cancel()
		return routeExit, err
	}
	if value == nil {
		return routePreview, nil
	}
	err := d.showSavePreview(value)
	if isStreamFailure(err) {
		d.operation.Cancel()
		return routeExit, err
	}
	if err != nil {
		return routePreview, errors.Join(err, joinedIssues(d.operation.ConfirmSave(value.Token, false, false)))
	}
	if joinedIssues(issues) != nil {
		closing := d.operation.ConfirmSave(value.Token, false, false)
		if err := d.showIssues(closing); err != nil {
			d.operation.Cancel()
			return routeExit, err
		}
		return routePreview, nil
	}
	return d.confirmSaving(value)
}

// showSavePreview показывает полный showSnapshot(value.Snapshot), затем
// точные Destination.Path и Exists. ManifestTimeWillChange сопровождает
// сообщением о будущей смене даты манифеста. Путь и весь план показаны до
// первого вопроса согласия; ошибки любого блока запрещают подтверждение.
func (d *dialog) showSavePreview(value *app.SavePreview) error {
	if value == nil {
		return presentationError(errors.New("Не задано предложение записи для просмотра."))
	}
	failure := d.showSnapshot(value.Snapshot)
	if isStreamFailure(failure) {
		return failure
	}
	exists := "нет"
	if value.Destination.Exists {
		exists = "да"
	}
	if err := d.writeText(fmt.Sprintf("Полный путь результата: %s\nЦелевой файл существует: %s\n", displayValue(value.Destination.Path), exists)); err != nil {
		return errors.Join(failure, err)
	}
	if value.ManifestTimeWillChange {
		if err := d.writeText("При фактической записи будет изменена дата сборки манифеста.\n"); err != nil {
			return errors.Join(failure, err)
		}
	}
	return errors.Join(failure, joinedIssues(value.Snapshot.Blockers))
}

// confirmSaving спрашивает «Записать показанный модуль? 1. Да / 2. Нет».
// Для согласия и Destination.Exists=true отдельно показывает полный путь
// и спрашивает «Перезаписать существующий файл? 1. Да / 2. Нет».
// Все ответы остаются локальными до завершения обоих вопросов. Только затем
// вызывает ConfirmSave(value.Token, accept, replaceExisting). Отказ в любом
// вопросе передаёт accept=false, replaceExisting=false, возвращает превью.
// EOF/ошибка вывода отменяет Operation без ConfirmSave с согласием и без Save.
// После успеха запрашивает Snapshot: только Actions.Save разрешает writeOnce.
// Смена цели/плана между ответом и записью не обходится новыми токенами.
func (d *dialog) confirmSaving(value *app.SavePreview) (route, error) {
	if value == nil {
		return routePreview, presentationError(errors.New("Не задано предложение записи для подтверждения."))
	}
	accept, err := d.confirm("Записать показанный модуль?")
	if err != nil {
		d.operation.Cancel()
		return routeExit, err
	}
	replaceExisting := false
	if accept && value.Destination.Exists {
		if err := d.writeText("Существующий файл: " + displayValue(value.Destination.Path) + "\n"); err != nil {
			d.operation.Cancel()
			return routeExit, err
		}
		replaceExisting, err = d.confirm("Перезаписать существующий файл?")
		if err != nil {
			d.operation.Cancel()
			return routeExit, err
		}
		if !replaceExisting {
			accept = false
		}
	}
	issues := d.operation.ConfirmSave(value.Token, accept, replaceExisting)
	if err := d.showIssues(issues); err != nil {
		d.operation.Cancel()
		return routeExit, err
	}
	if !accept || joinedIssues(issues) != nil {
		return routePreview, nil
	}
	snapshot := d.operation.Snapshot()
	if !snapshot.Actions.Save {
		return routePreview, nil
	}
	return d.writeOnce()
}

// writeOnce вызывает только Operation.Save(time.Now()) один раз после
// confirmSaving. Файловые callbacks и транзакция остаются внутри app.
// Результат и ошибки сохраняет app; console использует новый Snapshot и
// showWriteResult. При Installed либо recovery-файлах предлагает только
// terminalChoice. Иначе возвращается в обычное меню доступных Actions:
// новая попытка возможна лишь через новый ReviewSave и новые ответы.
// Не повторяет Save автоматически и не удаляет/перемещает оставшиеся файлы.
func (d *dialog) writeOnce() (route, error) {
	_, issues := d.operation.Save(time.Now())
	snapshot := d.operation.Snapshot()
	if err := d.showWriteResult(snapshot, issues); err != nil {
		if isStreamFailure(err) {
			d.operation.Cancel()
			return routeExit, err
		}
		return routePreview, err
	}
	result := snapshot.WriteResult
	if result != nil && (result.Installed || result.TemporaryPath != "" || (result.OriginalPath != "" && result.OriginalPath != result.TargetPath)) {
		return d.terminalChoice(snapshot)
	}
	return routePreview, nil
}
