package safewrite

import (
	"os"
	"path/filepath"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// reserveBackup выделяет уникальное служебное имя в том же каталоге и
// закрывает его дескриптор. Не использует фиксированное имя или чужой файл;
// ненужный резервный placeholder принадлежит последующей очистке.
func (t *transaction) reserveBackup() *diagnostic.Error {
	if t.backupPath != "" || t.originalMoved || t.installed {
		return t.failure("safewrite.reserveBackup", "Резервное имя уже выделено или замена уже выполнена", t.target, nil)
	}
	file, err := os.CreateTemp(filepath.Dir(t.target), ".safewrite-backup-*")
	if err != nil {
		return t.failure("safewrite.reserveBackup", "Не удалось выделить резервное имя рядом с целью", t.target, err)
	}
	t.backupPath = file.Name()
	if err := file.Close(); err != nil {
		return t.failure("safewrite.reserveBackup", "Не удалось закрыть файл резервного имени", t.backupPath, err)
	}
	return nil
}

// preserveOriginal перемещает имеющуюся цель в принадлежащее транзакции
// резервное имя только при verified и успешном checkTarget. Исходные байты
// не переписываются. При успехе originalMoved становится true.
// При отсутствии исходника шаг не нужен; сбой не разрешает установку.
func (t *transaction) preserveOriginal() *diagnostic.Error {
	if t.original == nil {
		return nil
	}
	if !t.verified || t.candidate != nil || t.originalMoved || t.installed {
		return t.failure("safewrite.preserveOriginal", "Исходник нельзя сохранять до подготовки кандидата или после замены", t.target, nil)
	}
	if t.backupPath == "" || t.backupPath == t.target || t.backupPath == t.candidatePath ||
		filepath.Dir(t.backupPath) != filepath.Dir(t.target) {
		return t.failure("safewrite.preserveOriginal", "Не выделено отдельное резервное имя в каталоге цели", t.backupPath, nil)
	}
	if failure := t.checkTarget(); failure != nil {
		return failure
	}
	placeholder, err := os.Lstat(t.backupPath)
	if err != nil {
		return t.failure("safewrite.preserveOriginal", "Не удалось проверить резервное имя", t.backupPath, err)
	}
	if !placeholder.Mode().IsRegular() || placeholder.Size() != 0 {
		return t.failure("safewrite.preserveOriginal", "Резервное имя больше не является пустым служебным файлом", t.backupPath, nil)
	}
	// Закрытый placeholder освобождает имя для переименования в Windows.
	if err := os.Remove(t.backupPath); err != nil {
		return t.failure("safewrite.preserveOriginal", "Не удалось освободить резервное имя", t.backupPath, err)
	}
	if err := os.Rename(t.target, t.backupPath); err != nil {
		return t.failure("safewrite.preserveOriginal", "Не удалось переместить исходный файл в резерву", t.target, err)
	}
	t.originalMoved = true
	return nil
}

// installCandidate переносит проверенный закрытый кандидат по свободному
// target. Для прежней цели требуется сохранённый исходник; для новой —
// подтверждённое отсутствие цели. Не удаляет резервную версию.
// Устанавливает installed только после успешного завершения файловой операции.
// Все переименования ограничены одним каталогом и файловой системой.
func (t *transaction) installCandidate() *diagnostic.Error {
	if !t.verified || t.candidate != nil || t.candidatePath == "" || t.installed {
		return t.failure("safewrite.installCandidate", "Кандидат не подготовлен к установке или уже установлен", t.candidatePath, nil)
	}
	if t.candidatePath == t.target || t.candidatePath == t.backupPath ||
		filepath.Dir(t.candidatePath) != filepath.Dir(t.target) {
		return t.failure("safewrite.installCandidate", "Кандидат должен иметь отдельное имя в каталоге цели", t.candidatePath, nil)
	}
	if t.original != nil {
		if !t.originalMoved || t.backupPath == "" || t.backupPath == t.target ||
			filepath.Dir(t.backupPath) != filepath.Dir(t.target) {
			return t.failure("safewrite.installCandidate", "Исходный файл не сохранён в резерву рядом с целью", t.backupPath, nil)
		}
		backup, err := os.Lstat(t.backupPath)
		if err != nil {
			return t.failure("safewrite.installCandidate", "Не удалось проверить сохранённый исходный файл", t.backupPath, err)
		}
		if !backup.Mode().IsRegular() || !os.SameFile(t.original, backup) ||
			backup.Size() != t.original.Size() || !backup.ModTime().Equal(t.original.ModTime()) {
			return t.failure("safewrite.installCandidate", "Сохранённый исходный файл изменился; установка остановлена", t.backupPath, nil)
		}
	} else if failure := t.checkTarget(); failure != nil {
		return failure
	}
	if _, err := os.Lstat(t.target); err == nil {
		return t.failure("safewrite.installCandidate", "Целевой путь занят; посторонний файл не заменён", t.target, nil)
	} else if !os.IsNotExist(err) {
		return t.failure("safewrite.installCandidate", "Не удалось проверить свободный целевой путь", t.target, err)
	}
	candidate, err := os.Lstat(t.candidatePath)
	if err != nil {
		return t.failure("safewrite.installCandidate", "Не удалось проверить файл кандидата перед установкой", t.candidatePath, err)
	}
	if !candidate.Mode().IsRegular() {
		return t.failure("safewrite.installCandidate", "Кандидат больше не является обычным файлом", t.candidatePath, nil)
	}
	if err := os.Rename(t.candidatePath, t.target); err != nil {
		return t.failure("safewrite.installCandidate", "Не удалось установить проверенный результат", t.target, err)
	}
	t.candidatePath = ""
	t.installed = true
	return nil
}

// restoreOriginal восстанавливает исходник из backupPath после сбоя установки.
// Не заменяет неожиданно появившийся посторонний файл и не удаляет единственную
// сохранённую копию. При успешном восстановлении снимает originalMoved.
// При невозможности восстановления сохраняет резерву и доступный кандидат,
// возвращает их точные пути и причину. Для первоначально отсутствовавшей цели
// исходник не создаётся; непроверенный кандидат целью не становится.
func (t *transaction) restoreOriginal() *diagnostic.Error {
	if t.original == nil || !t.originalMoved || t.installed {
		return nil
	}
	if t.backupPath == "" || t.backupPath == t.target || t.backupPath == t.candidatePath ||
		filepath.Dir(t.backupPath) != filepath.Dir(t.target) {
		return t.failure("safewrite.restoreOriginal", "Резервная копия не имеет отдельного имени в каталоге цели", t.backupPath, nil)
	}
	if _, err := os.Lstat(t.target); err == nil {
		return t.failure("safewrite.restoreOriginal", "Целевой путь занят; исходник и кандидат сохранены для восстановления", t.target, nil)
	} else if !os.IsNotExist(err) {
		return t.failure("safewrite.restoreOriginal", "Не удалось проверить целевой путь для восстановления исходника", t.target, err)
	}
	backup, err := os.Lstat(t.backupPath)
	if err != nil {
		return t.failure("safewrite.restoreOriginal", "Не удалось проверить резервную копию исходника", t.backupPath, err)
	}
	if !backup.Mode().IsRegular() || !os.SameFile(t.original, backup) ||
		backup.Size() != t.original.Size() || !backup.ModTime().Equal(t.original.ModTime()) {
		return t.failure("safewrite.restoreOriginal", "Резервная копия исходника изменилась; требуется ручное восстановление", t.backupPath, nil)
	}
	if err := os.Rename(t.backupPath, t.target); err != nil {
		return t.failure("safewrite.restoreOriginal", "Не удалось восстановить исходник; исходник и кандидат сохранены", t.backupPath, err)
	}
	t.backupPath = ""
	t.originalMoved = false
	return nil
}
