package app

import (
	"errors"
	"io"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/module"
	"github.com/verdoga/dsl-builder/moduleio"
	"github.com/verdoga/dsl-builder/safewrite"
)

// newWriteCallback возвращает safewrite.WriteFunc, замыкающий только закрытый
// неизменяемый value и context. Callback вызывает moduleio.Write(dst, value,
// context) и joinDiagnostics; dst не закрывает и не сохраняет. ZIP writer
// завершает moduleio. Подтверждение не переносится в callback как диалог.
func newWriteCallback(value *module.Module, context diagnostic.Context) safewrite.WriteFunc {
	return func(dst io.Writer) error {
		return joinDiagnostics(moduleio.Write(dst, value, context))
	}
}

// verifyCallback вызывает moduleio.Verify(path), затем joinDiagnostics.
// Все дескрипторы закрывает moduleio до возврата; цель не изменяется.
func verifyCallback(path string) error {
	return joinDiagnostics(moduleio.Verify(path))
}

// joinDiagnostics переводит ненулевые *diagnostic.Error в []error и вызывает
// стандартный errors.Join; nil не превращается в ненулевой typed-nil error.
// Сохраняет структурированные дочерние ошибки и Causes, не склеивает их в текст.
func joinDiagnostics(issues []*diagnostic.Error) error {
	errorsToJoin := make([]error, 0, len(issues))
	for _, issue := range issues {
		if issue != nil {
			errorsToJoin = append(errorsToJoin, issue)
		}
	}
	return errors.Join(errorsToJoin...)
}
