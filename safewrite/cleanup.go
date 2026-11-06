package safewrite

import (
	"os"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// cleanup удаляет только принадлежащие транзакции ненужные служебные файлы.
// После установки удаляет резерву; после сохранения или восстановления
// исходника удаляет кандидат и пустое резервное имя. После неудачного отката
// сохраняет исходник и кандидат для восстановления, не считает их мусором.
// До удаления закрывает ещё принадлежащие транзакции дескрипторы.
// Не прекращает остальные безопасные попытки очистки после одной ошибки;
// возвращает все ошибки с путями оставшихся файлов, не скрывая основной сбой.
func (t *transaction) cleanup() []*diagnostic.Error {
	var failures []*diagnostic.Error
	if t.candidate != nil {
		file := t.candidate
		t.candidate = nil
		if err := file.Close(); err != nil {
			failures = append(failures, t.failure("safewrite.cleanup.closeCandidate", "Не удалось закрыть временный файл при очистке", file.Name(), err))
		}
	}

	// Сохранённый исходник и кандидат нужны для ручного восстановления,
	// пока установка и откат не завершились успешно.
	if t.originalMoved && !t.installed {
		return failures
	}
	if !t.installed && t.candidatePath != "" {
		path := t.candidatePath
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			failures = append(failures, t.failure("safewrite.cleanup.removeCandidate", "Не удалось удалить ненужный временный файл", path, err))
		} else {
			t.candidatePath = ""
		}
	}
	if t.backupPath != "" {
		path := t.backupPath
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			failures = append(failures, t.failure("safewrite.cleanup.removeBackup", "Не удалось удалить ненужный резервный файл", path, err))
		} else {
			t.backupPath = ""
			t.originalMoved = false
		}
	}
	// Каждая ошибка очистки сообщает состояние после всех безопасных попыток,
	// включая успешное удаление другого служебного файла.
	for i, failure := range failures {
		failures[i] = t.failure(failure.Code, failure.Reason, failure.Context.Path, failure.Cause)
	}
	return failures
}

// failure оформляет ошибку этапа через существующий diagnostic.Error:
// Code — стабильный код safewrite с именем этапа, Reason — русская причина,
// Context.Path — path, Cause — исходная ошибка, включая ошибку callback.
// Содержимое callback-диагностики доступно через цепочку Cause без потери ID
// и координат. Related содержит известные пути исходника и кандидата;
// Details поясняет, где исходник, где результат и установлен ли результат.
// Собственного типа ошибки пакет не вводит.
func (t *transaction) failure(code string, reason string, path string, cause error) *diagnostic.Error {
	state := t.result()
	resultPath := state.TemporaryPath
	installed := "нет"
	readiness := "Подготовка и проверка не завершены"
	if t.verified {
		readiness = "Запись и проверка завершены"
	}
	if state.Installed {
		resultPath = state.TargetPath
		installed = "да"
	}
	failure := &diagnostic.Error{
		Code:    code,
		Reason:  reason,
		Context: diagnostic.Context{Path: path},
		Cause:   cause,
		Details: []diagnostic.Detail{
			{Label: "Целевой путь", Value: state.TargetPath},
			{Label: "Путь исходного файла", Value: state.OriginalPath},
			{Label: "Путь результата", Value: resultPath},
			{Label: "Результат установлен", Value: installed},
			{Label: "Состояние результата", Value: readiness},
		},
	}
	if state.OriginalPath != "" {
		failure.Related = append(failure.Related, diagnostic.Context{Path: state.OriginalPath})
	}
	if resultPath != "" && resultPath != state.OriginalPath {
		failure.Related = append(failure.Related, diagnostic.Context{Path: resultPath})
	}
	if t.backupPath != "" {
		failure.Details = append(failure.Details, diagnostic.Detail{Label: "Резервное имя", Value: t.backupPath})
	}
	return failure
}
