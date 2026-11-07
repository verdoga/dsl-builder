package preview

import (
	"fmt"
	"strings"

	"github.com/verdoga/dsl-builder/resourceusage"
)

// Resources показывает готовые Total, Used и Unused без повторного учёта.
// Всегда подписывает область: ресурсы обрабатываемых исходников текущей
// операции, включая resourceDirs, явные внешние и сопутствующие файлы.
// Поэтому обновление/добавление не приписывает статистике каталоги старых
// уроков архива. Неиспользованные файлы не обозначает ошибкой.
func Resources(report resourceusage.Report) string {
	return fmt.Sprintf("Область: ресурсы обрабатываемых исходников текущей операции, включая resourceDirs, явно подключённые внешние файлы и проверяемые сопутствующие файлы.\nВсего файлов: %d\nИспользовано: %d\nНе использовано: %d\n", report.Total, report.Used, report.Unused)
}

// UnusedResources показывает ту же область и счётчики через Resources,
// затем все UnusedPaths целиком с нумерацией с 1 в переданном порядке.
// При пустом списке пишет «Неиспользованных файлов нет».
// Уточняет, что эти файлы не будут встроены из текущих исходников;
// не обещает отсутствие их прежних копий внутри старых уроков архива.
// Не открывает пути и не добавляет пункт возврата в меню.
func UnusedResources(report resourceusage.Report) string {
	var text strings.Builder
	text.WriteString("НЕИСПОЛЬЗОВАННЫЕ ФАЙЛЫ\n\n")
	text.WriteString(Resources(report))
	text.WriteByte('\n')
	if len(report.UnusedPaths) == 0 {
		text.WriteString("Неиспользованных файлов нет\n")
		return text.String()
	}
	for i, path := range report.UnusedPaths {
		fmt.Fprintf(&text, "%d. %s\n", i+1, quotedText(path))
	}
	text.WriteString("\nЭти файлы не будут встроены из текущих исходников.\n")
	return text.String()
}
