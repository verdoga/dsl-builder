package console

import (
	"bufio"
	"io"

	"github.com/verdoga/dsl-builder/app"
	"github.com/verdoga/dsl-builder/diagnostic"
)

// dialog хранит только потоки и текущую операцию для гарантированной отмены.
type dialog struct {
	// input — единый буфер строк на весь запуск, без ограничения Scanner в 64 КБ.
	input *bufio.Reader
	// output — меню, вопросы, превью и последовательная диагностика операции.
	output io.Writer
	// baseDir — явный полный базовый каталог из main; не читается из окружения.
	baseDir string
	// operation — текущий app.Operation; nil до полного ввода начальных данных.
	operation *app.Operation
}

// Run проверяет ненулевые потоки, непустые baseDir и buildVersion, затем
// передаёт управление showWelcome и mainLoop. Не проверяет файловую систему.
// Версия берётся только из buildVersion; отсутствующую не заменяет v1.0.0.
// arguments никогда не становятся параметрами операции или настройками.
// При любом завершении вызывает Cancel текущей операции. EOF до Enter
// отменяет даже частично прочитанную строку и завершает Run без новой записи.
// Остальные ошибки потока возвращает main, не повторяя ошибочный вывод.
// Предметные ошибки после успешного показа остаются в диалоге продолжения.
// Не закрывает переданные потоки, не вызывает os.Exit и не меняет их режимы.
func Run(input io.Reader, output io.Writer, baseDir string, buildVersion string, arguments []string) error {
	if input == nil || output == nil || baseDir == "" || buildVersion == "" {
		return &diagnostic.Error{
			Code:   CodeConfiguration,
			Reason: "Для запуска нужны потоки ввода и вывода, базовый каталог и версия сборки.",
		}
	}
	d := &dialog{input: bufio.NewReader(input), output: output, baseDir: baseDir}
	defer func() { d.operation.Cancel() }()
	if err := d.showWelcome(buildVersion, arguments); err != nil {
		return err
	}
	err := d.mainLoop()
	if err == io.EOF {
		return nil
	}
	return err
}

// showWelcome выводит DSL-Builder и переданную версию; при аргументах пишет
// «Работа выполняется через интерактивное меню; аргументы не используются».
func (d *dialog) showWelcome(buildVersion string, arguments []string) error {
	if err := d.writeText("DSL-Builder " + displayValue(buildVersion) + "\n\n"); err != nil {
		return err
	}
	if len(arguments) != 0 {
		return d.writeText("Работа выполняется через интерактивное меню; аргументы не используются\n\n")
	}
	return nil
}

// mainLoop обслуживает только главное меню и запуск выбранного режима.
// После возврата и перед выходом отменяет незаписанное через app.Cancel.
func (d *dialog) mainLoop() error {
	defer func() { d.operation.Cancel() }()
	for {
		choice, err := d.mainChoice()
		if err != nil {
			return err
		}
		var mode app.Mode
		switch choice {
		case 1:
			mode = app.Create
		case 2:
			mode = app.Update
		case 3:
			mode = app.Add
		case 4:
			return nil
		}
		next, err := d.runMode(mode)
		d.operation.Cancel()
		d.operation = nil
		if err != nil {
			return err
		}
		if next == routeExit {
			return nil
		}
	}
}

// mainChoice показывает четыре отдельных пункта: создать, обновить, добавить,
// выход. Возвращает номер 1–4; неверный ввод повторяет именно это меню.
func (d *dialog) mainChoice() (int, error) {
	index, err := d.choose("Что вы хотите сделать?", []string{
		"Создать новый модуль",
		"Обновить существующий модуль",
		"Добавить уроки в существующий модуль",
		"Выход",
	})
	if err != nil {
		return 0, err
	}
	return index + 1, nil
}

// runMode соединяет ввод одного сценария, Analyze(time.Now()) и operationLoop.
// routeRestart означает отмену прежней Operation и полный новый ввод того же
// режима; ошибки конструктора показываются и дают повтор ввода/главное меню/выход.
// Сбой анализа не отменяет редактируемый каталог: действия определяет Snapshot.
func (d *dialog) runMode(mode app.Mode) (route, error) {
	for {
		d.operation.Cancel()
		d.operation = nil
		var err error
		switch mode {
		case app.Create:
			d.operation, err = d.startCreate()
		case app.Update:
			d.operation, err = d.startUpdate()
		case app.Add:
			d.operation, err = d.startAdd()
		default:
			return routeExit, &diagnostic.Error{Code: CodeConfiguration, Reason: "Не выбран допустимый режим диалога."}
		}
		if err != nil {
			return routeExit, err
		}
		if d.operation == nil {
			next, err := d.startFailureChoice()
			if err != nil || next != routeRestart {
				return next, err
			}
			continue
		}
		if err := d.analyze(); err != nil {
			return routeExit, err
		}
		for {
			next, err := d.operationLoop()
			if err != nil || next == routeMain || next == routeExit {
				return next, err
			}
			if next == routeRestart {
				break
			}
		}
	}
}

// route — только направление возврата диалога, не стадия предметной операции.
type route int

const (
	// routePreview — снова показать актуальный снимок текущей операции.
	routePreview route = iota
	// routeRestart — начать тот же режим с новым вводом и новой Operation.
	routeRestart
	// routeMain — отменить незаписанное и перейти в главное меню.
	routeMain
	// routeExit — отменить незаписанное и завершить Run.
	routeExit
)
