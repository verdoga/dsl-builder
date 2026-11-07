// Package main задаёт точку входа в корне проекта: запуск и завершение процесса.
// Псевдокод API v1; диалог и создание app.Operation принадлежат console.
package main

import (
	"github.com/verdoga/dsl-builder/console"
	"os"
)

const (
	// exitSuccess — штатное завершение диалога, включая отмену при EOF.
	exitSuccess = 0
	// exitFailure — ошибка подготовки запуска, потока либо вывода диагностики.
	exitFailure = 1
)

// main получает полный baseDir через os.Getwd один раз на весь запуск.
// Это явная база относительных интерактивных путей, не каталог ресурсов.
// После успешного получения базы передаёт управление точному стыку
// console.Run(os.Stdin, os.Stdout, baseDir, buildVersion, os.Args[1:]).
// Аргументы передаются только для пояснения об интерактивном меню;
// режимы, пути и настройки из аргументов, конфигов и окружения не читаются.
// Run сам вызывает app.NewCreate/NewUpdate/NewAdd: фабрика и импорт app не нужны.
// nil от Run означает exitSuccess: числовой выход или штатное окончание ввода;
// отмену незаписанного до возврата выполняет console, без повторной отмены здесь.
// Ошибку os.Getwd либо Run однократно передаёт console.PrintError(os.Stderr, err).
// При ошибке PrintError повторного вывода нет; итоговый код также exitFailure.
// Завершает процесс через os.Exit только после возврата Run/PrintError.
// Стандартные потоки не закрывает; паники, скрытой паузы и перезапуска нет.
// Обычный запуск и двойной щелчок Windows используют один и тот же диалог.
func main() {
	baseDir, err := os.Getwd()
	if err == nil {
		err = console.Run(os.Stdin, os.Stdout, baseDir, buildVersion, os.Args[1:])
	}
	if err != nil {
		_ = console.PrintError(os.Stderr, err)
		os.Exit(exitFailure)
	}
	os.Exit(exitSuccess)
}
