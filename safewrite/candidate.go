package safewrite

import (
	"io"
	"os"
	"path/filepath"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// createCandidate эксклюзивно создаёт уникальный временный файл в каталоге
// target и сохраняет его дескриптор. Не создаёт каталогов и не изменяет цель.
func (t *transaction) createCandidate() *diagnostic.Error {
	if t.candidate != nil || t.candidatePath != "" || t.installed {
		return t.failure("safewrite.createCandidate", "Временный файл уже создан или результат установлен", t.target, nil)
	}
	file, err := os.CreateTemp(filepath.Dir(t.target), ".safewrite-candidate-*")
	if err != nil {
		return t.failure("safewrite.createCandidate", "Не удалось создать временный файл рядом с целью", t.target, err)
	}
	t.candidatePath = file.Name()
	t.candidate = file
	t.verified = false
	return nil
}

// writeCandidate передаёт поток кандидата write; формат полностью принадлежит
// callback. Ошибка callback запрещает установку кандидата.
func (t *transaction) writeCandidate(write WriteFunc) *diagnostic.Error {
	t.verified = false
	if t.candidate == nil {
		return t.failure("safewrite.writeCandidate", "Временный файл не открыт для записи", t.candidatePath, nil)
	}
	if write == nil {
		return t.failure("safewrite.writeCandidate", "Не задана функция записи временного файла", t.candidatePath, nil)
	}
	// Передаётся только Writer: закрытие файла остаётся у транзакции.
	if err := write(struct{ io.Writer }{t.candidate}); err != nil {
		return t.failure("safewrite.writeCandidate", "Не удалось записать содержимое временного файла", t.candidatePath, err)
	}
	return nil
}

// closeCandidate выполняет Sync и закрывает файл, учитывая обе ошибки.
// Вызывается и после ошибки записи; ошибка Sync не отменяет попытку Close.
// Не оставляет владение дескриптором у callback; повторно закрытый файл
// повторно не закрывается. Ошибка любого завершения запрещает проверку и замену.
func (t *transaction) closeCandidate() []*diagnostic.Error {
	if t.candidate == nil {
		return nil
	}
	t.verified = false
	file := t.candidate
	syncErr := file.Sync()
	closeErr := file.Close()
	t.candidate = nil
	var failures []*diagnostic.Error
	if syncErr != nil {
		failures = append(failures, t.failure("safewrite.closeCandidate.sync", "Не удалось синхронизировать временный файл", t.candidatePath, syncErr))
	}
	if closeErr != nil {
		failures = append(failures, t.failure("safewrite.closeCandidate.close", "Не удалось закрыть временный файл", t.candidatePath, closeErr))
	}
	return failures
}

// verifyCandidate вызывает verify только для успешно записанного и закрытого
// файла. Только его успешное завершение устанавливает verified.
func (t *transaction) verifyCandidate(verify VerifyFunc) *diagnostic.Error {
	t.verified = false
	if t.candidate != nil || t.candidatePath == "" || t.installed {
		return t.failure("safewrite.verifyCandidate", "Временный файл не подготовлен к проверке", t.candidatePath, nil)
	}
	if verify == nil {
		return t.failure("safewrite.verifyCandidate", "Не задана функция проверки временного файла", t.candidatePath, nil)
	}
	// Write вызывает этот этап лишь после успешных записи, Sync и Close.
	if err := verify(t.candidatePath); err != nil {
		return t.failure("safewrite.verifyCandidate", "Не удалось проверить содержимое временного файла", t.candidatePath, err)
	}
	t.verified = true
	return nil
}
