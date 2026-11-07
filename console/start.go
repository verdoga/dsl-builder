package console

import (
	"time"

	"github.com/verdoga/dsl-builder/app"
)

// startCreate локально собирает каталог технических JSON, обязательный курс
// и необязательный юнит. Только полный ввод вызывает app.NewCreate с baseDir.
// Пути передаются буквально, включая внешнюю пару кавычек, для app.inputPath.
// Отмена любого вопроса отбрасывает все ответы и возвращает nil без ошибки.
func (d *dialog) startCreate() (*app.Operation, error) {
	source, accepted, err := d.readValue("Каталог с техническими JSON:", false)
	if err != nil || !accepted {
		return nil, err
	}
	course, accepted, err := d.readValue("Название курса:", false)
	if err != nil || !accepted {
		return nil, err
	}
	unit, accepted, err := d.readValue("Название юнита (Enter — без юнита):", true)
	if err != nil || !accepted {
		return nil, err
	}
	operation, issues := app.NewCreate(d.baseDir, source, course, unit)
	if len(issues) != 0 {
		return nil, d.showIssues(issues)
	}
	return operation, nil
}

// startUpdate локально собирает путь ZIP и ровно одного технического JSON,
// затем вызывает app.NewUpdate. Содержимое и вид путей проверяет только app.
// Отмена любого вопроса не создаёт Operation; ошибки конструктора показывает
// через showIssues, после чего возвращает nil для startFailureChoice.
func (d *dialog) startUpdate() (*app.Operation, error) {
	archive, accepted, err := d.readValue("Путь к существующему ZIP:", false)
	if err != nil || !accepted {
		return nil, err
	}
	source, accepted, err := d.readValue("Путь к одному техническому JSON:", false)
	if err != nil || !accepted {
		return nil, err
	}
	operation, issues := app.NewUpdate(d.baseDir, archive, source)
	if len(issues) != 0 {
		return nil, d.showIssues(issues)
	}
	return operation, nil
}

// startAdd локально собирает путь ZIP и путь JSON либо каталога, передаёт
// полную группу app.NewAdd. Контракт отмены и ошибок тот же, что startUpdate.
// startCreate также показывает ошибки конструктора без частичного запуска.
func (d *dialog) startAdd() (*app.Operation, error) {
	archive, accepted, err := d.readValue("Путь к существующему ZIP:", false)
	if err != nil || !accepted {
		return nil, err
	}
	source, accepted, err := d.readValue("Путь к техническому JSON или каталогу:", false)
	if err != nil || !accepted {
		return nil, err
	}
	operation, issues := app.NewAdd(d.baseDir, archive, source)
	if len(issues) != 0 {
		return nil, d.showIssues(issues)
	}
	return operation, nil
}

// startFailureChoice после отменённого/неуспешного первичного ввода предлагает
// новый ввод того же режима, главное меню и выход; Retry без Operation нет.
func (d *dialog) startFailureChoice() (route, error) {
	selected, err := d.choose("Продолжение работы:", []string{
		"Начать операцию заново",
		"Вернуться в главное меню",
		"Выход",
	})
	if err != nil {
		return routeExit, err
	}
	switch selected {
	case 0:
		return routeRestart, nil
	case 1:
		return routeMain, nil
	default:
		return routeExit, nil
	}
}

// analyze вызывает только Operation.Analyze(time.Now()), показывает все
// возвращённые ошибки и оставляет выбор продолжения Snapshot.Actions.
// Не печатает вымышленные промежуточные счётчики успешных этапов.
func (d *dialog) analyze() error {
	return d.showIssues(d.operation.Analyze(time.Now()))
}

// retry при Actions.Retry вызывает Operation.Retry(time.Now()) и показывает
// ошибки; следующий экран полностью заменяет старые снимки и нумерацию.
// После recovery-файлов/Installed доступность не восстанавливает самостоятельно.
func (d *dialog) retry() error {
	if !d.operation.Snapshot().Actions.Retry {
		return nil
	}
	return d.showIssues(d.operation.Retry(time.Now()))
}
