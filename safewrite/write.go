// Package safewrite выполняет файловую транзакцию после окончательного
// подтверждения в app. Содержимое файла и его проверку определяют callbacks;
// пакет не знает ZIP, JSON, модель модуля, меню и правила имени архива.
// Используется стандартная библиотека Go; внешние помощники не нужны.
package safewrite

import "github.com/verdoga/dsl-builder/diagnostic"

// Write координирует небольшие этапы подготовки, замены, отката и очистки.
// Вызывается только для актуального подтверждённого плана. Подтверждение
// охватывает временные записи; самостоятельного диалога здесь нет.
// target — полный подготовленный путь; его каталог уже существует.
// replaceExisting разрешает замену существующей цели, подтверждённую в app;
// false запрещает перезапись. Отсутствие цели допустимо в обоих случаях.
// Нулевые callbacks и недопустимая цель дают ошибку до создания файлов.
// Вызывающий код обеспечивает отсутствие параллельных изменений цели,
// её каталога и служебных файлов на протяжении вызова; пакет не является
// межпроцессным менеджером блокировок. Все исходные файловые дескрипторы,
// мешающие переименованию в Windows, должны быть закрыты до вызова.
//
// Цель не меняется до успешной записи, Sync, закрытия и проверки кандидата.
// Исходник сохраняется рядом до завершения замены; сбой замены вызывает
// попытку восстановления. Неудача отката сохраняет обе доступные версии.
// Ошибки подготовки также запускают очистку ненужных временных файлов.
// Успех означает Installed и пустой список ошибок, включая ошибки очистки.
// Installed с ошибкой очистки означает уже записанный результат: app
// сообщает неполное завершение и оставшиеся пути, не повторяет запись вслепую.
// Ошибки возвращаются в порядке: основной сбой, откат, очистка; исходные
// причины не теряются. Частично подготовленный файл никогда не становится целью.
// В Windows замена через резервное имя не обещает непрерывной видимости цели
// читателям или устойчивости к отключению питания и аварии процесса.
func Write(target string, replaceExisting bool, write WriteFunc, verify VerifyFunc) (result Result, failures []*diagnostic.Error) {
	t := transaction{target: target}
	defer func() {
		failures = append(failures, t.cleanup()...)
		result = t.result()
	}()

	original, failure := inspectTarget(target, replaceExisting)
	t.original = original
	if failure != nil {
		failures = append(failures, failure)
		return
	}
	if write == nil {
		failures = append(failures, t.failure("safewrite.Write", "Не задана функция записи временного файла", target, nil))
		return
	}
	if verify == nil {
		failures = append(failures, t.failure("safewrite.Write", "Не задана функция проверки временного файла", target, nil))
		return
	}
	if failure := t.createCandidate(); failure != nil {
		failures = append(failures, failure)
		return
	}

	if failure := t.writeCandidate(write); failure != nil {
		failures = append(failures, failure)
	}
	failures = append(failures, t.closeCandidate()...)
	if len(failures) != 0 {
		return
	}
	if failure := t.verifyCandidate(verify); failure != nil {
		failures = append(failures, failure)
		return
	}

	if t.original != nil {
		if failure := t.reserveBackup(); failure != nil {
			failures = append(failures, failure)
			return
		}
		if failure := t.preserveOriginal(); failure != nil {
			failures = append(failures, failure)
			return
		}
	}
	if failure := t.installCandidate(); failure != nil {
		failures = append(failures, failure)
		if failure := t.restoreOriginal(); failure != nil {
			failures = append(failures, failure)
		}
	}
	return
}
