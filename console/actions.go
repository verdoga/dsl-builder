package console

import "github.com/verdoga/dsl-builder/app"

// action — адрес одного пункта текущего меню; не замена app.Actions.
type action string

const (
	// actionCourse — предложить изменение курса при EditCatalog.
	actionCourse action = "course"
	// actionUnit — предложить изменение или удаление юнита при EditCatalog.
	actionUnit action = "unit"
	// actionLessonWeight — задать вес одного урока при EditCatalog.
	actionLessonWeight action = "lesson_weight"
	// actionGroupWeights — задать весь набор весов группы при EditCatalog.
	actionGroupWeights action = "group_weights"
	// actionSectionWeights — задать веса всех секций при SortSections.
	actionSectionWeights action = "section_weights"
	// actionNewSection — создать секцию при EditSections.
	actionNewSection action = "new_section"
	// actionMove — перенести выбранный урок при EditSections.
	actionMove action = "move"
	// actionFlat — завершить плоское размещение при EditSections и всех Section=nil.
	actionFlat action = "flat"
	// actionField — решить одно ожидающее поле при DecideFields.
	actionField action = "field"
	// actionPlacement — решить одно предложение размещения при DecidePlacement.
	actionPlacement action = "placement"
	// actionRevision — подготовить итоговые ID при PrepareRevision.
	actionRevision action = "revision"
	// actionDestination — выбрать или изменить цель при ChooseDestination.
	actionDestination action = "destination"
	// actionSave — полный показ и согласие при ReviewSave; прямого обхода нет.
	actionSave action = "save"
	// actionUnused — просмотреть список при UnusedResources.
	actionUnused action = "unused"
	// actionRetry — повторно анализировать при Retry.
	actionRetry action = "retry"
	// actionRestart — начать тот же режим заново, когда доступен MainMenu.
	actionRestart action = "restart"
	// actionMain — главное меню при MainMenu; Cancel вызывается перед уходом.
	actionMain action = "main"
	// actionExit — выход при Exit; Cancel вызывается перед уходом.
	actionExit action = "exit"
)

// availableActions получает разрешения только из snapshot.Actions; отсутствие
// блокировок само по себе не разрешает запись. Пункты над пустыми списками
// скрывает: веса/перенос требуют Items, веса секций — Sections. actionFlat
// требует Sectioned=true и все Items.Section=nil, согласно ProposeFlatPlacement.
// Ожидающие section/order скрывают ручные правки: app.checkCatalogEdit требует
// их решений первыми. Не выводит недоступные действия как отключённые пункты.
// Save не становится самостоятельным меню: это продолжение confirmSaving
// непосредственно после ConfirmSave. app.Actions.Cancel покрывается actionMain.
func availableActions(snapshot app.Snapshot) []action {
	permissions := snapshot.Actions
	hasItems, hasSections, flat := false, false, false
	if snapshot.Catalog != nil {
		hasItems = len(snapshot.Catalog.Items) != 0
		hasSections = len(snapshot.Catalog.Sections) != 0
		flat = snapshot.Catalog.Sectioned
		for _, item := range snapshot.Catalog.Items {
			flat = flat && item.Section == nil
		}
	}
	pendingPlacement := permissions.DecidePlacement
	for index := range snapshot.PlacementProposals {
		pendingPlacement = pendingPlacement || index >= len(snapshot.PlacementAnswered) || !snapshot.PlacementAnswered[index]
	}
	var actions []action
	for _, entry := range []struct {
		allowed bool
		value   action
	}{
		{permissions.EditCatalog && !pendingPlacement, actionCourse},
		{permissions.EditCatalog && !pendingPlacement, actionUnit},
		{permissions.EditCatalog && !pendingPlacement && hasItems, actionLessonWeight},
		{permissions.EditCatalog && !pendingPlacement && hasItems, actionGroupWeights},
		{permissions.SortSections && !pendingPlacement && hasSections, actionSectionWeights},
		{permissions.EditSections && !pendingPlacement, actionNewSection},
		{permissions.EditSections && !pendingPlacement && hasItems, actionMove},
		{permissions.EditSections && !pendingPlacement && flat, actionFlat},
		{permissions.DecideFields, actionField},
		{permissions.DecidePlacement, actionPlacement},
		{permissions.PrepareRevision, actionRevision},
		{permissions.ChooseDestination, actionDestination},
		{permissions.ReviewSave, actionSave},
		{permissions.UnusedResources, actionUnused},
		{permissions.Retry, actionRetry},
		{permissions.MainMenu, actionRestart},
		{permissions.MainMenu || permissions.Cancel, actionMain},
		{permissions.Exit, actionExit},
	} {
		if entry.allowed {
			actions = append(actions, entry.value)
		}
	}
	return actions
}

// actionLabel задаёт русскую подпись пункта без номера; номер назначает choose.
// actionRevision подписан «Перейти к итоговому просмотру изменений».
func actionLabel(value action) string {
	return map[action]string{
		actionCourse:         "Изменить название курса",
		actionUnit:           "Задать, изменить или убрать название юнита",
		actionLessonWeight:   "Задать вес одного урока",
		actionGroupWeights:   "Задать веса урокам выбранной группы",
		actionSectionWeights: "Изменить порядок секций",
		actionNewSection:     "Создать секцию",
		actionMove:           "Перенести урок",
		actionFlat:           "Завершить размещение без секций",
		actionField:          "Рассмотреть изменение отдельного поля",
		actionPlacement:      "Рассмотреть изменение размещения",
		actionRevision:       "Перейти к итоговому просмотру изменений",
		actionDestination:    "Выбрать или изменить каталог и имя архива",
		actionSave:           "Перейти к подтверждению записи модуля",
		actionUnused:         "Показать неиспользованные файлы",
		actionRetry:          "Повторить анализ после исправления файлов",
		actionRestart:        "Начать операцию заново",
		actionMain:           "Отменить и вернуться в главное меню",
		actionExit:           "Выход",
	}[value]
}

