package console

import (
	"errors"
	"strings"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

// showWriteResult читает snapshot.WriteResult, без собственной модели результата.
// Полный успех сообщает только при Installed=true и отсутствии всех issues,
// печатая полный TargetPath. Installed=true с issues означает установленный
// результат с ошибками завершения, а не «успешно сохранён» или «не изменён».
// При ошибках всегда показывает все известные TargetPath, OriginalPath и
// TemporaryPath с отдельными подписями, не гарантируя существование после
// внешних изменений. Все issues выводятся showIssues, включая ошибки очистки.
// Nil WriteResult не выдаётся за файловый результат и не вызывает повтор Save.
func (d *dialog) showWriteResult(snapshot app.Snapshot, issues []*diagnostic.Error) error {
	result := snapshot.WriteResult
	hasIssues := false
	for _, issue := range issues {
		hasIssues = hasIssues || issue != nil
	}
	var text strings.Builder
	if result == nil {
		text.WriteString("Сведения о результате файловой записи отсутствуют.\n")
	} else if result.Installed && !hasIssues {
		text.WriteString("Модуль успешно сохранён:\n" + displayValue(result.TargetPath) + "\n")
	} else {
		if result.Installed {
			text.WriteString("Модуль установлен, но завершение записи выполнено не полностью.\n")
		} else {
			text.WriteString("Результат записи не установлен.\n")
		}
		if result.TargetPath != "" {
			text.WriteString("Целевой файл: " + displayValue(result.TargetPath) + "\n")
		}
		if result.OriginalPath != "" {
			text.WriteString("Сохранённый исходник: " + displayValue(result.OriginalPath) + "\n")
		}
		if result.TemporaryPath != "" {
			text.WriteString("Временный результат: " + displayValue(result.TemporaryPath) + "\n")
		}
		text.WriteString("Пути отражают последнее известное состояние; существование файлов после внешних изменений не гарантируется.\n")
	}
	if err := d.writeText(text.String()); err != nil {
		return errors.Join(joinedIssues(issues), err)
	}
	return d.showIssues(issues)
}

// noChangesChoice сообщает об отсутствии содержательных изменений и записи,
// затем вызывает terminalChoice. Предусловия: актуальный показанный Snapshot,
// NoChanges=true, отсутствие Blockers и завершённые отдельные решения.
// Для Mode=app.Update дополнительно требуется RevisionReady=true после
// PrepareRevision и нового Snapshot. При RevisionReady=false вызов запрещён,
// даже если NoChanges=true, поля отсутствуют либо все предложения отклонены.
// Решения без подготовки ревизии не подтверждают отсутствие правок нод.
// ReviewSave/Save не вызываются; неверные предусловия не дают терминального
// сообщения и оставляют продолжение через обычное меню operationLoop.
func (d *dialog) noChangesChoice(snapshot app.Snapshot) (route, error) {
	if !snapshot.NoChanges || snapshot.Actions.DecideFields || snapshot.Actions.DecidePlacement ||
		(snapshot.Mode == app.Update && !snapshot.RevisionReady) {
		return routePreview, nil
	}
	if d.operation == nil || snapshot.Token != d.operation.Snapshot().Token {
		return routePreview, nil
	}
	for _, issue := range snapshot.Blockers {
		if issue != nil {
			return routePreview, nil
		}
	}
	if snapshot.Plan != nil && len(snapshot.FieldAnswered) != len(snapshot.Plan.Fields) {
		return routePreview, nil
	}
	for _, answered := range snapshot.FieldAnswered {
		if !answered {
			return routePreview, nil
		}
	}
	if len(snapshot.PlacementAnswered) != len(snapshot.PlacementProposals) {
		return routePreview, nil
	}
	for _, answered := range snapshot.PlacementAnswered {
		if !answered {
			return routePreview, nil
		}
	}
	if err := d.writeText("Содержательных изменений не обнаружено.\nЗапись архива не выполняется; урок, его версия и архив оставлены без изменений.\n"); err != nil {
		d.operation.Cancel()
		return routeExit, err
	}
	return d.terminalChoice(snapshot)
}

// terminalChoice предлагает «1. Вернуться в главное меню», «2. Выход»
// согласно Actions.MainMenu/Exit. Перед уходом вызывает Cancel: установленный
// результат не откатывается. Новый анализ recovery-файлов сам не запускается.
func (d *dialog) terminalChoice(snapshot app.Snapshot) (route, error) {
	var labels []string
	var routes []route
	if snapshot.Actions.MainMenu {
		labels = append(labels, "Вернуться в главное меню")
		routes = append(routes, routeMain)
	}
	if snapshot.Actions.Exit {
		labels = append(labels, "Выход")
		routes = append(routes, routeExit)
	}
	if len(labels) == 0 {
		return routePreview, nil
	}
	selected, err := d.choose("Продолжение работы:", labels)
	d.operation.Cancel()
	if err != nil {
		return routeExit, err
	}
	return routes[selected], nil
}

// presentationFailureChoice после успешно показанной ошибки представления
// предоставляет только Retry, Restart/MainMenu и Exit по Actions. Повторный
// показ сам по себе не даёт согласия; запись неполного превью недоступна.
func (d *dialog) presentationFailureChoice(snapshot app.Snapshot) (route, error) {
	if d.operation != nil {
		snapshot = d.operation.Snapshot()
	}
	var actions []action
	if snapshot.Actions.Retry && d.operation != nil {
		actions = append(actions, actionRetry)
	}
	if snapshot.Actions.MainMenu {
		actions = append(actions, actionRestart, actionMain)
	}
	if snapshot.Actions.Exit {
		actions = append(actions, actionExit)
	}
	if len(actions) == 0 {
		return routePreview, nil
	}
	selected, err := d.operationChoice(actions)
	if err != nil {
		d.operation.Cancel()
		return routeExit, err
	}
	if d.operation != nil {
		snapshot = d.operation.Snapshot()
	}
	switch selected {
	case actionRetry:
		if !snapshot.Actions.Retry {
			return routePreview, nil
		}
	case actionRestart, actionMain:
		if !snapshot.Actions.MainMenu {
			return routePreview, nil
		}
	case actionExit:
		if !snapshot.Actions.Exit {
			return routePreview, nil
		}
	default:
		return routePreview, nil
	}
	return d.dispatchAction(snapshot, selected)
}
