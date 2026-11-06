package safewrite

import (
	"os"
	"path/filepath"

	"github.com/verdoga/dsl-builder/diagnostic"
)

// transaction — приватное состояние одной операции; хранит владение файлами
// для отката и очистки, не представляет модель или план содержимого.
type transaction struct {
	// target — неизменяемый полный путь назначения.
	target string

	// original — исходное состояние цели; nil означает её отсутствие.
	original os.FileInfo

	// candidatePath — уникальный путь кандидата в каталоге target.
	candidatePath string

	// candidate — принадлежащий транзакции открытый файл; nil после закрытия.
	candidate *os.File

	// backupPath — уникальное резервное имя рядом с целью; пусто до выделения.
	backupPath string

	// originalMoved — исходник успешно перемещён в backupPath и ещё не
	// восстановлен или удалён после успешной установки результата.
	originalMoved bool

	// verified — запись, Sync, закрытие и проверка кандидата успешны.
	verified bool

	// installed — кандидат успешно установлен по target.
	installed bool
}

// inspectTarget проверяет путь и разрешение перезаписи без записи файлов.
// Различает отсутствие цели и ошибку доступа; существующая цель должна быть
// обычным файлом, а не каталогом или символической ссылкой. Возвращает её
// сведения для контроля идентичности перед заменой, либо nil при отсутствии.
func inspectTarget(target string, replaceExisting bool) (os.FileInfo, *diagnostic.Error) {
	t := transaction{target: target}
	if !filepath.IsAbs(target) {
		return nil, t.failure("safewrite.inspectTarget", "Требуется полный путь целевого файла", target, nil)
	}
	info, err := os.Lstat(target)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, t.failure("safewrite.inspectTarget", "Не удалось проверить целевой файл", target, err)
		}
		directory := filepath.Dir(target)
		parent, parentErr := os.Stat(directory)
		if parentErr != nil {
			return nil, t.failure("safewrite.inspectTarget", "Не удалось проверить каталог назначения", directory, parentErr)
		}
		if !parent.IsDir() {
			return nil, t.failure("safewrite.inspectTarget", "Путь каталога назначения не является каталогом", directory, nil)
		}
		return nil, nil
	}
	t.original = info
	if !info.Mode().IsRegular() {
		return nil, t.failure("safewrite.inspectTarget", "Цель должна быть обычным файлом, а не каталогом или символической ссылкой", target, nil)
	}
	if !replaceExisting {
		return info, t.failure("safewrite.inspectTarget", "Перезапись существующего целевого файла не подтверждена", target, nil)
	}
	return info, nil
}

// checkTarget проверяет перед заменой, что цель не появилась, не исчезла
// и не сменила известную идентичность, размер или время изменения с начала
// вызова. Изменение требует нового анализа в app. Это дополнительная проверка,
// а не гарантия исключения гонок с другими процессами.
func (t *transaction) checkTarget() *diagnostic.Error {
	current, err := os.Lstat(t.target)
	if err != nil {
		if os.IsNotExist(err) {
			if t.original == nil {
				return nil
			}
			return t.failure("safewrite.checkTarget", "Исходный целевой файл исчез; требуется повторный анализ", t.target, err)
		}
		return t.failure("safewrite.checkTarget", "Не удалось повторно проверить целевой файл", t.target, err)
	}
	if t.original == nil {
		return t.failure("safewrite.checkTarget", "Целевой файл появился после анализа; требуется повторный анализ", t.target, nil)
	}
	if !current.Mode().IsRegular() || !os.SameFile(t.original, current) ||
		t.original.Size() != current.Size() || !t.original.ModTime().Equal(current.ModTime()) {
		return t.failure("safewrite.checkTarget", "Целевой файл изменился после анализа; требуется повторный анализ", t.target, nil)
	}
	return nil
}

// result создаёт снимок текущих мест кандидата и исходника без файлового ввода-вывода.
func (t *transaction) result() Result {
	result := Result{TargetPath: t.target, Installed: t.installed}
	if !t.installed {
		result.TemporaryPath = t.candidatePath
	}
	if t.originalMoved {
		result.OriginalPath = t.backupPath
	} else if t.original != nil && !t.installed {
		result.OriginalPath = t.target
	}
	return result
}