// operationLoop получает свежий Snapshot, показывает его и меню, выполняет
// один выбранный пункт через dispatchAction. noChangesChoice допустим только
// при NoChanges без Blockers, после всех решений; для Mode=app.Update
// дополнительно обязательно RevisionReady=true. До этого NoChanges не
// разрешает объявлять отсутствие изменений или завершать операцию как пустую.
// При Actions.PrepareRevision меню содержит actionRevision: выбранный пункт
// вызывает prepareRevision. Автоматической подготовки и повторов после ошибки
// нет. После вызова и показа его ошибок цикл обязательно получает новый
// Operation.Snapshot и показывает его до новой оценки NoChanges/Blockers.
// При RevisionReady=false сохраняет обычное меню новых Actions; следующая
// попытка подготовки возможна только новым выбором разрешённого actionRevision.
// После Installed показывает результат и terminalChoice.
// Ошибка представления запрещает пункт записи и даёт retry/main/exit, если
// они разрешены app, через presentationFailureChoice. Ошибки showSnapshot
// и dispatchAction разделяет isStreamFailure: поток прекращает цикл целиком,
// а проблему представления сначала показывает PrintError в исправный поток.
func (d *dialog) operationLoop() (route, error) {
	for {
		snapshot := d.operation.Snapshot()
		next, err := routePreview, d.showSnapshot(snapshot)
		if err == nil && snapshot.WriteResult != nil && snapshot.WriteResult.Installed {
			if err = d.showWriteResult(snapshot, snapshot.Blockers); err == nil {
				return d.terminalChoice(snapshot)
			}
		}
		if err == nil {
			complete := !snapshot.Actions.DecideFields && !snapshot.Actions.DecidePlacement
			if snapshot.Plan != nil {
				complete = complete && len(snapshot.FieldAnswered) == len(snapshot.Plan.Fields)
			}
			for _, answered := range snapshot.FieldAnswered {
				complete = complete && answered
			}
			complete = complete && len(snapshot.PlacementAnswered) == len(snapshot.PlacementProposals)
			for _, answered := range snapshot.PlacementAnswered {
				complete = complete && answered
			}
			for _, issue := range snapshot.Blockers {
				complete = complete && issue == nil
			}
			if snapshot.NoChanges && complete && (snapshot.Mode != app.Update || snapshot.RevisionReady) {
				return d.noChangesChoice(snapshot)
			}
			var selected action
			selected, err = d.operationChoice(availableActions(snapshot))
			if err == nil {
				next, err = d.dispatchAction(snapshot, selected)
			}
		}
		if err != nil {
			if isStreamFailure(err) {
				d.operation.Cancel()
				return routeExit, err
			}
			if failure := PrintError(d.output, err); failure != nil {
				d.operation.Cancel()
				return routeExit, failure
			}
			next, err = d.presentationFailureChoice(snapshot)
		}
		if err != nil || next != routePreview {
			return next, err
		}
	}
}

// operationChoice связывает нумерацию только с переданным списком actions;
// повтор неверного номера не меняет список. Не хранит номера между снимками.
func (d *dialog) operationChoice(actions []action) (action, error) {
	labels := make([]string, len(actions))
	for index, value := range actions {
		labels[index] = actionLabel(value)
	}
	index, err := d.choose("Доступные действия:", labels)
	if err != nil {
		return "", err
	}
	return actions[index], nil
}

// dispatchAction только направляет один пункт в отдельный диалог. Снимок
// и его Token относятся к показанному меню; повторный выбор требует нового.
// actionRevision при snapshot.Actions.PrepareRevision вызывает prepareRevision
// ровно один раз и возвращает routePreview; ошибка потока прекращает диалог.
// Навигация не вычисляет предметную доступность и не выполняет запись сама.
func (d *dialog) dispatchAction(snapshot app.Snapshot, selected action) (route, error) {
	allowed := false
	for _, value := range availableActions(snapshot) {
		allowed = allowed || value == selected
	}
	if !allowed {
		return routePreview, nil
	}
	var err error
	switch selected {
	case actionCourse:
		err = d.editCourse(snapshot)
	case actionUnit:
		err = d.editUnit(snapshot)
	case actionLessonWeight:
		err = d.editLessonWeight(snapshot)
	case actionGroupWeights:
		err = d.editGroupWeights(snapshot)
	case actionSectionWeights:
		err = d.editSectionWeights(snapshot)
	case actionNewSection:
		err = d.createSection(snapshot)
	case actionMove:
		err = d.moveLesson(snapshot)
	case actionFlat:
		err = d.useFlatPlacement(snapshot)
	case actionField:
		err = d.decideField(snapshot)
	case actionPlacement:
		err = d.decidePlacement(snapshot)
	case actionRevision:
		err = d.prepareRevision(snapshot)
	case actionDestination:
		err = d.chooseDestination(snapshot)
	case actionSave:
		return d.reviewSaving(snapshot)
	case actionUnused:
		err = d.showUnused(snapshot)
	case actionRetry:
		err = d.retry()
	case actionRestart, actionMain, actionExit:
		d.operation.Cancel()
		if selected == actionRestart {
			return routeRestart, nil
		}
		if selected == actionMain {
			return routeMain, nil
		}
		return routeExit, nil
	}
	return routePreview, err
}
